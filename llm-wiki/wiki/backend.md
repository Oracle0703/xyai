# 后端知识基线

## 技术栈与入口

- Go module: `github.com/Wei-Shaw/sub2api`, 当前 `backend/go.mod` 声明 Go `1.26.5`。
- HTTP 框架: Gin。
- ORM: Ent, 生成代码位于 `backend/ent/`, schema 位于 `backend/ent/schema/`。
- 依赖注入: Google Wire, 入口在 `backend/cmd/server/wire.go`, 生成文件 `wire_gen.go`。
- 后端主入口: `backend/cmd/server/main.go`。
- 主要命令:
  - 启动: `cd backend && go run ./cmd/server`
  - 构建: `cd backend && make build`
  - 生成: `cd backend && make generate`

## 0.1.171 合并增量

- `AuthService` 通过 `ProvideAuthService` 注入 `TencentCaptchaService` 与 `AliyunCaptchaService`。登录、注册、Passkey begin 和 OAuth start 可携带统一动作验证码 proof；公开 settings 只暴露腾讯 AppID 与阿里云 scene/prefix/region, secret 仅管理端 masked 展示和保存。
- 内容审核配置新增 `proxy_id`。`ContentModerationService` 通过 `ProxyRepository` 校验与解析代理, 运行时按 proxy ID 缓存代理 URL；保存配置时 `proxy_id=nil` 表示保留、`<=0` 表示清除, 测试 API 中 `nil` 沿用已保存代理、`0` 强制直连、`>0` 指定代理。
- `OpenAICodexVersionSyncService` 由 Wire 启动并在 cleanup 中停止。默认每 6 小时查询 `openai/codex` release, 只接受 `rust-v*` 稳定 tag, 将最新版本写入设置供 `codex_cli_only` 版本策略使用；`openai_codex_version_auto_sync_enabled=false` 可关闭, 失败时保留旧值。
- 管理端分组新增 `GET /api/v1/admin/groups/live-capability`, 返回当前服务端是否具备 OpenAI Live attestation 运行环境。前端启用 `allow_live` 时先读取该能力, 不支持时要求管理员确认。
- Responses `*subpath` 入口继续使用 middleware 形态的 `guardResponsesSubpath`, 且必须位于本地 `RequestIntercept` 之前, 避免拦截规则短路 path allowlist。

## 0.1.170 合并增量

- 分组利润控制只对 `openai`、`anthropic`、`gemini`、`grok`、`antigravity` token 分组生效。启用后, 只有账号上游倍率 `U <= D * (1 - margin - buffer)` 的候选进入调度, `D` 是请求定价时刻的有效下游倍率；OpenAI/Grok 文本请求在请求开始冻结定价时刻, WS 在每个 turn 冻结。普通 Gateway token 路径同样过滤候选, 获取并发槽后还用最新账号快照终检。分组配置读取失败显式 fail-open, 不能把该能力描述为绝对利润保证。独立图片/视频、Grok media、count_tokens 和 Live 显式跳过利润门。
- upstream billing probe 扩展到所有 API Key 平台账号（OpenAI、Anthropic、Gemini、Antigravity、Grok）。非 OpenAI 且 base URL 为空或是官方根域时直接记录 `unsupported`, 不请求 `/v1/sub2api/billing`。账号可 opt-in `upstream_billing_rate_sync_enabled`, 由 probe 以 CAS 把 `resolved_rate_multiplier` 写回账号倍率；关闭 probe 会同时关闭 rate sync, 启用 rate sync 后管理端单个或批量人工修改倍率必须拒绝。OAuth、Bedrock 和旧 `type=upstream` relay 账号不在探测范围。
- OpenAI OAuth 的非 compact HTTP Responses 默认保留原生 namespace 工具和历史 tool-call item 的 `namespace`, 以支持 Codex round-trip；compact 始终摊平/清理, API Key 标准 Responses 继续清理。仅 OpenAI OAuth 账号显式开启 `extra.openai_responses_flatten_namespaces` 时才为不兼容上游恢复摊平, WS v2 原生路径始终不摊平。

## 0.1.168 合并增量

- Passkey 由 `config.WebAuthn` 提供固定 RP ID/origins, `repository.PasskeyRepository` 持久化凭据, `PasskeySessionStore` 保存短期 ceremony session, `service.PasskeyService` 执行 discoverable login 与凭据管理。公开登录入口为 `/api/v1/auth/passkey/login/{begin,finish}`, 已认证管理入口为 `/api/v1/user/passkeys`; 注册和删除凭据必须再次校验账号密码。
- 模型广场入口为 `GET /api/v1/model-plaza`。路由先执行 PublicIP Panel 限流、`OptionalJWTAuth` 和 backend-mode user guard；`model_plaza_enabled=false` 时返回 404, `model_plaza_require_auth=true` 时匿名访问被拒绝。服务只展示允许暴露的分组/模型价格, 登录用户可取得自己的有效倍率。
- `UserRepository.Update` 与 `APIKeyRepository.Update` 改为显式字段 mask, 未声明列不写回；管理员余额 set/add/subtract 分别走原子 `SetBalance` / `AdjustBalance`, 避免旧实体快照覆盖并发计费。合并本地子管理员链路时, `UserUpdateFields.AdminPermissions` 必须随权限变更显式置位。
- Wire 源图新增 Passkey repository/session/service/handler 与 optional JWT middleware；生成后的 `wire_gen.go` 必须同时保留本地 `promptmetrics.NewExtension`、RequestArchive/RequestIntercept 与上游新增 provider。

## 启动流程

`backend/cmd/server/main.go`:

1. 初始化 bootstrap logger。
2. 解析 `-setup` 和 `-version`。
3. 若首次运行需要 setup:
   - Docker/环境变量自动 setup 时调用 `setup.AutoSetupFromEnv()`。
   - 否则启动 setup wizard 路由。
4. 正常服务模式:
   - `config.LoadForBootstrap()` 读取配置。
   - 初始化 logger。
   - Wire 构建 `Application`。
   - 启动 HTTP server。
   - 监听 `SIGINT/SIGTERM`, 优雅关闭。

## 依赖注入结构

`backend/cmd/server/wire.go` 的 `initializeApplication` 汇总:

- `config.ProviderSet`
- `repository.ProviderSet`
- `service.ProviderSet`
- `promptmetrics.ProviderSet`
- `securityaudit.ProviderSet`
- `payment.ProviderSet`
- `middleware.ProviderSet`
- `handler.ProviderSet`
- `server.ProviderSet`

很多后台服务在 Provider 中自动 `Start()`, 例如 token refresh, dashboard aggregation, usage cleanup, ops collector, scheduled report, account/subscription expiry, proxy expiry(代理有效期清理与回退), token analysis 自动索引, channel monitor runner, user platform quota flusher、Ops ingress rejection aggregator 和 auth-cache invalidation worker。新增后台服务时要同时考虑 Wire 注入, 启动时机, multi-instance leader lock 和 `provideCleanup` 停止逻辑。

0.1.171 之后 `provideCleanup` 还必须覆盖 `OpenAICodexVersionSyncService.Stop()`；合并 Wire 冲突时要同时保留本地 `TokenAnalysisService.StopAutoIndex()` 和上游新增后台服务停止步骤。

Prompt Audit 由 `backend/internal/securityaudit/` 提供。`Application.PromptAudit` 在 `main.go` 启动, `provideCleanup` 调用 `PromptService.Shutdown`; 配置默认关闭。`Coordinator` 始终保留既有内容审核：off 只执行 legacy moderation, async 先 best-effort 入队再执行 legacy, blocking 并行执行 Prompt Audit 与 legacy 并按阻断优先级合并结果。

API Key 鉴权缓存跨实例失效由 `backend/migrations/184_auth_cache_invalidation_outbox.sql`、`repository/auth_cache_invalidation_outbox_repo.go` 和 `service/auth_cache_invalidation_outbox.go` 组成。数据库 trigger 只把 API Key 的 SHA-256 写入 outbox；worker 使用 `FOR UPDATE SKIP LOCKED` 领取, 先清本机/L2 并发布 Redis 通知, 30 秒后再做安全二次失效, 失败按有界退避重试。`provideCleanup` 必须同时停止 worker、Redis subscriber 和 Ops runtime refresh；健康入口为 `GET /api/v1/admin/ops/auth-cache-invalidation/health`。

OAuth token refresh 使用按账号 ID 递增的游标分页, 每页默认 `candidate_page_size=200`; 每个平台独立并行处理, 并共享各自的进程内并发、QPS 和当前周期熔断闸门。默认 `provider_concurrency=4`、`provider_qps=2`、`provider_failure_threshold=3`、`attempt_timeout_seconds=15`、`cycle_timeout_seconds=240`; 单个平台连续失败只隔离该 provider, 不阻断其他 provider 的刷新页。周期中断时不会越过未完整处理的页, 后续从保存的游标恢复。

代理有效期与失败回退(`ProvideProxyExpiryService`, 每分钟扫描):

- `backend/internal/service/proxy_expiry_service.go` 定时 `SweepExpiredProxies`; `proxy_fallback.go` 的 `ResolveProxyFallbackTarget` 按 `fallback_mode`(`none` / `proxy` / `direct`)沿 `backup_proxy_id` 链解析过期代理应改投的目标, `RevertProxyFallback` 支持手动回切。
- 账号回切来源记录在 `accounts.proxy_fallback_origin_id`; 前端入口在 `ProxiesView.vue`(有效期/回退模式)与 `AccountsView.vue`(回切)。
- `provideCleanup` 的 `ProxyExpiryService` 步骤负责停止该后台任务。

## HTTP Server 与路由

入口:

- `backend/internal/server/http.go`: 提供 Gin engine 和 `http.Server`。
- `backend/internal/server/router.go`: 注册全局中间件和业务路由。
- `backend/internal/server/routes/*.go`: 分组注册路由。

全局中间件包含 recovery, request logger, session binding context, CORS, security headers, opt-in Server-Timing, Prompt Metrics 和 embedded frontend。`SessionBindingContext(cfg)` 把客户端 IP/UA 注入 context, 供 token 签发、会话绑定和审计统一读取; IP 口径与 API Key ACL 共用 `Config.ForwardedClientIPSettings()` 原子快照 / `api_key_acl_trust_forwarded_ip` 运行时设置。兼容开关默认 `true`, 可由管理端保存后即时更新；开启时按最多 16 个 `forwarded_client_ip_headers` 配置顺序优先读取自定义 CDN header, 再回落 `CF-Connecting-IP` / `X-Real-IP` / `X-Forwarded-For`, 关闭时统一使用 Gin `server.trusted_proxies` 链。trusted proxies 只有显式配置或通过 `SERVER_TRUSTED_PROXIES` 提供时才启用, 显式空或未配置均回到直连 peer。`SetupRouter` 还把 `OpsService` 注册为 ingress rejection recorder, 由鉴权/入口中间件记录有界拒绝维度, 不是逐请求写数据库。`server.enable_server_timing` 默认关闭; 开启后为 admin UI 和 allowlist 内的 authenticated user UI 请求创建 request-scoped collector, 汇总 SQL、Redis、外部 HTTP、cache 和总耗时。`X-Admin-UI-Request` / `X-User-UI-Request` 只提供 UI scope signal, 响应写出前仍由 context role 和 user path allowlist 做最终 gate; 公开 payment/webhook 路由和 AI gateway 不返回 `Server-Timing`。Prompt Metrics 的 `CaptureMiddleware` 继续在同一链路中挂载, 合并时不能二选一。嵌入前端只对 Vite `assets/` 下文件名含 8 字符 fingerprint 的资源设置 `public, max-age=31536000, immutable`; unhashed assets、`logo.svg`/`logo.png`、`favicon.ico`、HTML 和 SPA fallback 不使用静态长缓存。`deploy/Caddyfile` 不再重复强制静态 immutable 规则, 缓存判定由后端统一负责。embedded frontend 必须旁路根级 API `/alpha/search` 和 `/videos/*`, 避免未命中前端静态文件时错误回退到 SPA HTML。

`SetupRouter` 创建 `PanelRateLimiter`, 用 Redis 固定一分钟窗口保护 `/api/v1` 管理面：已认证 auth/user/payment/admin 路由按 user ID 叠加 `Global`, 用户 usage 聚合和 API Key daily usage 再叠加 `Heavy`, `/settings/public` 与邮件退订按可公开路由的客户端 IP 使用 `PublicIP`；回环、内网、链路本地地址不进入 public bucket。数据库 setting `panel_rate_limit_settings` 默认启用, `user_rpm=240`、`heavy_rpm=60`、`public_ip_rpm=300`、完整管理员豁免；任一 RPM 为 0 表示该档不限。配置使用 60 秒进程缓存, 当前节点保存后立即生效, 多节点最迟 60 秒；Redis 故障 fail-open。Prompt Metrics 的独立 admin router group 也必须挂 `Global`, 不能成为限流旁路。

路由主分组:

- `/api/v1/auth`: 注册, 登录, OAuth, refresh, logout, pending auth。
- `/api/v1/user`, `/api/v1/keys`, `/api/v1/usage`, `/api/v1/redeem`, `/api/v1/subscriptions`: 用户侧接口。
- `/api/v1/admin`: 管理端接口, 由 admin auth 保护。
- `/v1`, `/v1beta`, `/responses`, `/chat/completions`, `/images/*`: AI 网关兼容接口。
- `/antigravity/v1`, `/antigravity/v1beta`: Antigravity 专用兼容接口。
- 支付用户接口只暴露 config/checkout/plans/limits/orders/refund 等业务合同; 旧 `/api/v1/payment/channels` 已删除, 因其会泄露内部 AI 渠道配置。前端 `paymentAPI` 也不再包含对应 client 方法。
- 管理端 `GET /api/v1/admin/audit-logs[/:id]` 查询 append-only 操作审计, `POST .../clear` 必须现场 TOTP; `POST /api/v1/admin/users/batch-limits` 批量覆盖 concurrency/RPM, `concurrency=0` 仍表示不限。
- 管理端 `/api/v1/admin/prompt-audit` 提供 config 更新、节点 probe、runtime、事件列表/详情、单条/批量删除和带预览确认的筛选删除；路由受 admin auth 和全局 risk-control feature gate 约束。
- 管理端 `GET/PUT /api/v1/admin/settings/panel-rate-limit` 读写 Panel API 限流的总开关、用户/重查询/public IP RPM 和管理员豁免；它是数据库运行时设置, 不是 YAML 配置组。
- 通用 `PUT /api/v1/admin/settings` 会先保留原始 JSON field set, value-typed 字段若未出现在 payload 中则从 `SetMultiple` 更新集中删除, 留下数据库现值；显式发送 `false`、`0`、空字符串/数组仍是有效更新。pointer-typed 字段继续使用各自的 omitted merge/fail-closed 归一化。partial write 后进程缓存从数据库重读, 不能用请求 struct 的零值覆盖未发送字段。
- 管理端 `/api/v1/admin/ops/ingress-rejections` 与 `/api/v1/admin/ops/ingress-rejections/health` 查询入口拒绝聚合与运行态, `/api/v1/admin/ops/auth-cache-invalidation/health` 汇总 outbox、Redis subscriber、DB lookup 和 invalid-auth limiter 健康信息。

管理端账号合同:

- OpenAI Agent Identity 账号使用 `auth_mode=agentIdentity`, 凭据包含 PKCS#8 Ed25519 `agent_private_key`、`agent_runtime_id` 和运行期 `task_id`。缺失 task 时按账号串行注册; 上游判定 task 失效时单次恢复并持久化新 task, 随后使旧 WS 连接失效。account test、quota 查询、usage 查询及 HTTP/WS gateway 都支持该认证模式; quota reset credit 明确不支持。私钥和其他 secret 在管理端 DTO 中剥离; 上游错误、日志与 ops 输出还会防御性脱敏 runtime/task ID、assertion 及可能回显的凭据值。
- `POST /api/v1/admin/accounts/:id/duplicate` 只复制 API Key、upstream、Bedrock、service account 等静态凭据账号。账号、精确 groups priority 和 scheduler outbox 在同一事务创建; 新账号固定 `schedulable=false`, 不复制 quota、capability probe、cache 或临时调度等运行态。提供 `Idempotency-Key` 时以 admin actor + source account + key 建立恢复身份, 模糊提交结果只做只读恢复、不重复创建; credential shadow 和 OAuth/Agent Identity 等旋转凭据账号直接拒绝。
- 所有 API Key 平台账号都可启用 upstream billing probe: 设置与单个/批量 probe 路由位于 `/api/v1/admin/accounts/upstream-billing-probe/*` 和 `/:id/upstream-billing-probe`; snapshot 存入账号 `extra.upstream_billing_probe`, 调度可按上游倍率参与成本评分。创建/批量编辑 DTO 的 `ProbeEnabled` 决定账号是否参加自动探测, 创建端可在成功后立即 probe。非 OpenAI 官方根域账号不发送 Sub2API billing 请求而记录 `unsupported`；`upstream_billing_rate_sync_enabled` 只能随 probe 使用, 以 CAS 同步上游 `resolved_rate_multiplier`, 并与人工倍率修改互斥。API Key 自省入口 `GET /v1/sub2api/billing` 只返回当前 key 的计费倍率合同。
- Ollama Cloud 官方用量仅适用于 endpoint 为 `ollama.com` 的 OpenAI/Anthropic API Key 账号。管理路由位于 `/api/v1/admin/accounts/:id/ollama-cloud-usage` 及其 session/auto-refresh/refresh 子路由, 全局设置由 `/api/v1/admin/accounts/ollama-cloud-usage/settings` 管理。服务通过账号代理读取官方 settings HTML, 只把解析后的 plan、5 小时/7 天窗口、余额和模型用量 snapshot 写入 `account.extra`, 不改变账号调度状态。自动刷新默认全局关闭, 由模型请求记录 latest requested time 后触发: `debounce_minutes` 默认 1、限制 1-60, 连续请求最长等待 `interval_minutes` 默认 60、限制 15-1440；无新请求的账号不轮询。多实例使用 leader lock, 候选扫描按 due 时间推进并保留最低抓取量, 单周期最多 20 个账号、并发 4；手工刷新同账号 30 秒限一次。

管理端子管理员权限:

- 用户角色为 `admin`、`sub_admin`、`user`; 权限码与路由白名单集中在 `backend/internal/service/admin_permission.go`。
- `AdminAuth` 对 `admin` 和 Admin API Key 全量放行; `sub_admin` 每次请求从数据库加载最新角色、状态、TokenVersion 和 `admin_permissions`, 再按 HTTP 方法 + Gin `FullPath()` 精确匹配。未登记路由返回 `403 ADMIN_PERMISSION_DENIED`。
- 固定权限为 `admin.subscriptions`、`admin.usage`、`admin.token_analysis`; `GET /api/v1/admin/permissions/catalog` 仅完整管理员可访问, 前端用户配置弹窗以该接口为目录来源。
- 订阅权限只允许查看、`POST /subscriptions/:id/reset-quota` 和 compact 用户/分组筛选。分组筛选走 `/admin/subscriptions/search-groups`, 不得重新放行返回完整 `AdminGroup` 的 `/admin/groups/all`。
- 使用记录权限允许 usage、Dashboard 聚合/排行与 Ops 错误只读接口; 账号和分组筛选使用 `/admin/usage/search-accounts`、`search-groups` 的 `{id,name}` 响应。Token 分析权限只允许相关 GET 和选中用户趋势查询。
- 0.1.161 新增的 `/admin/ops/ingress-rejections`、`/admin/ops/ingress-rejections/health` 与 `/admin/ops/auth-cache-invalidation/health` 不在 `admin.usage` 白名单，当前仅完整管理员可访问。它们包含入口拒绝身份维度和鉴权缓存安全运行态；未知路由默认拒绝是既有 fail-closed 合同，后续若向子管理员开放必须先做显式产品与最小权限评审。
- backend mode 仅允许至少有一项权限的子管理员登录/刷新 token; 权限清空后不能继续保留 backend 会话。管理端合规查询/确认是所有已认证子管理员的公共白名单, 不代表业务管理权限。

用户侧用量接口:

- `GET /api/v1/usage`, `/stats`, `/dashboard/trend`, `/dashboard/models` 共用 `parseUserUsageFilters`, 支持 user scope 下的 `api_key_id` 所有权校验、`group_id`、请求模型、`request_type`/legacy `stream`、`billing_type`、`billing_mode` 和用户时区日期范围。
- `GET /api/v1/usage/dashboard/snapshot-v2` 为用户用量页图表聚合接口, 按 include 参数返回 trend/model/group 分布, 只暴露当前用户数据; 用户侧 stats 会清空管理端专属的 account/upstream endpoint 明细。
- 网关会把客户端显式会话 header 归一后写入 `usage_logs.session_id` 并通过管理端/用户侧 UsageLog DTO 返回；仅接受有效 UTF-8、去空白后不超过 255 rune 且不含控制字符的值。该字段只用于用量关联, 不参与 sticky routing、账号选择、request ID 或 prompt cache identity。
- 管理端 `GET /api/v1/admin/usage` 额外支持 `request_id` 去空白后的精确等值筛选；该维度只作用于列表 SQL, 不改变用户侧聚合接口合同。模型统计的 `upstream` 维度在 `upstream_model` 为空时回落实际 `model`, `mapping` 维度显示 requested 与该上游口径, 避免错误回落到 requested model。

管理端组织用量报表:

- 完整设计见 `docs/features/organization-usage-report-design-cn.md`；趋势图见 `docs/features/organization-usage-trend-chart-design-cn.md`。
- `GET /api/v1/admin/usage/organization-report/summary` 返回组织概览、三组组织汇总、日/周/月 champions 和分页用户摘要; `GET .../periods` 返回有用量的 user-period 明细; `GET .../trend` 返回筛选范围内按 day/week/month 聚合、服务端补零且截止 `data_through` 的连续时间序列（无 user 维、不分页）。三者沿用 `/api/v1/admin` 的管理员认证与合规 guard。
- 三层边界独立为 `OrganizationUsageRepository`、`OrganizationUsageService` 和 `admin.OrganizationUsageHandler`; SQL 实现在 `internal/repository/organization_usage_repo.go`, 不扩张 `UsageLogRepository` 或 `DashboardHandler`。
- 日期合同是固定 `Asia/Shanghai` 的 `YYYY-MM-DD` 闭区间, service 转成 UTC 半开区间后查询; 最多 366 个自然日。SQL 先用原始 `usage_logs.created_at >= start AND created_at < end` 收敛, 再按北京时间分日/周/月桶; 周为周一到周日, 跨选区周期会裁剪起止日期并标记 `partial=true`。
- 可选 `as_of` 必须是严格 RFC3339/RFC3339Nano; service 将其规范化为 UTC 并裁剪到不晚于服务端当前时间, 响应回显 canonical `as_of`。usage 查询上界再取 canonical `as_of` 与日期 end 的较早值, 早于范围起点时钳成空用量区间。该值不是密码学签名或服务端 snapshot id。
- summary 从 active 且未删除用户出发 LEFT JOIN 范围用量, 因此保留零用量用户; periods 只返回存在用量的 user-period。组织、粒度、排序字段和排序方向均为严格 allowlist, 非法值返回 400。
- PostgreSQL integration 与 30/90/366 天性能基线见 `backend/internal/repository/organization_usage_repo_integration_test.go`、`organization_usage_explain_integration_test.go` 和 `docs/features/organization-usage-report-performance-cn.md`。600 用户/219,600 logs 的 90 天 Summary items 曾因三个 peak CTE 对 `ranked_periods` 各循环扫描 600 次达到约 11 秒; 显式物化 peak 的诊断候选约 418 ms。现有时间索引不是该慢计划根因, 后续先修 peak 连接形状, 再减少导出分页重复查询。

管理端选中用户用量趋势:

- `GET /api/v1/admin/dashboard/users-trend` 保留兼容双模式: 未传 `user_ids` 时继续返回现有 Top N; 显式传入逗号分隔 `user_ids` 时精确查询所选集合。显式空值、空片段、非法/非正整数或超过 5 个唯一用户返回 400; ID 去重升序后进入 Service/Repository 和 30 秒缓存键, 选人模式忽略 `limit`。
- 精确选人模式要求 `start_date`/`end_date` 为严格 `YYYY-MM-DD` 北京时间闭区间, `granularity=day|hour`; day 最多 90 个自然日, hour 只允许同一天。Handler 转为 UTC 半开区间, Repository 用 `pq.Array` 绑定 ID, 先按 `(user_id, created_at)` 过滤, 再以 `created_at AT TIME ZONE 'Asia/Shanghai'` 分桶。
- Repository SQL 在 `internal/repository/usage_log_repo_trend.go`; 总 Token 固定为 input/output/cache creation/cache read 四类之和。已有 `idx_usage_logs_user_created` 对应用户等值 + 时间范围访问, 不新增 migration。PostgreSQL 合同和 opt-in 执行计划测试位于 `usage_log_repo_integration_test.go`、`user_usage_trend_explain_integration_test.go`。
- 响应沿用 `UserUsageTrendPoint`, 不新增 DTO。`dashboard/snapshot-v2` 显式传空用户集合, 继续保持旧 Top N 行为。

## 网关路径

`backend/internal/server/routes/gateway.go` 是网关路由入口。

关键行为:

- OpenAI Live 创建入口为 `POST /v1/live` 与 `POST /backend-api/codex/realtime/calls`, sideband 控制入口为 `GET /v1/live/:call_id` 与 `GET /backend-api/codex/:call_id`。只允许 `platform=openai` 且分组 `allow_live=true`, 账号还需具备 Live endpoint capability；本地 API Key/group gate 与 RequestArchive/RequestIntercept 均先于 handler。创建时把 call identity、加密 attestation 和 controller 状态存入 Redis, 用独立 Live lease 同时约束 account/user/API key 并发；会话结束、过期或租约丢失时关闭并写 `request_type=live` 用量行。服务端 attestation 仅 Apple Silicon macOS + 官方 ChatGPT App 可用, 客户端平台不受此限制。
- `platform=composite` 的 group 先由 `CompositeRouteResolver` 按显式 route registry 解析 public model、endpoint、目标平台和 upstream model；exact 优先 prefix、endpoint-specific 优先 any、更长 prefix/更低 priority/更低 id 依次胜出, 未命中再走内置模型检测, 仍不明确则 fail-closed。exact route 的空 `upstream_model` 保存时仍回填 `public_model`; prefix route 留空则透传本次具体请求模型, 不把同前缀的多个模型塌缩成 route prefix。解析结果写入 request context 并在 JSON/Gemini native 路径改写上游模型；后续账号选择、user-platform quota、计费、Ops/channel attribution 和 usage report 均使用具体目标平台。管理 API 为 `/api/v1/admin/groups/:id/composite-routes` 的 CRUD 与 preview。
- `/v1/messages` 根据 API Key 所属 group platform 分流到 OpenAI 或 Claude 兼容处理。
- `/v1/messages/count_tokens` 对 OpenAI group 走 Anthropic-compatible 到 OpenAI `/v1/responses/input_tokens` 的桥接; 不占并发槽、不写 usage。Grok group 因上游没有兼容计数端点, 在本地把 Anthropic 请求转换为 Responses 形状后用 tiktoken/tokenizer 估算；不选账号、不取凭据、不检查 billing、不请求上游、不占并发槽且不写 usage, 但仍经过 API Key 与分组中间件, 并同时支持 `/v1` 与根级路径。其他不支持的平台继续走既有 count-tokens 错误/handler。
- `/v1/responses` 和根级 `/responses` 支持 OpenAI Responses API。
- `/v1/chat/completions` 和根级 `/chat/completions` 支持 OpenAI Chat Completions。
- `/v1/embeddings` 和根级 `/embeddings` 仅 OpenAI platform 支持。
- `/v1/images/generations` 和 `/v1/images/edits` 对 OpenAI platform 走 OpenAI images handler, 对 Grok platform 走 Grok media handler; 根级别名 `/images/generations`、`/images/edits` 也保留 `RequestArchive` / `RequestIntercept` 中间件链。
- `/v1/images/generations/async`, `/v1/images/edits/async` 和 `GET /v1/images/tasks/:task_id`（含根级别名）先把任务状态写入 Redis, 再在进程内 goroutine 复用同步 image handler; `image_storage` 启用且 S3 凭据齐全时才开放, 结果会转存对象存储后以短 URL 回写。运行时配置由 `ImageStorageSettingService` 缓存解析：管理端 `/api/v1/admin/backups/image-storage` 保存的 `image_storage_config` 优先于 YAML/env, 可复用备份 S3 凭据；保存后立即失效 uploader/resolver 缓存, 下一次请求即时重建；尚未保存数据库设置时回落 `config.yaml`/环境变量。关闭功能后仍允许轮询已创建任务, 但不再新建任务。路由继续经过 API Key、group gate、RequestArchive 与 RequestIntercept。
- `/v1/images/batches` 是 batch image 用户侧任务接口族: submit/list/models/get/items/item content/download/cancel/delete/delete outputs。入口在 `backend/internal/handler/batch_image_handler.go`, service/repository 分别在 `backend/internal/service/batch_image*.go` 与 `backend/internal/repository/batch_image*.go`; 受 API Key 用户、分组 `allow_batch_image_generation` 与批量生图折扣/hold multiplier 约束。
- Grok/xAI 使用 OpenAI-compatible gateway 入口, platform 为 `grok`; OAuth 订阅账号走 CLI subscription proxy, API Key 账号走官方 credit-backed API, 两类账号均支持 Responses/Chat 兼容文本与推理流量。管理端还可通过 `POST /api/v1/admin/grok/sso-to-oauth` 批量提交 Web SSO key, 后端走 xAI Device Flow 转换为 Build OAuth 凭据; 导入后会做最小 probe, 但单个失败不能覆盖已成功创建的账号。上游模型同步当前只支持 Grok API Key, OAuth 账号会返回 `unsupported`; 模型同步通过 `AccountTestService.validateUpstreamBaseURL` 校验, `security.url_allowlist` 开启时执行 upstream host/HTTPS 约束, 关闭时按 `allow_insecure_http` 只做格式校验。真实转发先由 `Account.GetGrokBaseURL` 选择地址: 默认安全模式下 OAuth 自定义地址必须通过 `xai.ValidateTrustedBaseURL` 的可信 host 约束, API Key 自定义地址由 `xai.Build*URL` / `ValidateBaseURL` 限制为公共 HTTPS 且路径为 `/v1`; 开启 `XAI_ALLOW_UNSAFE_URL_OVERRIDES` 后两者退化为格式校验。模型同步与真实转发不能描述为同一校验链。
- Grok media 路由支持 `/v1/images/generations`, `/v1/images/edits`, `/v1/videos/generations`, `/v1/videos/edits`, `/v1/videos/extensions`, `/v1/videos/:request_id`, `/v1/videos/:request_id/content` 及根级 images/videos 别名; 非 Grok platform 访问 videos 返回本地 404 feature gate。generation 要求账号通过 `grok_media_generation` capability：显式禁用、Free tier 或负面 probe 不调度；billing 未观察账号只作为候选, 转发前必须现场 probe, probe 不可用或不能形成付费资格证据时 fail-closed。生成成功会把 request ID 按 user/API key 绑定到实际账号, status/content 只允许原请求所有者并强制复用该账号；content 由网关代理签名上游内容并保持同源 URL。scheduler cache 必须保留资格与 billing snapshot。`grok-imagine` 图片别名会归一到 `grok-imagine-image-quality`, Grok 4.5 正式模型别名由 `internal/pkg/xai/models.go` 维护, video model 透传到 xAI `/v1/videos/*` 并按分辨率和生成秒数计费；image/edit/reference payload 的 `image_url` 会规范为上游 `url` 字段。
- `/v1beta/models/*` 提供 Gemini SDK/CLI 兼容。
- Antigravity 原生 OAuth 账号可从 OpenAI Chat Completions 或 Responses 请求经 `ForwardAsChatCompletions` / `ForwardAsResponses` 转为 Anthropic shape, 再按映射模型族转 Antigravity `v1internal:streamGenerateContent`；Gemini 模型走 Gemini request clean/identity patch, 其他模型走 Claude transform。兼容层保留 token limit、reasoning effort、工具调用和 usage, 对 stream/non-stream 都拒绝只有 usage 而没有语义输出的响应并触发 failover；仅允许 native OAuth 账号, 认证拒绝与网络/空流按既有账号切换边界处理。
- `GET /v1/models` 与根级 `GET /models` 共用 platform-aware handler: OpenAI group 且带 `client_version` 时返回 Codex manifest, 其他客户端继续返回 OpenAI-style model list。`GET /backend-api/codex/models` 仍由 `openai_codex_models_handler.go` / `openai_codex_models_service.go` 代理 manifest; plain OAuth manifest 401 会进入共享账号错误状态机并允许 failover, 普通无效 token 临时摘除账号, `token_revoked` / `token_invalidated` 永久标错。Agent Identity 的 task-scoped 401 继续走独立恢复, 自定义 API Key manifest 401 不禁用账号。`/backend-api/codex/responses` 继续作为 Codex 直连别名, 所有入口都受 API Key 与 group 校验保护。
- Codex standalone search 同时注册 `/v1/alpha/search`、根级 `/alpha/search` 和 `/backend-api/codex/alpha/search`, 由 `openai_alpha_search.go` 按 OAuth/API Key 账号选择 ChatGPT Codex 或 OpenAI 官方 endpoint。三路入口都经过 API Key/group gate; 本地根级与 Codex direct 路由继续经过 RequestArchive/RequestIntercept。只有上游 2xx 成功响应产生 `WebSearchCalls=1` 并进入按次计费, 上游错误原样透传且不计费。
- alpha search 的账号选择必须包含 OpenAI API Key 账号, 不可只调度 OAuth/PAT; API Key 走官方 endpoint, OAuth/PAT 继续走 Codex/Responses fallback。

OpenAI 上游请求会按官方 endpoint 做字段过滤:

- `/v1/responses` / Responses 透传路径删除 top-level `thinking`, 保留官方 `reasoning`。
- `/v1/chat/completions` raw 直转路径删除 top-level `thinking`, 保留官方 `reasoning_effort`。
- `/v1/chat/completions` 入口可选注入默认 `reasoning_effort`: 配置 `gateway.openai_default_reasoning_effort`(默认空=关闭)非空时, `applyDefaultOpenAIReasoningEffort` 在 `ForwardAsChatCompletions` 分流前对入站 body 注入一次, 同时覆盖 raw 直转与 CC→Responses 两条上游形状; 注入在 `json.Unmarshal` 前完成, 计费/用量日志自然读到。仅对**映射后** billingModel 命中 `SupportsOpenAIReasoningEffort`(gpt-5.x / o1·o3·o4)的推理模型注入; 客户端经 `reasoning_effort` / `reasoning.effort` / 模型名后缀(`gpt-5-high`)已指定时不覆盖; gate `messages` 存在以排除 Cursor 的 Responses-shape(`input`)透传。非推理模型(gpt-4o 等)不注入, 否则官方上游 400 unsupported parameter。
- OpenAI 分组可配置 `reasoning_effort_mappings` 与 `max_reasoning_effort`。网关只处理请求中显式存在的 `reasoning.effort` / `reasoning_effort`: 先执行一次精确映射, 再按 `minimal < low < medium < high < xhigh < max` 应用上限；未知值、非字符串和省略字段保持不变, 不覆盖上游默认。策略只允许 OpenAI platform, 同时接入 HTTP 与 WS ingress/passthrough；分组 DTO、API Key auth cache 和 scheduler snapshot 都必须携带这两个字段。
- `/v1/chat/completions` raw 直转到 GLM(`glm-*`)上游前会归一化 reasoning effort: `reasoning.effort` 或 `reasoning_effort` 中的 `low`/`medium`/`high` 映射为 `high`, `xhigh`/`extrahigh`/`max`/`ultracode` 映射为 `max`; 其他上游不受影响。
- GPT-5.6 支持 `max` reasoning effort; effort 提取按 `upstreamModel -> billingModel -> originalModel` 候选顺序判断, 避免账号映射或模型后缀归一化后丢失 `max`/后缀语义。该候选顺序同时覆盖 WS passthrough usage metadata。修改模型映射时要同步 raw Chat、Responses fallback、WS ingress 和相关候选测试。
- Anthropic/Gemini 等非 OpenAI 协议的 thinking 映射不复用该过滤规则, 需按各自协议能力单独处理。
- Anthropic OAuth/SetupToken 转发默认启用客户端 dateline 归一化, 只改写 `system` 或 `<system-reminder>` 内的 `Today's date is YYYY-MM-DD.` 指纹变体, 还原 ASCII 撇号和 `-` 分隔符; API Key 账号和普通用户正文不扫描。
- OpenAI Responses SSE 终止事件的 usage 可能在顶层 `usage` 或 `response.usage`; Chat Completions 和 Messages 的 buffered/streaming 转换及计费解析必须按实际 JSON 路径保留 `input_tokens_details.cached_tokens`、`cache_read_input_tokens`、`prompt_cache_hit_tokens`(DeepSeek Context Cache 命中)以及 `cache_write_tokens` / `cache_creation_input_tokens` 等缓存 token 字段; `prompt_cache_miss_tokens` 仍按普通 prompt/input token 口径计费, 不映射为 cache creation。hosted image tool 的图片 token 可能位于 `tool_usage.image_gen` 或 `response.tool_usage.image_gen`; 只在标准 usage 对应图片字段为 0 时补入 `ImageInputTokens` / `ImageOutputTokens`, 防止重复计费。GPT-5.6 cache write 必须从普通 input 中拆出并按官方 cache-write 价格计费, 显式 0 价格也不能被 fallback 覆盖。
- Responses/Chat 双向桥接需保留 `parallel_tool_calls`; Responses `text.format` 与 Chat `response_format` 支持 `json_object` / `json_schema` 映射。OpenAI-compatible Responses -> Chat fallback 仍要经过本地 `ResponsesToChatCompletionsRequestWithOptions`, 以保留第三方上游的 temperature/max token 过滤策略。v0.1.151 起 custom/freeform 工具降级为 function 后必须在回程还原为 `custom_tool_call`; namespace 子工具使用稳定摊平名并在回程还原 namespace; `tool_search` 使用同名代理 function 并还原为 `tool_search_call(execution=client)`。工具名撞名或 tool_choice 指向被丢弃/不存在工具时必须拒绝或删除, 不能把无效选择发给 Chat 上游。
- v0.1.153 起 Codex 还可能把客户端工具放在 Responses input 的 `{"type":"additional_tools","tools":[...]}` item 中。`EffectiveResponsesTools` 必须把它与顶层 `tools` 合并后交给本地 options adapter, custom/namespace/tool_search 的可逆映射和 tool choice 校验对两种来源一视同仁。
- native Responses namespace 在不支持原生 namespace 的账号/传输上使用 `namespace__tool` 稳定摊平, 并在 streaming、non-streaming、SSE→JSON 和 passthrough 回程恢复原始 namespace/tool 结构。OpenAI OAuth 非 compact HTTP 默认保留原生 namespace, 只有账号显式开启 `extra.openai_responses_flatten_namespaces` 才摊平；compact 始终清理, WS v2 原生转发始终保持原文。该处理发生在本地 options adapter 和账号 transport 决策之后, 不能破坏第三方 Responses→Chat 过滤或把摊平名泄漏给客户端。
- OpenAI API Key 的标准 HTTP Responses 请求在转发前会删除 input item 顶层残留的 `namespace`, 但保留 content 内部同名业务字段；续链 input 的 message/function-call item ID 只有符合官方前缀合同才保留, 非法客户端回放 ID 由 `sanitizeOpenAIResponsesInputItemIDs` 线性扫描删除。该清理不能扩大到 OAuth 非 compact HTTP、WS v2 或本地 compatible bridge。
- Responses -> Anthropic 流式桥对 `Read` 工具的 `function_call_arguments.delta` 要实时原样转成 `input_json_delta`, 不再等待 `.done` 才一次性发送。已收到 delta 时 `.done` 只关闭 block, 不再二次发送或 sanitize 完整参数; 仅非流式转换, 或流式完全没有 delta 而由 `.done` 携带完整参数时, 才调用 `sanitizeAnthropicToolUseInput`。Anthropic `stop_reason=max_tokens` 映射为 Responses `response.incomplete` + `max_output_tokens`, Responses incomplete 的 `content_filter` 映射为 Chat `finish_reason=content_filter`。
- Responses -> Anthropic fallback 在解析请求前先把 Codex `additional_tools` 提升到顶层并把 custom/namespace/tool_search 降级为无歧义 function tools, 流式与非流式回程再恢复原工具名和 namespace；namespace 名称同时在 arguments delta/done 事件恢复。`function_call_output.output` 可为字符串或 content-part 数组, 数组中的 text 与 data-URI image 分别转为 Anthropic text/image blocks；tool use/result 邻接和孤儿清理合同仍由 `normalizeAnthropicToolPairing` 保证。

OpenAI/Codex 兼容桥:

- `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go` 负责 Chat Completions 与 Responses 双向桥接; streaming bridge 会按 Responses 生命周期发出 `response.created`, `response.output_item.added`, `response.content_part.added`, `response.output_text.delta/done`, `response.output_item.done`, `response.completed`。
- Responses/Anthropic 双向转换必须保留 `cache_creation_input_tokens`; 流式路径既要在增量状态累积, 也要在最终 usage 输出中回填, 不能只修非流式 DTO。
- Chat -> Responses 流式 message item id 是动态生成的, 但同一条消息在 added/done/completed output 中必须保持一致; 测试不应断言固定 `item_msg_0`。
- Reasoning-only Chat stream 会先输出 reasoning item, 必要时合成可见 message 文本; tool call stream 必须补齐 `function_call_arguments.done` 和 `output_item.done`, 否则 Codex 客户端不会执行工具。
- OpenAI-compatible API key 走 Chat Completions -> Responses 上游时, `prompt_cache_key` 要写入 Responses body, 并用 API key ID + cache key 派生稳定 `session_id`; 修正模型名时必须先完成上游模型映射再注入缓存 key。
- Codex OAuth path(`store=false`) 的 reasoning item 不能整项丢弃: 需要保留 `encrypted_content`/`content`/`summary` 等跨轮上下文字段, 但必须剥离 `rs_*` id 防止上游按旧 id 查找 404; 缺失 `summary` 时补 `[]`。请求带 `reasoning` 时要确保 include 包含 `reasoning.encrypted_content`。
- Codex 模型归一化保留 `gpt-5.5` 和 `gpt-5.5-pro` 原名, 包括 `gpt5.5*`、`openai/gpt5.5*` 和 `-high` 等 effort 后缀别名; 不应回退成旧 `gpt-5.4`/`gpt-5.3-codex`。
- `/v1/responses` 对 OpenAI-compatible API key 若账号不支持 Responses, 会 fallback 到 raw `/v1/chat/completions`; fallback 仍要输出 Responses SSE 给客户端并记录 Chat usage。
- `openai_gateway_cc_pipeline.go` 是 Chat Completions fallback 共享读写路径; 非流式 JSON 读取后必须同时补 `applyOpenAICompatibleChatUsageDetailsFromJSON` 和 `OpenAIUsage` cache 字段, 否则 Responses fallback 的 `usage.input_tokens_details.cached_tokens` 或计费中的 `cache_read_input_tokens` 会丢失。
- OpenAI WS 首包过大时可保持客户端 WebSocket, 改用 HTTP Responses 上游 bridge, 配置位于 `gateway.openai_ws.http_bridge_*`。
- OpenAI WS ingress、HTTP bridge 与 passthrough v2 每个 `response.create` turn 都重新解析客户端模型并执行当时的 channel mapping 与账号 mapping；usage 分别记录 turn requested/upstream/billing model, 回给客户端的事件恢复该 turn 的原始模型。后续 turn 切换到当前账号不支持且映射确实变化的模型时返回 policy-violation 并要求重连, 不能沿用首 turn 模型计费或静默绕过 whitelist。
- native OpenAI HTTP streaming Responses 可用 `gateway.openai_first_output_timeout_seconds` 启用首次语义输出 deadline, 默认 `0` 关闭, 非零合法范围 30-600 秒; `gateway.openai_high_effort_first_output_timeout_seconds` 默认 `0` 继承标准值, high/xhigh/max 的非零 override 合法范围 30-1800 秒。deadline 包含等待上游响应头, 不作用于 passthrough 或 WebSocket transport; 首次语义输出前单次 attempt 最多暂存 8 MiB, 超时或溢出时最多切换账号一次且不向客户端暴露不完整 SSE。原 attempt 可能已产生上游用量, 切号重放存在重复上游计费风险。
- `gateway.openai_ws.client_first_message_timeout_seconds` 默认 30 秒且必须为正数, 覆盖客户端首条 `response.create` 的完整读取与解压, 不是只限制首字节到达时间。
- OpenAI WS ingress 长连接与单 turn 并发槽分离: `max_ingress_connections_per_api_key` 使用 Redis 短租约限制每个 API Key 在多实例上的存活连接数, 默认 64, 0 关闭; lease TTL 60 秒、每 20 秒刷新, 丢失租约时连接 fail-close。`ingress_inter_turn_idle_timeout_seconds` 默认 300 秒, 只限制已完成 turn 之间的客户端空闲, 0 关闭。
- OpenAI WebSocket 传输层会把 Windows `WSAECONNRESET` / `WSAECONNABORTED` 等连接重置识别为可分类的网络错误; 不要把这类平台错误文本当作业务响应或未知失败。
- `/v1/responses/compact`、根级 `/responses/compact` 和 `/backend-api/codex/responses/compact` 会保留 compact 子路径; `gateway.openai_compact_model` 默认 `gpt-5.4`, 可在 compact endpoint 落后普通 Responses 时降级。账号级 compact model mapping 只影响 compact 请求, 不改普通 `/v1/responses`。body-signal 客户端请求 `stream=true` 时响应必须重新合成为 SSE; upstream SSE -> unary JSON 会保留 raw `output_item.done`, 等待期间可向下游发送不污染 failover/终态判定的 keepalive。
- Grok 没有原生 `/responses/compact`; Grok 分组把 compact 请求转换为一次普通非流式 Responses 摘要 turn, 要求返回 `reasoning.encrypted_content`, 再映射成 OpenAI `compaction` item。后续 turn 会把该 item 还原为 Grok reasoning 与带 `<conversation_summary>` 的上下文；compact 路径不派生 prompt cache identity。
- OpenAI 上游传输层错误(连接/代理等持久网络故障)由 `backend/internal/service/openai_upstream_transport_error.go` 的 `handleOpenAIUpstreamTransportError` 统一处理: 在 Responses fallback 与 raw/passthrough 路径触发 failover 换账号, 持久故障会临时摘除该账号(temp unscheduled), 不污染上游 SLA。native Responses、raw/compatible pipeline、passthrough 和 Grok producer 都要把上游 response headers 留在内部 failover error, 但账号耗尽时只允许向客户端恢复安全的 `Retry-After`: 数字必须为 1-604800 秒, HTTP 日期必须晚于当前且不超过 7 天; CR/LF、超长或超界值一律丢弃, 其他上游 header 不恢复。
- OpenAI Responses SSE 经代理发生非 context-cancel/deadline 的中途断流时, `openai_proxy_stream_circuit.go` 按 proxy ID 在进程内计数；默认 60 秒内 2 次触发隔离 10 分钟, 成功流会清除该代理观察。被隔离代理在账号调度时跳过；状态有界为 4096 项、重启清空, 只隔离代理而不持久修改账号。
- 网关转发函数如果已经向客户端写入完整上游错误响应, 必须调用/依赖 `MarkResponseCommitted` 与 `gatewayForwardErrorAlreadyCommunicated` 防止 handler 再追加通用 SSE 错误帧; 仅 ping 或流式中途错误仍需协议级失败帧。
- OpenAI/Grok/Messages 流里的 `response.failed` 要复用 error passthrough 与 failover 规则, 不能硬编码为 502; 已向客户端写入的 in-band SSE 错误同时要落 ops error context, 避免 200 HTTP 流内失败从错误看板消失。
- OpenAI/ChatGPT/Codex 账号配额查询与重置由 `backend/internal/service/openai_quota_service.go` 提供(上游 v0.1.137): 调 `chatgpt.com/backend-api/wham/usage` 读 rate-limit 窗口、`/wham/rate-limit-reset-credits/consume` 重置 credits; 管理端入口 `GET /api/v1/admin/openai/accounts/:id/quota` 与 `POST .../reset-quota`。上游对未用窗口返回显式 `null`, 消费方按 nil 指针视作"无数据"。
- OpenAI Codex PAT 账号由 `backend/internal/service/openai_codex_pat_service.go` 校验 `at-*` token, 使用官方 whoami endpoint 读取 email/account/user/plan/FedRAMP 字段。PAT 账号会清理 OAuth-only credential 字段, refresh 时不走 OAuth refresh token。
- OpenAI 图片生成 Responses 路径会识别 `response.incomplete`: `content_filter`/moderation 视为 400 非重试, 其他 incomplete 视为 502 可 failover。若上游 `response.completed` 但无图片输出, 会记录 ops 诊断摘要并返回 `UpstreamFailoverError{RetryableOnSameAccount:true}`, 先同账号快速重试, 再按 handler 上限换账号。`/v1/responses` 文本请求若未产生图片输出, 不应误触发图片计费。
- OpenAI 非流式图片 JSON 可用 `gateway.image_nonstream_keepalive_interval` 定期发送空白心跳, 默认 0 关闭; 首个心跳会提交 HTTP 200, 因此后续上游错误只能走已提交响应的协议处理。图片 output item 已有 `result` 但 status 仍为 `generating/in_progress` 时, streaming 与 SSE→JSON 终态统一修正为 `completed`。
- OpenAI context-window 类上游错误不能触发账号 runtime block, 避免模型上下文长度问题被误判为账号故障切号。
- OpenAI Fast/Flex policy 的规则支持 `user_ids` 范围。API Key 认证完成后 user ID 必须进入 gateway context, policy 才能只对选中用户执行 `pass/filter/block/force_priority`; 未选用户继续走 fallback/default 规则。设置持久化仍使用 `openai_fast_policy_settings`。

Grok/xAI 兼容:

- Grok OAuth 管理路由在 `backend/internal/server/routes/admin.go` 的 `registerGrokOAuthRoutes`: `/api/v1/admin/grok/oauth/auth-url`, `/exchange-code`, `/refresh-token`, `/accounts/:id/refresh`, `/accounts/:id/quota`, `/accounts/:id/reset-quota`, `/runtime-sanity`。
- `POST /api/v1/admin/grok/oauth/reconcile` 默认 dry-run, 通过 `after_id` 游标扫描 Grok OAuth 缺失 refresh/access token、缺失或非法 expiry 及临期凭据; 只有显式 `apply=true,dry_run=false` 才执行 refresh/block。后台 token refresh 与 reconcile 共用 Grok provider 的进程内并发和 QPS gate, 防止两种入口叠加上游压力; 两条路径各自按 `provider_failure_threshold` 在当前周期隔离该 provider。
- OAuth/token/账号创建由 `backend/internal/service/grok_oauth_service.go`, `grok_token_provider.go`, `grok_token_refresher.go`, `backend/internal/repository/grok_oauth_client.go` 提供; token cache key 独立为 `GrokTokenCacheKey`。
- OpenAI-compatible 转发入口在 `backend/internal/service/openai_gateway_grok.go`: `forwardGrokResponses` 支持 OAuth/API Key 账号, 按账号模型映射替换 `model`, 删除 xAI 不支持的 `prompt_cache_retention` / `safety_identifier` / `external_web_access`, 过滤不支持的 `tools` 和失配 `tool_choice`, 发送到 `account.GetGrokBaseURL()` 下的 Responses endpoint。
- Grok Responses 会把 Codex client-side `custom`、`namespace`、`tool_search` 工具可逆适配为 xAI 接受的 function tool, 并在 streaming、non-streaming 与 SSE-to-JSON 回程恢复原协议事件；撞名或无效 `tool_choice` 继续由共享 `apicompat.ResponsesClientToolMapping` 拒绝。输入中的 `additional_tools` 与顶层 `tools` 使用同一映射合同。
- `Account.GetGrokBaseURL` 对 OAuth 空值或官方 `api.x.ai[/v1]` 旧值回落 `xai.DefaultCLIBaseURL`; 显式自定义 URL 只有通过 `xai.ValidateTrustedBaseURL` 才使用, 否则回落 CLI proxy。API Key 空值使用 `xai.DefaultBaseURL`; URL 校验规则或 `XAI_ALLOW_UNSAFE_URL_OVERRIDES` 变化必须同步 SSRF/信任边界测试。
- Grok Free OAuth cacheable Chat 请求会按稳定会话前缀派生 prompt cache identity, 转为 Responses 上游后再桥回 Chat; raw Chat 路径必须删除 Responses-only `prompt_cache_key`。quota headers 由账号级 snapshot 更新, exhausted 状态可在后续健康额度响应中恢复, 不能永久停留在 rate limited。
- 已确认的 Grok Free OAuth 账号对纯客户端 function tools 默认启用 cache-capable mixed-tools 路由, 账号 `extra.grok_client_tool_cache_enabled` 可显式关闭；请求头 `X-Sub2API-Grok-Client-Tool-Cache` 可逐请求开关且只在本地消费。付费、API Key、未知账号和非法非布尔配置保持 fail-closed, 不能因工具名碰巧叫 `web_search` / `x_search` 就自动进入缓存路由。
- Grok quota 由 `backend/internal/pkg/xai/quota.go` 解析 xAI rate-limit 和 entitlement headers, 快照写入账号 `extra`。管理端主动探测在 `backend/internal/service/grok_quota_service.go`, 使用最小 Responses 请求读取 headers; reset 当前返回不支持。账号连通性测试由 `AccountTestService.testGrokAccountConnection` 直连 xAI Responses API, 并同步 quota header 快照。
- Grok 上游 401/403/429/5xx 会按 `handleGrokAccountUpstreamError` 临时摘除账号: 401 约 10 分钟, 403 约 30 分钟, 429 优先按 `Retry-After`, 5xx 约 2 分钟。Grok group 现在可走 `/v1/messages`, `/v1/messages/count_tokens`（含根级别名）本地估算、`/v1/chat/completions`, 根级 `/chat/completions`、images/videos media 路由与 Responses WebSocket/HTTP/compact 兼容入口；本地 count-tokens 不触发上游账号错误状态。
- 旧 Grok group 会由 migration `158_enable_grok_media_generation_groups.sql` 回填 `allow_image_generation=true`, 因为 Grok media 路由复用图片生成能力 gate。
- `PlatformGrok` 已加入 `domain/constants.go`, `service/domain_constants.go`, scheduler snapshot 平台列表、token cache invalidator 和 user platform quota 允许列表。新增平台相关能力时要同步这些集中列表。
- 模型不可用诊断由 `gateway_model_availability.go` 和 `openai_gateway_model_availability.go` 提供; no-account 错误路径会区分"池中无支持该模型账号"并返回 404 `model_not_found`, 避免误报 503。

内容审核运行态:

- `risk_control_enabled`、`content_moderation_config` 和 `prompt_risk_config` 合并读取为同一个 stale-while-refresh runtime snapshot。snapshot 过期时热路径继续使用旧值并异步刷新, 避免每个网关请求查询 settings; 管理端通用 settings 成功保存 `risk_control_enabled` 后通过 SettingService callback 立即原子替换已有 snapshot 的总开关, 开启/关闭都不等待 TTL。
- `content_moderation_config.blocked_keywords` 在 snapshot 构建时预编译 matcher; 保存内容审核配置或 Prompt Risk 配置后会立即原子替换对应 snapshot 部分, 无需等待 TTL。
- 后台刷新失败时保留最后一个有效 snapshot, 并按 runtime TTL 退避后再尝试, 不能因 settings 短暂故障清空审核策略或阻塞请求热路径。
- Prompt Audit `ConfigManager` 独立保存最近一次可解码的 blocking intent。启动或 reload 失败只有在全局 risk control、audit enabled 和 blocking enabled 同时表明阻断意图时才强制 `ModeBlocking` fail-closed；未识别到阻断意图时不得仅因配置不可信把默认关闭/async 模式升级成全站 503, 成功保存 disabled 配置会清除 degraded 状态。
- Prompt Audit 管理端 `GET config` 只有在 `ConfigManager` 已成功加载 snapshot 时才返回配置；持久 setting 缺失会成功加载 version 1 的默认关闭配置, 但已有持久配置激活失败且没有可信 snapshot 时返回 `prompt_audit_config_unavailable`，不能伪装成默认关闭。后续 reload 失败会继续返回最后一次可信 snapshot。

OpenAI 账号调度:

- `gateway.openai_ws.scheduler_score_weights.reset` 是高级调度得分因子, 默认 `0` 关闭; 大于 0 时, 拥有未来 `SessionWindowEnd` 且剩余重置时间更短的账号得分更高。
- `gateway.openai_ws.scheduler_score_weights.quota_headroom` 默认 `0` 关闭; 大于 0 时, 基于账号 `extra` 中 `codex_primary_used_percent` / `codex_7d_used_percent` 和 `codex_usage_updated_at` 计算剩余额度健康度, 快照缺失、过期或窗口已重置时使用中性分。
- `gateway.scheduling.prefer_soonest_reset` 默认 `false`; 开启后负载感知选择会先过滤出会话窗口最早重置的账号, 用于 use-it-or-lose-it 策略。没有活跃窗口时返回原候选集合, 不改变旧行为。
- OpenAI Spark 影子账号使用 `parent_account_id + quota_dimension=spark`: 影子不持 OAuth 凭据, 运行时通过母账号 token 发起上游请求, 但独立读取 `codex_bengalfox`/spark 配额窗口。母账号 global 429 或过载不能连坐 spark 影子; 母账号凭据过期、临时摘除或非 OAuth 才会阻断影子调度。spark 模型当前只允许 `gpt-5.3-codex-spark` base, 默认 model_mapping 为恒等映射。

网关链路常见中间件:

- `RequestBodyLimit`
- `ClientRequestID`
- `OpsErrorLoggerMiddleware`
- `InboundEndpointMiddleware`
- `APIKeyAuth`
- `RequireGroupAssignment`
- `RequestArchive`
- `RequestIntercept`

`RequestArchive`(`backend/internal/server/middleware/request_archive.go`)把网关请求体和响应元信息写入本地 JSONL, 仅用于短期排障:

- 默认关闭: `gateway.request_archive.enabled=false`, `capture_response=false`(与 `backend/internal/config/config.go` 默认一致)。
- 开启后位于请求热路径, 会 `io.ReadAll` 完整请求体; `capture_response=true` 时只记录响应大小、hash、流式标记和 token `usage`, 不保存响应正文, 以降低归档体积。
- 响应 usage 支持从非流式 JSON 的 `usage` / `response.usage` / `usageMetadata`(Gemini)/ `message.usage`(Anthropic `message_start` 兜底)读取, 也会从 SSE `data:` JSON 事件中提取最后一次非空 usage(含无尾换行的终止事件兜底; 单行超 256KB 被裁剪后碎片行降级 fragment 提取); 非流式只保留 256KB 尾部窗口, 提取在请求结束后执行一次。
- 写入为异步有界队列(`gateway.request_archive.queue_size`, 默认 1024): 热路径只入队, 后台单 goroutine 持有当日文件句柄并按日期轮转, 队列满时丢弃记录不阻塞请求。
- 管理后台 `/admin/settings` 的 Gateway 标签页可热切换 `enabled`、`capture_response` 和归档目录 `dir`(后端接口 `GET/PUT /api/v1/admin/settings/request-archive`)。自定义 dir 必须为绝对路径, 保存时校验磁盘存在/目录可创建/可写(`internal/service/request_archive_dir.go`), 响应附带磁盘容量(`internal/pkg/diskspace`); 写入端经 `writer.SetDir` 在下一条记录生效, token_analysis 索引器同源跟随; 历史文件不自动迁移。请求体截断上限 `max_request_body_bytes` 同样可在该设置页热改(MB 输入框; 代码默认 16MB, 持久化 0/等于默认=未自定义回退 config, 合法范围 64KB~512MB, 中间件每请求读运行态缓存即时生效); 响应截断上限 `max_response_body_bytes` 和 `queue_size` 仍由实际加载的 `config.yaml` 控制, 修改后需重启。
- Token Analysis 索引(`internal/service/token_analysis_indexer.go`)读取归档 request 行时会做项目归因(`internal/service/project_attribution.go`): 从 Claude Code system prompt / Codex `<cwd>` / Copilot 附件路径提取工作目录与项目名, 写入 `token_analysis_request_summaries.client_workdir/client_project/client_branch/attribution_source`; 已知仓库根持久化在 `token_analysis_project_roots`, 供 Copilot 路径前缀匹配跨天累积。聚合接口 `GET /api/v1/admin/token-analysis/projects` 按"项目 × 成员"汇总 token 消耗, 未归因请求显式归入 `unattributed`。设计与实测命中率见 `docs/features/token-analysis-project-attribution-design-cn.md`。
- 索引时还会留存用户净输入全文(最后一条用户消息, 脱敏保留排版, `token_analysis.input_store_max_chars` 截断, 默认 8000, 0=关闭)到 `token_analysis_user_inputs`(键 `archive_id`, 含 `content_sha256` 去重哈希与 `quality_*` 占位字段, 重建索引不覆盖质量结果); 全文经 `GET /api/v1/admin/token-analysis/requests/input?archive_id=` 懒加载, 列表 LEFT JOIN 只带 `has_input/quality_score`。原始 JSONL 按保留期删除后输入分析仍可回溯。注意与在线采集的 `user_prompt_events`(prompt metrics)并存, 见 `docs/features/token-analysis-user-input-store-design-cn.md`。
- 索引触发: 服务启动即跑自动索引循环(`token_analysis_auto_index.go`, 间隔 `token_analysis.auto_index_interval_seconds` 默认 300 秒, 0=仅手动), 每轮对 [昨天, 今天] 增量索引(offset 续读, 跨天补拽昨日文件尾部); 与页面「索引当前范围」(`POST /api/v1/admin/token-analysis/index`)共用 running 互斥, 撞车时自动轮静默跳过, 手动触发保留用于补历史日期。手动触发为异步语义: handler 同步校验日期与互斥(非法范围 400 `TOKEN_ANALYSIS_INDEX_INVALID_RANGE`, 撞车 409 `TOKEN_ANALYSIS_INDEX_RUNNING`)后返回 202, 索引在服务生命周期 goroutine 内执行(`lifecycleCtx` + 共享 WaitGroup, 优雅停机时取消并等待); 前端每 3 秒轮询 `GET .../index/status` 至 `running=false` 后刷新页面。状态接口的 `running` 仅来自服务内存标志, 不再按 `started_at` 非空且 `finished_at` 为空推断(历史中断轮次会让该启发式永远为真, 轮询永不终止)。
- JSONL 生命周期: `GET /api/v1/admin/token-analysis/archive-files`(`token_analysis_archive_files.go`)列出当前归档目录文件并按索引水位打标签 — 今日写入中 / 待索引 / 可删除(水位追平且无失败行)/ 有失败行(谨慎)/ 已压缩(.gz 不参与索引); 页面 Token 分析「归档文件」卡片展示, 删除本身由运维在服务器手动执行(索引器对已删文件静默跳过)。失败行语义: 仅统计 `body_truncated=false` 却解析失败的真异常行; 归档端按 `max_request_body_bytes` 截断导致 body JSON 不完整的行降级为仅元数据入库(`summary_json` 标记 `degraded=body_truncated`, model/body_size/截断标记保留, 计入 processed 不计 failed)。multipart 归档体(`/v1/images/edits` 图改图等, body 以 `--boundary` 开头, 按 JSON 解析必报 `invalid character '-' in numeric literal`)由 `SummarizeTokenAnalysisMultipartRequest`(`token_analysis_summary.go`)按 form 字段解析出 prompt/model/n/size 与输入图数, 产出 image 形态摘要(`summary_json` 标记 `multipart=true`/`source_image_count`); boundary 优先取归档行 `headers.content_type`, 缺失时从 body 首行兜底; 因前端把文本域排在图片分片之前, 截断的 multipart 行同样能解出 prompt(优先于 body_truncated 降级); 是 multipart 但完全解不出时降级 `degraded=multipart_body`, 一律不计失败行。`token_analysis_index_state.last_error` 只被非空错误覆盖(COALESCE+NULLIF 粘性保留), 成功轮次不再清掉历史错误。失败行 offset 仍会前移不重试, 需重灌时删除对应 `token_analysis_index_state` 行后重新索引(`UpsertRequestSummary` 按 archive_id 幂等)。
- NUL 清洗: 归档 JSON 字符串里合法的 `\u0000` 转义解码后是真实 0x00 字节, Postgres text/jsonb 列拒收(`pq: invalid byte sequence for encoding "UTF8": 0x00`)。所有 body 衍生文本入库前必须过 `sanitizeTokenAnalysisText`(`token_analysis_summary.go`, 剥 NUL + 兜底替换非法 UTF-8): 已接入净输入留存(`RedactTokenAnalysisInputText`, 哈希对剥离后文本计算)、预览(`SanitizeTokenAnalysisPreview`)、`tokenAnalysisString`(model 等)、shape、endpoint、归档行 method/endpoint/path/model 字段、`last_error`(错误消息可内嵌归档原文); 项目归因侧含 NUL 的 cwd 直接拒绝(`validAttributionCwd`), branch 剥离后保留。典型来源是二进制内容被贴进对话; 修复前这类行在 user_inputs upsert 处失败且水位前移不重试(summary 已入库, 只丢净输入留存并污染 failed_rows), 重灌同样靠删水位重索引。(`tokenAnalysisIsCountTokensPath` 按原始 path 判断 -- endpoint 字段已被归一化折叠成 /v1/messages 看不出来), 否则它会与紧邻真实请求匹配到同一条 usage_logs, 概览/聚合把该条计费 token 累加两次; 入库 endpoint 还原为 `/v1/messages/count_tokens` 以便库内区分。历史误匹配行靠重置水位全量重索引原地修正(upsert 按 archive_id)。
- 概览口径: `GET /api/v1/admin/token-analysis/summary` 统计的是 `token_analysis_request_summaries`(已归档已索引的请求), 不是全量请求; service 层强制 include_unmatched(该参数只影响请求明细列表), 未匹配数/未匹配率卡片才有意义。总 token/费用始终只含匹配上 usage_logs 的行(未匹配行贡献 0)。响应附 `billed_requests`(同期 usage_logs 计费请求数, 只应用时间/用户/密钥/账号/分组/模型过滤)与 `archive_coverage`(matched/billed), 页面以"已归档请求数/同期计费请求数/归档覆盖率"卡片呈现差距。

端点归一化集中在 `backend/internal/handler/endpoint.go`。当前会把根级 `/responses`、`/responses/*`、`/backend-api/codex/responses*` 归一到 `/v1/responses`, 并识别 `/v1/videos/generations` / `/v1/videos/*`。新增网关端点时要同步常量, `NormalizeInboundEndpoint`, `DeriveUpstreamEndpoint`, 路由注册和相关 OpenAI/Claude/Gemini/Grok 分流测试。

## 上游拆分约定

上游 v0.1.146 将多处大文件做纯移动拆分: `setting_service.go`、`setting_handler.go`、`admin_service.go`、`gateway_service.go`、`openai_gateway_service.go`、`openai_ws_forwarder.go`、`usage_log_repo.go` 等保留薄入口, 具体能力分散到同目录按领域命名的文件。后续修改时先按函数名 `rg`, 不要把拆分后的文件重新合并回大文件; Wire/provider 变更仍以 `wire.go`/`wire_gen.go` 为准。
## 分层约定

- `internal/handler`: HTTP 请求绑定, 参数校验, 调用 service, 返回响应。
- `internal/service`: 业务逻辑和跨 repository 编排。
- `internal/repository`: Ent/SQL/Redis/外部 HTTP 访问实现。
- `internal/server/middleware`: 服务端通用 HTTP 中间件。
- `internal/payment`: 支付抽象, provider 注册和负载均衡。
- `internal/pkg`: 可复用基础设施包, 如 logger, errors, openai, gemini, proxyutil。

新增能力时优先按现有层次放置, 不要让 handler 直接承载复杂业务逻辑。

## 配置加载

`backend/internal/config/config.go` 使用 Viper:

- 显式 `CONFIG_FILE` 非空时通过 `SetConfigFile` 读取该文件并停止目录搜索；路径缺失会让加载返回错误。未设置时按 `DATA_DIR`、`/app/data`、当前目录、`./config`、`/etc/sub2api` 搜索 `config.yaml`。`GetServerAddress()` 使用同一来源选择, 环境变量仍覆盖配置文件。
- 支持环境变量, `.` 会映射为 `_`。
- `LoadForBootstrap()` 用于启动前完整配置。
- `GetServerAddress()` 是 setup 前的轻量读取。

配置结构很大, 修改配置必须同步:

- `Config` 结构体。
- 默认值。
- 校验逻辑。
- `deploy/config.example.yaml`。
- 必要时更新前端 settings 类型和 UI。

## 数据访问

`backend/internal/repository/ent.go`:

- `InitEnt` 初始化 PostgreSQL Ent client。
- 启动时自动执行 embedded migrations。

`backend/internal/repository/wire.go`:

- 集中注册 repository, cache, encryptor, external clients。
- `ProvideSQLDB` 从 Ent driver 暴露 `*sql.DB`, 用于复杂 SQL。
- `ProvideRedis` 调用 `InitRedis`。
- 通用分页使用 `internal/pkg/pagination.PaginationParams`; `Offset()` 必须乘以 `Limit()` 的规范化结果, 使小于 1 的 page size 回落 20、超过 1000 的值封顶 1000, 避免 offset 与实际 limit 脱节。

## 生成代码

修改以下内容后通常要生成:

- `backend/ent/schema/*.go`: 运行 `cd backend && go generate ./ent`。
- Wire provider: 运行 `cd backend && go generate ./cmd/server` 或 `make generate`。

生成代码必须随源码提交, 否则 CI/编译可能失败。

## 相关页面

- [[README]]
- [[backend]]
- [[frontend]]
- [[ops]]
- [[data-and-domain]]
- [[security-and-reliability]]
- [[ai-workflow]]
