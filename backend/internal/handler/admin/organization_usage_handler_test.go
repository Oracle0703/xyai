package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type organizationUsageServiceStub struct {
	summaryQuery service.OrganizationUsageSummaryQuery
	periodsQuery service.OrganizationUsagePeriodsQuery
	trendQuery   service.OrganizationUsageTrendQuery
	summary      *service.OrganizationUsageSummaryResponse
	periods      *service.OrganizationUsagePeriodsResponse
	trend        *service.OrganizationUsageTrendResponse
	err          error
	called       bool
}

func (s *organizationUsageServiceStub) Summary(_ context.Context, query service.OrganizationUsageSummaryQuery) (*service.OrganizationUsageSummaryResponse, error) {
	s.called = true
	s.summaryQuery = query
	return s.summary, s.err
}

func (s *organizationUsageServiceStub) Periods(_ context.Context, query service.OrganizationUsagePeriodsQuery) (*service.OrganizationUsagePeriodsResponse, error) {
	s.called = true
	s.periodsQuery = query
	return s.periods, s.err
}

func (s *organizationUsageServiceStub) Trend(_ context.Context, query service.OrganizationUsageTrendQuery) (*service.OrganizationUsageTrendResponse, error) {
	s.called = true
	s.trendQuery = query
	return s.trend, s.err
}

func TestOrganizationUsageHandlerSummary_BindsStrictQueryAndReturnsEnvelope(t *testing.T) {
	stub := &organizationUsageServiceStub{summary: &service.OrganizationUsageSummaryResponse{
		Range: service.OrganizationUsageRange{StartDate: "2026-01-01", EndDate: "2026-01-02"},
	}}
	h := &OrganizationUsageHandler{service: stub}
	w := performOrganizationUsageRequest(h.Summary, "/?start_date=2026-01-01&end_date=2026-01-02&as_of=2026-01-02T08%3A30%3A00%2B08%3A00&organization=xunyou&q=alice&sort_by=requests&sort_order=asc&page=2&page_size=30")

	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"code":0,"message":"success","data":{"range":{"start_date":"2026-01-01","end_date":"2026-01-02"},"overview":{"active_users":0,"used_users":0,"requests":0,"input_tokens":0,"output_tokens":0,"cache_creation_tokens":0,"cache_read_tokens":0,"total_tokens":0,"actual_cost":0},"organizations":null,"champions":{"day":null,"week":null,"month":null},"items":null,"pagination":{"total":0,"page":0,"page_size":0,"pages":0}}}`, w.Body.String())
	require.Equal(t, 2, stub.summaryQuery.Page)
	require.Equal(t, 30, stub.summaryQuery.PageSize)
	require.Equal(t, "alice", stub.summaryQuery.Q)
	require.Equal(t, "requests", stub.summaryQuery.SortBy)
	require.Equal(t, "2026-01-02T08:30:00+08:00", stub.summaryQuery.AsOf)
}

func TestOrganizationUsageHandlerSummary_RejectsInvalidPaginationWithoutCallingService(t *testing.T) {
	stub := &organizationUsageServiceStub{}
	h := &OrganizationUsageHandler{service: stub}
	w := performOrganizationUsageRequest(h.Summary, "/?start_date=2026-01-01&end_date=2026-01-02&page=abc")

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.False(t, stub.called)
}

func TestOrganizationUsageHandler_RejectsExplicitZeroPaginationWithoutCallingService(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		periods bool
	}{
		{name: "summary page", target: "/?start_date=2026-01-01&end_date=2026-01-02&page=0"},
		{name: "summary page size", target: "/?start_date=2026-01-01&end_date=2026-01-02&page_size=0"},
		{name: "periods page", target: "/?start_date=2026-01-01&end_date=2026-01-02&page=0", periods: true},
		{name: "periods page size", target: "/?start_date=2026-01-01&end_date=2026-01-02&page_size=0", periods: true},
		{name: "summary negative page", target: "/?start_date=2026-01-01&end_date=2026-01-02&page=-1"},
		{name: "periods negative page size", target: "/?start_date=2026-01-01&end_date=2026-01-02&page_size=-1", periods: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &organizationUsageServiceStub{}
			h := &OrganizationUsageHandler{service: stub}
			handler := h.Summary
			if tt.periods {
				handler = h.Periods
			}

			w := performOrganizationUsageRequest(handler, tt.target)

			require.Equal(t, http.StatusBadRequest, w.Code)
			require.False(t, stub.called)
		})
	}
}

func TestOrganizationUsageHandlerSummary_MapsValidationAndInternalErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "validation", err: &service.OrganizationUsageValidationError{Field: "sort_by", Message: "invalid sort_by"}, want: http.StatusBadRequest},
		{name: "internal", err: errors.New("db unavailable"), want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &organizationUsageServiceStub{err: tt.err}
			h := &OrganizationUsageHandler{service: stub}
			w := performOrganizationUsageRequest(h.Summary, "/?start_date=2026-01-01&end_date=2026-01-02")
			require.Equal(t, tt.want, w.Code)
		})
	}
}

func TestOrganizationUsageHandlerPeriods_BindsGranularity(t *testing.T) {
	stub := &organizationUsageServiceStub{periods: &service.OrganizationUsagePeriodsResponse{Granularity: "week"}}
	h := &OrganizationUsageHandler{service: stub}
	w := performOrganizationUsageRequest(h.Periods, "/?start_date=2026-01-01&end_date=2026-01-31&as_of=2026-01-20T12%3A00%3A00Z&granularity=week&page=3&page_size=5")

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "week", stub.periodsQuery.Granularity)
	require.Equal(t, 3, stub.periodsQuery.Page)
	require.Equal(t, 5, stub.periodsQuery.PageSize)
	require.Equal(t, "2026-01-20T12:00:00Z", stub.periodsQuery.AsOf)
}

func TestOrganizationUsageHandlerTrend_BindsQueryWithoutPagination(t *testing.T) {
	stub := &organizationUsageServiceStub{trend: &service.OrganizationUsageTrendResponse{
		Granularity: "day",
		DataThrough: "2026-01-15",
		Points:      []service.OrganizationUsageTrendPoint{},
	}}
	h := &OrganizationUsageHandler{service: stub}
	w := performOrganizationUsageRequest(h.Trend, "/?start_date=2026-01-01&end_date=2026-01-31&as_of=2026-01-15T12%3A00%3A00Z&organization=xunyou&q=alice&granularity=week")

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "week", stub.trendQuery.Granularity)
	require.Equal(t, "xunyou", stub.trendQuery.Organization)
	require.Equal(t, "alice", stub.trendQuery.Q)
	require.Equal(t, "2026-01-15T12:00:00Z", stub.trendQuery.AsOf)
}

type organizationUsageHandlerRepositoryStub struct{}

func (organizationUsageHandlerRepositoryStub) Summary(context.Context, service.OrganizationUsageSummaryRepositoryParams) (*service.OrganizationUsageSummaryRepositoryResult, error) {
	return &service.OrganizationUsageSummaryRepositoryResult{}, nil
}

func (organizationUsageHandlerRepositoryStub) Periods(context.Context, service.OrganizationUsagePeriodsRepositoryParams) (*service.OrganizationUsagePeriodsRepositoryResult, error) {
	return &service.OrganizationUsagePeriodsRepositoryResult{}, nil
}

func (organizationUsageHandlerRepositoryStub) Trend(context.Context, service.OrganizationUsageTrendRepositoryParams) (*service.OrganizationUsageTrendRepositoryResult, error) {
	return &service.OrganizationUsageTrendRepositoryResult{Points: []service.OrganizationUsageTrendPoint{}}, nil
}

func TestOrganizationUsageHandler_RejectsInvalidAsOf(t *testing.T) {
	h := NewOrganizationUsageHandler(service.NewOrganizationUsageService(organizationUsageHandlerRepositoryStub{}))
	tests := []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{name: "summary"},
		{name: "periods"},
		{name: "trend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := h.Summary
			switch tt.name {
			case "periods":
				handler = h.Periods
			case "trend":
				handler = h.Trend
			}
			w := performOrganizationUsageRequest(handler, "/?start_date=2026-01-01&end_date=2026-01-31&as_of=invalid")
			require.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func performOrganizationUsageRequest(handler gin.HandlerFunc, target string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	handler(c)
	return w
}
