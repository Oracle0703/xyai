package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeOrganizationUsageRollupRepo struct {
	calls          []string
	closable       int
	dirty          int
	reconciled     int
	reconcileEvery time.Duration
	closeErr       error
	reconcileDays  []string
}

func (f *fakeOrganizationUsageRollupRepo) InitializeOrganizationUsageRollups(context.Context) (bool, error) {
	f.calls = append(f.calls, "init")
	return false, nil
}

func (f *fakeOrganizationUsageRollupRepo) CloseNextOrganizationUsageRollupDay(context.Context) (bool, error) {
	f.calls = append(f.calls, "close")
	if f.closeErr != nil {
		return false, f.closeErr
	}
	if f.closable == 0 {
		return false, nil
	}
	f.closable--
	return true, nil
}

func (f *fakeOrganizationUsageRollupRepo) RebuildNextDirtyOrganizationUsageRollupDay(context.Context) (bool, error) {
	f.calls = append(f.calls, "dirty")
	if f.dirty == 0 {
		return false, nil
	}
	f.dirty--
	return true, nil
}

func (f *fakeOrganizationUsageRollupRepo) ReconcileOrganizationUsageRollups(_ context.Context, days int, every time.Duration) ([]string, error) {
	f.calls = append(f.calls, "reconcile")
	f.reconciled = days
	f.reconcileEvery = every
	return f.reconcileDays, nil
}

func (f *fakeOrganizationUsageRollupRepo) AnalyzeOrganizationUsageRollups(context.Context) error {
	f.calls = append(f.calls, "analyze")
	return nil
}

func TestOrganizationUsageRollupService_AnalyzesAfterBackfill(t *testing.T) {
	repo := &fakeOrganizationUsageRollupRepo{closable: organizationUsageRollupAnalyzeAfterDays}
	svc := NewOrganizationUsageRollupService(repo, nil, nil)
	require.NoError(t, svc.RunOnce(context.Background()))
	require.Contains(t, repo.calls, "analyze")
}

func TestOrganizationUsageRollupService_RunOnceDrainsWorkThenOffersReconcile(t *testing.T) {
	repo := &fakeOrganizationUsageRollupRepo{closable: 2, dirty: 1}
	svc := NewOrganizationUsageRollupService(repo, nil, nil)

	require.NoError(t, svc.RunOnce(context.Background()))
	require.Equal(t, []string{"init", "close", "close", "close", "dirty", "dirty", "reconcile"}, repo.calls)
	require.Equal(t, organizationUsageRollupReconcileDays, repo.reconciled)
	// The repository claims the hourly cadence on the shared state row.
	require.Equal(t, organizationUsageRollupReconcileInterval, repo.reconcileEvery)
}

func TestOrganizationUsageRollupService_StopsAtBudgetAndReportsErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &fakeOrganizationUsageRollupRepo{closable: 5}
	svc := NewOrganizationUsageRollupService(repo, nil, nil)
	require.NoError(t, svc.RunOnce(ctx))
	require.Equal(t, []string{"init"}, repo.calls, "an exhausted budget stops before any day is processed")

	failing := &fakeOrganizationUsageRollupRepo{closeErr: errors.New("boom")}
	require.ErrorContains(t, NewOrganizationUsageRollupService(failing, nil, nil).RunOnce(context.Background()), "close day: boom")
}

func TestOrganizationUsageRollupService_StartStopIsIdempotent(t *testing.T) {
	svc := NewOrganizationUsageRollupService(&fakeOrganizationUsageRollupRepo{}, nil, nil)
	svc.Start()
	svc.Start()
	svc.Stop()
	svc.Stop()
	var nilService *OrganizationUsageRollupService
	nilService.Start()
	nilService.Stop()
}
