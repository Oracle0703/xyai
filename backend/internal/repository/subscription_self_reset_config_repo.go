package repository

import (
	"context"
	"errors"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/setting"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Revisions must round-trip through a JavaScript JSON number.
const maxSelfResetRevision = 1<<53 - 1

func (r *subscriptionSelfResetRepository) ConfigRevision(ctx context.Context, lock bool) (int64, error) {
	query := clientFromContext(ctx, r.client).Setting.Query().Where(setting.KeyEQ(service.SubscriptionSelfResetRevisionKey))
	if lock {
		if dbent.TxFromContext(ctx) == nil {
			return 0, errors.New("locking the self reset revision requires a transaction")
		}
		query.ForUpdate()
	}
	row, err := query.Only(ctx)
	if err != nil {
		return 0, err
	}
	revision, err := strconv.ParseInt(row.Value, 10, 64)
	if err != nil || revision < 1 || revision > maxSelfResetRevision {
		return 0, errors.New("invalid self reset revision")
	}
	return revision, nil
}

func (r *subscriptionSelfResetRepository) AdvanceConfigRevision(ctx context.Context, revision int64) error {
	if dbent.TxFromContext(ctx) == nil {
		return errors.New("advancing the self reset revision requires a transaction")
	}
	if revision >= maxSelfResetRevision {
		return errors.New("self reset revision exhausted")
	}
	updated, err := clientFromContext(ctx, r.client).Setting.Update().
		Where(setting.KeyEQ(service.SubscriptionSelfResetRevisionKey), setting.ValueEQ(strconv.FormatInt(revision, 10))).
		SetValue(strconv.FormatInt(revision+1, 10)).SetUpdatedAt(time.Now()).Save(ctx)
	if err == nil && updated != 1 {
		return errors.New("locked self reset revision changed")
	}
	return err
}
