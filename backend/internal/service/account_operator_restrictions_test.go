package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubAdminUsageReadsDoNotClearRecoverableErrors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		restricted bool
		batch      bool
		wantClears int
	}{
		{name: "sub admin single read", restricted: true, wantClears: 0},
		{name: "sub admin batch read", restricted: true, batch: true, wantClears: 0},
		{name: "full admin single read keeps existing recovery", wantClears: 1},
		{name: "full admin batch read keeps existing recovery", batch: true, wantClears: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &usageErrorAccountRepo{stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{{
				ID:           9,
				Platform:     PlatformGemini,
				Type:         AccountTypeOAuth,
				Status:       StatusError,
				ErrorMessage: "Token refresh failed: invalid_client",
				Credentials:  map[string]any{"oauth_type": "code_assist", "project_id": "project-a"},
			}}}}
			svc := &AccountUsageService{accountRepo: repo, usageLogRepo: &usageBatchLogRepoStub{}, geminiQuotaService: NewGeminiQuotaService(nil, nil), cache: NewUsageCache()}
			ctx := context.Background()
			if tc.restricted {
				ctx = WithSubAdminAccountRestrictions(ctx)
			}

			var err error
			if tc.batch {
				_, _, err = svc.GetUsageBatch(ctx, []int64{9}, false)
			} else {
				_, err = svc.GetUsage(ctx, 9)
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantClears, repo.clearCalls)
		})
	}
}
