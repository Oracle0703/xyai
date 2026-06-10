# 数据与领域基线

## 核心领域

Sub2API 的核心对象:

- User: 用户, 角色, 余额, OAuth identity, 属性, TOTP。
- API Key: 用户侧调用凭证, 关联 group, rate limit, quota, last used。
- Group: 调度和计费分组, 控制 platform, model mapping, rate multiplier, RPM, 支持模型范围和自定义 `/v1/models` 列表。
- Account: 上游账号, 支持 OAuth/API Key/cookie/setup token 等类型, 可绑定 proxy, group, model whitelist 和 quota; OpenAI 账号支持 endpoint capability, pool retry status codes, quota threshold auto-pause, Codex CLI only 和允许 Claude Code 客户端。
- Channel: 模型平台定价和渠道能力管理。
- UsageLog: 请求用量记录, billing, token, endpoint, service tier, image metadata 等。
- SubscriptionPlan/UserSubscription: 套餐和用户订阅。
- PaymentOrder/PaymentProviderInstance/PaymentAuditLog: 内置支付系统。
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
- `subscription_plan.go`, `user_subscription.go`
- `payment_order.go`, `payment_provider_instance.go`, `payment_audit_log.go`
- `channel_monitor.go`, `channel_monitor_history.go`, `channel_monitor_daily_rollup.go`, `channel_monitor_request_template.go`
- `setting.go`, `security_secret.go`, `idempotency_record.go`

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
- `REFUNDED`

支付回调必须验签, 成功后充值, 并支持支付成功但充值失败后的重试。

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
- 前端全量“重置配额”会同时传 `daily/weekly/monthly=true`; “重置日限”只传 `daily=true`, 周/月窗口保持不变。

配置:

- `pricing.remote_url`
- `pricing.hash_url`
- `pricing.fallback_file`
- `billing.circuit_breaker`
- `dashboard_aggregation`
- `usage_cleanup`

计费相关修改要同时检查用量写入, dashboard aggregation, subscription progress, billing cache 和前端展示。

用量缓存 token 拆分: `UsageLogStats` 与 repository 聚合把缓存 token 拆为 `cache_creation_tokens`(缓存创建)与 `cache_read_tokens`(缓存命中), 前端 i18n 增加缓存创建/命中/命中率文案。修改用量聚合或展示时要保持两者分别统计。

User x platform quota:

- 读取路径使用 billing cache 和 sentinel entry 缓存无 limit 场景; sentinel TTL 由 `billing.user_platform_quota_sentinel_ttl_seconds` 控制, 默认短于正常 quota cache TTL。
- 写入路径可启用 `UserPlatformQuotaUsageFlusher`, 将 user x platform quota usage 聚合后批量刷入数据库; 配置在 `database.user_platform_quota_flusher_enabled`, `database.user_platform_quota_flush_interval_ms`, `database.user_platform_quota_flush_batch_size`。
- 多实例部署时 quota flusher 属于后台写任务, 必须结合 leader lock 或等价单主约束, 避免重复刷写。

失败请求与删除审计:

- `OpsErrorLog` 可向用户侧和管理侧展示失败请求, 用户侧视图必须走脱敏 DTO 和可见性开关。
- 删除用户/API Key 后仍需要支持错误日志归因和审计查询; 相关 migration 包含 deleted API key audit、ops error log api key prefix、user time index 等。
- 图片生成计费包含 image token/metadata 路径; 修改图片用量展示或计费时同时检查 `imageUsage` 前端工具、usage log 写入和 rate-limit cooldown/failover 逻辑。

## 模型与平台

网关支持多平台调度, 常见 platform 包括 Claude/Anthropic, OpenAI, Gemini, Antigravity。Group 的 platform 决定部分路由行为和协议兼容分流。

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
