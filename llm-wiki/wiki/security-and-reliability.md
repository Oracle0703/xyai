# 安全与可靠性基线

## 认证与权限

后端中间件:

- JWT 用户认证: `backend/internal/server/middleware/jwt_auth.go`
- 管理员认证: `backend/internal/server/middleware/admin_auth.go`
- API Key 网关认证: `backend/internal/server/middleware/api_key_auth.go`
- Google/Gemini API Key 认证兼容: `api_key_auth_google.go`
- backend mode guard: `backend_mode_guard.go`

前端:

- `frontend/src/stores/auth.ts` 负责 token, refresh token, user, pending auth session。
- `frontend/src/api/client.ts` 自动附加 Authorization 和处理 refresh。
- `frontend/src/router/index.ts` 做 `requiresAuth`, `requiresAdmin`, backend mode, simple mode, payment/risk-control gate。

## 登录和 OAuth

认证路由集中在 `backend/internal/server/routes/auth.go`:

- 注册, 登录, 2FA 登录。
- refresh, logout。
- 忘记密码, 重置密码。
- LinuxDo, WeChat, OIDC, DingTalk, GitHub, Google OAuth。
- pending OAuth completion/bind/create account。

高风险入口都通过 Redis rate limiter 做 fail-close 限流。

## 限流与并发

常见机制:

- 登录/注册/验证码等认证入口 Redis rate limit。
- API Key rate limit 和 subscription check。
- User concurrency 和 account concurrency。
- RPM cache: user/group/account 维度。
- Gateway scheduling: sticky session wait, fallback wait, snapshot/outbox, slot cleanup。
- User message queue 可选串行/节流。
- 并发 slot 获取失败由 `backend/internal/handler/concurrency_error_response.go` 统一映射; `ConcurrencyCacheError` 必须返回 503 和明确的 service-unavailable 文案, 不应被归类为普通 429 限流。

相关路径:

- `backend/internal/middleware/rate_limiter.go`
- `backend/internal/service/concurrency_service.go`
- `backend/internal/repository/concurrency_cache.go`
- `backend/internal/service/rate_limit_service.go`
- `backend/internal/repository/rpm_cache.go`, `user_rpm_cache.go`
- `backend/internal/service/scheduler_snapshot_service.go`
- `backend/internal/repository/scheduler_cache.go`, `scheduler_outbox_repo.go`

## 幂等

幂等服务:

- `backend/internal/service/idempotency.go`
- `backend/internal/repository/idempotency_repo.go`
- schema: `backend/ent/schema/idempotency_record.go`

配置:

- `idempotency.observe_only`
- `idempotency.default_ttl_seconds`
- `idempotency.system_operation_ttl_seconds`
- `idempotency.processing_timeout_seconds`
- `idempotency.failed_retry_backoff_seconds`
- `idempotency.cleanup_interval_seconds`

新增关键写接口, 尤其支付, 余额, 订阅, 系统操作, 应考虑 `Idempotency-Key` 和失败重试语义。

## URL, CSP 与响应头

配置位于 `security`:

- `security.url_allowlist.enabled`
- `upstream_hosts`, `pricing_hosts`, `crs_hosts`
- `allow_private_hosts`
- `allow_insecure_http`
- `response_headers.enabled`
- `security.csp`
- `proxy_probe.insecure_skip_verify`
- `proxy_fallback.allow_direct_on_error`

相关代码:

- URL 校验: `backend/internal/config/config.go`
- Security headers: `backend/internal/server/middleware/security_headers.go`
- CORS: `backend/internal/server/middleware/cors.go`

生产环境要谨慎允许 HTTP, private hosts 和 proxy fallback direct, 避免 token 泄露和 SSRF 风险。

## 网关可靠性

网关相关可靠性配置:

- `gateway.max_body_size`
- `gateway.upstream_response_read_max_bytes`
- `gateway.proxy_probe_response_read_max_bytes`
- `gateway.stream_data_interval_timeout`
- `gateway.stream_keepalive_interval`
- `gateway.image_stream_data_interval_timeout`
- `gateway.max_line_size`
- `gateway.request_archive`
- `gateway.request_intercept`
- `gateway.openai_ws`
- `gateway.large_request`

请求链路会记录 request id, ops error, request archive, 并可使用 request intercept 动态改写/阻断。

OpenAI 官方 endpoint 的上游 payload 必须避免透传非官方 top-level thinking 字段:

- Responses API 使用 `reasoning` 表达推理控制, `thinking` 在发送上游前删除。
- Chat Completions raw 直转保留 `reasoning_effort`, `thinking` 在发送上游前删除。
- 新增上游协议字段时优先放入对应 endpoint 的显式 allow/sanitize 逻辑, 不做跨平台全局删除。

修改流式响应时要同时验证:

- SSE flush。
- 非流式响应 body 上限。
- 上游错误体截断。
- client disconnect。
- OpenAI Responses WebSocket fallback。
- OpenAI endpoint capability 会按账号能力限制 chat completions / embeddings 等入口; 本地 feature gate 拒绝要标记 ops business-limited, 避免污染上游 SLA。

## 日志与监控

日志:

- `backend/internal/pkg/logger`
- `log` 配置支持 level, format, output, rotation, sampling。
- Warn 及以上日志消息首字母大写。

运维监控:

- Ops service, repository, dashboard, alert, cleanup, system logs。
- 入口: `backend/internal/server/routes/admin.go` 中 `/api/v1/admin/ops/*`。
- 前端页面: `frontend/src/views/admin/ops/OpsDashboard.vue`。

## 数据保护

- 不把 token, OAuth refresh token, payment secret, API key 明文写入文档。
- TOTP encryption key 生产必须固定, 空值会导致重启后 2FA 配置失效。
- JWT secret 生产必须随机且稳定。
- 支付 provider 凭证和 webhook secret 应加密存储并验签。
