package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	OrganizationAll     = "all"
	OrganizationXunyou  = "xunyou"
	OrganizationWsdashi = "wsdashi"
	OrganizationOther   = "other"

	OrganizationUsageGranularityDay   = "day"
	OrganizationUsageGranularityWeek  = "week"
	OrganizationUsageGranularityMonth = "month"
)

var organizationUsageLocation = time.FixedZone("Asia/Shanghai", 8*60*60)
var organizationUsageRFC3339Pattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$`)

type OrganizationUsageValidationError struct {
	Field   string
	Message string
}

func (e *OrganizationUsageValidationError) Error() string {
	return e.Message
}

type OrganizationUsageMetrics struct {
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	ActualCost          float64 `json:"actual_cost"`
}

type OrganizationUsageRange struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	AsOf      string `json:"as_of,omitempty"`
}

type OrganizationUsagePagination struct {
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Pages    int   `json:"pages"`
}

type OrganizationUsagePeriod struct {
	PeriodStart  string `json:"period_start"`
	PeriodEnd    string `json:"period_end"`
	Partial      bool   `json:"partial"`
	UserID       int64  `json:"user_id"`
	Email        string `json:"email"`
	Organization string `json:"organization"`
	OrganizationUsageMetrics
}

type OrganizationUsageSummaryItem struct {
	UserID       int64  `json:"user_id"`
	Email        string `json:"email"`
	Organization string `json:"organization"`
	OrganizationUsageMetrics
	PeakDay   *OrganizationUsagePeriod `json:"peak_day"`
	PeakWeek  *OrganizationUsagePeriod `json:"peak_week"`
	PeakMonth *OrganizationUsagePeriod `json:"peak_month"`
}

type OrganizationUsageOverview struct {
	ActiveUsers int64 `json:"active_users"`
	UsedUsers   int64 `json:"used_users"`
	OrganizationUsageMetrics
}

type OrganizationUsageOrganization struct {
	Organization string `json:"organization"`
	ActiveUsers  int64  `json:"active_users"`
	UsedUsers    int64  `json:"used_users"`
	OrganizationUsageMetrics
}

type OrganizationUsageChampions struct {
	Day   *OrganizationUsagePeriod `json:"day"`
	Week  *OrganizationUsagePeriod `json:"week"`
	Month *OrganizationUsagePeriod `json:"month"`
}

type OrganizationUsageSummaryResponse struct {
	Range         OrganizationUsageRange          `json:"range"`
	Overview      OrganizationUsageOverview       `json:"overview"`
	Organizations []OrganizationUsageOrganization `json:"organizations"`
	Champions     OrganizationUsageChampions      `json:"champions"`
	Items         []OrganizationUsageSummaryItem  `json:"items"`
	Pagination    OrganizationUsagePagination     `json:"pagination"`
}

type OrganizationUsagePeriodsResponse struct {
	Range       OrganizationUsageRange      `json:"range"`
	Granularity string                      `json:"granularity"`
	Items       []OrganizationUsagePeriod   `json:"items"`
	Pagination  OrganizationUsagePagination `json:"pagination"`
}

// OrganizationUsageTrendPoint is one zero-filled calendar bucket for the trend chart.
type OrganizationUsageTrendPoint struct {
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
	Partial     bool   `json:"partial"`
	OrganizationUsageMetrics
}

// OrganizationUsageTrendResponse is the continuous time series for organization-usage charts.
type OrganizationUsageTrendResponse struct {
	Range       OrganizationUsageRange        `json:"range"`
	DataThrough string                        `json:"data_through,omitempty"`
	Granularity string                        `json:"granularity"`
	Points      []OrganizationUsageTrendPoint `json:"points"`
}

type OrganizationUsageSummaryQuery struct {
	StartDate    string
	EndDate      string
	AsOf         string
	Organization string
	Q            string
	Page         int
	PageSize     int
	SortBy       string
	SortOrder    string
}

type OrganizationUsagePeriodsQuery struct {
	StartDate    string
	EndDate      string
	AsOf         string
	Organization string
	Q            string
	Page         int
	PageSize     int
	Granularity  string
}

// OrganizationUsageTrendQuery filters the zero-filled trend series (no pagination).
type OrganizationUsageTrendQuery struct {
	StartDate    string
	EndDate      string
	AsOf         string
	Organization string
	Q            string
	Granularity  string
}

type OrganizationUsageSummaryRepositoryParams struct {
	StartTime    time.Time
	EndTime      time.Time
	StartDate    time.Time
	EndDate      time.Time
	Organization string
	Q            string
	Page         int
	PageSize     int
	SortBy       string
	SortOrder    string
}

type OrganizationUsagePeriodsRepositoryParams struct {
	StartTime    time.Time
	EndTime      time.Time
	StartDate    time.Time
	EndDate      time.Time
	Organization string
	Q            string
	Page         int
	PageSize     int
	Granularity  string
}

// OrganizationUsageTrendRepositoryParams keeps Periods $1..$6 bindings and adds $7=data_through.
type OrganizationUsageTrendRepositoryParams struct {
	StartTime    time.Time
	EndTime      time.Time
	StartDate    time.Time
	EndDate      time.Time
	DataThrough  time.Time
	Organization string
	Q            string
	Granularity  string
}

type OrganizationUsageSummaryRepositoryResult struct {
	Overview      OrganizationUsageOverview
	Organizations []OrganizationUsageOrganization
	Champions     OrganizationUsageChampions
	Items         []OrganizationUsageSummaryItem
	Total         int64
}

type OrganizationUsagePeriodsRepositoryResult struct {
	Items []OrganizationUsagePeriod
	Total int64
}

type OrganizationUsageTrendRepositoryResult struct {
	Points []OrganizationUsageTrendPoint
}

type OrganizationUsageRepository interface {
	Summary(context.Context, OrganizationUsageSummaryRepositoryParams) (*OrganizationUsageSummaryRepositoryResult, error)
	Periods(context.Context, OrganizationUsagePeriodsRepositoryParams) (*OrganizationUsagePeriodsRepositoryResult, error)
	Trend(context.Context, OrganizationUsageTrendRepositoryParams) (*OrganizationUsageTrendRepositoryResult, error)
}

type OrganizationUsageService struct {
	repo OrganizationUsageRepository
	now  func() time.Time
}

func NewOrganizationUsageService(repo OrganizationUsageRepository) *OrganizationUsageService {
	return &OrganizationUsageService{repo: repo, now: time.Now}
}

func (s *OrganizationUsageService) Summary(ctx context.Context, query OrganizationUsageSummaryQuery) (*OrganizationUsageSummaryResponse, error) {
	base, err := normalizeOrganizationUsageQuery(query.StartDate, query.EndDate, query.AsOf, query.Organization, query.Q, query.Page, query.PageSize)
	if err != nil {
		return nil, err
	}
	if base.asOf != nil {
		base.clampAsOfToServerNow(s.now())
	}

	sortBy := strings.TrimSpace(query.SortBy)
	if sortBy == "" {
		sortBy = "total_tokens"
	}
	if !validOrganizationUsageSortBy(sortBy) {
		return nil, organizationUsageValidation("sort_by", "invalid sort_by")
	}
	sortOrder := strings.ToLower(strings.TrimSpace(query.SortOrder))
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		return nil, organizationUsageValidation("sort_order", "invalid sort_order")
	}

	startTime, endTime := base.repositoryRange()
	result, err := s.repo.Summary(ctx, OrganizationUsageSummaryRepositoryParams{
		StartTime: startTime, EndTime: endTime,
		StartDate: base.start, EndDate: base.end,
		Organization: base.organization, Q: base.q, Page: base.page, PageSize: base.pageSize,
		SortBy: sortBy, SortOrder: sortOrder,
	})
	if err != nil {
		return nil, err
	}
	return &OrganizationUsageSummaryResponse{
		Range:         OrganizationUsageRange{StartDate: query.StartDate, EndDate: query.EndDate, AsOf: base.asOfCanonical},
		Overview:      result.Overview,
		Organizations: result.Organizations,
		Champions:     result.Champions,
		Items:         result.Items,
		Pagination:    organizationUsagePagination(result.Total, base.page, base.pageSize),
	}, nil
}

func (s *OrganizationUsageService) Periods(ctx context.Context, query OrganizationUsagePeriodsQuery) (*OrganizationUsagePeriodsResponse, error) {
	base, err := normalizeOrganizationUsageQuery(query.StartDate, query.EndDate, query.AsOf, query.Organization, query.Q, query.Page, query.PageSize)
	if err != nil {
		return nil, err
	}
	if base.asOf != nil {
		base.clampAsOfToServerNow(s.now())
	}
	granularity := strings.ToLower(strings.TrimSpace(query.Granularity))
	if granularity == "" {
		granularity = OrganizationUsageGranularityDay
	}
	if granularity != OrganizationUsageGranularityDay && granularity != OrganizationUsageGranularityWeek && granularity != OrganizationUsageGranularityMonth {
		return nil, organizationUsageValidation("granularity", "invalid granularity")
	}

	startTime, endTime := base.repositoryRange()
	result, err := s.repo.Periods(ctx, OrganizationUsagePeriodsRepositoryParams{
		StartTime: startTime, EndTime: endTime,
		StartDate: base.start, EndDate: base.end,
		Organization: base.organization, Q: base.q, Page: base.page, PageSize: base.pageSize,
		Granularity: granularity,
	})
	if err != nil {
		return nil, err
	}
	return &OrganizationUsagePeriodsResponse{
		Range:       OrganizationUsageRange{StartDate: query.StartDate, EndDate: query.EndDate, AsOf: base.asOfCanonical},
		Granularity: granularity,
		Items:       result.Items,
		Pagination:  organizationUsagePagination(result.Total, base.page, base.pageSize),
	}, nil
}

// Trend returns a continuous zero-filled time series through data_through (no future buckets).
// Auto day/week/month inference is a frontend concern; the service only validates granularity.
func (s *OrganizationUsageService) Trend(ctx context.Context, query OrganizationUsageTrendQuery) (*OrganizationUsageTrendResponse, error) {
	// Placeholder page/pageSize only satisfy normalizeOrganizationUsageQuery; Trend is not paginated.
	base, err := normalizeOrganizationUsageQuery(query.StartDate, query.EndDate, query.AsOf, query.Organization, query.Q, 1, 20)
	if err != nil {
		return nil, err
	}
	serverNow := s.now()
	if base.asOf != nil {
		base.clampAsOfToServerNow(serverNow)
	}
	granularity := strings.ToLower(strings.TrimSpace(query.Granularity))
	if granularity == "" {
		granularity = OrganizationUsageGranularityDay
	}
	if granularity != OrganizationUsageGranularityDay && granularity != OrganizationUsageGranularityWeek && granularity != OrganizationUsageGranularityMonth {
		return nil, organizationUsageValidation("granularity", "invalid granularity")
	}

	startTime, endTime := base.repositoryRange()
	effectiveAsOf := serverNow.UTC()
	if base.asOf != nil {
		effectiveAsOf = base.asOf.UTC()
	}
	dataThrough := organizationUsageCalendarDate(effectiveAsOf)
	if dataThrough.After(base.end) {
		dataThrough = base.end
	}
	if dataThrough.Before(base.start) {
		return &OrganizationUsageTrendResponse{
			Range:       OrganizationUsageRange{StartDate: query.StartDate, EndDate: query.EndDate, AsOf: base.asOfCanonical},
			Granularity: granularity,
			Points:      []OrganizationUsageTrendPoint{},
		}, nil
	}

	result, err := s.repo.Trend(ctx, OrganizationUsageTrendRepositoryParams{
		StartTime: startTime, EndTime: endTime,
		StartDate: base.start, EndDate: base.end, DataThrough: dataThrough,
		Organization: base.organization, Q: base.q,
		Granularity: granularity,
	})
	if err != nil {
		return nil, err
	}
	points := result.Points
	if points == nil {
		points = []OrganizationUsageTrendPoint{}
	}
	return &OrganizationUsageTrendResponse{
		Range:       OrganizationUsageRange{StartDate: query.StartDate, EndDate: query.EndDate, AsOf: base.asOfCanonical},
		DataThrough: dataThrough.Format("2006-01-02"),
		Granularity: granularity,
		Points:      points,
	}, nil
}

// organizationUsageCalendarDate returns the Asia/Shanghai calendar day of t at local midnight.
func organizationUsageCalendarDate(t time.Time) time.Time {
	local := t.In(organizationUsageLocation)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, organizationUsageLocation)
}

type normalizedOrganizationUsageQuery struct {
	start         time.Time
	end           time.Time
	organization  string
	q             string
	page          int
	pageSize      int
	asOf          *time.Time
	asOfCanonical string
}

func normalizeOrganizationUsageQuery(startRaw, endRaw, asOfRaw, organization, q string, page, pageSize int) (normalizedOrganizationUsageQuery, error) {
	if strings.TrimSpace(startRaw) == "" || strings.TrimSpace(endRaw) == "" {
		return normalizedOrganizationUsageQuery{}, organizationUsageValidation("date_range", "start_date and end_date are required")
	}
	start, err := time.ParseInLocation("2006-01-02", startRaw, organizationUsageLocation)
	if err != nil || start.Format("2006-01-02") != startRaw {
		return normalizedOrganizationUsageQuery{}, organizationUsageValidation("start_date", "invalid start_date, use YYYY-MM-DD")
	}
	end, err := time.ParseInLocation("2006-01-02", endRaw, organizationUsageLocation)
	if err != nil || end.Format("2006-01-02") != endRaw {
		return normalizedOrganizationUsageQuery{}, organizationUsageValidation("end_date", "invalid end_date, use YYYY-MM-DD")
	}
	if end.Before(start) {
		return normalizedOrganizationUsageQuery{}, organizationUsageValidation("date_range", "end_date must not be before start_date")
	}
	if organizationUsageCalendarDays(start, end) > 366 {
		return normalizedOrganizationUsageQuery{}, organizationUsageValidation("date_range", "date range must not exceed 366 calendar days")
	}
	var asOf *time.Time
	var asOfCanonical string
	if asOfRaw != "" {
		if !organizationUsageRFC3339Pattern.MatchString(asOfRaw) {
			return normalizedOrganizationUsageQuery{}, organizationUsageValidation("as_of", "invalid as_of, use RFC3339")
		}
		parsed, err := time.Parse(time.RFC3339Nano, asOfRaw)
		if err != nil {
			return normalizedOrganizationUsageQuery{}, organizationUsageValidation("as_of", "invalid as_of, use RFC3339")
		}
		utc := parsed.UTC()
		asOf = &utc
		asOfCanonical = utc.Format(time.RFC3339Nano)
	}

	organization = strings.ToLower(strings.TrimSpace(organization))
	if organization == "" {
		organization = OrganizationAll
	}
	if organization != OrganizationAll && organization != OrganizationXunyou && organization != OrganizationWsdashi && organization != OrganizationOther {
		return normalizedOrganizationUsageQuery{}, organizationUsageValidation("organization", "invalid organization")
	}
	if page == 0 {
		page = 1
	}
	if page < 1 {
		return normalizedOrganizationUsageQuery{}, organizationUsageValidation("page", "page must be greater than zero")
	}
	if pageSize == 0 {
		pageSize = 20
	}
	if pageSize < 1 || pageSize > 1000 {
		return normalizedOrganizationUsageQuery{}, organizationUsageValidation("page_size", "page_size must be between 1 and 1000")
	}

	return normalizedOrganizationUsageQuery{
		start: start, end: end, organization: organization, q: strings.TrimSpace(q), page: page, pageSize: pageSize,
		asOf: asOf, asOfCanonical: asOfCanonical,
	}, nil
}

func (q normalizedOrganizationUsageQuery) repositoryRange() (time.Time, time.Time) {
	start := q.start.UTC()
	end := q.end.AddDate(0, 0, 1).UTC()
	if q.asOf != nil && q.asOf.Before(end) {
		end = *q.asOf
	}
	if end.Before(start) {
		end = start
	}
	return start, end
}

func (q *normalizedOrganizationUsageQuery) clampAsOfToServerNow(now time.Time) {
	if q.asOf == nil {
		return
	}
	signed := q.asOf.UTC()
	now = now.UTC()
	if now.Before(signed) {
		signed = now
	}
	q.asOf = &signed
	q.asOfCanonical = signed.Format(time.RFC3339Nano)
}

func organizationUsageCalendarDays(start, end time.Time) int {
	days := 1
	for day := start; day.Before(end); {
		day = day.AddDate(0, 0, 1)
		days++
		if days > 366 {
			return days
		}
	}
	return days
}

func validOrganizationUsageSortBy(sortBy string) bool {
	switch sortBy {
	case "email", "requests", "input_tokens", "output_tokens", "cache_creation_tokens", "cache_read_tokens", "total_tokens", "actual_cost", "peak_day_tokens", "peak_week_tokens", "peak_month_tokens":
		return true
	default:
		return false
	}
}

func organizationUsagePagination(total int64, page, pageSize int) OrganizationUsagePagination {
	pages := int((total + int64(pageSize) - 1) / int64(pageSize))
	if pages < 1 {
		pages = 1
	}
	return OrganizationUsagePagination{Total: total, Page: page, PageSize: pageSize, Pages: pages}
}

func organizationUsageValidation(field, message string) error {
	return &OrganizationUsageValidationError{Field: field, Message: fmt.Sprintf("%s: %s", field, message)}
}
