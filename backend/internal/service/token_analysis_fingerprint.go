package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"sort"
	"strings"
)

// 前缀指纹: 对请求输入条目做规范化编码后逐条串成 HMAC 链, 末端值相同即"父请求的
// 全部条目是子请求的前缀"。见风险分类设计第 5 节。
const (
	// tokenAnalysisFingerprintRuleVersion 是规范化规则与字段清单的版本; 规则变化必须加一,
	// 新旧版本的指纹不可比较。
	tokenAnalysisFingerprintRuleVersion = 1
	// tokenAnalysisFingerprintHashLen 是持久化哈希的字节数(128 位)。
	tokenAnalysisFingerprintHashLen  = 16
	tokenAnalysisFingerprintMaxItems = 1_000_000

	TokenAnalysisFingerprintOK          = "ok"
	tokenAnalysisFingerprintUnsupported = "unsupported:"
	tokenAnalysisFingerprintUnavailable = "unavailable:no_secret"

	tokenAnalysisFingerprintDomainPrefix = "sub2api/token-analysis/prefix/v1"
	tokenAnalysisFingerprintDomainItems  = "sub2api/token-analysis/items/v1"
	tokenAnalysisFingerprintDomainKey    = "sub2api/token-analysis/client-cache-key/v1"
)

// tokenAnalysisFingerprintNonPrefixFields 是不影响提示词前缀的顶层字段: 生成参数、
// 存储/流式开关和请求元数据。client_metadata 是 Codex 的 turn/thread 元数据, 频繁
// 变化但不进入提示词(2026-10-27 生产结构样本)。prompt_cache_key 单独比较, 不计入链。
var tokenAnalysisFingerprintNonPrefixFields = map[string]bool{
	"stream": true, "stream_options": true, "max_tokens": true, "max_output_tokens": true,
	"temperature": true, "top_p": true, "metadata": true, "user": true, "store": true,
	"include": true, "service_tier": true, "client_metadata": true, "prompt_cache_key": true,
}

// TokenAnalysisFingerprint 是持久化的指纹结果; 链本身不入库。
type TokenAnalysisFingerprint struct {
	State              string
	Version            int32
	PrefixEndHash      []byte
	PrefixTail1Hash    []byte
	ItemsEndHash       []byte
	ClientCacheKeyHash []byte
	PrefixItems        int
	HasContinuationRef bool
}

// Comparable 表示可参与候选匹配。
func (f TokenAnalysisFingerprint) Comparable() bool {
	return f.State == TokenAnalysisFingerprintOK && len(f.PrefixEndHash) == tokenAnalysisFingerprintHashLen
}

// tokenAnalysisFingerprintChains 是索引时在内存中的完整链: prefix[i]、items[i] 为前
// i+1 个条目的链值。用于查找父请求, 不入库。
type tokenAnalysisFingerprintChains struct {
	// root 是链的起点 h0(端点族与顶层字段), 不含条目。
	root   []byte
	prefix [][]byte
	items  [][]byte
}

type tokenAnalysisFingerprintInput struct {
	Secret      string
	Endpoint    string
	Root        map[string]any
	Truncated   bool
	Degraded    bool
	CountTokens bool
}

// computeTokenAnalysisFingerprint 计算指纹。不能可靠比较的请求(截断、降级、
// count_tokens、无条目字段的协议)返回 unsupported, 无密钥返回 unavailable。
func computeTokenAnalysisFingerprint(in tokenAnalysisFingerprintInput) (TokenAnalysisFingerprint, *tokenAnalysisFingerprintChains) {
	switch {
	case in.Secret == "":
		return TokenAnalysisFingerprint{State: tokenAnalysisFingerprintUnavailable}, nil
	case in.Truncated:
		return tokenAnalysisFingerprintUnsupportedResult("body_truncated"), nil
	case in.Degraded || in.Root == nil:
		return tokenAnalysisFingerprintUnsupportedResult("body_unparsed"), nil
	case in.CountTokens:
		return tokenAnalysisFingerprintUnsupportedResult("count_tokens"), nil
	}
	itemsField, protocol := tokenAnalysisFingerprintItemsField(in.Root)
	if itemsField == "" {
		return tokenAnalysisFingerprintUnsupportedResult("protocol"), nil
	}
	items, ok := tokenAnalysisFingerprintItems(in.Root[itemsField])
	if !ok {
		return tokenAnalysisFingerprintUnsupportedResult("items_shape"), nil
	}
	if len(items) == 0 {
		return tokenAnalysisFingerprintUnsupportedResult("no_items"), nil
	}
	if len(items) > tokenAnalysisFingerprintMaxItems {
		return tokenAnalysisFingerprintUnsupportedResult("too_many_items"), nil
	}

	key := []byte(in.Secret)
	family := protocol + "|" + tokenAnalysisFingerprintEndpointFamily(in.Endpoint)
	top := make(map[string]any, len(in.Root))
	for k, v := range in.Root {
		if k != itemsField && !tokenAnalysisFingerprintNonPrefixFields[k] {
			top[k] = v
		}
	}
	h := tokenAnalysisKeyedDigest(key, tokenAnalysisFingerprintDomainPrefix, []byte(family), tokenAnalysisCanonicalDigest(top))
	g := tokenAnalysisKeyedDigest(key, tokenAnalysisFingerprintDomainItems, []byte(family))
	chains := &tokenAnalysisFingerprintChains{root: h, prefix: make([][]byte, len(items)), items: make([][]byte, len(items))}
	for i, item := range items {
		// 条目内容只做一次 SHA-256, 两条链在摘要上各做一次 HMAC。
		digest := tokenAnalysisCanonicalDigest(item)
		h = tokenAnalysisKeyedDigest(key, tokenAnalysisFingerprintDomainPrefix, h, digest)
		g = tokenAnalysisKeyedDigest(key, tokenAnalysisFingerprintDomainItems, g, digest)
		chains.prefix[i], chains.items[i] = h, g
	}

	fp := TokenAnalysisFingerprint{
		State:         TokenAnalysisFingerprintOK,
		Version:       tokenAnalysisFingerprintVersion(in.Secret),
		PrefixEndHash: chains.prefix[len(items)-1],
		ItemsEndHash:  chains.items[len(items)-1],
		PrefixItems:   len(items),
	}
	if len(items) >= 2 {
		fp.PrefixTail1Hash = chains.prefix[len(items)-2]
	}
	// 缓存键按原值比较: 首尾空白不同的键是不同的键。
	if cacheKey, ok := in.Root["prompt_cache_key"].(string); ok && cacheKey != "" {
		fp.ClientCacheKeyHash = tokenAnalysisKeyedDigest(key, tokenAnalysisFingerprintDomainKey, []byte(cacheKey))
	}
	_, hasPrevious := in.Root["previous_response_id"]
	_, hasConversation := in.Root["conversation"]
	fp.HasContinuationRef = hasPrevious || hasConversation
	return fp, chains
}

func tokenAnalysisFingerprintUnsupportedResult(reason string) TokenAnalysisFingerprint {
	return TokenAnalysisFingerprint{State: tokenAnalysisFingerprintUnsupported + reason}
}

// tokenAnalysisFingerprintItemsField 与摘要器的协议判定一致: Gemini 与生图首版不支持。
func tokenAnalysisFingerprintItemsField(root map[string]any) (field, protocol string) {
	switch {
	case tokenAnalysisArray(root["contents"]) != nil:
		return "", ""
	case root["prompt"] != nil && root["messages"] == nil && root["input"] == nil:
		return "", ""
	case root["input"] != nil:
		return "input", "responses"
	case tokenAnalysisArray(root["messages"]) != nil:
		return "messages", "messages"
	}
	return "", ""
}

// tokenAnalysisFingerprintItems 取条目列表; Responses 的字符串 input 视为单个带专用
// 标签的条目, 永远不会与数组形式的条目相等。
func tokenAnalysisFingerprintItems(value any) ([]any, bool) {
	switch v := value.(type) {
	case []any:
		return v, true
	case string:
		return []any{map[string]any{"__sub2api_string_input": v}}, true
	}
	return nil, false
}

// tokenAnalysisFingerprintEndpointFamily 区分端点族(同一 body 发往不同协议端点不可比)。
func tokenAnalysisFingerprintEndpointFamily(endpoint string) string {
	endpoint = strings.ToLower(strings.TrimSpace(endpoint))
	if i := strings.IndexAny(endpoint, "?#"); i >= 0 {
		endpoint = endpoint[:i]
	}
	switch {
	case strings.HasSuffix(endpoint, "/responses") || strings.Contains(endpoint, "/responses/"):
		return "responses"
	case strings.HasSuffix(endpoint, "/chat/completions"):
		return "chat"
	case strings.HasSuffix(endpoint, "/messages"):
		return "messages"
	}
	return endpoint
}

// tokenAnalysisFingerprintVersion = 规则版本(高 8 位) + 密钥 ID(低 24 位, 密钥 SHA-256 前 3 字节)。
// 密钥轮换或规则变化后版本不同, 新旧指纹不会互相匹配。
func tokenAnalysisFingerprintVersion(secret string) int32 {
	sum := sha256.Sum256([]byte(secret))
	keyID := int32(sum[0])<<16 | int32(sum[1])<<8 | int32(sum[2])
	return int32(tokenAnalysisFingerprintRuleVersion)<<24 | keyID
}

func tokenAnalysisKeyedDigest(key []byte, domain string, parts ...[]byte) []byte {
	mac := hmac.New(sha256.New, key)
	writeTokenAnalysisCanonicalLength(mac, len(domain))
	_, _ = mac.Write([]byte(domain))
	for _, part := range parts {
		writeTokenAnalysisCanonicalLength(mac, len(part))
		_, _ = mac.Write(part)
	}
	return mac.Sum(nil)[:tokenAnalysisFingerprintHashLen]
}

// tokenAnalysisCanonicalDigest 是规范化编码的 SHA-256(仅作 HMAC 的输入, 不入库)。
func tokenAnalysisCanonicalDigest(value any) []byte {
	digest := sha256.New()
	writeTokenAnalysisCanonical(digest, value)
	return digest.Sum(nil)
}

// writeTokenAnalysisCanonical 写规范化编码: 每个节点带类型标签与长度前缀, 对象按键的
// 字节序排列。字符串与数字按原始字节/字面量编码, 不做脱敏、清洗或数值归一; 重复键
// 按 JSON 解码"后者生效"的结果处理。
func writeTokenAnalysisCanonical(w hash.Hash, value any) {
	switch v := value.(type) {
	case nil:
		_, _ = w.Write([]byte{'Z'})
	case bool:
		if v {
			_, _ = w.Write([]byte{'T'})
		} else {
			_, _ = w.Write([]byte{'F'})
		}
	case json.Number:
		_, _ = w.Write([]byte{'N'})
		writeTokenAnalysisCanonicalLength(w, len(v))
		_, _ = w.Write([]byte(v))
	case float64:
		// 不经 UseNumber 解码时的兜底; 索引器统一用 UseNumber。
		literal := fmt.Sprintf("%v", v)
		_, _ = w.Write([]byte{'N'})
		writeTokenAnalysisCanonicalLength(w, len(literal))
		_, _ = w.Write([]byte(literal))
	case string:
		_, _ = w.Write([]byte{'S'})
		writeTokenAnalysisCanonicalLength(w, len(v))
		_, _ = w.Write([]byte(v))
	case []any:
		_, _ = w.Write([]byte{'A'})
		writeTokenAnalysisCanonicalLength(w, len(v))
		for _, item := range v {
			writeTokenAnalysisCanonical(w, item)
		}
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		_, _ = w.Write([]byte{'O'})
		writeTokenAnalysisCanonicalLength(w, len(keys))
		for _, k := range keys {
			writeTokenAnalysisCanonicalLength(w, len(k))
			_, _ = w.Write([]byte(k))
			writeTokenAnalysisCanonical(w, v[k])
		}
	default:
		// JSON 解码不会产生其他类型; 防御性地按类型名编码, 保证不同于任何合法节点。
		name := fmt.Sprintf("%T", v)
		_, _ = w.Write([]byte{'X'})
		writeTokenAnalysisCanonicalLength(w, len(name))
		_, _ = w.Write([]byte(name))
	}
}

func writeTokenAnalysisCanonicalLength(w interface{ Write([]byte) (int, error) }, n int) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(n))
	_, _ = w.Write(length[:])
}

// parseTokenAnalysisRequestBody 用 UseNumber 解码请求体, 保留数字原始字面量(大整数不
// 转成 float64), 供摘要器与指纹共用同一次解析。
func parseTokenAnalysisRequestBody(body []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return nil, err
	}
	// 与 json.Unmarshal 一致: 第一个值之后不允许再有内容。
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("invalid character after top-level value")
	}
	return root, nil
}

// tokenAnalysisUpstreamPromptHashes 对最终发往上游的请求体计算条目链末端与 h0, 与索引器
// 对归档(入口)正文的算法、密钥和端点族相同: 最终条目与入口条目逐条相同时, 条目哈希等于
// 入口的 items_end_hash。网关对条目的任何改写(重排、剥离、补全)都会使两者不等。
// 正文不可解析或不可比较时返回空串。
func tokenAnalysisUpstreamPromptHashes(secret, endpoint string, body []byte) (itemsHash, rootHash string) {
	if secret == "" || len(body) == 0 {
		return "", ""
	}
	root, err := parseTokenAnalysisRequestBody(body)
	if err != nil {
		return "", ""
	}
	fp, chains := computeTokenAnalysisFingerprint(tokenAnalysisFingerprintInput{Secret: secret, Endpoint: endpoint, Root: root})
	if !fp.Comparable() || chains == nil {
		return "", ""
	}
	return hex.EncodeToString(fp.ItemsEndHash), hex.EncodeToString(chains.root)
}
