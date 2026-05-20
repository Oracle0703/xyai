package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestTokenAnalysisIndexerIndexesOnlyRequestEvents(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "2026-05-19.jsonl")
	err := os.WriteFile(file, []byte(
		`{"archive_id":"a1","event":"request","timestamp":"2026-05-19T01:02:03Z","method":"POST","endpoint":"/v1/chat/completions","user_id":7,"api_key_id":9,"model":"gpt-4.1","body":"{\"model\":\"gpt-4.1\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}","body_size":64,"body_sha256":"hash1"}`+"\n"+
			`{"archive_id":"a1","event":"response","timestamp":"2026-05-19T01:02:04Z","status":200,"body":"{\"ok\":true}"}`+"\n"),
		0o600,
	)
	require.NoError(t, err)

	repo := &tokenAnalysisRepoStub{}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway: config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{
			IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10,
		},
	})

	result, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{
		StartDate: "2026-05-19",
		EndDate:   "2026-05-19",
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), result.IndexedRows)
	require.Len(t, repo.upserts, 1)
	require.Equal(t, "a1", repo.upserts[0].ArchiveID)
	require.Equal(t, "hello", repo.upserts[0].LastUserPreview)
	require.Equal(t, int16(3), repo.upserts[0].MatchConfidence)
	require.Equal(t, int64(123), *repo.upserts[0].UsageLogID)
}

type tokenAnalysisRepoStub struct {
	upserts []TokenAnalysisRequestSummary
	states  []TokenAnalysisIndexState
}

func (r *tokenAnalysisRepoStub) UpsertRequestSummary(ctx context.Context, summary *TokenAnalysisRequestSummary) error {
	if summary == nil {
		return nil
	}
	r.upserts = append(r.upserts, *summary)
	return nil
}

func (r *tokenAnalysisRepoStub) FindNearestUsageLog(ctx context.Context, eventTime time.Time, userID, apiKeyID *int64, model string, window time.Duration) (*TokenAnalysisUsageMatch, error) {
	return &TokenAnalysisUsageMatch{
		UsageLogID:          123,
		MatchConfidence:     3,
		InputTokens:         1000,
		OutputTokens:        20,
		CacheReadTokens:     100,
		CacheCreationTokens: 50,
		TotalCost:           0.1,
		ActualCost:          0.2,
	}, nil
}

func (r *tokenAnalysisRepoStub) CountSameBodyRecent(ctx context.Context, bodySHA256 string, userID, apiKeyID *int64, eventTime time.Time, window time.Duration) (int, error) {
	return 1, nil
}

func (r *tokenAnalysisRepoStub) GetSummary(ctx context.Context, filters TokenAnalysisFilters) (*TokenAnalysisSummary, error) {
	return &TokenAnalysisSummary{}, nil
}

func (r *tokenAnalysisRepoStub) ListUserUsage(ctx context.Context, filters TokenAnalysisFilters, params pagination.PaginationParams) ([]TokenAnalysisUserUsage, *pagination.PaginationResult, error) {
	return []TokenAnalysisUserUsage{}, &pagination.PaginationResult{}, nil
}

func (r *tokenAnalysisRepoStub) ListRequests(ctx context.Context, filters TokenAnalysisFilters, params pagination.PaginationParams) ([]TokenAnalysisRequestItem, *pagination.PaginationResult, error) {
	return []TokenAnalysisRequestItem{}, &pagination.PaginationResult{}, nil
}

func (r *tokenAnalysisRepoStub) GetIndexStatus(ctx context.Context) (*TokenAnalysisIndexStatus, error) {
	return &TokenAnalysisIndexStatus{}, nil
}

func (r *tokenAnalysisRepoStub) UpdateIndexState(ctx context.Context, state TokenAnalysisIndexState) error {
	r.states = append(r.states, state)
	return nil
}
