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

// 转发侧记录随 response 写入摘要; 响应先于请求入库时经暂存表合并, 不丢失。
func TestTokenAnalysisForwardFactsRecordedAndMerged(t *testing.T) {
	ctx := context.Background()
	repo := NewTokenAnalysisRepository(integrationDB)
	prefix := fmt.Sprintf("forward-facts-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM token_analysis_request_summaries WHERE archive_id LIKE $1`, prefix+"%")
		_, _ = integrationDB.ExecContext(ctx, `DELETE FROM token_analysis_pending_responses WHERE archive_id LIKE $1`, prefix+"%")
	})
	eventTime := time.Date(2026, 10, 26, 1, 2, 3, 0, time.UTC)
	facts := service.TokenAnalysisForwardFacts{
		ForwardPath: service.TokenAnalysisForwardPathOpenAIResponses, CacheKeyHandling: service.TokenAnalysisCacheKeyScoped,
		UpstreamCacheKeyHash: "0123456789abcdef0123456789abcdef", UsageCacheSource: service.TokenAnalysisUsageCacheSourceUpstreamParsed,
	}
	read := func(archiveID string) service.TokenAnalysisForwardFacts {
		var got service.TokenAnalysisForwardFacts
		require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT forward_path, cache_key_handling, upstream_cache_key_hash, usage_cache_source
FROM token_analysis_request_summaries WHERE archive_id = $1`, archiveID).
			Scan(&got.ForwardPath, &got.CacheKeyHandling, &got.UpstreamCacheKeyHash, &got.UsageCacheSource))
		return got
	}
	upsert := func(archiveID string) {
		requireTokenAnalysisUpsert(t, repo, ctx, &service.TokenAnalysisRequestSummary{ArchiveID: archiveID, EventTime: eventTime, Endpoint: "/v1/responses"})
	}

	upsert(prefix + "-direct")
	require.NoError(t, repo.RecordResponse(ctx, service.TokenAnalysisResponseFacts{ArchiveID: prefix + "-direct", CompletedAt: eventTime.Add(time.Second), Status: 200, Forward: facts}))
	require.Equal(t, facts, read(prefix+"-direct"))

	require.NoError(t, repo.RecordResponse(ctx, service.TokenAnalysisResponseFacts{ArchiveID: prefix + "-late", CompletedAt: eventTime.Add(time.Second), Status: 200, Forward: facts}))
	upsert(prefix + "-late")
	require.Equal(t, service.TokenAnalysisForwardFacts{}, read(prefix+"-late"))
	require.NoError(t, repo.MergePendingResponses(ctx))
	require.Equal(t, facts, read(prefix+"-late"))
}

// requireTokenAnalysisUpsert 写入摘要并断言成功; 写入返回的候选快照状态在专门的测试中检查。
func requireTokenAnalysisUpsert(t *testing.T, repo service.TokenAnalysisRepository, ctx context.Context, summary *service.TokenAnalysisRequestSummary) {
	t.Helper()
	_, err := repo.UpsertRequestSummary(ctx, summary)
	require.NoError(t, err)
}
