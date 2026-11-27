package service

import (
	"context"
	"database/sql"
	"encoding/json"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const SubscriptionSelfResetRevisionKey = "subscription_self_daily_reset_revision"

var (
	ErrSelfResetRevisionConflict   = errors.Conflict("SELF_RESET_REVISION_CONFLICT", "Self reset configuration changed")
	ErrSelfResetGroupConflict      = errors.Conflict("SELF_RESET_GROUP_CONFLICT", "A selected group no longer belongs to this organization")
	ErrSelfResetClosePolicyInvalid = errors.Conflict("SELF_RESET_CLOSE_POLICY_INVALID", "Repair the missing or invalid self reset policy before closing it")
)

type SubscriptionSelfResetConfig struct {
	Revision int64                                   `json:"revision"`
	Policy   *SubscriptionSelfResetPolicy            `json:"policy"`
	Groups   map[string][]SubscriptionSelfResetGroup `json:"groups_by_organization"`
}

type SubscriptionSelfResetConfigChange struct {
	ExpectedRevision int64
	Policy           json.RawMessage
	Groups           map[string]SubscriptionSelfResetGroupChanges
}

// Config uses one MVCC snapshot even though the candidates are queried by
// organization. An unreadable section is null so the other can still be edited.
func (s *SubscriptionSelfResetService) Config(ctx context.Context) (*SubscriptionSelfResetConfig, error) {
	tx, err := s.configTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	ctx = dbent.NewTxContext(ctx, tx)
	revision, err := s.repo.ConfigRevision(ctx, false)
	if err != nil {
		return nil, selfResetStorageError(err)
	}
	policy, _ := s.Policy(ctx)
	groups, _ := s.repo.ListGroups(ctx)
	return &SubscriptionSelfResetConfig{Revision: revision, Policy: policy, Groups: groups}, nil
}

func (s *SubscriptionSelfResetService) configTx(ctx context.Context, options *sql.TxOptions) (*dbent.Tx, error) {
	if s.subscriptions == nil || s.subscriptions.entClient == nil {
		return nil, errors.ServiceUnavailable("SELF_RESET_UNAVAILABLE", "Self reset transaction unavailable")
	}
	tx, err := s.subscriptions.entClient.BeginTx(ctx, options)
	if err != nil {
		return nil, selfResetStorageError(err)
	}
	return tx, nil
}

// SaveConfig also accepts just an expected revision. Advancing it is a write
// fence: after this transaction commits, a delayed save at that revision cannot.
func (s *SubscriptionSelfResetService) SaveConfig(ctx context.Context, change SubscriptionSelfResetConfigChange) (int64, error) {
	if change.ExpectedRevision < 1 {
		return 0, errors.BadRequest("INVALID_SELF_RESET_CONFIG", "An expected revision is required")
	}
	var policy *SubscriptionSelfResetPolicy
	if change.Policy != nil {
		var err error
		policy, err = ParseSubscriptionSelfResetPolicy(change.Policy)
		if err != nil {
			return 0, err
		}
	}
	return s.updateConfig(ctx, change.ExpectedRevision, func(txCtx context.Context) error {
		if len(change.Groups) > 0 {
			ignored, err := s.repo.ApplyGroupChanges(txCtx, change.Groups)
			if err != nil {
				return err
			}
			if len(ignored) != 0 {
				return ErrSelfResetGroupConflict
			}
		}
		if policy != nil {
			return s.writeConfigPolicy(txCtx, policy)
		}
		return nil
	})
}

// CloseConfig never applies stale form fields. Repeated closes invalidate any
// pending save, even when rollout is already off.
func (s *SubscriptionSelfResetService) CloseConfig(ctx context.Context) (int64, error) {
	return s.updateConfig(ctx, 0, func(txCtx context.Context) error {
		raw, err := s.repo.ReadPolicy(txCtx)
		if dbent.IsNotFound(err) {
			return ErrSelfResetClosePolicyInvalid.WithCause(err)
		}
		if err != nil {
			return err
		}
		policy, err := ParseSubscriptionSelfResetPolicy([]byte(raw))
		if err != nil {
			return ErrSelfResetClosePolicyInvalid.WithCause(err)
		}
		policy.Rollout = SubscriptionSelfResetRolloutOff
		return s.writeConfigPolicy(txCtx, policy)
	})
}

func (s *SubscriptionSelfResetService) writeConfigPolicy(ctx context.Context, policy *SubscriptionSelfResetPolicy) error {
	encoded, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	return s.repo.WritePolicy(ctx, string(encoded))
}

func (s *SubscriptionSelfResetService) updateConfig(ctx context.Context, expected int64, apply func(context.Context) error) (int64, error) {
	tx, err := s.configTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	ctx = dbent.NewTxContext(ctx, tx)
	revision, err := s.repo.ConfigRevision(ctx, true)
	if err != nil {
		return 0, selfResetStorageError(err)
	}
	if expected != 0 && revision != expected {
		return 0, ErrSelfResetRevisionConflict
	}
	if err := apply(ctx); err != nil {
		return 0, selfResetStorageError(err)
	}
	if err := s.repo.AdvanceConfigRevision(ctx, revision); err != nil {
		return 0, selfResetStorageError(err)
	}
	if err := tx.Commit(); err != nil {
		return 0, selfResetStorageError(err)
	}
	return revision + 1, nil
}
