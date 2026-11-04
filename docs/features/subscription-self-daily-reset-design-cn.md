# 用户订阅自助重置日额度设计

更新：2026-11-04。状态：**已实现并合入 main（首版提交 `72fc79106`，历史表提交 `267d8dbba`）。产品待定项已全部关闭。开放范围由服务端策略的灰度开关控制（§5），线上当前取值以生产 settings 为准；上线核对步骤见 §15。**

本文是用户订阅自助日重置的唯一文档，包含产品规则、数据模型、接口合同、事务与可靠性要求、代码位置、易错点和验证方式。两份设计审核、审核裁决清单、实现审核和交付记录已移出仓库，最后存在于提交 `26f6363af`（可用 `git show 26f6363af:docs/reviews/subscription-self-daily-reset-review-decisions-cn.md`、`git show 26f6363af:docs/features/subscription-self-daily-reset-implementation-cn.md` 等查看）；凡是对后续修改有约束的结论都已写入正文，被否决的审核建议集中在 §12。

相关功能：管理端按筛选批量日重置、按每日剩余额度排序见 [订阅每日剩余排序与批量重置设计](subscription-daily-remaining-sort-bulk-reset-design-cn.md)；本功能复用其提交后缓存失效路径。子管理员权限边界见 [子管理员角色设计](sub-admin-role-design-cn.md)。

阅读顺序建议：
- 改规则或策略：先读 §1、§5，再读 §11；
- 改接口或事务：读 §7、§8；
- 改前端：读 §4、§9；
- 定位代码：读 §10；跑测试或上线核对：读 §14、§15。

## 1. 产品规则

| 项目 | 规则 |
| --- | --- |
| 按钮位置 | 用户订阅卡片"续费"按钮后增加 `重置（N）`，N 为剩余次数 |
| 默认上限 | 每天 1 次；按组织统一配置，不提供单个用户覆盖 |
| 计次单位 | 每个订阅独立计次；同一用户的订阅 A 用完不影响订阅 B |
| 重置范围 | 只清被点击订阅的当日已用额度；不改日限金额、周/月用量、余额、用量历史或到期时间 |
| 次数刷新 | 与订阅日额度使用相同的服务端配置时区，每天 0 点恢复；不累积前一天剩余次数 |
| 次数用尽 | 显示 `重置（0）` 并禁用，展示下次恢复时间 |
| 管理端重置 | 继续走现有管理接口，不受自助次数限制，不扣减也不补回自助次数 |
| 管理员身份 | 管理员在用户页使用自助接口同样计次；不限次的是管理端重置入口 |
| 策略配置权限 | 只有完整管理员；子管理员的订阅重置权限不带配置权限 |
| 一次性日卡 | 不开放自助重置，不提供开关；管理员仍可重置；长期订阅正常适用 |
| 开放范围 | 灰度开关 `off` / `admin` / `all`（§5） |

一次性日卡的决定（2026-09-23 用户授权）：实际业务通常主动续约十年，长期订阅正常适用；日卡排除是产品取舍，不定性为已发生的漏洞。

## 2. 计次选型

每订阅独立计次：组织配置 N 表示该组织每个用户的每份订阅每天各有 N 次，次数不能在订阅间转移。用户在审核期间询问过"按用户共享"的利弊，这不等于改选；只有用户明确改选后才更换计数主键。

不为假设中的切换预留冗余 `user_id` 列：订阅本身已有用户归属；将来改共享仍须合并同一用户的多条计数并确定当天次数折算规则，同一用户多个订阅直接加 `UNIQUE(user_id)` 会冲突，冗余列消除不了这次数据迁移。

## 3. 依赖的现有实现

| 现有入口 | 行为及对本功能的约束 |
| --- | --- |
| `frontend/src/views/user/SubscriptionsView.vue` | 卡片和续费按钮在页面中；页面独立调用 `getMySubscriptions()`，不是由全局订阅 store 驱动 |
| `frontend/src/stores/subscriptions.ts` | 全局订阅状态缓存 60 秒、轮询 5 分钟；`invalidateCache()` 只清时间戳，必须调用 `fetchActiveSubscriptions(true)` 才会刷新 |
| `backend/internal/service/user_subscription.go` | 普通日额度按 `timezone.StartOfDay()` 与 `AddDate(0,0,1)` 对齐自然日，不是从点击时刻起 24 小时 |
| `backend/internal/pkg/timezone/timezone.go` | 计次日期、日窗口和次日恢复时刻统一用服务端配置时区，不由浏览器或数据库会话时区决定 |
| `SubscriptionService.AdminResetQuota` | 缓存失效发生在方法内部且缺少跨实例发布，不能包进扣次事务，也不能照抄其失效流程 |
| `user_subscription_repo.go` 的 `ResetUsageWindows` | 经 `clientFromContext` 使用事务 client，可复用 `daily=true, weekly=false, monthly=false` 的底层写入 |
| `user_subscription_repo.go` 的 `GetByIDForUpdate` | 只按订阅 ID 加锁，无 `user_id` 条件、不加载用户/分组；自助接口不能复用 |
| `SubscriptionService.invalidateFilteredDailyResetCaches` | 批量日重置的完整提交后失效：本机同步、billing cache、跨实例发布；自助接口复用 |
| `idempotency.go` / `idempotency_repo.go` | AtomicSuccess：业务写入和幂等成功结果同事务提交，支持提交后回调与"提交成功但确认丢失"的只读恢复 |
| `handler/idempotency_helper.go` | 通用用户幂等 helper 不用 AtomicSuccess，coordinator 缺失时直接执行；自助接口不能照搬 |
| `organization_usage_repo.go` / `organization_usage_service.go` | 组织来自邮箱域名：`xunyou.com` → `xunyou`，`wsdashi.com` → `wsdashi`，其余 `other`；常量 `OrganizationXunyou/Wsdashi/Other` 可复用。同文件的 `organizationUsageLocation`（固定上海）只属于报表，禁止借用 |
| `server/middleware/backend_mode_guard.go` | 后端模式关闭时放行；开启时拦截所有非完整管理员（含子管理员）。用户接口沿用 |

组织口径不依赖部门功能，也不把组织当成 API 分组；将来有独立组织实体时只替换组织解析来源。

## 4. 用户交互与边界

| 状态 | 按钮/反馈 | 是否扣次 |
| --- | --- | --- |
| 有日限、订阅有效、有已用日额度、次数剩余 ≥1 | `重置（N）` 可点击 | 确认并成功后扣 1 |
| 点击按钮 | 确认框说明"清空此订阅今天的已用额度，消耗 1 次；周/月额度不变" | 尚不扣 |
| 请求进行中 | 显示处理中、禁用重复提交，保留同一操作的幂等键 | 由后端决定 |
| 重置成功 | 刷新卡片、次数状态与全局 store；刷新前若又发生计费，日用量可能已大于 0 | 扣 1 |
| 用尽 | `重置（0）` 置灰，提示下次恢复时间 | 不允许 |
| 当前有效日用量为 0 | 显示剩余次数但禁用，提示"当前无需重置"；服务端也不执行空重置 | 不扣 |
| 组织上限为 0 | `重置（0）` 置灰，提示"组织未开放自助重置" | 不扣 |
| 无日限、订阅过期/暂停/撤销、分组停用、一次性日卡 | 按钮禁用并显示原因，服务端拒绝 | 不扣 |
| 灰度未对当前账号开放（`ROLLOUT_DISABLED`） | 不显示按钮和提示行 | 不扣 |
| 状态未加载成功 | 不显示按钮；读取失败只显示可重试的错误横幅 | 不扣 |
| 失败或超时 | 保留重试；结果未知时用原幂等键重试，不能换键再执行 | 以服务端提交结果为准 |

- 不必等日额度耗尽，有效日用量大于 0 即可使用一次。
- 管理员刚清零后，用户剩余次数不变；日用量为 0 时不扣次。
- 重置日用量不能解除已触达的周/月限额，确认框必须写明。
- 一次性日卡复用 `HasOneTimeDailyQuota()`，看订阅当前 `starts_at/expires_at` 的日历跨度，不看剩余时长；十年订阅进入最后一天不会被误判为日卡。续约后按当前有效期重新判定，同一天已用次数不清零。
- 到达服务端返回的次日边界时重新请求状态，页面重新可见时也刷新；前端不能只靠本地倒计时把按钮改成可用。页面用 `next_reset_at` 与 `server_now` 的差安排下一次读取。
- 前一天未物理清零的旧用量按日窗口规则视为 0（`canAutomaticallyResetDailyAt`），不能当作今天的重置目标。
- `can_reset` 只认 status 接口；列表里的金额只做展示，不参与第二套可用性推断。POST 在事务内重新判定，status 可用不承诺 POST 一定成功。
- 按钮留在续费后，用换行/间距处理窄屏、长名称和英文。

## 5. 组织策略与灰度开关

配置入口在管理端订阅页工具栏的"自助重置设置"弹窗（`SubscriptionSelfResetPolicyDialog.vue`，仅完整管理员可见）：开放范围一项、迅游/速宝/其他三行次数，列标题为"每个订阅每日可重置次数"。

settings 独立 key：`subscription_self_daily_reset_policy`（`SubscriptionSelfResetPolicyKey`）。

```json
{
  "rollout": "admin",
  "daily_limit_by_organization": { "xunyou": 1, "wsdashi": 1, "other": 1 }
}
```

| 规则 | 说明 |
| --- | --- |
| 初始值 | migration 241 初始化：三类组织均为 1，`rollout=admin` |
| 灰度开关 `rollout` | `off`：所有人关闭（fail-closed）；`admin`：只有角色为完整 `admin` 的账号可用；`all`：全部用户。缺少该字段按 `admin` 处理。子管理员不因订阅管理权限获得自助入口。上线先用 `admin`，验证后再切 `all` |
| 校验（`ParseSubscriptionSelfResetPolicy`） | 必须是单个 JSON 对象；三个组织码必须齐全；次数只接收 0–100 整数；未知字段、未知组织码、缺项、负数、小数拒绝（`400 INVALID_SELF_RESET_POLICY`）。`1.0`、`1e0` 同样被拒绝（Go 解码到 int 报错），前端 `v-model.number` 提交 `1` 不受影响 |
| 次数为 0 | 关闭对应组织的自助能力；管理端重置照常 |
| 当天调高 | 当天已用 1 次，上限从 1 改为 3，剩余立即为 2 |
| 当天调低 | 当天已用 2 次，上限从 3 改为 1，剩余为 0，已用次数仍记为 2 |
| 改组织/邮箱 | 后续请求按新组织上限减去当天已用次数，不重新赠送一套次数 |
| 续费/恢复 | 同一订阅同一天保留已用次数 |
| 生效与缓存 | 每次请求从数据库读策略；不加 Redis 或 `SettingService` 进程内缓存。在途请求按已读取的快照完成 |
| 读取异常 | 缺失、损坏或数据库错误 → `503 SELF_RESET_UNAVAILABLE`，不静默放宽 |
| 弹窗读取失败 | 显示空白输入框（不填默认值），三项填好后可保存覆盖，也可重试读取 |

组织解析用纯函数 `ResolveSubscriptionOrganization`，复用 `OrganizationXunyou/Wsdashi/Other`，与 SQL `LOWER(SPLIT_PART(email,'@',2))` 保持一致：大小写不敏感、子域名不匹配主域名、取第二段（多个 @ 时不取最后一段）、空域名和尾随 @ 归 `other`。`other` 是其余邮箱集合，不等于外部付费用户，默认值保持 1。

**只复用组织码和分类语义，不复用组织报表的时间函数。** 自助计次一律用 `timezone.Location()/StartOfDay()`，禁止用 `organizationUsageLocation` 或报表日期归一化函数，也不为本功能改报表时区。

注册邮箱验证可以关闭，组织按邮箱域名判定意味着关闭验证后用户可自报域名；这是运维约束，不改代码（见 `llm-wiki/wiki/security-and-reliability.md`）。

## 6. 数据模型与迁移

### 6.1 计数表（migration 241）

`backend/migrations/241_subscription_self_daily_reset.sql`：新建 `subscription_self_daily_reset_usage` 并初始化 §5 的 settings。不向 `users`、`user_subscriptions` 增加字段。编号避开当时部门分支已用的 239/240。

| 字段 | 类型与约束 | 用途 |
| --- | --- | --- |
| `subscription_id` | bigint，主键，外键 `user_subscriptions(id) ON DELETE CASCADE` | 每个订阅最多一行 |
| `quota_date` | date，非空 | 这行计数归属的服务端自然日 |
| `used_count` | integer，非空，`>= 0` | 该日成功自助重置次数 |
| `updated_at` | timestamptz，非空 | 最近变化时间 |

公式：`今日已用 = quota_date == 今天 ? used_count : 0`；`剩余 = max(组织上限 - 今日已用, 0)`。跨日 GET 只按公式读，不写表；当天第一次成功 POST 才把日期改成今天并从 1 开始。无需凌晨清表或调度任务。

日期约定贯穿读写：取锁后的同一个 `now` 产生 `timezone.StartOfDay(now).Format("2006-01-02")`，以字符串绑定（写入 `$2::date`），读回用 `to_char(quota_date, 'YYYY-MM-DD')` 按日历值比较。禁止 `CURRENT_DATE`、`NOW()::date`，禁止先把计数日期转成 `timestamptz`/时间点再取 date。

计数行首次写入用 `INSERT ... ON CONFLICT (subscription_id) DO UPDATE`。并发安全来自"先锁订阅父行、再检查上限、再写计数"，主键或 UPSERT 本身防不了超次。

表通过独立 SQL repository 访问（`subscription_self_reset_repo.go`），使用 AtomicSuccess 的事务 client，不为这张内部表扩展 Ent 对象和 DTO。改用 Ent schema 时必须重新生成 Ent/Wire，并保持同样的日历值约定。

### 6.2 历史表（migration 242）

计数表只保存一个记录日，回答不了"每天谁重置了多少次"。`backend/migrations/242_subscription_self_reset_events.sql` 新增只追加的 `subscription_self_reset_events`：`subscription_id, user_id, group_id, organization, quota_date, used_count, daily_limit, daily_usage_usd_before, created_at`，索引 `quota_date` 与 `(user_id, id DESC)`。

- 与计数行在同一事务写入（`RecordEvent`）：成功响应必有历史行；幂等重放不重新执行，不会多写。
- 不建外键：订阅、用户或分组删除后历史仍保留。
- 242 之前的重置没有历史；对账查询要按 242 的 `applied_at` 过滤（§15）。

## 7. 接口合同

| 接口 | 权限 | 职责 |
| --- | --- | --- |
| `GET /api/v1/subscriptions/self-reset-status` | 登录用户 | 一次返回本人全部订阅的自助状态 |
| `POST /api/v1/subscriptions/:id/reset-daily` | 登录用户且拥有该订阅 | 只重置该订阅日用量，扣一次 |
| `GET /api/v1/admin/subscriptions/self-reset-policy` | 完整管理员 | 读策略 |
| `PUT /api/v1/admin/subscriptions/self-reset-policy` | 完整管理员 | 校验并整体保存策略，走管理审计 |
| `GET /api/v1/admin/subscriptions/self-reset-events` | 完整管理员 | 分页查询历史；筛选 `email`、`organization`、`start_date`、`end_date`、`page`、`page_size`；前端页面 `/admin/subscriptions/self-reset-events` |

不改现有订阅列表和公共 `UserSubscription` DTO。用户路由沿用 JWT、`BackendModeUserGuard`、面板限流和审计。三个管理接口不在任何子管理员白名单（`TestSubAdminWriteWhitelistStaysNarrow` 断言 policy 的 GET/PUT 对所有权限码都拒绝）。

状态响应示例（时间仅为合同示例）：

```json
{
  "organization": "xunyou",
  "daily_limit": 1,
  "quota_date": "2026-09-22",
  "server_now": "2026-09-22T15:00:00+08:00",
  "next_reset_at": "2026-09-23T00:00:00+08:00",
  "subscriptions": [
    { "subscription_id": 31, "used_count": 0, "remaining_count": 1, "can_reset": true, "disabled_reason": null },
    { "subscription_id": 32, "used_count": 1, "remaining_count": 0, "can_reset": false, "disabled_reason": "DAILY_LIMIT_REACHED" }
  ]
}
```

POST 合同：
- 必须带 `Idempotency-Key`；body 只允许 `{ "quota_date": "YYYY-MM-DD" }`，日期取自 status。拒绝客户端提供 `user_id`、`organization`、用量或剩余次数（未知 JSON 字段 400）。
- 幂等 scope 为 `SubscriptionSelfResetIdempotencyScope(userID)`（`subscriptions.self_reset.user:{id}`），指纹包含具体订阅 ID 和提交的 `quota_date`；`c.FullPath()` 里的 `:id` 不能代替具体 ID。
- `next_reset_at` 固定为 `timezone.StartOfDay(now).AddDate(0, 0, 1)`，不写死上海、不用 now+24h；多实例必须配置相同时区。
- 成功响应只含 `subscription_id, quota_date, daily_limit, used_count, remaining_count, next_reset_at`，避免把完整订阅存进幂等记录。重放结果是原操作快照，不能覆盖新的权威状态。

`disabled_reason` 是封闭集合，按下列顺序判定（`selfResetItem`），能重置时为 null；灰度不允许时所有订阅统一覆盖为 `ROLLOUT_DISABLED`。

| 顺序 | disabled_reason | 含义 | POST 错误 |
| --- | --- | --- | --- |
| – | `ROLLOUT_DISABLED` | 灰度开关未对当前账号开放 | 403 `SELF_RESET_ROLLOUT_DISABLED`（不加锁、不扣次） |
| 1 | `SUBSCRIPTION_INACTIVE` | 未生效、过期、暂停、撤销或已删除 | 409 `SELF_RESET_SUBSCRIPTION_INACTIVE` |
| 2 | `GROUP_DISABLED` | 分组停用或已软删除 | 409 `SELF_RESET_GROUP_DISABLED` |
| 3 | `ONE_TIME_QUOTA` | 一次性日卡 | 409 `SELF_RESET_ONE_TIME_QUOTA` |
| 4 | `NO_DAILY_LIMIT` | 分组无正数日限 | 409 `SELF_RESET_NO_DAILY_LIMIT` |
| 5 | `POLICY_DISABLED` | 组织上限为 0 | 403 `SELF_RESET_DISABLED` |
| 6 | `DAILY_LIMIT_REACHED` | 当天次数耗尽 | 409 `SELF_RESET_LIMIT_REACHED` |
| 7 | `NO_USAGE` | 当前有效日用量为 0 | 409 `SELF_RESET_NO_USAGE` |

其他错误：非本人、不存在或用户已停用/删除 → 404（统一响应，不泄漏他人订阅）；`quota_date` 不是今天 → 409 `SELF_RESET_DAY_CHANGED`；缺幂等键、日期格式非法、未知字段 → 400；策略/计数/幂等存储不可用或无事务 → 503。每个枚举都要有中英文文案；状态未取得或配置读取失败属于请求失败，不能伪装成默认有次数或某个禁用原因。

Gin 按 HTTP 方法分树并优先匹配静态节点，`self-reset-status` 与 `/:id/reset-daily` 的注册顺序不影响正确性（审核时用临时程序验证两种顺序 8/8 命中）；新增路由仍要跑真实路由和权限测试。

## 8. 事务与可靠性

业务重置、扣次、历史记录、幂等成功记录必须在同一个 PostgreSQL 事务内提交。不能"先扣 Redis 次数再调管理接口"，也不能"先清零、成功后再单独记次"。

执行顺序（`SubscriptionSelfResetHandler.Reset` → `SubscriptionSelfResetService.Reset`）：

1. handler 校验认证、ID、JSON、日期格式和非空幂等键；即使全局 `idempotency.observe_only=true` 也必须有键（通用 helper 在 observe_only 下会放过空键，曾是审核发现的半完成风险）。
2. 使用专属幂等入口，`AtomicSuccess=true`，**显式 `TTL: 48 * time.Hour`**（不依赖可配置的默认 TTL，也不改其他接口）。coordinator 或事务能力缺失时拒绝执行；service 入口断言 `ent.TxFromContext(ctx)` 非空（只在 `Reset` 入口断言一次）。
3. 在同一事务内、取订阅锁之前，经事务 executor 读完整策略并检查灰度开关。事务已占连接后不得再调普通 `SettingRepository.Get/GetValue`（会申请第二条连接，连接池为 1 时死锁）。
4. `GetOwnedByIDForUpdate(ctx, userID, subscriptionID)`：WHERE 同时包含认证 `user_id`、`subscription_id`、未删除，再 `FOR UPDATE`；无匹配统一 404。分组为空不返回 404，交给 `GROUP_DISABLED` 判定，使 status 与 POST 一致。锁后在同一事务加载用户和分组，用户非 active 或已删除按 404 处理。
5. 拿锁后用同一个 `now` 计算日期、组织上限和有效日用量；请求日期不是今天 → `SELF_RESET_DAY_CHANGED`。不可重置时不写任何业务数据；否则依次 `Consume`（UPSERT 计数）→ `RecordEvent`（历史）→ `ResetUsageWindows(..., true, false, false, timezone.StartOfDay(now), now)`。同一订阅的不同幂等键在父行锁上串行。
6. 业务回调返回前调用 `DeferIdempotencyPostCommit` 注册缓存失效；注册失败返回错误并回滚，不退回立即失效。coordinator 随后在同一事务保存幂等成功结果并提交。
7. 提交成功（或"提交成功但确认丢失"的只读恢复路径）后执行回调，复用 `invalidateFilteredDailyResetCaches`：`InvalidateSubCacheSync`（本机）→ `InvalidateSubscription`（billing cache）→ `PublishSubscriptionCacheInvalidation`（跨实例）。回滚不执行回调。缓存失败只告警，不把已成功的扣次说成失败。

并发计费：入账是 `daily_usage_usd = daily_usage_usd + 金额`，会等待同一行锁，不会把清零前的旧值写回；重置后才完成的入账会重新累加日用量，与管理端重置语义一致，不暂停网关、不改计费链。

成功幂等键再次请求只重放，跨日也不会再清零。尚未成功执行的旧日期请求返回日期变化错误。提交结果不确定时只经 coordinator 回读恢复，不盲目重做。coordinator 会保存业务错误的 `error_reason`，但失败状态是 `failed_retryable`，不会像成功那样重放完整业务 409，所以前端必须按原因分类：

| 结果 | 前端后续动作 |
| --- | --- |
| 网络中断、超时、结果未知的 5xx | 保留原键和原 `quota_date`，重试原操作，不换键 |
| 重试收到 401 / 408 / 429 | 不代表原操作失败，保留原键原日期；重新登录或限流解除后继续核实，遵守 `Retry-After` |
| `IDEMPOTENCY_IN_PROGRESS` / `IDEMPOTENCY_RETRY_BACKOFF` | 遵守 `Retry-After`，原键原日期重试 |
| `SELF_RESET_DAY_CHANGED` | 刷新状态，结束旧操作；用户重新确认后用新日期、新键 |
| 次数用尽、无用量、组织关闭、灰度关闭、订阅不可用 | 展示原因并停止；条件变化后的新点击用新键 |
| `IDEMPOTENCY_KEY_CONFLICT` | 不自动换键重发，刷新状态并结束本次操作 |
| 成功或成功重放 | 只当作原操作回执，刷新列表/次数/store |

观测：请求审计沿用用户路由中间件（含失败和重放，只有通过前置鉴权与 guard 的请求才会进入）；精确历史以 §6.2 的历史表为准；并发验收直接核对计数表与幂等记录。

## 9. 前端

- `frontend/src/composables/useSubscriptionSelfReset.ts` 集中管理状态和操作；首次进入、手动重试、成功回读、跨日、页面重新可见共用一个 `refresh`。
- 成功后刷新三处：页面 `getMySubscriptions()`、`self-reset-status`、全局 store `fetchActiveSubscriptions(true)`。全局 store 只在重置成功后强制刷新，切回标签页不强刷。开始重置时使旧读请求代次失效，防止重置前的在途响应覆盖新结果。提交已成功但全局刷新失败时提示"重置成功，状态刷新失败"，下一次 refresh 补刷，不提示重新清零。
- 刷新和提交期间保留上一份状态，按钮不闪成"—"；列表加载失败显示原有 `failedToLoad` 提示，与"自助重置状态暂不可用"分开。
- 幂等键在确认提交时才用 `crypto.getRandomValues` 生成（128 位；HTTP 环境下 `randomUUID` 不可用）。取消尚未提交的确认直接丢弃。
- 结果未知的键按订阅保存（内存 + 按用户隔离的 `sessionStorage`，写入失败退回内存、照常提交）；关闭弹窗、组件重挂载、刷新页面都不删除，只在成功或确定性失败时删除，日期变化后作废。再次打开同一订阅时沿用原键并提示"上一次重置结果尚未确认"；其他订阅不受影响。
- 管理端：`SubscriptionSelfResetPolicyDialog.vue`（完整管理员，按 `role === 'admin'` 显示）、`SubscriptionSelfResetEventsView.vue`（历史）。API client：`frontend/src/api/subscriptionSelfReset.ts`。

## 10. 代码地图

| 层 | 位置 |
| --- | --- |
| 迁移 | `backend/migrations/241_subscription_self_daily_reset.sql`、`242_subscription_self_reset_events.sql` |
| 服务 | `backend/internal/service/subscription_self_reset.go`：`SubscriptionSelfResetPolicyKey`、`SubscriptionSelfResetRollout*`、`SubscriptionSelfResetIdempotencyScope`、`ParseSubscriptionSelfResetPolicy`、`ResolveSubscriptionOrganization`、`selfResetRolloutAllowed`、`selfResetItem`、`SubscriptionSelfResetService.Status/Reset/Policy/SetPolicy/ListEvents` |
| 仓储 | `backend/internal/repository/subscription_self_reset_repo.go`：`ReadPolicy`、`WritePolicy`、`ListOwned`、`GetOwnedByIDForUpdate`、`GetUsage`、`Consume`、`RecordEvent`、`ListEvents` |
| Handler | `backend/internal/handler/subscription_self_reset_handler.go`：`Status`、`Reset`、`GetPolicy`、`SetPolicy`、`ListEvents` |
| 路由 | `backend/internal/server/routes/user.go`（用户两条）、`routes/admin.go` 的 `registerSubscriptionRoutes`（管理三条） |
| 复用 | `SubscriptionService.invalidateFilteredDailyResetCaches`、`UserSubscription.HasOneTimeDailyQuota`、`DeferIdempotencyPostCommit` |
| Wire | `backend/internal/service/wire.go`、`handler/wire.go`、`repository/wire.go`；`ProvidePluginManager` 保留插件 `SetAccountDirectory` 接线（与上游手改的 `wire_gen.go` 曾分叉，合并上游时检查，见 `docs/upstream-sync/merge-log.md`） |
| 前端 | `frontend/src/composables/useSubscriptionSelfReset.ts`、`api/subscriptionSelfReset.ts`、`views/user/SubscriptionsView.vue`、`views/admin/SubscriptionsView.vue`、`views/admin/SubscriptionSelfResetEventsView.vue`、`components/admin/subscription/SubscriptionSelfResetPolicyDialog.vue`、`i18n/locales/{zh,en}/misc.ts` |

## 11. 关键约束与易错点

1. **锁查询必须带用户归属条件。** 现成 `GetByIDForUpdate` 只按 ID 加锁；用它会先锁住他人的订阅行再补鉴权。规则：WHERE 同时含 `user_id`、`subscription_id`、未删除，然后 `FOR UPDATE`。
2. **提交后缓存失效必须是三段式，且只在提交后执行。** 单条 `AdminResetQuota` 缺少跨实例发布，照抄会让其他实例继续用旧的高用量拒绝请求。验收要由另一实例实际回读，不能只断言发布方法被调用；通知失败时靠既有 TTL 收敛，不承诺故障下立即全实例一致。
3. **计次日期与报表时区隔离。** 只用 `timezone.StartOfDay()`；不能借 `organizationUsageLocation`，不能用数据库 `CURRENT_DATE`。DATE 类型本身不必然错日，风险取决于绑定和 SQL 转换路径；驱动编码检查（pq 保留 `+08:00`）不能代替真实 PostgreSQL 验证。
4. **空幂等键必须在入口拒绝。** 全局 `observe_only` 默认开启时，通用 helper 会让空键请求绕过 coordinator 直接执行，产生"只清零不扣次"一类的半完成状态。
5. **事务内不能再借第二条连接。** 策略读取走事务 executor；连接池为 1 时普通 `GetValue` 会卡死。
6. **结果未知时绝不换键。** 曾复现的失败场景：组织上限 ≥2、原请求已提交，之后又有新用量入账；用户关闭弹窗后再次确认，旧实现生成新键，重复清零并多扣一次。默认 1/1/1 时第二次会得到 `LIMIT_REACHED`，掩盖了这个问题；"`NO_USAGE` 能兜住"的前提是中间没有新用量，而来点重置的用户恰好在大量使用。
7. **status 与 POST 的判定必须一致。** 曾复现：分组软删除后 status 显示 `GROUP_DISABLED`，POST 却因锁查询要求分组存在而返回 404；现锁查询不再因分组为空返回 404。
8. **灰度关闭时不显示按钮。** 曾复现："仅管理员"阶段普通用户仍看到置灰的 `重置（1）` 和"暂未对当前账号开放"；页面打开期间开放范围被收回时，错误提示成"组织未开放"。现单独错误码 `SELF_RESET_ROLLOUT_DISABLED`，状态为 `ROLLOUT_DISABLED` 或未加载时不渲染按钮。
9. **`invalidateCache()` 不会刷新数据。** 全局 store 必须 `fetchActiveSubscriptions(true)`。
10. **策略不能接入 `SettingService` 的进程内缓存。** `SettingService` 对若干 key 有 `atomic.Value` 缓存，照搬会让改策略在一个缓存周期内不生效。
11. **策略接口"默认不在子管理员白名单"要有测试钉住。** 白名单是逐条枚举，新路由天然不被 `admin.subscriptions` 覆盖；正因为默认成立，容易被后来者顺手加进去。
12. **只部署前端按钮会让状态接口失败。** 发布必须同时带 migration 241/242、后端和前端。

## 12. 已否决的审核建议

- 已否决：一次性日卡"开放自助等于永久增发、单价五折"的漏洞定性，原因：源码只证明日卡不自动刷新，不能证明销售定价合同；最终按产品决定排除日卡（§1），不作为漏洞记录。
- 已否决：计数日期改用 `quota_day_start timestamptz`，原因：业务标识是自然日，DATE + 日历字符串绑定足够；TIMESTAMPTZ 也能实现，但不是改表理由。
- 已否决："time.Time 写 DATE 必然在凌晨错日"的必现结论，原因：未给出实际 SQL/绑定复现；保留风险并写死绑定方式（§6.1）。
- 已否决："持锁后再读组织上限"应倒过来、且与"不读缓存"互相矛盾的定性，原因：两者可同时成立；采用事务内、取锁前读取，事务外先读也是可行方案。
- 已否决：改为按用户共享计次，原因：用户已明确选择每订阅独立；讨论共享利弊不等于改选。
- 已否决：预留冗余 `user_id` 以便将来切共享，原因：见 §2，冗余列消除不了数据迁移和当天折算。
- 已否决：`other` 默认值改 0，原因：需求是"默认每日一次"，未限定内部员工；管理员可自行配置为 0。
- 已否决：静态路由必须先于 `:id` 注册的硬性门禁，原因：Gin 优先匹配静态节点，实测两种顺序都正确；只保留为排版习惯。
- 已否决：按钮移到进度条旁，原因：与用户明确的"续费后"要求冲突，窄屏问题用布局解决。
- 已否决："计数行只能 UPSERT、先查再插必然错误"，原因：持有共同父行锁时先查再插并非必然错误；实现采用 UPSERT，但它不能替代锁和上限检查。
- 事实更正（审核原稿共同错误）：后端模式是"开启时"拦截非完整管理员，不是"关闭时"。

## 13. 验收清单

| 场景 | 必须满足 |
| --- | --- |
| 默认 1 次 | 成功后日用量归零、次数 1→0、按钮置灰；周/月用量和到期时间逐字段不变 |
| 多订阅 | A 用完不影响 B，组织上限 N 对每个订阅分别生效 |
| 多设备并发 | 同一订阅并发提交不同键，成功次数不超过上限；相同键只执行一次 |
| 故障原子性 | 在扣次、历史、清零、幂等成功写入和提交阶段注入故障，不留下只扣次或只清零的状态 |
| 空键与无事务 | `observe_only=true` 且无键仍 400；coordinator/事务/提交后回调缺失时不写业务数据 |
| 归属锁定 | 锁查询含 `user_id + subscription_id`；他人订阅统一 404，其用量/次数不变；另一用户用同一个键能正常执行，不会被重放 |
| 单连接池 | `MaxOpenConns=1` 下仍能完成策略读取、锁定、扣次与幂等提交 |
| 跨日 | 23:59→00:00 自动恢复，旧确认不扣新日次数，旧成功键重放不再清零；不同浏览器时区结果一致 |
| 日期绑定 | 真实 PostgreSQL 用 UTC 会话、业务用上海时区，覆盖 23:59/00:30/08:30，DATE 回读始终为业务日期 |
| 时区隔离 | 业务时区设为 UTC 及有夏令时的时区，组织报表仍固定上海；次数/日窗口/`next_reset_at` 跟随业务配置并用 `AddDate` |
| 跨实例失效 | 两实例预热旧缓存后重置：回滚不失效；成功后本机、billing 和另一实例都回读到 0 |
| 重试与 TTL | 默认 TTL 改短时本接口仍 48h；网络中断、处理中、退避、指纹冲突、业务拒绝分别验证 |
| 组织策略 | 0/1/N、当天升降限、邮箱变更、多实例按约定生效；缺失配置 fail-closed |
| 灰度 | `off` 全拒；`admin` 下普通用户状态全为 `ROLLOUT_DISABLED`、POST 403 且不加锁不扣次；`all` 全放开 |
| 权限 | 所有子管理员权限组合都不能读写策略和历史；普通用户/子管理员/完整管理员 × 后端模式开关两态 |
| 管理端 | 用户次数耗尽后管理员仍能多次重置，且不回补用户次数 |
| 特殊订阅 | 未生效、过期、暂停、撤销、分组停用、无日限、日卡均拒绝且不扣次；十年订阅最后一天不误判为日卡 |
| 前端 | 状态前禁用、确认期间防重、三处刷新、旧在途响应不能恢复次数；成功但刷新失败只重试读取；窄屏/长名称/英文布局正常 |

## 14. 验证

在 `backend/` 下：

```bash
go test -tags=unit -p 1 -count=1 ./internal/service ./internal/handler ./internal/handler/admin ./internal/server/middleware ./internal/server/routes -run 'SubscriptionSelfReset|AdminPermission|CanAccessAdmin|SubAdmin|AdminAuth|BackendModeUserGuard|SubscriptionBulkActionRoutes'
# 真实 PostgreSQL：DSN 只能指向新建的空库（会执行全部迁移）
SUB2API_POSTGRES_ONLY_INTEGRATION_DSN='postgres://...' \
  go test -tags=integration -p 1 -count=1 ./internal/repository -run SubscriptionSelfReset -v
```

集成测试必须看到 `TestSubscriptionSelfResetIntegration`、`TestSubscriptionSelfResetCrossInstanceCaches`、`TestSubscriptionSelfResetOrganizationParityAndDates` 三个 `--- PASS`。没有 Docker 且未设 DSN 时 `TestMain` 打印 `docker is not available; skipping integration tests` 并返回 `ok`，这不算通过；CI 不能替代这一步。跨实例测试是同进程两个 `SubscriptionService`/`BillingCacheService` + miniredis TCP/PubSub，不代表生产 Redis 集群或网络分区验收。

在 `frontend/` 下：

```bash
pnpm exec vitest run src/views/user/__tests__/SubscriptionsView.selfReset.spec.ts src/composables/__tests__/useSubscriptionSelfReset.spec.ts src/components/admin/subscription/__tests__/SubscriptionSelfResetPolicyDialog.spec.ts src/views/admin/__tests__/SubscriptionsView.spec.ts
pnpm typecheck
```

最近结果：
- 2026-11-04（HEAD `26f6363af`，本机）：`-tags=unit -run 'SubscriptionSelfReset|TestAdminResetDailyFiltered|TestFilteredDailyResetCache|SubAdminWriteWhitelist'` 等定向测试在 service、repository、handler/admin、server/routes 通过；前端与集成未重跑。
- 2026-09-24（功能分支，本机）：Go service、handler、handler/admin、middleware、routes 专项通过，`go vet -tags=integration ./internal/repository` 通过；前端 6 个文件 50 项通过，vue-tsc、ESLint 通过。首版自审时 golangci-lint（`--new-from-rev=HEAD`）0 issues，Wire 重复生成稳定。
- 2026-09-23 至 09-24 的 PostgreSQL 集成测试在后续修复（跨用户同键、软删除分组）之后未重跑，当时只通过编译。
- 2026-11-04（main，组织负责人功能验证）：嵌入式 PostgreSQL 18.3 空库上运行包含 `SubscriptionSelfReset` 的集成筛选，75 个顶层用例通过（见 [组织负责人权限设计](organization-leader-design-cn.md) §15.3）。
- 浏览器只用实际 Vue 页面 + 隔离模拟 API 看过桌面、375px 中文、无空格长名称英文；未用真实账号提交重置。

## 15. 上线与 PostgreSQL 验证

任何一步不符合预期，就在"自助重置设置"里把开放范围改为"关闭"，立即生效，不需要回滚表或数据。

**第 1 步：迁移和配置（只读 SQL）**

```sql
SELECT filename, applied_at FROM schema_migrations
 WHERE filename IN ('241_subscription_self_daily_reset.sql', '242_subscription_self_reset_events.sql');

SELECT column_name, data_type, is_nullable FROM information_schema.columns
 WHERE table_name = 'subscription_self_daily_reset_usage' ORDER BY ordinal_position;

SELECT conname, pg_get_constraintdef(oid) FROM pg_constraint
 WHERE conrelid = 'subscription_self_daily_reset_usage'::regclass;

SELECT value FROM settings WHERE key = 'subscription_self_daily_reset_policy';
```

期望：两条迁移都在；4 列依次为 `subscription_id bigint`、`quota_date date`、`used_count integer`、`updated_at timestamptz`；约束有主键、`REFERENCES user_subscriptions(id) ON DELETE CASCADE`、`used_count >= 0`；配置为 §5 的初始值。

**第 2 步：读取接口。** 完整管理员打开"订阅管理 → 自助重置设置"，看到开放范围"仅管理员"和三项次数；完整管理员的"我的订阅"有效订阅上出现 `重置（N）`；普通用户看不到按钮和提示；服务端日志没有 `SELF_RESET_UNAVAILABLE`。

**第 3 步：管理员真实重置一次。** 准备一个完整管理员：有有效订阅、分组有日限、当天已有用量、所在组织次数不为 0。记下订阅 ID `:sid` 和用户 ID `:uid`。

```sql
-- 重置前
SELECT daily_usage_usd, weekly_usage_usd, monthly_usage_usd, daily_window_start, expires_at
  FROM user_subscriptions WHERE id = :sid;
-- 页面点重置后：quota_date 为服务端时区今天，used_count = 1
SELECT subscription_id, quota_date, used_count, updated_at
  FROM subscription_self_daily_reset_usage WHERE subscription_id = :sid;
-- 日用量为 0，daily_window_start 为今天 00:00；周/月用量和到期时间不变（复跑重置前那条）
-- 幂等记录：最新一条 succeeded、200，TTL 48 小时
SELECT status, response_status, error_reason, expires_at - created_at AS ttl
  FROM idempotency_records WHERE scope = 'subscriptions.self_reset.user:' || :uid
 ORDER BY created_at DESC LIMIT 5;
```

服务端时区为 Asia/Shanghai 时，在北京时间 0–8 点做这一步可顺带确认 `quota_date` 是北京日期而非 UTC 日期。

**第 4 步：缓存失效。** 重置前让日用量达到限额，用该账号 API Key 调模型应被日额度拦截；重置后立刻再调应直接成功。多实例部署时连续调几次（落到不同实例）都应成功。

**第 5 步：重放和并发。** 复制刚才 POST 的 `Authorization` 和 `Idempotency-Key` 原样重发，期望 200、响应头 `X-Idempotency-Replayed: true`，`used_count` 仍为 1。并发：把该组织次数临时改为 2，再产生一点用量，同时发 5 个不同键的请求，期望恰好 1 个 200、4 个 409 `SELF_RESET_LIMIT_REACHED`，`used_count = 2`；做完把次数改回。

```bash
curl -i -X POST "$BASE/api/v1/subscriptions/$SID/reset-daily" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -H "Idempotency-Key: $KEY" -d "{\"quota_date\":\"$DATE\"}"
```

**第 6 步：开关。** 改为"关闭"：管理员页面按钮消失，直接调 POST 返回 403 `SELF_RESET_ROLLOUT_DISABLED`；改回"仅管理员"。以上都通过后改为"全部用户"。

**放开后的巡检**

```sql
-- 每天使用情况（历史表；计数表每个订阅只保留最后一天）
SELECT quota_date, organization, count(DISTINCT subscription_id) AS subscriptions, count(*) AS resets,
       sum(daily_usage_usd_before) AS usd_cleared
  FROM subscription_self_reset_events GROUP BY 1, 2 ORDER BY 1 DESC, 2 LIMIT 21;

-- 历史表与计数表应一致；有结果说明写入不同步（多为仍有实例跑旧版本）
-- 按 242 应用时间过滤，不用 CURRENT_DATE：会话时区可能不是服务时区，且 242 之前没有历史
SELECT u.subscription_id, u.quota_date, u.used_count, count(e.id) AS events
  FROM subscription_self_daily_reset_usage u
  LEFT JOIN subscription_self_reset_events e ON e.subscription_id = u.subscription_id AND e.quota_date = u.quota_date
 WHERE u.updated_at >= (SELECT applied_at FROM schema_migrations WHERE filename = '242_subscription_self_reset_events.sql')
 GROUP BY 1, 2, 3 HAVING u.used_count <> count(e.id);

-- 近一天失败原因：NO_USAGE、LIMIT_REACHED 属正常；出现 SELF_RESET_UNAVAILABLE 要排查
SELECT status, error_reason, count(*) FROM idempotency_records
 WHERE scope LIKE 'subscriptions.self_reset.%' AND created_at > NOW() - INTERVAL '1 day'
 GROUP BY 1, 2;
```

## 16. 状态与待办

- [ ] 生产按 §15 核对并记录结果（灰度开关当前取值、第 3–5 步）。
- [ ] 真实账号浏览器验收（当前只有隔离模拟 API 的页面观察）。
- [ ] 生产 Redis 集群 / 多进程部署下的跨实例失效验收（当前只有同进程 miniredis）。
- 已解决：实现审核提到"分配接口不检查分组是否启用"，现 `SubscriptionService.assignSubscriptionWithReuse` 对停用分组返回 `GROUP_NOT_ACTIVE`。
- 已解决：实现审核的两项前端低优先级问题（提交失败后卡片停在"重置（—）"、策略损坏时弹窗无法覆盖）已在 2026-09-23/24 修复（§9、§5）。
- 不做：自助重置的单用户次数覆盖、一次性日卡开关、权益扩展；需要时另行讨论，不能借技术实现扩大范围。
