//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// installOrganizationUsageRollupDirtyMarks adds the organization usage rollup dirty-mark table and
// function (migration 246) to an isolated group-rollup test schema. Production databases always
// have them; dropUsageLogsPartition marks the dropped month in the same transaction, so schema
// fixtures that exercise partition drops need them too.
func installOrganizationUsageRollupDirtyMarks(t *testing.T, ctx context.Context, schema string) {
	t.Helper()

	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, setGroupUsageRollupTriggerSearchPath(ctx, tx, pq.QuoteIdentifier(schema)))
	_, err = tx.ExecContext(ctx, `
		CREATE TABLE usage_user_rollup_dirty_days (
			id BIGSERIAL PRIMARY KEY,
			bucket_date DATE NOT NULL,
			marked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE FUNCTION mark_user_usage_rollup_dirty_day(p_day DATE)
		RETURNS VOID
		LANGUAGE plpgsql
		AS $$
		DECLARE
			marker TEXT := 'sub2api.user_rollup_dirty_' || to_char(p_day, 'YYYYMMDD');
		BEGIN
			IF current_setting(marker, true) = 't' THEN
				RETURN;
			END IF;
			INSERT INTO usage_user_rollup_dirty_days (bucket_date) VALUES (p_day);
			PERFORM set_config(marker, 't', true);
		END;
		$$;
	`)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}
