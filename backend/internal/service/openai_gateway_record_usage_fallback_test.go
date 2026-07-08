package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayServiceRecordUsage_MissingPricingImageRequestKeepsImageBillingMode(t *testing.T) {
	groupID := int64(1008)
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	userRepo := &openAIRecordUsageUserRepoStub{}
	subRepo := &openAIRecordUsageSubRepoStub{}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, userRepo, subRepo, nil)
	svc.resolver = newOpenAIEmptyTokenChannelPricingResolverForTest(groupID, "pricing-missing-image-model-for-test")

	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{
			RequestID:  "resp_missing_pricing_image",
			Usage:      OpenAIUsage{InputTokens: 1200, OutputTokens: 300, ImageOutputTokens: 50},
			Model:      "pricing-missing-image-model-for-test",
			Duration:   time.Second,
			ImageCount: 1,
			ImageSize:  "2K",
		},
		APIKey:  &APIKey{ID: 1008, Quota: 100, GroupID: i64p(groupID), Group: &Group{ID: groupID, RateMultiplier: 1}},
		User:    &User{ID: 2008},
		Account: &Account{ID: 3008, Type: AccountTypeAPIKey},
	})

	require.NoError(t, err)
	require.Equal(t, 1, usageRepo.calls)
	require.NotNil(t, usageRepo.lastLog)
	require.Equal(t, 1, usageRepo.lastLog.ImageCount)
	require.Zero(t, usageRepo.lastLog.TotalCost)
	require.Zero(t, usageRepo.lastLog.ActualCost)
	require.NotNil(t, usageRepo.lastLog.BillingMode)
	require.Equal(t, string(BillingModeImage), *usageRepo.lastLog.BillingMode)
	require.Equal(t, 1.0, usageRepo.lastLog.RateMultiplier)
}

func newOpenAIEmptyTokenChannelPricingResolverForTest(groupID int64, model string) *ModelPricingResolver {
	cache := newEmptyChannelCache()
	cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: model}] = &ChannelModelPricing{
		BillingMode: BillingModeToken,
	}
	cache.channelByGroupID[groupID] = &Channel{ID: groupID, Status: StatusActive}
	cache.groupPlatform[groupID] = ""
	cache.loadedAt = time.Now()
	cs := &ChannelService{}
	cs.cache.Store(cache)
	return NewModelPricingResolver(cs, NewBillingService(&config.Config{}, nil))
}
