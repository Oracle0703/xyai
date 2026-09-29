package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminAssignSubscriptionRejectsInactiveGroup(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription, Status: StatusDisabled}}
	subRepo := newSubscriptionUserSubRepoStub()
	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)

	_, err := svc.AssignSubscription(context.Background(), &AssignSubscriptionInput{UserID: 1001, GroupID: 1, ValidityDays: 30})
	require.ErrorIs(t, err, ErrGroupNotActive)

	result, err := svc.BulkAssignSubscription(context.Background(), &BulkAssignSubscriptionInput{UserIDs: []int64{1001}, GroupID: 1, ValidityDays: 30})
	require.NoError(t, err)
	require.Equal(t, 0, result.SuccessCount)
	require.Equal(t, 1, result.FailedCount)
	require.Zero(t, subRepo.createCalls)
}

// Redeem codes, payment fulfillment and default subscriptions share
// AssignOrExtendSubscription; a paid order must still be fulfilled when the
// group was disabled after checkout.
func TestAssignOrExtendSubscriptionStillFulfillsInactiveGroup(t *testing.T) {
	groupRepo := &subscriptionGroupRepoStub{group: &Group{ID: 1, SubscriptionType: SubscriptionTypeSubscription, Status: StatusDisabled}}
	subRepo := newSubscriptionUserSubRepoStub()
	svc := NewSubscriptionService(groupRepo, subRepo, nil, nil, nil)

	sub, renewed, err := svc.AssignOrExtendSubscription(context.Background(), &AssignSubscriptionInput{UserID: 1001, GroupID: 1, ValidityDays: 30})
	require.NoError(t, err)
	require.False(t, renewed)
	require.NotNil(t, sub)
	require.Equal(t, 1, subRepo.createCalls)
}

type progressErrorSubRepo struct {
	userSubRepoNoop
	err error
}

func (r progressErrorSubRepo) GetByID(context.Context, int64) (*UserSubscription, error) {
	return nil, r.err
}

func TestGetSubscriptionProgressPreservesRepositoryErrors(t *testing.T) {
	dbErr := errors.New("connection reset")
	for _, want := range []error{ErrDepartmentScopeDenied, ErrSubscriptionNotFound, dbErr} {
		svc := NewSubscriptionService(groupRepoNoop{}, progressErrorSubRepo{err: want}, nil, nil, nil)
		_, err := svc.GetSubscriptionProgress(context.Background(), 1)
		require.ErrorIs(t, err, want)
	}
}
