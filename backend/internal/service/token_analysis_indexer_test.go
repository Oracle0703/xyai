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

func TestTokenAnalysisIndexerResumesFromStoredOffset(t *testing.T) {
	dir := t.TempDir()
	firstLine := `{"archive_id":"old","event":"request","timestamp":"2026-05-19T01:02:03Z","method":"POST","endpoint":"/v1/chat/completions","user_id":7,"api_key_id":9,"model":"gpt-4.1","body":"{\"model\":\"gpt-4.1\",\"messages\":[{\"role\":\"user\",\"content\":\"old\"}]}","body_size":64,"body_sha256":"hash-old"}` + "\n"
	secondLine := `{"archive_id":"new","event":"request","timestamp":"2026-05-19T01:03:03Z","method":"POST","endpoint":"/v1/chat/completions","user_id":7,"api_key_id":9,"model":"gpt-4.1","body":"{\"model\":\"gpt-4.1\",\"messages\":[{\"role\":\"user\",\"content\":\"new\"}]}","body_size":64,"body_sha256":"hash-new"}` + "\n"
	file := filepath.Join(dir, "2026-05-19.jsonl")
	err := os.WriteFile(file, []byte(firstLine+secondLine), 0o600)
	require.NoError(t, err)

	repo := &tokenAnalysisRepoStub{
		indexStates: map[string]TokenAnalysisIndexState{
			file: {SourceFile: file, LastOffset: int64(len(firstLine))},
		},
	}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway:       config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10},
	})

	result, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{StartDate: "2026-05-19", EndDate: "2026-05-19"})

	require.NoError(t, err)
	require.Equal(t, int64(1), result.IndexedRows)
	require.Len(t, repo.upserts, 1)
	require.Equal(t, "new", repo.upserts[0].ArchiveID)
	require.Equal(t, "new", repo.upserts[0].LastUserPreview)
}

func TestTokenAnalysisIndexerSkipsIncompleteTrailingJSONLine(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "2026-05-21.jsonl")
	err := os.WriteFile(file, []byte(
		`{"archive_id":"a1","event":"request","timestamp":"2026-05-21T01:02:03Z","method":"POST","endpoint":"/v1/responses","user_id":7,"api_key_id":9,"model":"gpt-4.1","body":"{\"model\":\"gpt-4.1\",\"input\":\"hello\"}","body_size":64,"body_sha256":"hash1"}`+"\n"+
			`{"archive_id":"partial","event":"request"`),
		0o600,
	)
	require.NoError(t, err)

	repo := &tokenAnalysisRepoStub{}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway:       config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10},
	})

	result, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{StartDate: "2026-05-21", EndDate: "2026-05-21"})

	require.NoError(t, err)
	require.Equal(t, int64(1), result.IndexedRows)
	require.Equal(t, int64(1), result.SkippedRows)
	require.Equal(t, int64(0), result.FailedRows)
	require.Len(t, repo.upserts, 1)
	require.Equal(t, "a1", repo.upserts[0].ArchiveID)
	require.NotEmpty(t, repo.states)
	require.Equal(t, int64(len(`{"archive_id":"a1","event":"request","timestamp":"2026-05-21T01:02:03Z","method":"POST","endpoint":"/v1/responses","user_id":7,"api_key_id":9,"model":"gpt-4.1","body":"{\"model\":\"gpt-4.1\",\"input\":\"hello\"}","body_size":64,"body_sha256":"hash1"}`+"\n")), repo.states[len(repo.states)-1].LastOffset)
}

func TestTokenAnalysisIndexerExtractsProjectAttribution(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "2026-06-05.jsonl")
	// 第一条 Codex 请求直接命中 cwd 并学习仓库根; 第二条 Copilot 请求
	// 自身无 cwd, 依赖附件路径 × 已知根前缀匹配归因。
	codexLine := `{"archive_id":"cx1","event":"request","timestamp":"2026-06-05T01:02:03Z","method":"POST","endpoint":"/v1/responses","user_id":7,"api_key_id":9,"model":"gpt-5.4","user_agent":"Codex Desktop/0.136.0","body":"{\"model\":\"gpt-5.4\",\"input\":[{\"type\":\"message\",\"role\":\"user\",\"content\":[{\"type\":\"input_text\",\"text\":\"<environment_context><cwd>E:\\\\code\\\\lag-killer</cwd></environment_context>\"}]}]}","body_size":120,"body_sha256":"hash-cx"}` + "\n"
	copilotLine := `{"archive_id":"cp1","event":"request","timestamp":"2026-06-05T01:05:00Z","method":"POST","endpoint":"/v1/chat/completions","user_id":8,"api_key_id":10,"model":"xy-gpt-5.5","user_agent":"GitHubCopilotChat/0.51.0","body":"{\"model\":\"xy-gpt-5.5\",\"messages\":[{\"role\":\"user\",\"content\":\"<attachment filePath=\\\"E:\\\\\\\\code\\\\\\\\lag-killer\\\\\\\\src\\\\\\\\main.cpp\\\">code</attachment>\"}]}","body_size":140,"body_sha256":"hash-cp"}` + "\n"
	require.NoError(t, os.WriteFile(file, []byte(codexLine+copilotLine), 0o600))

	repo := &tokenAnalysisRepoStub{}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway:       config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10},
	})

	result, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{StartDate: "2026-06-05", EndDate: "2026-06-05"})

	require.NoError(t, err)
	require.Equal(t, int64(2), result.IndexedRows)
	require.Len(t, repo.upserts, 2)

	codex := repo.upserts[0]
	require.Equal(t, "E:/code/lag-killer", codex.ClientWorkdir)
	require.Equal(t, "lag-killer", codex.ClientProject)
	require.Equal(t, "env-context", codex.AttributionSource)

	copilot := repo.upserts[1]
	require.Equal(t, "lag-killer", copilot.ClientProject)
	require.Equal(t, ProjectAttributionSourceKnownRoot, copilot.AttributionSource)
	require.Equal(t, "e:/code/lag-killer", copilot.ClientWorkdir)

	// 学到的仓库根在索引结束时落库。
	require.NotEmpty(t, repo.rootUpserts)
	require.Equal(t, "lag-killer", repo.projectRoots["e:/code/lag-killer"])
}

type tokenAnalysisRepoStub struct {
	upserts      []TokenAnalysisRequestSummary
	states       []TokenAnalysisIndexState
	indexStates  map[string]TokenAnalysisIndexState
	projectRoots map[string]string
	rootUpserts  []map[string]string
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

func (r *tokenAnalysisRepoStub) GetIndexState(ctx context.Context, sourceFile string) (*TokenAnalysisIndexState, error) {
	if r.indexStates == nil {
		return nil, nil
	}
	state, ok := r.indexStates[sourceFile]
	if !ok {
		return nil, nil
	}
	return &state, nil
}

func (r *tokenAnalysisRepoStub) UpdateIndexState(ctx context.Context, state TokenAnalysisIndexState) error {
	r.states = append(r.states, state)
	return nil
}

func (r *tokenAnalysisRepoStub) ListProjectUsage(ctx context.Context, filters TokenAnalysisFilters, params pagination.PaginationParams) ([]TokenAnalysisProjectUsage, *pagination.PaginationResult, error) {
	return []TokenAnalysisProjectUsage{}, &pagination.PaginationResult{}, nil
}

func (r *tokenAnalysisRepoStub) LoadProjectRoots(ctx context.Context) (map[string]string, error) {
	roots := make(map[string]string, len(r.projectRoots))
	for k, v := range r.projectRoots {
		roots[k] = v
	}
	return roots, nil
}

func (r *tokenAnalysisRepoStub) UpsertProjectRoots(ctx context.Context, roots map[string]string) error {
	copied := make(map[string]string, len(roots))
	for k, v := range roots {
		copied[k] = v
	}
	r.rootUpserts = append(r.rootUpserts, copied)
	if r.projectRoots == nil {
		r.projectRoots = map[string]string{}
	}
	for k, v := range copied {
		r.projectRoots[k] = v
	}
	return nil
}
