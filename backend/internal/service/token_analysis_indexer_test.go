package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	}, nil)

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
	}, nil)

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
	}, nil)

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
	}, nil)

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
	userInputs   []TokenAnalysisUserInput
	// summary/billedRequests 供 GetSummary 口径测试注入返回值;
	// summaryFilters 记录 repo 实际收到的过滤条件。
	summary        *TokenAnalysisSummary
	billedRequests int64
	summaryFilters *TokenAnalysisFilters
	// usageLookups 记录 FindNearestUsageLog 调用次数(count_tokens 行不得匹配)。
	usageLookups int
}

func (r *tokenAnalysisRepoStub) UpsertRequestSummary(ctx context.Context, summary *TokenAnalysisRequestSummary) error {
	if summary == nil {
		return nil
	}
	r.upserts = append(r.upserts, *summary)
	return nil
}

func (r *tokenAnalysisRepoStub) FindNearestUsageLog(ctx context.Context, eventTime time.Time, userID, apiKeyID *int64, model string, window time.Duration) (*TokenAnalysisUsageMatch, error) {
	r.usageLookups++
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
	r.summaryFilters = &filters
	if r.summary != nil {
		copied := *r.summary
		return &copied, nil
	}
	return &TokenAnalysisSummary{}, nil
}

func (r *tokenAnalysisRepoStub) CountBilledRequests(ctx context.Context, filters TokenAnalysisFilters) (int64, error) {
	return r.billedRequests, nil
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

func (r *tokenAnalysisRepoStub) UpsertUserInput(ctx context.Context, input *TokenAnalysisUserInput) error {
	if input == nil {
		return nil
	}
	r.userInputs = append(r.userInputs, *input)
	return nil
}

func (r *tokenAnalysisRepoStub) GetUserInput(ctx context.Context, archiveID string) (*TokenAnalysisUserInput, error) {
	for i := len(r.userInputs) - 1; i >= 0; i-- {
		if r.userInputs[i].ArchiveID == archiveID {
			input := r.userInputs[i]
			return &input, nil
		}
	}
	return nil, nil
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

// cancelOnUpsertRepoStub 在首条 summary 落库后取消 ctx, 模拟扫描进行中触发停机。
type cancelOnUpsertRepoStub struct {
	tokenAnalysisRepoStub
	cancel context.CancelFunc
}

func (r *cancelOnUpsertRepoStub) UpsertRequestSummary(ctx context.Context, summary *TokenAnalysisRequestSummary) error {
	if err := r.tokenAnalysisRepoStub.UpsertRequestSummary(ctx, summary); err != nil {
		return err
	}
	if r.cancel != nil {
		r.cancel()
	}
	return nil
}

func TestTokenAnalysisIndexerStopsScanningSkippedLinesAfterCancel(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "2026-06-08.jsonl")
	// 1 条 request + 大量跳过行(response 事件不经过任何 repo 调用):
	// 取消后若循环不检查 ctx.Err(), 文件会被扫完并返回成功(回归场景)。
	requestLine := `{"archive_id":"a1","event":"request","timestamp":"2026-06-08T01:02:03Z","method":"POST","endpoint":"/v1/chat/completions","user_id":7,"api_key_id":9,"model":"gpt-4.1","body":"{\"model\":\"gpt-4.1\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}","body_size":64,"body_sha256":"hash1"}` + "\n"
	skipLine := `{"archive_id":"a1","event":"response","timestamp":"2026-06-08T01:02:04Z","status":200,"body":"{\"ok\":true}"}` + "\n"
	require.NoError(t, os.WriteFile(file, []byte(requestLine+strings.Repeat(skipLine, 5000)), 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo := &cancelOnUpsertRepoStub{cancel: cancel}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway:       config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10},
	}, nil)

	_, err := svc.IndexRange(ctx, TokenAnalysisIndexRequest{StartDate: "2026-06-08", EndDate: "2026-06-08"})

	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, repo.upserts, 1)
}

func TestTokenAnalysisIndexerStoresUserInput(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "2026-06-07.jsonl")
	// 用户输入含换行, 留存内容必须保留排版; maxChars=5 触发截断。
	line := `{"archive_id":"in1","event":"request","timestamp":"2026-06-07T01:02:03Z","method":"POST","endpoint":"/v1/chat/completions","user_id":7,"api_key_id":9,"model":"gpt-4.1","body":"{\"model\":\"gpt-4.1\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\\nworld\"}]}","body_size":64,"body_sha256":"hash-in1"}` + "\n"
	require.NoError(t, os.WriteFile(file, []byte(line), 0o600))

	repo := &tokenAnalysisRepoStub{}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway: config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{
			IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10,
			InputStoreMaxChars: 5,
		},
	}, nil)

	result, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{StartDate: "2026-06-07", EndDate: "2026-06-07"})

	require.NoError(t, err)
	require.Equal(t, int64(1), result.IndexedRows)
	require.Len(t, repo.userInputs, 1)
	input := repo.userInputs[0]
	require.Equal(t, "in1", input.ArchiveID)
	require.Equal(t, "hello", input.Content)
	require.True(t, input.Truncated)
	require.Equal(t, 5, input.Chars)
	// 哈希对脱敏前原文计算, 跨截断稳定。
	require.Len(t, input.ContentSHA256, 64)
	require.Equal(t, int64(7), *input.UserID)
}

func TestTokenAnalysisIndexerUserInputHashNotRawFingerprint(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "2026-06-08.jsonl")
	// 输入含 secret: content 必须脱敏, 且 content_sha256 不得等于原文 SHA256
	// (否则拿候选 secret 可离线比对确认其出现过, codex 审查重要1)。
	line := `{"archive_id":"sec1","event":"request","timestamp":"2026-06-08T01:02:03Z","method":"POST","endpoint":"/v1/chat/completions","user_id":7,"api_key_id":9,"model":"gpt-4.1","body":"{\"model\":\"gpt-4.1\",\"messages\":[{\"role\":\"user\",\"content\":\"use sk-secret-1234567890 please\"}]}","body_size":64,"body_sha256":"hash-sec1"}` + "\n"
	require.NoError(t, os.WriteFile(file, []byte(line), 0o600))

	repo := &tokenAnalysisRepoStub{}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway: config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{
			IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10,
			InputStoreMaxChars: 8000,
		},
	}, nil)

	_, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{StartDate: "2026-06-08", EndDate: "2026-06-08"})

	require.NoError(t, err)
	require.Len(t, repo.userInputs, 1)
	input := repo.userInputs[0]
	require.NotContains(t, input.Content, "sk-secret")
	require.Contains(t, input.Content, "[redacted]")
	rawSum := sha256.Sum256([]byte("use sk-secret-1234567890 please"))
	require.NotEqual(t, hex.EncodeToString(rawSum[:]), input.ContentSHA256)
	// 哈希与脱敏后文本一致, 去重语义保留。
	redactedSum := sha256.Sum256([]byte(input.Content))
	require.Equal(t, hex.EncodeToString(redactedSum[:]), input.ContentSHA256)
}

func TestTokenAnalysisIndexerSkipsUserInputWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "2026-06-07.jsonl")
	line := `{"archive_id":"in2","event":"request","timestamp":"2026-06-07T01:02:03Z","method":"POST","endpoint":"/v1/chat/completions","user_id":7,"api_key_id":9,"model":"gpt-4.1","body":"{\"model\":\"gpt-4.1\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}","body_size":64,"body_sha256":"hash-in2"}` + "\n"
	require.NoError(t, os.WriteFile(file, []byte(line), 0o600))

	repo := &tokenAnalysisRepoStub{}
	// InputStoreMaxChars 0 = 不留存全文, 只写摘要。
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway: config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{
			IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10,
		},
	}, nil)

	result, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{StartDate: "2026-06-07", EndDate: "2026-06-07"})

	require.NoError(t, err)
	require.Equal(t, int64(1), result.IndexedRows)
	require.Len(t, repo.upserts, 1)
	require.Empty(t, repo.userInputs)
}

func TestTokenAnalysisIndexerDegradesTruncatedBodyToMetadata(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "2026-06-09.jsonl")
	// 归档端按 max_request_body_bytes 截断后 body 不是完整 JSON:
	// 行本身合法 + body_truncated=true, 应降级为元数据入库而非失败行。
	line := `{"archive_id":"tr1","event":"request","timestamp":"2026-06-09T01:02:03Z","method":"POST","endpoint":"/v1/responses","user_id":7,"api_key_id":9,"model":"gpt-5.5","body":"{\"model\":\"gpt-5.5\",\"input\":[{\"type\":\"mess","body_size":9299897,"body_sha256":"hash-tr1","body_truncated":true}` + "\n"
	require.NoError(t, os.WriteFile(file, []byte(line), 0o600))

	repo := &tokenAnalysisRepoStub{}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway:       config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10},
	}, nil)

	result, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{StartDate: "2026-06-09", EndDate: "2026-06-09"})

	require.NoError(t, err)
	require.Equal(t, int64(1), result.IndexedRows)
	require.Equal(t, int64(0), result.FailedRows)
	require.Len(t, repo.upserts, 1)
	summary := repo.upserts[0]
	require.Equal(t, "tr1", summary.ArchiveID)
	require.True(t, summary.RequestBodyTruncated)
	require.Equal(t, "gpt-5.5", summary.Model)
	require.Equal(t, int64(9299897), summary.RequestBodySize)
	require.Equal(t, "body_truncated", summary.SummaryJSON["degraded"])
	for _, state := range repo.states {
		require.Empty(t, state.LastError)
		require.Zero(t, state.FailedRows)
	}
}

func TestTokenAnalysisIndexerSkipsUsageMatchForCountTokens(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "2026-06-09.jsonl")
	// count_tokens 探测不计费, 不得做用量匹配(否则与紧邻真实请求匹配到同一条
	// usage_logs, 概览把 token 累加两次); endpoint 还原 /count_tokens 后缀,
	// 库内才能与真实 messages 请求区分。
	line := `{"archive_id":"ct1","event":"request","timestamp":"2026-06-09T01:02:03Z","method":"POST","path":"/antigravity/v1/messages/count_tokens","endpoint":"/v1/messages","user_id":7,"api_key_id":9,"model":"claude-sonnet-4-6","body":"{\"model\":\"claude-sonnet-4-6\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}]}","body_size":72,"body_sha256":"hash-ct1"}` + "\n"
	require.NoError(t, os.WriteFile(file, []byte(line), 0o600))

	repo := &tokenAnalysisRepoStub{}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway:       config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10},
	}, nil)

	result, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{StartDate: "2026-06-09", EndDate: "2026-06-09"})

	require.NoError(t, err)
	require.Equal(t, int64(1), result.IndexedRows)
	require.Equal(t, int64(0), result.FailedRows)
	require.Zero(t, repo.usageLookups)
	require.Len(t, repo.upserts, 1)
	summary := repo.upserts[0]
	require.Nil(t, summary.UsageLogID)
	require.Equal(t, int16(0), summary.MatchConfidence)
	require.Equal(t, "/v1/messages/count_tokens", summary.Endpoint)
}

func TestTokenAnalysisIndexerIndexesMultipartImageEditRow(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "2026-06-10.jsonl")
	contentType, mpBody := buildTokenAnalysisMultipartImageEditBody(t, 64, 64)
	// 生产事故形态: /v1/images/edits 的 multipart 体被原样归档, 修复前按
	// JSON 解析报 parse request body: invalid character '-' 并计失败行。
	// string(body) 经 json.Marshal 把非法 UTF-8 替换为 U+FFFD, 复现归档端
	// 对图片二进制的损毁; 文本域不受影响。
	event := map[string]any{
		"archive_id": "mp1", "event": "request", "timestamp": "2026-06-10T01:02:03Z",
		"method": "POST", "path": "/v1/images/edits", "endpoint": "/v1/images/edits",
		"user_id": 7, "api_key_id": 9,
		"headers": map[string]any{"content_type": contentType},
		"body":    string(mpBody), "body_size": len(mpBody), "body_sha256": "hash-mp1",
	}
	raw, err := json.Marshal(event)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, append(raw, '\n'), 0o600))

	repo := &tokenAnalysisRepoStub{}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway:       config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10},
	}, nil)

	result, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{StartDate: "2026-06-10", EndDate: "2026-06-10"})

	require.NoError(t, err)
	require.Equal(t, int64(1), result.IndexedRows)
	require.Equal(t, int64(0), result.FailedRows)
	require.Len(t, repo.upserts, 1)
	summary := repo.upserts[0]
	require.Equal(t, "gpt-image-2", summary.Model)
	require.Contains(t, summary.LastUserPreview, "赛博朋克")
	require.Equal(t, true, summary.SummaryJSON["multipart"])
	require.NotContains(t, summary.SummaryJSON, "degraded")
	for _, state := range repo.states {
		require.Empty(t, state.LastError)
		require.Zero(t, state.FailedRows)
	}
}

func TestTokenAnalysisIndexerUpgradesTruncatedMultipartRow(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "2026-06-10.jsonl")
	_, mpBody := buildTokenAnalysisMultipartImageEditBody(t, 4096)
	// 截断的 multipart 行此前只能降级 body_truncated 仅元数据; 现在优先走
	// multipart 解析(文本域在前缀内完整), 升级为带 prompt 的摘要。本行故意
	// 不带 headers, 同时覆盖 body 首行 boundary 兜底路径。
	event := map[string]any{
		"archive_id": "mp2", "event": "request", "timestamp": "2026-06-10T02:02:03Z",
		"method": "POST", "path": "/v1/images/edits", "endpoint": "/v1/images/edits",
		"user_id": 7, "api_key_id": 9,
		"body": string(mpBody[:len(mpBody)-2048]), "body_size": len(mpBody),
		"body_sha256": "hash-mp2", "body_truncated": true,
	}
	raw, err := json.Marshal(event)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, append(raw, '\n'), 0o600))

	repo := &tokenAnalysisRepoStub{}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway:       config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10},
	}, nil)

	result, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{StartDate: "2026-06-10", EndDate: "2026-06-10"})

	require.NoError(t, err)
	require.Equal(t, int64(1), result.IndexedRows)
	require.Equal(t, int64(0), result.FailedRows)
	require.Len(t, repo.upserts, 1)
	summary := repo.upserts[0]
	require.True(t, summary.RequestBodyTruncated)
	require.Equal(t, "gpt-image-2", summary.Model)
	require.Contains(t, summary.LastUserPreview, "赛博朋克")
	require.NotContains(t, summary.SummaryJSON, "degraded")
}

func TestTokenAnalysisIndexerCountsUntruncatedBadBodyAsFailed(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "2026-06-09.jsonl")
	// 未截断却解析不了的 body 是真异常: 维持计失败行 + 记录 last_error。
	line := `{"archive_id":"bad1","event":"request","timestamp":"2026-06-09T01:02:03Z","method":"POST","endpoint":"/v1/responses","user_id":7,"api_key_id":9,"model":"gpt-5.5","body":"not-json","body_size":8,"body_sha256":"hash-bad1"}` + "\n"
	require.NoError(t, os.WriteFile(file, []byte(line), 0o600))

	repo := &tokenAnalysisRepoStub{}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway:       config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10},
	}, nil)

	result, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{StartDate: "2026-06-09", EndDate: "2026-06-09"})

	require.NoError(t, err)
	require.Equal(t, int64(0), result.IndexedRows)
	require.Equal(t, int64(1), result.FailedRows)
	require.Empty(t, repo.upserts)
	var lastError string
	var failedRows int64
	for _, state := range repo.states {
		if state.LastError != "" {
			lastError = state.LastError
		}
		failedRows += state.FailedRows
	}
	require.Contains(t, lastError, "parse request body")
	require.Equal(t, int64(1), failedRows)
}

func TestTokenAnalysisIndexerStripsNULFromArchiveText(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "2026-06-10.jsonl")
	// 用户输入含 JSON 的 NUL 转义(真实来源: 二进制内容被贴进工具结果),
	// 解码后是 0x00; Postgres text 列拒收, 预览/留存/哈希入库前必须剥离。
	// 生产事故形态: upsert token analysis user input: pq: invalid byte sequence。
	bodyJSON := `{"model":"gpt-4.1","messages":[{"role":"user","content":"bad` + "\\" + `u0000text"}]}`
	event := map[string]any{
		"archive_id": "nul1", "event": "request", "timestamp": "2026-06-10T01:02:03Z",
		"method": "POST", "endpoint": "/v1/chat/completions", "user_id": 7, "api_key_id": 9,
		"model": "gpt-4.1", "body": bodyJSON, "body_size": len(bodyJSON), "body_sha256": "hash-nul1",
	}
	raw, err := json.Marshal(event)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, append(raw, '\n'), 0o600))

	repo := &tokenAnalysisRepoStub{}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway: config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{
			IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10,
			InputStoreMaxChars: 8000,
		},
	}, nil)

	result, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{StartDate: "2026-06-10", EndDate: "2026-06-10"})

	require.NoError(t, err)
	require.Equal(t, int64(1), result.IndexedRows)
	require.Equal(t, int64(0), result.FailedRows)
	require.Len(t, repo.upserts, 1)
	require.Equal(t, "badtext", repo.upserts[0].LastUserPreview)
	require.Len(t, repo.userInputs, 1)
	input := repo.userInputs[0]
	require.Equal(t, "badtext", input.Content)
	require.NotContains(t, input.Content, "\x00")
	// 哈希对剥离 NUL 后的脱敏文本计算, 与 content 一致。
	sum := sha256.Sum256([]byte("badtext"))
	require.Equal(t, hex.EncodeToString(sum[:]), input.ContentSHA256)
}

func TestTokenAnalysisIndexerSanitizesLastErrorNUL(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "2026-06-10.jsonl")
	// 解析错误消息内嵌归档原文(此处为带 NUL 的 timestamp): last_error 落库
	// 前同样要剥离, 否则记录失败行状态本身又触发 pq invalid byte sequence。
	event := map[string]any{
		"archive_id": "ts1", "event": "request", "timestamp": "2026-06-10T01:02:03Z\x00bad",
		"method": "POST", "endpoint": "/v1/chat/completions", "user_id": 7, "api_key_id": 9,
		"model": "gpt-4.1", "body": `{"model":"gpt-4.1"}`, "body_size": 19, "body_sha256": "hash-ts1",
	}
	raw, err := json.Marshal(event)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, append(raw, '\n'), 0o600))

	repo := &tokenAnalysisRepoStub{}
	svc := NewTokenAnalysisService(repo, &config.Config{
		Gateway:       config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10},
	}, nil)

	result, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{StartDate: "2026-06-10", EndDate: "2026-06-10"})

	require.NoError(t, err)
	require.Equal(t, int64(1), result.FailedRows)
	var lastError string
	for _, state := range repo.states {
		if state.LastError != "" {
			lastError = state.LastError
		}
	}
	require.Contains(t, lastError, "parse archive timestamp")
	require.NotContains(t, lastError, "\x00")
}

func TestTokenAnalysisServiceGetUserInputNotFound(t *testing.T) {
	repo := &tokenAnalysisRepoStub{}
	svc := NewTokenAnalysisService(repo, &config.Config{}, nil)

	_, err := svc.GetUserInput(context.Background(), "missing")

	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}
