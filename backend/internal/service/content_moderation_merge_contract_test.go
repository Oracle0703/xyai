package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRiskControlAllowlistSurvivesLocalRuntimeUpdates(t *testing.T) {
	svc := &ContentModerationService{}
	pr := DefaultPromptRiskConfig()
	svc.runtimeSnapshot.Store(&contentModerationRuntimeSnapshot{
		allowlistedUsers: map[int64]struct{}{12: {}},
		config:           defaultContentModerationConfig(),
		promptRiskConfig: &pr,
	})
	raw, err := json.Marshal(pr)
	require.NoError(t, err)
	svc.replaceRuntimePromptRiskConfig(&pr, raw)
	require.Contains(t, svc.runtimeSnapshot.Load().allowlistedUsers, int64(12))
	svc.replaceRuntimeRiskControlEnabled(true)
	require.True(t, svc.runtimeSnapshot.Load().riskControlEnabled)
	require.Contains(t, svc.runtimeSnapshot.Load().allowlistedUsers, int64(12))
}
