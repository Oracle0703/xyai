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
- `payment.ProviderSet`
- `middleware.ProviderSet`
- `handler.ProviderSet`
- `server.ProviderSet`

很多后台服务在 Provider 中自动 `Start()`, 例如 token refresh, dashboard aggregation, usage cleanup, ops collector, scheduled report, account/subscription expiry, proxy expiry(代理有效期清理与回退), token analysis 自动索引, channel monitor runner 和 user platform quota flusher。新增后台服务时要同时考虑 Wire 注入, 启动时机, multi-instance leader lock 和 `provideCleanup` 停止逻辑。

代理有效期与失败回退(`ProvideProxyExpiryService`, 每分钟扫描):

- `backend/internal/service/proxy_expiry_service.go` 定时 `SweepExpiredProxies`; `proxy_fallback.go` 的 `ResolveProxyFallbackTarget` 按 `fallback_mode`(`none` / `proxy` / `direct`)沿 `backup_proxy_id` 链解析过期代理应改投的目标, `RevertProxyFallback` 支持手动回切。
- 账号回切来源记录在 `accounts.proxy_fallback_origin_id`; 前端入口在 `ProxiesView.vue`(有效期/回退模式)与 `AccountsView.vue`(回切)。
- `provideCleanup` 的 `ProxyExpiryService` 步骤负责停止该后台任务。

## HTTP Server 与路由

入口:

- `backend/internal/server/http.go`: 提供 Gin engine 和 `http.Server`。
- `backend/internal/server/router.go`: 注册全局中间件和业务路由。
- `backend/internal/server/routes/*.go`: 分组注册路由。

全局中间件包含 recovery, request logger, CORS, security headers, embedded frontend。`server.trusted_proxies` 为空时会禁用可信代理解析。

路由主分组:

- `/api/v1/auth`: 注册, 登录, OAuth, refresh, logout, pending auth。
- `/api/v1/user`, `/api/v1/keys`, `/api/v1/usage`, `/api/v1/redeem`, `/api/v1/subscriptions`: 用户侧接口。
- `/api/v1/admin`: 管理端接口, 由 admin auth 保护。
- `/v1`, `/v1beta`, `/responses`, `/chat/completions`, `/images/*`: AI 网关兼容接口。
- `/antigravity/v1`, `/antigravity/v1beta`: Antigravity 专用兼容接口。

用户侧用量接口:

- `GET /api/v1/usage`, `/stats`, `/dashboard/trend`, `/dashboard/models` 共用 `parseUserUsageFilters`, 支持 user scope 下的 `api_key_id` 所有权校验、`group_id`、请求模型、`request_type`/legacy `stream`、`billing_type`、`billing_mode` 和用户时区日期范围。
- `GET /api/v1/usage/dashboard/snapshot-v2` 为用户用量页图表聚合接口, 按 include 参数返回 trend/model/group 分布, 只暴露当前用户数据; 用户侧 stats 会清空管理端专属的 account/upstream endpoint 明细。

管理端组织用量报表:

- 完整设计见 `docs/features/organization-usage-report-design-cn.md`。
- `GET /api/v1/admin/usage/organization-report/summary` 返回组织概览、三组组织汇总、日/周/月 champions 和分页用户摘要; `GET .../periods` 返回有用量的 user-period 明细。二者沿用 `/api/v1/admin` 的管理员认证与合规 guard。
- 三层边界独立为 `OrganizationUsageRepository`、`OrganizationUsageService` 和 `admin.OrganizationUsageHandler`; SQL 实现在 `internal/repository/organization_usage_repo.go`, 不扩张 `UsageLogRepository` 或 `DashboardHandler`。
- 日期合同是固定 `Asia/Shanghai` 的 `YYYY-MM-DD` 闭区间, service 转成 UTC 半开区间后查询; 最多 366 个自然日。SQL 先用原始 `usage_logs.created_at >= start AND created_at < end` 收敛, 再按北京时间分日/周/月桶; 周为周一到周日, 跨选区周期会裁剪起止日期并标记 `partial=true`。
- 可选 `as_of` 必须是严格 RFC3339/RFC3339Nano; service 将其规范化为 UTC 并裁剪到不晚于服务端当前时间, 响应回显 canonical `as_of`。usage 查询上界再取 canonical `as_of` 与日期 end 的较早值, 早于范围起点时钳成空用量区间。该值不是密码学签名或服务端 snapshot id。
- summary 从 active 且未删除用户出发 LEFT JOIN 范围用量, 因此保留零用量用户; periods 只返回存在用量的 user-period。组织、粒度、排序字段和排序方向均为严格 allowlist, 非法值返回 400。
- PostgreSQL integration 与 30/90/366 天性能基线见 `backend/internal/repository/organization_usage_repo_integration_test.go`、`organization_usage_explain_integration_test.go` 和 `docs/features/organization-usage-report-performance-cn.md`。600 用户/219,600 logs 的 90 天 Summary items 曾因三个 peak CTE 对 `ranked_periods` 各循环扫描 600 次达到约 11 秒; 显式物化 peak 的诊断候选约 418 ms。现有时间索引不是该慢计划根因, 后续先修 peak 连接形状, 再减少导出分页重复查询。

## 网关路径

`backend/internal/server/routes/gateway.go` 是网关路由入口。

关键行为:

- `/v1/messages` 根据 API Key 所属 group platform 分流到 OpenAI 或 Claude 兼容处理。
- `/v1/messages/count_tokens` 对 OpenAI group 走 Anthropic-compatible 到 OpenAI `/v1/responses/input_tokens` 的桥接; 不占并发槽、不写 usage。Grok 等其他 OpenAI-compatible platform 仍返回本地不支持。
- `/v1/responses` 和根级 `/responses` 支持 OpenAI Responses API。
- `/v1/chat/completions` 和根级 `/chat/completions` 支持 OpenAI Chat Completions。
- `/v1/embeddings` 和根级 `/embeddings` 仅 OpenAI platform 支持。
- `/v1/images/generations` 和 `/v1/images/edits` 对 OpenAI platform 走 OpenAI images handler, 对 Grok platform 走 Grok media handler; 根级别名 `/images/generations`、`/images/edits` 也保留 `RequestArchive` / `RequestIntercept` 中间件链。
- `/v1/images/batches` 是 batch image 用户侧任务接口族: submit/list/models/get/items/item content/download/cancel/delete/delete outputs。入口在 `backend/internal/handler/batch_image_handler.go`, service/repository 分别在 `backend/internal/service/batch_image*.go` 与 `backend/internal/repository/batch_image*.go`; 受 API Key 用户、分组 `allow_batch_image_generation` 与批量生图折扣/hold multiplier 约束。
- Grok/xAI 使用 OpenAI-compatible gateway 入口, platform 为 `grok`; 当前支持 OAuth 订阅账号的 Responses/Chat 兼容文本与推理流量, API Key 账号不在 Grok 首版范围。
- Grok media 路由支持 `/v1/images/generations`, `/v1/images/edits`, `/v1/videos/generations`, `/v1/videos/:request_id` 及根级 images/videos 别名; 非 Grok platform 访问 videos 返回本地 404 feature gate。`grok-imagine` 图片别名会归一到 `grok-imagine-image-quality`, Grok 4.5 正式模型别名由 `internal/pkg/xai/models.go` 维护, video model 透传到 xAI `/v1/videos/*` 并按分辨率和生成秒数计费。
- `/v1beta/models/*` 提供 Gemini SDK/CLI 兼容。
- `/backend-api/codex/responses` 支持 Codex 直连别名; `GET /backend-api/codex/models` 由 `openai_codex_models_handler.go` / `openai_codex_models_service.go` 代理 Codex 客户端 model manifest, 入口仍受 API Key 与 group 校验保护。

OpenAI 上游请求会按官方 endpoint 做字段过滤:

- `/v1/responses` / Responses 透传路径删除 top-level `thinking`, 保留官方 `reasoning`。
- `/v1/chat/completions` raw 直转路径删除 top-level `thinking`, 保留官方 `reasoning_effort`。
- `/v1/chat/completions` 入口可选注入默认 `reasoning_effort`: 配置 `gateway.openai_default_reasoning_effort`(默认空=关闭)非空时, `applyDefaultOpenAIReasoningEffort` 在 `ForwardAsChatCompletions` 分流前对入站 body 注入一次, 同时覆盖 raw 直转与 CC→Responses 两条上游形状; 注入在 `json.Unmarshal` 前完成, 计费/用量日志自然读到。仅对**映射后** billingModel 命中 `SupportsOpenAIReasoningEffort`(gpt-5.x / o1·o3·o4)的推理模型注入; 客户端经 `reasoning_effort` / `reasoning.effort` / 模型名后缀(`gpt-5-high`)已指定时不覆盖; gate `messages` 存在以排除 Cursor 的 Responses-shape(`input`)透传。非推理模型(gpt-4o 等)不注入, 否则官方上游 400 unsupported parameter。
- `/v1/chat/completions` raw 直转到 GLM(`glm-*`)上游前会归一化 reasoning effort: `reasoning.effort` 或 `reasoning_effort` 中的 `low`/`medium`/`high` 映射为 `high`, `xhigh`/`extrahigh`/`max`/`ultracode` 映射为 `max`; 其他上游不受影响。
- GPT-5.6 支持 `max` reasoning effort; effort 提取按 `upstreamModel -> billingModel -> originalModel` 候选顺序判断, 避免账号映射或模型后缀归一化后丢失 `max`/后缀语义。修改模型映射时要同步 raw Chat、Responses fallback、WS ingress 和相关候选测试。
- Anthropic/Gemini 等非 OpenAI 协议的 thinking 映射不复用该过滤规则, 需按各自协议能力单独处理。
- Anthropic OAuth/SetupToken 转发默认启用客户端 dateline 归一化, 只改写 `system` 或 `<system-reminder>` 内的 `Today's date is YYYY-MM-DD.` 指纹变体, 还原 ASCII 撇号和 `-` 分隔符; API Key 账号和普通用户正文不扫描。
- OpenAI Responses SSE 终止事件的 usage 可能在顶层 `usage` 或 `response.usage`; Chat Completions 和 Messages 的 buffered/streaming 转换及计费解析必须按实际 JSON 路径保留 `input_tokens_details.cached_tokens`、`cache_read_input_tokens`、`prompt_cache_hit_tokens`(DeepSeek Context Cache 命中)等缓存 token 字段; `prompt_cache_miss_tokens` 仍按普通 prompt/input token 口径计费, 不映射为 cache creation。
- Responses/Chat 双向桥接需保留 `parallel_tool_calls`; Responses `text.format` 与 Chat `response_format` 支持 `json_object` / `json_schema` 映射。OpenAI-compatible Responses -> Chat fallback 仍要经过本地 `ResponsesToChatCompletionsRequestWithOptions`, 以保留第三方上游的 temperature/max token 过滤策略。

OpenAI/Codex 兼容桥:

- `backend/internal/pkg/apicompat/chatcompletions_responses_bridge.go` 负责 Chat Completions 与 Responses 双向桥接; streaming bridge 会按 Responses 生命周期发出 `response.created`, `response.output_item.added`, `response.content_part.added`, `response.output_text.delta/done`, `response.output_item.done`, `response.completed`。
- Chat -> Responses 流式 message item id 是动态生成的, 但同一条消息在 added/done/completed output 中必须保持一致; 测试不应断言固定 `item_msg_0`。
- Reasoning-only Chat stream 会先输出 reasoning item, 必要时合成可见 message 文本; tool call stream 必须补齐 `function_call_arguments.done` 和 `output_item.done`, 否则 Codex 客户端不会执行工具。
- OpenAI-compatible API key 走 Chat Completions -> Responses 上游时, `prompt_cache_key` 要写入 Responses body, 并用 API key ID + cache key 派生稳定 `session_id`; 修正模型名时必须先完成上游模型映射再注入缓存 key。
- Codex OAuth path(`store=false`) 的 reasoning item 不能整项丢弃: 需要保留 `encrypted_content`/`content`/`summary` 等跨轮上下文字段, 但必须剥离 `rs_*` id 防止上游按旧 id 查找 404; 缺失 `summary` 时补 `[]`。请求带 `reasoning` 时要确保 include 包含 `reasoning.encrypted_content`。
- Codex 模型归一化保留 `gpt-5.5` 和 `gpt-5.5-pro` 原名, 包括 `gpt5.5*`、`openai/gpt5.5*` 和 `-high` 等 effort 后缀别名; 不应回退成旧 `gpt-5.4`/`gpt-5.3-codex`。
- `/v1/responses` 对 OpenAI-compatible API key 若账号不支持 Responses, 会 fallback 到 raw `/v1/chat/completions`; fallback 仍要输出 Responses SSE 给客户端并记录 Chat usage。
- `openai_gateway_cc_pipeline.go` 是 Chat Completions fallback 共享读写路径; 非流式 JSON 读取后必须同时补 `applyOpenAICompatibleChatUsageDetailsFromJSON` 和 `OpenAIUsage` cache 字段, 否则 Responses fallback 的 `usage.input_tokens_details.cached_tokens` 或计费中的 `cache_read_input_tokens` 会丢失。
- OpenAI WS 首包过大时可保持客户端 WebSocket, 改用 HTTP Responses 上游 bridge, 配置位于 `gateway.openai_ws.http_bridge_*`。
- `/v1/responses/compact`、根级 `/responses/compact` 和 `/backend-api/codex/responses/compact` 会保留 compact 子路径; `gateway.openai_compact_model` 默认 `gpt-5.4`, 可在 compact endpoint 落后普通 Responses 时降级。账号级 compact model mapping 只影响 compact 请求, 不改普通 `/v1/responses`。body-signal 客户端请求 `stream=true` 时响应必须重新合成为 SSE; upstream SSE -> unary JSON 会保留 raw `output_item.done`, 等待期间可向下游发送不污染 failover/终态判定的 keepalive。
- OpenAI 上游传输层错误(连接/代理等持久网络故障)由 `backend/internal/service/openai_upstream_transport_error.go` 的 `handleOpenAIUpstreamTransportError` 统一处理: 在 Responses fallback 与 raw/passthrough 路径触发 failover 换账号, 持久故障会临时摘除该账号(temp unscheduled), 不污染上游 SLA。
- 网关转发函数如果已经向客户端写入完整上游错误响应, 必须调用/依赖 `MarkResponseCommitted` 与 `gatewayForwardErrorAlreadyCommunicated` 防止 handler 再追加通用 SSE 错误帧; 仅 ping 或流式中途错误仍需协议级失败帧。
- OpenAI/Grok/Messages 流里的 `response.failed` 要复用 error passthrough 与 failover 规则, 不能硬编码为 502; 已向客户端写入的 in-band SSE 错误同时要落 ops error context, 避免 200 HTTP 流内失败从错误看板消失。
- OpenAI/ChatGPT/Codex 账号配额查询与重置由 `backend/internal/service/openai_quota_service.go` 提供(上游 v0.1.137): 调 `chatgpt.com/backend-api/wham/usage` 读 rate-limit 窗口、`/wham/rate-limit-reset-credits/consume` 重置 credits; 管理端入口 `GET /api/v1/admin/openai/accounts/:id/quota` 与 `POST .../reset-quota`。上游对未用窗口返回显式 `null`, 消费方按 nil 指针视作"无数据"。
- OpenAI Codex PAT 账号由 `backend/internal/service/openai_codex_pat_service.go` 校验 `at-*` token, 使用官方 whoami endpoint 读取 email/account/user/plan/FedRAMP 字段。PAT 账号会清理 OAuth-only credential 字段, refresh 时不走 OAuth refresh token。
- OpenAI 图片生成 Responses 路径会识别 `response.incomplete`: `content_filter`/moderation 视为 400 非重试, 其他 incomplete 视为 502 可 failover。若上游 `response.completed` 但无图片输出, 会记录 ops 诊断摘要并返回 `UpstreamFailoverError{RetryableOnSameAccount:true}`, 先同账号快速重试, 再按 handler 上限换账号。`/v1/responses` 文本请求若未产生图片输出, 不应误触发图片计费。
- OpenAI context-window 类上游错误不能触发账号 runtime block, 避免模型上下文长度问题被误判为账号故障切号。

Grok/xAI 兼容:

- Grok OAuth 管理路由在 `backend/internal/server/routes/admin.go` 的 `registerGrokOAuthRoutes`: `/api/v1/admin/grok/oauth/auth-url`, `/exchange-code`, `/refresh-token`, `/accounts/:id/refresh`, `/accounts/:id/quota`, `/accounts/:id/reset-quota`, `/runtime-sanity`。
- OAuth/token/账号创建由 `backend/internal/service/grok_oauth_service.go`, `grok_token_provider.go`, `grok_token_refresher.go`, `backend/internal/repository/grok_oauth_client.go` 提供; token cache key 独立为 `GrokTokenCacheKey`。
- OpenAI-compatible 转发入口在 `backend/internal/service/openai_gateway_grok.go`: `forwardGrokResponses` 强制 OAuth 账号, 按账号模型映射替换 `model`, 删除 xAI 不支持的 `prompt_cache_retention` / `safety_identifier` / `external_web_access`, 过滤不支持的 `tools` 和失配 `tool_choice`, 发送到 `account.GetGrokBaseURL()` 下的 Responses endpoint。
- Grok quota 由 `backend/internal/pkg/xai/quota.go` 解析 xAI rate-limit 和 entitlement headers, 快照写入账号 `extra`。管理端主动探测在 `backend/internal/service/grok_quota_service.go`, 使用最小 Responses 请求读取 headers; reset 当前返回不支持。账号连通性测试由 `AccountTestService.testGrokAccountConnection` 直连 xAI Responses API, 并同步 quota header 快照。
- Grok 上游 401/403/429/5xx 会按 `handleGrokAccountUpstreamError` 临时摘除账号: 401 约 10 分钟, 403 约 30 分钟, 429 优先按 `Retry-After`, 5xx 约 2 分钟。Grok group 现在可走 `/v1/messages`, `/v1/chat/completions`, 根级 `/chat/completions`、images/videos media 路由与 Responses WebSocket/HTTP 兼容入口; `/v1/messages/count_tokens` 仍返回不支持。
- 旧 Grok group 会由 migration `158_enable_grok_media_generation_groups.sql` 回填 `allow_image_generation=true`, 因为 Grok media 路由复用图片生成能力 gate。
- `PlatformGrok` 已加入 `domain/constants.go`, `service/domain_constants.go`, scheduler snapshot 平台列表、token cache invalidator 和 user platform quota 允许列表。新增平台相关能力时要同步这些集中列表。
- 模型不可用诊断由 `gateway_model_availability.go` 和 `openai_gateway_model_availability.go` 提供; no-account 错误路径会区分"池中无支持该模型账号"并返回 404 `model_not_found`, 避免误报 503。

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

- 支持 `config.yaml`, `./config`, `/etc/sub2api` 等路径。
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

## 生成代码

修改以下内容后通常要生成:

- `backend/ent/schema/*.go`: 运行 `cd backend && go generate ./ent`。
- Wire provider: 运行 `cd backend && go generate ./cmd/server` 或 `make generate`。

生成代码必须随源码提交, 否则 CI/编译可能失败。
