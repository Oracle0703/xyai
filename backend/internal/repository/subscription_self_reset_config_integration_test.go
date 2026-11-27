//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// Hooks sit at real repository call boundaries. All other queries and commit
// or rollback still run on PostgreSQL and the production transaction code.
type selfResetConfigProbe struct {
	service.SubscriptionSelfResetRepository
	readPolicyErr     error
	beforeWritePolicy func(context.Context) error
	beforeListGroups  func(context.Context) error
}

func (p selfResetConfigProbe) ReadPolicy(ctx context.Context) (string, error) {
	if p.readPolicyErr != nil {
		return "", p.readPolicyErr
	}
	return p.SubscriptionSelfResetRepository.ReadPolicy(ctx)
}

func (p selfResetConfigProbe) WritePolicy(ctx context.Context, policy string) error {
	if p.beforeWritePolicy != nil {
		if err := p.beforeWritePolicy(ctx); err != nil {
			return err
		}
	}
	return p.SubscriptionSelfResetRepository.WritePolicy(ctx, policy)
}

func (p selfResetConfigProbe) ListGroups(ctx context.Context) (map[string][]service.SubscriptionSelfResetGroup, error) {
	if p.beforeListGroups != nil {
		if err := p.beforeListGroups(ctx); err != nil {
			return nil, err
		}
	}
	return p.SubscriptionSelfResetRepository.ListGroups(ctx)
}

func TestSubscriptionSelfResetConfig(t *testing.T) {
	f := newSelfResetGroupFixture(t)
	prior, err := f.repo.ReadPolicy(f.ctx)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, writeSelfResetFixturePolicy(context.Background(), prior)) })
	const org = service.OrganizationXunyou
	g := f.group("config", org)
	admin := json.RawMessage(`{"rollout":"admin","daily_limit_by_organization":{"xunyou":3,"wsdashi":2,"other":0}}`)
	all := json.RawMessage(`{"rollout":"all","daily_limit_by_organization":{"xunyou":7,"wsdashi":2,"other":0}}`)
	check := selfResetGroupChanges{org: {Enable: []int64{g}}}
	uncheck := selfResetGroupChanges{org: {Disable: []int64{g}}}
	save := func(policy json.RawMessage, groups selfResetGroupChanges) int64 {
		t.Helper()
		start := f.revision()
		saved, err := f.svc.SaveConfig(f.ctx, service.SubscriptionSelfResetConfigChange{ExpectedRevision: start, Policy: policy, Groups: groups})
		require.NoError(t, err)
		require.Equal(t, start+1, saved)
		return saved
	}

	t.Run("partial writes roll back on policy storage failure", func(t *testing.T) {
		rev := save(admin, uncheck)
		probe := selfResetConfigProbe{SubscriptionSelfResetRepository: f.repo, beforeWritePolicy: func(context.Context) error {
			require.Empty(t, f.ticks(g), "uncommitted allowlist enable must be invisible on another connection")
			return errors.New("injected policy write failure")
		}}
		svc := service.NewSubscriptionSelfResetService(probe, f.subs)
		_, err := svc.SaveConfig(f.ctx, service.SubscriptionSelfResetConfigChange{ExpectedRevision: rev, Policy: all, Groups: check})
		require.Error(t, err)
		config, err := f.svc.Config(f.ctx)
		require.NoError(t, err)
		require.Equal(t, rev, config.Revision)
		require.Equal(t, "admin", config.Policy.Rollout)
		require.False(t, f.candidates(config.Groups, org)[g])
	})

	t.Run("lost response after commit and fence before delayed save", func(t *testing.T) {
		rev := save(admin, uncheck)
		change := service.SubscriptionSelfResetConfigChange{ExpectedRevision: rev, Groups: check}
		_, err := f.svc.SaveConfig(f.ctx, change) // The handler's reply never arrives.
		require.NoError(t, err)
		_, err = f.svc.SaveConfig(f.ctx, service.SubscriptionSelfResetConfigChange{ExpectedRevision: rev})
		require.ErrorIs(t, err, service.ErrSelfResetRevisionConflict)
		fresh, err := f.svc.Config(f.ctx)
		require.NoError(t, err)
		require.True(t, f.candidates(fresh.Groups, org)[g])
		rev, err = f.svc.SaveConfig(f.ctx, service.SubscriptionSelfResetConfigChange{ExpectedRevision: fresh.Revision, Groups: uncheck})
		require.NoError(t, err)
		// This time the confirmation reaches the DB before the original request.
		_, err = f.svc.SaveConfig(f.ctx, service.SubscriptionSelfResetConfigChange{ExpectedRevision: rev})
		require.NoError(t, err)
		change.ExpectedRevision = rev
		_, err = f.svc.SaveConfig(f.ctx, change)
		require.ErrorIs(t, err, service.ErrSelfResetRevisionConflict)
		require.Empty(t, f.ticks(g))
	})

	t.Run("emergency close and stale form", func(t *testing.T) {
		rev := save(all, check)
		closed, err := f.svc.CloseConfig(f.ctx)
		require.NoError(t, err)
		require.Equal(t, rev+1, closed)
		_, err = f.svc.SaveConfig(f.ctx, service.SubscriptionSelfResetConfigChange{ExpectedRevision: rev, Policy: admin, Groups: uncheck})
		require.ErrorIs(t, err, service.ErrSelfResetRevisionConflict)
		fresh, err := f.svc.Config(f.ctx)
		require.NoError(t, err)
		require.Equal(t, "off", fresh.Policy.Rollout)
		require.Equal(t, 7, fresh.Policy.DailyLimitByOrganization[org])
		require.True(t, f.candidates(fresh.Groups, org)[g])
		again, err := f.svc.CloseConfig(f.ctx)
		require.NoError(t, err)
		require.Equal(t, closed+1, again, "already-off closes must invalidate queued opens too")
	})

	t.Run("scope moves during editing", func(t *testing.T) {
		rev := save(admin, uncheck)
		f.setScopes([]int64{g}, service.OrganizationWsdashi)
		t.Cleanup(func() { f.setScopes([]int64{g}, org) })
		_, err := f.svc.SaveConfig(f.ctx, service.SubscriptionSelfResetConfigChange{ExpectedRevision: rev, Policy: all, Groups: check})
		require.ErrorIs(t, err, service.ErrSelfResetGroupConflict)
		fresh, err := f.svc.Config(f.ctx)
		require.NoError(t, err)
		require.Equal(t, rev, fresh.Revision)
		require.Equal(t, "admin", fresh.Policy.Rollout)
		require.NotContains(t, f.candidates(fresh.Groups, org), g)
		require.Empty(t, f.ticks(g))
	})

	t.Run("config reads one snapshot during another save", func(t *testing.T) {
		rev := save(admin, uncheck)
		probe := selfResetConfigProbe{SubscriptionSelfResetRepository: f.repo, beforeListGroups: func(context.Context) error {
			_, err := f.svc.SaveConfig(f.ctx, service.SubscriptionSelfResetConfigChange{ExpectedRevision: rev, Policy: all, Groups: check})
			return err
		}}
		config, err := service.NewSubscriptionSelfResetService(probe, f.subs).Config(f.ctx)
		require.NoError(t, err)
		require.Equal(t, rev, config.Revision)
		require.Equal(t, "admin", config.Policy.Rollout)
		require.Equal(t, map[int64]bool{g: false}, f.candidates(config.Groups, org))
		require.Equal(t, org, f.ticks(g))
	})

	t.Run("unavailable candidates and damaged policy", func(t *testing.T) {
		rev := save(all, uncheck)
		probe := selfResetConfigProbe{SubscriptionSelfResetRepository: f.repo, beforeListGroups: func(context.Context) error {
			return errors.New("candidates unavailable")
		}}
		svc := service.NewSubscriptionSelfResetService(probe, f.subs)
		fresh, err := svc.Config(f.ctx)
		require.NoError(t, err)
		require.Equal(t, rev, fresh.Revision)
		require.Equal(t, "all", fresh.Policy.Rollout)
		require.Nil(t, fresh.Groups)
		_, err = svc.CloseConfig(f.ctx)
		require.NoError(t, err)
		rev = f.revision()
		probe.readPolicyErr = errors.New("policy storage unavailable")
		_, err = service.NewSubscriptionSelfResetService(probe, f.subs).CloseConfig(f.ctx)
		require.Equal(t, 503, infraerrors.Code(err))
		require.Equal(t, rev, f.revision())
		require.NoError(t, writeSelfResetFixturePolicy(f.ctx, "{"))
		fresh, err = f.svc.Config(f.ctx)
		require.NoError(t, err)
		require.Nil(t, fresh.Policy)
		require.NotNil(t, fresh.Groups)
		_, err = f.svc.CloseConfig(f.ctx)
		require.ErrorIs(t, err, service.ErrSelfResetClosePolicyInvalid)
		require.Equal(t, 409, infraerrors.Code(err))
		require.Equal(t, fresh.Revision, f.revision())
		_, err = integrationDB.ExecContext(f.ctx, "DELETE FROM settings WHERE key=$1", service.SubscriptionSelfResetPolicyKey)
		require.NoError(t, err)
		_, err = f.svc.CloseConfig(f.ctx)
		require.ErrorIs(t, err, service.ErrSelfResetClosePolicyInvalid)
		require.Equal(t, fresh.Revision, f.revision())
		save(nil, check)
		save(admin, nil)
		require.Equal(t, org, f.ticks(g))
	})
}
