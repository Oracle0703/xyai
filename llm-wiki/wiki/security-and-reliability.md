# 安全与可靠性基线

## 认证与权限

后端中间件:

- JWT 用户认证: `backend/internal/server/middleware/jwt_auth.go`
- 管理员认证: `backend/internal/server/middleware/admin_auth.go`
- API Key 网关认证: `backend/internal/server/middleware/api_key_auth.go`
- Google/Gemini API Key 认证兼容: `api_key_auth_google.go`
- backend mode guard: `backend_mode_guard.go`
- 操作审计: `backend/internal/server/middleware/audit_log.go`
- 会话 IP/UA 绑定: `backend/internal/server/middleware/session_binding.go`
- 敏感操作 step-up: `backend/internal/server/middleware/step_up.go`

`APIKeyAuth` 对独占分组(exclusive group)做强制校验: 当 API Key 绑定的用户已不再被授权该独占分组时直接拒绝访问, 避免越权复用; 相关 middleware 单测在 `api_key_auth_test.go`。

API Key 认证缓存 miss 受 `api_key_auth_cache.lookup_concurrency` 约束, confirmed invalid/missing/malformed/deprecated-query 凭据还会进入进程内有界 abuse limiter；有效凭据和 Redis/DB 故障不消耗该预算。数据库侧 `auth_cache_invalidation_outbox` 只保存 key SHA-256, 通过两阶段 Redis invalidation 和本机 cache 清理让 API Key、用户、分组/授权变化在多实例收敛；健康端点不得回显明文凭据。

Google/Gemini 兼容认证必须复用 API Key 用户、分组与订阅校验, 不能只解析 `x-goog-api-key` 后跳过 group assignment; 相关边界集中在 `api_key_auth_google.go`。管理端修改用户角色时必须阻止删除/降级最后一名管理员。

管理端角色与细粒度权限:

- `AdminAuth` 支持 `admin` 和 `sub_admin`; 完整管理员与 Admin API Key 绕过细粒度检查。
- 子管理员权限以数据库最新用户为准, 不信任 JWT 内旧角色或前端菜单状态。检查键是 HTTP 方法 + Gin 路由模板, 白名单外默认拒绝并返回 `ADMIN_PERMISSION_DENIED`。
- 权限目录和白名单在 `backend/internal/service/admin_permission.go`; 当前仅有订阅管理、使用记录和 Token 分析。新增权限时必须同步后端 catalog/白名单、前端路由 meta/侧边栏/i18n 和允许/拒绝测试。
- 订阅权限是唯一含业务写操作的子管理员权限, 只允许 `POST /api/v1/admin/subscriptions/:id/reset-quota`; 使用记录清理、Token 立即索引、订阅分配/延期/撤销/恢复/删除始终拒绝。
- 依赖筛选数据必须使用 compact DTO。子管理员不得为筛选方便访问 `/admin/accounts`、`/admin/groups/all` 等完整管理接口。
- `admin_permissions` 只属于完整用户响应。`UserFromServiceShallow` 被 API Key、订阅、兑换码和用量日志等嵌套对象复用, 不得映射权限数组, 避免向无关响应扩散账号授权信息。
- 权限撤销后下一次管理请求立即失败。backend mode 下权限清空还必须结束前端会话, 避免“已登录但只能停在登录页”的脏状态。

OpenAI Agent Identity:

- `auth_mode=agentIdentity` 要求 PKCS#8 Ed25519 `agent_private_key` 和非空 `agent_runtime_id`; `task_id` 可缺省, 首次使用时按账号锁注册并持久化。私钥不得返回前端, runtime/task ID、AgentAssertion 和上游可能回显的凭据值在错误、日志与 ops 事件前也必须脱敏。
- task 被上游判定失效时每个调用链只允许恢复一次; 新 task 持久化后必须使该账号旧 WS 连接失效, 防止连接继续使用旧 assertion。account test、quota/usage query 及 HTTP/WS gateway 共用该认证边界。

管理面安全链:

- 管理端和用户管理面变更请求写入 `audit_logs`; action/path/request body/credential 必须先归一化、脱敏和截断。审计列表/详情受 admin auth, 全量清空不复用 sudo 窗口而要求现场 TOTP。
- `session_binding_enabled` 与 `step_up_enabled` 都默认关闭；管理设置请求使用可空字段, 旧客户端省略新字段时保持数据库现值。step-up 开启后, 账号/代理导出、S3 配置修改、备份创建/下载/恢复、管理员角色提升等敏感动作使用 15 分钟会话 grant；admin API key 不能取得该 grant, 未启用 TOTP 时明确返回 blocked error。开启前要求当前管理员已启用 TOTP, 关闭开关本身也必须二次验证。前端 `BackupView.vue` 的 S3 保存和 `SettingsView.vue` 的保存都必须经 `useStepUp`。
- 异步图片对象存储运行时设置位于 `/api/v1/admin/backups/image-storage`。修改存储目标会把生成内容导向外部账号, 因此 PUT 与备份 S3 一样必须通过 step-up；独立 `secret_access_key` 用 `SecretEncryptor` 加密入库, GET 清空 secret 只返回 configured 状态。选择复用备份 S3 时不再保存第二份凭据, 只保留图片 bucket/prefix 等覆盖项。
- Grok 视频生成成功后把 request ID 与原 user/API key 和实际上游账号绑定。status 与 `/videos/:request_id/content` 查找必须命中该绑定, 非所有者统一返回 not found；签名内容由原账号代理, 客户端只看到同源网关 URL。
- Composite group 不把客户端 public model 直接当作任意 provider 选择器：显式 route 的 target platform/endpoint 受 allowlist 与 group ownership 校验, 未命中时内置 detector 只识别已知模型族, 未知或歧义模型 fail-closed。resolved platform/model 放入请求 context 后再执行具体平台账号选择、配额和计费；根级 Responses/Chat/images/videos 入口仍在 composite 解析和 group gate 后执行本地 RequestArchive/RequestIntercept。
- `api_key_acl_trust_forwarded_ip` 是旧部署/升级兼容开关, 代码默认 `true`。开启时 API Key ACL、会话绑定和审计使用同一请求级快照（`Config.ForwardedClientIPSettings()`）, 原始转发头可绕过 Gin `server.trusted_proxies` 链；最多 16 个 `forwarded_client_ip_headers` 经规范化/去重后按配置顺序优先于 `CF-Connecting-IP`、`X-Real-IP`、`X-Forwarded-For`, 因此只应填写由可信边缘覆盖且外部不能伪造的 header。关闭后才由 Gin `server.trusted_proxies` / 直连 peer 作为统一权威来源, 此时自定义 header 不生效；trusted proxies 未配置或显式空时仅信任直连 peer。`deploy/config.example.yaml` 采用更安全的 false, 生产应按真实代理拓扑显式收紧。

Prompt Audit 安全与隐私边界:

- `backend/internal/securityaudit/` 叠加在既有内容审核链上, 默认关闭。已激活 blocking 模式对 Guard 不可用或非法响应 fail-closed 为 503；配置启动/reload 失败也只有在最近可解码的存储配置与全局 risk control 明确要求 blocking 时进入 degraded fail-closed。未观察到 blocking intent 时保持 off/已有非阻断模式, 让管理员仍能关闭或修复审计；async 入队失败不阻断当前请求, legacy moderation 仍照常执行。
- Guard endpoint 只接受 HTTP(S) URL, 禁止 userinfo/query/fragment, HTTP client 不继承代理且 HTTPS 最低 TLS 1.2；当前标准 dialer 允许管理员配置私网、loopback 和保留地址, 因而 endpoint 是管理员信任边界, 不是面向不可信用户的 URL 输入。
- Guard token 通过现有 `SecretEncryptor` 加密后写入 `prompt_audit_config`, 管理 API 只返回 `has_token` / `token_status`, 不回显明文。Prompt Risk judge 的内部签名头必须从 handler 经 `securityaudit.Request` 继续传给 legacy moderation, 防止上游 Prompt Audit coordinator 接入后恢复 HTTP 回环递归。
- async 扫描原文在 Redis 最长保留 30 分钟；job 表不存原文, 但 migration 182 后 event 的 `full_prompt` 会持久化最多 65,536 rune 的未脱敏文本并在管理员详情返回。该字段属于高敏数据, 数据库访问、备份、保留周期和事件删除权限必须按原始提示词处理, 不能沿用 181 migration 的早期“原文不进 PostgreSQL”假设。

前端:

- `frontend/src/stores/auth.ts` 负责 token, refresh token, user, pending auth session。
- `frontend/src/api/client.ts` 自动附加 Authorization 和处理 refresh。
- `frontend/src/router/index.ts` 做 `requiresAuth`, `requiresAdmin`, backend mode, simple mode, payment/risk-control gate。

邮箱绑定:

- `AuthService.SendEmailIdentityBindCode` 和 `BindEmailIdentity` 会复用注册邮箱后缀白名单策略(`registration.email_suffix_whitelist` 对应 setting key `registration_email_suffix_whitelist`)。空白名单允许任意邮箱; `["@qq.com"]` 这类精确后缀和 `"*.edu.cn"` 这类通配后缀均按注册策略执行。
- OAuth/合成邮箱用户补绑真实邮箱时也会执行该策略, 防止绕过注册入口限制。

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
- 订阅窗口维护属于鉴权正确性边界: 普通与 Google/Gemini API Key middleware 都必须在放行前同步完成 `EnsureWindowMaintenance` 并使用回读快照复核额度; 维护失败返回 500, 不允许用内存清零值 fail-open。
- User concurrency 和 account concurrency。
- 纯文本 embeddings/alpha search 使用 `gateway.text_max_body_size`（默认 32 MiB）, 多模态/media 继续使用 `gateway.max_body_size`（默认 256 MiB）；HTTP request header 另受 10 秒读取超时和 64 KiB 上限约束, 不设置全局 `WriteTimeout`。
- OpenAI WS ingress 生命周期使用独立于单 turn 槽位的 API Key 级 Redis lease。默认每 key 最多 64 条存活连接, lease TTL 60 秒、20 秒刷新; 容量满返回 WebSocket 1013, 缓存不可用或租约丢失时 fail-close。completed turn 之间默认 300 秒空闲超时, 两个限制都可用 0 显式关闭。
- RPM cache: user/group/account 维度。
- Gateway scheduling: sticky session wait, fallback wait, snapshot/outbox, slot cleanup。
- 管理员配置的临时不可调度规则在已知请求模型时写入 model rate-limit, 只隔离 `(account, model)`；401 或无法确定模型时保留账号级语义。pool mode 仍应用显式规则, 但不能把模型级失败扩大为整账号阻断。
- OpenAI scheduler sticky escape: 当 sticky 账号 TTFT EWMA 或错误率劣化到阈值以上时可临时跳过 sticky, 配置位于 `gateway.openai_scheduler`。
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

`POST /api/v1/admin/accounts/:id/duplicate` 只允许复制静态凭据账号。账号、groups priority 和 scheduler outbox 必须原子落库, 新账号默认 `schedulable=false`, quota/probe/cache/临时调度状态不得复制; credential shadow 和 OAuth/Agent Identity 等旋转凭据账号拒绝。幂等恢复身份绑定 admin actor、source account 和 `Idempotency-Key`; ambiguous commit 只能查询已创建副本, 不能重放 create side effect。

幂等响应入库前会做脱敏和长度裁剪; `MaxStoredResponseLen` 截断必须使用 UTF-8 安全边界(`truncateUTF8`), 避免把多字节字符切坏后写入不可解析响应。

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

CSP 注意点:

- 默认策略是 `config.DefaultCSPPolicy`; 部署可用 `security.csp.policy` 覆盖。
- `security_headers.go` 的 `requiredCSPDirectiveValues` 是"旧自定义策略缺新指令"的运行时补丁列表(支付 SDK 域名、`img-src blob:` 等都走这里)。给 CSP 加新的必需指令时**必须同时改默认串和该列表**, 否则覆盖了 policy 的存量部署不会生效。
- `img-src` 必须含 `blob:`: 图片生成页原图预览用 `URL.createObjectURL`, 缺了会被浏览器静默拦截(dev 无 CSP 头, 只在生产复现)。
- 前端渲染 public settings 时, `site_logo` 和 `doc_url` 必须经过 `sanitizeUrl`; 邮件模板中的 `site_name` 必须 HTML escape。不能依赖管理员输入天然可信, 对应回归测试在 layout URL sanitization 与 `email_html_escape_test.go`。
- `Server-Timing` 默认关闭。开启后只为 admin UI 或 allowlist 内的 authenticated user UI scope 收集; `X-Admin-UI-Request: 1` / `X-User-UI-Request: 1` 只是 scope signal, 不能替代认证。响应写出前必须再按 context role 与 user path allowlist gate: 管理员可读取已收集的 admin/user UI timing, 普通已认证用户只能读取 allowlisted user API; 未认证请求、公开 payment/webhook 和 AI gateway 不得返回 SQL/Redis/依赖耗时。CORS allow/expose headers 必须与两类 UI signal 和 `Server-Timing` 保持同步。
- embedded static cache 只信任 `assets/` 下文件名中的 8 字符 fingerprint; 只有这类资源可返回一年 immutable。unhashed assets、默认 `logo.svg`、favicon、HTML/SPA 和根级 API fallback 都不得长缓存; Caddy 不应覆盖后端的 fingerprint 判定。
- `UPDATE_GITHUB_TOKEN` 只允许发送到精确的 `https://api.github.com` release API, redirect 离开该 host 时必须剥离 Authorization；release asset/checksum 下载客户端保持匿名, 不能把 token 扩散到下载 URL 或日志。

生产环境要谨慎允许 HTTP, private hosts 和 proxy fallback direct, 避免 token 泄露和 SSRF 风险。

Grok endpoint 也属于 URL 信任边界:

- Grok OAuth 默认走 CLI subscription proxy; 空 `base_url` 或旧官方 `api.x.ai[/v1]` 值会归一到该 proxy。
- OAuth 自定义 `base_url` 必须通过 `xai.ValidateTrustedBaseURL`; 未允许 unsafe override 时, 普通第三方 host 回落默认 proxy。
- Grok API Key 无自定义值时走官方 `https://api.x.ai/v1`。上游模型同步只支持 API Key, OAuth 同步显式返回 unsupported; 同步路径在 `security.url_allowlist` 开启时通过 `AccountTestService.validateUpstreamBaseURL` 执行 upstream host/HTTPS 约束, 关闭时按 `allow_insecure_http` 只做格式校验。真实转发默认安全模式下, OAuth 自定义 `base_url` 经 `xai.ValidateTrustedBaseURL` 的可信 host allowlist, API Key 自定义 endpoint 由 `xai.Build*URL` / `ValidateBaseURL` 约束为公共 HTTPS 且路径为 `/v1`; `XAI_ALLOW_UNSAFE_URL_OVERRIDES` 开启后两者只做格式校验。不要把模型同步、OAuth 转发和 API Key 转发误认为同一 URL 校验路径。

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
- `gateway.openai_compact_model`
- `gateway.openai_ws`
- `gateway.large_request`

请求链路会记录 request id, ops error, request archive, 并可使用 request intercept 动态改写/阻断。

native OpenAI HTTP streaming Responses 的 first-output guard 默认关闭。启用后 deadline 从 attempt 开始并包含响应头等待; 首次语义输出前最多暂存 8 MiB, 只允许 keepalive 等非语义字节先行, 超时/溢出后清理 attempt 并最多换账号一次。该机制不覆盖 passthrough/WS, 也不能撤销已经发生的上游计算或用量, 因而 failover 重放可能造成重复上游计费; 开启前必须把该风险纳入成本与幂等评估。
OpenAI proxy stream circuit 只把非取消/非 deadline 的 Responses SSE 中途断流计入 proxy-ID 本地观察, 达阈值后临时跳过该代理；它不持久修改 proxy/account、不跨实例共享, 成功流清除观察且 TTL 自动恢复。日志只记录 proxy/account/request ID 与经过 `sanitizeUpstreamErrorMessage` 的错误, 不输出上游凭据或原始响应。

OpenAI 官方 endpoint 的上游 payload 必须避免透传非官方 top-level thinking 字段:

- Responses API 使用 `reasoning` 表达推理控制, `thinking` 在发送上游前删除。
- Responses passthrough 路径也必须调用 `sanitizeOpenAIResponsesOfficialRequestBody`, 不只是在 native/bridge 路径过滤; OpenAI API Key 透传时要保留 `reasoning`、删除顶层 `thinking`。
- Chat Completions raw 直转保留 `reasoning_effort`, `thinking` 在发送上游前删除。
- 新增上游协议字段时优先放入对应 endpoint 的显式 allow/sanitize 逻辑, 不做跨平台全局删除。
- `/responses/compact` 上游不接受 `tool_choice`, Codex image-generation bridge 对 compact 请求必须整体跳过工具注入/压缩桥接注入; compact 使用独立默认模型/账号级 mapping, 不应影响普通 Responses。

Anthropic OAuth/SetupToken 请求体默认启用客户端 dateline 归一化(`enable_client_dateline_normalization=true`):

- 实现位于 `backend/internal/pkg/anthropicfp` 和 `GatewayService.normalizeClientDatelineIfEnabled`。仅对 Anthropic OAuth/SetupToken 账号生效, API Key 账号和非 Anthropic 平台跳过。
- 归一化只扫描顶层 `system` 文本或 `messages[].content` 文本里的 `<system-reminder>...</system-reminder>` 块, 将客户端可能注入的撇号/日期分隔符指纹还原为 `Today's date is YYYY-MM-DD.`; 不扫描用户自由正文、tool_use/tool_result 或代码块。

历史 thinking block 过滤是协议感知的(`internal/service/thinking_protocol.go`), 不能跨上游一刀切:

- `ResolveThinkingProtocol(model)` 按厂商前缀判定协议族: `anthropic-strict`(claude-/opus-/sonnet-/haiku-, 缺失/非法签名应剥离)、`passback-required`(deepseek-/kimi-/moonshot-/glm-/minimax-/qwen*-thinking, 历史 thinking block 必须原样回传, 预过滤会导致上游 400)、`unknown`(其他, 保守不剥离)。
- 传入的 model 语义随调用路径不同: Anthropic gateway 传 `mappedModel`(账号 model mapping 后的上游 model), Gemini messages compat 传 `originalModel`(客户端 Anthropic 请求 model)。改 `FilterThinkingBlocksForRetry` 调用时要传对路径对应的 model。
- 国产模型 `thinking.type=enabled` 走 fallback: `ApplyThinkingEnabledFallback`(`gateway_request.go`)在 billingModel 判定后按需补 `reasoning_effort` 默认值; MiniMax M 系列 `thinking.type=enabled` 改写为 adaptive。Responses->Chat fallback 路径必须在 `billingModel` 算出后再调用。
- `/v1/chat/completions` 缺省 effort 注入(`applyDefaultOpenAIReasoningEffort`, 开关 `gateway.openai_default_reasoning_effort` 默认空=关闭): 同样在 billingModel 算出后判定, **强制模型门控**只对 `SupportsOpenAIReasoningEffort`(gpt-5.x / o 系列)的推理模型注入——向 gpt-4o / 第三方模型注入 `reasoning_effort` 会被官方上游 400 拒绝, 故门控不可省。用 `gjson.Exists()` 判定"是否已指定"而非归一化值, 避免覆盖客户端显式的 `none`/`minimal`; 模型名后缀(`gpt-5-high`)也视为已指定; gate `messages` 存在排除 Responses-shape 透传。默认空=零行为变更, opt-in。

Codex CLI only 客户端限制(`openai_client_restriction_detector.go` / `engine_fingerprint_signal.go`):

- 账号级 `codex_cli_only` 开启后才进入检测; `gateway.force_codex_cli` 是全局旁路放行, 仅用于兼容兜底。
- 检测顺序是黑名单 deny、官方 UA/originator、全局白名单、app-server 开闸、官方候选版本范围、engine fingerprint AND 硬门。黑名单为 OR deny; 白名单为 originator + UA 双因子 AND allow。
- `codex_cli_only_engine_fingerprint_signals` 是 JSON 数组, 每条 `required=true` 之间 AND, 同条 `match` 变体 OR; 默认种子只要求 `x-codex-` header prefix。旧 `codex_cli_only_allow_body_engine_fingerprint` 只作为迁移输入, 运行时看统一 signals。
- app-server 有全局开关 `codex_cli_only_allow_app_server_clients` 和账号级 `codex_cli_only_allow_app_server`; 任一开启会把未列名 app-server client 作为候选, 但仍需通过 engine fingerprint 门。
- 白名单条目可设置 skip engine fingerprint, 风险高于默认策略, 只应用于确实不发送 Codex 引擎指纹的可信第三方客户端。

Codex OAuth reasoning 续轮可靠性:

- `applyCodexOAuthTransform` 在请求带 `reasoning` 时补齐 `include:["reasoning.encrypted_content"]`; `filterCodexInput` 保留 reasoning item 的 `encrypted_content`/`content`/`summary`, 但剥离 `rs_*` id 并在缺失时补空 `summary`。不要恢复旧的"丢弃 reasoning item"策略, 否则多轮 Codex 推理上下文会丢失。
- Codex models manifest 的 401 只对 plain OpenAI OAuth 账号进入共享 token/rate-limit 状态机：普通无效 token 临时摘除并允许换号, `token_revoked` / `token_invalidated` 才永久禁用。Agent Identity 的 401 可能仅表示 task 失效, 必须保留独立 task recovery；自定义 API Key 上游的 `/models` 认证可能与 chat 不同, 401 不得禁用账号或自动 failover。

Prompt Risk 关键词规则与 LLM 语义复核(`content_moderation.go` / `prompt_risk_judge.go`):

- Prompt Risk 是内容审核前置阶段: 先从网关请求体抽取 prompt, 按独立 `prompt_risk_config` 做关键词/正则/等级评估; block 模式下命中拦截会短路请求, observe 只记录后继续既有内容审核。
- LLM 语义复核 judge 仅在关键词规则将要真正拦截、且命中等级在 `judge.trigger_levels` 内时触发; 调用 OpenAI 兼容 `/v1/chat/completions`, 失败、超时、非 2xx 或无法解析结果一律 fail-open, 降级为观察放行, 避免语义复核故障扩大为生产拦截。
- judge `base_url` 复用 `security.url_allowlist.upstream_hosts` 出站校验: 开启 `security.url_allowlist.enabled` 时必须命中 allowlist, 且私网/localhost 是否允许由 `allow_private_hosts` 决定; 关闭 allowlist 时只做 URL 格式和 scheme 校验。`fail_open` 不是配置项, judge 失败固定 fail-open。
- judge HTTP 调用有固定进程内并发闸门和成功响应体上限; 闸门满载、响应体超限、超时、非 2xx 或解析失败都按 judge error 记录并 fail-open。
- judge 走本网关或同域 base_url 时会形成真实 HTTP 回环。出站 judge 请求会携带 `X-Sub2API-Prompt-Risk-Judge`, 头值由 judge API key 和请求体派生; 入站内容审核只在 judge 当前启用且该头能用当前 judge key 与原始请求体校验通过时跳过整个 Prompt Risk stage, 防止二次 judge 或关键词规则反向拦截 judge 请求。不要把该头当作外部可配置开关暴露。
- 如果 judge 使用本网关 API Key, 仍建议使用专属 API Key / 分组, 便于计费、审计和故障定位; 不要复用普通用户 key。
- 管理端 Prompt Risk 在线测试器只调用 `TestPromptRisk` 评估关键词规则, 响应 `scope=keyword_rules_only` 且 `judge_evaluated=false`; 它不调用 LLM judge, 不能作为 judge 语义复核效果的验收入口。
- 内容审核关键词 block 会把命中的具体关键词写入 `content_moderation_logs.matched_keyword`, 管理端风险控制列表和详情展示该字段; 记录前仍需走既有输入摘要/脱敏边界。
- `risk_control_enabled`、`content_moderation_config` 和 `prompt_risk_config` 共用 stale-while-refresh runtime snapshot; blocked keywords 在 snapshot 构建时预编译 matcher。保存内容审核或 Prompt Risk 配置会立即替换对应 snapshot 部分, 通用 settings 成功保存总开关后也必须通过回调立即原子替换已有 snapshot 的 enabled 状态; 后台刷新失败继续使用最后一个有效快照并按 TTL 退避, 不得清空策略或把 settings 故障扩散到网关热路径。
cyber 内容审计硬阻断(`openai_cyber_policy.go` / `openai_cyber_session_block.go`):

- 上游 `error.code=="cyber_policy"` 命中时由 gateway 层 `MarkOpsCyberPolicy` 在 gin context 写一次性标记(同 turn 只记一次, WS 多轮每 turn 结束 `ClearOpsCyberPolicy`); compat 出口(`ForwardAsChatCompletions`/`ForwardAsAnthropic`)返回哨兵 `errOpenAICyberPolicyForwarded`, handler 落 tokens=0 免费用量行(对齐 `/v1/responses`): 不计费、不 failover、不二次写响应。前端 usage 请求类型新增 `cyber` 维度(label/badge/export, 与 stream 正交, 不映射 legacy stream)。
- 会话级自动屏蔽默认关, 开关 `cyber_session_block_enabled` + `cyber_session_block_ttl_seconds`(默认 3600s), runtime 经 `SettingService.GetCyberSessionBlockRuntime` 进程内缓存(60s)避免热路径 DB 往返。屏蔽 key 仅由显式会话标识派生(header session_id/conversation_id 或 body `prompt_cache_key`, 混入 apiKeyID 后 sha256); 无显式标识返回空串必须放行, 不退化到 user/apikey/内容派生。store 由 repository `gatewayCache` 类型断言接入(`CyberSessionBlockStore`), 测试 stub 不实现时屏蔽能力静默降级关闭。

OpenAI-compatible cache usage 字段可能出现在官方 `input_tokens_details.cached_tokens` / `prompt_tokens_details.cached_tokens`, 也可能是兼容上游顶层 `cache_read_input_tokens`、`cached_tokens`、`prompt_cache_hit_tokens`、`cache_write_tokens` 和 `cache_creation_input_tokens`。cache write/cache creation 必须与普通 input、cache read 拆成互斥计费桶; compatible cache read 补入 Responses/Chat details 时必须原位更新, 不能替换整个 details 对象后丢失已有 cache-write 字段。修改 Chat Completions fallback、Responses fallback、SSE usage parser 或 billing usage 提取时, 必须同时验证 DTO 响应体和 `OpenAIUsage` 计费字段。

Responses -> Chat 工具降级属于安全边界: custom、namespace、tool_search 的代理名必须可逆且无歧义; namespace 摊平名撞顶层工具或其他 namespace 时显式拒绝。`tool_choice` 只能指向转换后真实存在的工具, 被丢弃的服务端工具和不存在的名字必须一并删除, 防止上游 400 或把调用还原到错误工具。Grok Responses 现在也复用 `apicompat.ResponsesClientToolMapping`, 将 Codex client-side tools 适配为 xAI function tools 后在 streaming、non-streaming 与 SSE-to-JSON 回程恢复；顶层 `tools` 和 `additional_tools` 必须经过同一套撞名与选择校验。
Codex `additional_tools` input item 与顶层 `tools` 具有相同信任级别, 必须经 `EffectiveResponsesTools` 合并后复用上述过滤、撞名和回程规则; 不得只转发新增工具而绕过本地 `ResponsesToChatCompletionsRequestWithOptions` 的第三方参数过滤。Read 工具流式 delta 实时原样透传; 一旦收到 delta, `.done` 只关闭 block, 不再二次发送或 sanitize。只有非流式, 或流式完全没有 delta 而由 `.done` 携带完整参数时, 才执行 `sanitizeAnthropicToolUseInput`。`max_tokens` / `content_filter` stop reason 要映射为目标协议的标准终态, 避免连接悬挂或错误重试。
修改流式响应时要同时验证:

- SSE flush。
- 非流式响应 body 上限。
- 上游错误体截断。
- client disconnect。
- OpenAI Responses WebSocket fallback。
- Windows WebSocket reset/abort 错误分类(`WSAECONNRESET` / `WSAECONNABORTED`)。
- Chat Completions -> Responses bridge 的 item 生命周期完整性, 包括动态 item id 一致性、reasoning item、content part 和 tool call done 事件。
- 非流式上游错误透传不能二次写响应: `GatewayService` 写完整 JSON 错误后应标记 response committed, handler 层通过 `gatewayForwardErrorAlreadyCommunicated` 跳过通用 fallback; 流式中途错误仍要补协议级终止帧。
- OpenAI endpoint capability 会按账号能力限制 chat completions / embeddings 等入口; 本地 feature gate 拒绝要标记 ops business-limited, 避免污染上游 SLA。
- 模型不可用诊断会在 no-account 错误路径返回 404 `model_not_found`, 仅当配置池里没有任何账号支持请求模型时触发; 查询失败或无法判断时保守回到 503, 避免把瞬时故障误判为模型不存在。
- OpenAI `response.failed` 及上游错误事件透传前必须使用现有 sanitize 逻辑剥离冗长/敏感细节, 并套用 error passthrough/failover 规则, 不能硬编码 502; 避免把 verbose upstream body 直接暴露给用户或前端错误视图。HTTP 200 SSE 内的失败也要记录 ops error context。
- OpenAI failover error 可在服务内部保留上游 response headers, 供账号耗尽后的 handler 恢复退避提示; 客户端只允许收到通过统一 allowlist 校验的 `Retry-After`。数字必须为 1-604800 秒, HTTP 日期必须处于未来 7 天内, 且 header 不得含 CR/LF 或超过 128 字符; Authorization、Cookie、proxy、upstream detail 等其他 header 不得随 failover 恢复。
- Grok quota readiness 与 auto-pause 依赖 xAI rate-limit/entitlement headers; 未观察到 headers 时前端显示 unknown, 不应把 unknown 当作 exhausted。Grok quota 主动 probe 会写账号 `extra` 快照, reset 当前显式不支持。
- Grok prompt cache identity 只能从显式 conversation/prompt cache 线索或稳定消息前缀派生并与 API Key/模型边界组合; raw Chat 上游不能收到 Responses-only `prompt_cache_key`。纯客户端工具的 mixed native-tool 缓存路由只对已确认 Free OAuth 默认开启, 账号布尔开关与请求头可显式退出；paid/API Key/unknown 或非法配置 fail-closed, 因为注入 native search tools 可能改变自动工具选择。健康 quota headers 可以解除此前的 exhausted/rate-limit snapshot, 避免账号永久被误停用。
- Grok media 路由复用 OpenAI-compatible API key auth 与 group gate, videos 仅 Grok platform 可用; 非 Grok 请求必须本地 404 并标记 business-limited, 不应落到上游错误或污染 SLA。`grok-imagine` 别名归一和 multipart image edit 上传转换属于上游 payload sanitize 的一部分。
- Grok Web SSO 导入只在管理员路由接收 SSO key, 服务端经 Device Flow 换成 Build OAuth 凭据后创建账号; key、device code、access/refresh token 不得写入日志、wiki 或前端持久化。批量导入允许部分成功, 失败项只返回索引和脱敏错误。
- Grok OAuth credential failure 必须按 scope 隔离: 缺失/吊销/entitlement/proxy 等账号级问题只对选中账号执行 next-account retry 和隔离; provider 配置、共享状态或上游整体不可用属于 provider 级问题, 停止本请求继续切号, 不批量污染账号状态。永久/临时账号 mutation 以请求选中时的 credentials/token version/proxy snapshot 做 CAS; CAS miss 要回读并接受并发 refresh 的新 token, 不得覆盖新凭据。
- `POST /api/v1/admin/grok/oauth/reconcile` 默认 dry-run, 用游标列出缺失或临期凭据; apply 必须显式 `apply=true,dry_run=false`, destructive block 同样要求凭据 CAS 未变化。reconcile 与后台 refresh 共用 provider QPS/并发 gate, 防止并行入口绕过限速; 两条路径各自的当前周期熔断仍保持 provider scope。
- OpenAI 上游传输层错误(持久网络/代理故障)经 `handleOpenAIUpstreamTransportError`(`openai_upstream_transport_error.go`)在 Responses fallback 与 raw/passthrough 路径触发 failover 换账号, 持久故障临时摘除账号(temp unscheduled), 详见 `backend.md`。context-window 错误不应走 runtime block, 防止超上下文请求误伤账号可用性。
- Bedrock Claude Code 兼容由 `ApplyBedrockCCCompat` 统一清理 body 专有字段并过滤 `anthropic-beta` header; `context-management-2025-06-27` 是 Bedrock 支持 token, 不能被通用 beta 过滤误删。
- Vertex Anthropic service account 路径会对 `anthropic-beta` 做白名单过滤: 保留 Vertex 支持 token(如 `interleaved-thinking-2025-05-14`, `context-management-2025-06-27`), 剥离 Claude Code/OAuth 身份 token 和 Vertex 不支持 token(如 `advisor-tool`, `prompt-caching-scope`, `redact-thinking`, `thinking-token-count`)。最终 beta 为空时不下发 header; body sanitize 以最终 beta 为准。管理员 BetaPolicy block 规则仍先执行并可直接拒绝请求。
- 默认 BetaPolicy 对 `context-1m-2025-08-07` 只放行 Claude Sonnet 5 及其直连/Vertex/Bedrock ID 变体, 其余模型 fallback filter; 修改 Sonnet 5/1M context 能力时要同步 `DefaultBetaPolicySettings` 和 `gateway_beta_test.go`。

后台任务可靠性:

- 多实例周期性后台任务应通过 `LeaderLock`/`leader_lock_cache` 取得单主执行权; 新增会写数据库或刷新全局缓存的 runner/flusher 时, 必须明确是否需要 leader lock。
- scheduler cache 写快照时若单个账号包含不可 JSON 编码字段, `writeAccounts` 跳过该账号而不阻断整批快照; `SetAccount` 遇到同类账号会删除其 full/meta cache。`UpdateLastUsed` 重编码失败时同样删除该账号缓存并继续处理其他账号。
- user platform quota flusher 默认关闭, 开启后按批聚合写库; shutdown cleanup 必须 flush/stop, Wire `provideCleanup` 测试要覆盖。
- Spark 影子账号的凭据不落库且不参与凭据型导出; 401/refresh/privacy 操作要先解析母账号, 不能把母账号 token 错误永久写到 shadow。global 429/overload 不应连坐 spark 影子, 但母账号凭据过期、临时摘除、非 OAuth 仍要阻断 shadow。

用户可见错误:

- 用户侧失败请求视图由配置开关控制并 fail-closed; 后端返回前必须脱敏, 前端隐藏不是唯一保护。
- API Key name 等用户可控展示字段要进行 HTML 转义, 未授权 key 访问应避免泄露存在性。
- 用户支付 API 不得暴露内部 AI 渠道配置; 旧 `/api/v1/payment/channels` 及前端 client 已删除。支付方式展示只能使用 payment config/checkout-info 等专用 DTO。

## 日志与监控

日志:

- `backend/internal/pkg/logger`
- `log` 配置支持 level, format, output, rotation, sampling。
- Warn 及以上日志消息首字母大写。

运维监控:

- Ops service, repository, dashboard, alert, cleanup, system logs。系统日志持久化 `api_key_id` 和有界 `host`, 后端 `ListSystemLogs` / cleanup 支持按 API Key 与 host 过滤, 前端系统日志表有 KEY ID / host 筛选。
- ingress rejection 先在进程内按分钟、有界 8192 维聚合, 每 5 秒批量 upsert 到 `ops_ingress_reject_aggregates`; pending batch 最多 4 个, overflow/drop/flush failure 都进入 health。它避免拒绝风暴逐请求写库, 但不是边缘 DDoS 防护。
- 入口: `backend/internal/server/routes/admin.go` 中 `/api/v1/admin/ops/*`。
- 前端页面: `frontend/src/views/admin/ops/OpsDashboard.vue`。
- 告警指标新增 `account_temp_unscheduled_count`(临时摘除账号数, 配合 OpenAI transport failover); 规则配置在前端 `ops/components/OpsAlertRulesCard.vue` 与 `ops_alert_evaluator_service.go`。

## 数据保护

- 不把 token, OAuth refresh token, payment secret, API key 明文写入文档。
- TOTP encryption key 生产必须固定, 空值会导致重启后 2FA 配置失效。
- JWT secret 生产必须随机且稳定。
- 支付 provider 凭证和 webhook secret 应加密存储并验签。
- 后台异步生图对象存储的 SecretAccessKey 使用现有固定 `SecretEncryptor` 加密；未配置持久加密 key 时拒绝保存新 secret, 防止自动生成临时 key 导致重启后无法解密。复用备份 S3 时不重复持久化凭据, 读取 API 只返回 `secret_configured` 状态而不回显明文。
- Ollama Cloud web session 最多 16 KiB, 拒绝 CR/LF、重复 cookie、Set-Cookie attributes 和非 allowlist session cookie；规范化后使用固定 `TOTP_ENCRYPTION_KEY` 对应的 `SecretEncryptor` 加密写入 `account.extra`, 未配置固定 key 时拒绝保存。DTO、usage snapshot 和 audit log 均不回显 session 或原始 settings HTML；刷新只访问固定 `https://ollama.com/settings`, 响应上限 512 KiB, 并以账号/代理/session identity CAS 防止并发刷新覆盖新凭据。
- API Key 删除只 tombstone 原 key 以释放唯一约束, 不再把明文 key 复制到 `deleted_api_key_audits`；Ops 入口拒绝只保留有界聚合维度。旧明文审计表/列的 finalizer 必须在滚动升级全部完成并确认恢复点后人工执行。
- 退款状态可能进入 `REFUND_PENDING`: 这表示 provider 已受理但尚未最终成功。再次扣减余额/订阅前必须通过 provider query 终态确认, 并依赖 `PaymentAuditLog` 的 `REFUND_PENDING` / `REFUND_SUCCESS` / `REFUND_FAILED` 审计避免重复扣减。

当前上游已知限制:

- `backend/internal/service/image_storage.go` 会下载上游生图响应的 `data[].url` 后转存 S3, 当前上游默认 HTTP client 只有 timeout/大小限制, 没有私网、云元数据、DNS rebinding 或 redirect SSRF 防护。合并分支按用户要求不做本地修补; upstream 修复前不要在可被不可信自定义上游影响的环境启用 `image_storage`。
- async image 任务只由请求进程启动 goroutine, 没有持久 worker/recovery; 服务重启后 Redis 中的 `processing` 任务可能保留到默认 24h TTL。upstream 修复前要把滚动重启和任务可恢复性纳入运维预期。

## 相关页面

- [[README]]
- [[backend]]
- [[frontend]]
- [[ops]]
- [[data-and-domain]]
- [[security-and-reliability]]
- [[ai-workflow]]
