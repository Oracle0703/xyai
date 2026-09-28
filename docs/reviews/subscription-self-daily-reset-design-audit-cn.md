# 《用户订阅自助重置日额度方案》设计审核

被审文档：`docs/features/subscription-self-daily-reset-design-cn.md`（2026-09-22，设计稿，未实现）
审核日期：2026-09-22
代码基线：`feature/hy/10208_sub_admin_assign_subscription`（基于 `main@d120d499e`）
审核方式：对照订阅重置、幂等、组织口径和用户订阅页源码核对设计结论。未执行迁移、未启动服务、未做真实重置。

本文与 `docs/features/subscription-self-daily-reset-design-review-cn.md` 相互独立。

## 结论

设计可以按现有订阅重置链路落地。计次表、同一 PostgreSQL 事务、提交后再失效缓存，这三件事和现有实现对齐，也没有改计费主路径。

落地前有三处必须写死，否则会出现当天无限重置，或一次失败点击在次日被当成新的清零。自助次数的权威是计数表。幂等记录只保证同一次点击不会执行两遍。

## 与现有实现一致的部分

| 设计结论 | 核对结果 |
| --- | --- |
| 用户侧现有幂等 helper 在 coordinator 缺失时直接执行，不能照搬 | `backend/internal/handler/idempotency_helper.go` 在 coordinator 为 nil 时调用 `execute` 并返回成功 |
| 管理端按筛选重置在 coordinator 缺失且要求原子成功时直接失败 | `backend/internal/handler/admin/idempotency_helper.go` 返回 `ErrIdempotencyStoreUnavail` |
| `ResetUsageWindows` 能进入外层事务 | `backend/internal/repository/user_subscription_repo.go` 使用 `clientFromContext` |
| `AdminResetQuota` 不能整段包进新事务 | `backend/internal/service/subscription_service.go` 在方法内部同步失效本机缓存和 billing cache |
| 日额度按配置时区自然日，不是点击后再等 24 小时 | `UserSubscription.automaticDailyWindowStartAt` 使用 `timezone.StartOfDay` |
| 一次性日卡不自动补日额度 | `HasOneTimeDailyQuota` 为真时，`automaticDailyWindowStartAt` 直接返回不可自动重置 |
| 组织按邮箱域名分成迅游、速宝、其他，子域名归入其他 | `organizationUsageOrganizationExpression`：`LOWER(SPLIT_PART(email,'@',2))` |
| 子管理员权限是路由白名单，未知管理路由默认拒绝 | `backend/internal/service/admin_permission.go` 的 `CanAccessAdminRoute` |
| 用户订阅页自己拉列表，全局 store 另有 60 秒缓存和 5 分钟轮询 | `SubscriptionsView.vue` 调用 `getMySubscriptions()`；`stores/subscriptions.ts` |
| 用户路由已有 JWT、面板限流和审计 | `backend/internal/server/routes/user.go` |
| 设置表普通读取不进事务 | `setting_repo.go` 的 `Get` 固定使用 `r.client` |
| 续费同一分组沿用原订阅 ID | `AssignOrExtendSubscription` / `renewedSubscriptionTerm` 更新原行 |

行锁应使用现成的 `GetByIDForUpdate`。它会 `SELECT ... FOR UPDATE`，但不加载用户和分组。锁住订阅行之后，要在同一事务里再读邮箱、分组日限和分组状态。

## 落地前必须写死

### 1. 计数日期不要用会被时区换算的 `date`

日额度用 `timezone.StartOfDay()`，配置时区默认是 `Asia/Shanghai`。上海当天 0 点对应 UTC 前一日 16:00。把这个 `time.Time` 交给驱动写成 PostgreSQL `date` 时，会话时区若是 UTC，库里的日期会变成前一天。集成测试的数据源就使用 `TimeZone=UTC`。

设计稿的公式是 `quota_date == 今天 ? used_count : 0`。日期一旦偏一天，刚扣完的次数会被当成不是今天，剩余次数立刻回到满额，当天可以反复重置。错位窗口主要落在服务端本地 00:00 到 08:00。

保存方式与现有 `daily_window_start` 对齐：列用 `quota_day_start timestamptz`，值就是 `timezone.StartOfDay(now)`，比较用同一时刻。不要和报表里写死的 `Asia/Shanghai` 混用。若坚持用 `date`，只能绑定 Go 算出的日历字符串 `timezone.StartOfDay(now).Format("2006-01-02")`，SQL 里不要出现 `CURRENT_DATE`、`NOW()::date` 或任何 `::date` 强转。

验收要在 `TimeZone=UTC` 的会话下跑 23:59、00:30、08:30。读回的计数日必须是配置时区的当天。

### 2. 幂等键要在进 coordinator 之前拒绝空键，前端按结果分三类

服务端默认 `idempotency.observe_only=true`。`RequireKey` 只在非观察模式拦截空键；空键会跳过幂等、也不包事务，直接执行业务函数。这正好会写出设计稿禁止的「只扣次或只清零」。

空键必须在 handler 里先拒绝，和 `ResetDailyFiltered` 一样。coordinator 或事务能力缺失时返回 503。service 写入前再断言当前上下文里已经有事务，避免以后改调用链时把检查漏掉。

`AtomicSuccess` 遇到业务错误会回滚，再把幂等记录标成 `failed_retryable`，默认退避约 5 秒。它不会记住这次 409 的业务原因。

默认成功记录保留 24 小时，还会被 `idempotency.default_ttl_seconds` 缩短。这次接口必须显式传入 `TTL: 48 * time.Hour`，不能用 `DefaultWriteIdempotencyTTL()`。

前端按结果处理：

| 结果 | 动作 |
| --- | --- |
| 超时、网络失败、503 | 原键、原 `quota_date` 重试 |
| 已收到 `SELF_RESET_DAY_CHANGED` | 重新拉状态，换新键和新日期 |
| 已收到次数用尽、无需重置、未开放 | 展示原因；用户稍后再点时用新键 |

`quota_date` 用状态接口返回的服务端日期。成功响应只当本次回执，卡片额度以随后的列表和状态接口为准。重放快照不能覆盖新的权威状态。

计数表才是当天次数的权威。幂等记录过期后，旧请求仍靠 body 里的 `quota_date` 与服务端当天比较来拒绝。

### 3. 「有效日用量」必须走现有日窗口函数

普通订阅跨日后，库里的 `daily_usage_usd` 可能仍是昨天的数，但 `automaticDailyWindowStartAt` 会把它视为 0。这种记录不能扣次，也不能把按钮显示成可重置。

一次性日卡 `HasOneTimeDailyQuota()` 不会自动跨日清零，库里的用量仍然有效。设计稿允许它在到期前手动重置，且不改 `expires_at`。这和现有管理端重置一致，但会让「整个有效期只有一份日额度」的日卡再得到一份。组织次数配置没有按商品分开，`other` 一旦为 1，外部日卡买家会一起得到这份额度。若这不是要卖的权益，第一版直接拒绝，`disabled_reason` 用 `ONE_TIME_QUOTA`。若确定要开放，需要单独开关，默认关闭。

列表接口已经通过 `normalizeExpiredWindows` 把过期日窗口显示为 0。按钮可用性只认状态接口的 `can_reset`，不用列表上的金额自己推断。状态返回前按钮保持禁用。

## 计次身份

建议第一版按用户共享每天 1 次，而不是每个订阅各 1 次。

配置是按迅游、速宝、其他用户分的，这是员工福利口径，不是套餐附带权益。独立计次时，总次数随订阅数上涨：5 个分组订阅就是 5 份完整日额度。共享计次是每个账号每天固定一次，点哪个订阅就清哪个。两套方案的事务、归属检查和测试都省不掉，开发速度不是选择依据。

附属表主键一旦上线，当天已用次数无法原地改挂到另一种身份上。开工前把「每个订阅独立」确认成最终口径。若确认独立，可以保持 `subscription_id` 主键，并额外保存不参与唯一约束的 `user_id`，便于以后改成共享计次。

三类组织的初始值都是 1。其他用户包含非迅游、非速宝邮箱。若其中有外部付费用户，上线即送一份额外日额度。需求若确实如此，保持 1；若只先给内部组织，把其他用户的初始值设为 0。

Go 侧目前没有「邮箱 → xunyou / wsdashi / other」的函数，只有 SQL `CASE`，以及组织码到域名的反向映射。反向映射的常量里没有 `other`。需要一个函数，并用大小写、子域名、多个 `@`、空域名与 SQL 表达式做对照测试。`SPLIT_PART(email, '@', 2)` 取的是第二段，不能用最后一个 `@`。

## 事务、锁和缓存

策略读取不要在持有订阅行锁时调用 `SettingRepository.Get`。那个方法固定走默认连接，持锁期间会再申请一条连接。策略在开事务或加锁之前读数据库当前行；锁内只复核日期、有效日用量、剩余次数、归属和订阅状态。不要走 `SettingService` 里带进程内缓存的读取。

同一订阅的并发请求靠 `GetByIDForUpdate` 串行。计数行第一次写入用 `INSERT ... ON CONFLICT (subscription_id) DO UPDATE`。不存在的行锁不住。

缓存失效对齐按筛选批量重置：本机缓存、`InvalidateSubscription`，以及 `PublishSubscriptionCacheInvalidation`。单条 `AdminResetQuota` 不发跨实例通知。billing cache 大约 5 分钟过期，漏掉通知时其他实例会继续按旧额度拦截。缓存失败只告警。

并发计费的 `IncrementUsage` 是对同一行的原子加。重置把 `daily_usage_usd` 写成 0 之后才提交的入账，仍可能把日用量再加回去。这和现有管理端重置相同。

## 接口和权限

管理端 `GET/PUT /api/v1/admin/subscriptions/self-reset-policy` 要注册在 `/admin/subscriptions/:id` 之前。这两条路由不要写进子管理员 `admin.subscriptions` 白名单。完整管理员走全路由放行；子管理员对未知路由默认拒绝。需要一条测试钉住该路由不在任何子管理员规则里。

用户页上的自助重置仍然计次。次数不限的只有现有管理端重置。面板限流对完整管理员的豁免不代表自助次数豁免。

非本人或不存在的订阅统一 404。用户身份只取认证上下文。

`disabled_reason` 收成封闭集合，至少包括：

| 值 | 含义 |
| --- | --- |
| `POLICY_DISABLED` | 组织上限为 0 |
| `NO_DAILY_LIMIT` | 分组没有日限 |
| `NO_USAGE` | 当前有效日用量为 0 |
| `DAILY_LIMIT_REACHED` | 当天次数已用尽 |
| `SUBSCRIPTION_INACTIVE` | 过期、暂停或撤销 |
| `GROUP_DISABLED` | 分组停用 |
| `ONE_TIME_QUOTA` | 一次性日卡被拒绝时 |

`next_reset_at` 写成 `timezone.StartOfDay(now).AddDate(0, 0, 1)`。机会恢复时间和订阅到期时间分两处展示。确认文案写明周/月额度不变；日额度清零后，周或月已用尽的请求仍会被拒绝。

用户路由上还有 `BackendModeUserGuard`。后端模式关闭时的拒绝要和 `SELF_RESET_DISABLED` 分开，避免提示成「组织未开放」。

成功后页面列表和全局 store 都要刷新。store 的 `invalidateCache()` 只清时间戳，还要 `fetchActiveSubscriptions(true)`。两处读的是不同接口。

审计会记录每次 POST，包括重放和失败。对账以计数表为准。同一订阅续费仍是原来的订阅 ID，当天已用次数保留。

按钮若必须放在续费后面，窄屏验收要覆盖卡片头部。更稳的位置是日用量进度条那一行：无日限时整块本来就不渲染，也不挤状态徽章。

## 验收补充

| 场景 | 必须满足 |
| --- | --- |
| UTC 会话跨日 | 上海时区 0 点前后写入后，读回的计数日是当天，不能在 00:00–08:00 重复扣次 |
| 空幂等键 | `observe_only=true` 且不带键时返回 400，订阅用量和计数表都不变 |
| 有效用量 | 一次性日卡跨日后的旧用量按最终产品口径处理；普通订阅的昨日残留不扣次 |
| 并发 | 两个不同幂等键并发时，成功次数不超过组织上限；提交后另一实例的订阅缓存已失效 |
| 权限 | 子管理员不能读写策略；用户不能重置他人订阅 |
| 前端 | 状态未返回时按钮禁用；超时沿用原键；确定的业务拒绝不沿用原键去改日期 |
