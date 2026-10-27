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
		`{"archive_id":"a1","client_request_id":"cid-1","event":"request","timestamp":"2026-05-19T01:02:03Z","method":"POST","endpoint":"/v1/chat/completions","user_id":7,"api_key_id":9,"model":"gpt-4.1","body":"{\"model\":\"gpt-4.1\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}","body_size":64,"body_sha256":"hash1"}`+"\n"+
			`{"archive_id":"a1","client_request_id":"cid-1","event":"response","timestamp":"2026-05-19T01:02:04Z","status":200,"usage":{"completion_tokens":20},"response_id":"resp_1","token_analysis":{"forward_path":"openai_responses","cache_key_handling":"scoped","upstream_cache_key_hash":"0123456789abcdef0123456789abcdef","usage_cache_source":"upstream_parsed"}}`+"\n"),
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
	// 请求行入库时不找账单; response 行写回完成事实, 扫描结束后的关联轮次再关联。
	require.Nil(t, repo.upserts[0].UsageLogID)
	require.Equal(t, "cid-1", repo.upserts[0].ClientRequestID)
	require.Len(t, repo.responses, 1)
	outputTokens := int64(20)
	require.Equal(t, TokenAnalysisResponseFacts{
		ArchiveID: "a1", CompletedAt: time.Date(2026, 5, 19, 1, 2, 4, 0, time.UTC), Status: 200, OutputTokens: &outputTokens, ResponseID: "resp_1",
		// 转发侧记录随 response 行进入完成事实。
		Forward: TokenAnalysisForwardFacts{
			ForwardPath: TokenAnalysisForwardPathOpenAIResponses, CacheKeyHandling: TokenAnalysisCacheKeyScoped,
			UpstreamCacheKeyHash: "0123456789abcdef0123456789abcdef", UsageCacheSource: TokenAnalysisUsageCacheSourceUpstreamParsed,
		},
	}, repo.responses[0])
	require.Len(t, repo.usageLookups, 1)
	require.Equal(t, "cid-1", repo.usageLookups[0].ClientRequestID)
	require.Equal(t, "resp_1", repo.usageLookups[0].ResponseID)
	// 有关联键与上游 response.id 的新请求只做精确关联。
	require.True(t, repo.usageLookups[0].ExactOnly)
	require.Equal(t, time.Date(2026, 5, 19, 1, 2, 4, 0, time.UTC), *repo.usageLookups[0].CompletedAt)
	require.Equal(t, int64(123), repo.links["a1"].UsageLogID)
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
	// 客户端识别取自归档事件 UA, 不依赖计费关联。
	require.Equal(t, "Codex Desktop/0.136.0", codex.UserAgent)
	require.Equal(t, "codex", codex.ClientFamily)
	require.NotZero(t, codex.ClientFamilyVersion)

	copilot := repo.upserts[1]
	require.Equal(t, "copilot", copilot.ClientFamily)
	require.Equal(t, codex.ClientFamilyVersion, copilot.ClientFamilyVersion)
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
	// 计费关联: responses 记录 response 行完成事实; linkCandidates 非 nil 时
	// 直接作为待关联行, 否则由 upserts 推出(仿 SQL 排除 count_tokens 与已确认行);
	// findUsage 决定 FindUsageLink 返回(默认 123 号账单), usageLookups 记录查询,
	// links 记录 SetUsageLink 结果(nil 值表示解除关联)。
	responses      []TokenAnalysisResponseFacts
	linkCandidates []TokenAnalysisUsageLinkCandidate
	findUsage      func(TokenAnalysisUsageLinkQuery) *TokenAnalysisUsageMatch
	usageLookups   []TokenAnalysisUsageLinkQuery
	links          map[string]*TokenAnalysisUsageMatch
	linkRisk       map[string][]TokenAnalysisRiskReason
	pendingMerges  int
	pendingCleared []string
	keptPending    []string
	// requestItems/requestFilters 供请求明细口径测试注入返回值并记录过滤条件。
	requestItems   []TokenAnalysisRequestItem
	requestFilters *TokenAnalysisFilters
	// staleFamilies 是待重新映射的行; familyUpdates 记录写回, familyListCalls 计查询次数。
	staleFamilies   []TokenAnalysisClientFamilyRow
	familyUpdates   []TokenAnalysisClientFamilyRow
	familyListCalls int
	// 候选边: candidateQueries 记录查询, edgeWrites 按子请求记录最后一次写回, rematchRows 是待重新匹配的行。
	candidateQueries []TokenAnalysisPrefixCandidateQuery
	edgeWrites       map[string]TokenAnalysisPrefixEdgeWrite
	rematchRows      []TokenAnalysisCandidateRematchRow
	// 分类: clsRows 是全部行(按 id 排序后作待分类来源), clsEdges 是边表; clsWrites
	// 记录写回, clsRowLoads/clsEdgeLoads 记录每次加载的 id, staleMarks 记录维护扫描区间。
	clsRows      []TokenAnalysisClassificationRow
	clsEdges     []TokenAnalysisStoredEdge
	clsWrites    []TokenAnalysisClassificationResult
	clsRowLoads  [][]string
	clsEdgeLoads [][]string
	clsMaxID     int64
	staleMarks   [][2]int64
	drainCalls   int
}

func (r *tokenAnalysisRepoStub) ListClassificationDue(ctx context.Context, afterID int64, limit int) ([]TokenAnalysisClassificationRow, error) {
	var out []TokenAnalysisClassificationRow
	for _, row := range r.clsRows {
		if row.ID > afterID && len(out) < limit {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *tokenAnalysisRepoStub) LoadClassificationRows(ctx context.Context, archiveIDs []string) ([]TokenAnalysisClassificationRow, error) {
	r.clsRowLoads = append(r.clsRowLoads, archiveIDs)
	var out []TokenAnalysisClassificationRow
	for _, id := range archiveIDs {
		for _, row := range r.clsRows {
			if row.ArchiveID == id {
				out = append(out, row)
			}
		}
	}
	return out, nil
}

func (r *tokenAnalysisRepoStub) LoadPrefixEdges(ctx context.Context, childArchiveIDs []string) ([]TokenAnalysisStoredEdge, error) {
	r.clsEdgeLoads = append(r.clsEdgeLoads, childArchiveIDs)
	var out []TokenAnalysisStoredEdge
	for _, id := range childArchiveIDs {
		for _, e := range r.clsEdges {
			if e.ChildArchiveID == id {
				out = append(out, e)
			}
		}
	}
	return out, nil
}

func (r *tokenAnalysisRepoStub) WriteClassifications(ctx context.Context, results []TokenAnalysisClassificationResult) (int, error) {
	r.clsWrites = append(r.clsWrites, results...)
	return len(results), nil
}

func (r *tokenAnalysisRepoStub) MaxSummaryID(ctx context.Context) (int64, error) {
	return r.clsMaxID, nil
}

func (r *tokenAnalysisRepoStub) MarkClassificationStale(ctx context.Context, ruleVersion int32, fromID, toID int64) (int64, error) {
	r.staleMarks = append(r.staleMarks, [2]int64{fromID, toID})
	return 0, nil
}

func (r *tokenAnalysisRepoStub) ListStaleClientFamilies(ctx context.Context, version int32, afterID int64, limit int) ([]TokenAnalysisClientFamilyRow, error) {
	r.familyListCalls++
	var out []TokenAnalysisClientFamilyRow
	for _, row := range r.staleFamilies {
		if row.ID > afterID && row.Version != 0 && row.Version != version && len(out) < limit {
			out = append(out, row)
		}
	}
	return out, nil
}

// UpdateClientFamilies 仿仓储的快照条件: 只有 UA 与版本仍等于读出值的行被写回。
func (r *tokenAnalysisRepoStub) UpdateClientFamilies(ctx context.Context, version int32, rows []TokenAnalysisClientFamilyRow) error {
	for _, row := range rows {
		for i := range r.staleFamilies {
			current := &r.staleFamilies[i]
			if current.ID == row.ID && current.UserAgent == row.UserAgent && current.Version == row.Version {
				current.Family, current.Version = row.Family, version
				r.familyUpdates = append(r.familyUpdates, row)
			}
		}
	}
	return nil
}

// UpsertRequestSummary 记录写入; 可比较的指纹视为新行, 候选版本记为 1、待计算。
func (r *tokenAnalysisRepoStub) UpsertRequestSummary(ctx context.Context, summary *TokenAnalysisRequestSummary) (TokenAnalysisSummaryUpsert, error) {
	if summary == nil {
		return TokenAnalysisSummaryUpsert{}, nil
	}
	r.upserts = append(r.upserts, *summary)
	if !summary.Fingerprint.Comparable() {
		return TokenAnalysisSummaryUpsert{}, nil
	}
	return TokenAnalysisSummaryUpsert{CandInputVersion: 1, CandidatesDue: true}, nil
}

// FindPrefixCandidates 仿仓储: 在已写入的行中按 Key、指纹版本、时间窗口与哈希筛选。
func (r *tokenAnalysisRepoStub) FindPrefixCandidates(ctx context.Context, query TokenAnalysisPrefixCandidateQuery) ([]TokenAnalysisPrefixCandidate, error) {
	r.candidateQueries = append(r.candidateQueries, query)
	contains := func(list [][]byte, h []byte) bool {
		for _, x := range list {
			if len(h) > 0 && string(x) == string(h) {
				return true
			}
		}
		return false
	}
	var out []TokenAnalysisPrefixCandidate
	for _, s := range r.upserts {
		fp := s.Fingerprint
		if !fp.Comparable() || s.ArchiveID == query.ArchiveID || s.APIKeyID == nil || *s.APIKeyID != query.APIKeyID ||
			fp.Version != query.FPVersion || s.EventTime.Before(query.From) || !s.EventTime.Before(query.To) {
			continue
		}
		if contains(query.PrefixHashes, fp.PrefixEndHash) || contains(query.PrefixHashes, fp.PrefixTail1Hash) || contains(query.ItemsHashes, fp.ItemsEndHash) {
			out = append(out, TokenAnalysisPrefixCandidate{
				ArchiveID: s.ArchiveID, EventTime: s.EventTime, PrefixItems: fp.PrefixItems,
				PrefixEndHash: fp.PrefixEndHash, PrefixTail1Hash: fp.PrefixTail1Hash, ItemsEndHash: fp.ItemsEndHash,
			})
		}
	}
	return out, nil
}

// ReplacePrefixEdges 仿仓储: 带 Query 时先查找候选再由 Select 选边, 记录最终写入。
func (r *tokenAnalysisRepoStub) ReplacePrefixEdges(ctx context.Context, write TokenAnalysisPrefixEdgeWrite) (bool, error) {
	if r.edgeWrites == nil {
		r.edgeWrites = map[string]TokenAnalysisPrefixEdgeWrite{}
	}
	if write.Query != nil {
		candidates, err := r.FindPrefixCandidates(ctx, *write.Query)
		if err != nil {
			return false, err
		}
		write.Edges, write.Overflow = write.Select(candidates)
		if write.Overflow {
			write.State = TokenAnalysisCandidateStateTieOverflow
		}
	}
	r.edgeWrites[write.ArchiveID] = write
	return true, nil
}

func (r *tokenAnalysisRepoStub) DrainClassificationInvalidations(ctx context.Context, limit int) (int, error) {
	r.drainCalls++
	return 0, nil
}

func (r *tokenAnalysisRepoStub) ListCandidateRematch(ctx context.Context, limit int) ([]TokenAnalysisCandidateRematchRow, error) {
	if len(r.rematchRows) > limit {
		return r.rematchRows[:limit], nil
	}
	return r.rematchRows, nil
}

func testTokenAnalysisUsageMatch() *TokenAnalysisUsageMatch {
	return &TokenAnalysisUsageMatch{
		UsageLogID:          123,
		MatchConfidence:     3,
		Method:              TokenAnalysisUsageMatchRequestID,
		InputTokens:         1000,
		OutputTokens:        20,
		CacheReadTokens:     100,
		CacheCreationTokens: 50,
		TotalCost:           0.1,
		ActualCost:          0.2,
	}
}

func (r *tokenAnalysisRepoStub) RecordResponse(ctx context.Context, facts TokenAnalysisResponseFacts) error {
	r.responses = append(r.responses, facts)
	return nil
}

func (r *tokenAnalysisRepoStub) ListUsageLinkCandidates(ctx context.Context, start, end time.Time, afterID int64, limit int) ([]TokenAnalysisUsageLinkCandidate, error) {
	all := r.linkCandidates
	if all == nil {
		for i, summary := range r.upserts {
			if tokenAnalysisIsCountTokensPath(summary.Endpoint) || summary.EventTime.Before(start) || !summary.EventTime.Before(end) {
				continue
			}
			c := TokenAnalysisUsageLinkCandidate{
				ID: int64(i + 1), ArchiveID: summary.ArchiveID, ClientRequestID: summary.ClientRequestID,
				EventTime: summary.EventTime, UserID: summary.UserID, APIKeyID: summary.APIKeyID, Model: summary.Model,
			}
			for _, facts := range r.responses {
				if facts.ArchiveID == summary.ArchiveID {
					completedAt, status := facts.CompletedAt, facts.Status
					c.CompletedAt, c.OutputTokens, c.ResponseID, c.ResponseStatus = &completedAt, facts.OutputTokens, facts.ResponseID, &status
				}
			}
			all = append(all, c)
		}
	}
	out := make([]TokenAnalysisUsageLinkCandidate, 0, len(all))
	for _, c := range all {
		if link, ok := r.links[c.ArchiveID]; c.ID > afterID && !(ok && link != nil && link.Method != "") {
			if ok {
				c.UsageLogID, c.Method, c.LinkPending = nil, "", false
				if link != nil {
					c.UsageLogID = &link.UsageLogID
				}
			}
			for _, cleared := range r.pendingCleared {
				if cleared == c.ArchiveID {
					c.LinkPending = false
				}
			}
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *tokenAnalysisRepoStub) FindUsageLink(ctx context.Context, query TokenAnalysisUsageLinkQuery) (*TokenAnalysisUsageMatch, error) {
	r.usageLookups = append(r.usageLookups, query)
	if r.findUsage != nil {
		return r.findUsage(query), nil
	}
	return testTokenAnalysisUsageMatch(), nil
}

func (r *tokenAnalysisRepoStub) SetUsageLink(ctx context.Context, archiveID string, expected TokenAnalysisUsageLinkState, match *TokenAnalysisUsageMatch, riskScore int, riskReasons []TokenAnalysisRiskReason) (TokenAnalysisUsageLinkWrite, error) {
	if r.links == nil {
		r.links, r.linkRisk = map[string]*TokenAnalysisUsageMatch{}, map[string][]TokenAnalysisRiskReason{}
	}
	r.links[archiveID] = match
	r.linkRisk[archiveID] = riskReasons
	if expected.KeepPending {
		r.keptPending = append(r.keptPending, archiveID)
	}
	return TokenAnalysisUsageLinkWrite{Applied: true}, nil
}

func (r *tokenAnalysisRepoStub) ClearUsageLinkPending(ctx context.Context, archiveID string, version int64) error {
	r.pendingCleared = append(r.pendingCleared, archiveID)
	return nil
}

func (r *tokenAnalysisRepoStub) MergePendingResponses(ctx context.Context) error {
	r.pendingMerges++
	return nil
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
	r.requestFilters = &filters
	return append([]TokenAnalysisRequestItem{}, r.requestItems...), &pagination.PaginationResult{}, nil
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

func (r *tokenAnalysisRepoStub) DeleteUserInput(ctx context.Context, archiveID string) error {
	kept := r.userInputs[:0]
	for _, input := range r.userInputs {
		if input.ArchiveID != archiveID {
			kept = append(kept, input)
		}
	}
	r.userInputs = kept
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

func (r *cancelOnUpsertRepoStub) UpsertRequestSummary(ctx context.Context, summary *TokenAnalysisRequestSummary) (TokenAnalysisSummaryUpsert, error) {
	upserted, err := r.tokenAnalysisRepoStub.UpsertRequestSummary(ctx, summary)
	if err != nil {
		return upserted, err
	}
	if r.cancel != nil {
		r.cancel()
	}
	return upserted, nil
}

func TestTokenAnalysisIndexerStopsScanningSkippedLinesAfterCancel(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "2026-06-08.jsonl")
	// 1 条 request + 大量 response 行(只写回完成事实, 不计入索引行):
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
	// count_tokens 探测不计费, 不得做用量关联(否则与紧邻真实请求关联到同一条
	// usage_logs, 概览把 token 累加两次); endpoint 还原 /count_tokens 后缀,
	// 关联轮次据此排除(SQL 条件见仓储测试), 库内才能与真实请求区分。
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
	require.Empty(t, repo.usageLookups)
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

func TestTokenAnalysisIndexerNULInEveryFieldStillIndexesClean(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "2026-06-11.jsonl")
	// 生产事故形态: upsert token analysis request summary: pq: invalid byte
	// sequence 0x00。把 NUL 塞进归档行全部用户可控字段, 断言 summary upsert
	// 的每个 text/jsonb 参数都干净且不计失败行(任一字段漏剥该行即永久失败)。
	bodyObj := map[string]any{
		"model":  "claude-sonnet\x00-x",
		"system": "Primary working directory: E:/code/lag\x00killer\nCurrent branch: feat\x00ure\n",
		"messages": []map[string]any{
			{"role": "user", "content": "ask\x00 something"},
		},
	}
	bodyJSON, err := json.Marshal(bodyObj)
	require.NoError(t, err)
	event := map[string]any{
		"archive_id": "nul\x00all", "event": "request", "timestamp": "2026-06-11T01:02:03Z",
		"method": "POST\x00", "endpoint": "/v1/messages\x00", "path": "/v1/messages\x00",
		"user_id": 7, "api_key_id": 9, "model": "claude\x00", "user_agent": "claude-cli/1.0",
		"body": string(bodyJSON), "body_size": len(bodyJSON), "body_sha256": "hash\x00nul",
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

	result, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{StartDate: "2026-06-11", EndDate: "2026-06-11"})

	require.NoError(t, err)
	require.Equal(t, int64(1), result.IndexedRows)
	require.Equal(t, int64(0), result.FailedRows)
	require.Len(t, repo.upserts, 1)
	got := repo.upserts[0]
	for name, v := range map[string]string{
		"archive_id": got.ArchiveID, "model": got.Model, "endpoint": got.Endpoint,
		"method": got.Method, "body_sha256": got.BodySHA256, "preview": got.LastUserPreview,
		"workdir": got.ClientWorkdir, "project": got.ClientProject, "branch": got.ClientBranch,
		"attribution_source": got.AttributionSource, "source_file": got.SourceFile,
	} {
		require.NotContains(t, v, "\x00", "field %s", name)
	}
	summaryJSON, err := json.Marshal(got.SummaryJSON)
	require.NoError(t, err)
	// jsonb 列拒收 u0000 转义(unsupported Unicode escape sequence),
	// marshal 后的摘要里也不允许出现。
	require.NotContains(t, string(summaryJSON), `\u`+"0000")
	require.Equal(t, "claude-sonnet-x", got.Model)
	require.Equal(t, "/v1/messages", got.Endpoint)
	// 带 NUL 的 cwd 是坏数据, 整个拒绝而非剥后入库; branch 剥 NUL 保留。
	require.Empty(t, got.ClientWorkdir)
	require.Equal(t, "feature", got.ClientBranch)
	require.Len(t, repo.userInputs, 1)
	require.NotContains(t, repo.userInputs[0].Content, "\x00")
}

func TestTokenAnalysisServiceGetUserInputNotFound(t *testing.T) {
	repo := &tokenAnalysisRepoStub{}
	svc := NewTokenAnalysisService(repo, &config.Config{}, nil)

	_, err := svc.GetUserInput(context.Background(), "missing")

	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

// indexTokenAnalysisTestLine indexes one archive request line through the real
// indexArchiveLine and returns the stub; fields override the line defaults.
func indexTokenAnalysisTestLine(t *testing.T, cfg *config.Config, fields map[string]any, seed ...TokenAnalysisUserInput) *tokenAnalysisRepoStub {
	t.Helper()
	repo := &tokenAnalysisRepoStub{userInputs: seed}
	line := map[string]any{"archive_id": "test-line", "event": "request", "timestamp": "2026-10-23T01:02:03Z"}
	for key, value := range fields {
		line[key] = value
	}
	raw, err := json.Marshal(line)
	require.NoError(t, err)
	indexed, _, failed, _, err := NewTokenAnalysisService(repo, cfg, nil).indexArchiveLine(context.Background(), "test.jsonl", int64(len(raw)), raw, false, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, indexed)
	require.Zero(t, failed)
	require.Len(t, repo.upserts, 1)
	return repo
}

// 回填时请求已确认没有用户正文(只有 tool_result): 旧版本误存的工具输出必须失效;
// 采集关闭、归档截断或降级解析不能证明"没有正文", 不得误删历史留存。
func TestTokenAnalysisIndexerReindexClearsStaleUserInputOnlyWhenConfirmedEmpty(t *testing.T) {
	const toolOnlyBody = `{"model":"claude-sonnet-4-5","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Read","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"tool output"}]}]}`
	tests := []struct {
		name      string
		maxChars  int
		truncated bool
		body      string
		wantKept  bool
	}{
		{"confirmed empty clears stale input", 8000, false, toolOnlyBody, false},
		{"truncated archive keeps history", 8000, true, toolOnlyBody, true},
		{"degraded multipart keeps history", 8000, false, "--b\r\n--b--\r\n", true},
		{"input store disabled keeps history", 0, false, toolOnlyBody, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := indexTokenAnalysisTestLine(t,
				&config.Config{TokenAnalysis: config.TokenAnalysisConfig{InputStoreMaxChars: tt.maxChars}},
				map[string]any{"archive_id": "reindex-1", "endpoint": "/v1/messages", "body": tt.body, "body_truncated": tt.truncated},
				TokenAnalysisUserInput{ArchiveID: "reindex-1", Content: "tool output", ContentSHA256: "stale"},
			)
			require.Empty(t, repo.upserts[0].LastUserPreview)
			got, err := repo.GetUserInput(context.Background(), "reindex-1")
			require.NoError(t, err)
			if tt.wantKept {
				require.NotNil(t, got)
				require.Equal(t, "tool output", got.Content)
			} else {
				require.Nil(t, got)
			}
		})
	}
}
