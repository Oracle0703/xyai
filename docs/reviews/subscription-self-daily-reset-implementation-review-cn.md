# 用户订阅自助日重置实现审核

日期：2026-09-23。分支：`feature/hy/10208_sub_admin_assign_subscription`，基于 `main@d120d499e`。范围：该分支上尚未提交的实现，主体是自助日重置，同时看了同工作区的子管理员分配订阅接口。

对照：`docs/features/subscription-self-daily-reset-design-cn.md`（v2.1）、`docs/features/subscription-self-daily-reset-review-decisions-cn.md`、`docs/features/subscription-self-daily-reset-implementation-cn.md`。

**结论：可以合入。** 未发现会超次、串用户清零，或扣次与日用量清零只完成一半的问题。下面三项是低优先级缺口，不改变扣次结果。PostgreSQL 集成测试本轮没有重跑。

> 2026-09-23/24 追记：本文前两项前端问题，以及复核中另外发现的列表加载失败回归、待确认横幅锁住全部订阅等，已修复，见交付记录“审核后修复”和“二轮复核修复”。未知结果的原键仍按订阅保存在 sessionStorage，关闭弹窗或刷新页面后再次确认仍沿用原键。第三项（分配接口不检查分组是否启用）属于同分支的分配功能，这次没有改。

## 审核范围

| 层次 | 主要路径 |
| --- | --- |
| 迁移 | `backend/migrations/241_subscription_self_daily_reset.sql` |
| 后端 | `backend/internal/service/subscription_self_reset.go`、`backend/internal/repository/subscription_self_reset_repo.go`、`backend/internal/handler/subscription_self_reset_handler.go` |
| 权限与路由 | `backend/internal/service/admin_permission.go`、`backend/internal/server/routes/user.go`、`backend/internal/server/routes/admin.go` |
| 前端 | `frontend/src/composables/useSubscriptionSelfReset.ts`、`frontend/src/views/user/SubscriptionsView.vue`、`frontend/src/components/admin/subscription/SubscriptionSelfResetPolicyDialog.vue` |
| 同分支分配接口 | `backend/internal/handler/admin/subscription_assignment_options.go` |

分三轮：业务口径、权限与事务、前端与验证证据。

## 第一轮：业务口径

实现与已定稿规则一致。

| 口径 | 审核结果 |
| --- | --- |
| 计次单位 | 每个订阅一行 `subscription_self_daily_reset_usage`。组织上限 N 对每个订阅分别生效，不能把次数转给另一订阅 |
| 自然日 | 取锁后的同一个 `now` 生成 `timezone.StartOfDay(now)` 的 `YYYY-MM-DD`。读回用 `to_char(..., 'YYYY-MM-DD')`，写入用 `$2::date`。旧日期在读取时视为 0，不另加定时任务 |
| 禁用顺序 | 订阅不可用 → 分组停用 → 一次性日卡 → 无日限 → 组织关闭 → 次数用尽 → 当日无用量。能重置时 `disabled_reason` 为 null |
| 日卡 | 复用 `HasOneTimeDailyQuota()`，看 `starts_at/expires_at` 的日历跨度。十年订阅进入最后一小时仍可按普通订阅判定 |
| 上限变化 | 剩余 = max(上限 − 今日已用, 0)。当天把上限从 3 降到 1 且已用 2 次时剩余为 0，已用次数不回退 |
| 组织关闭 | 上限 0 返回 403 `SELF_RESET_DISABLED`。管理端原重置不读写计数表 |
| 身份 | 策略 GET/PUT 只给完整管理员。用户入口不因角色跳过计次 |
| 刷新 | 成功后刷新页面订阅列表、自助状态和 `fetchActiveSubscriptions(true)`。未知结果留在按用户隔离的 `sessionStorage`，继续用原键和原 `quota_date` |

按钮只认 status 的 `can_reset`。POST 在事务内重新判定，不把此前的 status 当成成功承诺。确认文案写明只清当日已用额度，周/月额度和到期时间不变。

## 第二轮：权限、事务和并发

写路径是收口的。

1. `GetOwnedByIDForUpdate` 的条件同时包含认证 `user_id`、`subscription_id` 和未删除，再 `FOR UPDATE`。不匹配统一为订阅不存在，不会先锁到其他用户的行。
2. Handler 在进入幂等协调器之前拒绝空 `Idempotency-Key`。全局 `idempotency.observe_only` 默认为开，也不能绕过：`Reset` 发现上下文里没有事务就直接返回，不会先清零。
3. scope 绑定 `user:{id}`。指纹使用具体订阅 ID 和提交的 `quota_date`，不是路由模板里的 `:id`。TTL 显式传入 `48 * time.Hour`。
4. 扣次 UPSERT、日窗口清零和幂等成功记录在同一个 PostgreSQL 事务里。`DeferIdempotencyPostCommit` 注册失败会让该事务回滚。
5. 回调只在提交成功或“提交已成功但确认丢失”的恢复路径执行。失效顺序复用批量日重置：本机同步缓存、billing cache 删除、跨实例发布。回滚不执行这段回调。
6. `GET/PUT /api/v1/admin/subscriptions/self-reset-policy` 不在任何子管理员白名单。页面按钮使用 `role === 'admin'`。`BackendModeUserGuard` 在后端模式开启时仍拦截非完整管理员，该提示与组织关闭的 `SELF_RESET_DISABLED` 分开。

同一订阅的不同幂等键堵在父行锁上。计费入账是 `daily_usage_usd = daily_usage_usd + 金额`，会等待这把锁，不会把清零前读到的旧用量覆盖回去。重置之后才完成的入账可以重新累加日用量，这与现有管理端重置语义一致。

策略在事务内、加锁前读取。邮箱归属在拿到订阅锁之后按当前用户解析。配置损坏或缺失时自助重置返回 503，不退回到默认 1 次。

## 第三轮：前端、测试和发布

审核当时复跑：

| 检查 | 结果 |
| --- | --- |
| 后端单元测试 | `go test -tags=unit -p 1 -count=1 ./internal/service ./internal/handler ./internal/server/middleware ./internal/server/routes -run 'SubscriptionSelfReset\|AdminPermission\|CanAccessAdmin\|SubAdmin\|AdminAuth\|BackendModeUserGuard\|SubscriptionBulkActionRoutes'` 通过 |
| 前端 | 自助重置页面、composable、策略弹窗，以及既有管理订阅三份用例，共 6 个文件、46 项通过 |
| PostgreSQL 集成 | 未重跑。并发、单连接池、回滚、DATE 和组织分类的断言来自阅读 `backend/internal/repository/subscription_self_reset_integration_test.go` |

交付记录中的“回滚后缓存不失效”与协调器代码一致：业务错误或提交前回滚不会跑 post-commit 回调。跨实例测试本身只断言了成功重置后两个 `SubscriptionService` 都回读到日用量 0。该测试使用同一进程里的两个服务实例和 miniredis，不代替生产 Redis 集群或网络分区验收。

发布必须同时带上 migration `241`、后端和前端。只部署按钮时，缺表或缺配置会使状态接口失败，按钮保持禁用。

## 问题

| 严重度 | 位置 | 说明 |
| --- | --- | --- |
| 低 | `frontend/src/composables/useSubscriptionSelfReset.ts` 的 `confirm` | 提交前会把 `status` 置空。网络超时、401/408/429、5xx、处理中和退避会保留原键，但这个分支不调用 `refresh()`。卡片会停在“重置（—）”，直到切换可见性、次日定时刷新或整页重载。原操作不会因此多扣一次 |
| 低 | `frontend/src/components/admin/subscription/SubscriptionSelfResetPolicyDialog.vue` | 读取失败不填默认值，表单仅在读成功后出现，保存按钮在无 policy 时禁用。库中的策略 JSON 已损坏时，这个弹窗不能覆盖写入，只能重试读取或直接改数据库 |
| 低 | `backend/internal/handler/admin/subscription_assignment_options.go` | 注释写返回有效订阅分组，循环只排除非订阅类型。`assignOrExtendSubscription` 也不检查分组是否启用。管理页下拉框另外过滤了 `status === 'active'`。直接调用 API 仍可能分配停用分组。这属于同分支的分配接口，不影响自助重置扣次 |
| 说明 | `ParseSubscriptionSelfResetPolicy` | `1.5`、负数、缺组织和未知字段会拒绝。`1.0` 和 `1e0` 同样会被拒绝：Go 解码到 int 时报 `cannot unmarshal number 1.0 ... of type int`，不会收成 1（2026-09-23 复核更正）。前端 `v-model.number` 提交的是 `1`，不受影响 |

## 不作为缺陷的核对项

- 一次性日卡和没有日限的有效订阅仍显示禁用按钮，并带上对应原因。页面测试锁定了这个表现；服务端 POST 同样拒绝且不扣次。
- 机会恢复时刻由服务端 `StartOfDay(now).AddDate(0, 0, 1)` 算出。页面用该时刻与 `server_now` 的时间差安排下一次读取，不在本地把按钮改成可点。
- 次数上限的判断在持有订阅行锁之后、UPSERT 之前完成。当前没有第二条不拿这把锁就写计数表的路径。
