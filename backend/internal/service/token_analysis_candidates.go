package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// 候选边: 子请求的候选父请求, 见风险分类设计 5.4-5.6。K 与 L 首版为代码常量, 改动时
// 需同时规划已有行的重算。
const (
	// TokenAnalysisCandidateLookback 是查找父请求的回看窗口 L。
	TokenAnalysisCandidateLookback = 24 * time.Hour
	// TokenAnalysisCandidateMaxTies 是每组并列候选的上限 K。
	TokenAnalysisCandidateMaxTies = 8
	// tokenAnalysisCandidateRematchBatch 是每轮自动索引重新匹配的行数上限。
	tokenAnalysisCandidateRematchBatch = 500
	// tokenAnalysisArchiveMaxLineBytes 是读回归档行的上限: 正文写入归档行时按 JSON 字符串
	// 转义, 最坏每字节变 6 字节(如 `<` 变 `\u003c`), 按可配置的最大正文上限推出, 另加请求头等
	// 元数据余量。只用于防止读到损坏的位置, 不限制合法的未截断请求。
	tokenAnalysisArchiveMaxLineBytes  = 6*maxRequestArchiveBodyBytes + 8<<20
	tokenAnalysisArchiveBackwardChunk = 1 << 20
)

type tokenAnalysisScoredCandidate struct {
	candidate TokenAnalysisPrefixCandidate
	matched   int
}

// selectTokenAnalysisPrefixEdges 从候选中选边: 完整前缀与去尾前缀各取 matched_items
// 最大的一组并列者; 去尾前缀只在比完整前缀更长时保留。任一组超过 maxTies 时
// overflow, 不截取后冒充完整集合。关联边只作线索, 取最长一组并截断到 maxTies。
// 候选的哈希必须出现在子请求链中与其条目数一致的位置, 否则视为不匹配(防碰撞)。
func selectTokenAnalysisPrefixEdges(chains *tokenAnalysisFingerprintChains, candidates []TokenAnalysisPrefixCandidate, maxTies int) ([]TokenAnalysisPrefixEdge, bool) {
	if chains == nil {
		return nil, false
	}
	prefixPos := tokenAnalysisChainPositions(chains.prefix)
	itemsPos := tokenAnalysisChainPositions(chains.items)
	var full, tail, items []tokenAnalysisScoredCandidate
	for _, c := range candidates {
		if c.PrefixItems < 1 {
			continue
		}
		if p, ok := prefixPos[string(c.PrefixEndHash)]; ok && p == c.PrefixItems {
			full = append(full, tokenAnalysisScoredCandidate{c, p})
			continue
		}
		if len(c.PrefixTail1Hash) > 0 {
			if p, ok := prefixPos[string(c.PrefixTail1Hash)]; ok && p == c.PrefixItems-1 {
				tail = append(tail, tokenAnalysisScoredCandidate{c, p})
				continue
			}
		}
		if p, ok := itemsPos[string(c.ItemsEndHash)]; ok && p == c.PrefixItems {
			items = append(items, tokenAnalysisScoredCandidate{c, p})
		}
	}

	var edges []TokenAnalysisPrefixEdge
	overflow := false
	appendGroup := func(group []tokenAnalysisScoredCandidate, kind string, flagOverflow bool) {
		if len(group) > maxTies {
			if flagOverflow {
				overflow = true
			}
			group = group[:maxTies]
		}
		for _, g := range group {
			edges = append(edges, TokenAnalysisPrefixEdge{ParentArchiveID: g.candidate.ArchiveID, Kind: kind, MatchedItems: g.matched})
		}
	}
	bestFull := tokenAnalysisTopCandidates(full)
	bestTail := tokenAnalysisTopCandidates(tail)
	appendGroup(bestFull, TokenAnalysisEdgeFullPrefix, true)
	if len(bestTail) > 0 && (len(bestFull) == 0 || bestTail[0].matched > bestFull[0].matched) {
		appendGroup(bestTail, TokenAnalysisEdgePrefixMinusTail, true)
	}
	appendGroup(tokenAnalysisTopCandidates(items), TokenAnalysisEdgeItemsLink, false)
	return edges, overflow
}

func tokenAnalysisChainPositions(chain [][]byte) map[string]int {
	positions := make(map[string]int, len(chain))
	for i, h := range chain {
		if _, exists := positions[string(h)]; !exists {
			positions[string(h)] = i + 1
		}
	}
	return positions
}

// tokenAnalysisTopCandidates 取 matched 最大的一组, 按时间倒序、archive_id 升序确定顺序。
func tokenAnalysisTopCandidates(list []tokenAnalysisScoredCandidate) []tokenAnalysisScoredCandidate {
	best := 0
	for _, c := range list {
		if c.matched > best {
			best = c.matched
		}
	}
	var top []tokenAnalysisScoredCandidate
	for _, c := range list {
		if c.matched == best {
			top = append(top, c)
		}
	}
	sort.Slice(top, func(i, j int) bool {
		if !top[i].candidate.EventTime.Equal(top[j].candidate.EventTime) {
			return top[i].candidate.EventTime.After(top[j].candidate.EventTime)
		}
		return top[i].candidate.ArchiveID < top[j].candidate.ArchiveID
	})
	return top
}

// computeTokenAnalysisCandidates 用内存中的完整链查找候选并按 expected 版本条件写回;
// 版本已变时结果被丢弃, 由重新匹配任务再算。
func (s *TokenAnalysisService) computeTokenAnalysisCandidates(ctx context.Context, archiveID string, apiKeyID *int64, eventTime time.Time, fp TokenAnalysisFingerprint, chains *tokenAnalysisFingerprintChains, expected int64) error {
	write := TokenAnalysisPrefixEdgeWrite{ArchiveID: archiveID, ExpectedCandVersion: expected, State: TokenAnalysisCandidateStateOK}
	switch {
	case !fp.Comparable() || chains == nil:
		write.State = fp.State
	case apiKeyID == nil:
		write.State = TokenAnalysisCandidateStateNoAPIKey
	default:
		// 查找在仓储的候选锁内进行, 与并发写入的新父请求串行(见 Query 的说明)。
		write.Query = &TokenAnalysisPrefixCandidateQuery{
			ArchiveID: archiveID, APIKeyID: *apiKeyID, FPVersion: fp.Version,
			From: eventTime.Add(-TokenAnalysisCandidateLookback), To: eventTime,
			PrefixHashes: chains.prefix, ItemsHashes: chains.items,
		}
		write.Select = func(candidates []TokenAnalysisPrefixCandidate) ([]TokenAnalysisPrefixEdge, bool) {
			return selectTokenAnalysisPrefixEdges(chains, candidates, TokenAnalysisCandidateMaxTies)
		}
	}
	_, err := s.repo.ReplacePrefixEdges(ctx, write)
	return err
}

// rematchTokenAnalysisCandidates 处理一批候选快照过期的行: 按归档位置重读原文, 重新
// 计算链与候选边。原文不可用时记为 unknown:body_unavailable。返回处理行数。
func (s *TokenAnalysisService) rematchTokenAnalysisCandidates(ctx context.Context, limit int) (int, error) {
	rows, err := s.repo.ListCandidateRematch(ctx, limit)
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	dir := s.archiveDir(ctx)
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if err := s.rematchTokenAnalysisCandidateRow(ctx, dir, row); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

func (s *TokenAnalysisService) rematchTokenAnalysisCandidateRow(ctx context.Context, dir string, row TokenAnalysisCandidateRematchRow) error {
	unavailable := TokenAnalysisPrefixEdgeWrite{ArchiveID: row.ArchiveID, ExpectedCandVersion: row.CandInputVersion, State: TokenAnalysisCandidateStateBodyUnavailable}
	event, ok := readTokenAnalysisArchiveRequest(dir, row)
	if !ok {
		_, err := s.repo.ReplacePrefixEdges(ctx, unavailable)
		return err
	}
	eventTime, err := time.Parse(time.RFC3339Nano, event.Timestamp)
	if err != nil {
		_, err := s.repo.ReplacePrefixEdges(ctx, unavailable)
		return err
	}
	var root map[string]any
	if !event.BodyTruncated {
		root, _ = parseTokenAnalysisRequestBody([]byte(event.Body))
	}
	rawPath := firstNonEmptyTokenAnalysisString(event.Path, event.Endpoint)
	fp, chains := computeTokenAnalysisFingerprint(tokenAnalysisFingerprintInput{
		Secret:      s.cfg.TokenAnalysis.FingerprintSecret,
		Endpoint:    firstNonEmptyTokenAnalysisString(event.Endpoint, event.Path),
		Root:        root,
		Truncated:   event.BodyTruncated,
		Degraded:    root == nil,
		CountTokens: tokenAnalysisIsCountTokensPath(rawPath),
	})
	if !tokenAnalysisFingerprintMatchesStored(fp, row) {
		// 重读的正文按当前规则/密钥算出的指纹与存下的不同(规则或密钥已变, 或原文漂移):
		// 不能用新链为旧快照确认候选。
		_, err := s.repo.ReplacePrefixEdges(ctx, unavailable)
		return err
	}
	return s.computeTokenAnalysisCandidates(ctx, row.ArchiveID, event.APIKeyID, eventTime, fp, chains, row.CandInputVersion)
}

func tokenAnalysisFingerprintMatchesStored(fp TokenAnalysisFingerprint, row TokenAnalysisCandidateRematchRow) bool {
	return fp.Comparable() && fp.Version == row.FPVersion &&
		bytes.Equal(fp.PrefixEndHash, row.PrefixEndHash) &&
		bytes.Equal(fp.PrefixTail1Hash, row.PrefixTail1Hash) &&
		bytes.Equal(fp.ItemsEndHash, row.ItemsEndHash)
}

// readTokenAnalysisArchiveRequest 按归档位置读回请求行并校验 event/archive_id, 并对读到的
// 完整正文重算 SHA-256 与摘要存下的值比较(不信任行内的 body_sha256 字段); 截断行不可重读。
// 新行用行首与长度直接读取; 旧行只有行尾之后的位置, 向前有界查找上一个换行符。
// 文件不存在、已压缩、已替换或校验失败时返回 false。
func readTokenAnalysisArchiveRequest(dir string, row TokenAnalysisCandidateRematchRow) (tokenAnalysisArchiveEvent, bool) {
	name := filepath.Base(strings.TrimSpace(row.SourceFile))
	if name == "" || name == "." || strings.HasSuffix(name, ".gz") {
		return tokenAnalysisArchiveEvent{}, false
	}
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return tokenAnalysisArchiveEvent{}, false
	}
	defer func() { _ = f.Close() }()

	var line []byte
	switch {
	case row.SourceLineStart != nil && row.SourceLineLength != nil && *row.SourceLineLength > 0 && *row.SourceLineLength <= tokenAnalysisArchiveMaxLineBytes:
		line = make([]byte, *row.SourceLineLength)
		if _, err := f.ReadAt(line, *row.SourceLineStart); err != nil {
			return tokenAnalysisArchiveEvent{}, false
		}
	case row.SourceOffset != nil:
		line, err = readTokenAnalysisLineEndingAt(f, *row.SourceOffset)
		if err != nil {
			return tokenAnalysisArchiveEvent{}, false
		}
	default:
		return tokenAnalysisArchiveEvent{}, false
	}
	var event tokenAnalysisArchiveEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return tokenAnalysisArchiveEvent{}, false
	}
	sum := sha256.Sum256([]byte(event.Body))
	if event.Event != "request" || event.BodyTruncated ||
		sanitizeTokenAnalysisText(event.ArchiveID) != row.ArchiveID ||
		hex.EncodeToString(sum[:]) != row.BodySHA256 {
		return tokenAnalysisArchiveEvent{}, false
	}
	return event, true
}

// readTokenAnalysisLineEndingAt 读取结束于 end(行尾之后的位置)的那一行: 先跳过末尾的
// 换行符(末行可能没有), 再向前查找上一个换行符, 上限 tokenAnalysisArchiveMaxLineBytes。
func readTokenAnalysisLineEndingAt(f *os.File, end int64) ([]byte, error) {
	if end <= 0 {
		return nil, errors.New("empty line position")
	}
	contentEnd := end
	last := make([]byte, 1)
	if _, err := f.ReadAt(last, end-1); err != nil {
		return nil, err
	}
	if last[0] == '\n' {
		contentEnd = end - 1
	}
	start := int64(0)
	chunk := make([]byte, tokenAnalysisArchiveBackwardChunk)
	for pos := contentEnd; pos > 0; {
		if contentEnd-pos > tokenAnalysisArchiveMaxLineBytes {
			return nil, errors.New("line exceeds archive line limit")
		}
		size := int64(len(chunk))
		if pos < size {
			size = pos
		}
		buf := chunk[:size]
		if _, err := f.ReadAt(buf, pos-size); err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		if i := bytes.LastIndexByte(buf, '\n'); i >= 0 {
			start = pos - size + int64(i) + 1
			break
		}
		pos -= size
	}
	line := make([]byte, contentEnd-start)
	if _, err := f.ReadAt(line, start); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return line, nil
}

// logTokenAnalysisCandidateError 记录候选计算失败; 行保持待重新匹配状态, 不影响摘要入库。
func logTokenAnalysisCandidateError(archiveID string, err error) {
	if err != nil {
		log.Printf("[TokenAnalysis] compute candidates failed archive_id=%s: %v", archiveID, err)
	}
}
