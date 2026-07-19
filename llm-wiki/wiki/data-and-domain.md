# 数据与领域基线

## 核心领域

Sub2API 的核心对象:

- User: 用户, 角色, 余额, OAuth identity, 属性, TOTP。
- API Key: 用户侧调用凭证, 关联 group, rate limit, quota, last used。
- Group: 调度和计费分组, 控制 platform, model mapping, rate multiplier, 高峰时段倍率, Grok 图片/视频独立定价, Codex alpha search 按次价格, RPM, 支持模型范围和自定义 `/v1/models` 列表。
- Account: 上游账号, 支持 OAuth/API Key/cookie/setup token 等类型, 可绑定 proxy, group, model whitelist 和 quota; OpenAI 账号支持 endpoint capability, pool retry status codes, quota threshold auto-pause, Codex CLI only、允许 Claude Code 客户端、Agent Identity 和 Spark 影子账号。管理员可安全复制拥有静态凭据的账号, 但新副本默认不可调度且不会继承运行态 quota/probe/cache 状态。
- Channel: 模型平台定价和渠道能力管理。
- UsageLog: 请求用量记录, billing, token, endpoint, service tier, image/video metadata 等; 视频行记录 `video_count`, `video_resolution`, `video_duration_seconds` 以支持按秒审计计费。
- BatchImageJob/Item/Event: 批量生图任务、单项结果与事件流, 配合用户 frozen balance / hold / settlement / download cleanup。
- SubscriptionPlan/UserSubscription: 套餐和用户订阅。
- PaymentOrder/PaymentProviderInstance/PaymentAuditLog: 内置支付系统。
- GrokOAuthClient/Grok quota snapshot: Grok/xAI OAuth 与订阅配额快照支撑, 运行态快照主要保存在账号 `extra`。
- RedeemCode/PromoCode/Affiliate: 兑换码, 优惠码, 邀请返利。
- Ops: error log, upstream error, metrics, alert, dashboard aggregation。
- ChannelMonitor: 渠道监控, 模板, 历史和 daily rollup。
- SecuritySecret/IdempotencyRecord/TLSFingerprintProfile/ErrorPassthroughRule/ContentModeration: 安全和可靠性支撑。

## Ent Schema

位置: `backend/ent/schema/`。

当前重要 schema 文件包括:

- `user.go`, `auth_identity.go`, `auth_identity_channel.go`
- `api_key.go`, `group.go`, `account.go`, `account_group.go`
- `usage_log.go`
- `batch_image_job.go`, `batch_image_item.go`, `batch_image_event.go`
- `subscription_plan.go`, `user_subscription.go`
- `payment_order.go`, `payment_provider_instance.go`, `payment_audit_log.go`
- `channel_monitor.go`, `channel_monitor_history.go`, `channel_monitor_daily_rollup.go`, `channel_monitor_request_template.go`
- `setting.go`, `security_secret.go`, `idempotency_record.go`

`users` 角色与管理权限:

- `role` 支持 `admin`、`sub_admin`、`user`。
- `admin_permissions` 是 `JSONB NOT NULL DEFAULT '[]'::jsonb`, 仅 `sub_admin` 保存权限码; 用户离开该角色时 service 必须清空数组, 避免 stale 权限复用。
- 权限按账号保存, 不自动给存量子管理员授予后续新增权限。创建/更新时未知权限码返回 400。

修改 schema 后:

```bash
cd backend
go generate ./ent
go generate ./cmd/server
```

生成代码在 `backend/ent/`, 必须随 schema 一起提交。

## SQL Migration

位置: `backend/migrations/`。

运行器: `backend/internal/repository/migrations_runner.go`。

关键规则:

- migration 启动时自动执行。
- `schema_migrations` 表记录 filename, checksum, applied_at。
- checksum 是 trimmed file content 的 SHA256。
- 已应用 migration 不可修改, 不可删除, 不可重命名, 不可重排。
- 普通 `*.sql` 在事务内执行。
- `*_notx.sql` 非事务执行, 仅用于 `CREATE INDEX CONCURRENTLY` / `DROP INDEX CONCURRENTLY` 等场景。
- `_notx.sql` 必须写幂等 SQL, 如 `CREATE INDEX CONCURRENTLY IF NOT EXISTS`。

新增 migration 时只新增下一个编号文件, 不修旧文件。

当前调度性能相关索引:

- `backend/migrations/150_account_group_scheduler_indexes_notx.sql` 为 `account_groups` 新增 `(group_id, priority, account_id)` 和 `(account_id, priority, group_id)` 并发索引, 用于账号分组调度查询; 这是 `_notx.sql`, 必须保持 `CREATE INDEX CONCURRENTLY IF NOT EXISTS`。
- `backend/migrations/151_account_autopause_expiry_index_notx.sql`(上游 v0.1.137)为 `accounts (expires_at)` 加部分索引(`deleted_at IS NULL AND schedulable AND auto_pause_on_expired`), 加速到期自动暂停扫描; `_notx.sql`。
- `backend/migrations/151_channel_monitor_jitter.sql`(上游 v0.1.137)为渠道监控加 `jitter_seconds`(每次调度在 `interval_seconds` 基础上 ± [0, jitter_seconds] 均匀随机偏移, 0=固定间隔与历史一致), 同步 Ent schema `channel_monitor`。
- `backend/migrations/152_scheduler_outbox_dedup_key.sql` + `153_scheduler_outbox_pending_dedup_key_index_notx.sql`(上游 v0.1.137)为 scheduler outbox 加 `dedup_key` 列与 pending 部分唯一索引, 配合 claim 时释放 / 消费后清理(10s grace)防止快照事件重复。
- `backend/migrations/154_add_ops_system_logs_api_key_id.sql` + `155_add_ops_system_logs_api_key_id_index_notx.sql`(上游 v0.1.140)为 `ops_system_logs` 新增 `api_key_id` 与 `(api_key_id, created_at DESC)` 并发索引, 支持系统日志按 API Key 精确筛选和清理。
- `backend/migrations/156_content_moderation_matched_keyword.sql`(上游 v0.1.140)为 `content_moderation_logs` 新增 `matched_keyword`, 用于风控关键词拦截审计。
- `backend/migrations/157_user_platform_quotas_add_grok.sql`(上游 v0.1.140)重建 `user_platform_quotas.platform` CHECK 约束, 将 `grok` 纳入允许平台; 这是旧约束的超集, 用于修复注册/补全平台 quota 快照时写入 Grok 默认配额失败。
- `backend/migrations/154_account_spark_shadow.sql` + `154a_account_spark_shadow_indexes_notx.sql`(上游 v0.1.142/v0.1.143)为 `accounts` 增加 `parent_account_id`、`quota_dimension(global|spark)`、父账号外键和 active spark 影子一父一影子部分唯一索引。影子账号不能自持凭据, 软删除后可重建同母账号 shadow。
- `backend/migrations/158_add_group_peak_rate_multiplier.sql` 为 `groups` 增加 `peak_rate_enabled`, `peak_start`, `peak_end`, `peak_rate_multiplier`; 仅订阅类型分组可启用, 窗口格式 `HH:MM`, 不支持跨天, 高峰因子只乘入 token 计费倍率, 图片按次倍率不受影响。
- `backend/migrations/158_enable_grok_media_generation_groups.sql` 回填既有 Grok group 的 `allow_image_generation=true`, 支撑 Grok images/videos media 路由复用图片能力 gate。
- `backend/migrations/159_batch_image_foundation.sql` 到 `169_batch_image_parent_batch.sql` 是 batch image 任务基础表、用户 frozen balance、provider refs、定价快照、分组 gate、默认折扣/hold、下载/删除、失败隐藏、任务名和 parent batch 的连续迁移; 这些 migration 已进入上游, 只能追加后续编号, 不要改旧文件。
- `backend/migrations/170_add_grok_video_pricing_controls.sql` 为 `groups` 增加视频独立倍率和 480p/720p/1080p 单价; `171_allow_video_usage_without_image_size.sql` 放宽旧 image size 约束; `172_video_per_second_billing_metadata.sql` 为 `usage_logs` 增加视频数量、分辨率、时长并把价格口径明确为 USD/s。视频总成本为分辨率每秒单价乘请求时长; token-mode 渠道的视频行也必须通过约束并完整落 usage。
- `backend/migrations/174_add_usage_logs_api_key_latest_ip_index_notx.sql` 为每个 API Key 查询最近非空 IP 增加 `(api_key_id, created_at DESC, id DESC) INCLUDE (ip_address)` 部分并发索引; API Key 列表查询还会限制每个 key 的 latest-IP 子查询, 避免扫描全部历史。
- `backend/migrations/174_group_web_search_price_per_call.sql` 为 `groups` 增加可空 `web_search_price_per_call DECIMAL(20,8)`。NULL 使用内置默认价 0.01 USD/次; 两个 `174_` 文件按完整文件名独立执行, 不得为数字前缀重复而重命名。
- `backend/migrations/174_add_usage_log_long_context_billing.sql` 为 `usage_logs` 增加 `long_context_billing_applied`; `175_default_openai_long_context_billing.sql` 将既有 OpenAI 账号的 `extra.openai_long_context_billing_enabled` 默认写为 `false`。是否应用长上下文费率由最终凭据账号控制, Spark shadow 必须先解析母账号。
- `backend/migrations/175_add_ops_system_logs_host.sql` + `175a_add_ops_system_logs_host_index_notx.sql` 为 `ops_system_logs` 增加有长度边界的 `host` 和 `(host, created_at DESC)` 并发索引, 支持多实例日志按主机筛选。
- `backend/migrations/176_channel_monitor_grok_provider.sql` 扩展 channel monitor provider CHECK 与请求模板, 允许 `grok`; 同步 Ent schema 和 fixture migration test。
- `backend/migrations/177_add_sub_admin_permissions.sql` 为 `users` 增加非空 JSONB `admin_permissions`, 默认空数组; Ent schema 与生成代码必须同步提交。
- `backend/migrations/177_add_subscription_plan_currency.sql` 为订阅套餐增加 display-only ISO 4217 `currency`; 空字符串保持旧套餐无币种标签。
- `backend/migrations/178_channel_image_input_price.sql` 与 `179_usage_log_image_input_tokens.sql` 分别增加渠道 `image_input_price` 和 usage log 的 `image_input_tokens` / `image_input_cost`; 图生图/图片编辑可把图片输入 token 与文本输入 token 分价, 但 `total_cost` 口径不变。
- `backend/migrations/180_audit_logs.sql` 新增 append-only `audit_logs` 及 created/actor/action/client IP 索引; request body 和 credential 只保存脱敏/截断值。
- `backend/migrations/181_group_duplicate_operation_id.sql` 为 group duplicate 增加 active partial unique operation identity, 用于模糊提交后的幂等恢复。
- `backend/migrations/181_prompt_audit.sql` 新增 `prompt_audit_jobs` 与 `prompt_audit_events` 及调度、request/user/API key/group/hash/时间查询索引。job 只存 hash、脱敏 preview、长度和状态等元数据；event 保存 scanner 决策、证据和策略版本。
- `backend/migrations/182_prompt_audit_full_prompt.sql` 为 `prompt_audit_events` 增加 `full_prompt TEXT NOT NULL DEFAULT ''`。当前源码会把未脱敏审计原文持久化到 event, 最多 65,536 rune, 仅详情查询加载；这条后续 migration 已改变 181 文件头部“raw prompts 不进 PostgreSQL”的早期说明, 数据保护判断必须以 182 和当前 repository 为准。
- `backend/migrations/183_ops_ingress_reject_aggregates.sql` 新增分钟桶入口拒绝聚合表, 唯一维度为 bucket/reason/route/protocol/client IP/user/API key；服务端在内存中有界聚合后批量 upsert, 不逐请求写库。
- `backend/migrations/184_auth_cache_invalidation_outbox.sql` 新增只保存 API Key SHA-256 的 durable outbox, 并在 API Key、用户、分组和独占分组授权关系变化时由 trigger 入队。worker 使用 lease/重试/二次安全失效保证多实例 L1/L2 收敛, 明文 API Key 不离开 `api_keys`。旧 `deleted_api_key_audits` 与 `ops_error_logs` 凭据归因列只允许在全量实例升级、dry-run 清理和恢复点确认后, 由 `backend/scripts/finalize-ingress-reject-cleanup.sql` 人工删除；该脚本不是自动 migration。

> 已知双 `151_` 前缀(上游 v0.1.137 自带): `151_account_autopause_expiry_index_notx.sql` 与 `151_channel_monitor_jitter.sql` 来自上游不同分支。runner 按**完整文件名** `sort.Strings` 排序并以 `WHERE filename = $1` 去重, 不依赖数字前缀唯一, 故两文件独立执行互不覆盖, 运行无影响; 不要为"对齐编号"去重命名已发布 migration(违反不可重命名/重排规则)。

## 支付领域

内置支付系统支持:

- EasyPay
- Alipay 官方
- WeChat Pay 官方
- Stripe
- Airwallex

主要路径:

- 后端支付抽象: `backend/internal/payment/`
- Provider: `backend/internal/payment/provider/`
- 支付服务: `backend/internal/service/payment_service.go`, `payment_config_service.go`, `payment_order_expiry_service.go`
- 前端用户支付页: `frontend/src/views/user/*Payment*`
- 前端管理支付页: `frontend/src/views/admin/orders/*`
- 文档: `docs/PAYMENT_CN.md`, `docs/ADMIN_PAYMENT_INTEGRATION_API.md`

支付订单状态包括:

- `PENDING`
- `PAID`
- `COMPLETED`
- `EXPIRED`
- `CANCELLED`
- `FAILED`
- `REFUND_REQUESTED`
- `REFUNDING`
- `REFUND_PENDING`
- `PARTIALLY_REFUNDED`
- `REFUNDED`
- `REFUND_FAILED`

支付回调必须验签, 成功后充值, 并支持支付成功但充值失败后的重试。

支付金额与订阅修复口径:

- 余额充值金额计算在 `backend/internal/service/payment_amounts.go`: `calculateCreditedBalance` 按充值倍率入账; 订阅套餐 price 是直付价, 不再用余额充值倍率反算支付金额; refund 金额按订单金额、实付金额和币种 fraction digits 计算。
- 余额扣费在 `usage_billing_repo.go` 先尝试 `balance >= amount` 的原子更新; 不足时仍写入负余额并返回 `BalanceOverdrafted`, 调用方必须据此处理防持续透支策略。
- 订阅订单履约需要应用兑换倍率并保持幂等审计; validity unit 支持单复数输入归一化。
- 余额/订阅履约使用 `payment_fulfillment.go` 的 5 分钟 lease: `PAID`/`FAILED` 或超时 `RECHARGING` 订单通过 status + `updated_at` 条件抢占, 完成/失败写回也按 lease version CAS, 防止旧 worker 覆盖新 worker; 通知和 audit action 仍需幂等去重。
- 管理端订单金额展示应优先读取订单自身 `currency` 字段决定币种符号, 不要只依赖当前 provider 默认币种。
- 退款 provider 可实现 `payment.RefundQueryProvider`; 网关退款返回 pending 时订单进入 `REFUND_PENDING`, 后台 `POST /api/v1/admin/payment/orders/:id/refund/query` 查询并最终落 `REFUNDED`/`REFUND_FAILED`。匿名 public out_trade_no 查单只返回最小状态字段; resume token 查单才返回支付结果页所需完整合同。

## 外部支付 Admin API

`docs/ADMIN_PAYMENT_INTEGRATION_API.md` 记录外部支付系统对接:

- 推荐认证: `x-api-key: admin-<64hex>`。
- 幂等写接口额外传 `Idempotency-Key`。
- 创建并兑换: `POST /api/v1/admin/redeem-codes/create-and-redeem`。
- 用户查询: `GET /api/v1/admin/users/:id`。
- 余额调整: `POST /api/v1/admin/users/:id/balance`。
- 购买页和自定义 iframe 会追加 `user_id`, `token`, `theme`, `lang`, `ui_mode`。

## 计费与用量

主要路径:

- `backend/internal/service/billing_service.go`
- `backend/internal/service/pricing_service.go`
- `backend/internal/repository/usage_log_repo.go`
- `backend/resources/model-pricing/model_prices_and_context_window.json`
- `backend/resources/model-pricing/README.md`

订阅配额重置:

- 管理端接口 `POST /api/v1/admin/subscriptions/:id/reset-quota` 接收 `daily`, `weekly`, `monthly` 三个布尔字段, 至少一个为 true。
- `SubscriptionService.AdminResetQuota` 只重置被选中的用量窗口, 并在成功后失效订阅缓存和 billing cache。
- API Key `GET /v1/usage` 的 unrestricted subscription 响应在 `subscription.weekly_window_start` 返回周窗口起点, 与 daily/weekly/monthly usage 和 limit 一起供客户端展示当前周口径。
- API Key 鉴权发现订阅窗口过期时必须同步调用 `EnsureWindowMaintenance`, 用 expected window start 做条件重置并回读数据库快照后再校验限额; 不再异步清零后直接放行。管理员 `ResetUsageWindows` 是显式重置, 会原子更新所选窗口并返回刷新后的订阅。
- 前端全量“重置配额”会同时传 `daily/weekly/monthly=true`; “重置日限”只传 `daily=true`, 周/月窗口保持不变。
- 支付订单履约时, 余额充值和订阅购买都会尝试邀请返利。订阅履约先写 `SUBSCRIPTION_ASSIGNED` 审计再执行返利, 最后 `SUBSCRIPTION_SUCCESS`; 历史已有 `SUBSCRIPTION_SUCCESS` 或新审计时不会重复延长订阅。返利幂等通过 `payment_audit_logs` 的 `AFFILIATE_REBATE_APPLIED` / `AFFILIATE_REBATE_SKIPPED` 动作占位和 `order_id, action` 唯一约束防重, SQL 会按 PostgreSQL 与 SQLite 方言分别生成占位符和时间函数。

配置:

- `pricing.remote_url`
- `pricing.hash_url`
- `pricing.fallback_file`
- `billing.circuit_breaker`
- `dashboard_aggregation`
- `usage_cleanup`

计费相关修改要同时检查用量写入, dashboard aggregation, subscription progress, billing cache 和前端展示。

Codex alpha search 按次计费:

- `OpenAIGatewayService.ForwardAlphaSearch` 只有上游 2xx 时返回 `WebSearchCalls=1`; 上游错误已直接透传时返回空结果, 不计费。
- `BillingService` 对成功调用使用分组 `web_search_price_per_call`; NULL 回落 0.01 USD/次, 显式 0 必须保留为免费, 不能被默认价覆盖。
- 计费结果仍写 usage log, 需要保持用户余额、订阅和 user x platform quota 的原子扣减/回滚语义。

用量缓存 token 拆分: `UsageLogStats` 与 repository 聚合把缓存 token 拆为 `cache_creation_tokens`(cache write/缓存创建)与 `cache_read_tokens`(缓存命中), 管理端和用户侧用量统计 DTO/卡片都展示 `total_cache_creation_tokens` / `total_cache_read_tokens` 明细。OpenAI usage 的总 input 要扣除 cache read 和 cache write 后再计算普通输入; GPT-5.6 的 cache-write 单价来自模型价格或渠道显式覆盖, 显式 0 必须保留。修改用量聚合或展示时要保持三类 token 互斥统计。

OpenAI 长上下文计费是账号级 opt-in: `accounts.extra.openai_long_context_billing_enabled` 默认 `false`, 只有该账号真实上游按 OpenAI API 长上下文阈值收费时才开启。计费结果把是否应用写入 `usage_logs.long_context_billing_applied`, 供审计和管理端用量表展示; shadow 账号沿用母账号策略, 不能按 shadow 的空凭据自行决定。

用户侧用量统计已与管理端过滤口径对齐: `UsageLogFilters` 支持 `group_id`、请求模型源(`requested`)、`request_type`/legacy `stream`、`billing_type`、`billing_mode` 和日期范围; `/api/v1/usage/dashboard/snapshot-v2` 可以一次返回 trend、model、group 分布。新增 usage 聚合字段时要同时检查 `usage_handler.go`、`usage_service.go`、`usage_log_repo.go`、前端 `frontend/src/api/usage.ts` 和 `UsageView.vue`。

Token Analysis 选中用户趋势口径:

- 图表权威来源是 `usage_logs`, 不使用 `token_analysis_request_summaries`; 用户排行仍是归档样本, 因而排行值与“计费用量趋势”可能不同。
- 首版指标只有总 Token, 计算为 `input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens`; 费用字段虽然沿用现有趋势 DTO 聚合, 前端不提供指标切换。
- 日/小时桶固定按 `Asia/Shanghai`; 日范围最多 90 个自然日, 小时仅单日。原始 `usage_logs` 的默认保留边界决定该能力不承诺查询 90 天以前的数据, 不新增日/小时预聚合表。
- 精确选人 SQL 使用已有 `idx_usage_logs_user_created (user_id, created_at)`; 该列序符合 user ID 等值过滤 + created_at 范围过滤。本功能没有 schema/migration 变更, 是否需要新索引只能由真实 `EXPLAIN ANALYZE` 证据决定。

组织用量报表口径:

- 完整设计见 `docs/features/organization-usage-report-design-cn.md`。
- 活跃用户是 `users.deleted_at IS NULL AND status='active'`, 角色同时包含 user/admin。组织按当前 `users.email` 的 `@` 后域名做大小写不敏感精确匹配: `xunyou.com -> xunyou`, `wsdashi.com -> wsdashi`, 其他域名及其子域名都归 `other`。
- 报表指标固定为 requests、input/output/cache creation/cache read tokens、total tokens 和 actual cost; total tokens 是四类 token 之和。`used_users` 表示选区内至少存在一条 usage log 的活跃用户。
- 个人 peak 与团队 champion 都按 `total_tokens DESC, actual_cost DESC, requests DESC, user_id ASC` 选择; 个人同用户同指标周期再按 period start 保持确定性。没有 usage log 的用户 peak 为 null。
- `as_of` 由 service 规范化为 UTC 并裁剪到不晚于服务器当前时间, 防止客户端时钟超前; 日期 end 只进一步限制 usage log 查询, 不改变响应中的 canonical `as_of`。它不是密码学签名, 也不回溯用户状态或邮箱; active user 和组织分类始终按查询时的当前 `users` 数据计算。

User x platform quota:

- 读取路径使用 billing cache 和 sentinel entry 缓存无 limit 场景; sentinel TTL 由 `billing.user_platform_quota_sentinel_ttl_seconds` 控制, 默认短于正常 quota cache TTL。
- 写入路径可启用 `UserPlatformQuotaUsageFlusher`, 将 user x platform quota usage 聚合后批量刷入数据库; 配置在 `database.user_platform_quota_flusher_enabled`, `database.user_platform_quota_flush_interval_ms`, `database.user_platform_quota_flush_batch_size`。
- 多实例部署时 quota flusher 属于后台写任务, 必须结合 leader lock 或等价单主约束, 避免重复刷写。

失败请求与删除审计:

- `OpsErrorLog` 可向用户侧和管理侧展示失败请求, 用户侧视图必须走脱敏 DTO 和可见性开关。
- 删除用户/API Key 后仍需要支持错误日志归因和审计查询; 相关 migration 包含 deleted API key audit、ops error log api key prefix、user time index 等。
- 图片生成计费包含 image token/metadata 路径; 修改图片用量展示或计费时同时检查 `imageUsage` 前端工具、usage log 写入和 rate-limit cooldown/failover 逻辑。
- OpenAI 图片请求写 usage 时, 若渠道 token 模式没有任何有效 token/image token 定价并触发缺价兜底, 仍写入零费用但 `billing_mode=image`, 避免 Token Analysis 和用量筛选把图片请求误归为 token 计费。

## 账号复制与凭据所有权

- `POST /api/v1/admin/accounts/:id/duplicate` 只接受 API Key、upstream、Bedrock 和 service account 等自持静态凭据类型；OAuth/cookie 等旋转凭据及 Spark/其他 credential shadow 必须拒绝，shadow 应从母账号重新创建。
- 副本深拷贝静态 credentials、业务配置、proxy 和有序 account-group priority；若源账号处于 proxy fallback，复制配置 origin 而不是暂态 fallback 目标。账号与 group 关系通过 `AccountDuplicateRepository.CreateWithAccountGroups` 在同一事务创建，并写 scheduler outbox。
- 副本名称追加 ` (Copy)`、`schedulable=false`，需要管理员检查后再启用。外部同步 identity、配额窗口、provider probe、quota snapshot、调度暂态和旧 duplicate operation id 不得继承。
- 幂等 identity 由 admin actor scope、源账号 ID 和 `Idempotency-Key` 派生后存入副本 `extra.duplicate_operation_id`; coordinator 不确定响应是否持久化时只做只读恢复，不重复创建副本。

## 模型与平台

网关支持多平台调度, 常见 platform 包括 Claude/Anthropic, OpenAI, Gemini, Antigravity, Grok/xAI。Group 的 platform 决定部分路由行为和协议兼容分流。

- `PlatformGrok = "grok"` 已加入后端 domain/service 常量和前端 `GroupPlatform` / `AccountPlatform`; user x platform quota 的允许平台和数据库 CHECK 约束都包含 `grok`。新增 quota 维度时要同步 Ent schema validate、service `AllowedQuotaPlatforms`、SQL CHECK 约束、前端 Settings/UserPlatformQuota UI。
- Grok OAuth 账号的 quota 由 xAI 响应头快照和本地 usage 聚合共同展示: `grok_request_quota`, `grok_token_quota`, `grok_retry_after_seconds`, `grok_entitlement_status`, `grok_local_usage` 等字段属于账号 usage DTO 扩展。
- OpenAI Spark 影子账号是 OpenAI 账号的 `quota_dimension=spark` 子账号, 凭据透传母账号, 调度/分组/并发可独立配置, 但导出备份会排除 shadow 并返回 `skipped_shadows`。spark 请求按 `gpt-5.3-codex-spark` 模型路由, 计价固定映射到 `gpt-5.1-codex`。

模型映射和白名单相关改动要检查:

- group 默认映射。
- account 可用模型和 whitelist。
- channel pricing 和 model default pricing。
- OpenAI/Codex responses, chat completions, embeddings, images 路径。
- Gemini v1beta 路径。

## 代理有效期与失败回退

数据模型(migration `149_proxy_expiry_fallback.sql`, 自上游 v0.1.135):

- `proxies` 新增 `expires_at`(有效期)、`fallback_mode`(`none` / `proxy` / `direct`)、`backup_proxy_id`(自引用备用代理, `ON DELETE SET NULL`)、`expiry_warn_days`(默认 7, 临期提醒天数)。
- `accounts` 新增 `proxy_fallback_origin_id`, 记录手动回切来源。
- 后台逻辑见 `backend.md` 的"代理有效期与失败回退"。

> 已知约束不一致(上游自带, 当前不修): `backend/ent/schema/proxy.go` 的 `backup_proxy` edge 用 `.Unique()`(无反向 `.From()` 边), 生成的 `ent/migrate/schema.go` 把 `backup_proxy_id` 标记为唯一列; 但 migration 149 是普通外键 + 普通索引(非唯一)。本项目建表只走 SQL migration、不使用 Ent auto-migrate, 故真实库为非唯一(多个代理可共用同一备用代理), 与回退链逻辑一致, 运行无影响。修改该 edge 或新增相关 migration 时需对齐二者。详见 `docs/features/sub2api-v0.1.135-merge-review-cn.md` P2。
