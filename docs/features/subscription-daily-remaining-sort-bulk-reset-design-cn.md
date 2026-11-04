# 订阅每日剩余排序与批量重置设计

更新：2026-11-04。状态：**已实现并合入 main（功能提交 `fbc812fcf`，分支 `feature/hy/10176_subscription_sort_daily_reset`；排序参数热修复 `fb43b3349`，分支 `hotfix/hy/0000_subscription_sort_param`）。此后部门功能为同一组接口加了部门范围与 `scope_version`，组织筛选增加了 `other`；本文按 HEAD `26f6363af` 的源码校对。**

本文是管理端订阅列表"按每日剩余额度排序 + 组织筛选 + 按筛选一键重置日限"的唯一文档。原设计稿、实施计划和热修复计划已移出仓库，最后存在于提交 `26f6363af`（`git show 26f6363af:docs/superpowers/specs/2026-08-12-subscription-daily-remaining-sort-bulk-reset-design.md`、`git show 26f6363af:docs/superpowers/plans/2026-08-12-subscription-daily-remaining-sort-bulk-reset.md`、`git show 26f6363af:docs/superpowers/plans/2026-08-12-subscription-sort-param-hotfix.md`）；计划中的任务已全部落地，以代码为准。

相关功能：
- 用户在自己的订阅卡片上自助重置日额度：[用户订阅自助重置日额度设计](subscription-self-daily-reset-design-cn.md)。它复用本功能的提交后三段式缓存失效 `invalidateFilteredDailyResetCaches`。
- 子管理员权限码与白名单：[子管理员角色设计](sub-admin-role-design-cn.md) §3。
- 部门负责人的范围与可重置对象：[组织负责人权限设计](organization-leader-design-cn.md) §4.5。

阅读顺序建议：改排序读 §2、§6；改批量重置读 §3、§4；改页面读 §5；跑测试读 §8。

## 1. 需求

在管理端订阅列表：
- 增加组织筛选（全部 / 迅游 / 速宝 / 其他）；
- 整个筛选结果按"每日额度剩余比例"从少到多稳定排序（跨页，不是只排当前页）；
- 允许完整管理员和有订阅重置权限的子管理员一键重置当前筛选结果中全部生效订阅的日限。

不新增数据库字段或 migration，不改组织用量报表，不改单条"重置配额"（日/周/月）和"重置日限"（只传 `daily=true`）的现有语义，不把组织名持久化到用户或订阅表。

## 2. 组织筛选与排序

### 2.1 组织筛选

| 邮箱域名（`@` 后完整域名，忽略大小写） | API 值 | 展示 |
| --- | --- | --- |
| 精确等于 `xunyou.com` | `xunyou` | 迅游 |
| 精确等于 `wsdashi.com` | `wsdashi` | 速宝 |
| 其余 | `other` | 其他 |

- 子域名（如 `team.xunyou.com`）不归入迅游/速宝。SQL 复用组织用量报表的分类表达式 `organizationUsageOrganizationExpression`，与报表口径一致。
- 空值或 `all` 表示不过滤；未知非空值返回 400，不回退为全部。
- 首版只有 `xunyou`/`wsdashi`；`other` 由后续部门功能加入（`OrganizationOther`）。

### 2.2 每日剩余比例

有效日限额（分组 `daily_limit_usd`）大于 0 的订阅：

```text
剩余比例 = max(日限额 - 当前有效日用量, 0) / 日限额
```

"当前有效日用量"与列表展示的归一化一致：
- 普通订阅：`daily_window_start` 早于服务端配置时区今天 0 点时按 0；
- 一次性日额度订阅（`expires_at <= starts_at + 1 day`）：始终用已存储的 `daily_usage_usd`；
- 日限额为空、0 或负数视为无限额，不参与比例计算。

### 2.3 排序顺序（在数据库分页前完成）

1. 有有效日限额的订阅在前，无限额的在后；
2. 每日剩余比例升序；
3. 比例相同按请求的 `sort_by`（`created_at` | `expires_at` | `status`，默认 `created_at`）和 `sort_order`（`asc` | `desc`，默认 `desc`）；
4. 仍相同按订阅 ID 升序。

`sort_by`/`sort_order` 仍被接受，但含义收窄为"比例相同记录的次排序"，所以表头排序交互和默认 `created_at desc` 保留。分组通过 LEFT JOIN（`admin_sort_group`，排除已删除分组）取日限额。

## 3. 接口合同

### 3.1 列表

`GET /api/v1/admin/subscriptions`

| 参数 | 说明 |
| --- | --- |
| `page`、`page_size` | 分页 |
| `status`、`user_id`、`group_id`、`platform` | 原有筛选；非法值 400 |
| `organization` | `xunyou` / `wsdashi` / `other`，空或 `all` 不过滤 |
| `department_id`、`scope_version` | 部门功能加入的范围参数，规则见部门/组织负责人文档 |
| `sort_by`、`sort_order` | 次排序，见 §2.3 |

响应 `{items, total, page, page_size, pages, scope_version}`；子管理员拿到的 `items` 经 `adminSubscriptionResponse` 字段投影。所有筛选由 `NormalizeSubscriptionAdminFilter` 统一归一化和校验（列表与批量重置共用 `SubscriptionAdminFilter`）。

### 3.2 按筛选批量重置日限

`POST /api/v1/admin/subscriptions/reset-daily-filtered`

请求头必须带 `Idempotency-Key`。请求体是单个 JSON 对象，只接收当前已应用的筛选值。筛选字段都可省略；`scope_version` 只有"无部门范围限制的全站请求且不指定部门"时可省略——部门/组织负责人等受限范围，以及指定了 `department_id` 的管理员请求，必须带最近一次成功列表响应返回的 `scope_version`：

```json
{
  "status": "active",
  "user_id": 123,
  "group_id": 456,
  "platform": "openai",
  "organization": "xunyou",
  "department_id": "12",
  "scope_version": "<最近一次成功列表响应的 scope_version>"
}
```

- 未知字段、尾随第二个 JSON、`null` 体、格式错误 → 400；页码、页大小、排序不进入请求体。
- 筛选非法 → 400，不按全部范围执行。
- 范围版本：受限范围或指定部门却缺 `scope_version`，或与当前范围版本不一致 → 409 `REPORT_SCOPE_CHANGED`（"department scope changed; refresh and retry"），客户端应重新拉列表后重试。`SubscriptionHandler.prepareDepartmentReset` 在幂等重放之前检查（经 `QueryScope` 的 `ExpectedVersion` 比较），`beginDepartmentSubscriptionWrite` 在写事务内再校验一次；全站管理员未传版本时不做此检查。
- 缺少幂等键 → `IDEMPOTENCY_KEY_REQUIRED`（即使全局 `observe_only` 开启）；幂等 coordinator 不可用 → fail-closed，不执行。
- 成功响应 `{"reset_count": N}`；无匹配返回成功且 `reset_count: 0`。同一个键重放返回第一次结果。

执行范围：无论传入什么 `status`，SQL 固定附加 `us.deleted_at IS NULL`、`u.deleted_at IS NULL`、`g.deleted_at IS NULL`、`us.status='active'`、`us.expires_at > now`；所以传 `status=expired` 等只会得到 0。覆盖全部分页结果。部门子管理员额外限定在其授权部门成员内（`beginDepartmentSubscriptionWrite` 计算成员 ID），并写部门审计 `department.reset_daily_filtered`。

## 4. 写入与缓存

- 仓储 `userSubscriptionRepository.ResetDailyFiltered`：一条 `WITH candidates AS (...) UPDATE user_subscriptions ... RETURNING user_id, group_id`，把 `daily_usage_usd` 清零、`daily_window_start` 设为服务端配置时区今天 0 点、刷新 `updated_at`；周/月用量和窗口不动。单条 SQL 原子执行，不返回逐条部分成功。
- `reset_count` 是返回行按 `(user_id, group_id)` 去重后的数量。未删除订阅上 `(user_id, group_id)` 有部分唯一索引（migration 016），所以它等于实际重置的订阅数。
- handler 用 `executeAdminIdempotentJSONAtomicSuccess`（scope `admin.subscriptions.reset_daily_filtered`，TTL 为 `DefaultWriteIdempotencyTTL()`），业务写入和幂等成功结果同事务提交。
- `SubscriptionService.AdminResetDailyFiltered` 通过 `DeferIdempotencyPostCommit` 把缓存失效登记到提交后执行（拿不到提交后钩子时立即失效）。失效由 `invalidateFilteredDailyResetCaches` 完成三段：`InvalidateSubCacheSync`（本机 L1）→ billing cache 删除 → 跨实例发布；默认 5 秒总预算，8 个 worker 并发。缓存失效失败只记日志，不能把已提交的写入说成未执行。

## 5. 前端

`frontend/src/views/admin/SubscriptionsView.vue` 与 `frontend/src/api/admin/subscriptions.ts`（`SubscriptionAdminFilters`、`SubscriptionOrganization`、`resetDailyFiltered`）：

- 组织筛选在状态、分组、平台筛选旁；请求值为空、`xunyou`、`wsdashi`、`other`。
- "一键重置日限"按钮在"分配订阅"之后，按 `canResetSubscriptionQuotas`（持有 `admin.subscriptions` 或 `admin.department_subscriptions`，完整管理员恒为真）显示；没有分配权限的子管理员也能看到。
- 列表成功返回后才记录 `appliedResetFilters`（含服务端返回的 `scope_version`）；加载中或失败时为空，按钮禁用。
- 打开确认框时复制 `appliedResetFilters` 为快照，并生成一次幂等键；确认时提交快照。确认框打开后再改页面筛选控件，不改变本次范围。
- 进行中禁用按钮并显示进行中文案；成功显示实际数量，0 条显示"没有可重置订阅"，然后刷新列表；普通失败保留确认框和快照，可复核后用同一个键重试。
- 部门范围失效（403 或 scope 变化）时关闭确认框、丢弃快照和键并重新加载；新查询必须重新建立快照和幂等键。
- 中英文文案在 `frontend/src/i18n/locales/{zh,en}/admin/channels.ts`（`bulkResetDaily*`）。

## 6. 关键约束与易错点

1. **Ent 的 `OrderExprFunc` 会丢掉表达式里 `Arg()` 绑定的参数。** 曾复现的失败场景（2026-08-12 热修复）：排序 CASE 里用 `.Arg(startOfDay)` 绑定今天 0 点，但经 `selector.OrderExprFunc(...)` 构造时参数没有并入外层查询的参数列表，占位符编号也不随 WHERE 参数递增；带筛选条件（如 `status=active` 占用 `$1`）时，排序仍引用 `$1` 或缺少时间参数，列表返回 HTTP 500。修复：改为 `selector.OrderExpr(entsql.ExprFunc(...))`，参数由外层 selector 统一编号。规则：ORDER BY 里需要绑定参数时只用 `OrderExpr(entsql.ExprFunc(...))`，并用 PostgreSQL 方言的查询生成测试断言"WHERE 参数在前、排序参数在后"（`TestUserSubscriptionAdminOrder_BindsStartOfDayAfterFilterArguments` 断言 `daily_window_start < $2` 且参数依次为 `active`、`startOfDay`）。
2. **排序必须在数据库分页之前。** 前端或单页排序会让第 2 页出现比第 1 页更紧张的订阅。
3. **有效日用量的归一化要与列表展示一致。** 跨日未清零的普通订阅按 0，一次性日卡按存储值；改其中一处必须同步排序 SQL、批量重置判断和列表展示。
4. **批量重置的 `active`/未过期/未删除条件写死在 SQL 里，不信任请求的 `status`。** 否则会重置历史、撤销或已过期记录。
5. **批量重置只碰日字段。** 周/月用量和窗口不变；单条"重置配额"仍是日/周/月三项，"重置日限"仍只传 `daily=true`。
6. **缓存失效只能在提交后执行，且必须三段齐全。** 单条 `AdminResetQuota` 的失效缺少跨实例发布，不能作为参照；用户自助重置也复用这里的 `invalidateFilteredDailyResetCaches`，改动它会同时影响两个功能。
7. **确认框必须冻结筛选快照和幂等键。** 网络重试复用同一个键，避免重复执行和误导性计数；新的查询结果必须换新快照、新键。
8. **组织值不能静默回退为全部。** 未知非空值 400，否则会把"只重置迅游"变成"重置全站"。
9. **`bulk-action` 不是本功能的入口。** 上游的 `POST /admin/subscriptions/bulk-action` 只给完整管理员，子管理员白名单不增加该路由；本地 `reset-daily-filtered` 的 AtomicSuccess fail-close 与提交后失效不因上游批量能力改变。

## 7. 代码地图

| 层 | 位置 |
| --- | --- |
| 筛选合同 | `backend/internal/service/subscription_admin_filter.go`：`SubscriptionAdminFilter`、`NormalizeSubscriptionAdminFilter`、`SubscriptionOrganizationXunyou/Wsdashi` |
| 服务 | `backend/internal/service/subscription_service.go`：`AdminResetDailyFiltered`、`invalidateFilteredDailyResetCaches`、`invalidateFilteredDailyResetCachesWithTimeout` |
| 仓储 | `backend/internal/repository/user_subscription_repo.go`：`userSubscriptionAdminOrder`、`ResetDailyFiltered`；`department_subscription_read.go`：`ListAdmin`；`department_subscription_scope.go`：`dailyResetPredicates`、`beginDepartmentSubscriptionWrite` |
| Handler | `backend/internal/handler/admin/subscription_handler.go`：`List`、`ResetDailyFiltered`（`ResetDailyFilteredRequest`/`Response`） |
| 路由与权限 | `backend/internal/server/routes/admin.go` 的 `registerSubscriptionRoutes`；`backend/internal/service/admin_permission.go`（`admin.subscriptions` 与 `admin.department_subscriptions` 都允许 `POST /admin/subscriptions/reset-daily-filtered`） |
| 前端 | `frontend/src/views/admin/SubscriptionsView.vue`（`openResetDailyFilteredConfirm`、`confirmResetDailyFiltered`、`appliedResetFilters`）、`frontend/src/api/admin/subscriptions.ts` |

## 8. 验证

在 `backend/` 下：

```bash
go test -tags=unit -count=1 ./internal/service -run 'TestNormalizeSubscriptionAdminFilter|TestAdminResetDailyFiltered|AdminPermission|SubAdminWrite'
go test -tags=unit -count=1 ./internal/repository -run TestUserSubscriptionAdminOrder_BindsStartOfDayAfterFilterArguments -v
go test -count=1 ./internal/handler/admin -run 'TestSubscriptionHandler_(List|ResetDailyFiltered)'
# 真实 PostgreSQL（Docker 或 SUB2API_POSTGRES_ONLY_INTEGRATION_DSN 指向新建空库）；跳过不算通过
go test -tags=integration -p 1 -count=1 ./internal/repository -run 'TestUserSubscriptionRepoSuite/(TestListAdmin|TestResetDailyFiltered)' -v
```

在 `frontend/` 下：

```bash
pnpm exec vitest run src/views/admin/__tests__/SubscriptionsView.spec.ts
pnpm typecheck
```

覆盖要点：
- 仓储集成：组织域名精确且忽略大小写、子域名不匹配（`TestListAdmin_FiltersOrganizationByExactCaseInsensitiveDomain`）；跨页前按比例排序（`TestListAdmin_SortsGloballyBeforePagination`）；负剩余钳为 0、跨日用量按 0、无限额置后（`TestListAdmin_UsesEffectiveDailyUsageAndSortsUnlimitedLast`）；相同比例按次排序再按 ID（`TestListAdmin_UsesSecondaryOrderThenStableID`）；批量只重置匹配的生效订阅（`TestResetDailyFiltered_UpdatesOnlyMatchingActiveSubscriptions`）；零匹配（`TestResetDailyFiltered_ZeroMatchReturnsEmptyKeys`）。
- Handler：参数校验、未知字段/尾随 JSON/null 体、计数响应、同键重放、`observe_only` 下仍要求键、无 coordinator 时 fail-closed。
- 前端：组织值和已应用筛选的传递、按钮位置与权限、确认时冻结快照、重复点击受控、成功/零匹配/失败反馈；单条重置日限仍只传 `daily=true`。

最近结果：2026-11-04（HEAD `26f6363af`，本机）：`-tags=unit` 下 `TestNormalizeSubscriptionAdminFilter`、`TestAdminResetDailyFiltered*`、`TestFilteredDailyResetCache*`、`TestUserSubscriptionAdminOrder_BindsStartOfDayAfterFilterArguments`、`TestSubscriptionHandler_*` 通过；前端与仓储集成未重跑。2026-08-12 交付与热修复时的逐条结果未单独留存。2026-11-04 组织负责人功能的全量 `go test -tags=unit ./...`（61 个包）与嵌入式 PostgreSQL 集成筛选通过（见组织负责人文档 §15.3），该集成筛选不含 `TestUserSubscriptionRepoSuite`。

## 9. 状态与待办

- [ ] 在真实 PostgreSQL 上补跑一次 `TestUserSubscriptionRepoSuite/(TestListAdmin|TestResetDailyFiltered)` 并在本节记录结果。
- 已否决：只对当前页做前端排序，原因：结果跨页不一致（§6 第 2 条）。
- 已否决：把组织展示名写入 API 或数据库，原因：API 只用组织码，展示名由前端 i18n 决定。
