package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type organizationUsageRepository struct {
	db *sql.DB
}

func NewOrganizationUsageRepository(db *sql.DB) service.OrganizationUsageRepository {
	return &organizationUsageRepository{db: db}
}

func (r *organizationUsageRepository) Summary(ctx context.Context, params service.OrganizationUsageSummaryRepositoryParams) (*service.OrganizationUsageSummaryRepositoryResult, error) {
	orderBy, err := organizationUsageOrderBy(params.SortBy, params.SortOrder)
	if err != nil {
		return nil, err
	}

	organizations, err := r.queryOrganizations(ctx, params)
	if err != nil {
		return nil, err
	}
	items, total, err := r.querySummaryItems(ctx, params, orderBy)
	if err != nil {
		return nil, err
	}
	champions, err := r.queryChampions(ctx, params)
	if err != nil {
		return nil, err
	}

	return &service.OrganizationUsageSummaryRepositoryResult{
		Overview:      organizationUsageOverview(organizations, params.Organization),
		Organizations: organizations,
		Champions:     champions,
		Items:         items,
		Total:         total,
	}, nil
}

func (r *organizationUsageRepository) Periods(ctx context.Context, params service.OrganizationUsagePeriodsRepositoryParams) (*service.OrganizationUsagePeriodsRepositoryResult, error) {
	countQuery, err := organizationUsagePeriodsCountQuery(params.Granularity)
	if err != nil {
		return nil, err
	}
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery,
		params.StartTime, params.EndTime, organizationUsageSearchPattern(params.Q), params.Organization,
	).Scan(&total); err != nil {
		return nil, fmt.Errorf("count organization usage periods: %w", err)
	}

	query, err := organizationUsagePeriodsQuery(params.Granularity)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, query,
		params.StartTime, params.EndTime, organizationUsageSearchPattern(params.Q), params.Organization,
		params.StartDate.Format("2006-01-02"), params.EndDate.Format("2006-01-02"), params.PageSize, (params.Page-1)*params.PageSize,
	)
	if err != nil {
		return nil, fmt.Errorf("query organization usage periods: %w", err)
	}
	defer rows.Close()

	items := make([]service.OrganizationUsagePeriod, 0)
	for rows.Next() {
		var item service.OrganizationUsagePeriod
		var periodStart, periodEnd sql.NullTime
		var rowTotal int64
		if err := rows.Scan(
			&rowTotal, &periodStart, &periodEnd, &item.Partial, &item.UserID, &item.Email, &item.Organization,
			&item.Requests, &item.InputTokens, &item.OutputTokens, &item.CacheCreationTokens, &item.CacheReadTokens, &item.TotalTokens, &item.ActualCost,
		); err != nil {
			return nil, fmt.Errorf("scan organization usage period: %w", err)
		}
		item.PeriodStart = formatSQLDate(periodStart)
		item.PeriodEnd = formatSQLDate(periodEnd)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate organization usage periods: %w", err)
	}
	return &service.OrganizationUsagePeriodsRepositoryResult{Items: items, Total: total}, nil
}

func (r *organizationUsageRepository) Trend(ctx context.Context, params service.OrganizationUsageTrendRepositoryParams) (*service.OrganizationUsageTrendRepositoryResult, error) {
	query, err := organizationUsageTrendQuery(params.Granularity)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, query,
		params.StartTime, params.EndTime, organizationUsageSearchPattern(params.Q), params.Organization,
		params.StartDate.Format("2006-01-02"), params.EndDate.Format("2006-01-02"), params.DataThrough.Format("2006-01-02"),
	)
	if err != nil {
		return nil, fmt.Errorf("query organization usage trend: %w", err)
	}
	defer rows.Close()

	points := make([]service.OrganizationUsageTrendPoint, 0)
	for rows.Next() {
		var point service.OrganizationUsageTrendPoint
		var periodStart, periodEnd sql.NullTime
		if err := rows.Scan(
			&periodStart, &periodEnd, &point.Partial,
			&point.Requests, &point.InputTokens, &point.OutputTokens, &point.CacheCreationTokens, &point.CacheReadTokens, &point.TotalTokens, &point.ActualCost,
		); err != nil {
			return nil, fmt.Errorf("scan organization usage trend: %w", err)
		}
		point.PeriodStart = formatSQLDate(periodStart)
		point.PeriodEnd = formatSQLDate(periodEnd)
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate organization usage trend: %w", err)
	}
	return &service.OrganizationUsageTrendRepositoryResult{Points: points}, nil
}

func (r *organizationUsageRepository) queryOrganizations(ctx context.Context, params service.OrganizationUsageSummaryRepositoryParams) ([]service.OrganizationUsageOrganization, error) {
	rows, err := r.db.QueryContext(ctx, organizationUsageOrganizationsQuery(), params.StartTime, params.EndTime, organizationUsageSearchPattern(params.Q))
	if err != nil {
		return nil, fmt.Errorf("query organization usage organizations: %w", err)
	}
	defer rows.Close()

	result := make([]service.OrganizationUsageOrganization, 0, 3)
	for rows.Next() {
		var item service.OrganizationUsageOrganization
		if err := rows.Scan(
			&item.Organization, &item.ActiveUsers, &item.UsedUsers,
			&item.Requests, &item.InputTokens, &item.OutputTokens, &item.CacheCreationTokens, &item.CacheReadTokens, &item.TotalTokens, &item.ActualCost,
		); err != nil {
			return nil, fmt.Errorf("scan organization usage organization: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate organization usage organizations: %w", err)
	}
	return result, nil
}

func (r *organizationUsageRepository) querySummaryItems(ctx context.Context, params service.OrganizationUsageSummaryRepositoryParams, orderBy string) ([]service.OrganizationUsageSummaryItem, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, organizationUsageSummaryItemsCountQuery(), organizationUsageSearchPattern(params.Q), params.Organization).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count organization usage summary items: %w", err)
	}
	rows, err := r.db.QueryContext(ctx, organizationUsageSummaryItemsQuery(orderBy),
		params.StartTime, params.EndTime, organizationUsageSearchPattern(params.Q), params.Organization,
		params.StartDate.Format("2006-01-02"), params.EndDate.Format("2006-01-02"), params.PageSize, (params.Page-1)*params.PageSize,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query organization usage summary items: %w", err)
	}
	defer rows.Close()

	items := make([]service.OrganizationUsageSummaryItem, 0)
	for rows.Next() {
		var item service.OrganizationUsageSummaryItem
		var day, week, month nullableOrganizationUsagePeriod
		var rowTotal int64
		if err := rows.Scan(
			&rowTotal, &item.UserID, &item.Email, &item.Organization,
			&item.Requests, &item.InputTokens, &item.OutputTokens, &item.CacheCreationTokens, &item.CacheReadTokens, &item.TotalTokens, &item.ActualCost,
			&day.start, &day.end, &day.partial, &day.requests, &day.inputTokens, &day.outputTokens, &day.cacheCreationTokens, &day.cacheReadTokens, &day.totalTokens, &day.actualCost,
			&week.start, &week.end, &week.partial, &week.requests, &week.inputTokens, &week.outputTokens, &week.cacheCreationTokens, &week.cacheReadTokens, &week.totalTokens, &week.actualCost,
			&month.start, &month.end, &month.partial, &month.requests, &month.inputTokens, &month.outputTokens, &month.cacheCreationTokens, &month.cacheReadTokens, &month.totalTokens, &month.actualCost,
		); err != nil {
			return nil, 0, fmt.Errorf("scan organization usage summary item: %w", err)
		}
		item.PeakDay = day.period(item.UserID, item.Email, item.Organization)
		item.PeakWeek = week.period(item.UserID, item.Email, item.Organization)
		item.PeakMonth = month.period(item.UserID, item.Email, item.Organization)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate organization usage summary items: %w", err)
	}
	return items, total, nil
}

func (r *organizationUsageRepository) queryChampions(ctx context.Context, params service.OrganizationUsageSummaryRepositoryParams) (service.OrganizationUsageChampions, error) {
	rows, err := r.db.QueryContext(ctx, organizationUsageChampionsQuery(),
		params.StartTime, params.EndTime, organizationUsageSearchPattern(params.Q), params.Organization,
		params.StartDate.Format("2006-01-02"), params.EndDate.Format("2006-01-02"),
	)
	if err != nil {
		return service.OrganizationUsageChampions{}, fmt.Errorf("query organization usage champions: %w", err)
	}
	defer rows.Close()

	var result service.OrganizationUsageChampions
	for rows.Next() {
		var granularity string
		var period service.OrganizationUsagePeriod
		var start, end sql.NullTime
		if err := rows.Scan(
			&granularity, &start, &end, &period.Partial, &period.UserID, &period.Email, &period.Organization,
			&period.Requests, &period.InputTokens, &period.OutputTokens, &period.CacheCreationTokens, &period.CacheReadTokens, &period.TotalTokens, &period.ActualCost,
		); err != nil {
			return service.OrganizationUsageChampions{}, fmt.Errorf("scan organization usage champion: %w", err)
		}
		period.PeriodStart = formatSQLDate(start)
		period.PeriodEnd = formatSQLDate(end)
		switch granularity {
		case service.OrganizationUsageGranularityDay:
			result.Day = &period
		case service.OrganizationUsageGranularityWeek:
			result.Week = &period
		case service.OrganizationUsageGranularityMonth:
			result.Month = &period
		}
	}
	if err := rows.Err(); err != nil {
		return service.OrganizationUsageChampions{}, fmt.Errorf("iterate organization usage champions: %w", err)
	}
	return result, nil
}

type nullableOrganizationUsagePeriod struct {
	start, end                                        sql.NullTime
	partial                                           sql.NullBool
	requests, inputTokens, outputTokens               sql.NullInt64
	cacheCreationTokens, cacheReadTokens, totalTokens sql.NullInt64
	actualCost                                        sql.NullFloat64
}

func (p nullableOrganizationUsagePeriod) period(userID int64, email, organization string) *service.OrganizationUsagePeriod {
	if !p.start.Valid {
		return nil
	}
	return &service.OrganizationUsagePeriod{
		PeriodStart: formatSQLDate(p.start), PeriodEnd: formatSQLDate(p.end), Partial: p.partial.Bool,
		UserID: userID, Email: email, Organization: organization,
		OrganizationUsageMetrics: service.OrganizationUsageMetrics{
			Requests: p.requests.Int64, InputTokens: p.inputTokens.Int64, OutputTokens: p.outputTokens.Int64,
			CacheCreationTokens: p.cacheCreationTokens.Int64, CacheReadTokens: p.cacheReadTokens.Int64,
			TotalTokens: p.totalTokens.Int64, ActualCost: p.actualCost.Float64,
		},
	}
}

func formatSQLDate(value sql.NullTime) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format("2006-01-02")
}

func organizationUsageOverview(organizations []service.OrganizationUsageOrganization, selected string) service.OrganizationUsageOverview {
	var result service.OrganizationUsageOverview
	for _, item := range organizations {
		if selected != service.OrganizationAll && item.Organization != selected {
			continue
		}
		result.ActiveUsers += item.ActiveUsers
		result.UsedUsers += item.UsedUsers
		result.Requests += item.Requests
		result.InputTokens += item.InputTokens
		result.OutputTokens += item.OutputTokens
		result.CacheCreationTokens += item.CacheCreationTokens
		result.CacheReadTokens += item.CacheReadTokens
		result.TotalTokens += item.TotalTokens
		result.ActualCost += item.ActualCost
	}
	return result
}

func organizationUsageOrganizationExpression(alias string) string {
	return fmt.Sprintf(`CASE
            WHEN LOWER(SPLIT_PART(%[1]s.email, '@', 2)) = 'xunyou.com' THEN 'xunyou'
            WHEN LOWER(SPLIT_PART(%[1]s.email, '@', 2)) = 'wsdashi.com' THEN 'wsdashi'
            ELSE 'other'
        END`, alias)
}

func organizationUsageActiveUsersCTE() string {
	return fmt.Sprintf(`active_users AS (
    SELECT u.id AS user_id, u.email, %s AS organization
    FROM users u
    WHERE u.deleted_at IS NULL
      AND u.status = 'active'
      AND ($3 = '' OR u.email ILIKE $3 ESCAPE E'\\')
)`, organizationUsageOrganizationExpression("u"))
}

const organizationUsageRowsCTE = `usage_rows AS (
    SELECT
        ul.user_id,
        ul.created_at AT TIME ZONE 'Asia/Shanghai' AS local_created_at,
        ul.input_tokens::bigint AS input_tokens,
        ul.output_tokens::bigint AS output_tokens,
        ul.cache_creation_tokens::bigint AS cache_creation_tokens,
        ul.cache_read_tokens::bigint AS cache_read_tokens,
        (ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens)::bigint AS total_tokens,
        ul.actual_cost::double precision AS actual_cost
    FROM usage_logs ul
    WHERE ul.created_at >= $1 AND ul.created_at < $2
)`

const organizationUsageTotalsCTE = `usage_totals AS (
    SELECT
        user_id,
        COUNT(*)::bigint AS requests,
        COALESCE(SUM(input_tokens), 0)::bigint AS input_tokens,
        COALESCE(SUM(output_tokens), 0)::bigint AS output_tokens,
        COALESCE(SUM(cache_creation_tokens), 0)::bigint AS cache_creation_tokens,
        COALESCE(SUM(cache_read_tokens), 0)::bigint AS cache_read_tokens,
        COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens,
        COALESCE(SUM(actual_cost), 0)::double precision AS actual_cost
    FROM usage_rows
    GROUP BY user_id
)`

func organizationUsageOrganizationsQuery() string {
	return fmt.Sprintf(`WITH %s,
%s,
%s,
organizations(organization, sort_order) AS (
    VALUES ('xunyou', 1), ('wsdashi', 2), ('other', 3)
)
SELECT
    o.organization,
    COUNT(au.user_id)::bigint AS active_users,
    COUNT(ut.user_id)::bigint AS used_users,
    COALESCE(SUM(ut.requests), 0)::bigint AS requests,
    COALESCE(SUM(ut.input_tokens), 0)::bigint AS input_tokens,
    COALESCE(SUM(ut.output_tokens), 0)::bigint AS output_tokens,
    COALESCE(SUM(ut.cache_creation_tokens), 0)::bigint AS cache_creation_tokens,
    COALESCE(SUM(ut.cache_read_tokens), 0)::bigint AS cache_read_tokens,
    COALESCE(SUM(ut.total_tokens), 0)::bigint AS total_tokens,
    COALESCE(SUM(ut.actual_cost), 0)::double precision AS actual_cost
FROM organizations o
LEFT JOIN active_users au ON au.organization = o.organization
LEFT JOIN usage_totals ut ON ut.user_id = au.user_id
GROUP BY o.organization, o.sort_order
ORDER BY o.sort_order`, organizationUsageActiveUsersCTE(), organizationUsageRowsCTE, organizationUsageTotalsCTE)
}

func organizationUsageSummaryItemsCountQuery() string {
	return fmt.Sprintf(`WITH active_users AS (
    SELECT u.id AS user_id, %s AS organization
    FROM users u
    WHERE u.deleted_at IS NULL
      AND u.status = 'active'
      AND ($1 = '' OR u.email ILIKE $1 ESCAPE E'\\')
)
SELECT COUNT(*)::bigint
FROM active_users
WHERE $2 = 'all' OR organization = $2`, organizationUsageOrganizationExpression("u"))
}

func organizationUsagePeriodBucketSQL(granularity string) (string, string, error) {
	switch granularity {
	case service.OrganizationUsageGranularityDay:
		return "date_trunc('day', local_created_at)::date", "date_trunc('day', local_created_at)::date", nil
	case service.OrganizationUsageGranularityWeek:
		return "date_trunc('week', local_created_at)::date", "(date_trunc('week', local_created_at) + interval '6 days')::date", nil
	case service.OrganizationUsageGranularityMonth:
		return "date_trunc('month', local_created_at)::date", "(date_trunc('month', local_created_at) + interval '1 month - 1 day')::date", nil
	default:
		return "", "", fmt.Errorf("invalid organization usage granularity %q", granularity)
	}
}

func organizationUsagePeriodAggregationSQL(granularity string) (string, error) {
	start, end, err := organizationUsagePeriodBucketSQL(granularity)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`SELECT
        user_id,
        '%s'::text AS granularity,
        %s AS bucket_start,
        %s AS bucket_end,
        COUNT(*)::bigint AS requests,
        COALESCE(SUM(input_tokens), 0)::bigint AS input_tokens,
        COALESCE(SUM(output_tokens), 0)::bigint AS output_tokens,
        COALESCE(SUM(cache_creation_tokens), 0)::bigint AS cache_creation_tokens,
        COALESCE(SUM(cache_read_tokens), 0)::bigint AS cache_read_tokens,
        COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens,
        COALESCE(SUM(actual_cost), 0)::double precision AS actual_cost
    FROM usage_rows
    GROUP BY user_id, bucket_start, bucket_end`, granularity, start, end), nil
}

func organizationUsageAllPeriodAggregationsSQL() string {
	day, _ := organizationUsagePeriodAggregationSQL(service.OrganizationUsageGranularityDay)
	week, _ := organizationUsagePeriodAggregationSQL(service.OrganizationUsageGranularityWeek)
	month, _ := organizationUsagePeriodAggregationSQL(service.OrganizationUsageGranularityMonth)
	return day + "\n    UNION ALL\n    " + week + "\n    UNION ALL\n    " + month
}

func organizationUsageSummaryItemsQuery(orderBy string) string {
	return fmt.Sprintf(`WITH %s,
selected_users AS (
    SELECT * FROM active_users
    WHERE $4 = 'all' OR organization = $4
),
%s,
%s,
period_aggregates AS (
    %s
),
ranked_periods AS (
    SELECT period_aggregates.*,
        ROW_NUMBER() OVER (
            PARTITION BY user_id, granularity
            ORDER BY total_tokens DESC, actual_cost DESC, requests DESC, user_id ASC, bucket_start ASC
        ) AS rn
    FROM period_aggregates
),
day_peak AS (SELECT * FROM ranked_periods WHERE granularity = 'day' AND rn = 1),
week_peak AS (SELECT * FROM ranked_periods WHERE granularity = 'week' AND rn = 1),
month_peak AS (SELECT * FROM ranked_periods WHERE granularity = 'month' AND rn = 1)
SELECT
    COUNT(*) OVER()::bigint AS total_count,
    su.user_id, su.email, su.organization,
    COALESCE(ut.requests, 0)::bigint AS requests,
    COALESCE(ut.input_tokens, 0)::bigint AS input_tokens,
    COALESCE(ut.output_tokens, 0)::bigint AS output_tokens,
    COALESCE(ut.cache_creation_tokens, 0)::bigint AS cache_creation_tokens,
    COALESCE(ut.cache_read_tokens, 0)::bigint AS cache_read_tokens,
    COALESCE(ut.total_tokens, 0)::bigint AS total_tokens,
    COALESCE(ut.actual_cost, 0)::double precision AS actual_cost,
    CASE WHEN dp.user_id IS NULL THEN NULL ELSE GREATEST(dp.bucket_start, $5::date) END AS peak_day_start,
    CASE WHEN dp.user_id IS NULL THEN NULL ELSE LEAST(dp.bucket_end, $6::date) END AS peak_day_end,
    (dp.bucket_start < $5::date OR dp.bucket_end > $6::date) AS peak_day_partial,
    dp.requests AS peak_day_requests, dp.input_tokens AS peak_day_input_tokens, dp.output_tokens AS peak_day_output_tokens,
    dp.cache_creation_tokens AS peak_day_cache_creation_tokens, dp.cache_read_tokens AS peak_day_cache_read_tokens,
    dp.total_tokens AS peak_day_total_tokens, dp.actual_cost AS peak_day_actual_cost,
    CASE WHEN wp.user_id IS NULL THEN NULL ELSE GREATEST(wp.bucket_start, $5::date) END AS peak_week_start,
    CASE WHEN wp.user_id IS NULL THEN NULL ELSE LEAST(wp.bucket_end, $6::date) END AS peak_week_end,
    (wp.bucket_start < $5::date OR wp.bucket_end > $6::date) AS peak_week_partial,
    wp.requests AS peak_week_requests, wp.input_tokens AS peak_week_input_tokens, wp.output_tokens AS peak_week_output_tokens,
    wp.cache_creation_tokens AS peak_week_cache_creation_tokens, wp.cache_read_tokens AS peak_week_cache_read_tokens,
    wp.total_tokens AS peak_week_total_tokens, wp.actual_cost AS peak_week_actual_cost,
    CASE WHEN mp.user_id IS NULL THEN NULL ELSE GREATEST(mp.bucket_start, $5::date) END AS peak_month_start,
    CASE WHEN mp.user_id IS NULL THEN NULL ELSE LEAST(mp.bucket_end, $6::date) END AS peak_month_end,
    (mp.bucket_start < $5::date OR mp.bucket_end > $6::date) AS peak_month_partial,
    mp.requests AS peak_month_requests, mp.input_tokens AS peak_month_input_tokens, mp.output_tokens AS peak_month_output_tokens,
    mp.cache_creation_tokens AS peak_month_cache_creation_tokens, mp.cache_read_tokens AS peak_month_cache_read_tokens,
    mp.total_tokens AS peak_month_total_tokens, mp.actual_cost AS peak_month_actual_cost
FROM selected_users su
    LEFT JOIN usage_totals ut ON ut.user_id = su.user_id
    LEFT JOIN day_peak dp ON dp.user_id = su.user_id
    LEFT JOIN week_peak wp ON wp.user_id = su.user_id
    LEFT JOIN month_peak mp ON mp.user_id = su.user_id
ORDER BY %s
LIMIT $7 OFFSET $8`, organizationUsageActiveUsersCTE(), organizationUsageRowsCTE, organizationUsageTotalsCTE, organizationUsageAllPeriodAggregationsSQL(), orderBy)
}

func organizationUsageChampionsQuery() string {
	return fmt.Sprintf(`WITH %s,
selected_users AS (
    SELECT * FROM active_users
    WHERE $4 = 'all' OR organization = $4
),
%s,
period_aggregates AS (
    %s
),
ranked AS (
    SELECT pa.*,
        ROW_NUMBER() OVER (
            PARTITION BY granularity
            ORDER BY pa.total_tokens DESC, pa.actual_cost DESC, pa.requests DESC, pa.user_id ASC, pa.bucket_start ASC
        ) AS rn
    FROM period_aggregates pa
    JOIN selected_users su ON su.user_id = pa.user_id
)
SELECT
    r.granularity,
    GREATEST(r.bucket_start, $5::date) AS period_start,
    LEAST(r.bucket_end, $6::date) AS period_end,
    (r.bucket_start < $5::date OR r.bucket_end > $6::date) AS partial,
    su.user_id, su.email, su.organization,
    r.requests, r.input_tokens, r.output_tokens, r.cache_creation_tokens, r.cache_read_tokens, r.total_tokens, r.actual_cost
FROM ranked r
JOIN selected_users su ON su.user_id = r.user_id
WHERE r.rn = 1
ORDER BY CASE r.granularity WHEN 'day' THEN 1 WHEN 'week' THEN 2 ELSE 3 END`, organizationUsageActiveUsersCTE(), organizationUsageRowsCTE, organizationUsageAllPeriodAggregationsSQL())
}

func organizationUsagePeriodsQuery(granularity string) (string, error) {
	aggregation, err := organizationUsagePeriodAggregationSQL(granularity)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`WITH %s,
selected_users AS (
    SELECT * FROM active_users
    WHERE $4 = 'all' OR organization = $4
),
%s,
period_aggregates AS (
    %s
)
SELECT
    COUNT(*) OVER()::bigint AS total_count,
    GREATEST(bucket_start, $5::date) AS period_start,
    LEAST(bucket_end, $6::date) AS period_end,
    (bucket_start < $5::date OR bucket_end > $6::date) AS partial,
    su.user_id, su.email, su.organization,
    pa.requests, pa.input_tokens, pa.output_tokens, pa.cache_creation_tokens, pa.cache_read_tokens, pa.total_tokens, pa.actual_cost
FROM period_aggregates pa
JOIN selected_users su ON su.user_id = pa.user_id
ORDER BY period_start DESC, user_id ASC
LIMIT $7 OFFSET $8`, organizationUsageActiveUsersCTE(), organizationUsageRowsCTE, aggregation), nil
}

func organizationUsagePeriodsCountQuery(granularity string) (string, error) {
	aggregation, err := organizationUsagePeriodAggregationSQL(granularity)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`WITH %s,
selected_users AS (
    SELECT * FROM active_users
    WHERE $4 = 'all' OR organization = $4
),
%s,
period_aggregates AS (
    %s
)
SELECT COUNT(*)::bigint
FROM period_aggregates pa
JOIN selected_users su ON su.user_id = pa.user_id`, organizationUsageActiveUsersCTE(), organizationUsageRowsCTE, aggregation), nil
}

// organizationUsageTrendBucketSeriesSQL builds generate_series buckets from start_date ($5) through data_through ($7).
func organizationUsageTrendBucketSeriesSQL(granularity string) (string, error) {
	switch granularity {
	case service.OrganizationUsageGranularityDay:
		return `SELECT gs::date AS bucket_start, gs::date AS bucket_end
    FROM generate_series($5::date, $7::date, interval '1 day') AS gs`, nil
	case service.OrganizationUsageGranularityWeek:
		return `SELECT
        gs::date AS bucket_start,
        (gs + interval '6 days')::date AS bucket_end
    FROM generate_series(
        date_trunc('week', $5::timestamp)::date,
        date_trunc('week', $7::timestamp)::date,
        interval '7 days'
    ) AS gs`, nil
	case service.OrganizationUsageGranularityMonth:
		return `SELECT
        gs::date AS bucket_start,
        (gs + interval '1 month - 1 day')::date AS bucket_end
    FROM generate_series(
        date_trunc('month', $5::timestamp)::date,
        date_trunc('month', $7::timestamp)::date,
        interval '1 month'
    ) AS gs`, nil
	default:
		return "", fmt.Errorf("invalid organization usage granularity %q", granularity)
	}
}

// organizationUsageTrendAggregationSQL aggregates usage by period bucket without user_id dimension.
func organizationUsageTrendAggregationSQL(granularity string) (string, error) {
	start, end, err := organizationUsagePeriodBucketSQL(granularity)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`SELECT
        %s AS bucket_start,
        %s AS bucket_end,
        COUNT(*)::bigint AS requests,
        COALESCE(SUM(input_tokens), 0)::bigint AS input_tokens,
        COALESCE(SUM(output_tokens), 0)::bigint AS output_tokens,
        COALESCE(SUM(cache_creation_tokens), 0)::bigint AS cache_creation_tokens,
        COALESCE(SUM(cache_read_tokens), 0)::bigint AS cache_read_tokens,
        COALESCE(SUM(total_tokens), 0)::bigint AS total_tokens,
        COALESCE(SUM(actual_cost), 0)::double precision AS actual_cost
    FROM filtered_usage
    GROUP BY bucket_start, bucket_end`, start, end), nil
}

func organizationUsageTrendQuery(granularity string) (string, error) {
	aggregation, err := organizationUsageTrendAggregationSQL(granularity)
	if err != nil {
		return "", err
	}
	buckets, err := organizationUsageTrendBucketSeriesSQL(granularity)
	if err != nil {
		return "", err
	}
	// $1 start_time, $2 end_time, $3 q, $4 org, $5 start_date, $6 end_date, $7 data_through
	return fmt.Sprintf(`WITH %s,
selected_users AS (
    SELECT * FROM active_users
    WHERE $4 = 'all' OR organization = $4
),
%s,
filtered_usage AS (
    SELECT ur.*
    FROM usage_rows ur
    JOIN selected_users su ON su.user_id = ur.user_id
),
period_aggregates AS (
    %s
),
buckets AS (
    %s
)
SELECT
    GREATEST(b.bucket_start, $5::date) AS period_start,
    LEAST(b.bucket_end, $6::date, $7::date) AS period_end,
    (b.bucket_start < $5::date OR b.bucket_end > $6::date OR b.bucket_end > $7::date) AS partial,
    COALESCE(pa.requests, 0)::bigint AS requests,
    COALESCE(pa.input_tokens, 0)::bigint AS input_tokens,
    COALESCE(pa.output_tokens, 0)::bigint AS output_tokens,
    COALESCE(pa.cache_creation_tokens, 0)::bigint AS cache_creation_tokens,
    COALESCE(pa.cache_read_tokens, 0)::bigint AS cache_read_tokens,
    COALESCE(pa.total_tokens, 0)::bigint AS total_tokens,
    COALESCE(pa.actual_cost, 0)::double precision AS actual_cost
FROM buckets b
LEFT JOIN period_aggregates pa ON pa.bucket_start = b.bucket_start
ORDER BY period_start ASC`, organizationUsageActiveUsersCTE(), organizationUsageRowsCTE, aggregation, buckets), nil
}

func organizationUsageOrderBy(sortBy, sortOrder string) (string, error) {
	columns := map[string]string{
		"email": "LOWER(email)", "requests": "requests", "input_tokens": "input_tokens", "output_tokens": "output_tokens",
		"cache_creation_tokens": "cache_creation_tokens", "cache_read_tokens": "cache_read_tokens", "total_tokens": "total_tokens",
		"actual_cost": "actual_cost", "peak_day_tokens": "COALESCE(dp.total_tokens, 0)",
		"peak_week_tokens": "COALESCE(wp.total_tokens, 0)", "peak_month_tokens": "COALESCE(mp.total_tokens, 0)",
	}
	column, ok := columns[sortBy]
	if !ok {
		return "", fmt.Errorf("invalid organization usage sort_by %q", sortBy)
	}
	order := strings.ToLower(sortOrder)
	if order != "asc" && order != "desc" {
		return "", fmt.Errorf("invalid organization usage sort_order %q", sortOrder)
	}
	return fmt.Sprintf("%s %s, user_id ASC", column, strings.ToUpper(order)), nil
}

func organizationUsageSearchPattern(q string) string {
	if q == "" {
		return ""
	}
	escaped := strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	).Replace(q)
	return "%" + escaped + "%"
}
