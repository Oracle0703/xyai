package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newTokenAnalysisForwardTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c
}

// testTokenAnalysisUpstreamRequest 构造与原生构造器相同形态的待发送请求(bytes.Reader 正文, 带 GetBody)。
func testTokenAnalysisUpstreamRequest(body string, header http.Header) *http.Request {
	req, _ := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/responses", strings.NewReader(body))
	for key, values := range header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	return req
}

func TestClassifyTokenAnalysisCacheKeyHandling(t *testing.T) {
	require.Equal(t, TokenAnalysisCacheKeyAbsent, classifyTokenAnalysisCacheKeyHandling("", ""))
	require.Equal(t, TokenAnalysisCacheKeyPassthrough, classifyTokenAnalysisCacheKeyHandling("k", "k"))
	require.Equal(t, TokenAnalysisCacheKeyInjected, classifyTokenAnalysisCacheKeyHandling("", "k"))
	require.Equal(t, TokenAnalysisCacheKeyDeleted, classifyTokenAnalysisCacheKeyHandling("k", ""))
	require.Equal(t, TokenAnalysisCacheKeyScoped, classifyTokenAnalysisCacheKeyHandling("k", "scoped-k"))
}

// 记录比较最终缓存键与入口值, 哈希覆盖最终 session_id/conversation_id 头;
// 同一最终值得到同一哈希, 无密钥时不出哈希。
func TestTokenAnalysisNativeForwardFacts(t *testing.T) {
	const secret = "0123456789abcdef0123456789abcdef"
	header := http.Header{}
	header.Set("session_id", "sess-1")
	header.Set("conversation_id", "sess-1")

	c := newTokenAnalysisForwardTestContext()
	beginTokenAnalysisForwardFacts(c, []byte(`{"prompt_cache_key":"client-key"}`))
	recordTokenAnalysisNativeForwardFacts(c, secret, TokenAnalysisForwardPathOpenAIResponses, testTokenAnalysisUpstreamRequest(`{"prompt_cache_key":"scoped-key"}`, header))
	facts := TokenAnalysisForwardFactsFromContext(c)
	require.NotNil(t, facts)
	require.Equal(t, TokenAnalysisForwardPathOpenAIResponses, facts.ForwardPath)
	require.Equal(t, TokenAnalysisCacheKeyScoped, facts.CacheKeyHandling)
	require.Equal(t, TokenAnalysisUsageCacheSourceUpstreamParsed, facts.UsageCacheSource)
	require.True(t, isTokenAnalysisHashHex(facts.UpstreamCacheKeyHash))

	// 同账号同原始键的下一次请求: 确定性改写得到同一最终值, 哈希相同。
	c2 := newTokenAnalysisForwardTestContext()
	beginTokenAnalysisForwardFacts(c2, []byte(`{"prompt_cache_key":"client-key"}`))
	recordTokenAnalysisNativeForwardFacts(c2, secret, TokenAnalysisForwardPathOpenAIResponses, testTokenAnalysisUpstreamRequest(`{"prompt_cache_key":"scoped-key"}`, header))
	require.Equal(t, facts.UpstreamCacheKeyHash, TokenAnalysisForwardFactsFromContext(c2).UpstreamCacheKeyHash)

	// 会话头变化即视为不同的缓存路由。
	other := http.Header{}
	other.Set("session_id", "sess-2")
	c3 := newTokenAnalysisForwardTestContext()
	beginTokenAnalysisForwardFacts(c3, []byte(`{"prompt_cache_key":"client-key"}`))
	recordTokenAnalysisNativeForwardFacts(c3, secret, TokenAnalysisForwardPathOpenAIResponses, testTokenAnalysisUpstreamRequest(`{"prompt_cache_key":"scoped-key"}`, other))
	require.NotEqual(t, facts.UpstreamCacheKeyHash, TokenAnalysisForwardFactsFromContext(c3).UpstreamCacheKeyHash)

	// 无密钥: 记录路径与处理方式, 不出哈希。
	c4 := newTokenAnalysisForwardTestContext()
	beginTokenAnalysisForwardFacts(c4, nil)
	recordTokenAnalysisNativeForwardFacts(c4, "", TokenAnalysisForwardPathOpenAIResponsesPassthrough, testTokenAnalysisUpstreamRequest(`{}`, nil))
	require.Equal(t, TokenAnalysisForwardFacts{
		ForwardPath:      TokenAnalysisForwardPathOpenAIResponsesPassthrough,
		CacheKeyHandling: TokenAnalysisCacheKeyAbsent,
		UsageCacheSource: TokenAnalysisUsageCacheSourceUpstreamParsed,
	}, *TokenAnalysisForwardFactsFromContext(c4))
}

// 每次转发尝试先清空: 前一次原生尝试的记录不能留给后一次(如改走协议转换)的尝试;
// 没有经过 Forward 入口时不记录。
func TestTokenAnalysisForwardFactsResetPerAttempt(t *testing.T) {
	c := newTokenAnalysisForwardTestContext()
	recordTokenAnalysisNativeForwardFacts(c, "s", TokenAnalysisForwardPathOpenAIResponses, testTokenAnalysisUpstreamRequest(`{}`, nil))
	require.Nil(t, TokenAnalysisForwardFactsFromContext(c))

	beginTokenAnalysisForwardFacts(c, nil)
	recordTokenAnalysisNativeForwardFacts(c, "s", TokenAnalysisForwardPathOpenAIResponses, testTokenAnalysisUpstreamRequest(`{}`, nil))
	require.NotNil(t, TokenAnalysisForwardFactsFromContext(c))

	beginTokenAnalysisForwardFacts(c, nil)
	require.Nil(t, TokenAnalysisForwardFactsFromContext(c))
}

func TestTokenAnalysisKeyedHashIsUnambiguous(t *testing.T) {
	require.NotEqual(t, tokenAnalysisKeyedHash("k", "ab", "c"), tokenAnalysisKeyedHash("k", "a", "bc"))
	require.NotEqual(t, tokenAnalysisKeyedHash("k1", "a"), tokenAnalysisKeyedHash("k2", "a"))
	require.Len(t, tokenAnalysisKeyedHash("k", "a"), tokenAnalysisHashHexLen)
}

func TestValidTokenAnalysisForwardFacts(t *testing.T) {
	require.Equal(t, TokenAnalysisForwardFacts{}, validTokenAnalysisForwardFacts(nil))
	require.Equal(t, TokenAnalysisForwardFacts{}, validTokenAnalysisForwardFacts(&TokenAnalysisForwardFacts{ForwardPath: "chat", CacheKeyHandling: "scoped"}))
	got := validTokenAnalysisForwardFacts(&TokenAnalysisForwardFacts{
		ForwardPath:          TokenAnalysisForwardPathOpenAIResponses,
		CacheKeyHandling:     "weird",
		UpstreamCacheKeyHash: "0123456789ABCDEF0123456789ABCDEF",
		UsageCacheSource:     "rewritten?",
	})
	require.Equal(t, TokenAnalysisForwardFacts{ForwardPath: TokenAnalysisForwardPathOpenAIResponses}, got)

	// 插件路径只保留缓存键处理方式, 即使归档里带了哈希与 usage 来源也不采信。
	got = validTokenAnalysisForwardFacts(&TokenAnalysisForwardFacts{
		ForwardPath:          TokenAnalysisForwardPathOpenAIResponsesPlugin,
		CacheKeyHandling:     TokenAnalysisCacheKeyScoped,
		UpstreamCacheKeyHash: "0123456789abcdef0123456789abcdef",
		UsageCacheSource:     TokenAnalysisUsageCacheSourceUpstreamParsed,
	})
	require.Equal(t, TokenAnalysisForwardFacts{ForwardPath: TokenAnalysisForwardPathOpenAIResponsesPlugin, CacheKeyHandling: TokenAnalysisCacheKeyScoped}, got)
}

// 实际 transport 是插件时降级: 外部插件是否改写请求未核实, 不提供哈希与 usage 来源;
// 插件未命中(rollout 0)的同类账号仍按原生路径记录。只依据本次实际选中的 transport。
func TestTokenAnalysisForwardFactsPluginTransport(t *testing.T) {
	for _, tt := range []struct {
		name     string
		extra    map[string]any
		rollout  int
		wantPath string
	}{
		{"plugin standard", map[string]any{}, 100, TokenAnalysisForwardPathOpenAIResponsesPlugin},
		{"plugin passthrough", map[string]any{"openai_oauth_passthrough": true}, 100, TokenAnalysisForwardPathOpenAIResponsesPlugin},
		{"plugin miss", map[string]any{}, 0, TokenAnalysisForwardPathOpenAIResponses},
	} {
		t.Run(tt.name, func(t *testing.T) {
			manager := &PluginManager{}
			manager.route.Store(&pluginRoute{pluginID: 1, rolloutPercent: tt.rollout, unavailable: "test fixture"})
			c := newTokenAnalysisForwardTestContext()
			c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
			cfg := &config.Config{}
			cfg.TokenAnalysis.FingerprintSecret = "fedcba9876543210fedcba9876543210"
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid"}},
				Body:       io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
			}}
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, toolCorrector: NewCodexToolCorrector()}
			svc.SetPluginManager(manager)
			account := newTestOAuthAccount(4502, tt.extra)
			account.Status, account.Schedulable, account.Concurrency = StatusActive, true, 1
			account.Credentials = map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"}

			_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.2","stream":false,"prompt_cache_key":"client-session","input":[{"type":"message","role":"user","content":"hi"}]}`))
			facts := TokenAnalysisForwardFactsFromContext(c)
			require.NotNil(t, facts)
			require.Equal(t, tt.wantPath, facts.ForwardPath)
			require.Equal(t, TokenAnalysisCacheKeyScoped, facts.CacheKeyHandling)
			if tt.wantPath == TokenAnalysisForwardPathOpenAIResponsesPlugin {
				require.Error(t, err, "unavailable plugin fixture fails the attempt")
				require.Empty(t, facts.UpstreamCacheKeyHash)
				require.Empty(t, facts.UsageCacheSource)
				require.Nil(t, upstream.lastReq, "request never reached the native transport")
			} else {
				require.NoError(t, err)
				require.True(t, isTokenAnalysisHashHex(facts.UpstreamCacheKeyHash))
				require.Equal(t, TokenAnalysisUsageCacheSourceUpstreamParsed, facts.UsageCacheSource)
			}
		})
	}
}

// 端到端: OAuth 账号经 Forward 时缓存键被按作用域改写, 记录的哈希等于对实际
// 发往上游的缓存键与会话头计算的值; 透传分支同理。
func TestTokenAnalysisForwardFactsThroughForward(t *testing.T) {
	const secret = "fedcba9876543210fedcba9876543210"
	for _, tt := range []struct {
		name  string
		extra map[string]any
		path  string
	}{
		{"standard", map[string]any{}, TokenAnalysisForwardPathOpenAIResponses},
		{"passthrough", map[string]any{"openai_oauth_passthrough": true}, TokenAnalysisForwardPathOpenAIResponsesPassthrough},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := newTokenAnalysisForwardTestContext()
			c.Request.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
			body := []byte(`{"model":"gpt-5.2","stream":false,"prompt_cache_key":"client-session","input":[{"type":"message","role":"user","content":"hi"}]}`)
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid"}},
				Body:       io.NopCloser(strings.NewReader("data: [DONE]\n\n")),
			}}
			cfg := &config.Config{}
			cfg.TokenAnalysis.FingerprintSecret = secret
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, toolCorrector: NewCodexToolCorrector()}
			account := newTestOAuthAccount(4501, tt.extra)
			account.Status = StatusActive
			account.Schedulable = true
			account.Concurrency = 1
			account.Credentials = map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-acc"}

			_, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, upstream.lastReq)

			facts := TokenAnalysisForwardFactsFromContext(c)
			require.NotNil(t, facts)
			require.Equal(t, tt.path, facts.ForwardPath)
			finalKey := gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String()
			require.NotEqual(t, "client-session", finalKey, "OAuth namespace scopes the cache key")
			require.Equal(t, TokenAnalysisCacheKeyScoped, facts.CacheKeyHandling)
			require.Equal(t, tokenAnalysisKeyedHash(secret, tokenAnalysisUpstreamCacheKeyHashDomain, finalKey,
				upstream.lastReq.Header.Get("session_id"), upstream.lastReq.Header.Get("conversation_id")), facts.UpstreamCacheKeyHash)
		})
	}
}

// testTokenAnalysisInboundItemsHash 按索引器口径计算入口(归档)正文的条目链末端。
func testTokenAnalysisInboundItemsHash(t *testing.T, secret, endpoint string, body []byte) []byte {
	t.Helper()
	root, err := parseTokenAnalysisRequestBody(body)
	require.NoError(t, err)
	fp, _ := computeTokenAnalysisFingerprint(tokenAnalysisFingerprintInput{Secret: secret, Endpoint: endpoint, Root: root})
	require.True(t, fp.Comparable())
	return fp.ItemsEndHash
}

// 缓存键按实际发送的原值哈希: 首尾空白不同的键原样发往上游, 证据不能把它们归并(审核 R5)。
// 网关未改写条目时, 最终条目哈希等于入口条目哈希。
func TestTokenAnalysisForwardFactsHashExactCacheKey(t *testing.T) {
	for _, tc := range []struct {
		name        string
		passthrough bool
		path        string
	}{
		{"standard", false, TokenAnalysisForwardPathOpenAIResponses},
		{"passthrough", true, TokenAnalysisForwardPathOpenAIResponsesPassthrough},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const secret = "synthetic-fingerprint-secret"
			var hashes []string
			for _, key := range []string{"cache-key", " cache-key "} {
				upstream := &httpUpstreamRecorder{resp: &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"id":"resp_1","model":"gpt-5.6-sol","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)),
				}}
				svc := newOpenAIImageGenerationControlTestService(upstream)
				svc.cfg.TokenAnalysis.FingerprintSecret = secret
				c, _ := newOpenAIImageGenerationControlTestContext(true, "test-client/1.0")
				account := newOpenAIImageGenerationControlTestAccount()
				account.Extra = map[string]any{"openai_passthrough": tc.passthrough}
				body, err := json.Marshal(map[string]any{
					"model": "gpt-5.6-sol", "stream": false, "prompt_cache_key": key,
					"input": []any{map[string]any{"role": "user", "content": "hello"}},
				})
				require.NoError(t, err)
				_, err = svc.Forward(context.Background(), c, account, body)
				require.NoError(t, err)
				require.Equal(t, key, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
				facts := TokenAnalysisForwardFactsFromContext(c)
				require.NotNil(t, facts)
				require.Equal(t, tc.path, facts.ForwardPath)
				require.NotEmpty(t, facts.UpstreamCacheKeyHash)
				require.Equal(t, hex.EncodeToString(testTokenAnalysisInboundItemsHash(t, secret, c.Request.URL.Path, body)), facts.UpstreamItemsHash)
				require.Len(t, facts.UpstreamRootHash, tokenAnalysisHashHexLen)
				hashes = append(hashes, facts.UpstreamCacheKeyHash)
			}
			require.NotEqual(t, hashes[0], hashes[1])
		})
	}
}

// 成功的重试剥离了加密 reasoning(审核 R1): 最终条目与入口不同, 入口前缀证据不能用于资格。
func TestTokenAnalysisForwardFactsDetectRewrittenRetryPrompt(t *testing.T) {
	c := newTokenAnalysisForwardTestContext()
	c.Request.Header.Set("User-Agent", "custom-client/1.0")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	upstream := &httpUpstreamSequenceRecorder{responses: []*http.Response{
		{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"invalid_encrypted_content","type":"invalid_request_error","message":"The encrypted content could not be verified."}}`))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_retry_ok","usage":{"input_tokens":62000,"output_tokens":2,"input_tokens_details":{"cached_tokens":2000}}}`))},
	}}
	const secret = "synthetic-fingerprint-secret"
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.TokenAnalysis.FingerprintSecret = secret
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg)}
	account := &Account{ID: 102, Name: "synthetic-openai", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
		Credentials: map[string]any{"api_key": "synthetic-key", "base_url": "https://api.openai.com"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true}}
	body := []byte(`{"model":"gpt-5.1","stream":false,"prompt_cache_key":"same-key","input":[{"type":"reasoning","encrypted_content":"gAAA","summary":[{"type":"summary_text","text":"keep summary"}]},{"type":"input_text","text":"hello"}]}`)
	_, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.Equal(t, 2, upstream.callCount)
	require.False(t, gjson.GetBytes(upstream.bodies[1], "input.0.encrypted_content").Exists())

	facts := TokenAnalysisForwardFactsFromContext(c)
	require.NotNil(t, facts)
	child := testTokenAnalysisClsRow(2, "c", 60, 60000, 2000)
	child.ForwardPath, child.UpstreamCacheKeyHash, child.UsageCacheSource = facts.ForwardPath, facts.UpstreamCacheKeyHash, facts.UsageCacheSource
	child.UpstreamItemsHash, child.UpstreamRootHash = facts.UpstreamItemsHash, facts.UpstreamRootHash
	child.ItemsEndHash = testTokenAnalysisInboundItemsHash(t, secret, c.Request.URL.Path, body)
	require.NotEqual(t, hex.EncodeToString(child.ItemsEndHash), facts.UpstreamItemsHash)
	require.False(t, tokenAnalysisChildSideEligible(&child))
}

// 构造器内部还会改写正文(如 DeepSeek 原生 Responses 提升工具结果图片): 记录必须取构造器
// 产出、实际交给 transport 的正文(2026-10-27 复核 N2)。
func TestTokenAnalysisForwardFactsHashBuilderFinalBody(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		name := "standard"
		if passthrough {
			name = "passthrough"
		}
		t.Run(name, func(t *testing.T) {
			const secret = "synthetic-fingerprint-secret"
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{"id":"resp_image","model":"deepseek-flash","output":[],"usage":{"input_tokens":60000,"output_tokens":2,"input_tokens_details":{"cached_tokens":1024}}}`))}}
			svc := newOpenAIImageGenerationControlTestService(upstream)
			svc.cfg.TokenAnalysis.FingerprintSecret = secret
			c, _ := newOpenAIImageGenerationControlTestContext(true, "test-client/1.0")
			account := openaiMappedDeepSeekResponsesImageAccount()
			account.Extra["openai_passthrough"] = passthrough
			body := []byte(`{"model":"deepseek-flash","stream":false,"prompt_cache_key":"same-key","input":[{"type":"function_call","call_id":"call_image","name":"view_image","arguments":"{}"},{"type":"function_call_output","call_id":"call_image","output":[{"type":"input_image","image_url":"data:image/png;base64,AQID"}]}]}`)
			_, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.Equal(t, "input_image", gjson.GetBytes(upstream.lastBody, "input.2.content.1.type").String(), "builder lifted the tool image")
			require.Equal(t, "data:image/png;base64,AQID", gjson.GetBytes(upstream.lastBody, "input.2.content.1.image_url").String())
			require.False(t, gjson.GetBytes(upstream.lastBody, "input.2.content.1.url").Exists(), "DeepSeek image payload has no duplicate URL field")
			facts := TokenAnalysisForwardFactsFromContext(c)
			require.NotNil(t, facts)
			items, root := tokenAnalysisUpstreamPromptHashes(secret, c.Request.URL.Path, upstream.lastBody)
			require.NotEmpty(t, items)
			require.Equal(t, items, facts.UpstreamItemsHash)
			require.Equal(t, root, facts.UpstreamRootHash)
		})
	}
}
