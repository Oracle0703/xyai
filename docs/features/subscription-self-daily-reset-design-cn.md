# 用户订阅自助重置日额度设计

更新：2026-11-27。基础功能已合入 main。分支 `feature/hy/10241_self_reset_group_allowlist` 增加分组织勾选、版本化保存和同快照准入。验收记录见 §14，上线和回滚前置见 §15。

本文是用户订阅自助日重置的唯一文档，包含产品规则、数据模型、接口合同、事务与可靠性要求、代码位置、易错点和验证方式。两份设计审核、审核裁决清单、实现审核和交付记录已移出仓库，最后存在于提交 `26f6363af`（可用 `git show 26f6363af:docs/reviews/subscription-self-daily-reset-review-decisions-cn.md`、`git show 26f6363af:docs/features/subscription-self-daily-reset-implementation-cn.md` 等查看）；凡是对后续修改有约束的结论都已写入正文，被否决的审核建议集中在 §12。2026-11-25 扩展的初稿（把分组勾选存进策略 JSON 的方案）归档在仓库外 `E:\tmp\subscription-self-reset_archive\2026-11-25\`，其中被否决的部分同样记在 §12。

相关功能：管理端按筛选批量日重置、按每日剩余额度排序见 [订阅每日剩余排序与批量重置设计](subscription-daily-remaining-sort-bulk-reset-design-cn.md)；本功能复用其提交后缓存失效路径。分组所属组织见 [组织负责人权限设计](organization-leader-design-cn.md) §1.2（D13）。子管理员权限边界见 [子管理员角色设计](sub-admin-role-design-cn.md)。

阅读顺序建议：
- 改规则或策略：先读 §1、§5、§6.3，再读 §11；
- 改接口或事务：读 §7、§8；
- 改前端：读 §4、§9；
- 定位代码：读 §10；跑测试或上线核对：读 §14、§15。

## 1. 产品规则

| 项目 | 规则 |
| --- | --- |
| 按钮位置 | 用户订阅卡片"续费"按钮后增加 `重置（N）`，N 为剩余次数 |
| 默认上限 | 每天 1 次；按组织统一配置每日次数，不提供单个用户或单个分组独立次数覆盖 |
| 适用分组 | 管理员按组织勾选哪些订阅分组可自助重置。**用户所属组织必须等于分组归属，且该分组已在该组织下勾选**：迅游（`xunyou`）、速宝（`wsdashi`）用户对应 `/admin/groups` 中归属本组织的订阅分组，“其他（`other`）”用户对应未设置所属组织的订阅分组（§5.1）。2026-11-25 用户确认不放宽 |
| 未勾选或不匹配 | 默认方案为全部未勾选（fail-closed）；上线前可按 §15 明确选择兼容回填。未勾选、或用户组织与分组归属不一致的订阅卡片在用户端直接隐藏“重置”按钮和提示行，后端拒绝重置。要开放不一致的订阅只能在 `/admin/groups` 调整分组归属 |
| 计次单位 | 每个订阅独立计次；同一用户的订阅 A 用完不影响订阅 B |
| 重置范围 | 只清被点击订阅的当日已用额度；不改日限金额、周/月用量、余额、用量历史或到期时间 |
| 次数刷新 | 与订阅日额度使用相同的服务端配置时区，每天 0 点恢复；不累积前一天剩余次数 |
| 次数用尽 | 已勾选分组显示 `重置（0）` 并禁用，展示下次恢复时间 |
| 管理端重置 | 继续走现有管理接口，不受自助次数与分组勾选限制，不扣减也不补回自助次数 |
| 管理员身份 | 管理员在用户页使用自助接口同样受次数和分组勾选约束；不限次的是管理端重置入口 |
| 策略配置权限 | 只有完整管理员；子管理员（含全站订阅子管理员、组织/部门负责人）不带配置或候选分组查询权限 |
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
| `group_organization_scopes`（migration 263/264，`group_organization_scope_repo.go`） | `/admin/groups` 维护的订阅型分组所属组织表，仅存 `xunyou` / `wsdashi`（多对多）；未设所属组织的分组无记录；分组离开 `subscription` 类型时由触发器自动清空归属；`ReplaceGroupOrganizationScopes` 修改归属时持 `groups FOR UPDATE`，对列出的分组先删后插；扩展在它的事务末尾追加勾选清理（§8.1） |
| `groupRepository.DeleteCascade`（`group_repo.go`） | 删除分组时锁 `groups FOR UPDATE`，并在同一事务软删除该分组的全部订阅；软删除分组由准入过滤和巡检处理；分组删除后其订阅在自助接口表现为 404 或不在列表。`Reset` 不对 `groups` 加锁（§12） |
| `lockLiveGroups`（`group_repo.go`） | 按 ID 升序 `groups FOR SHARE`、只锁未删除行，与删除分组和归属替换的 `FOR UPDATE` 互斥；勾选保存使用独立的按 ID 升序 `FOR UPDATE` 协议（§8.1） |
| `server/middleware/backend_mode_guard.go` | 后端模式关闭时放行；开启时拦截所有非完整管理员（含子管理员）。用户接口沿用 |

组织口径不依赖部门功能，也不把组织当成 API 分组；分组候选集合直接复用 `/admin/groups` 的 `group_organization_scopes`，不另建第二套分组归属表；勾选状态存独立表（§6.3），它只记录“是否开放自助”，不是归属。

## 4. 用户交互与边界

| 状态 | 按钮/反馈 | 是否扣次 |
| --- | --- | --- |
| 分组已在当前组织勾选、有日限、订阅有效、有已用日额度、次数剩余 ≥1 | `重置（N）` 可点击 | 确认并成功后扣 1 |
| 点击按钮 | 确认框说明"清空此订阅今天的已用额度，消耗 1 次；周/月额度不变" | 尚不扣 |
| 请求进行中 | 显示处理中、禁用重复提交，保留同一操作的幂等键 | 由后端决定 |
| 重置成功 | 刷新卡片、次数状态与全局 store；刷新前若又发生计费，日用量可能已大于 0 | 扣 1 |
| 用尽 | `重置（0）` 置灰，提示下次恢复时间 | 不允许 |
| 当前有效日用量为 0 | 显示剩余次数但禁用，提示"当前无需重置"；服务端也不执行空重置 | 不扣 |
| 组织上限为 0（分组已勾选） | `重置（0）` 置灰，提示"组织未开放自助重置" | 不扣 |
| 无日限、订阅过期/暂停/撤销、分组停用、一次性日卡（分组已勾选） | 按钮禁用并显示原因，服务端拒绝 | 不扣 |
| 订阅分组不在当前用户组织的已勾选范围：未勾选、用户组织与分组归属不一致、分组已不再归属该组织、已软删除或已不是订阅型（`GROUP_NOT_ALLOWED`） | **直接隐藏按钮和提示行**（与灰度未开放一致）；若页面打开期间被取消勾选并点击提交，返回错误提示并刷新隐藏 | 不扣 |
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

## 5. 组织策略、分组勾选与灰度开关

配置入口在管理端订阅页工具栏的"自助重置设置"弹窗（`SubscriptionSelfResetPolicyDialog.vue`，仅完整管理员可见）：顶部为开放范围，下方按**迅游 / 速宝 / 其他**三个组织分别配置“每个订阅每日可重置次数”并勾选该组织允许自助重置的订阅分组。开放范围和次数属于策略（§5.2），分组勾选独立存储（§5.3）。普通保存以同一配置版本提交两类改动（§8.1）。

### 5.1 匹配规则与候选分组

2026-11-25 用户确认：**用户所属组织必须等于分组归属，且该分组已在该组织下勾选**，不放宽。用户所属组织来自邮箱域名；分组归属来自 `/admin/groups` 维护的 `group_organization_scopes`。

候选分组统一限定为**未软删除的订阅型分组**（`groups.deleted_at IS NULL AND groups.subscription_type = 'subscription'`，按 `sort_order ASC, id ASC` 排序）：

| 组织键 | 名称 | 候选订阅分组集合 |
| --- | --- | --- |
| `xunyou` | 迅游 | `EXISTS (SELECT 1 FROM group_organization_scopes gos WHERE gos.group_id = g.id AND gos.organization_key = 'xunyou')` |
| `wsdashi` | 速宝 | `EXISTS (SELECT 1 FROM group_organization_scopes gos WHERE gos.group_id = g.id AND gos.organization_key = 'wsdashi')` |
| `other` | 其他 | `NOT EXISTS (SELECT 1 FROM group_organization_scopes gos WHERE gos.group_id = g.id)`（即 `/admin/groups` 中未设置任何所属组织的订阅分组） |

准入判定只有一个口径，候选列表、勾选保存、`Status` 和 `Reset` 共用：`允许(分组, 组织) = 分组属于该组织的候选集 AND 勾选表存在 (分组, 组织) 行`（SQL 见 §6.3）。

- 在 `/admin/groups` 同时归属迅游和速宝的订阅分组，会同时出现在两张卡片中；勾选按 `(group_id, organization_key)` 独立保存和判断，迅游勾选不会自动给速宝开放。
- **严格按用户邮箱组织对号入座（无跨组织兜底）**：用户邮箱解析为 `xunyou` 时只看 `xunyou` 的候选集与勾选，即使持有管理员特批分配的 `wsdashi` 分组或未设归属的公共分组（在 `other` 下已勾选），也不能自助重置；其他邮箱用户在已归属迅游/速宝的分组里同样不能。购买、兑换码和完整管理员分配都不检查归属，这类订阅会持续产生。若需让迅游/速宝员工重置某公共分组，管理员须先在 `/admin/groups` 为该分组设置对应所属组织，再回“自助重置设置”在该组织下勾选；设置归属会同时改变组织负责人能管理的分组（[组织负责人权限设计](organization-leader-design-cn.md) D13）。
- “其他”在用户侧指非迅游/速宝邮箱，在分组侧指未设归属，两者只在本规则里对应。
- 候选分组包含停用（`status <> 'active'`）或无正数日限（`daily_limit_usd` 为空或 `<= 0`）的订阅分组，弹窗在分组旁显示只读提示标签（“已停用”/“无日限”），避免管理员因分组临时停用而找不到勾选项；运行时已勾选但停用或无日限的分组仍按 `GROUP_DISABLED` / `NO_DAILY_LIMIT` 拦截。
- 按量分组（`subscription_type <> 'subscription'`）由 migration 264 保证无组织归属，且不参与订阅分配与自助重置，不进入任何组织的候选集。

### 5.2 策略（开放范围与次数）

策略存于 settings 的 `subscription_self_daily_reset_policy`，JSON 保持原结构。migration 267 将独立 key `subscription_self_daily_reset_revision` 初始化为 1，以便旧版严格解码器仍可读取策略。

```json
{
  "rollout": "all",
  "daily_limit_by_organization": { "xunyou": 1, "wsdashi": 1, "other": 1 }
}
```

| 规则 | 说明 |
| --- | --- |
| 初始值 | migration 241 初始化：三类组织均为 1，`rollout=admin`；生产已改为 `all`（2026-11-25 用户确认） |
| 灰度开关 `rollout` | `off`：所有人关闭（fail-closed）；`admin`：只有角色为完整 `admin` 的账号可用；`all`：全部用户。缺少该字段按 `admin` 处理。子管理员不因订阅管理权限获得自助入口 |
| 校验（`ParseSubscriptionSelfResetPolicy`） | 必须是单个 JSON 对象；三个组织码必须齐全；次数只接收 0–100 整数；未知字段、未知组织码、缺项、负数、小数拒绝（`400 INVALID_SELF_RESET_POLICY`）。`1.0`、`1e0` 同样被拒绝（Go 解码到 int 报错），前端 `v-model.number` 提交 `1` 不受影响 |
| 次数为 0（保留已选分组） | 关闭对应组织全部已勾选分组的自助能力（已勾选分组在用户端卡片显示置灰的 `重置（0）` 与“组织未开放自助重置”；未勾选分组仍隐藏按钮）。次数设为 `0` 不删除勾选行，管理弹窗也**不清空、不禁用**已勾选分组（仅提示次数为 0 时暂不生效），调回 `> 0` 直接恢复；管理端重置照常 |
| 当天调高 / 调低 | 当天已用 1 次，上限 1→3 则剩余立即为 2；当天已用 2 次，上限 3→1 则剩余为 0、已用仍记 2 |
| 改组织/邮箱 | 后续请求按新组织判断分组是否允许，并按新组织上限减去当天已用次数，不重新赠送次数 |
| 续费/恢复 | 同一订阅同一天保留已用次数 |
| 生效与缓存 | 每次请求从数据库读策略和分组准入；不加 Redis 或 `SettingService` 进程内缓存。在途请求按已读取的快照完成 |
| 读取异常 | 策略缺失、损坏或数据库错误 → `503 SELF_RESET_UNAVAILABLE`，不静默放宽 |
| 弹窗读取失败 | 已取得版本但策略损坏或缺失时，留空次数，可单独存分组或填全三项显式修复。候选故障时仍可存策略或紧急关闭。联合配置 GET、普通保存、确认及接口紧急关闭均依赖版本行；版本行损坏不会自动撤销已开放的用户准入。此时按 §15 隔离配置写入，直接关闭现存策略。 |

### 5.3 分组勾选

勾选存独立表 `subscription_self_reset_groups`（§6.3），一行表示“该组织已勾选该分组”。

| 规则 | 说明 |
| --- | --- |
| 默认状态（fail-closed） | 默认上线方案是 migration 266 只建空表，所有用户的按钮消失，直到管理员勾选并保存；上线前若确认兼容回填，则只回填当前归属匹配且明确允许的分组行；新建分组、新设归属默认未勾选 |
| 保存方式 | 按版本保存策略及分组差量，同一版本并发保存至多一笔成功。 |
| 写入校验 | 持组锁复核所有新增勾选。任一分组不合格则整笔回滚并返回 409。取消已不存在的勾选可重复执行。 |
| 归属变化 | `/admin/groups` 把分组移出某组织，或给未设归属的分组设归属时，同一事务删除不再匹配的勾选行；移回后不恢复，需要重新勾选 |
| 类型变化 | 分组离开订阅型时由触发器删除它的全部勾选行；改回订阅型不恢复（与 migration 264 对归属的处理一致） |
| 分组删除 | 软删除不清理勾选行：准入查询和候选列表都排除已软删除分组，残留行只由巡检统计（§6.3、§15）；硬删除由外键级联删除勾选行 |
| 运行时判定 | 始终同时检查“已勾选”和“当前归属匹配”，不只查勾选表 |
| 与策略的关系 | 候选分组或勾选接口出故障时，仍然可以单独把开放范围改为 `off` |

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
- 不建外键：订阅、用户或分组删除后历史仍保留；按 `group_id` 筛选直接匹配事件行的 `e.group_id`，不新增索引迁移（按自然日低频写入，现有主键/日期索引足够）。
- 242 之前的重置没有历史；对账查询要按 242 的 `applied_at` 过滤（§15）。

### 6.3 分组勾选表（migration 266）

`backend/migrations/266_subscription_self_reset_groups.sql`（当前最大编号 265；实施时若 266 已被占用，取下一个可用文件名）：

```sql
CREATE TABLE subscription_self_reset_groups (
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    organization_key VARCHAR(20) NOT NULL CHECK (organization_key IN ('xunyou', 'wsdashi', 'other')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (group_id, organization_key)
);
```

- 行存在即已勾选，不设布尔列；不回填，初始为空。
- 同一迁移新建 `groups_clear_self_reset_on_type_change` 触发器，类型为 `AFTER UPDATE OF subscription_type`，条件为 `OLD.subscription_type = 'subscription' AND NEW.subscription_type IS DISTINCT FROM 'subscription'`。分组离开订阅型时删除全部勾选行，改回订阅型后必须重新保存。不修改 migration 264 及其函数。
- 软删除分组不进入候选集和准入查询。当前没有正常恢复分组的入口，勾选残留由巡检单独统计；是否增加软删除清理触发器属于维护优化，不作为授权正确性的前置条件。
- 原生 SQL 仓储访问，不新增 Ent schema。

准入查询（`$1` 分组 ID 数组，`$2` 用户组织），返回允许的分组 ID：

```sql
SELECT r.group_id
  FROM subscription_self_reset_groups r
  JOIN groups g ON g.id = r.group_id
 WHERE r.group_id = ANY($1) AND r.organization_key = $2
   AND g.deleted_at IS NULL AND g.subscription_type = 'subscription'
   AND CASE WHEN $2 = 'other'
            THEN NOT EXISTS (SELECT 1 FROM group_organization_scopes s WHERE s.group_id = g.id)
            ELSE EXISTS (SELECT 1 FROM group_organization_scopes s
                          WHERE s.group_id = g.id AND s.organization_key = $2) END;
```

`Status` 用当前用户全部订阅的分组 ID 调一次，`Reset` 在事务内用锁定订阅的分组 ID 调一次。选独立表而不是策略 JSON 或归属表加列的原因见 §12。

## 7. 接口合同

| 接口 | 权限 | 职责 |
| --- | --- | --- |
| `GET /api/v1/subscriptions/self-reset-status` | 登录用户 | 一次返回本人全部订阅的自助状态 |
| `POST /api/v1/subscriptions/:id/reset-daily` | 登录用户且拥有该订阅 | 只重置该订阅日用量，扣一次 |
| `GET /api/v1/admin/subscriptions/self-reset-groups` | 完整管理员 | 按 `xunyou` / `wsdashi` / `other` 返回当前候选订阅分组及是否已勾选 |
| `GET /api/v1/admin/subscriptions/self-reset-config` | 完整管理员 | 用同一快照返回 `revision`、`policy`、`groups_by_organization` 供编辑和恢复 |
| `GET /api/v1/admin/subscriptions/self-reset-event-groups` | 完整管理员 | 返回历史记录页的分组筛选选项，独立于候选分组查询 |
| `GET /api/v1/admin/subscriptions/self-reset-policy` | 完整管理员 | 读策略（开放范围与次数，结构不变） |
| `PUT /api/v1/admin/subscriptions/self-reset-config` | 完整管理员 | 按 `expected_revision` 原子提交策略与分组差量，返回新版本。仅带版本时执行结果确认 |
| `POST /api/v1/admin/subscriptions/self-reset-config/close` | 完整管理员 | 无请求体，只把现存 rollout 改为 off 并推进版本 |
| `GET /api/v1/admin/subscriptions/self-reset-events` | 完整管理员 | 分页查询历史；筛选 `email`、`organization`、`group_id`、`start_date`、`end_date`、`page`、`page_size`；前端页面 `/admin/subscriptions/self-reset-events` |

不改现有订阅列表和公共 `UserSubscription` DTO。用户路由沿用 JWT、`BackendModeUserGuard`、面板限流和审计。上述管理接口仅对完整管理员（`role=admin` 或传统/全权兼容管理密钥）开放：均不在任何 `sub_admin` 白名单（`TestSubAdminWriteWhitelistStaysNarrow` 及只读路由检查断言对所有子管理员权限码都拒绝），也不在受限管理密钥 `pkg/adminaccess/catalog.go` 或管理 MCP 目录中登记。

兼容只读接口 `GET /api/v1/admin/subscriptions/self-reset-groups` 响应示例（`groups_by_organization` 严格按 §5.1 返回未软删除订阅分组，`enabled` 表示该组织已勾选该分组）：

```json
{
  "groups_by_organization": {
    "xunyou": [
      { "id": 12, "name": "xunyou-ai", "platform": "openai", "status": "active", "daily_limit_usd": 50, "enabled": true }
    ],
    "wsdashi": [
      { "id": 25, "name": "subao-300", "platform": "openai", "status": "active", "daily_limit_usd": 300, "enabled": false }
    ],
    "other": []
  }
}
```

历史分组接口返回当前分组行仍存在的分组名称，即使分组已软删除或已转为按量；只有物理行不存在时才使用稳定的 `#id`：

```json
{
  "event_groups": [
    { "id": 12, "name": "xunyou-ai" },
    { "id": 25, "name": "subao-300" },
    { "id": 99, "name": "#99" }
  ]
}
```

普通保存示例。只调整分组也要提交表单的原版本。旧的两个独立 PUT 均已退役。

```json
{
  "expected_revision": 7,
  "changes_by_organization": {
    "xunyou": { "enable": [12, 18], "disable": [30] },
    "other": { "enable": [], "disable": [41] }
  }
}
```

- `expected_revision` 必填。可选的 `policy` 继续用 §5.2 的严格校验。`changes_by_organization` 校验组织、正整数 ID、去重、互斥与每个数组最多 1000 项，显式传入空变更或 null 属参数错误。
- 确认请求只带 `{"expected_revision":7}`，匹配时推进版本并提交。原请求随后到达将被拒绝。
- 成功返回 `{"revision":8}`。版本冲突返回 `409 SELF_RESET_REVISION_CONFLICT`，新增勾选不合格返回 `409 SELF_RESET_GROUP_CONFLICT`，均不提交任何业务变更。
- 仅关闭接口返回 `409 SELF_RESET_CLOSE_POLICY_INVALID` 表示本次已读到缺失或损坏的策略、已回滚，允许显式修复。数据库读取失败或提交结果不明仍返回 503；这些错误也可能表现为客户端断连，不能按已关闭处理。

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
    { "subscription_id": 32, "used_count": 1, "remaining_count": 0, "can_reset": false, "disabled_reason": "DAILY_LIMIT_REACHED" },
    { "subscription_id": 33, "used_count": 0, "remaining_count": 1, "can_reset": false, "disabled_reason": "GROUP_NOT_ALLOWED" }
  ]
}
```

POST 合同：
- 必须带 `Idempotency-Key`；body 只允许 `{ "quota_date": "YYYY-MM-DD" }`，日期取自 status。拒绝客户端提供 `user_id`、`organization`、用量或剩余次数（未知 JSON 字段 400）。
- 幂等 scope 为 `SubscriptionSelfResetIdempotencyScope(userID)`（`subscriptions.self_reset.user:{id}`），指纹包含具体订阅 ID 和提交的 `quota_date`；`c.FullPath()` 里的 `:id` 不能代替具体 ID。
- `next_reset_at` 固定为 `timezone.StartOfDay(now).AddDate(0, 0, 1)`，不写死上海、不用 now+24h；多实例必须配置相同时区。
- 成功响应只含 `subscription_id, quota_date, daily_limit, used_count, remaining_count, next_reset_at`，避免把完整订阅存进幂等记录。重放结果是原操作快照，不能覆盖新的权威状态。

`disabled_reason` 是封闭集合，按下列顺序判定（`selfResetItem`），能重置时为 null；灰度不允许时所有订阅统一覆盖为 `ROLLOUT_DISABLED`。`GROUP_NOT_ALLOWED` 置于订阅状态/用量等业务原因之前，确保未勾选分组的订阅卡片在任何状态（含已过期或零用量）下都不展示重置按钮与提示行：

| 顺序 | disabled_reason | 含义 | POST 错误 |
| --- | --- | --- | --- |
| – | `ROLLOUT_DISABLED` | 灰度开关未对当前账号开放 | 403 `SELF_RESET_ROLLOUT_DISABLED`（不加锁、不扣次） |
| 1 | `GROUP_NOT_ALLOWED` | 订阅分组不在当前用户所属组织的已勾选范围：未勾选、用户组织与分组归属不一致、分组已不再属于该组织候选集（含已软删除、已不是订阅型） | 403 `SELF_RESET_GROUP_NOT_ALLOWED` |
| 2 | `SUBSCRIPTION_INACTIVE` | 未生效、过期、暂停、撤销或已删除 | 409 `SELF_RESET_SUBSCRIPTION_INACTIVE` |
| 3 | `GROUP_DISABLED` | 分组停用（分组软删除归入 1） | 409 `SELF_RESET_GROUP_DISABLED` |
| 4 | `ONE_TIME_QUOTA` | 一次性日卡 | 409 `SELF_RESET_ONE_TIME_QUOTA` |
| 5 | `NO_DAILY_LIMIT` | 分组无正数日限 | 409 `SELF_RESET_NO_DAILY_LIMIT` |
| 6 | `POLICY_DISABLED` | 组织上限为 0 | 403 `SELF_RESET_DISABLED` |
| 7 | `DAILY_LIMIT_REACHED` | 当天次数耗尽 | 409 `SELF_RESET_LIMIT_REACHED` |
| 8 | `NO_USAGE` | 当前有效日用量为 0 | 409 `SELF_RESET_NO_USAGE` |

其他错误：非本人、不存在或用户已停用/删除 → 404（统一响应，先于分组准入返回，不泄漏他人订阅）；`quota_date` 不是今天 → 409 `SELF_RESET_DAY_CHANGED`（在 `GROUP_NOT_ALLOWED` 通过之后才校验，避免已取消勾选的分组跨日残留提交时提示“请刷新后重新确认”）；勾选保存请求体非法 → 400 `INVALID_SELF_RESET_GROUPS`；新增勾选不合格时整笔返回 409；历史查询 `group_id` 非正整数、缺幂等键、日期格式非法、未知字段 → 400；策略/计数/幂等存储不可用或无事务 → 503。每个枚举都要有中英文文案；状态未取得或配置读取失败属于请求失败，不能伪装成默认有次数或某个禁用原因。

Gin 按 HTTP 方法分树并优先匹配静态节点，`self-reset-status` 与 `/:id/reset-daily` 的注册顺序不影响正确性（审核时用临时程序验证两种顺序 8/8 命中）；新增路由仍要跑真实路由和权限测试。

## 8. 事务与可靠性

业务重置、扣次、历史记录、幂等成功记录必须在同一个 PostgreSQL 事务内提交。不能"先扣 Redis 次数再调管理接口"，也不能"先清零、成功后再单独记次"。

执行顺序（`SubscriptionSelfResetHandler.Reset` → `SubscriptionSelfResetService.Reset`）：

1. handler 校验认证、ID、JSON、日期格式和非空幂等键；即使全局 `idempotency.observe_only=true` 也必须有键（通用 helper 在 observe_only 下会放过空键，曾是审核发现的半完成风险）。
2. 使用专属幂等入口，`AtomicSuccess=true`，**显式 `TTL: 48 * time.Hour`**（不依赖可配置的默认 TTL，也不改其他接口）。coordinator 或事务能力缺失时拒绝执行；service 入口断言 `ent.TxFromContext(ctx)` 非空（只在 `Reset` 入口断言一次）。
3. 在同一事务内、取订阅锁之前，经事务 executor 读完整策略并快速拒绝未通过灰度的请求。通过后仍须执行锁后的最终授权。事务已占连接后不得再调普通 `SettingRepository.Get/GetValue`（会申请第二条连接，连接池为 1 时死锁）。
4. `GetOwnedByIDForUpdate(ctx, userID, subscriptionID)`：WHERE 同时包含认证 `user_id`、`subscription_id`、未删除，再 `FOR UPDATE`；无匹配统一 404。分组为空（`sub.Group == nil`，分组已软删除）不返回 404：准入查询不会返回已软删除的分组，统一判 `GROUP_NOT_ALLOWED`，使 status 与 POST 一致。锁后在同一事务加载用户和分组，用户非 active 或已删除按 404 处理。
5. 拿锁后在事务连接上用一条查询取得策略和当前组织的分组准入，重新判定 rollout 和组织次数。按锁后时间检查日期和有效用量：**先**检查 `GROUP_NOT_ALLOWED`（不满足立即返回 403 `SELF_RESET_GROUP_NOT_ALLOWED`），**再**检查请求日期 `date != today`（返回 409 `SELF_RESET_DAY_CHANGED`），随后检查其余 `disabled_reason`。不可重置时不写任何业务数据；全部通过后依次 `Consume`（UPSERT 计数）→ `RecordEvent`（历史）→ `ResetUsageWindows(..., true, false, false, timezone.StartOfDay(now), now)`。同一订阅的不同幂等键在父行锁上串行。
6. 业务回调返回前调用 `DeferIdempotencyPostCommit` 注册缓存失效；注册失败返回错误并回滚，不退回立即失效。coordinator 随后在同一事务保存幂等成功结果并提交。
7. 提交成功（或"提交成功但确认丢失"的只读恢复路径）后执行回调，复用 `invalidateFilteredDailyResetCaches`：`InvalidateSubCacheSync`（本机）→ `InvalidateSubscription`（billing cache）→ `PublishSubscriptionCacheInvalidation`（跨实例）。回滚不执行回调。缓存失败只告警，不把已成功的扣次说成失败。

`Reset` 以取得订阅锁后读取的一份完整授权快照判定。已按有效快照获准的请求可在并发收紧后完成。`Status` 也使用同一条查询读取策略与各组准入。

并发计费：入账是 `daily_usage_usd = daily_usage_usd + 金额`，会等待同一行锁，不会把清零前的旧值写回；重置后才完成的入账会重新累加日用量，与管理端重置语义一致，不暂停网关、不改计费链。

成功幂等键再次请求只重放，跨日也不会再清零。尚未成功执行的旧日期请求返回日期变化错误。提交结果不确定时只经 coordinator 回读恢复，不盲目重做。coordinator 会保存业务错误的 `error_reason`，但失败状态是 `failed_retryable`，不会像成功那样重放完整业务 409，所以前端必须按原因分类：

| 结果 | 前端后续动作 |
| --- | --- |
| 网络中断、超时、结果未知的 5xx | 保留原键和原 `quota_date`，重试原操作，不换键 |
| 重试收到 401 / 408 / 429 | 不代表原操作失败，保留原键原日期；重新登录或限流解除后继续核实，遵守 `Retry-After` |
| `IDEMPOTENCY_IN_PROGRESS` / `IDEMPOTENCY_RETRY_BACKOFF` | 遵守 `Retry-After`，原键原日期重试 |
| `SELF_RESET_DAY_CHANGED` | 刷新状态，结束旧操作；用户重新确认后用新日期、新键 |
| 次数用尽、无用量、组织关闭、分组未开放、灰度关闭、订阅不可用 | 展示原因并停止；条件变化后的新点击用新键 |
| `IDEMPOTENCY_KEY_CONFLICT` | 不自动换键重发，刷新状态并结束本次操作 |
| 成功或成功重放 | 只当作原操作回执，刷新列表/次数/store |

观测：请求审计沿用用户路由中间件（含失败和重放，只有通过前置鉴权与 guard 的请求才会进入）；精确历史以 §6.2 的历史表为准；并发验收直接核对计数表与幂等记录。

### 8.1 分组勾选保存与归属清理

普通保存遵循以下事务合同。

1. 锁配置版本行，核对 `expected_revision`。版本过期直接返回冲突。
2. 按分组 ID 升序取得 `groups FOR UPDATE`，用新的语句快照复核资格并应用差量。任一新增勾选已不合格，整笔回滚。取消不存在的勾选不产生新行。
3. 同事务写入本次提交的策略、推进版本，提交后返回新版本。确认请求只推进版本，使未执行的旧写入失效。
4. 强制关闭取得同一版本锁，将库内策略的 rollout 改为 off 并推进版本。次数取库内当前值。运维 SQL 也先锁版本（§15）。归属和类型清理沿用现有分组锁顺序。

归属清理：`ReplaceGroupOrganizationScopes` 在同一事务里、写完新归属之后追加一条语句，按**最终归属**删除这些分组不再匹配的勾选行：

```sql
DELETE FROM subscription_self_reset_groups r
 WHERE r.group_id = ANY($1)
   AND ((r.organization_key = 'other'
         AND EXISTS (SELECT 1 FROM group_organization_scopes s WHERE s.group_id = r.group_id))
     OR (r.organization_key <> 'other'
         AND NOT EXISTS (SELECT 1 FROM group_organization_scopes s
                          WHERE s.group_id = r.group_id AND s.organization_key = r.organization_key)));
```

它不能做成 `group_organization_scopes` 的删除触发器或外键级联：`ReplaceGroupOrganizationScopes` 是先删后插，重复保存相同归属（批量设置时常见）会把勾选清空。清理语句必须在删除旧归属后执行，且在 `organizationKeys` 为空时也不能提前返回；清空归属同样要删除不再匹配的勾选行。分组离开订阅型时归属由 migration 264 的触发器删除，勾选行由 migration 266 的类型变化触发器删除（§6.3）。

## 9. 前端

- `frontend/src/composables/useSubscriptionSelfReset.ts` 集中管理状态和操作；首次进入、手动重试、成功回读、跨日、页面重新可见共用一个 `refresh`。
- 用户端 `frontend/src/views/user/SubscriptionsView.vue` 的 `selfResetVisible(id)` 同时排除 `ROLLOUT_DISABLED` 与 `GROUP_NOT_ALLOWED`，保证未勾选分组的订阅卡片（无论生效/已过期/零用量）既不渲染“重置”按钮，也不渲染卡片内的重置提示行；在确认弹窗打开期间若管理员取消了该分组勾选，提交收到 `SELF_RESET_GROUP_NOT_ALLOWED` 后按确定性拒绝关闭弹窗、提示原因并刷新列表。
- 成功后刷新三处：页面 `getMySubscriptions()`、`self-reset-status`、全局 store `fetchActiveSubscriptions(true)`。全局 store 只在重置成功后强制刷新，切回标签页不强刷。开始重置时使旧读请求代次失效，防止重置前的在途响应覆盖新结果。提交已成功但全局刷新失败时提示"重置成功，状态刷新失败"，下一次 refresh 补刷，不提示重新清零。
- 刷新和提交期间保留上一份状态，按钮不闪成"—"；列表加载失败显示原有 `failedToLoad` 提示，与"自助重置状态暂不可用"分开。
- 幂等键在确认提交时才用 `crypto.getRandomValues` 生成（128 位；HTTP 环境下 `randomUUID` 不可用）。取消尚未提交的确认直接丢弃。
- 结果未知的键按订阅保存（内存 + 按用户隔离的 `sessionStorage`，写入失败退回内存、照常提交）；关闭弹窗、组件重挂载、刷新页面都不删除，只在成功或确定性失败时删除，日期变化后作废。再次打开同一订阅时沿用原键并提示"上一次重置结果尚未确认"；其他订阅不受影响。
- 管理端 `SubscriptionSelfResetPolicyDialog.vue`（完整管理员，按 `canManageSubscriptions` 显示）：
  - 用管理快照同时装载版本、策略、勾选。冲突恢复后，服务端快照整体替换编辑值与基线。
  - 按 `xunyou`（迅游）、`wsdashi`（速宝）、`other`（其他 · 未设置所属组织的分组）渲染三个组织配置卡：每卡头部含组织名称和每日可重置次数输入框（0–100），其下是已选计数（`已选 M / N`）和全选/清空快捷按钮；主体为该组织的订阅分组复选列表（展示分组名、`#id`、平台标识、日限额；无正数日限显示“无日限”，停用显示“已停用”，两个标签各自独立判断，可同时出现；空列表展示空态说明，超过一定高度时内部滚动）。勾选初值取接口返回的 `enabled`。次数设为 `0` 时保留已勾选状态不清空、不禁用，仅在卡片内提示当前次数为 0 暂不开放。
  - 普通保存只发一个请求。没有待确认请求且无修改时可直接关闭。版本冲突或分组资格冲突时回读当前状态，管理员可重新编辑。
  - 保存断连、超时或 5xx 进入待确认状态。用原版本提交仅含 `expected_revision` 的确认请求；得到成功或明确版本冲突后才回读。版本冲突不能判定是哪位管理员完成了原写入。确认或回读失败时保留待确认状态，支持重试和同会话重开恢复。
  - 紧急关闭只提交关闭动作。回包未知时核对服务端 rollout，仍开放时可重发仅关闭动作。其他修改须在确认后的新表单重新编辑。
  - 初次读取部分失败时，已取得版本且可读的一侧可独立编辑。回读必须覆盖待核对的字段，版本过旧或字段缺失时继续锁定普通保存。当前标签页通过按管理员隔离的 `sessionStorage` 恢复待确认标记；禁用该存储时使用内存，仅支持跨弹窗恢复。
- 历史页 `SubscriptionSelfResetEventsView.vue`：筛选栏在“组织”右侧增加“分组”下拉筛选（`Select`，选项来自独立的 `getSelfResetEventGroups()`，默认“全部分组” `''`），切换时立即重查第 1 页，“重置”按钮同步清空 `group_id`。进入页面时选项和列表并行加载，选项加载失败只提示，不影响列表。API client：`frontend/src/api/subscriptionSelfReset.ts`。

## 10. 代码地图

| 层 | 位置 |
| --- | --- |
| 迁移 | `backend/migrations/241_subscription_self_daily_reset.sql`、`242_subscription_self_reset_events.sql`、`266_subscription_self_reset_groups.sql`（含 `subscription_type` 清理触发器）、`267_subscription_self_reset_revision.sql`；分组归属复用 `263_group_organization_scopes.sql`、`264_group_organization_scope_subscription_only.sql` |
| 服务 | `backend/internal/service/subscription_self_reset.go` 处理状态、重置和最终授权；`subscription_self_reset_config.go` 管理版本化配置 |
| 仓储 | `backend/internal/repository/subscription_self_reset_repo.go` 实现订阅锁、准入和组差量；`subscription_self_reset_config_repo.go` 提供管理配置存取。归属清理见 `group_organization_scope_repo.go` |
| Handler | `backend/internal/handler/subscription_self_reset_handler.go`、`subscription_self_reset_config_handler.go` |
| 路由 | `backend/internal/server/routes/user.go`（用户两条）、`routes/admin.go` 的 `registerSubscriptionRoutes`（策略、事件、候选分组和历史分组选项接口） |
| 复用 | `SubscriptionService.invalidateFilteredDailyResetCaches`、`UserSubscription.HasOneTimeDailyQuota`、`DeferIdempotencyPostCommit`、`group_organization_scopes`、`lockLiveGroups` 的锁协议 |
| Wire | `backend/internal/service/wire.go`、`handler/wire.go`、`repository/wire.go`；`ProvidePluginManager` 保留插件 `SetAccountDirectory` 接线（与上游手改的 `wire_gen.go` 曾分叉，合并上游时检查，见 `docs/upstream-sync/merge-log.md`） |
| 前端 | `frontend/src/composables/useSubscriptionSelfReset.ts`、`api/subscriptionSelfReset.ts`、`views/user/SubscriptionsView.vue`、`views/admin/SubscriptionsView.vue`、`views/admin/SubscriptionSelfResetEventsView.vue`、`components/admin/subscription/SubscriptionSelfResetPolicyDialog.vue`、`i18n/locales/{zh,en}/misc.ts` |

## 11. 关键约束与易错点

1. **锁查询必须带用户归属条件。** 现成 `GetByIDForUpdate` 只按 ID 加锁；用它会先锁住他人的订阅行再补鉴权。规则：WHERE 同时含 `user_id`、`subscription_id`、未删除，然后 `FOR UPDATE`。
2. **提交后缓存失效必须是三段式，且只在提交后执行。** 单条 `AdminResetQuota` 缺少跨实例发布，照抄会让其他实例继续用旧的高用量拒绝请求。验收要由另一实例实际回读，不能只断言发布方法被调用；通知失败时靠既有 TTL 收敛，不承诺故障下立即全实例一致。
3. **计次日期与报表时区隔离。** 只用 `timezone.StartOfDay()`；不能借 `organizationUsageLocation`，不能用数据库 `CURRENT_DATE`。DATE 类型本身不必然错日，风险取决于绑定和 SQL 转换路径；驱动编码检查（pq 保留 `+08:00`）不能代替真实 PostgreSQL 验证。
4. **空幂等键必须在入口拒绝。** 全局 `observe_only` 默认开启时，通用 helper 会让空键请求绕过 coordinator 直接执行，产生"只清零不扣次"一类的半完成状态。
5. **事务内不能再借第二条连接。** 策略读取和分组准入查询都必须走事务 `clientFromContext` executor；连接池为 1 时普通连接池读取会卡死。
6. **结果未知时绝不换键。** 曾复现的失败场景：组织上限 ≥2、原请求已提交，之后又有新用量入账；用户关闭弹窗后再次确认，旧实现生成新键，重复清零并多扣一次。默认 1/1/1 时第二次会得到 `LIMIT_REACHED`，掩盖了这个问题；"`NO_USAGE` 能兜住"的前提是中间没有新用量，而来点重置的用户恰好在大量使用。
7. **status 与 POST 的判定必须一致。** 曾复现：分组软删除后 status 显示 `GROUP_DISABLED`，POST 却因锁查询要求分组存在而返回 404；现锁查询不再因分组为空返回 404；扩展后分组软删除在 status 和 POST 两处都判为 `GROUP_NOT_ALLOWED`（准入查询不返回已软删除的分组）。
8. **灰度关闭和分组未勾选时都不显示按钮与提示行。** 注意用户页卡片除了头部的 `重置（N）` 按钮（有 `subscription.status === 'active'` 条件），正文顶部还有一行 `<p v-if="selfResetVisible(subscription.id)">` 提示语（无 `active` 条件）。因此 `GROUP_NOT_ALLOWED` 在 `selfResetItem` 中必须先于 `SUBSCRIPTION_INACTIVE` / `NO_USAGE` 等判定，且 `selfResetVisible` 同时拦截 `ROLLOUT_DISABLED` 和 `GROUP_NOT_ALLOWED`，否则未勾选分组在已过期或零用量时仍会漏出提示或按钮。
9. **`invalidateCache()` 不会刷新数据。** 全局 store 必须 `fetchActiveSubscriptions(true)`。
10. **策略不能接入 `SettingService` 的进程内缓存。** `SettingService` 对若干 key 有 `atomic.Value` 缓存，照搬会让改策略在一个缓存周期内不生效。
11. **管理配置仅对完整管理员开放。** 权限测试覆盖配置 GET/PUT、强制关闭 POST 和兼容只读接口。
12. **只部署前端按钮会让状态接口失败。** 发布必须同时带 migration 266、后端和前端。未刷新的旧版页面不认识 `GROUP_NOT_ALLOWED`，上线后会对未开放的订阅显示置灰按钮和未翻译的原因 key，刷新后恢复。
13. **策略 JSON 保留旧结构，勾选存独立表。** 旧解析器拒绝未知字段。版本独立存入 settings；关闭事务只需取得版本和当前策略，候选故障仍可关停。
14. **滚动发布和回滚必须有数据库前置关闭。** 扩展前的旧后端不会读取 `subscription_self_reset_groups`，在旧 key 为 `all` 时会把所有订阅分组视为可用。管理页不可用时先执行 §15 的只读备份和 SQL 关闭，不能把“登录管理页改 off”作为唯一门禁。关闭提交并确认旧实例读到 `off` 后，排空旧实例，再启动支持 migration 266 的新实例。任何回滚同样先关闭旧 key，再切回旧二进制。migration 266 自动执行不能替代前置关闭，因为迁移失败时旧实例仍可能保持开放。
15. **策略 JSON 不加字段与发布门禁是两个问题。** 新增分组字段会让旧解析器拒绝整份策略并返回 503，因此继续使用独立表。独立表又会被旧实例忽略，因此需要旧 key 的数据库前置关闭。仅复制到新 key 仍需防止旧接口在混跑期间把旧 key 改回 `all`，不能作为单独的兼容方案。
16. **上线前置必须按分组而不是按订阅条数判断。** 分组归属对同一分组的全部订阅生效。先按 `group_id` 汇总所有有效订阅的用户组织、订阅数量和 API Key 数量，再为每个分组生成目标归属集合并计算投影后的不匹配订阅。`xunyou` 与 `wsdashi` 可同时归属，不要求拆分；含 `other` 的混合分组不能让两类用户同时匹配，拆分或接受部分用户不可自助必须记录产品决定。
17. **上线初始勾选策略必须单独确认。** 空表会让当前匹配订阅也暂时不可自助，按当前文档的 fail-closed 方案需要接受这段中断。另一方案是在 rollout 关闭期间回填当前匹配的勾选行。225/195/30/4 只是文档快照，未核验前不能写成上线事实。
18. **软删除不需要作为授权正确性的强制触发器。** 准入查询已带 `g.deleted_at IS NULL`，当前没有正常恢复分组的入口。巡检要报告已软删除分组的残留勾选行，但不能把它们与活跃分组的归属不匹配混为发布阻断项。类型从订阅型离开后仍必须清理，因为改回订阅型是正常管理操作，不能自动恢复旧勾选。
19. **历史分组名称按可用数据定义。** 事件表只保存 `group_id`，但软删除行仍保留名称，历史查询没有 `deleted_at` 过滤。分组行存在时显示当前名称，物理行不存在时才显示 `#id`，候选分组和历史分组接口不能使用两套名称口径。
20. **准入判定必须同时带当前归属条件，不能只查勾选表。** 依据：勾选和归属是两张表；判定里带上归属条件后，即使清理遗漏，移出组织或落入“其他”的分组也不会被放通。候选列表、保存、`Status`、`Reset` 共用 §6.3 的同一条件，不各写一份。
21. **归属变化后的勾选清理必须按最终归属计算，空归属也要清理。** `ReplaceGroupOrganizationScopes` 先删后插，清空归属路径不能在删除旧归属后提前返回。重复保存相同归属不能清空勾选，移出后再移回不能恢复旧勾选（§8.1）。
22. **勾选保存必须避免反向差量死锁。** `groups FOR SHARE` 允许两个勾选保存同时进入勾选表。A 取消 30 勾选 12、B 取消 12 勾选 30 时，可能形成相互等待。勾选保存应使用按 ID 升序的 `groups FOR UPDATE`，或按 `(group_id, organization_key)` 升序先锁定已有勾选行。
23. **一次普通保存只提交一笔事务。** 策略和分组可以独立编辑。任一环节失败，整笔业务配置与版本保持原值。
24. **历史筛选选项不应依赖候选分组接口。** 候选分组读取失败时，历史事件仍应可查询。`event_groups` 独立接口，勾选保存响应不扫描事件表。
25. **管理审计要覆盖归属清理影响。** `/admin/groups` 改归属时除管理请求审计外，`ReplaceGroupOrganizationScopes` 每次写一行日志 `[GroupOrganizationScope] scopes replaced: group_ids=… previous_scopes=… organization_keys=… cleared_self_reset_groups=N`（旧归属取自 `DELETE … RETURNING`，不在 `previous_scopes` 里的分组原本未设归属）。这行日志在调用方提交事务之前写出，回滚的请求也会留下一行，是否生效以管理请求审计的结果为准。分组离开订阅型时由触发器删除的勾选行没有这类记录。
26. **“其他”卡片不等于其他邮箱用户的全部订阅。** 依据：2026-11-25 生产的 4 个其他邮箱订阅都在已归属分组里，“其他”卡片当时管不到任何订阅。排查“为什么没有按钮”先核对用户邮箱组织与分组归属是否一致（§15 第 0 步的查询）。
27. **勾选写入语句里的组织参数必须写成 `$1::text`。** 依据：`INSERT INTO subscription_self_reset_groups (group_id, organization_key) SELECT g.id, $1 … WHERE … $1 = 'other' …` 中，同一个参数在 WHERE 里按 `text` 推断，在 SELECT 列表里又被插入目标列强制为 `character varying`，PostgreSQL 报 `inconsistent types deduced for parameter $1`，勾选保存全部 503。这类错误只在真实 PostgreSQL 上出现，单测和编译发现不了；改这几条准入 SQL 后必须跑 §14 的集成测试。
28. **旧表单不能沿用新版本号。** 版本冲突或未知结果的确认结束后，整体替换编辑值和基线。仅换版本号仍可重新提交已关闭的开放范围。

## 12. 已否决的审核建议

- 已否决：一次性日卡"开放自助等于永久增发、单价五折"的漏洞定性，原因：源码只证明日卡不自动刷新，不能证明销售定价合同；最终按产品决定排除日卡（§1），不作为漏洞记录。
- 已否决：计数日期改用 `quota_day_start timestamptz`，原因：业务标识是自然日，DATE + 日历字符串绑定足够；TIMESTAMPTZ 也能实现，但不是改表理由。
- 已否决："time.Time 写 DATE 必然在凌晨错日"的必现结论，原因：未给出实际 SQL/绑定复现；保留风险并写死绑定方式（§6.1）。
- 取锁前读策略只用于快速拒绝，最终授权采用锁后的同快照读取（§8）。
- 已否决：改为按用户共享计次，原因：用户已明确选择每订阅独立；讨论共享利弊不等于改选。
- 已否决：预留冗余 `user_id` 以便将来切共享，原因：见 §2，冗余列消除不了数据迁移和当天折算。
- 已否决：`other` 默认值改 0，原因：需求是"默认每日一次"，未限定内部员工；管理员可自行配置为 0。
- 已否决：静态路由必须先于 `:id` 注册的硬性门禁，原因：Gin 优先匹配静态节点，实测两种顺序都正确；只保留为排版习惯。
- 已否决：按钮移到进度条旁，原因：与用户明确的"续费后"要求冲突，窄屏问题用布局解决。
- 已否决："计数行只能 UPSERT、先查再插必然错误"，原因：持有共同父行锁时先查再插并非必然错误；实现采用 UPSERT，但它不能替代锁和上限检查。
- 已否决：把分组勾选存进策略 JSON 的 `enabled_group_ids_by_organization`，并在保存时整批校验归属、不符合返回 `400 SELF_RESET_GROUP_ORG_MISMATCH`（2026-11-25 扩展初稿），原因：现有解析拒绝未知字段，新字段保存后回滚或滚动发布期间的旧实例对全部用户返回 503；开放范围和分组列表整体保存，候选接口故障或存量 ID 漂移时无法把开放范围改为关闭；没有外键，要靠运行时双重校验、弹窗回显求交集才能兜住归属漂移，而且分组移出组织再移回时勾选是否恢复取决于期间有没有人保存过弹窗。改为独立表（§6.3）。
- 已否决：在 `group_organization_scopes` 上加 `self_reset_enabled` 列，原因：未设归属的分组没有行，“其他”卡片无处存放勾选；用户要求保留“其他”卡片。
- 已否决：`Reset` 对 `groups` 加 `FOR SHARE` 并规定“先分组后订阅”的锁序（初稿照搬组织负责人分配订阅的 K22），原因：重置是一次性动作，不产生需要与归属保持一致的持久关系；策略、分组状态和日限本来就按快照读取，只把归属变更串行化得不到额外保证，却多一次预读、多一个锁，并与分组编辑互相阻塞。锁保留在管理员保存勾选的路径上（§8.1）。
- 分组只提交差量；配置级版本处理策略和勾选的并发编辑。
- 已否决：放宽匹配规则（其他邮箱用户可在任意分组开放，或未设归属分组对所有用户生效），原因：2026-11-25 用户确认用户组织与分组归属必须一致；不一致的订阅通过调整分组归属解决。
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
| 组织策略 | 0/1/N、当天升降限、邮箱变更、多实例按约定生效；缺失配置 fail-closed；策略 JSON 结构与扩展前一致 |
| 分组勾选与归属 | 迅游/速宝按 `group_organization_scopes` 展示并独立勾选，“其他”展示未设归属的订阅分组；严格按用户邮箱组织对号入座（无跨组织或公共分组兜底）；次数设为 0 保留已选分组；`/admin/groups` 移出组织、给未设归属分组设归属或改为标准分组后立即失去自助资格且勾选行被删除，移回不恢复；重复保存相同归属不清勾选；任一新增非候选分组使整笔保存回滚并返回 409；两个管理员反向修改同一批分组经 `groups FOR UPDATE` 串行且无死锁；并发改归属/删组与勾选保存经同一锁序处理；软删除分组不进入准入，残留勾选只作为巡检信息 |
| 未勾选前端表现与记录筛选 | 未勾选分组的订阅卡片在生效、已过期、零用量等任意状态下均不显示“重置”按钮和提示语；POST 先于跨日检查返回 `403 SELF_RESET_GROUP_NOT_ALLOWED` 且不扣次；历史页通过独立事件分组选项支持含已软删除历史分组的筛选 |
| 回滚与紧急关闭 | 新版本启动前先用 SQL 备份并关闭旧 rollout，确认旧实例读到 `off` 后排空旧实例；再启动自动迁移的新版本；候选准入 SQL、迁移和健康检查通过后才允许保存勾选并恢复 `all`；回滚先关闭 rollout，确认生效后再切回旧二进制；旧二进制恢复“全部分组可用”只允许发生在 rollout 已关闭的状态 |
| 灰度 | `off` 全拒；`admin` 下普通用户状态全为 `ROLLOUT_DISABLED`、POST 403 且不加锁不扣次；`all` 全放开 |
| 权限 | 所有子管理员权限组合都不能访问 配置读写、强制关闭、兼容读取及历史；普通用户/子管理员/完整管理员 × 后端模式开关两态 |
| 管理端 | 用户次数耗尽或分组未勾选时，管理员在后台仍能多次重置，且不回补用户次数 |
| 特殊订阅 | 已勾选分组下未生效、过期、暂停、撤销、分组停用、无日限、日卡均拒绝且不扣次；十年订阅最后一天不误判为日卡 |
| 上线前置校验 | 先按 `group_id` 汇总全部有效订阅的用户组织，标出混合组织分组；为每个分组记录目标归属集合，计算变更后的不匹配订阅数；含 `other` 的混合分组有明确处理决定；执行归属变更后重新核对投影结果 |
| 迁移和数据巡检 | 263、264、266 均已应用；勾选表主键和 `subscription_type` 清理触发器存在；巡检分别报告活跃分组的不匹配勾选和已软删除分组的残留勾选；活跃分组不匹配结果为 0 行 |
| 原子保存与关停 | 写入失败或归属漂移时版本及配置均不变。HTTP 和运维 SQL 关闭保留次数，旧版本保存返回冲突 |
| 锁后准入 | 等待订阅锁时完成配置切换，两套配置均拒绝的订阅始终不能重置，次数、用量与事件不变。另有可重置正对照 |
| 丢包恢复 | 先提交后丢包、原写迟到、确认再次丢包、回读失败及同会话重开均有回归。撤销操作应产生实际写入，候选故障仍可关闭，策略损坏可显式修复 |

## 14. 验证

在 `backend/` 下：

```bash
go test -tags=unit -p 1 -count=1 ./internal/service ./internal/handler ./internal/handler/admin ./internal/server/middleware ./internal/server/routes -run 'SubscriptionSelfReset|ParseSelfReset|AdminPermission|CanAccessAdmin|SubAdmin|AdminAuth|BackendModeUserGuard|SubscriptionBulkActionRoutes|GroupOrganizationScope'
# 真实 PostgreSQL：DSN 只能指向新建的空库（会执行全部迁移）
SUB2API_POSTGRES_ONLY_INTEGRATION_DSN='postgres://...' \
  go test -tags=integration -p 1 -count=1 ./internal/repository -run 'SubscriptionSelfReset|GroupOrganizationScope' -v
```

独立 PostgreSQL 上须看到全部命中用例真实运行并通过。夹具约定：正常重置用例由 `selfResetAllow` 按组织写入 `group_organization_scopes`（`other` 用户使用无归属分组）和 `subscription_self_reset_groups`；主用例在写入前先断言未勾选、只有归属两种状态都是 `GROUP_NOT_ALLOWED` 且不扣次。没有 Docker 且未设 DSN 时 `TestMain` 打印 `docker is not available; skipping integration tests` 并返回 `ok`，这不算通过；CI 不能替代这一步。跨实例测试是同进程两个 `SubscriptionService`/`BillingCacheService` + miniredis TCP/PubSub，不代表生产 Redis 集群或网络分区验收。

在 `frontend/` 下：

```bash
pnpm exec vitest run src/views/user/__tests__/SubscriptionsView.selfReset.spec.ts src/composables/__tests__/useSubscriptionSelfReset.spec.ts src/components/admin/subscription/__tests__/SubscriptionSelfResetPolicyDialog.spec.ts src/views/admin/__tests__/SubscriptionSelfResetEventsView.spec.ts src/views/admin/__tests__/SubscriptionsView.spec.ts src/i18n/__tests__/localeKeyCompleteness.spec.ts
pnpm typecheck
```

最近结果：
- 本轮 PG 首次全套的等锁断言超时，当时请求尚未进入业务事务。集成测试改为从请求的真实事务取得 PID，再以持锁连接核对 `pg_blocking_pids`。单项连续 3 次观测到预期等待并通过。在另一独立数据库复跑，10 个顶层用例全部实际通过。
- 2026-11-27，紧急关闭恢复专项。应用内浏览器经测试身份连接真实 Go 配置接口及隔离 PostgreSQL：策略 JSON 损坏时，关闭得到确定性 409，原弹窗可填全次数、选择 off 并保存。缺少策略行时的同类拒绝由 PostgreSQL 集成用例验证。旧表单停在版本 3，其他写入已推进到 4/off；关闭请求未送达时，重读和重开均保持未确认。显式重发后返回版本 5，版本 4 的开放请求得到 409。版本行损坏时的两阶段故障关停与恢复 SQL 也已在隔离库验证。
- 前端专项 6 个文件 89 项通过。分别移除业务保存版本推进校验、空确认版本推进校验或 409 原因校验，三种变异均使对应测试失败。服务层 `Status` 的测试让独立策略和准入快照返回不同值，正常实现连续 10 次通过，修改为采用独立策略后连续 3 次失败。仓储层的查询计数仅验证 `ReadAdmission` 自身使用一条 SQL。
- 2026-11-27，版本化保存与准入修复在本地分支完成验收。后端五个包的专项通过。独立 PostgreSQL 18.3 空库实际运行 10 个顶层集成用例，包含 6 个管理配置子用例、订阅锁等待、24 轮配置切换与重置成功正对照。拒绝路径核对计数、日用量与事件。运维关闭 SQL 实测保留次数和勾选，重复关闭推进版本，旧表单收到 409。Go 1.27.4 的 vet 通过，golangci-lint 在 unit、integration 两种标签下对含新增文件的改动均报 0 issues。
- 前端 6 个专项文件 77 项通过，其中管理弹窗 24 项。类型检查、改动文件 ESLint、前端生产构建及 Go 服务构建通过。应用内浏览器在本机构建上连接真实配置 Handler 和独立 PostgreSQL。勾选提交后丢弃成功回包，页面自动确认并重绘；撤销后实际发出 disable，重开显示未勾选。另验两字段联合保存、第二个标签页紧急关闭后旧表单被拒，直接查库为 off、次数 1/2/0、勾选 0。登录身份使用本机夹具。历史筛选和用户卡片使用模拟只读数据核对。生产发布仍待 §15。
- 2026-11-27（分支 `feature/hy/10241_self_reset_group_allowlist` 工作区，未提交，本机）：上面的后端 unit 命令 5 个包通过；嵌入式 PostgreSQL 18.3 新建空库上集成命令 8 个用例全部 `--- PASS`；前端 6 个文件 74 项通过，`pnpm typecheck`、改动文件 ESLint 通过；golangci-lint（`--new-from-rev=HEAD`，`unit` 与 `integration` 标签各一次）0 issues。去掉勾选保存的 `FOR UPDATE` 后并发用例按预期失败（两个分组同时被勾选），确认该用例有效。浏览器未验收。
- 2026-11-04（HEAD `26f6363af`，本机）：`-tags=unit -run 'SubscriptionSelfReset|TestAdminResetDailyFiltered|TestFilteredDailyResetCache|SubAdminWriteWhitelist'` 等定向测试在 service、repository、handler/admin、server/routes 通过；前端与集成未重跑。
- 2026-09-24（功能分支，本机）：Go service、handler、handler/admin、middleware、routes 专项通过，`go vet -tags=integration ./internal/repository` 通过；前端 6 个文件 50 项通过，vue-tsc、ESLint 通过。首版自审时 golangci-lint（`--new-from-rev=HEAD`）0 issues，Wire 重复生成稳定。
- 2026-09-23 至 09-24 的 PostgreSQL 集成测试在后续修复（跨用户同键、软删除分组）之后未重跑，当时只通过编译。
- 2026-11-04（main，组织负责人功能验证）：嵌入式 PostgreSQL 18.3 空库上运行包含 `SubscriptionSelfReset` 的集成筛选，75 个顶层用例通过（见 [组织负责人权限设计](organization-leader-design-cn.md) §15.3）。
- 扩展前另有桌面、375px 中文和长英文布局检查。本次扩展的实际浏览器验收见本节本轮记录。

## 15. 上线与 PostgreSQL 验证

任何一步不符合预期，就将开放范围改为“关闭”。提交后，新的准入检查即按关闭处理，已完成准入检查的在途请求按原快照收尾。管理页不可用时按下文库状态选择对应 SQL，在主库每次只执行一段；客户端遇错即停（psql 使用 `-v ON_ERROR_STOP=1`），提交后用新连接核对。候选分组接口故障不阻塞关闭。

**发布和回滚总门禁**

1. 扩展发布前备份策略并停止旧管理写入，按版本选用本节关闭 SQL。确认旧实例均读到 `off`，排空并停止旧实例的管理和用户流量。
2. 在 rollout 保持 `off` 且旧实例已排空的状态下，启动支持 migration 266 和 267 的新实例。迁移失败时保持关闭，修复或回滚。
3. 确认所有流量实例都已支持分组准入和版本化保存，并核对 266、267 均已应用。检查实例版本、§6.3 准入 SQL 和健康检查，全部通过后继续。
4. 完成第 0 步的分组级前置校验和归属处理，再由完整管理员保存分组勾选。通过独立准入 SQL 和新版本状态接口核对隐藏/显示后，才允许将 rollout 改为 `admin` 或 `all`。
5. 回滚时先执行已迁移库适用的版本化 SQL 关闭 rollout，确认所有实例读到关闭状态并排空新实例，再切回扩展前的二进制。旧版本不读勾选表，关闭状态下不会产生越权重置；回滚完成后是否恢复开放，必须重新执行迁移、实例版本和勾选状态核对。

**267 已应用、版本可信时的紧急关闭。** 在主库执行一个事务，先锁版本再改策略。重复关闭也推进版本。任一检查失败均回滚；版本异常时转到下方故障关闭步骤。

```sql
BEGIN;
DO $close$
DECLARE old_revision text;
        applied_rollout text;
BEGIN
  SELECT value INTO STRICT old_revision FROM settings
   WHERE key = 'subscription_self_daily_reset_revision' FOR UPDATE;
  IF old_revision !~ '^[1-9][0-9]{0,15}$' THEN
    RAISE EXCEPTION 'Invalid self-reset configuration revision';
  END IF;
  IF old_revision::bigint >= 9007199254740991 THEN
    RAISE EXCEPTION 'Self-reset configuration revision exhausted';
  END IF;
  UPDATE settings SET value = (old_revision::bigint + 1)::text, updated_at = NOW()
   WHERE key = 'subscription_self_daily_reset_revision';
  UPDATE settings SET value = jsonb_set(value::jsonb, '{rollout}', '"off"'::jsonb, true)::text,
                      updated_at = NOW()
   WHERE key = 'subscription_self_daily_reset_policy' RETURNING value::jsonb->>'rollout' INTO STRICT applied_rollout;
  IF applied_rollout IS DISTINCT FROM 'off' THEN RAISE EXCEPTION 'Self-reset policy could not be closed'; END IF;
END;
$close$;
COMMIT;
```

首次升级、尚未应用 migration 267 时，先停止旧管理写流量，备份当前策略并执行下面的旧库关闭 SQL。脚本检查 267 迁移记录和版本行，任何一个已存在都会报错退出。确认关闭并排空旧实例后启动迁移。新版本运行期间及回滚前使用上面的版本化 SQL。

```sql
BEGIN;
DO $close_legacy$
BEGIN
  IF EXISTS (SELECT 1 FROM schema_migrations WHERE filename = '267_subscription_self_reset_revision.sql')
     OR EXISTS (SELECT 1 FROM settings WHERE key = 'subscription_self_daily_reset_revision') THEN
    RAISE EXCEPTION 'Use the versioned self-reset close SQL';
  END IF;
  UPDATE settings SET value = jsonb_set(value::jsonb, '{rollout}', '"off"'::jsonb, true)::text,
                      updated_at = NOW()
   WHERE key = 'subscription_self_daily_reset_policy';
  IF NOT FOUND THEN RAISE EXCEPTION 'Missing self-reset policy'; END IF;
END;
$close_legacy$;
COMMIT;
```

策略 JSON 已损坏时，服务端本来就按 `SELF_RESET_UNAVAILABLE` 关闭自助能力；SQL 转换失败后保持关闭，不得写入一个未经核对的默认策略。

**267 已应用、版本行缺失、损坏、重复、耗尽或疑似回退。**

1. **先止血。** 暂停所有实例的新旧配置写入口、自动重试和运维脚本，排空在途事务，阻断队列和旧客户端重送。保留两项 settings 的原始行（含 id、value、updated_at）及备份。在主库执行下段 SQL。事务先取得 settings 表的 `EXCLUSIVE` 锁，可等待已有的 `FOR UPDATE` 和写事务，也能保护版本行缺失的情况；普通查询仍可进行。锁超时则回滚，查明持锁者并排空后重试。

```sql
BEGIN;
SET LOCAL lock_timeout = '5s';
LOCK TABLE settings IN EXCLUSIVE MODE;
DO $close_damaged$
DECLARE policy jsonb;
BEGIN
  IF NOT EXISTS (SELECT 1 FROM schema_migrations WHERE filename = '267_subscription_self_reset_revision.sql') THEN
    RAISE EXCEPTION 'Verify migration 267 before using damaged-revision close';
  END IF;
  SELECT value::jsonb INTO STRICT policy FROM settings WHERE key = 'subscription_self_daily_reset_policy';
  IF jsonb_typeof(policy) IS DISTINCT FROM 'object' THEN RAISE EXCEPTION 'Invalid self-reset policy'; END IF;
  UPDATE settings SET value = jsonb_set(policy, '{rollout}', '"off"'::jsonb, true)::text, updated_at = NOW()
   WHERE key = 'subscription_self_daily_reset_policy';
END;
$close_damaged$;
COMMIT;
```

在新连接回读主库，确认唯一策略的 `rollout=off`、三个次数与备份一致、勾选未变、原版本行（或缺行）原样保留。用户状态应显示关闭，配置写入口继续隔离。版本缺失或不可解析时，新版普通保存会拒绝；即使版本外观正常，也不能用它证明待重放的旧请求已经失效。若策略本身不可读、缺失或重复，保持隔离并核验用户侧拒绝，单独恢复策略完整性。

2. **有证据后再恢复管理写入。** 用连续 WAL 等能覆盖故障前最后一次提交的记录，在隔离副本核对所有已发出版本的可信上界 `H`，包括应答丢失的提交、空确认和紧急关闭。单个旧快照、浏览器当前值、时间戳和迁移初值 1 都不能证明高水位。没有可靠上界时保持 `off` 和写隔离；现有协议只校验整数版本。另一条恢复路径需要独立切换写入协议或凭据，使旧会话、旧表单和在途重试永久失效，并实际重放验证。关闭页面或单纯重新登录不能提供这种保证。若键重复，先保全所有原始行，由 DBA 在写隔离下恢复 `settings.key` 全表唯一性及一条可信策略；下面的脚本拒绝重复行，不替操作者选择保留哪条。

仍在写隔离期间执行。将下方空串替换为经核实并记录出处的 `H`。新版本取 `H+1`，并为后续正常保存保留一个 JavaScript 安全整数。超过容量时继续保持关闭。

```sql
BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL self_reset.verified_high_water = '';
LOCK TABLE settings IN EXCLUSIVE MODE;
DO $recover_revision$
DECLARE supplied text := current_setting('self_reset.verified_high_water');
        high_water bigint;
        rollout text;
        rows_found bigint;
        existing text;
BEGIN
  IF NOT EXISTS (SELECT 1 FROM schema_migrations WHERE filename = '267_subscription_self_reset_revision.sql') THEN
    RAISE EXCEPTION 'Verify migration 267 before repairing revision';
  END IF;
  IF supplied !~ '^[1-9][0-9]{0,15}$' THEN RAISE EXCEPTION 'Verified high-water mark required'; END IF;
  high_water := supplied::bigint;
  IF high_water > 9007199254740989 THEN RAISE EXCEPTION 'Insufficient safe revisions remaining'; END IF;
  SELECT value::jsonb->>'rollout' INTO STRICT rollout FROM settings WHERE key = 'subscription_self_daily_reset_policy';
  IF rollout IS DISTINCT FROM 'off' THEN RAISE EXCEPTION 'Close the policy first'; END IF;
  SELECT count(*), min(value) INTO rows_found, existing FROM settings WHERE key = 'subscription_self_daily_reset_revision';
  IF rows_found > 1 THEN RAISE EXCEPTION 'Restore unique settings keys first'; END IF;
  IF existing ~ '^[+]?[0-9]+$' AND length(ltrim(existing, '+0')) BETWEEN 1 AND 16 THEN
    IF ltrim(existing, '+0')::bigint BETWEEN high_water+1 AND 9007199254740991 THEN
      RAISE EXCEPTION 'Existing revision exceeds verified high-water mark';
    END IF;
  END IF;
  INSERT INTO settings(key, value, updated_at) VALUES ('subscription_self_daily_reset_revision', (high_water+1)::text, NOW())
  ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at;
END;
$recover_revision$;
COMMIT;
```

用新连接及管理配置 GET 核对版本恰为 `H+1`、策略仍为 `off`、次数与勾选一致。在隔离入口使用一个已发出的旧版本做仅含 `expected_revision` 的确认，应返回 `409 SELF_RESET_REVISION_CONFLICT` 且数据库不变。通过后让管理员重新打开表单，恢复新版配置写入口；重新灰度开放另行决定。

**第 0 步：扩展上线前置（按分组校验，2026-11-25 生产快照）**

- 文档记录的生产快照为 225 个有效订阅，其中 195 个当前用户组织与分组归属匹配，30 个不匹配，另有 4 个其他邮箱订阅位于已归属分组。上述数字只来自文档，执行前必须重新查询，不能直接作为上线事实或分组归属变更数量。
- 分组归属是 `group_id` 级别的设置。每个分组必须先汇总全部有效订阅的用户组织，列出未设归属、单组织归属、多组织归属和同时包含 `other` 的混合分组。
- 含 `other` 用户的分组不能同时设置 `xunyou` 或 `wsdashi` 归属后仍让 `other` 用户匹配。`xunyou` 与 `wsdashi` 用户可以通过多组织归属同时匹配。含 `other` 的混合分组可以拆分，也可以由管理员明确记录哪些订阅上线后保持不可自助；拆分前必须核对该分组的 API Key 和订阅迁移成本。
- 管理员先根据分组级投影结果在 `/admin/groups` 设置最终归属，再执行自助重置弹窗里的勾选。顺序是先设归属、再勾选，归属变化会删除不再匹配的勾选行。
- 下面第一条查询展示当前不匹配订阅，第二条查询按分组汇总全部用户组织和 API Key 数量，第三条查询用于填入计划归属后计算预计不匹配订阅，第四条查询在 rollout 关闭时直接核对当前准入。第二、第三、第四条查询都要在归属变更前后执行，变更后的预计不匹配数必须与上线决定一致：

```sql
SELECT g.id, g.name, g.subscription_type, t.user_org,
       COALESCE((SELECT string_agg(s.organization_key, ',' ORDER BY s.organization_key)
                   FROM group_organization_scopes s WHERE s.group_id = g.id), '(未设归属)') AS group_scope,
       count(*)
  FROM user_subscriptions us
  JOIN users u ON u.id = us.user_id
  JOIN groups g ON g.id = us.group_id
 CROSS JOIN LATERAL (SELECT CASE LOWER(SPLIT_PART(u.email, '@', 2))
          WHEN 'xunyou.com' THEN 'xunyou' WHEN 'wsdashi.com' THEN 'wsdashi' ELSE 'other' END AS user_org) t
 WHERE us.deleted_at IS NULL AND us.status = 'active' AND us.expires_at > NOW() AND g.deleted_at IS NULL
   AND CASE WHEN t.user_org = 'other'
            THEN EXISTS (SELECT 1 FROM group_organization_scopes s WHERE s.group_id = g.id)
            ELSE NOT EXISTS (SELECT 1 FROM group_organization_scopes s
                              WHERE s.group_id = g.id AND s.organization_key = t.user_org) END
 GROUP BY 1, 2, 3, 4, 5 ORDER BY count(*) DESC;

-- 按分组汇总全部有效订阅的用户组织，先处理混合分组再决定目标归属
SELECT g.id, g.name, g.subscription_type,
       array_agg(DISTINCT t.user_org ORDER BY t.user_org) AS user_organizations,
       count(*) AS active_subscriptions,
       (SELECT count(*) FROM api_keys k WHERE k.group_id = g.id AND k.deleted_at IS NULL) AS active_api_keys,
       COALESCE((SELECT string_agg(s.organization_key, ',' ORDER BY s.organization_key)
                   FROM group_organization_scopes s WHERE s.group_id = g.id), '(未设归属)') AS current_scope
  FROM user_subscriptions us
  JOIN users u ON u.id = us.user_id
  JOIN groups g ON g.id = us.group_id
 CROSS JOIN LATERAL (SELECT CASE LOWER(SPLIT_PART(u.email, '@', 2))
          WHEN 'xunyou.com' THEN 'xunyou' WHEN 'wsdashi.com' THEN 'wsdashi' ELSE 'other' END AS user_org) t
 WHERE us.deleted_at IS NULL AND us.status = 'active' AND us.expires_at > NOW() AND g.deleted_at IS NULL
 GROUP BY g.id, g.name, g.subscription_type ORDER BY g.id;

-- 将所有分组的计划归属填入 target_scopes，不能只填变更项。
-- 未列出的 group_id 会按 other 计算。
WITH target_scopes(group_id, organization_key) AS (
    VALUES
      (12345, 'xunyou')
      -- ,(12346, 'wsdashi')
), active_subscriptions AS (
    SELECT us.id AS subscription_id, g.id AS group_id, g.subscription_type,
           CASE LOWER(SPLIT_PART(u.email, '@', 2))
             WHEN 'xunyou.com' THEN 'xunyou'
             WHEN 'wsdashi.com' THEN 'wsdashi'
             ELSE 'other'
           END AS user_org
      FROM user_subscriptions us
      JOIN users u ON u.id = us.user_id
      JOIN groups g ON g.id = us.group_id
     WHERE us.deleted_at IS NULL AND us.status = 'active' AND us.expires_at > NOW() AND g.deleted_at IS NULL
), projected AS (
    SELECT a.*,
           CASE WHEN a.subscription_type <> 'subscription' THEN false
                WHEN a.user_org = 'other'
                THEN NOT EXISTS (SELECT 1 FROM target_scopes t WHERE t.group_id = a.group_id)
                ELSE EXISTS (SELECT 1 FROM target_scopes t
                              WHERE t.group_id = a.group_id AND t.organization_key = a.user_org)
           END AS will_match
      FROM active_subscriptions a
)
SELECT p.group_id, g.name, p.user_org, count(*) AS subscriptions
  FROM projected p
  JOIN groups g ON g.id = p.group_id
 WHERE NOT p.will_match
 GROUP BY p.group_id, g.name, p.user_org
 ORDER BY p.group_id, p.user_org;

-- 不依赖 rollout 的当前准入核对。每个有效订阅都直接按 §6.3 条件返回 allowed。
SELECT us.id AS subscription_id, u.email, g.id AS group_id, g.name,
       CASE LOWER(SPLIT_PART(u.email, '@', 2))
         WHEN 'xunyou.com' THEN 'xunyou'
         WHEN 'wsdashi.com' THEN 'wsdashi'
         ELSE 'other'
       END AS user_org,
       CASE
         WHEN g.deleted_at IS NOT NULL OR g.subscription_type <> 'subscription' THEN false
         ELSE EXISTS (
           SELECT 1
             FROM subscription_self_reset_groups r
            WHERE r.group_id = g.id
              AND r.organization_key = CASE LOWER(SPLIT_PART(u.email, '@', 2))
                WHEN 'xunyou.com' THEN 'xunyou'
                WHEN 'wsdashi.com' THEN 'wsdashi'
                ELSE 'other'
              END
              AND (
                (r.organization_key = 'other' AND NOT EXISTS (
                   SELECT 1 FROM group_organization_scopes s WHERE s.group_id = g.id
                ))
                OR
                (r.organization_key <> 'other' AND EXISTS (
                   SELECT 1 FROM group_organization_scopes s
                    WHERE s.group_id = g.id AND s.organization_key = r.organization_key
                ))
              )
         )
       END AS allowed
  FROM user_subscriptions us
  JOIN users u ON u.id = us.user_id
  JOIN groups g ON g.id = us.group_id
 WHERE us.deleted_at IS NULL AND us.status = 'active' AND us.expires_at > NOW()
 ORDER BY us.id;
```

**第 1 步：迁移和配置（只读 SQL）**

```sql
SELECT filename, applied_at FROM schema_migrations
 WHERE filename IN ('241_subscription_self_daily_reset.sql', '242_subscription_self_reset_events.sql',
                    '263_group_organization_scopes.sql', '264_group_organization_scope_subscription_only.sql',
                    '266_subscription_self_reset_groups.sql', '267_subscription_self_reset_revision.sql');

SELECT column_name, data_type, is_nullable FROM information_schema.columns
 WHERE table_name = 'subscription_self_daily_reset_usage' ORDER BY ordinal_position;

SELECT conname, pg_get_constraintdef(oid) FROM pg_constraint
 WHERE conrelid = 'subscription_self_daily_reset_usage'::regclass;

SELECT key, value FROM settings WHERE key IN ('subscription_self_daily_reset_policy','subscription_self_daily_reset_revision');

SELECT count(*) FROM subscription_self_reset_groups;

SELECT column_name, data_type, is_nullable FROM information_schema.columns
 WHERE table_name = 'subscription_self_reset_groups' ORDER BY ordinal_position;

SELECT conname, pg_get_constraintdef(oid) FROM pg_constraint
 WHERE conrelid = 'subscription_self_reset_groups'::regclass;

SELECT tgname, pg_get_triggerdef(oid) FROM pg_trigger
 WHERE tgrelid = 'groups'::regclass AND NOT tgisinternal
   AND tgname = 'groups_clear_self_reset_on_type_change'
 ORDER BY tgname;
```

期望：241、242、263、264、266、267 六条迁移都在；计数表 4 列依次为 `subscription_id bigint`、`quota_date date`、`used_count integer`、`updated_at timestamptz`，约束有主键、`REFERENCES user_subscriptions(id) ON DELETE CASCADE`、`used_count >= 0`；勾选表 3 列依次为 `group_id bigint`、`organization_key varchar(20)`、`created_at timestamptz`，约束有主键、组织码 CHECK 和 `REFERENCES groups(id) ON DELETE CASCADE`；配置版本为正整数，策略 JSON 结构与扩展前一致；`subscription_self_reset_groups` 刚发布时为 0 行或按已确认方案回填；存在 `subscription_type` 离开订阅型的自助勾选清理触发器。软删除残留由巡检 SQL 报告，不要求存在 `deleted_at` 触发器。若 migration 266 被占用，查询中的文件名替换为实际采用的下一个编号。

**第 2 步：核对分组准入和弹窗勾选保存。**
1. rollout 保持 `off` 时先执行第 0 步第四条准入 SQL，核对每个有效订阅的 `allowed`，不依赖前端状态接口。
2. 先在 `/admin/groups` 确认各订阅分组已按实际业务设置好所属组织（迅游 / 速宝，未设置的归入“其他”）。
3. 完整管理员打开“订阅管理 → 自助重置设置”，确认迅游、速宝、“其他”三张卡片正确展示各自候选分组；勾选允许自助重置的目标分组并保存。再次执行准入 SQL，确认匹配分组为 `allowed=true`，不匹配分组为 `allowed=false`。确认无误后再将 rollout 改为 `admin` 或 `all`，分别观察管理员和普通用户的页面结果。

**第 3 步：管理员真实重置一次。** 准备一个完整管理员：管理员邮箱组织与订阅分组归属一致，分组已勾选，有效订阅、分组有日限、当天已有用量、所在组织次数不为 0。记下订阅 ID `:sid` 和用户 ID `:uid`。

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

-- 活跃分组的失效或归属不匹配勾选行应为 0 行
SELECT r.organization_key, r.group_id
  FROM subscription_self_reset_groups r
  JOIN groups g ON g.id = r.group_id
 WHERE g.deleted_at IS NULL
   AND (g.subscription_type <> 'subscription'
    OR (r.organization_key = 'other'
        AND EXISTS (SELECT 1 FROM group_organization_scopes s WHERE s.group_id = g.id))
    OR (r.organization_key <> 'other'
        AND NOT EXISTS (SELECT 1 FROM group_organization_scopes s
                         WHERE s.group_id = g.id AND s.organization_key = r.organization_key)));

-- 软删除分组的残留勾选只作维护统计，不阻断授权核对；准入查询已拒绝这些行
SELECT r.organization_key, r.group_id, g.name, g.deleted_at
  FROM subscription_self_reset_groups r
  JOIN groups g ON g.id = r.group_id
 WHERE g.deleted_at IS NOT NULL
 ORDER BY r.group_id, r.organization_key;

-- 近一天失败原因：NO_USAGE、LIMIT_REACHED、GROUP_NOT_ALLOWED（分组被取消勾选或未开放）属正常业务拒绝；出现 SELF_RESET_UNAVAILABLE 要排查
SELECT status, error_reason, count(*) FROM idempotency_records
 WHERE scope LIKE 'subscriptions.self_reset.%' AND created_at > NOW() - INTERVAL '1 day'
 GROUP BY 1, 2;
```

## 16. 状态与待办

- [x] **按组织勾选可自助重置分组（2026-11-25 设计审核定稿）**：migration 266 勾选表和 `subscription_type` 清理触发器、准入查询接入 `Status`/`Reset`、勾选读写接口、独立历史分组选项接口、`ReplaceGroupOrganizationScopes` 勾选清理、管理弹窗分组勾选、记录页分组筛选、用户端 `GROUP_NOT_ALLOWED` 隐藏已实现；单测、PostgreSQL 18 集成（含并发）和前端测试已通过（§14，2026-11-27），尚未提交和合入。
- [x] 本机浏览器已核对管理弹窗三组织、勾选与撤销、强制关闭、历史记录分组筛选、未准入订阅卡片隐藏重置操作。
- [ ] 扩展上线前置：按 `group_id` 汇总用户组织，处理混合组织分组，记录目标归属和投影后的不匹配订阅数，再执行归属变更（§15 第 0 步）。
- [ ] 发布门禁：先备份并用 SQL 将 rollout 设为 `off`，确认旧实例排空后再启动自动迁移的新版本；核对准入 SQL 后再勾选和恢复开放；回滚先执行版本化 SQL 关闭 rollout（§15 发布和回滚总门禁）。
- [ ] 生产按 §15 核对并记录结果（上线后先由完整管理员在“自助重置设置”勾选各组织开放的分组并保存，再核对灰度开关与第 3–5 步）。
- [ ] 真实账号浏览器验收（当前只有隔离模拟 API 的页面观察）。
- [ ] 生产 Redis 集群 / 多进程部署下的跨实例失效验收（当前只有同进程 miniredis）。
- 已解决：实现审核提到"分配接口不检查分组是否启用"，现 `SubscriptionService.assignSubscriptionWithReuse` 对停用分组返回 `GROUP_NOT_ACTIVE`。
- 已解决：实现审核的两项前端低优先级问题（提交失败后卡片停在"重置（—）"、策略损坏时弹窗无法覆盖）已在 2026-09-23/24 修复（§9、§5）。
- 不做：自助重置的单用户或单分组独立次数覆盖、一次性日卡开关、权益扩展；需要时另行讨论，不能借技术实现扩大范围。
