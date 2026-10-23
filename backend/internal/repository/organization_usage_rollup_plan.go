package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const organizationUsageDateLayout = "2006-01-02"

var organizationUsageRollupLocation = service.OrganizationUsageLocation()

// organizationUsageRollupPlan splits one report range into published rollup days and raw
// usage_logs ranges: [rollupStart, rollupEnd) minus dirtyDates reads usage_user_daily_rollups,
// every rawStarts[i]..rawEnds[i] range reads usage_logs. The two parts never overlap.
type organizationUsageRollupPlan struct {
	rollupStart string
	rollupEnd   string
	dirtyDates  []string
	rawStarts   []string
	rawEnds     []string
}

// Template slots of organizationUsageDaysCTE. Values are always bound as parameters.
var organizationUsageRollupPlanTokens = []string{
	"{{rollup_start}}", "{{rollup_end}}", "{{dirty_dates}}", "{{raw_starts}}", "{{raw_ends}}",
}

func (p *organizationUsageRollupPlan) bind(query string, args []any) (string, []any) {
	if p == nil || !strings.Contains(query, organizationUsageRollupPlanTokens[0]) {
		return query, args
	}
	values := []any{p.rollupStart, p.rollupEnd, pq.Array(p.dirtyDates), pq.Array(p.rawStarts), pq.Array(p.rawEnds)}
	for i, token := range organizationUsageRollupPlanTokens {
		args = append(args, values[i])
		query = strings.ReplaceAll(query, token, fmt.Sprintf("$%d", len(args)))
	}
	return query, args
}

func organizationUsageCalendarDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, organizationUsageRollupLocation)
}

// buildOrganizationUsageRollupPlan reads a day from the rollup only when the whole Beijing day
// lies before both the published watermark and endTime, and the day is not dirty. Everything
// else, including the partial as_of day, is read from usage_logs. closedBefore nil means raw only.
func buildOrganizationUsageRollupPlan(startDate, startTime, endTime time.Time, closedBefore *time.Time, dirty []time.Time) organizationUsageRollupPlan {
	startDay := organizationUsageCalendarDay(startDate)
	rollupEnd := startDay
	if closedBefore != nil && endTime.After(startTime) {
		end := organizationUsageCalendarDay(endTime.In(organizationUsageRollupLocation))
		if closed := organizationUsageCalendarDay(*closedBefore); closed.Before(end) {
			end = closed
		}
		if end.After(startDay) {
			rollupEnd = end
		}
	}

	plan := organizationUsageRollupPlan{
		rollupStart: startDay.Format(organizationUsageDateLayout),
		rollupEnd:   rollupEnd.Format(organizationUsageDateLayout),
		dirtyDates:  []string{},
		rawStarts:   []string{},
		rawEnds:     []string{},
	}
	type rawRange struct{ start, end time.Time }
	ranges := make([]rawRange, 0, len(dirty)+1)
	sort.Slice(dirty, func(i, j int) bool { return dirty[i].Before(dirty[j]) })
	for _, value := range dirty {
		day := organizationUsageCalendarDay(value)
		if day.Before(startDay) || !day.Before(rollupEnd) {
			continue
		}
		plan.dirtyDates = append(plan.dirtyDates, day.Format(organizationUsageDateLayout))
		ranges = append(ranges, rawRange{start: day, end: day.AddDate(0, 0, 1)})
	}
	tailStart := startTime
	if rollupEnd.After(tailStart) {
		tailStart = rollupEnd
	}
	if endTime.After(tailStart) {
		ranges = append(ranges, rawRange{start: tailStart, end: endTime})
	}

	for _, r := range ranges {
		if n := len(plan.rawEnds); n > 0 && plan.rawEnds[n-1] == formatOrganizationUsageInstant(r.start) {
			plan.rawEnds[n-1] = formatOrganizationUsageInstant(r.end)
			continue
		}
		plan.rawStarts = append(plan.rawStarts, formatOrganizationUsageInstant(r.start))
		plan.rawEnds = append(plan.rawEnds, formatOrganizationUsageInstant(r.end))
	}
	return plan
}

func formatOrganizationUsageInstant(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// loadOrganizationUsageRollupState reads the watermark and published dirty days through the
// report executor, so a repeatable-read report sees them in the same snapshot as the rollups.
func loadOrganizationUsageRollupState(ctx context.Context, q organizationUsageExecutor, startDate time.Time) (*time.Time, []time.Time, error) {
	var closedBefore sql.NullString
	var dirty pq.StringArray
	err := q.QueryRowContext(ctx, `SELECT s.closed_before::text,
    ARRAY(
        SELECT d.bucket_date::text
        FROM usage_user_rollup_dirty_days d
        WHERE d.bucket_date >= $1::date AND d.bucket_date < s.closed_before
        ORDER BY d.bucket_date
    )
FROM usage_user_rollup_state s
WHERE s.id = 1`, organizationUsageCalendarDay(startDate).Format(organizationUsageDateLayout)).Scan(&closedBefore, &dirty)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !closedBefore.Valid) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read organization usage rollup state: %w", err)
	}
	closed, err := time.ParseInLocation(organizationUsageDateLayout, closedBefore.String, organizationUsageRollupLocation)
	if err != nil {
		return nil, nil, fmt.Errorf("parse organization usage rollup watermark %q: %w", closedBefore.String, err)
	}
	days := make([]time.Time, 0, len(dirty))
	for _, value := range dirty {
		day, err := time.ParseInLocation(organizationUsageDateLayout, value, organizationUsageRollupLocation)
		if err != nil {
			return nil, nil, fmt.Errorf("parse organization usage dirty day %q: %w", value, err)
		}
		days = append(days, day)
	}
	return &closed, days, nil
}

// withRollupPlan returns a copy whose queries bind the read plan for this report range. A copy
// that already carries a plan is a per-request scoped repository and is returned unchanged.
func (r *organizationUsageRepository) withRollupPlan(ctx context.Context, startDate, startTime, endTime time.Time) (*organizationUsageRepository, error) {
	if r.plan != nil {
		return r, nil
	}
	var closedBefore *time.Time
	var dirty []time.Time
	if !r.rollupDisabled {
		var err error
		closedBefore, dirty, err = loadOrganizationUsageRollupState(ctx, r.executor(), startDate)
		if err != nil {
			return nil, err
		}
	}
	plan := buildOrganizationUsageRollupPlan(startDate, startTime, endTime, closedBefore, dirty)
	planned := *r
	planned.plan = &plan
	return &planned, nil
}
