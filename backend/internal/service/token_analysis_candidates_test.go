package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func testTokenAnalysisChains(n int) *tokenAnalysisFingerprintChains {
	chains := &tokenAnalysisFingerprintChains{}
	for i := 0; i < n; i++ {
		chains.prefix = append(chains.prefix, []byte(fmt.Sprintf("p%02d", i+1)))
		chains.items = append(chains.items, []byte(fmt.Sprintf("g%02d", i+1)))
	}
	return chains
}

func TestSelectTokenAnalysisPrefixEdges(t *testing.T) {
	chains := testTokenAnalysisChains(10)
	at := time.Date(2026, 10, 27, 1, 0, 0, 0, time.UTC)
	cand := func(id string, items int, end, tail, itemsEnd string, minute int) TokenAnalysisPrefixCandidate {
		c := TokenAnalysisPrefixCandidate{ArchiveID: id, PrefixItems: items, EventTime: at.Add(time.Duration(minute) * time.Minute)}
		if end != "" {
			c.PrefixEndHash = []byte(end)
		}
		if tail != "" {
			c.PrefixTail1Hash = []byte(tail)
		}
		if itemsEnd != "" {
			c.ItemsEndHash = []byte(itemsEnd)
		}
		return c
	}

	// 完整前缀取最长一组; 更短的祖先不入选; 位置与条目数不符(碰撞)的候选忽略。
	edges, overflow := selectTokenAnalysisPrefixEdges(chains, []TokenAnalysisPrefixCandidate{
		cand("p6", 6, "p06", "p05", "g06", 1),
		cand("p8a", 8, "p08", "p07", "g08", 2),
		cand("p8b", 8, "p08", "p07", "g08", 3),
		cand("collision", 5, "p08", "", "", 4),
	}, 8)
	require.False(t, overflow)
	require.Equal(t, []TokenAnalysisPrefixEdge{
		{ParentArchiveID: "p8b", Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 8},
		{ParentArchiveID: "p8a", Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 8},
	}, edges, "ties ordered by event_time desc")

	// 去尾前缀只在比完整前缀更长时保留(Claude Code 末尾临时消息被替换)。
	edges, _ = selectTokenAnalysisPrefixEdges(chains, []TokenAnalysisPrefixCandidate{
		cand("full4", 4, "p04", "p03", "", 1),
		cand("tail9", 10, "zz", "p09", "", 2),
	}, 8)
	require.Equal(t, []TokenAnalysisPrefixEdge{
		{ParentArchiveID: "full4", Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 4},
		{ParentArchiveID: "tail9", Kind: TokenAnalysisEdgePrefixMinusTail, MatchedItems: 9},
	}, edges)
	edges, _ = selectTokenAnalysisPrefixEdges(chains, []TokenAnalysisPrefixCandidate{
		cand("full9", 9, "p09", "p08", "", 1),
		cand("tail9", 10, "zz", "p09", "", 2),
	}, 8)
	require.Equal(t, []TokenAnalysisPrefixEdge{{ParentArchiveID: "full9", Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 9}}, edges)

	// 只有关联链相同(system/tools 变化): items_link。
	edges, _ = selectTokenAnalysisPrefixEdges(chains, []TokenAnalysisPrefixCandidate{cand("link", 3, "x", "y", "g03", 1)}, 8)
	require.Equal(t, []TokenAnalysisPrefixEdge{{ParentArchiveID: "link", Kind: TokenAnalysisEdgeItemsLink, MatchedItems: 3}}, edges)

	// 并列超过 K: overflow, 只写前 K 个, 不冒充完整集合。
	var many []TokenAnalysisPrefixCandidate
	for i := 0; i < 3; i++ {
		many = append(many, cand(fmt.Sprintf("dup%d", i), 2, "p02", "p01", "", i))
	}
	edges, overflow = selectTokenAnalysisPrefixEdges(chains, many, 2)
	require.True(t, overflow)
	require.Len(t, edges, 2)
}

func writeTokenAnalysisArchiveFile(t *testing.T, dir, day string, lines ...string) string {
	t.Helper()
	path := filepath.Join(dir, day+".jsonl")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	return path
}

func tokenAnalysisArchiveRequestLine(t *testing.T, archiveID, ts, endpoint string, apiKeyID int64, body string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"archive_id": archiveID, "event": "request", "timestamp": ts, "method": "POST", "endpoint": endpoint,
		"user_id": 7, "api_key_id": apiKeyID, "body": body, "body_size": len(body), "body_sha256": testTokenAnalysisSHA256(body),
	})
	require.NoError(t, err)
	return string(raw)
}

func testTokenAnalysisSHA256(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

func newTokenAnalysisFingerprintTestService(repo TokenAnalysisRepository, dir string) *TokenAnalysisService {
	cfg := &config.Config{
		Gateway:       config.GatewayConfig{RequestArchive: config.GatewayRequestArchiveConfig{Dir: dir}},
		TokenAnalysis: config.TokenAnalysisConfig{IndexEnabled: true, IndexBatchSize: 1000, MaxPreviewChars: 300, UsageMatchWindowSeconds: 10},
	}
	cfg.TokenAnalysis.FingerprintSecret = testTokenAnalysisFingerprintSecret
	return NewTokenAnalysisService(repo, cfg, nil)
}

// 端到端: 索引时写入指纹与行位置, 并用内存中的链立即计算候选边。Responses 正常追加得到
// full_prefix; Claude Code 式末尾临时消息被替换得到 prefix_minus_tail; 不同 Key 互不匹配。
func TestTokenAnalysisIndexerComputesCandidateEdges(t *testing.T) {
	dir := t.TempDir()
	turn := func(n int, tail string) string {
		var items []string
		for i := 1; i <= n; i++ {
			items = append(items, fmt.Sprintf(`{"role":"user","content":"m%d"}`, i))
		}
		if tail != "" {
			items = append(items, `{"role":"system","content":"`+tail+`"}`)
		}
		return `{"model":"gpt","instructions":"sys","input":[` + strings.Join(items, ",") + `]}`
	}
	writeTokenAnalysisArchiveFile(t, dir, "2026-10-27",
		tokenAnalysisArchiveRequestLine(t, "r1", "2026-10-27T01:00:00Z", "/v1/responses", 9, turn(2, "")),
		tokenAnalysisArchiveRequestLine(t, "r2", "2026-10-27T01:00:10Z", "/v1/responses", 9, turn(3, "")),
		tokenAnalysisArchiveRequestLine(t, "c1", "2026-10-27T01:01:00Z", "/v1/responses", 9, turn(4, "reminder-a")),
		tokenAnalysisArchiveRequestLine(t, "c2", "2026-10-27T01:01:10Z", "/v1/responses", 9, turn(5, "reminder-b")),
		tokenAnalysisArchiveRequestLine(t, "other", "2026-10-27T01:02:00Z", "/v1/responses", 10, turn(6, "")),
	)
	repo := &tokenAnalysisRepoStub{}
	svc := newTokenAnalysisFingerprintTestService(repo, dir)

	_, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{StartDate: "2026-10-27", EndDate: "2026-10-27"})
	require.NoError(t, err)
	require.Len(t, repo.upserts, 5)
	first := repo.upserts[0]
	require.True(t, first.Fingerprint.Comparable())
	require.EqualValues(t, 0, *first.SourceLineStart)
	require.Equal(t, *first.SourceOffset, *first.SourceLineStart+*first.SourceLineLength, "source_offset stays the line end")

	require.Empty(t, repo.edgeWrites["r1"].Edges)
	require.Equal(t, TokenAnalysisCandidateStateOK, repo.edgeWrites["r1"].State)
	require.Contains(t, repo.edgeWrites["r2"].Edges, TokenAnalysisPrefixEdge{ParentArchiveID: "r1", Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 2})
	// c1 的父请求是 r2(完整前缀 3 条); c2 与 c1 只差末尾临时消息。
	require.Contains(t, repo.edgeWrites["c1"].Edges, TokenAnalysisPrefixEdge{ParentArchiveID: "r2", Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 3})
	require.Contains(t, repo.edgeWrites["c2"].Edges, TokenAnalysisPrefixEdge{ParentArchiveID: "c1", Kind: TokenAnalysisEdgePrefixMinusTail, MatchedItems: 4})
	require.Empty(t, repo.edgeWrites["other"].Edges, "different API key never matches")
	for _, q := range repo.candidateQueries {
		require.Equal(t, TokenAnalysisCandidateLookback, q.To.Sub(q.From))
	}
}

// 重新匹配: 新行按行首与长度读回, 旧行按行尾之后的位置向前定位; 校验失败或文件
// 已压缩/不存在时记为 body_unavailable。
func TestTokenAnalysisRematchCandidates(t *testing.T) {
	dir := t.TempDir()
	parentLine := tokenAnalysisArchiveRequestLine(t, "p", "2026-10-27T01:00:00Z", "/v1/responses", 9, `{"input":[{"role":"user","content":"a"}]}`)
	childLine := tokenAnalysisArchiveRequestLine(t, "c", "2026-10-27T01:00:05Z", "/v1/responses", 9, `{"input":[{"role":"user","content":"a"},{"role":"user","content":"b"}]}`)
	writeTokenAnalysisArchiveFile(t, dir, "2026-10-27", parentLine, childLine)

	repo := &tokenAnalysisRepoStub{}
	svc := newTokenAnalysisFingerprintTestService(repo, dir)
	_, err := svc.IndexRange(context.Background(), TokenAnalysisIndexRequest{StartDate: "2026-10-27", EndDate: "2026-10-27"})
	require.NoError(t, err)
	child := repo.upserts[1]
	end := *child.SourceOffset
	start, length := *child.SourceLineStart, *child.SourceLineLength
	repo.edgeWrites = nil

	// 重新匹配的行带着摘要存下的正文哈希与指纹。
	stored := func(version int64) TokenAnalysisCandidateRematchRow {
		return TokenAnalysisCandidateRematchRow{
			ArchiveID: "c", CandInputVersion: version, SourceFile: "2026-10-27.jsonl", BodySHA256: child.BodySHA256,
			FPVersion: child.Fingerprint.Version, PrefixEndHash: child.Fingerprint.PrefixEndHash,
			PrefixTail1Hash: child.Fingerprint.PrefixTail1Hash, ItemsEndHash: child.Fingerprint.ItemsEndHash,
		}
	}
	row := stored(2)
	row.SourceLineStart, row.SourceLineLength = &start, &length
	repo.rematchRows = []TokenAnalysisCandidateRematchRow{row}
	n, err := svc.rematchTokenAnalysisCandidates(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, int64(2), repo.edgeWrites["c"].ExpectedCandVersion)
	require.Contains(t, repo.edgeWrites["c"].Edges, TokenAnalysisPrefixEdge{ParentArchiveID: "p", Kind: TokenAnalysisEdgeFullPrefix, MatchedItems: 1})

	// 旧行只有行尾位置。
	row = stored(3)
	row.SourceOffset = &end
	repo.rematchRows = []TokenAnalysisCandidateRematchRow{row}
	_, err = svc.rematchTokenAnalysisCandidates(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, TokenAnalysisCandidateStateOK, repo.edgeWrites["c"].State)
	require.NotEmpty(t, repo.edgeWrites["c"].Edges)

	unavailable := func(mutate func(*TokenAnalysisCandidateRematchRow)) TokenAnalysisCandidateRematchRow {
		row := stored(4)
		row.SourceOffset = &end
		mutate(&row)
		return row
	}
	for name, row := range map[string]TokenAnalysisCandidateRematchRow{
		"hash mismatch": unavailable(func(r *TokenAnalysisCandidateRematchRow) { r.BodySHA256 = "other" }),
		"compressed":    unavailable(func(r *TokenAnalysisCandidateRematchRow) { r.SourceFile = "2026-10-27.jsonl.gz" }),
		"missing file":  unavailable(func(r *TokenAnalysisCandidateRematchRow) { r.SourceFile = "2026-10-26.jsonl" }),
		"no position":   unavailable(func(r *TokenAnalysisCandidateRematchRow) { r.SourceOffset = nil }),
		// 存下的指纹与按当前规则重算的不同(规则/密钥已变): 不能用新链确认旧快照。
		"stored fingerprint differs": unavailable(func(r *TokenAnalysisCandidateRematchRow) { r.PrefixEndHash = []byte("old-prefix-hash!") }),
	} {
		repo.rematchRows = []TokenAnalysisCandidateRematchRow{row}
		_, err = svc.rematchTokenAnalysisCandidates(context.Background(), 10)
		require.NoError(t, err, name)
		require.Equal(t, TokenAnalysisCandidateStateBodyUnavailable, repo.edgeWrites["c"].State, name)
		require.Empty(t, repo.edgeWrites["c"].Edges, name)
	}
}

// 读回校验对正文本身重算哈希: 行内的 body_sha256 字段保持旧值而正文被改, 不能通过。
func TestReadTokenAnalysisArchiveRequestRehashesBody(t *testing.T) {
	dir := t.TempDir()
	original := `{"input":[{"role":"user","content":"A"},{"role":"user","content":"Z"}]}`
	line := tokenAnalysisArchiveRequestLine(t, "c", "2026-10-27T01:00:05Z", "/v1/responses", 9, original)
	tampered := strings.Replace(line, `\"content\":\"A\"`, `\"content\":\"B\"`, 1)
	require.NotEqual(t, line, tampered)
	writeTokenAnalysisArchiveFile(t, dir, "2026-10-27", tampered)
	start, length := int64(0), int64(len(tampered))
	row := TokenAnalysisCandidateRematchRow{ArchiveID: "c", SourceFile: "2026-10-27.jsonl", SourceLineStart: &start, SourceLineLength: &length, BodySHA256: testTokenAnalysisSHA256(original)}
	_, ok := readTokenAnalysisArchiveRequest(dir, row)
	require.False(t, ok)

	writeTokenAnalysisArchiveFile(t, dir, "2026-10-27", line)
	length = int64(len(line))
	event, ok := readTokenAnalysisArchiveRequest(dir, row)
	require.True(t, ok)
	require.Equal(t, original, event.Body)
}

// 读回上限按正文上限与 JSON 转义推出: 合法正文在归档行里最坏膨胀 6 倍。
func TestTokenAnalysisArchiveLineLimitCoversEscapedBody(t *testing.T) {
	body := `{"input":"` + strings.Repeat("<", 1024) + `"}`
	line := tokenAnalysisArchiveRequestLine(t, "c", "2026-10-27T01:00:05Z", "/v1/responses", 9, body)
	require.Greater(t, len(line), 6*1024, "html escaping expands each < to six bytes")
	require.GreaterOrEqual(t, int64(tokenAnalysisArchiveMaxLineBytes), 6*maxRequestArchiveBodyBytes)
}

func TestReadTokenAnalysisLineEndingAt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("first\nsecond\nthird"), 0o600))
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	line, err := readTokenAnalysisLineEndingAt(f, int64(len("first\n")))
	require.NoError(t, err)
	require.Equal(t, "first", string(line))
	line, err = readTokenAnalysisLineEndingAt(f, int64(len("first\nsecond\n")))
	require.NoError(t, err)
	require.Equal(t, "second", string(line))
	line, err = readTokenAnalysisLineEndingAt(f, int64(len("first\nsecond\nthird")))
	require.NoError(t, err)
	require.Equal(t, "third", string(line), "last line without trailing newline")
}
