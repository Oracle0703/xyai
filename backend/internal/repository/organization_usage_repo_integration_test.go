//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestOrganizationUsageRepositoryIntegration_SummaryContracts(t *testing.T) {
	ctx := context.Background()
	prefix := organizationUsageIntegrationPrefix("summary")
	account := organizationUsageIntegrationAccount(t, prefix)
	cleanupOrganizationUsageIntegrationData(t, prefix)

	xunyou, xunyouKey := organizationUsageIntegrationUser(t, prefix+"alpha@XUNYOU.COM", service.StatusActive)
	wsdashi, wsdashiKey := organizationUsageIntegrationUser(t, prefix+"beta@wsdashi.com", service.StatusActive)
	subdomain, subdomainKey := organizationUsageIntegrationUser(t, prefix+"gamma@team.xunyou.com", service.StatusActive)
	zero, _ := organizationUsageIntegrationUser(t, prefix+"zero@example.com", service.StatusActive)
	organizationUsageIntegrationUser(t, prefix+"disabled@xunyou.com", service.StatusDisabled)
	deleted, _ := organizationUsageIntegrationUser(t, prefix+"deleted@wsdashi.com", service.StatusActive)
	_, err := integrationDB.ExecContext(ctx, `UPDATE users SET deleted_at = NOW() WHERE id = $1`, deleted.ID)
	require.NoError(t, err)

	base := time.Date(2026, 1, 2, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*60*60))
	insertOrganizationUsageIntegrationLog(t, xunyou.ID, xunyouKey.ID, account.ID, 10, 20, 30, 40, 1.25, base.UTC())
	insertOrganizationUsageIntegrationLog(t, xunyou.ID, xunyouKey.ID, account.ID, 1, 2, 3, 4, 0.25, base.Add(2*time.Hour).UTC())
	insertOrganizationUsageIntegrationLog(t, wsdashi.ID, wsdashiKey.ID, account.ID, 5, 6, 7, 8, 0.5, base.AddDate(0, 0, 1).UTC())
	insertOrganizationUsageIntegrationLog(t, subdomain.ID, subdomainKey.ID, account.ID, 2, 3, 4, 5, 0.75, base.AddDate(0, 0, 2).UTC())

	repo := NewOrganizationUsageRepository(integrationDB)
	result, err := repo.Summary(ctx, organizationUsageSummaryIntegrationParams(prefix, "2026-01-01", "2026-01-10"))
	require.NoError(t, err)
	require.Equal(t, int64(4), result.Total)
	require.Equal(t, int64(4), result.Overview.ActiveUsers)
	require.Equal(t, int64(3), result.Overview.UsedUsers)
	require.Equal(t, int64(4), result.Overview.Requests)
	require.Equal(t, int64(150), result.Overview.TotalTokens)
	require.InDelta(t, 2.75, result.Overview.ActualCost, 0.0000001)

	items := make(map[int64]service.OrganizationUsageSummaryItem, len(result.Items))
	for _, item := range result.Items {
		items[item.UserID] = item
	}
	require.Equal(t, service.OrganizationXunyou, items[xunyou.ID].Organization)
	require.Equal(t, int64(110), items[xunyou.ID].TotalTokens)
	require.Equal(t, int64(2), items[xunyou.ID].Requests)
	require.InDelta(t, 1.5, items[xunyou.ID].ActualCost, 0.0000001)
	require.Equal(t, service.OrganizationWsdashi, items[wsdashi.ID].Organization)
	require.Equal(t, service.OrganizationOther, items[subdomain.ID].Organization)
	require.Equal(t, service.OrganizationOther, items[zero.ID].Organization)
	require.Zero(t, items[zero.ID].TotalTokens)
	require.Nil(t, items[zero.ID].PeakDay)
	require.Nil(t, items[zero.ID].PeakWeek)
	require.Nil(t, items[zero.ID].PeakMonth)

	organizations := make(map[string]service.OrganizationUsageOrganization, len(result.Organizations))
	for _, organization := range result.Organizations {
		organizations[organization.Organization] = organization
	}
	require.Len(t, organizations, 3)
	require.Equal(t, int64(1), organizations[service.OrganizationXunyou].ActiveUsers)
	require.Equal(t, int64(1), organizations[service.OrganizationWsdashi].ActiveUsers)
	require.Equal(t, int64(2), organizations[service.OrganizationOther].ActiveUsers)
	require.Equal(t, int64(1), organizations[service.OrganizationOther].UsedUsers)
}

func TestOrganizationUsageRepositoryIntegration_SearchTreatsWildcardsLiterally(t *testing.T) {
	prefix := organizationUsageIntegrationPrefix("search")
	cleanupOrganizationUsageIntegrationData(t, prefix)

	literal, _ := organizationUsageIntegrationUser(t, prefix+`needle_100%\ops@xunyou.com`, service.StatusActive)
	organizationUsageIntegrationUser(t, prefix+"needlex100ZZops@xunyou.com", service.StatusActive)

	params := organizationUsageSummaryIntegrationParams(prefix+`needle_100%\ops`, "2026-01-01", "2026-01-02")
	result, err := NewOrganizationUsageRepository(integrationDB).Summary(context.Background(), params)
	require.NoError(t, err)
	require.Equal(t, int64(1), result.Total)
	require.Len(t, result.Items, 1)
	require.Equal(t, literal.ID, result.Items[0].UserID)
}

func TestOrganizationUsageRepositoryIntegration_BeijingBoundariesAndPartialPeriods(t *testing.T) {
	prefix := organizationUsageIntegrationPrefix("periods")
	account := organizationUsageIntegrationAccount(t, prefix)
	cleanupOrganizationUsageIntegrationData(t, prefix)
	user, apiKey := organizationUsageIntegrationUser(t, prefix+"periods@xunyou.com", service.StatusActive)

	// 2026-01-30 00:00:00 through 2026-02-03 23:59:59 in Asia/Shanghai are included.
	insertOrganizationUsageIntegrationLog(t, user.ID, apiKey.ID, account.ID, 1, 0, 0, 0, 0.1, time.Date(2026, 1, 29, 15, 59, 59, 0, time.UTC))
	insertOrganizationUsageIntegrationLog(t, user.ID, apiKey.ID, account.ID, 10, 0, 0, 0, 0.2, time.Date(2026, 1, 29, 16, 0, 0, 0, time.UTC))
	insertOrganizationUsageIntegrationLog(t, user.ID, apiKey.ID, account.ID, 20, 0, 0, 0, 0.3, time.Date(2026, 2, 1, 15, 59, 59, 0, time.UTC))
	insertOrganizationUsageIntegrationLog(t, user.ID, apiKey.ID, account.ID, 30, 0, 0, 0, 0.4, time.Date(2026, 2, 1, 16, 0, 0, 0, time.UTC))
	insertOrganizationUsageIntegrationLog(t, user.ID, apiKey.ID, account.ID, 40, 0, 0, 0, 0.5, time.Date(2026, 2, 3, 15, 59, 59, 0, time.UTC))
	insertOrganizationUsageIntegrationLog(t, user.ID, apiKey.ID, account.ID, 2, 0, 0, 0, 0.6, time.Date(2026, 2, 3, 16, 0, 0, 0, time.UTC))

	repo := NewOrganizationUsageRepository(integrationDB)
	day, err := repo.Periods(context.Background(), organizationUsagePeriodsIntegrationParams(prefix, "2026-01-30", "2026-02-03", service.OrganizationUsageGranularityDay))
	require.NoError(t, err)
	require.Equal(t, int64(4), day.Total)
	require.Len(t, day.Items, 4)

	week, err := repo.Periods(context.Background(), organizationUsagePeriodsIntegrationParams(prefix, "2026-01-30", "2026-02-03", service.OrganizationUsageGranularityWeek))
	require.NoError(t, err)
	require.Equal(t, int64(2), week.Total)
	require.Len(t, week.Items, 2)
	require.Equal(t, "2026-02-02", week.Items[0].PeriodStart)
	require.Equal(t, "2026-02-03", week.Items[0].PeriodEnd)
	require.True(t, week.Items[0].Partial)
	require.Equal(t, int64(70), week.Items[0].TotalTokens)
	require.Equal(t, "2026-01-30", week.Items[1].PeriodStart)
	require.Equal(t, "2026-02-01", week.Items[1].PeriodEnd)
	require.True(t, week.Items[1].Partial)
	require.Equal(t, int64(30), week.Items[1].TotalTokens)

	month, err := repo.Periods(context.Background(), organizationUsagePeriodsIntegrationParams(prefix, "2026-01-30", "2026-02-03", service.OrganizationUsageGranularityMonth))
	require.NoError(t, err)
	require.Equal(t, int64(2), month.Total)
	require.Len(t, month.Items, 2)
	require.Equal(t, "2026-02-01", month.Items[0].PeriodStart)
	require.Equal(t, "2026-02-03", month.Items[0].PeriodEnd)
	require.True(t, month.Items[0].Partial)
	require.Equal(t, int64(90), month.Items[0].TotalTokens)
	require.Equal(t, "2026-01-30", month.Items[1].PeriodStart)
	require.Equal(t, "2026-01-31", month.Items[1].PeriodEnd)
	require.True(t, month.Items[1].Partial)
	require.Equal(t, int64(10), month.Items[1].TotalTokens)
}

func TestOrganizationUsageRepositoryIntegration_ChampionTieUsesLowestUserID(t *testing.T) {
	prefix := organizationUsageIntegrationPrefix("tie")
	account := organizationUsageIntegrationAccount(t, prefix)
	cleanupOrganizationUsageIntegrationData(t, prefix)
	lowerID, lowerKey := organizationUsageIntegrationUser(t, prefix+"lower@xunyou.com", service.StatusActive)
	higherID, higherKey := organizationUsageIntegrationUser(t, prefix+"higher@wsdashi.com", service.StatusActive)
	require.Less(t, lowerID.ID, higherID.ID)

	createdAt := time.Date(2026, 2, 2, 4, 0, 0, 0, time.UTC)
	insertOrganizationUsageIntegrationLog(t, lowerID.ID, lowerKey.ID, account.ID, 25, 25, 25, 25, 1, createdAt)
	insertOrganizationUsageIntegrationLog(t, higherID.ID, higherKey.ID, account.ID, 25, 25, 25, 25, 1, createdAt)

	result, err := NewOrganizationUsageRepository(integrationDB).Summary(
		context.Background(),
		organizationUsageSummaryIntegrationParams(prefix, "2026-02-02", "2026-02-02"),
	)
	require.NoError(t, err)
	require.NotNil(t, result.Champions.Day)
	require.NotNil(t, result.Champions.Week)
	require.NotNil(t, result.Champions.Month)
	require.Equal(t, lowerID.ID, result.Champions.Day.UserID)
	require.Equal(t, lowerID.ID, result.Champions.Week.UserID)
	require.Equal(t, lowerID.ID, result.Champions.Month.UserID)
}

func organizationUsageIntegrationPrefix(label string) string {
	return fmt.Sprintf("org-usage-%s-%d-", label, time.Now().UnixNano())
}

func cleanupOrganizationUsageIntegrationData(t *testing.T, prefix string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		_, err := integrationDB.ExecContext(ctx, `DELETE FROM users WHERE email LIKE $1`, prefix+"%")
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, `DELETE FROM accounts WHERE name LIKE $1`, prefix+"%")
		require.NoError(t, err)
	})
}

func organizationUsageIntegrationAccount(t *testing.T, prefix string) *service.Account {
	t.Helper()
	return mustCreateAccount(t, integrationEntClient, &service.Account{Name: prefix + "account"})
}

func organizationUsageIntegrationUser(t *testing.T, email, status string) (*service.User, *service.APIKey) {
	t.Helper()
	user := mustCreateUser(t, integrationEntClient, &service.User{Email: email, Status: status})
	apiKey := mustCreateApiKey(t, integrationEntClient, &service.APIKey{
		UserID: user.ID,
		Key:    "sk-" + uuid.NewString(),
		Name:   "organization-usage-integration",
	})
	return user, apiKey
}

func insertOrganizationUsageIntegrationLog(
	t *testing.T,
	userID, apiKeyID, accountID int64,
	input, output, cacheCreation, cacheRead int,
	actualCost float64,
	createdAt time.Time,
) {
	t.Helper()
	_, err := integrationDB.ExecContext(context.Background(), `
INSERT INTO usage_logs (
    user_id, api_key_id, account_id, request_id, model,
    input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
    actual_cost, created_at
) VALUES ($1, $2, $3, $4, 'organization-usage-integration', $5, $6, $7, $8, $9, $10)`,
		userID, apiKeyID, accountID, uuid.NewString(), input, output, cacheCreation, cacheRead, actualCost, createdAt,
	)
	require.NoError(t, err)
}

func organizationUsageSummaryIntegrationParams(prefix, startDate, endDate string) service.OrganizationUsageSummaryRepositoryParams {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	start, _ := time.ParseInLocation("2006-01-02", startDate, location)
	end, _ := time.ParseInLocation("2006-01-02", endDate, location)
	return service.OrganizationUsageSummaryRepositoryParams{
		StartTime:    start.UTC(),
		EndTime:      end.AddDate(0, 0, 1).UTC(),
		StartDate:    start,
		EndDate:      end,
		Organization: service.OrganizationAll,
		Q:            prefix,
		Page:         1,
		PageSize:     100,
		SortBy:       "total_tokens",
		SortOrder:    "desc",
	}
}

func organizationUsagePeriodsIntegrationParams(prefix, startDate, endDate, granularity string) service.OrganizationUsagePeriodsRepositoryParams {
	summary := organizationUsageSummaryIntegrationParams(prefix, startDate, endDate)
	return service.OrganizationUsagePeriodsRepositoryParams{
		StartTime:    summary.StartTime,
		EndTime:      summary.EndTime,
		StartDate:    summary.StartDate,
		EndDate:      summary.EndDate,
		Organization: summary.Organization,
		Q:            summary.Q,
		Page:         1,
		PageSize:     100,
		Granularity:  granularity,
	}
}
