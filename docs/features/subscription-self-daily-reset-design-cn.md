# 用户订阅自助重置日额度方案

更新日期：2026-09-23。版本：**v2.2 实施口径已落地，未提交、未部署**；产品待定项已关闭。验证结果和边界见 `docs/features/subscription-self-daily-reset-implementation-cn.md`。

2026-09-23 用户授权决定日卡范围，并说明实际通常主动续约十年：首版排除一次性日卡自助重置，不增加配置开关；长期订阅正常适用，管理端重置不受此限制。

依据当前 `feature/hy/10208_sub_admin_assign_subscription` 工作区（基于 `main@d120d499e`）实现。该分支上一项“子管理员分配订阅”改动仍保留；本地实现不表示线上自助重置已开放。

本版已合入两份审核、[审核裁决](../reviews/subscription-self-daily-reset-review-decisions-cn.md) 的技术结论及后续复核反馈。本文为当前功能设计入口；审核原文统一归档至 `docs/reviews/`，其中早期建议和待定项不覆盖本文最终口径。以下三项列为实施必检：**带用户归属条件的锁定查询、批量路径的三段式缓存失效、计次与报表时区隔离**。定性按实际影响，不把未经运行验证的风险写成已发生漏洞。

## 1. 已确认的产品口径

| 项目 | 方案 |
| --- | --- |
| 按钮位置 | 用户订阅卡片“续费”按钮后增加 `重置（1）`，括号内为剩余次数 |
| 默认上限 | 每天 1 次；组织内统一配置上限，不增加单个用户覆盖配置 |
| 计次单位 | 每个订阅独立计次；同一用户的订阅 A 用完，不影响订阅 B |
| 重置范围 | 只重置被点击订阅的日用量，不改日限金额、周/月用量、余额、用量历史或到期时间 |
| 次数刷新 | 跟普通订阅日额度使用相同服务端时区，每天 0 点获得新次数；不累积昨天剩余次数 |
| 次数用尽 | 显示 `重置（0）` 并禁用；展示下次机会恢复时间 |
| 管理端重置 | 继续走现有管理接口，不受自助次数限制，不扣减或补回自助次数 |
| 管理员身份 | 管理员在用户页面使用自助接口时也按自助规则计次；不限次数的是已授权的管理端重置入口 |
| 组织配置权限 | 完整管理员可配置；子管理员原有订阅重置权限不自动获得配置权限 |
| 一次性日卡 | 首版排除自助重置，不增加开关；管理员仍可重置，长期订阅正常适用 |

## 2. 计次选型

已选定每订阅独立计次。组织配置 N 表示该组织每个用户的每份订阅每天各有 N 次，不能把某订阅的次数转给另一订阅。共享方案只保留为历史讨论，不进入第一版接口、表结构或运行时开关。

不为假设中的切换添加冗余 user_id：原订阅已有用户归属；将来切换共享仍须合并同一用户的多条计数并确定当天次数折算规则，不能靠新增唯一索引消除迁移。完整选型取舍见裁决清单。

## 3. 已核实的现有实现

| 现有入口 | 已核实行为及设计影响 |
| --- | --- |
| `frontend/src/views/user/SubscriptionsView.vue` | 卡片和续费按钮直接在页面中；页面独立调用 `getMySubscriptions()`，并非直接由全局订阅 store 驱动 |
| `frontend/src/stores/subscriptions.ts` | 另有全局订阅状态，缓存 60 秒、轮询 5 分钟；自助成功后需同步使其失效并刷新，避免其他区域仍显示旧额度 |
| `backend/internal/service/user_subscription.go` | 普通日额度按 `timezone.StartOfDay()` 和 `AddDate(0,0,1)` 对齐自然日；不是从点击时刻再等 24 小时 |
| `backend/internal/pkg/timezone/timezone.go` | 计次日期、日窗口及次日机会恢复统一复用服务端配置时区，不能由浏览器或数据库会话时区决定 |
| `SubscriptionService.AdminResetQuota` | 日重置清零日用量并把日窗口设为当天 0 点；其缓存失效发生在方法内，不能直接包进新的扣次事务当作原子操作 |
| `user_subscription_repo.go#ResetUsageWindows` | 已通过 `clientFromContext` 使用事务 client，可以复用 `daily=true, weekly=false, monthly=false` 的底层写入 |
| `user_subscription_repo.go#GetByIDForUpdate` | 现有方法只按订阅 ID 加锁，无 user_id 条件，也不加载用户/分组；自助接口必须新增带归属条件的锁查询，不能直接复用此方法 |
| `SubscriptionService.invalidateFilteredDailyResetCaches` | 批量日重置已同时覆盖本机同步失效、billing cache 删除、跨实例发布；自助接口复用这一完整提交后路径 |
| `idempotency.go` / `idempotency_repo.go` | 已支持 AtomicSuccess：业务写入和幂等成功结果同事务提交，并支持提交后回调及不确定提交的只读恢复 |
| `handler/idempotency_helper.go` | 现有用户幂等 helper 不使用 AtomicSuccess，coordinator 缺失时还会直接执行；新接口不能直接照搬这一入口 |
| `organization_usage_repo.go` | 当前组织来自邮箱域名：xunyou.com → xunyou，wsdashi.com → wsdashi，其余归 other；大小写不敏感，子域名不匹配主域名 |
| `organization_usage_service.go` | 已有 `OrganizationXunyou/Wsdashi/Other` 常量，可复用；同文件的 `organizationUsageLocation = time.FixedZone("Asia/Shanghai", 8*60*60)` 只属于报表，禁止借用到自助计次 |
| `repository/setting_repo.go` | 现有 settings 可保存独立 JSON 配置；普通读取直接使用默认 client，新重置事务中的权威读取应明确走事务 executor |
| `server/middleware/backend_mode_guard.go` | 后端模式关闭时放行；开启时拦截非完整管理员，包括子管理员。新用户接口沿用此规则 |

本方案以当前 main 的组织口径为基础，不依赖尚未合入本分支的部门功能，不把组织误当成 API 分组。未来有独立组织实体时，只替换组织解析来源。

## 4. 用户交互与边界

| 状态 | 按钮/反馈 | 是否扣次数 |
| --- | --- | --- |
| 有日限、订阅有效、有已用日额度且次数剩余 1 | 续费后显示 `重置（1）`，可点击 | 确认并成功后扣 1 |
| 点击按钮 | 确认框说明“清空此订阅今天的已用额度，消耗 1 次；周/月额度不变” | 尚不扣 |
| 请求进行中 | 显示处理中并禁用重复提交，保留同一次操作的幂等键 | 由后端决定 |
| 重置成功 | 清零结果提交、次数减 1；刷新卡片、次数状态与全局 store。刷新前若又发生计费，日用量可能已大于 0 | 扣 1 |
| 用尽 | `重置（0）` 置灰，提示下次恢复时间 | 不再允许 |
| 当前有效日用量已经为 0 | 按钮保留剩余次数但禁用，提示“当前无需重置”；服务端同样不执行空重置 | 不扣 |
| 组织上限配置为 0 | `重置（0）` 置灰，提示“组织未开放自助重置” | 不扣 |
| 无日限、订阅过期/暂停/撤销、分组停用 | 不提供可用的重置操作，服务端拒绝 | 不扣 |
| 状态加载失败 | 禁用按钮并提供重试，不能自行假定还有 1 次 | 不扣 |
| 失败或超时 | 保留重试机会；未知结果使用原幂等键重试，不能立即换键再执行 | 以服务端提交结果为准 |

- 无须等日额度完全耗尽，只要有效日用量大于 0，即可使用一次机会。
- 管理员刚清零后，用户的自助剩余次数保持原值；日用量为 0 时不做无意义扣次。稍后再次产生用量且有剩余次数时可用。
- 重置日用量不能解除已触达的周/月限额，确认框必须明确这一点。
- 一次性日卡首版不允许自助重置：复用 `HasOneTimeDailyQuota()` 判定，status 返回 `can_reset=false, disabled_reason=ONE_TIME_QUOTA`，POST 返回 409 + `SELF_RESET_ONE_TIME_QUOTA`，日用量与次数均不变。前端不提供可用的重置操作，组织次数配置不能绕过该限制，管理端仍可重置。
- 日卡判定看订阅当前 `starts_at/expires_at`，不看距离到期还剩几小时；十年订阅不会因进入到期前最后一天就被误判为日卡。续约后按当前有效期重新判定，同一天已用自助次数不因此清零。
- 到达服务端返回的次日边界时重新请求状态；页面重新可见时也刷新。前端不能仅靠本地倒计时把按钮改成可用。
- 普通跨日旧用量按现有日窗口规则视为 0，不能把昨天尚未物理清零的记录当作今天可扣次的重置目标。
- `can_reset` 只认 status 接口；页面列表中的金额用于展示，不参与第二套可用性推断。POST 在事务内重新判定，不能以此前 status 可用作为一定成功的承诺。
- 按钮保留在续费后；通过头部换行/间距处理窄屏、长名称和英文，不擅自改放进度条。

成功后的刷新包含三处：页面 `getMySubscriptions()`、`self-reset-status`、全局 store 的 `fetchActiveSubscriptions(true)`。`invalidateCache()` 只清时间戳，单独调用不够。开始重置时使旧读请求代次失效，避免重置前的在途响应覆盖新结果；提交已成功但刷新失败时，显示“重置成功，状态刷新失败”，禁用自助按钮并允许重试读取，不提示重新清零。

前端操作由 `useSubscriptionSelfReset` 集中管理：首次进入、重试读取、成功回读、跨日和页面重新可见均使用同一组刷新逻辑。确认提交时才用 `crypto.getRandomValues` 生成 128 位随机幂等键；提交前将订阅 ID、原日期和原键写入按用户隔离的 `sessionStorage`，写入失败则不发送重置。取消尚未提交的确认直接丢弃操作；已提交但结果未知的操作在组件重新挂载或同一标签页刷新后恢复，继续使用原键核实，不新建一次清零。

## 5. 组织策略

配置入口位于管理端订阅页工具栏，使用完整管理员可见的“自助重置设置”弹窗。三行配置：迅游、速宝、其他用户；列标题明确为“每个订阅每日可重置次数”。无需增加组织管理页面，也不扩展庞大的通用设置表单。

复用 settings，新建独立 key：`subscription_self_daily_reset_policy`。

```json
{
  "daily_limit_by_organization": {
    "xunyou": 1,
    "wsdashi": 1,
    "other": 1
  }
}
```

| 配置行为 | 规则 |
| --- | --- |
| 初始值 | 三类组织默认均为 1，与需求一致；迁移初始化设置项 |
| 校验 | 三个组织码必须完整；只接收 0–100 整数；未知组织码、缺项、负数、小数及未知字段拒绝保存 |
| 设置为 0 | 关闭对应组织的自助能力，管理端重置继续可用 |
| 当天从 1 改为 3 | 当天已用 1 次，则剩余立即变为 2 次 |
| 当天从 3 改为 1 | 当天已用 2 次，则剩余为 0，已用次数仍记为 2，不回退也不追加扣费 |
| 改组织/邮箱 | 后续请求重新解析当前组织，按新上限减去原有当天已用次数；不因组织变化重新赠送一套次数 |
| 同一订阅续费/恢复 | 同一天保留已用次数，不额外补机会 |
| 生效与缓存 | 写接口每次由事务 executor 读取数据库当前策略；不增加 Redis 或 SettingService 进程内策略缓存。新请求使用新策略，在途请求按其已读取快照完成 |
| 读取异常 | 缺失/损坏配置或数据库错误时拒绝自助重置并提示重试，不静默放宽上限 |

组织解析使用 `ResolveSubscriptionOrganization` 纯函数，并复用现有 `OrganizationXunyou/Wsdashi/Other` 常量。与 SQL `LOWER(SPLIT_PART(email,'@',2))` 做一致性测试，覆盖大小写、子域名、多个 @、空域名和尾随 @；不能改用最后一个 @ 导致口径漂移。other 是其余邮箱集合，不等同于外部付费用户，默认值继续为 1。

**只复用组织码和分类语义，不复用组织报表的时间函数。** 自助计次一律使用 `timezone.Location()/StartOfDay()`；禁止使用 `organizationUsageLocation` 或报表日期归一化函数，也不为这个功能改报表原有时区。

## 6. 数据模型：一张小附属表

当前独立计次方案新建 `subscription_self_daily_reset_usage`，不向 users 或 user_subscriptions 增加计次字段。

| 字段 | 类型与约束 | 用途 |
| --- | --- | --- |
| subscription_id | bigint，主键、外键关联 user_subscriptions.id | 每个订阅最多保存一行 |
| quota_date | date，非空；以 YYYY-MM-DD 日历字符串绑定 | 这行计数归属的服务端自然日 |
| used_count | integer，非空且 >= 0 | 该自然日成功自助重置次数 |
| updated_at | timestamptz，非空 | 最近变化时间 |

计算公式：`今日已用 = quota_date == 今天 ? used_count : 0`；`剩余 = max(组织上限 - 今日已用, 0)`。

跨日 GET 只按公式读取，不写表；当天第一次成功 POST 才把日期改成今天并从 1 开始。无需凌晨批量清表、调度任务或每天新增记录。并发安全依赖先锁同一订阅父行、再检查上限并写计数；主键或 UPSERT 本身不能防止超次。计数行首次写入采用 `INSERT ... ON CONFLICT (subscription_id) DO UPDATE`。硬删除订阅时可级联删除附属行，软删除时仍以订阅归属及状态校验为准。

日期约定必须贯穿读写：取锁后的同一个 now 产生 `timezone.StartOfDay(now).Format("2006-01-02")`，以字符串绑定 DATE；读回按相同日历格式比较，不先转时区再取日期。允许对该字符串显式 `::date`；禁止使用 `CURRENT_DATE`、`NOW()::date`，或把计数日期先经 `::timestamptz`/时间点转换后再取 date。审计字段 updated_at 仍可正常使用时间戳。

DATE 类型本身不代表必然发生错日；风险取决于具体绑定与 SQL 转换路径。即使未来采用 Ent schema，仍需保持相同日历值约定，并用真实 PostgreSQL 验证，不把驱动时间编码检查当成数据库转换结论。

表由新增 SQL migration 管理，通过独立 SQL repository 访问，使用与现有 idempotency repository 相同的事务 executor 接入方式，避免为这张内部计数表扩展核心 Ent 对象及所有 DTO。若实施时选择 Ent schema，则必须按项目规则重新生成 Ent/Wire，不能只改生成结果。

附属表仅保存一个记录日，不提供跨日历史账本；不存冗余 user_id，也不自动随订阅续费、恢复或管理员清零而重置计数。

## 7. 接口设计

| 接口 | 权限 | 职责 |
| --- | --- | --- |
| `GET /api/v1/subscriptions/self-reset-status` | 登录用户 | 批量返回本人订阅的自助状态，一次请求完成所有卡片查询 |
| `POST /api/v1/subscriptions/:id/reset-daily` | 登录用户且拥有该订阅 | 仅重置当前订阅的日用量，并扣一次自助机会 |
| `GET /api/v1/admin/subscriptions/self-reset-policy` | 完整管理员 | 读取组织策略 |
| `PUT /api/v1/admin/subscriptions/self-reset-policy` | 完整管理员 | 校验并原子保存整个组织策略，沿用管理审计 |

不改变现有订阅列表数组或公共 UserSubscription DTO，新增状态接口仅由用户订阅页面使用。

用户路由沿用 JWT、BackendModeUserGuard、面板限流和审计。后端模式开启时的非管理员拒绝，与 `SELF_RESET_DISABLED` 分开提示；完整管理员经过用户自助入口仍然计次。管理策略 GET/PUT 不加入任何子管理员白名单。

Gin 当前版本按方法分树并优先静态匹配；静态路由可放在参数路由之前便于阅读，但注册顺序不作为正确性保证。新增路由仍须执行真实路由及权限测试。

状态示例（下列时间仅为契约示例，不代表已上线数据）：

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

POST 必须携带 `Idempotency-Key`，body 仅允许 `{ "quota_date": "2026-09-22" }`。用户身份取认证上下文；拒绝客户端提供 user_id、organization、daily/weekly/monthly 或剩余次数。认证用户、具体订阅 ID 和客户端已展示的日期共同参与幂等身份/指纹；`c.FullPath()` 中的 `:id` 不能代替具体 ID。

`quota_date` 来自 status；`next_reset_at` 固定为 `timezone.StartOfDay(now).AddDate(0, 0, 1)`。使用服务端配置时区，不写死上海、不使用 now＋24h；实例之间必须使用相同配置时区。

成功响应使用小型结果：`subscription_id, quota_date, daily_limit, used_count, remaining_count, next_reset_at`；页面随后重新读取订阅及次数状态。避免把完整订阅响应存入幂等记录，降低长度裁剪风险；重放结果是原操作快照，不应拿它覆盖新的权威状态。

| 错误 | 接口结果 |
| --- | --- |
| 非本人或不存在的订阅 | 404，统一响应，避免泄漏其他用户订阅信息 |
| 次数已用尽 / 日期已经变化 | 409 + `SELF_RESET_LIMIT_REACHED` / `SELF_RESET_DAY_CHANGED`，刷新状态；不写入业务变化 |
| 没有日限、无有效日用量或订阅不可用 | 明确的 409 业务错误和可展示原因；不扣次 |
| 组织上限为 0 | 403 + `SELF_RESET_DISABLED` |
| 缺少幂等键、非法日期、未知 JSON 字段 | 400 |
| 配置/计数/幂等存储不可用 | 503；不退化为无限次或非事务执行 |

`disabled_reason` 为封闭集合，按下列顺序判断；能重置时为 null。HTTP 业务错误码到展示枚举显式映射，两者不要求同名。

| 顺序 | disabled_reason | 含义 |
| --- | --- | --- |
| 1 | SUBSCRIPTION_INACTIVE | 未生效、过期、暂停或撤销 |
| 2 | GROUP_DISABLED | 分组停用 |
| 3 | ONE_TIME_QUOTA | 一次性日卡不开放自助重置 |
| 4 | NO_DAILY_LIMIT | 分组无正数日限 |
| 5 | POLICY_DISABLED | 组织上限为 0 |
| 6 | DAILY_LIMIT_REACHED | 当天自助次数耗尽 |
| 7 | NO_USAGE | 当前有效日用量为 0 |

每个枚举均需中英文文案；状态未取得、配置读取失败属于请求失败，不能伪装成默认有次数或某个业务禁用原因。

## 8. 事务与可靠性

业务重置、扣次、幂等成功记录必须在同一 PostgreSQL 事务内提交。不能采用“先扣 Redis 次数，再调用管理员接口”或“先清零，成功后再单独记次数”。

执行顺序：

1. handler 验证认证、ID、JSON、日期格式及非空幂等键。即使全局 `idempotency.observe_only=true` 也必须有键。
2. 使用专属用户幂等入口，启用 `AtomicSuccess=true`，缺少 coordinator/事务能力时拒绝执行；service 写入前断言 `ent.TxFromContext(ctx)` 非空。scope 绑定用户，payload 固定包含订阅 ID 与提交日期。**显式传 `TTL: 48 * time.Hour`**，不依赖 `DefaultWriteIdempotencyTTL()` 或可配置的默认 TTL，不改变其他用户幂等接口行为。
3. 在 AtomicSuccess 的同一事务中、取订阅锁之前，经事务 executor 读取完整组织策略。不得在事务已占连接后调用普通 `SettingRepository.Get/GetValue` 再申请第二条连接；不读进程内缓存。事务外先读数据库也是可行方案，并不与普通 GetValue 冲突，但本实现统一采用事务内读取。
4. **新增 `GetOwnedByIDForUpdate(ctx, userID, subscriptionID)` 或等价的专属锁查询**，SQL/Ent 的 WHERE 同时包含认证 user_id、subscription_id 及未删除条件，再 `FOR UPDATE`。禁止复用只含 IDEQ(id) 的现有 GetByIDForUpdate，也不能先锁别人的行再把校验所有权当成替代。无匹配统一 404。随后在同一事务内读取计数、当前用户邮箱/状态和分组日限/状态。
5. 获得写锁后，以同一个 now 计算服务端日期、组织上限和有效日用量，复查用户/订阅/分组状态及有效期。请求日期不等于当前日期则拒绝。当天已用达到上限或有效日用量为 0 时，不做业务写入；否则 UPSERT 附属计数，并复用 `ResetUsageWindows(..., true, false, false, timezone.StartOfDay(now), now)`。同订阅的不同幂等键通过父行锁串行，防止超次。
6. 在事务业务回调返回前，调用 `DeferIdempotencyPostCommit` 注册缓存失效回调；注册失败就返回错误并回滚，不退回立即失效。随后 coordinator 在同一事务保存幂等成功结果并提交；任一步失败则计数和日用量都回滚。不创建嵌套的独立 sql.Tx；新增 repository 必须使用 AtomicSuccess 的事务 client。
7. **事务提交成功后，执行已注册的回调，复用批量日重置的完整三段式失效**：`InvalidateSubCacheSync`（本机同步）→ `InvalidateSubscription`（billing cache）→ `PublishSubscriptionCacheInvalidation`（跨实例通知）。可向现有批量 helper 传入一个 SubscriptionCacheKey。**禁止参照单条 AdminResetQuota 复制失效流程，禁止在提交前失效缓存**。缓存失败告警，不把已成功扣次的操作说成失败再执行。

三段式失效是实施必检项：单条 AdminResetQuota 缺少跨实例发布，照抄会遗漏通知。缺失时其他节点可能继续使用旧用量判断额度；本功能清零日用量的典型表现是旧高用量导致继续拒绝。通知成功也需由另一实例实际回读验证，不能仅断言发布方法被调用；通知失败时依赖既有 TTL 收敛，不承诺故障下立即全实例一致。

并发计费与管理员重置继续由数据库订阅行更新串行化；在重置之后才完成入账的请求可能再次增加日用量，这与现有管理端重置语义一致，无须暂停网关或改变计费链。

策略在事务内从数据库读取，不信任 JWT、前端组织值或旧缓存。配置更新后的新请求使用新策略；已开始执行的事务按其读取到的策略快照完成，不引入全站互斥锁。

成功幂等键再次请求只重放，跨日也不能再次清零。对尚未成功执行的旧日期请求返回日期变化错误；新操作刷新日期后使用新键。提交结果不确定时只通过现有 coordinator 回读恢复，不盲目重做。

前端重试规则必须按原因区分：

| 结果 | 后续动作 |
| --- | --- |
| 网络中断、超时、结果未知的 5xx | 保留原键和原 quota_date，查询/重试原操作，不自动换键 |
| 重试收到 401 / 408 / 429 | 不代表原操作失败，保留原键原日期；重新登录或限流解除后继续核实原操作，尊重 Retry-After |
| IDEMPOTENCY_IN_PROGRESS / IDEMPOTENCY_RETRY_BACKOFF | 遵守 Retry-After，原键原日期重试 |
| SELF_RESET_DAY_CHANGED | 刷新状态，结束旧操作；用户重新确认后用新日期、新键 |
| 明确的次数用尽、无用量、组织关闭或不可用状态 | 展示原因并停止；条件改变后的新点击使用新键 |
| IDEMPOTENCY_KEY_CONFLICT | 不自动换键重发，先刷新状态并结束本次异常操作 |
| 成功或成功重放 | 只当作原操作回执，刷新当前列表/次数/store；不让重放快照覆盖权威状态 |

现有 coordinator 会保存业务错误的 error_reason，但失败状态是 failed_retryable，不会像成功一样重放完整业务 409 响应；不能将所有 409 视作同一种重试条件。日期格式在入口校验，是否今天在事务执行分支校验，避免旧成功操作跨日重放被错误拦截后又重新清零。

审计沿用用户路由中间件；请求只有经过前置鉴权和 guard 后才进入该中间件，其记录也包含失败和重放。计数表仅保存当前记录日；低维“组织＋结果”指标可辅助观察，成功仅提交后记录、重放不算新成功。指标或请求行数都不是严格历史账本，数据库并发验收直接核对计数与幂等结果。

## 9. 最小改动边界

以下为模块边界；实际文件和验证入口见交付记录。

| 层次 | 已实现边界 |
| --- | --- |
| 数据库 | `backend/migrations/241_subscription_self_daily_reset.sql`：附属计数表及独立 settings 初始值，避开部门分支 239/240；不改已有 migration |
| 后端新增模块 | `subscription_self_reset.go` service、独立 repository、独立 handler 及定向测试；包含带归属条件的锁查询、事务策略读取和组织解析 |
| 后端接线 | handler registry / Wire provider、用户与管理路由各增加小段注册；按项目要求生成 Wire |
| 现有重置能力 | 复用底层日窗口写入、配置时区、AtomicSuccess 和批量路径三段式缓存失效；不把自助次数校验塞进 AdminResetQuota，不使用无归属条件的锁方法 |
| 前端用户页 | 续费后增加按钮、确认框、一次批量状态读取与成功后刷新；中英文文案 |
| 前端管理页 | 增加独立组织策略弹窗，完整管理员可见；使用 `SubscriptionSelfResetPolicyDialog.vue`，组件目录 README 同步说明 |
| API client | 用户侧新增 status/reset 方法，管理侧新增 policy GET/PUT |
| 保持独立 | 无新定时任务；无 Redis 次数账本；无用户/订阅核心字段扩散；不改网关计费、周/月额度或现有管理员重置限制 |

## 10. 验收要求

| 场景 | 必须满足 |
| --- | --- |
| 默认 1 次 | 成功后日用量归零、次数 1→0、按钮置灰；周/月用量和到期时间逐字段不变 |
| 多订阅 | A 用完不影响 B，组织上限 N 对每个订阅分别生效 |
| 多设备并发 | 同一计数身份并发提交不同键，成功次数不超过组织上限；相同键只执行一次 |
| 故障原子性 | 在扣次、清零、幂等成功写入和提交阶段注入故障，不能留下只扣次或只清零的半完成状态 |
| 空键与无事务 | observe_only=true 且无键仍返回 400；coordinator/事务/提交后回调能力缺失不执行业务写入 |
| 归属锁定（必检） | 实际锁查询包含 user_id＋subscription_id；无法用他人 ID 锁住其订阅后再补鉴权。统一 404，且他人用量/次数不变 |
| 单连接池 | MaxOpenConns=1 仍能完成策略读取、锁定、扣次与幂等提交，不在事务内另借连接 |
| 跨日 | 23:59→00:00 自动恢复机会，旧确认不扣新日次数，旧成功键重放不再清零；不同浏览器时区一致 |
| 日期绑定 | 真 PostgreSQL 使用 UTC 会话、业务使用上海时区，覆盖 23:59/00:30/08:30，DATE 回读比较始终为业务日期；不经时间戳转换 |
| 时区隔离（必检） | 业务时区设为 UTC 及有夏令时的时区，组织报表仍固定上海；次数/日窗口/next_reset_at 跟随业务配置，在 DST 日使用 AddDate，而非报表时区或 24h 加法 |
| 跨实例失效（必检） | 两实例预热旧缓存后重置：回滚不失效；成功后本机、billing 和另一实例正确回读。通知失败告警且不重做扣次，不把发布调用次数当成完整验收 |
| 重试与 TTL | 默认 TTL 改短仍使用本接口 48h；网络中断、处理中、退避、指纹冲突、业务拒绝分别验证；旧日期过期键不能再清零 |
| 零用量 / 自然刷新 | 不浪费次数；前一天窗口中的旧用量不造成今天误扣 |
| 组织策略 | 0/1/N、当天升降限、邮箱变更和多实例均按约定生效；缺失配置失败关闭 |
| 权限 | 不能重置他人订阅；所有子管理员权限组合均不能读写策略；普通用户/子管理员/完整管理员覆盖后端模式开关两态 |
| 管理端 | 用户次数耗尽后，管理员仍能多次重置；管理员操作不回补用户次数 |
| 特殊订阅 | 未生效、过期、暂停、撤销、分组停用、无限日限拒绝；日卡 status 禁用且 POST 拒绝，两者均不扣次；十年订阅进入最后一天仍不按剩余时长误判为日卡 |
| 前端同步 | 状态前禁用、确认期间防重、三个读取刷新、旧在途响应不能恢复次数；已成功但刷新失败只重试读取；窄屏/长名称/英文的续费后按钮正常 |

落地验证包括定向 Vitest、Go service/handler 测试和真实 PostgreSQL 并发/回滚集成测试，以及类型检查、ESLint 与 `git diff --check`。已执行结果见交付记录；两个服务实例的缓存测试使用 miniredis，浏览器使用实际 Vue 页面＋隔离模拟 API。未以 mock 替代数据库并发/回滚测试，也未在业务环境提交真实重置。

**产品口径已定：每订阅独立计次、组织统一上限、默认每日 1 次、续费后按钮；用户授权决定后首版排除一次性日卡，保留管理员重置能力，不改变 other 默认值，不增加日卡开关。后续权益扩展另行讨论，不能借技术实现扩大范围。**
