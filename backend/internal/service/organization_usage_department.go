package service

import "strings"

type OrganizationUsageDepartmentFilters struct {
	DepartmentID string `json:"department_id"`
	Platform     string `json:"platform"`
	ScopeVersion string `json:"scope_version,omitempty"`
}

type OrganizationUsageAppliedFilters struct {
	Organization string `json:"organization"`
	DepartmentID string `json:"department_id"`
	Platform     string `json:"platform"`
	Q            string `json:"q"`
	Attribution  string `json:"attribution"`
}

type OrganizationUsageDepartment struct {
	DepartmentID   *int64 `json:"department_id"`
	DepartmentName string `json:"department_name"`
	Organization   string `json:"organization"`
	OrganizationUsageOverview
}

type OrganizationUsagePlatform struct {
	Platform  string `json:"platform"`
	UsedUsers int64  `json:"used_users"`
	OrganizationUsageMetrics
}

func (f OrganizationUsageDepartmentFilters) Normalize(organization string) (OrganizationUsageDepartmentFilters, error) {
	_, department, err := NormalizeDepartmentFilter(organization, f.DepartmentID)
	if err != nil {
		return f, err
	}
	f.DepartmentID = department
	f.Platform = strings.TrimSpace(f.Platform)
	if f.Platform == "" {
		f.Platform = "all"
	}
	switch f.Platform {
	case "all", "unknown", PlatformOpenAI, PlatformAnthropic, PlatformGemini, PlatformAntigravity, PlatformGrok, PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo, PlatformKiro:
	default:
		return f, ErrDepartmentInvalid
	}
	return f, nil
}

func (m *OrganizationUsageMetrics) Add(other OrganizationUsageMetrics) {
	m.Requests += other.Requests
	m.InputTokens += other.InputTokens
	m.OutputTokens += other.OutputTokens
	m.CacheCreationTokens += other.CacheCreationTokens
	m.CacheReadTokens += other.CacheReadTokens
	m.TotalTokens += other.TotalTokens
	m.ActualCost += other.ActualCost
}
