# 数据与领域基线

## 核心领域

Sub2API 的核心对象:

- User: 用户, 角色, 余额, OAuth identity, 属性, TOTP。
- API Key: 用户侧调用凭证, 关联 group, rate limit, quota, last used。
- Group: 调度和计费分组, 控制 platform, model mapping, rate multiplier, RPM, 支持模型范围。
- Account: 上游账号, 支持 OAuth/API Key/cookie/setup token 等类型, 可绑定 proxy, group, model whitelist 和 quota。
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

配置:

- `pricing.remote_url`
- `pricing.hash_url`
- `pricing.fallback_file`
- `billing.circuit_breaker`
- `dashboard_aggregation`
- `usage_cleanup`

计费相关修改要同时检查用量写入, dashboard aggregation, subscription progress, billing cache 和前端展示。

## 模型与平台

网关支持多平台调度, 常见 platform 包括 Claude/Anthropic, OpenAI, Gemini, Antigravity。Group 的 platform 决定部分路由行为和协议兼容分流。

模型映射和白名单相关改动要检查:

- group 默认映射。
- account 可用模型和 whitelist。
- channel pricing 和 model default pricing。
- OpenAI/Codex responses 路径。
- Gemini v1beta 路径。
