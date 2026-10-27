//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 摘要保存归档 UA 与客户端族; 规则版本变化后只重新映射已识别(版本非 0)的旧版本行。
// 写回按读出时的 UA 与版本快照条件提交: 期间被重新索引(无论新版本是什么)的行不被覆盖。
func TestTokenAnalysisClientIdentityUpsertAndRemap(t *testing.T) {
	ctx := context.Background()
	repo := NewTokenAnalysisRepository(integrationDB)
	prefix := fmt.Sprintf("client-identity-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM token_analysis_request_summaries WHERE archive_id LIKE $1`, prefix+"%")
	})
	eventTime := time.Date(2026, 10, 26, 1, 2, 3, 0, time.UTC)
	upsert := func(suffix, ua, family string, version int32) {
		requireTokenAnalysisUpsert(t, repo, ctx, &service.TokenAnalysisRequestSummary{
			ArchiveID: prefix + suffix, EventTime: eventTime, Endpoint: "/v1/responses",
			UserAgent: ua, ClientFamily: family, ClientFamilyVersion: version,
		})
	}
	read := func(suffix string) (string, string, int32) {
		var ua, family string
		var version int32
		require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT user_agent, client_family, client_family_version FROM token_analysis_request_summaries WHERE archive_id = $1`,
			prefix+suffix).Scan(&ua, &family, &version))
		return ua, family, version
	}

	upsert("-a", "Codex Desktop/0.160.0", "other", 5)
	upsert("-b", "hertz", "other", 5)
	upsert("-legacy", "", "", 0)
	ua, family, version := read("-a")
	require.Equal(t, "Codex Desktop/0.160.0", ua)
	require.Equal(t, "other", family)
	require.EqualValues(t, 5, version)

	stale, err := repo.ListStaleClientFamilies(ctx, 7, 0, 10000)
	require.NoError(t, err)
	ours := map[string]service.TokenAnalysisClientFamilyRow{}
	for _, row := range stale {
		switch row.UserAgent {
		case "Codex Desktop/0.160.0", "hertz":
			ours[row.UserAgent] = row
		}
	}
	require.Len(t, ours, 2, "version-0 legacy rows are not remapped")

	// 重新映射之前, -b 被另一规则实例重新索引为 UA=GitHubCopilotChat、版本 9(版本是
	// 规则哈希, 不表示先后): 写回不得覆盖它。
	upsert("-b", "GitHubCopilotChat/0.69.0", "copilot", 9)
	codex := ours["Codex Desktop/0.160.0"]
	codex.Family = "codex"
	hertz := ours["hertz"]
	hertz.Family = "stale-family"
	require.NoError(t, repo.UpdateClientFamilies(ctx, 7, []service.TokenAnalysisClientFamilyRow{codex, hertz}))

	_, family, version = read("-a")
	require.Equal(t, "codex", family)
	require.EqualValues(t, 7, version)
	ua, family, version = read("-b")
	require.Equal(t, "GitHubCopilotChat/0.69.0", ua)
	require.Equal(t, "copilot", family)
	require.EqualValues(t, 9, version)

	// 游标: 从 -a 的 id 之后续读, 不再返回 -a。
	var idA int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT id FROM token_analysis_request_summaries WHERE archive_id = $1`, prefix+"-a").Scan(&idA))
	after, err := repo.ListStaleClientFamilies(ctx, 11, idA, 10000)
	require.NoError(t, err)
	for _, row := range after {
		require.Greater(t, row.ID, idA)
	}
	_, family, version = read("-legacy")
	require.Empty(t, family)
	require.Zero(t, version)
}
