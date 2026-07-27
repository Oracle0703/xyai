package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type organizationUsageRepositoryStub struct {
	summaryParams OrganizationUsageSummaryRepositoryParams
	periodParams  OrganizationUsagePeriodsRepositoryParams
	trendParams   OrganizationUsageTrendRepositoryParams
	summaryCalls  int
	periodCalls   int
	trendCalls    int
}

func (s *organizationUsageRepositoryStub) Summary(_ context.Context, params OrganizationUsageSummaryRepositoryParams) (*OrganizationUsageSummaryRepositoryResult, error) {
	s.summaryCalls++
	s.summaryParams = params
	return &OrganizationUsageSummaryRepositoryResult{
		Overview:      OrganizationUsageOverview{ActiveUsers: 2, UsedUsers: 1},
		Organizations: []OrganizationUsageOrganization{{Organization: OrganizationXunyou}},
		Items:         []OrganizationUsageSummaryItem{{UserID: 9, Email: "dev@xunyou.com", Organization: OrganizationXunyou}},
		Total:         21,
	}, nil
}

func (s *organizationUsageRepositoryStub) Periods(_ context.Context, params OrganizationUsagePeriodsRepositoryParams) (*OrganizationUsagePeriodsRepositoryResult, error) {
	s.periodCalls++
	s.periodParams = params
	return &OrganizationUsagePeriodsRepositoryResult{
		Items: []OrganizationUsagePeriod{{UserID: 9, Email: "dev@xunyou.com", Organization: OrganizationXunyou}},
		Total: 7,
	}, nil
}

func (s *organizationUsageRepositoryStub) Trend(_ context.Context, params OrganizationUsageTrendRepositoryParams) (*OrganizationUsageTrendRepositoryResult, error) {
	s.trendCalls++
	s.trendParams = params
	return &OrganizationUsageTrendRepositoryResult{
		Points: []OrganizationUsageTrendPoint{{
			PeriodStart: params.StartDate.Format("2006-01-02"),
			PeriodEnd:   params.DataThrough.Format("2006-01-02"),
		}},
	}, nil
}

func TestOrganizationUsageServiceSummary_NormalizesShanghaiRangeAndPagination(t *testing.T) {
	repo := &organizationUsageRepositoryStub{}
	svc := NewOrganizationUsageService(repo)

	got, err := svc.Summary(context.Background(), OrganizationUsageSummaryQuery{
		StartDate:    "2026-01-01",
		EndDate:      "2026-01-31",
		Organization: OrganizationXunyou,
		Q:            " DEV@ ",
		Page:         2,
		PageSize:     10,
		SortBy:       "requests",
		SortOrder:    "asc",
	})
	require.NoError(t, err)
	require.Equal(t, time.Date(2025, 12, 31, 16, 0, 0, 0, time.UTC), repo.summaryParams.StartTime)
	require.Equal(t, time.Date(2026, 1, 31, 16, 0, 0, 0, time.UTC), repo.summaryParams.EndTime)
	require.Equal(t, "DEV@", repo.summaryParams.Q)
	require.Equal(t, OrganizationUsageRange{StartDate: "2026-01-01", EndDate: "2026-01-31"}, got.Range)
	require.Equal(t, OrganizationUsagePagination{Total: 21, Page: 2, PageSize: 10, Pages: 3}, got.Pagination)
}

func TestOrganizationUsageServiceSummary_AcceptsAtMost366CalendarDays(t *testing.T) {
	svc := NewOrganizationUsageService(&organizationUsageRepositoryStub{})

	_, err := svc.Summary(context.Background(), OrganizationUsageSummaryQuery{
		StartDate: "2024-01-01", EndDate: "2024-12-31",
	})
	require.NoError(t, err)

	_, err = svc.Summary(context.Background(), OrganizationUsageSummaryQuery{
		StartDate: "2024-01-01", EndDate: "2025-01-01",
	})
	require.ErrorContains(t, err, "366")
}

func TestOrganizationUsageServiceSummary_RejectsInvalidQueryValues(t *testing.T) {
	tests := []struct {
		name  string
		query OrganizationUsageSummaryQuery
	}{
		{name: "missing start", query: OrganizationUsageSummaryQuery{EndDate: "2026-01-01"}},
		{name: "invalid date", query: OrganizationUsageSummaryQuery{StartDate: "2026-1-1", EndDate: "2026-01-01"}},
		{name: "reverse range", query: OrganizationUsageSummaryQuery{StartDate: "2026-01-02", EndDate: "2026-01-01"}},
		{name: "organization", query: OrganizationUsageSummaryQuery{StartDate: "2026-01-01", EndDate: "2026-01-01", Organization: "unknown"}},
		{name: "sort by", query: OrganizationUsageSummaryQuery{StartDate: "2026-01-01", EndDate: "2026-01-01", SortBy: "cost desc"}},
		{name: "sort order", query: OrganizationUsageSummaryQuery{StartDate: "2026-01-01", EndDate: "2026-01-01", SortOrder: "sideways"}},
		{name: "page", query: OrganizationUsageSummaryQuery{StartDate: "2026-01-01", EndDate: "2026-01-01", Page: -1}},
		{name: "page size", query: OrganizationUsageSummaryQuery{StartDate: "2026-01-01", EndDate: "2026-01-01", PageSize: 1001}},
	}

	svc := NewOrganizationUsageService(&organizationUsageRepositoryStub{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Summary(context.Background(), tt.query)
			var validationErr *OrganizationUsageValidationError
			require.ErrorAs(t, err, &validationErr)
		})
	}
}

func TestOrganizationUsageServicePeriods_ValidatesGranularityAndDefaults(t *testing.T) {
	repo := &organizationUsageRepositoryStub{}
	svc := NewOrganizationUsageService(repo)

	got, err := svc.Periods(context.Background(), OrganizationUsagePeriodsQuery{
		StartDate: "2026-02-01", EndDate: "2026-02-02", Granularity: OrganizationUsageGranularityWeek,
	})
	require.NoError(t, err)
	require.Equal(t, OrganizationAll, repo.periodParams.Organization)
	require.Equal(t, 1, repo.periodParams.Page)
	require.Equal(t, 20, repo.periodParams.PageSize)
	require.Equal(t, OrganizationUsageGranularityWeek, got.Granularity)
	require.Equal(t, OrganizationUsagePagination{Total: 7, Page: 1, PageSize: 20, Pages: 1}, got.Pagination)

	_, err = svc.Periods(context.Background(), OrganizationUsagePeriodsQuery{
		StartDate: "2026-02-01", EndDate: "2026-02-02", Granularity: "quarter",
	})
	var validationErr *OrganizationUsageValidationError
	require.ErrorAs(t, err, &validationErr)
}

func TestOrganizationUsageServiceSummary_AsOfClipsEndAndReturnsCanonicalUTC(t *testing.T) {
	repo := &organizationUsageRepositoryStub{}
	svc := NewOrganizationUsageService(repo)

	got, err := svc.Summary(context.Background(), OrganizationUsageSummaryQuery{
		StartDate: "2026-01-01",
		EndDate:   "2026-01-31",
		AsOf:      "2026-01-20T18:30:00.123456789+08:00",
	})
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 1, 20, 10, 30, 0, 123456789, time.UTC), repo.summaryParams.EndTime)
	require.Equal(t, "2026-01-20T10:30:00.123456789Z", got.Range.AsOf)
}

func TestOrganizationUsageServicePeriods_AsOfLaterThanRangeKeepsOriginalEnd(t *testing.T) {
	repo := &organizationUsageRepositoryStub{}
	svc := NewOrganizationUsageService(repo)

	got, err := svc.Periods(context.Background(), OrganizationUsagePeriodsQuery{
		StartDate: "2026-01-01",
		EndDate:   "2026-01-31",
		AsOf:      "2026-02-10T00:00:00Z",
	})
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 1, 31, 16, 0, 0, 0, time.UTC), repo.periodParams.EndTime)
	require.Equal(t, "2026-02-10T00:00:00Z", got.Range.AsOf)
}

func TestOrganizationUsageService_AsOfBeforeRangeClampsEndToStart(t *testing.T) {
	repo := &organizationUsageRepositoryStub{}
	svc := NewOrganizationUsageService(repo)

	_, err := svc.Summary(context.Background(), OrganizationUsageSummaryQuery{
		StartDate: "2026-01-10",
		EndDate:   "2026-01-31",
		AsOf:      "2026-01-01T00:00:00Z",
	})
	require.NoError(t, err)
	require.Equal(t, repo.summaryParams.StartTime, repo.summaryParams.EndTime)
}

func TestOrganizationUsageService_RejectsInvalidAsOfWithoutCallingRepository(t *testing.T) {
	repo := &organizationUsageRepositoryStub{}
	svc := NewOrganizationUsageService(repo)

	_, summaryErr := svc.Summary(context.Background(), OrganizationUsageSummaryQuery{
		StartDate: "2026-01-01", EndDate: "2026-01-31", AsOf: "2026-01-20T12:00:00,123Z",
	})
	_, periodsErr := svc.Periods(context.Background(), OrganizationUsagePeriodsQuery{
		StartDate: "2026-01-01", EndDate: "2026-01-31", AsOf: "not-a-timestamp",
	})

	var validationErr *OrganizationUsageValidationError
	require.ErrorAs(t, summaryErr, &validationErr)
	require.ErrorAs(t, periodsErr, &validationErr)
	require.Zero(t, repo.summaryCalls)
	require.Zero(t, repo.periodCalls)
}

func TestOrganizationUsageService_AsOfIsClampedToServerNow(t *testing.T) {
	repo := &organizationUsageRepositoryStub{}
	svc := NewOrganizationUsageService(repo)
	svc.now = func() time.Time { return time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC) }

	got, err := svc.Summary(context.Background(), OrganizationUsageSummaryQuery{
		StartDate: "2026-07-01", EndDate: "2026-07-31", AsOf: "2030-01-01T00:00:00Z",
	})
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC), repo.summaryParams.EndTime)
	require.Equal(t, "2026-07-10T08:00:00Z", got.Range.AsOf)
}

func TestOrganizationUsageService_PeriodsAsOfIsClampedToServerNow(t *testing.T) {
	repo := &organizationUsageRepositoryStub{}
	svc := NewOrganizationUsageService(repo)
	svc.now = func() time.Time { return time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC) }

	got, err := svc.Periods(context.Background(), OrganizationUsagePeriodsQuery{
		StartDate: "2030-01-01", EndDate: "2030-12-31", AsOf: "2030-07-01T00:00:00Z",
	})
	require.NoError(t, err)
	require.Equal(t, repo.periodParams.StartTime, repo.periodParams.EndTime)
	require.Equal(t, "2026-07-10T08:00:00Z", got.Range.AsOf)
}

func TestOrganizationUsageService_PastAsOfIsNotChangedByServerNow(t *testing.T) {
	repo := &organizationUsageRepositoryStub{}
	svc := NewOrganizationUsageService(repo)
	svc.now = func() time.Time { return time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC) }

	got, err := svc.Periods(context.Background(), OrganizationUsagePeriodsQuery{
		StartDate: "2026-07-01", EndDate: "2026-07-31", AsOf: "2026-07-05T10:15:30.25Z",
	})
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 7, 5, 10, 15, 30, 250000000, time.UTC), repo.periodParams.EndTime)
	require.Equal(t, "2026-07-05T10:15:30.25Z", got.Range.AsOf)
}

func TestOrganizationUsageService_DateEndClipsRepositoryButNotSignedAsOf(t *testing.T) {
	repo := &organizationUsageRepositoryStub{}
	svc := NewOrganizationUsageService(repo)
	svc.now = func() time.Time { return time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC) }

	got, err := svc.Summary(context.Background(), OrganizationUsageSummaryQuery{
		StartDate: "2025-01-01", EndDate: "2025-01-31", AsOf: "2030-01-01T00:00:00Z",
	})
	require.NoError(t, err)
	require.Equal(t, time.Date(2025, 1, 31, 16, 0, 0, 0, time.UTC), repo.summaryParams.EndTime)
	require.Equal(t, "2026-07-10T08:00:00Z", got.Range.AsOf)
}

func TestOrganizationUsageService_WithoutAsOfDoesNotReadSigningClock(t *testing.T) {
	repo := &organizationUsageRepositoryStub{}
	svc := NewOrganizationUsageService(repo)
	svc.now = func() time.Time {
		t.Fatal("signing clock must not be read without as_of")
		return time.Time{}
	}

	got, err := svc.Summary(context.Background(), OrganizationUsageSummaryQuery{
		StartDate: "2026-07-01", EndDate: "2026-07-31",
	})
	require.NoError(t, err)
	require.Empty(t, got.Range.AsOf)
}

func TestOrganizationUsageServiceTrend_DefaultsGranularityAndDerivesDataThrough(t *testing.T) {
	repo := &organizationUsageRepositoryStub{}
	svc := NewOrganizationUsageService(repo)
	svc.now = func() time.Time { return time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC) } // 18:00 Shanghai

	got, err := svc.Trend(context.Background(), OrganizationUsageTrendQuery{
		StartDate: "2026-07-01", EndDate: "2026-07-31", Organization: OrganizationXunyou, Q: " alice ",
	})
	require.NoError(t, err)
	require.Equal(t, OrganizationUsageGranularityDay, got.Granularity)
	require.Equal(t, "2026-07-15", got.DataThrough)
	require.Empty(t, got.Range.AsOf)
	require.Equal(t, OrganizationXunyou, repo.trendParams.Organization)
	require.Equal(t, "alice", repo.trendParams.Q)
	require.Equal(t, time.Date(2026, 7, 15, 0, 0, 0, 0, organizationUsageLocation), repo.trendParams.DataThrough)
	require.Equal(t, 1, repo.trendCalls)
}

func TestOrganizationUsageServiceTrend_UsesAsOfCanonicalAndClipsDataThroughToEnd(t *testing.T) {
	repo := &organizationUsageRepositoryStub{}
	svc := NewOrganizationUsageService(repo)
	svc.now = func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) }

	got, err := svc.Trend(context.Background(), OrganizationUsageTrendQuery{
		StartDate: "2026-01-01", EndDate: "2026-01-10",
		AsOf:        "2026-01-20T18:30:00+08:00",
		Granularity: OrganizationUsageGranularityWeek,
	})
	require.NoError(t, err)
	require.Equal(t, "2026-01-10", got.DataThrough)
	require.Equal(t, "2026-01-20T10:30:00Z", got.Range.AsOf)
	require.Equal(t, OrganizationUsageGranularityWeek, got.Granularity)
	require.Equal(t, time.Date(2026, 1, 10, 0, 0, 0, 0, organizationUsageLocation), repo.trendParams.DataThrough)
}

func TestOrganizationUsageServiceTrend_FutureOnlyRangeReturnsEmptyPointsWithoutRepo(t *testing.T) {
	repo := &organizationUsageRepositoryStub{}
	svc := NewOrganizationUsageService(repo)
	svc.now = func() time.Time { return time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC) }

	got, err := svc.Trend(context.Background(), OrganizationUsageTrendQuery{
		StartDate: "2030-01-01", EndDate: "2030-01-31", AsOf: "2030-01-15T00:00:00Z",
	})
	require.NoError(t, err)
	require.Empty(t, got.Points)
	require.Empty(t, got.DataThrough)
	require.Equal(t, "2026-07-10T08:00:00Z", got.Range.AsOf)
	require.Zero(t, repo.trendCalls)
}

func TestOrganizationUsageServiceTrend_RejectsInvalidGranularity(t *testing.T) {
	repo := &organizationUsageRepositoryStub{}
	svc := NewOrganizationUsageService(repo)
	svc.now = func() time.Time { return time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC) }

	_, err := svc.Trend(context.Background(), OrganizationUsageTrendQuery{
		StartDate: "2026-07-01", EndDate: "2026-07-31", Granularity: "quarter",
	})
	var validationErr *OrganizationUsageValidationError
	require.ErrorAs(t, err, &validationErr)
	require.Zero(t, repo.trendCalls)
}
