package service

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// 冷暖观察、原因归类与条件性复用机会金额, 见风险分类设计第 6-8、10 节。分类是输入的
// 纯函数: 同一组事实(子请求、候选边、祖先)总得到同一结果, 写回以版本为条件。
const (
	// TokenAnalysisClassificationRuleVersion 是分类规则版本; 改动规则时加一, 旧版本的
	// 行由维护扫描登记重算。
	TokenAnalysisClassificationRuleVersion int32 = 1
	// TokenAnalysisClassificationMaxDepth 是回溯祖先与传播分类版本的层数上限 D。
	TokenAnalysisClassificationMaxDepth = 64
	// tokenAnalysisClassificationWalkBudget 是每个子请求回溯时访问的节点上限。
	tokenAnalysisClassificationWalkBudget = 256

	tokenAnalysisSmallContextTokens = 50000
	tokenAnalysisCacheHitRatio      = 0.2
	// tokenAnalysisOpenAIReuseWindow 是 OpenAI 原生 Responses 的复用窗口 W, 从父请求开始
	// 时刻起算; 取平台文档明确的最短保留期。
	tokenAnalysisOpenAIReuseWindow  = 300 * time.Second
	tokenAnalysisReusableTokenBlock = 128
	tokenAnalysisReusableTokenMin   = 1024

	tokenAnalysisAmountHashDomain = "sub2api/token-analysis/amount-inputs/v1"
)

// cls_state: 本行分类在当前输入下的状态。pending 状态同样写回(版本对齐), 待相应
// 输入变化经版本传播重新登记。
const (
	TokenAnalysisClsClassified        = "classified"
	TokenAnalysisClsPendingUsageLink  = "pending:usage_link"
	TokenAnalysisClsPendingCandidates = "pending:candidates"
)

// cache_obs_state, 按计费口径。
const (
	TokenAnalysisObsSmall       = "small"
	TokenAnalysisObsHit         = "hit"
	TokenAnalysisObsCold        = "cold"
	TokenAnalysisObsNoUsage     = "no_usage"
	TokenAnalysisObsUnsupported = "unsupported"
)

// cause 只描述观察, 不归责; 只对 cold 行给出。
const (
	TokenAnalysisCauseMissedWithEligibleParent = "missed_with_eligible_parent"
	TokenAnalysisCauseParentUnusable           = "parent_unusable"
	TokenAnalysisCauseParentPending            = "parent_pending"
	TokenAnalysisCauseParentOverlapping        = "parent_overlapping"
	TokenAnalysisCauseModelChanged             = "model_changed"
	TokenAnalysisCauseAccountChanged           = "account_changed"
	TokenAnalysisCauseContinuationRef          = "continuation_ref"
	TokenAnalysisCauseUnsupportedPath          = "unsupported_path"
	TokenAnalysisCauseCacheKeyChanged          = "cache_key_changed"
	// TokenAnalysisCausePromptRewritten: 网关改写了最终发送的条目或顶层字段(如重排触发器、
	// 剥离加密 reasoning), 入口前缀证据不能代表上游实际收到的提示词。
	TokenAnalysisCausePromptRewritten   = "prompt_rewritten"
	TokenAnalysisCauseUsageNotUpstream  = "usage_not_upstream"
	TokenAnalysisCauseBeyondReuseWindow = "beyond_reuse_window"
	TokenAnalysisCausePrefixChanged     = "prefix_changed"
	TokenAnalysisCauseNoCandidate       = "no_candidate"

	tokenAnalysisCacheKeyChangedClient  = "client"
	tokenAnalysisCacheKeyChangedGateway = "gateway"

	tokenAnalysisNoCandidateNoneInWindow   = "none_in_window"
	tokenAnalysisNoCandidateBodyUnavail    = "body_unavailable"
	tokenAnalysisNoCandidateTieOverflow    = "tie_overflow"
	tokenAnalysisNoCandidateFingerprintOff = "fingerprint_unavailable"
)

// 三类证据。
const (
	TokenAnalysisPrefixFull            = "full_prefix"
	TokenAnalysisPrefixMinusTail       = "prefix_minus_tail"
	TokenAnalysisPrefixItemsLink       = "items_link"
	TokenAnalysisPrefixNone            = "none"
	tokenAnalysisEvidenceUnknownPrefix = "unknown:"

	TokenAnalysisEligibleAssumed       = "eligible_assumed"
	TokenAnalysisEligibilityPending    = "pending"
	tokenAnalysisIneligiblePrefix      = "ineligible:"
	tokenAnalysisEligibilityWalkBudget = "unknown:walk_budget"
	tokenAnalysisEligibilityTransform  = "unknown:transform_policy"
	tokenAnalysisEligibilityNoWindow   = "unknown:no_window"

	TokenAnalysisPriceRowExact           = "row_exact"
	tokenAnalysisPriceUnavailablePrefix  = "unavailable:"
	tokenAnalysisPriceNotFullPrefix      = "unavailable:prefix_minus_tail"
	tokenAnalysisPriceOpenAICacheWrite   = "unavailable:openai_cache_write"
	tokenAnalysisPriceImageInput         = "unavailable:image_input"
	tokenAnalysisPriceNoCacheRead        = "unavailable:no_cache_read"
	tokenAnalysisPriceNoInputPrice       = "unavailable:no_input_price"
	tokenAnalysisClassificationTokenMode = "token"
)

// TokenAnalysisClassificationUsage 是摘要行当前关联、且能 JOIN 到的账单(计费口径)。
type TokenAnalysisClassificationUsage struct {
	UsageLogID          int64
	AccountID           int64
	Platform            string
	SentModel           string
	BillingMode         string
	InputTokens         int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	ImageInputTokens    int64
	InputCost           decimal.Decimal
	CacheReadCost       decimal.Decimal
	CacheCreationCost   decimal.Decimal
	ImageInputCost      decimal.Decimal
}

// TokenAnalysisClassificationRow 是分类用到的摘要行事实, 子请求与祖先共用。
type TokenAnalysisClassificationRow struct {
	ID                      int64
	ArchiveID               string
	EventTime               time.Time
	CompletedAt             *time.Time
	ResponseStatus          *int
	Endpoint                string
	UsageLinkVersion        int64
	UsageLinkCheckedVersion int64
	FPVersion               int32
	HasContinuationRef      bool
	CandInputVersion        int64
	CandCheckedVersion      int64
	CandState               string
	CandOverflow            bool
	ForwardPath             string
	UpstreamCacheKeyHash    string
	// UpstreamItemsHash/UpstreamRootHash 是最终请求体的条目链末端与 h0; ItemsEndHash 是
	// 入口(归档)正文的条目链末端。两条目哈希相等表示网关没有改写条目。
	UpstreamItemsHash  string
	UpstreamRootHash   string
	ItemsEndHash       []byte
	UsageCacheSource   string
	ClientCacheKeyHash []byte
	ImageCount         int
	// ClsInputVersion 只随本行自身的分类输入变化(子请求记下并比较它); ClsDepVersion
	// 随依赖传播、规则版本扫描与候选变化标记变化, 只表示需要重算。
	ClsInputVersion int64
	ClsDepVersion   int64
	// Usage 为 nil 表示没有关联账单, 或关联的账单已被删除(外键置空)。
	Usage *TokenAnalysisClassificationUsage
}

// TokenAnalysisStoredEdge 是边表中的一条候选边。
type TokenAnalysisStoredEdge struct {
	ChildArchiveID  string
	ParentArchiveID string
	Kind            string
	MatchedItems    int
}

// TokenAnalysisClassificationResult 是一行的分类结果, 按 ClsInputVersion、ClsDepVersion、
// CandCheckedVersion 与(有父请求时)ParentClsVersion 条件写回。
type TokenAnalysisClassificationResult struct {
	ArchiveID          string
	ClsInputVersion    int64
	ClsDepVersion      int64
	CandCheckedVersion int64
	ParentArchiveID    string
	ParentClsVersion   *int64
	RuleVersion        int32
	ClsState           string
	CacheObsState      string
	Cause              string
	PrefixEvidence     string
	CacheEligibility   string
	PriceEvidence      string
	ReusableTokens     *int64
	RecoverableTokens  *int64
	Amount             *decimal.Decimal
	ColdInputCost      *decimal.Decimal
	AmountInputsHash   []byte
}

// tokenAnalysisClassificationFacts 是一批分类已加载的事实。rows/edges 中键存在即已加载:
// rows 值为 nil 表示该行不存在; edges 值为空表示该行没有候选边。
type tokenAnalysisClassificationFacts struct {
	secret string
	rows   map[string]*TokenAnalysisClassificationRow
	edges  map[string][]TokenAnalysisStoredEdge
}

func newTokenAnalysisClassificationFacts(secret string) *tokenAnalysisClassificationFacts {
	return &tokenAnalysisClassificationFacts{
		secret: secret,
		rows:   map[string]*TokenAnalysisClassificationRow{},
		edges:  map[string][]TokenAnalysisStoredEdge{},
	}
}

// tokenAnalysisClassificationNeed 是分类继续进行所缺的事实。
type tokenAnalysisClassificationNeed struct {
	rows  []string
	edges []string
}

func (n *tokenAnalysisClassificationNeed) empty() bool {
	return n == nil || (len(n.rows) == 0 && len(n.edges) == 0)
}

// tokenAnalysisGateFailure 是父子成对资格判定遇到的第一个不合格项(设计第 7 节)。
type tokenAnalysisGateFailure struct {
	gate        int
	cause       string
	eligibility string
}

// tokenAnalysisWalkNode 是回溯中的祖先: matched 是它与子请求共享的前缀条目数,
// full 表示它的全部条目都是子请求的前缀。
type tokenAnalysisWalkNode struct {
	archiveID string
	matched   int
	full      bool
}

// classify 对子请求 c 分类; 缺少事实时返回 need, 由调用方加载后重试。
func (f *tokenAnalysisClassificationFacts) classify(c *TokenAnalysisClassificationRow) (TokenAnalysisClassificationResult, *tokenAnalysisClassificationNeed) {
	res := TokenAnalysisClassificationResult{
		ArchiveID:          c.ArchiveID,
		ClsInputVersion:    c.ClsInputVersion,
		ClsDepVersion:      c.ClsDepVersion,
		CandCheckedVersion: c.CandCheckedVersion,
		RuleVersion:        TokenAnalysisClassificationRuleVersion,
		ClsState:           TokenAnalysisClsClassified,
	}
	res.CacheObsState = tokenAnalysisObserveCache(c)
	if res.CacheObsState == TokenAnalysisObsCold {
		cost := c.Usage.InputCost.Add(c.Usage.ImageInputCost).Add(c.Usage.CacheCreationCost)
		res.ColdInputCost = &cost
	}
	needsParent := res.CacheObsState == TokenAnalysisObsHit || res.CacheObsState == TokenAnalysisObsCold
	if tokenAnalysisUsageLinkPending(c) || (needsParent && c.CompletedAt == nil) {
		// 账单待复核或缺完成事实(设计第 7 节表, 两者同属关联状态): 冷暖照常给出(页面标
		// "账单待复核"), 资格与金额待确认后重算; 晚到的 response 经版本传播触发重算。
		res.ClsState = TokenAnalysisClsPendingUsageLink
		if needsParent {
			res.CacheEligibility = TokenAnalysisEligibilityPending
		}
		return res, nil
	}
	if !needsParent {
		return res, nil
	}
	if c.CandInputVersion != c.CandCheckedVersion {
		res.ClsState = TokenAnalysisClsPendingCandidates
		return res, nil
	}
	cold := res.CacheObsState == TokenAnalysisObsCold
	setCause := func(cause string) {
		if cold {
			res.Cause = cause
		}
	}
	switch {
	case c.CandOverflow || c.CandState == TokenAnalysisCandidateStateTieOverflow:
		res.PrefixEvidence = tokenAnalysisEvidenceUnknownPrefix + tokenAnalysisNoCandidateTieOverflow
		setCause(TokenAnalysisCauseNoCandidate + ":" + tokenAnalysisNoCandidateTieOverflow)
		return res, nil
	case c.CandState == TokenAnalysisCandidateStateBodyUnavailable:
		res.PrefixEvidence = tokenAnalysisEvidenceUnknownPrefix + tokenAnalysisNoCandidateBodyUnavail
		setCause(TokenAnalysisCauseNoCandidate + ":" + tokenAnalysisNoCandidateBodyUnavail)
		return res, nil
	case c.CandState == TokenAnalysisCandidateStateNoAPIKey:
		res.PrefixEvidence = TokenAnalysisPrefixNone
		setCause(TokenAnalysisCauseNoCandidate + ":" + tokenAnalysisNoCandidateNoneInWindow)
		return res, nil
	case c.CandState != TokenAnalysisCandidateStateOK:
		// 指纹不可用(无密钥、正文截断/未解析、协议不支持等)。
		res.PrefixEvidence = tokenAnalysisEvidenceUnknownPrefix + tokenAnalysisNoCandidateFingerprintOff
		setCause(TokenAnalysisCauseNoCandidate + ":" + tokenAnalysisNoCandidateFingerprintOff)
		return res, nil
	}
	edges, loaded := f.edges[c.ArchiveID]
	if !loaded {
		return res, &tokenAnalysisClassificationNeed{edges: []string{c.ArchiveID}}
	}
	var layer []tokenAnalysisWalkNode
	hasItemsLink := false
	for _, e := range edges {
		switch e.Kind {
		case TokenAnalysisEdgeFullPrefix, TokenAnalysisEdgePrefixMinusTail:
			layer = append(layer, tokenAnalysisWalkNode{archiveID: e.ParentArchiveID, matched: e.MatchedItems, full: e.Kind == TokenAnalysisEdgeFullPrefix})
		case TokenAnalysisEdgeItemsLink:
			hasItemsLink = true
		}
	}
	if len(layer) == 0 {
		if hasItemsLink {
			res.PrefixEvidence = TokenAnalysisPrefixItemsLink
			setCause(TokenAnalysisCausePrefixChanged)
		} else {
			res.PrefixEvidence = TokenAnalysisPrefixNone
			setCause(TokenAnalysisCauseNoCandidate + ":" + tokenAnalysisNoCandidateNoneInWindow)
		}
		return res, nil
	}

	walk, need := f.walkTokenAnalysisAncestors(c, layer)
	if need != nil {
		return res, need
	}
	if walk.selected == nil {
		res.PrefixEvidence = walk.firstPrefixEvidence
		setCause(walk.firstFailure.cause)
		res.CacheEligibility = walk.eligibility
		return res, nil
	}
	p := f.rows[walk.selected.archiveID]
	version := p.ClsInputVersion
	res.ParentArchiveID, res.ParentClsVersion = p.ArchiveID, &version
	res.CacheEligibility = TokenAnalysisEligibleAssumed
	res.PrefixEvidence = TokenAnalysisPrefixMinusTail
	if walk.selected.full {
		res.PrefixEvidence = TokenAnalysisPrefixFull
	}
	setCause(TokenAnalysisCauseMissedWithEligibleParent)
	f.computeTokenAnalysisReuseAmount(&res, c, p, walk.selected.full)
	return res, nil
}

type tokenAnalysisWalkResult struct {
	selected            *tokenAnalysisWalkNode
	firstFailure        tokenAnalysisGateFailure
	firstPrefixEvidence string
	eligibility         string
}

// walkTokenAnalysisAncestors 从子请求的前缀边出发逐层找有资格的父请求(设计 5.4):
// 当前层按(共享条目数 desc、完成时间 desc、archive_id asc)取第一个有资格者; 都没有
// 资格时沿它们自己的前缀边回溯, 每层只保留共享条目数最多的一组。原因取第一层(最长
// 一组)中走得最远的不合格项。祖先比后代更早, 超出复用窗口 W 的节点及其祖先都不
// 可能有资格, 不再展开; 只依赖子请求自身的门槛不满足时也不展开。
func (f *tokenAnalysisClassificationFacts) walkTokenAnalysisAncestors(c *TokenAnalysisClassificationRow, layer []tokenAnalysisWalkNode) (tokenAnalysisWalkResult, *tokenAnalysisClassificationNeed) {
	var out tokenAnalysisWalkResult
	visited := make(map[string]struct{}, len(layer))
	for _, n := range layer {
		visited[n.archiveID] = struct{}{}
	}
	window, hasWindow := tokenAnalysisReuseWindow(c)
	expandable := hasWindow && tokenAnalysisChildSideEligible(c)
	for depth := 1; ; depth++ {
		if need := f.missingRows(layer); need != nil {
			return out, need
		}
		layer = f.rankTokenAnalysisLayer(layer)
		var furthest *tokenAnalysisGateFailure
		var furthestNode tokenAnalysisWalkNode
		pending := false
		for i := range layer {
			n := layer[i]
			failure := tokenAnalysisPairGate(c, f.rows[n.archiveID], window, hasWindow)
			if failure == nil {
				out.selected = &n
				return out, nil
			}
			if failure.eligibility == TokenAnalysisEligibilityPending {
				pending = true
			}
			if furthest == nil || failure.gate > furthest.gate {
				furthest, furthestNode = failure, n
			}
		}
		if depth == 1 {
			out.firstFailure = *furthest
			out.firstPrefixEvidence = TokenAnalysisPrefixMinusTail
			if furthestNode.full {
				out.firstPrefixEvidence = TokenAnalysisPrefixFull
			}
			out.eligibility = furthest.eligibility
		}
		if pending {
			// 更近的候选账单未确认: 停止回溯, 确认后经版本传播重算。
			out.eligibility = TokenAnalysisEligibilityPending
			return out, nil
		}
		if !expandable || depth >= TokenAnalysisClassificationMaxDepth {
			return out, nil
		}
		var next []tokenAnalysisWalkNode
		var needEdges []string
		for _, n := range layer {
			p := f.rows[n.archiveID]
			if p == nil || c.EventTime.Sub(p.EventTime) > window {
				continue
			}
			if p.CandInputVersion != p.CandCheckedVersion {
				// 祖先的候选快照未完成: 快照提交后经传播触发重算。
				out.eligibility = TokenAnalysisEligibilityPending
				return out, nil
			}
			edges, loaded := f.edges[n.archiveID]
			if !loaded {
				needEdges = append(needEdges, n.archiveID)
				continue
			}
			for _, e := range edges {
				if e.Kind != TokenAnalysisEdgeFullPrefix && e.Kind != TokenAnalysisEdgePrefixMinusTail {
					continue
				}
				if _, seen := visited[e.ParentArchiveID]; seen {
					continue
				}
				next = append(next, tokenAnalysisAncestorNode(n, e))
			}
		}
		if len(needEdges) > 0 {
			return out, &tokenAnalysisClassificationNeed{edges: needEdges}
		}
		next = tokenAnalysisLongestGroup(next)
		if len(next) == 0 {
			return out, nil
		}
		if len(visited)+len(next) > tokenAnalysisClassificationWalkBudget {
			out.eligibility = tokenAnalysisEligibilityWalkBudget
			return out, nil
		}
		for _, n := range next {
			visited[n.archiveID] = struct{}{}
		}
		layer = next
	}
}

// tokenAnalysisAncestorNode 由后代 x 的前缀边 x→a 推出 a 与子请求的关系: a 与 x 共享
// e.MatchedItems 条, x 与子请求共享 x.matched 条; 完整前缀边且 a 的条目数不超过
// x.matched 时, a 的全部条目都是子请求的前缀。
func tokenAnalysisAncestorNode(x tokenAnalysisWalkNode, e TokenAnalysisStoredEdge) tokenAnalysisWalkNode {
	matched := min(e.MatchedItems, x.matched)
	return tokenAnalysisWalkNode{
		archiveID: e.ParentArchiveID,
		matched:   matched,
		full:      e.Kind == TokenAnalysisEdgeFullPrefix && e.MatchedItems <= x.matched,
	}
}

// tokenAnalysisLongestGroup 去重并只保留共享条目数最多的一组; 同一祖先经多条路径
// 到达时取完整前缀的关系。
func tokenAnalysisLongestGroup(nodes []tokenAnalysisWalkNode) []tokenAnalysisWalkNode {
	byID := make(map[string]tokenAnalysisWalkNode, len(nodes))
	best := 0
	for _, n := range nodes {
		if prev, ok := byID[n.archiveID]; ok {
			if prev.matched > n.matched || (prev.matched == n.matched && prev.full) {
				continue
			}
		}
		byID[n.archiveID] = n
	}
	for _, n := range byID {
		best = max(best, n.matched)
	}
	out := make([]tokenAnalysisWalkNode, 0, len(byID))
	for _, n := range byID {
		if n.matched == best {
			out = append(out, n)
		}
	}
	return out
}

func (f *tokenAnalysisClassificationFacts) missingRows(layer []tokenAnalysisWalkNode) *tokenAnalysisClassificationNeed {
	var missing []string
	for _, n := range layer {
		if _, loaded := f.rows[n.archiveID]; !loaded {
			missing = append(missing, n.archiveID)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return &tokenAnalysisClassificationNeed{rows: missing}
}

// rankTokenAnalysisLayer 按(共享条目数 desc、完成时间 desc、archive_id asc)排序;
// 不存在的行与缺完成时间的行排在后面。
func (f *tokenAnalysisClassificationFacts) rankTokenAnalysisLayer(layer []tokenAnalysisWalkNode) []tokenAnalysisWalkNode {
	sorted := append([]tokenAnalysisWalkNode(nil), layer...)
	completed := func(n tokenAnalysisWalkNode) *time.Time {
		if row := f.rows[n.archiveID]; row != nil {
			return row.CompletedAt
		}
		return nil
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.matched != b.matched {
			return a.matched > b.matched
		}
		ta, tb := completed(a), completed(b)
		switch {
		case ta != nil && tb == nil:
			return true
		case ta == nil && tb != nil:
			return false
		case ta != nil && tb != nil && !ta.Equal(*tb):
			return ta.After(*tb)
		}
		return a.archiveID < b.archiveID
	})
	return sorted
}

// tokenAnalysisObserveCache 按计费口径给出冷暖状态(设计 6.1)。
func tokenAnalysisObserveCache(c *TokenAnalysisClassificationRow) string {
	if c.Usage == nil || tokenAnalysisIsCountTokensPath(c.Endpoint) || tokenAnalysisResponseFailed(c) {
		return TokenAnalysisObsNoUsage
	}
	if !tokenAnalysisTokenBilled(c.Usage) {
		return TokenAnalysisObsUnsupported
	}
	ctx := c.Usage.InputTokens + c.Usage.CacheReadTokens + c.Usage.CacheCreationTokens
	switch {
	case ctx < tokenAnalysisSmallContextTokens:
		return TokenAnalysisObsSmall
	case float64(c.Usage.CacheReadTokens) >= tokenAnalysisCacheHitRatio*float64(ctx):
		return TokenAnalysisObsHit
	}
	return TokenAnalysisObsCold
}

func tokenAnalysisResponseFailed(row *TokenAnalysisClassificationRow) bool {
	return row.ResponseStatus != nil && (*row.ResponseStatus < 200 || *row.ResponseStatus > 299)
}

func tokenAnalysisTokenBilled(usage *TokenAnalysisClassificationUsage) bool {
	mode := strings.TrimSpace(usage.BillingMode)
	return mode == "" || mode == tokenAnalysisClassificationTokenMode
}

// tokenAnalysisUsageLinkPending: 关联登记了复核但尚未完成(含复核期间仍挂着旧账单)。
func tokenAnalysisUsageLinkPending(row *TokenAnalysisClassificationRow) bool {
	return row.UsageLinkVersion != row.UsageLinkCheckedVersion
}

func tokenAnalysisNativeResponsesPath(path string) bool {
	return path == TokenAnalysisForwardPathOpenAIResponses || path == TokenAnalysisForwardPathOpenAIResponsesPassthrough
}

// tokenAnalysisReuseWindow 返回子请求所在平台的复用窗口 W; 只有 OpenAI 有文档依据。
func tokenAnalysisReuseWindow(c *TokenAnalysisClassificationRow) (time.Duration, bool) {
	if c.Usage != nil && c.Usage.Platform == PlatformOpenAI {
		return tokenAnalysisOpenAIReuseWindow, true
	}
	return 0, false
}

// tokenAnalysisChildSideEligible 判断只依赖子请求自身的门槛(续接引用、转发路径、最终
// 条目未被改写、最终缓存键、usage 来源)。不满足时任何祖先都不可能有资格, 回溯只看第一层给原因。
func tokenAnalysisChildSideEligible(c *TokenAnalysisClassificationRow) bool {
	return !c.HasContinuationRef && tokenAnalysisNativeResponsesPath(c.ForwardPath) &&
		tokenAnalysisPromptItemsPreserved(c) && c.UpstreamRootHash != "" &&
		c.UpstreamCacheKeyHash != "" && c.UsageCacheSource == TokenAnalysisUsageCacheSourceUpstreamParsed
}

// tokenAnalysisPromptItemsPreserved: 最终发送的条目与入口条目逐条相同(两条链末端哈希相等)。
func tokenAnalysisPromptItemsPreserved(row *TokenAnalysisClassificationRow) bool {
	return row.UpstreamItemsHash != "" && len(row.ItemsEndHash) > 0 && row.UpstreamItemsHash == hex.EncodeToString(row.ItemsEndHash)
}

// tokenAnalysisPairGate 按设计第 7 节的顺序检查父子成对资格, 返回第一个不合格项;
// 全部满足时返回 nil。调用方已确认子请求账单、token 计费且非 count_tokens。
func tokenAnalysisPairGate(c, p *TokenAnalysisClassificationRow, window time.Duration, hasWindow bool) *tokenAnalysisGateFailure {
	fail := func(gate int, cause string) *tokenAnalysisGateFailure {
		return &tokenAnalysisGateFailure{gate: gate, cause: cause, eligibility: tokenAnalysisIneligiblePrefix + cause}
	}
	// 1. 账单已确认。
	if p == nil {
		return fail(1, TokenAnalysisCauseParentUnusable)
	}
	if tokenAnalysisUsageLinkPending(p) {
		return &tokenAnalysisGateFailure{gate: 1, cause: TokenAnalysisCauseParentPending, eligibility: TokenAnalysisEligibilityPending}
	}
	if p.Usage == nil || tokenAnalysisIsCountTokensPath(p.Endpoint) || tokenAnalysisResponseFailed(p) || !tokenAnalysisTokenBilled(p.Usage) {
		return fail(1, TokenAnalysisCauseParentUnusable)
	}
	// 2. 时序: 父请求完成严格早于子请求开始; 完成事实缺失时等待晚到的 response。
	if p.CompletedAt == nil {
		return &tokenAnalysisGateFailure{gate: 2, cause: TokenAnalysisCauseParentPending, eligibility: TokenAnalysisEligibilityPending}
	}
	if !p.CompletedAt.Before(c.EventTime) {
		return fail(2, TokenAnalysisCauseParentOverlapping)
	}
	// 3. 成对一致。
	if p.Usage.SentModel == "" || p.Usage.SentModel != c.Usage.SentModel {
		return fail(3, TokenAnalysisCauseModelChanged)
	}
	if p.Usage.AccountID != c.Usage.AccountID {
		return fail(3, TokenAnalysisCauseAccountChanged)
	}
	if p.FPVersion != c.FPVersion || tokenAnalysisFingerprintEndpointFamily(p.Endpoint) != tokenAnalysisFingerprintEndpointFamily(c.Endpoint) {
		return fail(3, TokenAnalysisCauseUnsupportedPath)
	}
	if p.HasContinuationRef || c.HasContinuationRef {
		return fail(3, TokenAnalysisCauseContinuationRef)
	}
	// 4. 转发路径与最终缓存键。历史行(无记录)与插件路径未经核实, 记为 unknown。
	if !tokenAnalysisNativeResponsesPath(c.ForwardPath) || !tokenAnalysisNativeResponsesPath(p.ForwardPath) {
		failure := fail(4, TokenAnalysisCauseUnsupportedPath)
		if tokenAnalysisForwardPathUnverified(c.ForwardPath) && tokenAnalysisForwardPathUnverified(p.ForwardPath) {
			failure.eligibility = tokenAnalysisEligibilityTransform
		}
		return failure
	}
	// 入口前缀只有在网关没有改写条目、且父子最终顶层字段相同时才代表上游实际收到的前缀。
	// 没有最终正文哈希的行(未经核实)记为 unknown。
	if !tokenAnalysisPromptItemsPreserved(c) || !tokenAnalysisPromptItemsPreserved(p) ||
		c.UpstreamRootHash == "" || c.UpstreamRootHash != p.UpstreamRootHash {
		failure := fail(4, TokenAnalysisCausePromptRewritten)
		if c.UpstreamItemsHash == "" || p.UpstreamItemsHash == "" || c.UpstreamRootHash == "" || p.UpstreamRootHash == "" {
			failure.eligibility = tokenAnalysisEligibilityTransform
		}
		return failure
	}
	if c.UpstreamCacheKeyHash == "" || c.UpstreamCacheKeyHash != p.UpstreamCacheKeyHash {
		who := tokenAnalysisCacheKeyChangedClient
		if bytes.Equal(c.ClientCacheKeyHash, p.ClientCacheKeyHash) {
			who = tokenAnalysisCacheKeyChangedGateway
		}
		return &tokenAnalysisGateFailure{gate: 4, cause: TokenAnalysisCauseCacheKeyChanged + ":" + who,
			eligibility: tokenAnalysisIneligiblePrefix + TokenAnalysisCauseCacheKeyChanged}
	}
	// 5. usage 来自上游解析。
	if c.UsageCacheSource != TokenAnalysisUsageCacheSourceUpstreamParsed || p.UsageCacheSource != TokenAnalysisUsageCacheSourceUpstreamParsed {
		return fail(5, TokenAnalysisCauseUsageNotUpstream)
	}
	// 6. 复用窗口, 从父请求开始时刻起算。
	if !hasWindow {
		return &tokenAnalysisGateFailure{gate: 6, cause: TokenAnalysisCauseUnsupportedPath, eligibility: tokenAnalysisEligibilityNoWindow}
	}
	if c.EventTime.Sub(p.EventTime) > window {
		return fail(6, TokenAnalysisCauseBeyondReuseWindow)
	}
	return nil
}

// tokenAnalysisForwardPathUnverified: 原生路径, 或未经核实(历史行没有记录、插件路径)。
// 双方都属于此类而又不都是原生路径时, 资格记为 unknown 而不是 ineligible。
func tokenAnalysisForwardPathUnverified(path string) bool {
	return path == "" || path == TokenAnalysisForwardPathOpenAIResponsesPlugin || tokenAnalysisNativeResponsesPath(path)
}

// computeTokenAnalysisReuseAmount 计算条件性复用机会金额(设计第 8 节): 只对完整前缀、
// 纯文本、子请求有缓存读(可得本行读价)且双方都没有缓存写入的行给出。
func (f *tokenAnalysisClassificationFacts) computeTokenAnalysisReuseAmount(res *TokenAnalysisClassificationResult, c, p *TokenAnalysisClassificationRow, full bool) {
	cu, pu := c.Usage, p.Usage
	switch {
	case !full:
		res.PriceEvidence = tokenAnalysisPriceNotFullPrefix
		return
	case cu.CacheCreationTokens > 0 || pu.CacheCreationTokens > 0:
		res.PriceEvidence = tokenAnalysisPriceOpenAICacheWrite
		return
	case c.ImageCount > 0 || cu.ImageInputTokens > 0:
		res.PriceEvidence = tokenAnalysisPriceImageInput
		return
	case cu.CacheReadTokens <= 0:
		res.PriceEvidence = tokenAnalysisPriceNoCacheRead
		return
	}
	parentTotal := pu.InputTokens + pu.CacheReadTokens + pu.CacheCreationTokens
	reusable := parentTotal / tokenAnalysisReusableTokenBlock * tokenAnalysisReusableTokenBlock
	if reusable < tokenAnalysisReusableTokenMin {
		reusable = 0
	}
	// 包内测试文件定义了 int 版 max, 这里不用内置 max。
	recoverable := min(reusable-cu.CacheReadTokens, cu.InputTokens)
	if recoverable < 0 {
		recoverable = 0
	}
	amount := decimal.Zero
	if recoverable > 0 {
		if cu.InputTokens <= 0 || !cu.InputCost.IsPositive() {
			res.PriceEvidence = tokenAnalysisPriceNoInputPrice
			return
		}
		// R × (输入费/输入量 − 读费/读量) 合成一个有理式, 只在最后做一次 10 位舍入;
		// 分别求单价会先各自舍入, 末位可能偏差。
		in, read := decimal.NewFromInt(cu.InputTokens), decimal.NewFromInt(cu.CacheReadTokens)
		numerator := decimal.NewFromInt(recoverable).Mul(cu.InputCost.Mul(read).Sub(cu.CacheReadCost.Mul(in)))
		amount = decimal.Max(decimal.Zero, numerator.DivRound(in.Mul(read), 10))
	}
	res.PriceEvidence = TokenAnalysisPriceRowExact
	res.ReusableTokens, res.RecoverableTokens, res.Amount = &reusable, &recoverable, &amount
	res.AmountInputsHash = tokenAnalysisAmountInputsHash(f.secret, c, p)
}

// tokenAnalysisAmountInputsHash 绑定金额的输入: 规则版本、双方账单 ID 与用量/费用字段。
func tokenAnalysisAmountInputsHash(secret string, c, p *TokenAnalysisClassificationRow) []byte {
	parts := [][]byte{tokenAnalysisInt64Bytes(int64(TokenAnalysisClassificationRuleVersion))}
	for _, row := range []*TokenAnalysisClassificationRow{c, p} {
		u := row.Usage
		parts = append(parts,
			tokenAnalysisInt64Bytes(u.UsageLogID),
			tokenAnalysisInt64Bytes(u.InputTokens), tokenAnalysisInt64Bytes(u.CacheReadTokens),
			tokenAnalysisInt64Bytes(u.CacheCreationTokens), tokenAnalysisInt64Bytes(u.ImageInputTokens),
			[]byte(u.InputCost.String()), []byte(u.CacheReadCost.String()),
			[]byte(u.CacheCreationCost.String()), []byte(u.ImageInputCost.String()),
		)
	}
	return tokenAnalysisKeyedDigest([]byte(secret), tokenAnalysisAmountHashDomain, parts...)
}

func tokenAnalysisInt64Bytes(v int64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(v))
	return b[:]
}
