package repository

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOrganizationUsageSQL_UsesExactDomainAndActiveUserContract(t *testing.T) {
	query := organizationUsageSummaryItemsQuery("total_tokens DESC, user_id ASC")
	normalized := strings.ToLower(query)

	require.Contains(t, normalized, "lower(split_part(u.email, '@', 2)) = 'xunyou.com'")
	require.Contains(t, normalized, "lower(split_part(u.email, '@', 2)) = 'wsdashi.com'")
	require.NotContains(t, normalized, "like '%xunyou.com'")
	require.Contains(t, normalized, "u.deleted_at is null")
	require.Contains(t, normalized, "u.status = 'active'")
	require.Contains(t, normalized, "from selected_users su\n    left join usage_totals")
	require.Contains(t, normalized, "ul.created_at >= $1 and ul.created_at < $2")
	require.Contains(t, normalized, "at time zone 'asia/shanghai'")
	require.Contains(t, normalized, "date_trunc('week'")
	require.Contains(t, normalized, "order by total_tokens desc, actual_cost desc, requests desc, user_id asc")
	require.Contains(t, strings.ToLower(organizationUsageChampionsQuery()), "order by pa.total_tokens desc, pa.actual_cost desc, pa.requests desc, pa.user_id asc")
}

func TestOrganizationUsageSearchPattern_EscapesPostgresLikeWildcards(t *testing.T) {
	require.Empty(t, organizationUsageSearchPattern(""))
	require.Equal(t, `%qa\_100\%\\ops%`, organizationUsageSearchPattern(`qa_100%\ops`))
	require.Contains(t, organizationUsageActiveUsersCTE(), `u.email ILIKE $3 ESCAPE E'\\'`)
	require.Contains(t, organizationUsageSummaryItemsCountQuery(), `u.email ILIKE $1 ESCAPE E'\\'`)
}

func TestOrganizationUsageSummarySQL_LeavesZeroUsagePeaksNull(t *testing.T) {
	query := organizationUsageSummaryItemsQuery("total_tokens DESC, user_id ASC")
	require.Contains(t, query, "CASE WHEN dp.user_id IS NULL THEN NULL ELSE GREATEST(dp.bucket_start, $5::date) END")
	require.Contains(t, query, "CASE WHEN wp.user_id IS NULL THEN NULL ELSE GREATEST(wp.bucket_start, $5::date) END")
	require.Contains(t, query, "CASE WHEN mp.user_id IS NULL THEN NULL ELSE GREATEST(mp.bucket_start, $5::date) END")
}

func TestOrganizationUsageOrderBy_IsFixedAllowlistWithStableUserTieBreak(t *testing.T) {
	tests := map[string]struct {
		order string
		want  string
	}{
		"email":             {order: "asc", want: "LOWER(email) ASC, user_id ASC"},
		"requests":          {order: "desc", want: "requests DESC, user_id ASC"},
		"actual_cost":       {order: "desc", want: "actual_cost DESC, user_id ASC"},
		"peak_day_tokens":   {order: "desc", want: "COALESCE(dp.total_tokens, 0) DESC, user_id ASC"},
		"peak_week_tokens":  {order: "desc", want: "COALESCE(wp.total_tokens, 0) DESC, user_id ASC"},
		"peak_month_tokens": {order: "desc", want: "COALESCE(mp.total_tokens, 0) DESC, user_id ASC"},
	}
	for sortBy, tt := range tests {
		got, err := organizationUsageOrderBy(sortBy, tt.order)
		require.NoError(t, err)
		require.Equal(t, tt.want, got)
	}

	_, err := organizationUsageOrderBy("requests; drop table users", "desc")
	require.Error(t, err)
	_, err = organizationUsageOrderBy("requests", "desc nulls last")
	require.Error(t, err)
}

func TestOrganizationUsagePeriodBucketSQL_UsesMondayAndClipsSelectedRange(t *testing.T) {
	bucketStart, bucketEnd, err := organizationUsagePeriodBucketSQL(service.OrganizationUsageGranularityWeek)
	require.NoError(t, err)
	require.Contains(t, bucketStart, "date_trunc('week'")
	require.Contains(t, bucketEnd, "6 days")

	query, err := organizationUsagePeriodsQuery(service.OrganizationUsageGranularityWeek)
	require.NoError(t, err)
	require.Contains(t, query, "GREATEST(bucket_start, $5::date)")
	require.Contains(t, query, "LEAST(bucket_end, $6::date)")
	require.Contains(t, query, "(bucket_start < $5::date OR bucket_end > $6::date) AS partial")
}

func TestOrganizationUsageTrendSQL_ZeroFillsWithoutUserDimension(t *testing.T) {
	for _, granularity := range []string{
		service.OrganizationUsageGranularityDay,
		service.OrganizationUsageGranularityWeek,
		service.OrganizationUsageGranularityMonth,
	} {
		query, err := organizationUsageTrendQuery(granularity)
		require.NoError(t, err, granularity)
		normalized := strings.ToLower(query)
		require.Contains(t, normalized, "generate_series")
		require.NotContains(t, normalized, "group by user_id")
		require.Contains(t, normalized, "group by bucket_start, bucket_end")
		require.Contains(t, query, "$7::date")
		require.Contains(t, query, "ORDER BY period_start ASC")
		require.Contains(t, query, "LEAST(b.bucket_end, $6::date, $7::date)")
	}

	weekQuery, err := organizationUsageTrendQuery(service.OrganizationUsageGranularityWeek)
	require.NoError(t, err)
	require.Contains(t, weekQuery, "date_trunc('week', $5::timestamp)")
	require.Contains(t, weekQuery, "date_trunc('week', $7::timestamp)")
	require.NotContains(t, weekQuery, "interval '7 day') AS gs")
}

func TestOrganizationUsageRepositoryTrend_ScansZeroFilledPoint(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	start := time.Date(2025, 12, 31, 16, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 10, 16, 0, 0, 0, time.UTC)
	params := service.OrganizationUsageTrendRepositoryParams{
		StartTime: start, EndTime: end,
		StartDate:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)),
		EndDate:      time.Date(2026, 1, 10, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)),
		DataThrough:  time.Date(2026, 1, 5, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)),
		Organization: service.OrganizationAll, Q: "dev",
		Granularity: service.OrganizationUsageGranularityDay,
	}

	query, err := organizationUsageTrendQuery(service.OrganizationUsageGranularityDay)
	require.NoError(t, err)
	mock.ExpectQuery(regexp.QuoteMeta(query)).
		WithArgs(start, end, "%dev%", service.OrganizationAll, "2026-01-01", "2026-01-10", "2026-01-05").
		WillReturnRows(sqlmock.NewRows([]string{
			"period_start", "period_end", "partial",
			"requests", "input_tokens", "output_tokens", "cache_creation_tokens", "cache_read_tokens", "total_tokens", "actual_cost",
		}).AddRow(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), false,
			0, 0, 0, 0, 0, 0, 0.0))

	repo := NewOrganizationUsageRepository(db)
	got, err := repo.Trend(context.Background(), params)
	require.NoError(t, err)
	require.Len(t, got.Points, 1)
	require.Equal(t, "2026-01-01", got.Points[0].PeriodStart)
	require.Equal(t, int64(0), got.Points[0].TotalTokens)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOrganizationUsageRepositoryPeriods_ScansPartialWeekAndPagination(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	start := time.Date(2025, 12, 31, 16, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 10, 16, 0, 0, 0, time.UTC)
	params := service.OrganizationUsagePeriodsRepositoryParams{
		StartTime: start, EndTime: end,
		StartDate:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)),
		EndDate:      time.Date(2026, 1, 10, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60)),
		Organization: service.OrganizationAll, Q: "dev", Page: 2, PageSize: 10,
		Granularity: service.OrganizationUsageGranularityWeek,
	}

	countQuery, err := organizationUsagePeriodsCountQuery(service.OrganizationUsageGranularityWeek)
	require.NoError(t, err)
	mock.ExpectQuery(regexp.QuoteMeta(countQuery)).
		WithArgs(start, end, "%dev%", service.OrganizationAll).
		WillReturnRows(sqlmock.NewRows([]string{"total_count"}).AddRow(11))
	mock.ExpectQuery(regexp.QuoteMeta("ul.created_at >= $1 AND ul.created_at < $2")).
		WithArgs(start, end, "%dev%", service.OrganizationAll, "2026-01-01", "2026-01-10", 10, 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"total_count", "period_start", "period_end", "partial", "user_id", "email", "organization",
			"requests", "input_tokens", "output_tokens", "cache_creation_tokens", "cache_read_tokens", "total_tokens", "actual_cost",
		}).AddRow(11, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC), true,
			9, "DEV@XUNYOU.COM", service.OrganizationXunyou, 3, 10, 20, 30, 40, 100, 1.25))

	repo := NewOrganizationUsageRepository(db)
	got, err := repo.Periods(context.Background(), params)
	require.NoError(t, err)
	require.Equal(t, int64(11), got.Total)
	require.Len(t, got.Items, 1)
	require.Equal(t, "2026-01-01", got.Items[0].PeriodStart)
	require.Equal(t, "2026-01-04", got.Items[0].PeriodEnd)
	require.True(t, got.Items[0].Partial)
	require.Equal(t, int64(100), got.Items[0].TotalTokens)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOrganizationUsageRepositoryPeriods_PreservesTotalForEmptyPage(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	start := time.Date(2025, 12, 31, 16, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 10, 16, 0, 0, 0, time.UTC)
	params := service.OrganizationUsagePeriodsRepositoryParams{
		StartTime: start, EndTime: end,
		StartDate:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:      time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
		Organization: service.OrganizationAll, Page: 99, PageSize: 10,
		Granularity: service.OrganizationUsageGranularityDay,
	}

	countQuery, err := organizationUsagePeriodsCountQuery(service.OrganizationUsageGranularityDay)
	require.NoError(t, err)
	mock.ExpectQuery(regexp.QuoteMeta(countQuery)).
		WithArgs(start, end, "", service.OrganizationAll).
		WillReturnRows(sqlmock.NewRows([]string{"total_count"}).AddRow(11))
	itemsQuery, err := organizationUsagePeriodsQuery(service.OrganizationUsageGranularityDay)
	require.NoError(t, err)
	mock.ExpectQuery(regexp.QuoteMeta(itemsQuery)).
		WithArgs(start, end, "", service.OrganizationAll, "2026-01-01", "2026-01-10", 10, 980).
		WillReturnRows(sqlmock.NewRows([]string{
			"period_start", "period_end", "partial", "user_id", "email", "organization",
			"requests", "input_tokens", "output_tokens", "cache_creation_tokens", "cache_read_tokens", "total_tokens", "actual_cost",
		}))

	repo := NewOrganizationUsageRepository(db)
	got, err := repo.Periods(context.Background(), params)
	require.NoError(t, err)
	require.Empty(t, got.Items)
	require.Equal(t, int64(11), got.Total)
	require.NoError(t, mock.ExpectationsWereMet())
}
