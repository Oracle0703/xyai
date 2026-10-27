package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const testTokenAnalysisFingerprintSecret = "0123456789abcdef0123456789abcdef"

func testTokenAnalysisFingerprint(t *testing.T, endpoint, body string) (TokenAnalysisFingerprint, *tokenAnalysisFingerprintChains) {
	t.Helper()
	root, err := parseTokenAnalysisRequestBody([]byte(body))
	require.NoError(t, err)
	return computeTokenAnalysisFingerprint(tokenAnalysisFingerprintInput{Secret: testTokenAnalysisFingerprintSecret, Endpoint: endpoint, Root: root})
}

func TestTokenAnalysisCanonicalDigest(t *testing.T) {
	parse := func(s string) any {
		root, err := parseTokenAnalysisRequestBody([]byte(`{"v":` + s + `}`))
		require.NoError(t, err)
		return root["v"]
	}
	digest := func(s string) string { return string(tokenAnalysisCanonicalDigest(parse(s))) }

	require.Equal(t, digest(`{"a":1,"b":[true,null]}`), digest(`{"b":[true,null],"a":1}`), "object key order does not matter")
	require.NotEqual(t, digest(`"1"`), digest(`1`), "type tags separate string and number")
	require.NotEqual(t, digest(`["ab","c"]`), digest(`["a","bc"]`), "length prefixes prevent concatenation ambiguity")
	require.NotEqual(t, digest(`12345678901234567890`), digest(`12345678901234567891`), "UseNumber keeps large integers exact")
	require.NotEqual(t, digest(`1.0`), digest(`1`), "number literals are compared verbatim")
	require.NotEqual(t, digest(`{"a":null}`), digest(`{}`))
}

func TestParseTokenAnalysisRequestBodyRejectsTrailingData(t *testing.T) {
	_, err := parseTokenAnalysisRequestBody([]byte(`{"a":1} {"b":2}`))
	require.Error(t, err)
	_, err = parseTokenAnalysisRequestBody([]byte(`{"a":1}` + "\n"))
	require.NoError(t, err)
}

// 正常追加: 父请求全部条目是子请求前缀, 父请求末端哈希等于子请求链中同条目数的位置;
// 去尾哈希等于前 n-1 条的链值。
func TestTokenAnalysisFingerprintAppendOnlyChain(t *testing.T) {
	parent, _ := testTokenAnalysisFingerprint(t, "/v1/responses", `{"model":"gpt","instructions":"sys","input":[{"role":"user","content":"a"},{"role":"assistant","content":"b"}]}`)
	child, chains := testTokenAnalysisFingerprint(t, "/v1/responses", `{"model":"gpt","instructions":"sys","input":[{"role":"user","content":"a"},{"role":"assistant","content":"b"},{"role":"user","content":"c"}]}`)
	require.True(t, parent.Comparable())
	require.Equal(t, 2, parent.PrefixItems)
	require.Equal(t, 3, child.PrefixItems)
	require.Equal(t, parent.PrefixEndHash, chains.prefix[1])
	require.Equal(t, chains.prefix[1], child.PrefixTail1Hash)
	require.Equal(t, parent.ItemsEndHash, chains.items[1])
	require.Len(t, child.PrefixEndHash, tokenAnalysisFingerprintHashLen)
}

// 非前缀字段(生成参数、流式/存储开关、元数据、Codex client_metadata、缓存键)不影响链;
// instructions、tools、model 等进入 h0, 变化即前缀变化。关联链不含顶层字段。
func TestTokenAnalysisFingerprintFieldDivision(t *testing.T) {
	const items = `"input":[{"role":"user","content":"a"}]`
	base, _ := testTokenAnalysisFingerprint(t, "/v1/responses", `{"model":"gpt","tools":[{"type":"function","name":"f"}],`+items+`}`)
	same, _ := testTokenAnalysisFingerprint(t, "/v1/responses", `{"model":"gpt","tools":[{"type":"function","name":"f"}],`+items+`,"stream":true,"store":false,"max_output_tokens":9,"metadata":{"x":1},"client_metadata":{"turn_id":"t2"},"prompt_cache_key":"k","include":["r"]}`)
	require.Equal(t, base.PrefixEndHash, same.PrefixEndHash)

	for _, changed := range []string{
		`{"model":"gpt-2","tools":[{"type":"function","name":"f"}],` + items + `}`,
		`{"model":"gpt","tools":[],` + items + `}`,
		`{"model":"gpt","tools":[{"type":"function","name":"f"}],"parallel_tool_calls":false,` + items + `}`,
		`{"model":"gpt","tools":[{"type":"function","name":"f"}],"instructions":"new",` + items + `}`,
	} {
		fp, _ := testTokenAnalysisFingerprint(t, "/v1/responses", changed)
		require.NotEqual(t, base.PrefixEndHash, fp.PrefixEndHash, changed)
		require.Equal(t, base.ItemsEndHash, fp.ItemsEndHash, "items chain ignores top-level fields: %s", changed)
	}

	// 同一 body 发往不同协议端点不可比。
	chat, _ := testTokenAnalysisFingerprint(t, "/v1/chat/completions", `{"model":"gpt","messages":[{"role":"user","content":"a"}]}`)
	messages, _ := testTokenAnalysisFingerprint(t, "/v1/messages", `{"model":"gpt","messages":[{"role":"user","content":"a"}]}`)
	require.NotEqual(t, chat.PrefixEndHash, messages.PrefixEndHash)
}

func TestTokenAnalysisFingerprintMetadataFields(t *testing.T) {
	fp, _ := testTokenAnalysisFingerprint(t, "/v1/responses", `{"input":"hello","prompt_cache_key":" k ","previous_response_id":"resp_1"}`)
	require.True(t, fp.Comparable())
	require.Equal(t, 1, fp.PrefixItems)
	require.Nil(t, fp.PrefixTail1Hash)
	require.Len(t, fp.ClientCacheKeyHash, tokenAnalysisFingerprintHashLen)
	require.True(t, fp.HasContinuationRef)

	// 字符串 input 与数组 input 永不相等。
	asArray, _ := testTokenAnalysisFingerprint(t, "/v1/responses", `{"input":[{"role":"user","content":"hello"}]}`)
	asString, _ := testTokenAnalysisFingerprint(t, "/v1/responses", `{"input":"hello"}`)
	require.NotEqual(t, asArray.PrefixEndHash, asString.PrefixEndHash)

	noKey, _ := testTokenAnalysisFingerprint(t, "/v1/responses", `{"input":"hello"}`)
	require.Nil(t, noKey.ClientCacheKeyHash)
	require.False(t, noKey.HasContinuationRef)
}

func TestTokenAnalysisFingerprintUnsupported(t *testing.T) {
	root := map[string]any{"input": []any{"x"}}
	cases := map[string]tokenAnalysisFingerprintInput{
		"unavailable:no_secret":        {Root: root},
		"unsupported:body_truncated":   {Secret: "s", Root: root, Truncated: true},
		"unsupported:body_unparsed":    {Secret: "s", Degraded: true},
		"unsupported:count_tokens":     {Secret: "s", Root: root, CountTokens: true},
		"unsupported:protocol":         {Secret: "s", Root: map[string]any{"contents": []any{map[string]any{}}}},
		"unsupported:no_items":         {Secret: "s", Root: map[string]any{"messages": []any{}}},
		"unsupported:items_shape":      {Secret: "s", Root: map[string]any{"input": map[string]any{}}},
		"unsupported:protocol (image)": {Secret: "s", Root: map[string]any{"prompt": "p"}},
	}
	for name, in := range cases {
		fp, chains := computeTokenAnalysisFingerprint(in)
		require.False(t, fp.Comparable(), name)
		require.Nil(t, chains, name)
		require.Contains(t, name, fp.State)
	}
}

// 版本 = 规则版本 + 密钥 ID: 密钥不同的指纹互不匹配。
func TestTokenAnalysisFingerprintVersion(t *testing.T) {
	a := tokenAnalysisFingerprintVersion("secret-a")
	require.Equal(t, a, tokenAnalysisFingerprintVersion("secret-a"))
	require.NotEqual(t, a, tokenAnalysisFingerprintVersion("secret-b"))
	require.Equal(t, int32(tokenAnalysisFingerprintRuleVersion), a>>24)
	require.Positive(t, a)
}
