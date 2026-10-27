package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// 转发侧记录: 网关在定型发往上游的请求时写下与缓存复用相关的事实, 归档 response
// 行在 c.Next() 之后读出并随归档进入 Token Analysis。见风险分类设计 11.2。
const (
	tokenAnalysisForwardFactsContextKey = "_token_analysis_forward_facts"

	TokenAnalysisForwardPathOpenAIResponses            = "openai_responses"
	TokenAnalysisForwardPathOpenAIResponsesPassthrough = "openai_responses_passthrough"
	// TokenAnalysisForwardPathOpenAIResponsesPlugin 表示实际交给了 OpenAI OAuth 插件:
	// 外部插件是否继续改写请求未经核实, 不提供缓存键哈希与 usage 来源证据。
	TokenAnalysisForwardPathOpenAIResponsesPlugin = "openai_responses_plugin"

	TokenAnalysisCacheKeyPassthrough = "passthrough"
	TokenAnalysisCacheKeyAbsent      = "absent"
	TokenAnalysisCacheKeyScoped      = "scoped"
	TokenAnalysisCacheKeyInjected    = "injected"
	TokenAnalysisCacheKeyDeleted     = "deleted"

	// TokenAnalysisUsageCacheSourceUpstreamParsed 表示 usage 缓存桶是上游解析值, 未被本地改写。
	TokenAnalysisUsageCacheSourceUpstreamParsed = "upstream_parsed"

	tokenAnalysisUpstreamCacheKeyHashDomain = "sub2api/token-analysis/upstream-cache-key/v1"
	// tokenAnalysisHashHexLen 是 128 位哈希的十六进制长度。
	tokenAnalysisHashHexLen = 32
)

// TokenAnalysisForwardFacts 是一次转发尝试的缓存相关事实; 只含枚举与哈希, 不含正文。
// UpstreamItemsHash/UpstreamRootHash 是最终请求体的条目链末端与 h0(算法同指纹), 用来
// 验证入口前缀证据在最终请求中仍然成立(设计 11.2)。
type TokenAnalysisForwardFacts struct {
	ForwardPath          string `json:"forward_path,omitempty"`
	CacheKeyHandling     string `json:"cache_key_handling,omitempty"`
	UpstreamCacheKeyHash string `json:"upstream_cache_key_hash,omitempty"`
	UsageCacheSource     string `json:"usage_cache_source,omitempty"`
	UpstreamItemsHash    string `json:"upstream_items_hash,omitempty"`
	UpstreamRootHash     string `json:"upstream_root_hash,omitempty"`
}

type tokenAnalysisForwardState struct {
	inboundCacheKey string
	endpoint        string
	// 最后一次原生尝试: recorded 表示已有记录; viaPlugin 表示实际交给了插件。正文取自构造器
	// 产出的请求(GetBody), 是真正交给 transport 的字节, 包含构造器内部的改写; 哈希留到
	// 归档读取记录时计算(请求协程内、响应结束之后), 只算一次。
	recorded       bool
	viaPlugin      bool
	forwardPath    string
	secret         string
	sessionID      string
	conversationID string
	finalBody      func() (io.ReadCloser, error)
	facts          *TokenAnalysisForwardFacts
}

// beginTokenAnalysisForwardFacts 在每次转发尝试开始时调用: 清空上一次尝试的记录,
// 记下入口的 prompt_cache_key。之后只有原生路径会写入记录, 改走协议转换的尝试
// 因此不会继承前一次原生尝试的事实。
func beginTokenAnalysisForwardFacts(c *gin.Context, body []byte) {
	if c == nil {
		return
	}
	endpoint := ""
	if c.Request != nil && c.Request.URL != nil {
		endpoint = c.Request.URL.Path
	}
	// 缓存键按原值比较, 不做 TrimSpace: 首尾空白不同的键会被原样发往上游。
	c.Set(tokenAnalysisForwardFactsContextKey, &tokenAnalysisForwardState{
		inboundCacheKey: gjson.GetBytes(body, "prompt_cache_key").String(),
		endpoint:        endpoint,
	})
}

// recordTokenAnalysisNativeForwardFacts 由 Forward 内两处原生构造调用点在构造器返回后
// 调用, req 是构造器产出、即将发送的请求; 实际 transport 是插件时, 发送后再由
// markTokenAnalysisForwardFactsViaPlugin 降级。secret 为空时不计算哈希(资格判定为 unknown)。
func recordTokenAnalysisNativeForwardFacts(c *gin.Context, secret, forwardPath string, req *http.Request) {
	if c == nil || req == nil {
		return
	}
	raw, ok := c.Get(tokenAnalysisForwardFactsContextKey)
	state, _ := raw.(*tokenAnalysisForwardState)
	if !ok || state == nil {
		// 未经过 Forward 入口(如独立调用构造函数), 无入口值可比较, 不记录。
		return
	}
	// 每次尝试覆盖: 重试改写正文(如剥离加密 reasoning)后, 留下的是最后一次实际发送的请求。
	state.recorded, state.viaPlugin, state.facts = true, false, nil
	state.forwardPath, state.secret, state.finalBody = forwardPath, secret, req.GetBody
	state.sessionID, state.conversationID = req.Header.Get("session_id"), req.Header.Get("conversation_id")
}

// tokenAnalysisFingerprintSecret 返回启动时引导的指纹密钥; 未引导时为空。
func (s *OpenAIGatewayService) tokenAnalysisFingerprintSecret() string {
	if s == nil || s.cfg == nil {
		return ""
	}
	return s.cfg.TokenAnalysis.FingerprintSecret
}

// markTokenAnalysisForwardFactsViaPlugin 在本次尝试实际交给插件后调用: 只保留网关侧
// 的缓存键处理方式作观测, 不提供依赖原生 transport 的哈希与 usage 来源。
func markTokenAnalysisForwardFactsViaPlugin(c *gin.Context) {
	if c == nil {
		return
	}
	raw, _ := c.Get(tokenAnalysisForwardFactsContextKey)
	state, _ := raw.(*tokenAnalysisForwardState)
	if state == nil || !state.recorded {
		return
	}
	state.viaPlugin, state.facts = true, nil
}

func classifyTokenAnalysisCacheKeyHandling(inbound, final string) string {
	switch {
	case inbound == "" && final == "":
		return TokenAnalysisCacheKeyAbsent
	case inbound == final:
		return TokenAnalysisCacheKeyPassthrough
	case inbound == "":
		return TokenAnalysisCacheKeyInjected
	case final == "":
		return TokenAnalysisCacheKeyDeleted
	default:
		return TokenAnalysisCacheKeyScoped
	}
}

// TokenAnalysisForwardFactsFromContext 返回最后一次转发尝试的记录; 没有原生路径记录时
// 返回 nil。首次调用时读取实际发送的正文计算缓存键与提示词哈希(归档在 c.Next 返回后、
// 同一请求协程内调用: 计算被后移到响应结束之后, 不是异步, 也不是零成本)。
func TokenAnalysisForwardFactsFromContext(c *gin.Context) *TokenAnalysisForwardFacts {
	if c == nil {
		return nil
	}
	raw, ok := c.Get(tokenAnalysisForwardFactsContextKey)
	state, _ := raw.(*tokenAnalysisForwardState)
	if !ok || state == nil || !state.recorded {
		return nil
	}
	if state.facts == nil {
		state.facts = state.computeFacts()
		state.finalBody = nil
	}
	facts := *state.facts
	return &facts
}

func (state *tokenAnalysisForwardState) computeFacts() *TokenAnalysisForwardFacts {
	var body []byte
	if state.finalBody != nil {
		if reader, err := state.finalBody(); err == nil {
			body, _ = io.ReadAll(reader)
			_ = reader.Close()
		}
	}
	finalKey := gjson.GetBytes(body, "prompt_cache_key").String()
	handling := classifyTokenAnalysisCacheKeyHandling(state.inboundCacheKey, finalKey)
	if state.viaPlugin {
		return &TokenAnalysisForwardFacts{ForwardPath: TokenAnalysisForwardPathOpenAIResponsesPlugin, CacheKeyHandling: handling}
	}
	facts := &TokenAnalysisForwardFacts{ForwardPath: state.forwardPath, CacheKeyHandling: handling, UsageCacheSource: TokenAnalysisUsageCacheSourceUpstreamParsed}
	if state.secret == "" || body == nil {
		// 没有密钥或读不到实际正文: 不提供哈希, 资格判定为 unknown。
		return facts
	}
	facts.UpstreamCacheKeyHash = tokenAnalysisKeyedHash(state.secret, tokenAnalysisUpstreamCacheKeyHashDomain, finalKey, state.sessionID, state.conversationID)
	facts.UpstreamItemsHash, facts.UpstreamRootHash = tokenAnalysisUpstreamPromptHashes(state.secret, state.endpoint, body)
	return facts
}

// validTokenAnalysisForwardFacts 只保留取值合法的字段: 归档是外部文件, 未知枚举或
// 非法哈希按"没有记录"处理, 不让坏数据进入资格判定。
func validTokenAnalysisForwardFacts(facts *TokenAnalysisForwardFacts) TokenAnalysisForwardFacts {
	if facts == nil {
		return TokenAnalysisForwardFacts{}
	}
	var out TokenAnalysisForwardFacts
	switch facts.ForwardPath {
	case TokenAnalysisForwardPathOpenAIResponses, TokenAnalysisForwardPathOpenAIResponsesPassthrough, TokenAnalysisForwardPathOpenAIResponsesPlugin:
		out.ForwardPath = facts.ForwardPath
	default:
		// 路径未知时其余事实没有意义。
		return TokenAnalysisForwardFacts{}
	}
	switch facts.CacheKeyHandling {
	case TokenAnalysisCacheKeyPassthrough, TokenAnalysisCacheKeyAbsent, TokenAnalysisCacheKeyScoped, TokenAnalysisCacheKeyInjected, TokenAnalysisCacheKeyDeleted:
		out.CacheKeyHandling = facts.CacheKeyHandling
	}
	if out.ForwardPath == TokenAnalysisForwardPathOpenAIResponsesPlugin {
		// 插件路径不提供原生 transport 的证据, 即使归档里带了也不采信。
		return out
	}
	if isTokenAnalysisHashHex(facts.UpstreamCacheKeyHash) {
		out.UpstreamCacheKeyHash = facts.UpstreamCacheKeyHash
	}
	if facts.UsageCacheSource == TokenAnalysisUsageCacheSourceUpstreamParsed {
		out.UsageCacheSource = facts.UsageCacheSource
	}
	if isTokenAnalysisHashHex(facts.UpstreamItemsHash) && isTokenAnalysisHashHex(facts.UpstreamRootHash) {
		out.UpstreamItemsHash, out.UpstreamRootHash = facts.UpstreamItemsHash, facts.UpstreamRootHash
	}
	return out
}

func isTokenAnalysisHashHex(value string) bool {
	if len(value) != tokenAnalysisHashHexLen {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// tokenAnalysisKeyedHash 对各部分做长度前缀编码后取 HMAC-SHA256 的前 128 位,
// 避免拼接歧义("ab"+"c" 与 "a"+"bc" 不同)。
func tokenAnalysisKeyedHash(secret string, parts ...string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	var length [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(length[:], uint64(len(part)))
		_, _ = mac.Write(length[:])
		_, _ = mac.Write([]byte(part))
	}
	return hex.EncodeToString(mac.Sum(nil))[:tokenAnalysisHashHexLen]
}
