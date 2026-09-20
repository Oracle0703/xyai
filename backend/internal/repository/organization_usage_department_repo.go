package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type organizationUsageExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// A global champion is necessarily one user's peak. Reuse the three materialized
// peak sets instead of aggregating the complete usage range a second time.
const organizationUsageChampionMetadataCTE = `,
report_champions AS MATERIALIZED (
    SELECT COALESCE(jsonb_object_agg(c.granularity, jsonb_build_object(
        'period_start', GREATEST(c.bucket_start, $5::date),
        'period_end', LEAST(c.bucket_end, $6::date),
        'partial', c.bucket_start < $5::date OR c.bucket_end > $6::date,
        'user_id', c.user_id, 'email', su.email, 'organization', su.organization,
        'requests', c.requests, 'input_tokens', c.input_tokens, 'output_tokens', c.output_tokens,
        'cache_creation_tokens', c.cache_creation_tokens, 'cache_read_tokens', c.cache_read_tokens,
        'total_tokens', c.total_tokens, 'actual_cost', c.actual_cost
    )), '{}'::jsonb) AS payload
    FROM (
        SELECT DISTINCT ON (peaks.granularity) peaks.*
        FROM (SELECT * FROM day_peak UNION ALL SELECT * FROM week_peak UNION ALL SELECT * FROM month_peak) peaks
        JOIN selected_users allowed_user ON allowed_user.user_id=peaks.user_id
        ORDER BY peaks.granularity, peaks.total_tokens DESC, peaks.actual_cost DESC,
            peaks.requests DESC, peaks.user_id ASC, peaks.bucket_start ASC
    ) c JOIN selected_users su ON su.user_id=c.user_id
)`

func (r *organizationUsageRepository) executor() organizationUsageExecutor {
	if r.query != nil {
		return r.query
	}
	return r.db
}

// The two markers are internal SQL template slots. Values are always bound;
// neither user input nor authorization IDs are interpolated into SQL text.
func (r *organizationUsageRepository) scopedQuery(query string, args []any) (string, []any) {
	if r.scope == nil {
		return query, args
	}
	if strings.Contains(query, "/* department_members */") {
		args = append(args, pq.Array(r.scopeIDs))
		query = strings.ReplaceAll(query, "/* department_members */", fmt.Sprintf("AND u.id=ANY($%d::bigint[])", len(args)))
	}
	if strings.Contains(query, "/* department_platform */") {
		args = append(args, r.filters.Platform)
		n := len(args)
		query = strings.ReplaceAll(query, "/* department_platform */", fmt.Sprintf("AND ($%d='all' OR COALESCE(NULLIF(%s,''),'unknown')=$%d)", n, usageLogEffectivePlatformExpr, n))
	}
	return query, args
}

func (r *organizationUsageRepository) queryRows(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	query, args = r.scopedQuery(query, args)
	return r.executor().QueryContext(ctx, query, args...)
}
func (r *organizationUsageRepository) queryRow(ctx context.Context, query string, args ...any) *sql.Row {
	query, args = r.scopedQuery(query, args)
	return r.executor().QueryRowContext(ctx, query, args...)
}

func (r *organizationUsageRepository) beginScoped(ctx context.Context, organization, q string, filters service.OrganizationUsageDepartmentFilters) (*organizationUsageRepository, *sql.Tx, *service.OrganizationUsageAppliedFilters, error) {
	filters, err := filters.Normalize(organization)
	if err != nil {
		return nil, nil, nil, err
	}
	if organization == "" {
		organization = service.OrganizationAll
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, nil, nil, err
	}
	scope, err := resolveDepartmentQueryScope(ctx, tx, service.AdminPermissionOrganizationUsage, service.DepartmentScopeQuery{Organization: organization, DepartmentID: filters.DepartmentID, Platform: filters.Platform, Q: q})
	if err == nil {
		err = scope.ValidateSelection(organization, filters.DepartmentID, filters.ScopeVersion)
	}
	if err != nil {
		_ = tx.Rollback()
		return nil, nil, nil, err
	}
	copyRepo := *r
	copyRepo.query = tx
	copyRepo.scope = scope
	copyRepo.filters = filters
	copyRepo.scopeIDs = []int64{}
	for _, m := range scope.SelectedMembers(organization, filters.DepartmentID, q) {
		copyRepo.scopeIDs = append(copyRepo.scopeIDs, m.ID)
	}
	applied := &service.OrganizationUsageAppliedFilters{Organization: organization, DepartmentID: filters.DepartmentID, Platform: filters.Platform, Q: q, Attribution: "current_membership"}
	return &copyRepo, tx, applied, nil
}

func (r *organizationUsageRepository) scopedSummary(ctx context.Context, p service.OrganizationUsageSummaryRepositoryParams) (*service.OrganizationUsageSummaryRepositoryResult, error) {
	q, tx, applied, err := r.beginScoped(ctx, p.Organization, p.Q, p.Filters)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := q.Summary(ctx, p)
	if err != nil {
		return nil, err
	}
	result.ScopeVersion = q.scope.Version
	result.AppliedFilters = applied
	organizations := make([]service.OrganizationUsageOrganization, 0)
	for _, o := range result.Organizations {
		if applied.Organization != "all" && o.Organization != applied.Organization {
			continue
		}
		for _, allowed := range q.scope.Organizations {
			if o.Organization == allowed {
				organizations = append(organizations, o)
				break
			}
		}
	}
	result.Organizations = organizations
	byID := q.memberMap()
	for i := range result.Items {
		item := &result.Items[i]
		m := byID[item.UserID]
		item.DepartmentID = m.DepartmentID
		item.DepartmentName = m.DepartmentName
		q.enrichPeriod(item.PeakDay, byID)
		q.enrichPeriod(item.PeakWeek, byID)
		q.enrichPeriod(item.PeakMonth, byID)
	}
	q.enrichPeriod(result.Champions.Day, byID)
	q.enrichPeriod(result.Champions.Week, byID)
	q.enrichPeriod(result.Champions.Month, byID)
	if err = q.breakdowns(ctx, p, applied, result); err != nil {
		return nil, err
	}
	return result, tx.Commit()
}

func (r *organizationUsageRepository) scopedPeriods(ctx context.Context, p service.OrganizationUsagePeriodsRepositoryParams) (*service.OrganizationUsagePeriodsRepositoryResult, error) {
	q, tx, applied, err := r.beginScoped(ctx, p.Organization, p.Q, p.Filters)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := q.Periods(ctx, p)
	if err != nil {
		return nil, err
	}
	result.ScopeVersion = q.scope.Version
	result.AppliedFilters = applied
	byID := q.memberMap()
	for i := range result.Items {
		q.enrichPeriod(&result.Items[i], byID)
	}
	return result, tx.Commit()
}

func (r *organizationUsageRepository) scopedTrend(ctx context.Context, p service.OrganizationUsageTrendRepositoryParams) (*service.OrganizationUsageTrendRepositoryResult, error) {
	q, tx, applied, err := r.beginScoped(ctx, p.Organization, p.Q, p.Filters)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if p.DataThrough.Before(p.StartDate) {
		return &service.OrganizationUsageTrendRepositoryResult{Points: []service.OrganizationUsageTrendPoint{}, ScopeVersion: q.scope.Version, AppliedFilters: applied}, tx.Commit()
	}
	result, err := q.Trend(ctx, p)
	if err != nil {
		return nil, err
	}
	result.ScopeVersion = q.scope.Version
	result.AppliedFilters = applied
	return result, tx.Commit()
}

func (r *organizationUsageRepository) memberMap() map[int64]service.DepartmentMember {
	result := map[int64]service.DepartmentMember{}
	for _, m := range r.scope.Members {
		result[m.ID] = m
	}
	return result
}
func (r *organizationUsageRepository) enrichPeriod(p *service.OrganizationUsagePeriod, members map[int64]service.DepartmentMember) {
	if p != nil {
		m := members[p.UserID]
		p.DepartmentID = m.DepartmentID
		p.DepartmentName = m.DepartmentName
	}
}

func departmentBucketKey(organization string, id *int64) string {
	if id == nil {
		return organization + ":unassigned"
	}
	return organization + ":" + strconv.FormatInt(*id, 10)
}

func (r *organizationUsageRepository) breakdowns(ctx context.Context, p service.OrganizationUsageSummaryRepositoryParams, applied *service.OrganizationUsageAppliedFilters, result *service.OrganizationUsageSummaryRepositoryResult) error {
	result.Departments = []service.OrganizationUsageDepartment{}
	result.Platforms = []service.OrganizationUsagePlatform{}
	departments := map[string]*service.OrganizationUsageDepartment{}
	order := []string{}
	if applied.Organization != "all" {
		for _, d := range r.scope.Departments {
			if d.Organization != applied.Organization || applied.DepartmentID == "unassigned" {
				continue
			}
			if applied.DepartmentID != "all" && strconv.FormatInt(d.ID, 10) != applied.DepartmentID {
				continue
			}
			id := d.ID
			key := departmentBucketKey(d.Organization, &id)
			departments[key] = &service.OrganizationUsageDepartment{DepartmentID: &id, DepartmentName: d.Name, Organization: d.Organization}
			order = append(order, key)
		}
		if r.scope.Unrestricted && (applied.DepartmentID == "all" || applied.DepartmentID == "unassigned") {
			key := departmentBucketKey(applied.Organization, nil)
			departments[key] = &service.OrganizationUsageDepartment{DepartmentName: "", Organization: applied.Organization}
			order = append(order, key)
		}
	}
	byID := r.memberMap()
	for _, id := range r.scopeIDs {
		m := byID[id]
		if d := departments[departmentBucketKey(m.Organization, m.DepartmentID)]; d != nil {
			d.ActiveUsers++
		}
	}
	platforms := map[string]*service.OrganizationUsagePlatform{}
	usedMembers := map[int64]bool{}
	query := `SELECT ul.user_id,COALESCE(NULLIF(` + usageLogEffectivePlatformExpr + `,''),'unknown'),COUNT(*)::bigint,
COALESCE(SUM(ul.input_tokens),0)::bigint,COALESCE(SUM(ul.output_tokens),0)::bigint,
COALESCE(SUM(ul.cache_creation_tokens),0)::bigint,COALESCE(SUM(ul.cache_read_tokens),0)::bigint,
COALESCE(SUM(ul.input_tokens+ul.output_tokens+ul.cache_creation_tokens+ul.cache_read_tokens),0)::bigint,COALESCE(SUM(ul.actual_cost),0)::double precision
FROM usage_logs ul LEFT JOIN groups g ON g.id=ul.group_id LEFT JOIN accounts a ON a.id=ul.account_id
WHERE ul.user_id=ANY($1::bigint[]) AND ul.created_at >= $2 AND ul.created_at < $3
AND ($4='all' OR COALESCE(NULLIF(` + usageLogEffectivePlatformExpr + `,''),'unknown')=$4)
GROUP BY ul.user_id,2 ORDER BY ul.user_id,2`
	rows, err := r.executor().QueryContext(ctx, query, pq.Array(r.scopeIDs), p.StartTime, p.EndTime, r.filters.Platform)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var userID int64
		var platform string
		var metrics service.OrganizationUsageMetrics
		if err := rows.Scan(&userID, &platform, &metrics.Requests, &metrics.InputTokens, &metrics.OutputTokens, &metrics.CacheCreationTokens, &metrics.CacheReadTokens, &metrics.TotalTokens, &metrics.ActualCost); err != nil {
			return err
		}
		if platforms[platform] == nil {
			platforms[platform] = &service.OrganizationUsagePlatform{Platform: platform}
		}
		platforms[platform].Add(metrics)
		platforms[platform].UsedUsers++
		m := byID[userID]
		if d := departments[departmentBucketKey(m.Organization, m.DepartmentID)]; d != nil {
			d.Add(metrics)
			if !usedMembers[userID] {
				d.UsedUsers++
			}
		}
		usedMembers[userID] = true
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for _, key := range order {
		result.Departments = append(result.Departments, *departments[key])
	}
	platformNames := make([]string, 0, len(platforms))
	for platform := range platforms {
		platformNames = append(platformNames, platform)
	}
	sort.Strings(platformNames)
	for _, platform := range platformNames {
		result.Platforms = append(result.Platforms, *platforms[platform])
	}
	return nil
}
