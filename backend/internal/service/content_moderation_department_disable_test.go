package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// Locked acceptance test for docs/features/department-member-management-design-cn.md §3:
// an auto-ban must be written even when a department manager already disabled the user,
// so the repository records the system source and the manager can no longer re-enable.
func TestContentModerationAutoBanReappliesDisableForAlreadyDisabledUser(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.BanThreshold = 2
	cfg.ViolationWindowHours = 24

	userID := int64(1002)
	repo := &contentModerationTestRepo{}
	require.NoError(t, repo.CreateLog(context.Background(), newContentModerationFlaggedLog(userID)))
	userRepo := &contentModerationTestUserRepo{user: &User{ID: userID, Role: RoleUser, Status: StatusDisabled}}
	invalidator := &contentModerationTestAuthCacheInvalidator{}
	svc := NewContentModerationService(nil, repo, nil, nil, userRepo, nil, invalidator, nil)

	svc.persistContentModerationLog(context.Background(), cfg, newContentModerationFlaggedLog(userID), "", false, true)

	logs := requireContentModerationLogCount(t, repo, 2)
	require.True(t, logs[1].AutoBanned)
	require.Len(t, userRepo.updated, 1, "the explicit system disable must reach the repository")
	require.Equal(t, StatusDisabled, userRepo.updated[0].Status)
}
