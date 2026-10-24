package admin

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSubAdminCodexImportSkipsExistingAccounts(t *testing.T) {
	svc := newCodexImportMemoryAdminService(nil)
	h := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	entries := []codexImportEntry{{Index: 1, Value: buildCodexRefreshImportValue(t, "workspace-1", "user-1", "synthetic-refresh")}}

	initial, err := h.importCodexSessions(context.Background(), CodexSessionImportRequest{SkipDefaultGroupBind: boolPtr(true)}, entries)
	require.NoError(t, err)
	require.Equal(t, 1, initial.Created)

	concurrency, priority, proxyID, rate := 99, 1, int64(77), 0.01
	req := CodexSessionImportRequest{
		SkipDefaultGroupBind:    boolPtr(true),
		UpdateExisting:          boolPtr(true),
		Concurrency:             &concurrency,
		Priority:                &priority,
		RateMultiplier:          &rate,
		ProxyID:                 &proxyID,
		GroupIDs:                []int64{88},
		ConfirmMixedChannelRisk: boolPtr(true),
	}

	restricted, err := h.importCodexSessions(service.WithSubAdminAccountRestrictions(context.Background()), req, entries)
	require.NoError(t, err)
	require.Equal(t, 0, restricted.Updated)
	require.Equal(t, 0, restricted.Created)
	require.Equal(t, 1, restricted.Skipped)
	require.Equal(t, "skipped", restricted.Items[0].Action)
	require.NotZero(t, restricted.Items[0].AccountID)
	require.Empty(t, svc.updatedAccounts, "sub admin import must not edit the existing account")
	require.Len(t, svc.createdAccounts, 1, "sub admin import must not create a duplicate either")

	full, err := h.importCodexSessions(context.Background(), req, entries)
	require.NoError(t, err)
	require.Equal(t, 1, full.Updated, "full admin import keeps updating existing accounts")
}
