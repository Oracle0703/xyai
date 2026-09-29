# 组织、部门与多平台用量报表：设计落地审核

日期：2026-09-28。审核对象：[设计文档](organization-department-usage-design-cn.md)（2026-09-20 版，§1–§15）。代码基线：`main` = `feature/hy/10207_department_usage` = `273b76c35`。部门功能提交为 `a9c53a9f6`、`9834ba72a`、`927c164d9`；之后合入了上游 0.2.8、10208（子管理员分配订阅）和 10209（GPT 额度展示）。

本轮只读审核，没有修改业务代码。

## 1. 结论

设计的主体合同**已全部落地**，没有发现越权写入、跨组织或跨部门数据读取、撤权后回退全站等 P0/P1 问题。已核实成立的机制：
- 空授权、省略筛选或传 `all` 时一律 fail-closed；
- 报表权限与部门订阅权限拆分，并与全站订阅权限互斥；
- 统一的权限版本 CAS；
- 降级、软删除时清理授权；
- 邮箱跨组织变更由数据库触发器兜底；
- `catalog_version`、`admin_access_version`、`scope_version` 三种版本分离；
- 重置在锁内重新鉴权，并保留幂等与缓存失效；
- Excel 共 8 个 Sheet，并有 10 万行总预算。

但设计**并非全部按合同实现**，也有部分文档已经失真：

| 级别 | 数量 | 含义 |
| --- | ---: | --- |
| 合并阻断（功能外） | 1 | `main` 的 `-tags=unit` 构建失败，CI 单测步骤会红 |
| P2 | 4 | 明确的设计合同未满足，或权限描述失真；常规操作或管理员直连 API 即可触发 |
| P3 | 21 | 口径或错误码偏差、性能与锁范围、界面缺口、测试缺口；不扩大访问范围 |
| 文档不一致 | 16 | 设计、实施、验收、代码审核文档或 wiki 与 HEAD 不符 |

上一轮[实现审核](organization-department-usage-implementation-audit-cn.md)留下的 3 项在 HEAD **全部仍未修复**（见下文 P2-2、P3-1、P3-2）。

## 2. 审核方法（三轮）

| 轮次 | 内容 |
| --- | --- |
| 第 1 轮 | 按设计章节分 4 个区域逐条对照源码：A 数据模型与部门管理，B 权限、CAS、生命周期与导航，C 报表与 Excel，D 部门订阅查询与重置。同时在 HEAD 上运行相关测试 |
| 第 2 轮 | 对第 1 轮全部结论做对抗复核（后端 19 条、前端 11 条），并做补漏排查。2 条被推翻（keep-alive 忽略深链参数、部门页没有创建入口），5 条降为部分成立，新增 1 条 P2（N1 → 本文 P2-1）和若干 P3 |
| 第 3 轮 | 本人逐条复核 P2 和上轮遗留项的源码证据（行号见下），核对 wiki 与文档状态，并汇总成文 |

## 3. 设计覆盖矩阵

| 设计章节 | 结论 | 偏差 / 缺口（编号见第 4 节） |
| --- | --- | --- |
| §1 方案摘要、§2 边界 | 已实现 | 10208 合入后，“子管理员订阅权限不开放分配”一句已失真（P2-3） |
| §4.1 / 4.1.1 / 4.1.2 页面 | 已实现 | 用户管理只有部门列，没有组织列；改邮箱、降级都没有“将清空部门/授权”的提示；深链参数没有测试（P3-19） |
| §4.2 部门生命周期 | 已实现（邮箱变更由 239/240 触发器覆盖所有写入口） | 撤掉两项部门权限后授权仍保留，重新授权会复活（P2-2） |
| §4.3 报表交互 | 已实现 | — |
| §5.1 / 5.2 数据模型、索引、事务 | 已实现 | 重复 ID 被拒绝而不是去重；转岗逐人往返（P3-13） |
| §6 权限与查询流程 | 已实现（每个白名单路由都追到了仓储范围） | 部门模式的订阅响应返回完整用户 DTO（P2-1） |
| §6.1 额度重置 | 已实现 | 进度接口返回 404（P3-1）；新候选被冻结而不是返回 409（P3-5）；前端重置遇 403/409 处理不全（P3-6） |
| §6.2 权限并发合同 | 已实现 | `SetAccess` 省略 `department_ids` 时不清授权（P2-2） |
| §6.3 管理员查询优化 | 已实现 | 管理员带部门筛选的批量重置会锁住全部候选；订阅列表逐 ID 绑定参数（P3-11、P3-12） |
| §7.1 – 7.3 指标、多平台、调岗口径 | 已实现 | 缺“平台按当前配置归类”的说明；平台目录三处硬编码（P3-9） |
| §7.4 查询与导出一致性 | 部分实现 | 导出没有沿用页面的 `as_of`（P2-4）；管理员全站版本把空部门也算进去（P3-10） |
| §8.1 管理接口 | 已实现 | 部门 PUT 为全量覆盖（P3-4）；`GET /admin/departments/members` 未写进设计（D4） |
| §8.2 报表接口 | 已实现 | 分页溢出（P3-2）；`organization` 大写返回 400（P3-3） |
| §8.3 订阅接口 | 已实现 | `search-groups` 含已软删除分组（P3-7）；`search-users` 兼作全站分配搜索，设计未写（D10） |
| §8.4 错误码 | 已实现（9 个码都有 i18n） | “成员或目标已消失”被报成 403 `DEPARTMENT_SCOPE_DENIED`（P3-8） |
| §9 后端、前端、Excel | 已实现（8 个 Sheet、元信息、Worker、取消、10 万行预算） | 每页重复加载全部成员并重算汇总（P3-14） |
| §10 初始化与回退 | 迁移已实现，前向兼容 | 没有“撤销新权限码”的回退工具（P3-21）；状态描述过期（D1） |
| §11 验收与验证 | 本轮复跑见第 6 节 | 缺 handler 级部门重置、批量重置并发、进度接口、前端 403/409 等测试（P3-20） |

## 4. 问题与修改意见

### 4.0 合并阻断（非本设计代码，但阻断本功能的单测验证）

**B-1　`go test -tags=unit ./internal/service` 编译失败**
- 证据：`backend/internal/service/gpt_quota_display_test.go:287` 没有 build tag，定义了 `func ptrFloat`；`backend/internal/service/payment_config_plans_validation_test.go:137`（`//go:build unit`）定义了同名函数。报错为 `ptrFloat redeclared in this block`。前者来自 `88afc6589`（10209）。
- 影响：CI `backend-ci.yml` 运行 `make test-unit`，即 `go test -tags=unit ./...`，所以 `main` 的单测步骤会失败，service 包中所有部门相关单测都跑不起来。2026-09-29 已由 GitHub Actions run `36439528136` 证实：单测步骤因此失败，集成测试步骤被跳过。修复后还剩既有失败 `TestApplyDefaultOpenAIReasoningEffort`，见[对比裁决](feature-10207-release-audit-adjudication-cn.md)。
- 建议：删掉或重命名 `gpt_quota_display_test.go` 里重复的 helper（例如改成 `ptrFloatGPT`），并补跑 `make test-unit`。

### 4.1 P2

**P2-1　部门负责人的订阅响应包含授权外用户和财务字段**
- 证据：
  - 部门模式下的列表、详情和重置返回都走 `dto.UserSubscriptionFromServiceAdmin`（`backend/internal/handler/admin/subscription_handler.go:133,161,393,430`）。
  - 它为 `user` 和 `assigned_by_user` 都调用 `UserFromServiceShallow`（`backend/internal/handler/dto/mappers.go:882-892,912`）。
  - 输出字段包括 `role`、`balance`、`frozen_balance`、`total_recharged`、`balance_notify_extra_emails`、`allowed_groups`、`last_active_at`（`mappers.go:12-36`）。
  - 仓储预加载了 `WithAssignedByUser()`（`backend/internal/repository/user_subscription_repo.go:76,374`）。
- 影响：负责人能看到分配人的邮箱、余额和累计充值。分配人通常是部门之外的完整管理员，这属于授权外对象的信息。负责人还能看到成员的余额、充值和通知邮箱，超出 §6.1“查看订阅、额度进度”，也违背 §6“负责人只用 compact 接口，不调用完整 `/admin/users`”。全站订阅子管理员原本就有这种行为，不算本功能的回归；但部门负责人这个受限角色是新引入的暴露面。
- 建议：非全站模式（持有 `admin.department_subscriptions`）改用精简 DTO：`user` 只保留 `{id,email,username,status}`，`assigned_by_user` 省略或只保留 `{id,email}`，`group` 只保留 `{id,name,platform}`。补一个 handler 测试，断言部门模式的响应不含财务字段。

**P2-2　部门授权的生命周期不闭合，休眠授权可能被静默复活（含上轮第 3 项）**
- 证据：
  1. `SetAccess` 不把 nil 规范化（`backend/internal/service/department_service.go:367-383`）。仓储执行 `DELETE ... NOT (department_id=ANY($2))`，参数是 `pq.Array(nil)`，即 SQL NULL，一行也不删（`backend/internal/repository/department_repo.go:499`）。而权限开关 `report`/`reset_quota` 缺省为 false，仍会按替换语义改写（`:469-496`）。
  2. 只有角色离开 `sub_admin` 时才清授权（`backend/internal/repository/user_repo.go:367-371`）。在用户编辑里去掉两项部门权限，授权会继续保留。
  3. 负责人弹窗在账号没有部门权限时，默认两项都勾选（`frontend/src/components/admin/department/DepartmentAccessDialog.vue:91-93`），保存时提交已有授权与当前部门的并集（`:103-108`），但界面不列出其他部门。
- 影响：管理员从部门 X 重新授权时，之前遗留的 Y、Z 部门访问范围会被一并恢复。这与 §4.2“重新提权不恢复旧授权”的原则相反。Admin API 或脚本省略 `department_ids` 时，会产生“权限已撤、授权还在”的状态。部门列表的“负责人”列只按 `role='sub_admin'` 统计（`department_repo.go:192`），休眠持有人也会显示为负责人。
- 建议：
  - `department_ids`、`report`、`reset_quota` 设为必填且不能为 null（用 RawMessage 或指针检测），或者把 nil 视为 `[]`。
  - 两项部门权限都被移除时（包括用户编辑和 SetAccess），在同一事务里清空授权并写审计。
  - 弹窗列出该账号的全部已授权部门，并允许逐个撤销；账号没有部门权限时，两项默认不勾选。
  - 负责人列只统计持有部门权限码的账号。
  - 补 PG 用例，覆盖省略、`null`、`[]`、撤权后重新授权四种情况。

**P2-3　10208 合入后，全站订阅权限变成“全站分配”，设计和权限文案都失真**
- 证据：
  - `backend/internal/service/admin_permission.go:65-68` 为 `admin.subscriptions` 加了 `assignable-groups`、`assign`、`bulk-assign`。
  - 分配服务对子管理员没有额外限制：可以给自己分配，可以续期已过期订阅，只排除软删除分组、不检查分组启用状态，最长 36500 天（`backend/internal/service/subscription_service.go:507-545`）。
  - 设计 §6.2 明确允许“部门报表 + 全站订阅”的组合，SetAccess 在不勾重置时会保留全站订阅权限。
  - 权限描述仍写“查看订阅，并可重置全部配额或仅重置日限”（`frontend/src/i18n/locales/zh/admin/overview.ts:636`，en 同义）。
  - 负责人弹窗只提示“另有全站权限：订阅管理”。
  - 在用户编辑里从部门订阅切到全站订阅，只是互斥勾选，没有确认（`frontend/src/components/admin/user/UserEditModal.vue:168-172`）。
- 影响：纯部门负责人（`admin.department_subscriptions`）仍然无法分配，这一点在后端白名单和前端都已核实。但 10208 之前授予的全站订阅子管理员被静默扩权；“报表负责人 + 全站订阅”账号可以给任何人（包括自己）分配任意订阅组。管理员依据现有文案会低估这项授权。
- 建议：
  - 立即更新中英文权限描述，以及设计 §1、§2、§4.1、§6.1、§6.2、§13 的表述；弹窗明确写出“含全站分配订阅”。
  - 切换到全站订阅时增加确认。
  - 作为 10208 的治理项：禁止子管理员给自己分配，分配时校验 `group.status='active'`，考虑把分配拆成独立权限码，并排查现有 `admin.subscriptions` 持有者。

**P2-4　导出没有沿用页面快照的 `as_of`（§7.4 合同未满足）**
- 证据：`frontend/src/views/admin/OrganizationUsageView.vue:534` 调用的是 `currentQuery()`，没有传 `snapshotAsOf`。`frontend/src/api/admin/organizationUsage.ts:262` 于是用 `query.as_of ?? new Date().toISOString()`。`:292` 的 `fixedSnapshot` 只有同时传入 `as_of` 和 `scope_version` 时才生效，页面从不满足这个条件。后端 `scope_version` 不含 `as_of`，所以会接受这个新时间。页面导出测试用的是 `objectContaining` 断言，发现不了这个问题。
- 影响：10:00 打开报表、10:30 导出，Excel 本身前后一致，但包含了这 30 分钟新增的用量，和页面数字对不上。[代码审核](organization-department-usage-code-review-cn.md)第 12 行宣称已“严格核对时间”，从端到端看并不成立。此外，报表加载失败时导出按钮依然可用。
- 建议：只允许在报表已成功加载时导出，并传入 `as_of: snapshotAsOf.value` 和 `scope_version`；补一个页面级断言，要求导出首个请求的 `as_of` 等于页面快照。

### 4.2 P3

| 编号 | 问题 | 证据 | 修改意见 |
| --- | --- | --- | --- |
| P3-1 | 进度接口把所有错误改写成 404（上轮第 1 项，仍在） | `backend/internal/service/subscription_service.go:1257-1260` | 原样返回 `GetByID` 的错误；补单测，覆盖越权 403 和数据库错误 |
| P3-2 | 组织用量分页溢出（上轮第 2 项，仍在）：只拒绝 `page<1`，`(page-1)*pageSize` 可能溢出成负 OFFSET，导致 500 | `organization_usage_service.go:472-477`；`organization_usage_repo.go:80,181` | 复用部门列表的溢出守卫，返回 400 |
| P3-3 | `organization=XUNYOU` 被拒为 400：传给 `Filters.Normalize` 的是原始值 | `organization_usage_service.go:261,312,355` 对照 `:465` | 统一传入已转小写的组织值；平台非法时单独给出错误信息 |
| P3-4 | 部门 PUT 为全量覆盖：省略 status 会把停用部门恢复为 active，省略 sort_order 会重置为 0；组织不一致返回 409 而不是 400 | `department_service.go:335-337`；`department_repo.go:232-240` | status/sort_order 改为指针，缺省表示不改；或者要求必填。并更新 §8.1 |
| P3-5 | 筛选批量重置遇到“未锁定的新候选”时静默跳过，仍返回 200，与 §6.1“返回范围变化”不一致 | `department_subscription_scope.go:215-223`（注释 “Freeze the locked candidate population”） | 二选一：加锁后用同一组谓词再查一次候选，发现新成员就返回 409；或者把冻结语义写进设计和 wiki |
| P3-6 | 订阅页两个重置路径遇到 403/409 都只提示通用失败：403 时不清受保护数据；409 时确认框、快照和幂等键都不变，用户会反复收到 409 | `SubscriptionsView.vue:1716-1722,1771-1775`；列表路径 `:1355-1366` 已正确处理 | 复用列表路径的清理逻辑；409 时关闭确认框、清空快照和幂等键并重新查询；网络错误仍保留幂等键 |
| P3-7 | `search-groups` 没有过滤已软删除的分组（功能上线前走 Ent 查询，会自动排除） | `department_repo.go:589-591` | 加 `g.deleted_at IS NULL` |
| P3-8 | `lockDepartmentUsers` 发现行数不符时一律返回 403 `DEPARTMENT_SCOPE_DENIED`：成员被删、SetAccess 目标不存在、候选成员并发删除时，完整管理员也会看到“无权访问” | `department_repo.go:95-97`，调用方 `:295,:411`、`department_subscription_scope.go:188`、`user_admin_access.go:48` | 返回专门的“目标缺失”错误，由各调用方映射为 409/404/400；`CONFLICTING_ADMIN_PERMISSIONS` 等补中文文案（目前显示英文原文，`UserEditModal.vue:281`） |
| P3-9 | 平台口径：缺少 §7.2 要求的“按当前分组/账号配置归类，非历史快照”说明；平台白名单在后端、前端筛选、常量三处各写一份，标签不一致；目录外的非空平台值会原样显示，而不是归入 `unknown` | `organization_usage_department.go:43`；`OrganizationUsageFilters.vue:220`；`organizationUsageReport.ts:185-195` | 界面和 Excel 概览补上说明；前后端从同一目录派生（注意保留 `kiro` 和 `unknown`）；SQL 把目录外的值映射为 `unknown` |
| P3-10 | 管理员全站（all/all）的 `scope_version` 包含全部部门的名称：改名或新建一个空部门，也会让进行中的导出收到 409 | `department_query_scope.go:144-153` | 只摘要被成员引用、或会出现在输出里的部门 |
| P3-11 | 锁范围过大：`FOR UPDATE` 会阻塞 `usage_logs`/`api_keys` 外键的 KEY SHARE（usage_log 为异步批量写，影响的是整批写入延迟）；管理员按部门或“未分配”筛选的批量重置会锁住全部候选用户 | `department_repo.go:79`；`001_init.sql:89,136`；`department_subscription_scope.go:145,164-188` | 改为 `FOR NO KEY UPDATE`（写者之间仍然串行）；管理员路径不加成员锁，在单条 UPDATE 里带上部门谓词；补并发回归 |
| P3-12 | 部门订阅列表通过 Ent `UserIDIn(ids...)` 把每个成员 ID 绑定成一个参数，超过 65535 个时查询直接失败（例如管理员查 `other`/“未分配”） | `department_subscription_scope.go:29-42` | 改用 `= ANY($1::bigint[])` 或 SQL 子查询（报表和重置路径已经这样做） |
| P3-13 | 批量转岗在锁内逐人 SELECT + UPDATE：200 人约 400 次往返，同时持有 201 行锁 | `department_repo.go:313-336` | 一次查询取出所有成员，再用 `UPDATE ... FROM unnest(...)` 批量写入；审计不变 |
| P3-14 | 每个 Summary/Periods/Trend 请求都把范围内全部成员装进 Go 做哈希；导出每页 500 行，每一页都重算组织汇总和部门/平台汇总；完整管理员不带筛选时也会全量加载 | `department_query_scope.go:106,124,155-163`；`organization_usage_repo.go:34`；`organization_usage_department_repo.go:144` | 管理员无筛选时在 SQL 内选人；导出续页跳过汇总；评估设计 S3 备选“在 SQL 内计算摘要”；按生产规模压测 |
| P3-15 | 前端重复请求：用户管理每次加载列表都重新拉取带成员计数的部门目录；订阅页完整管理员每次加载也先串行请求一次 `/subscriptions/scope` | `UsersView.vue:1693`；`SubscriptionsView.vue:945,1296-1300` | 缓存部门目录，在组织切换或转岗后再失效；提供一个不带计数的轻量名称接口 |
| P3-16 | 单项重置不广播跨实例 L1 缓存失效（筛选重置和自助重置会广播）。属上游既有行为 | `subscription_service.go:929-932` 对照 `:177-192` | 改为调用 `invalidateSubscriptionCaches` |
| P3-17 | `SearchSubscriptionAssignmentUsers` 没有挂路由，但会列出全站用户（含停用用户），还能匹配 API Key 子串；wiki 描述的是它，而不是实际挂路由的实现 | `subscription_assignment_options.go:11-33`；`llm-wiki/wiki/backend.md:231` | 删除该 handler 及其测试，防止被误接到部门白名单路由上 |
| P3-18 | 审计缺改前值：部门编辑只记录 input，不含旧的 name/status/sort_order/version | `department_repo.go:247` | 更新前先 `SELECT ... FOR UPDATE` 取旧值写入审计 |
| P3-19 | 界面缺口：用户管理没有组织列；改邮箱跨组织、取消子管理员角色时，都不提示部门/授权会被清空；报表深链参数没有测试；导出遇 403/409 后没有清 scope 和快照；当前选择失效后“重试”会循环 | `UsersView.vue`；`UserEditModal.vue`；`OrganizationUsageView.vue:380-393,566-569` | 逐项补上；报表页 403 时参照订阅页，回到授权默认范围 |
| P3-20 | 测试缺口 | handler 部门重置（撤权重放 403、范围变化 409）；批量重置与转岗或新分配并发；进度接口；前端重置 403/409；SetAccess 传 nil；backend mode 下仅部门权限账号的落地页；页面导出的 `as_of` | 按 P2/P3 修复逐项补齐 |
| P3-21 | 回退缺少工具：旧版本遇到未知权限码，任何用户编辑都会返回 400；现有界面也不能按权限码筛选用户 | 设计 §10；`de5a3e383:admin_user.go:278` | 提供回退前检查用的 SQL（列出持新权限码的账号）和一键撤销脚本，写进 ops |

另有一个待产品确认的问题：负责人如果本人就属于某个已授权部门，可以用管理端重置不限次数地重置自己的订阅，绕过自助重置每订阅每日 1 次的上限（`department_subscription_scope.go:145-167` 没有排除操作者本人）。有 `department.reset_quota` 审计留痕。如果需要限制，可以禁止 sub_admin 以自己为目标，或者把这类重置计入自助次数。

## 5. 文档与代码不一致

| 编号 | 位置 | 问题 | 处理建议 |
| --- | --- | --- | --- |
| D1 | 设计 `:3,:526,:650`；实施方案 `:3`；验收 `:3,:12`；wiki README | 写的是“未推送、未部署、未合回 main”，实际已合入 `main` 并推到 `github/main` | 改为“已合入 main（`273b76c35`）”；部署状态以运维记录为准 |
| D2 | 设计 §1、§2 `:35`、§4.1 `:90`、§6.1 `:245,:258`、§13 | 10208 之后，全站订阅子管理员可以分配订阅 | 与 P2-3 一起修正，注明“负责人不能分配”只针对部门订阅权限 |
| D3 | 设计 §8 `:381` 对照 §8.4 `:456` | 一处说先读 `reason`，一处说“以 code 为准”，前后矛盾 | 改成“以业务错误码（`reason` 或字符串 `code`）为准，不以 HTTP 状态为准”；代码已按 reason 优先实现 |
| D4 | §8.1 | 漏写 `GET /admin/departments/members`（前端实际只用它）和列表的 `q` 参数；PUT 实际是全量覆盖 | 补齐接口；PUT 语义与 P3-4 保持一致 |
| D5 | §5.2 | 写的是“ID 去重”，实际拒绝重复 ID；`expected_*` 非必填，缺省等价于“未分配、版本 0” | 按实现改写 |
| D6 | §6 边界表、§8.4 | 未列出 `DEPARTMENT_INVALID`(400) 和 `DEPARTMENT_NOT_FOUND`(404)；管理员请求不存在的部门返回 404，不是 400 | 补进错误码表 |
| D7 | §5.1 “新增组织改动清单” | 漏了 `subscription_self_reset.go` 和 `user_subscription_repo.go` 两处邮箱→组织映射（后端共 5 处实现加 1 个触发器，目前一致） | 补进清单；建议收敛成一个函数，并加一致性测试 |
| D8 | §4.1 对照 §9.2 | 一处写“组织/部门列”，一处只写“部门列” | 与 P3-19 的取舍保持一致 |
| D9 | §6.1 `:264`；`llm-wiki/wiki/backend.md:15` | 写“避免重复全量解析”“新候选触发范围变化”；实际预检加锁内共解析两次，新候选被冻结 | 与 P3-5 的决定保持一致 |
| D10 | §8.3 | 没写 `search-users` 还承担 `admin.subscriptions` 的全站分配搜索（active 用户、按 id 排序、空查询返回前 30 条） | 补写 |
| D11 | §7.4 流程图 | 写“首个 Summary 返回 as_of”，实际只有请求带了 `as_of` 才会回显（前端总会带） | 按实现改写 |
| D12 | [代码审核](organization-department-usage-code-review-cn.md) `:12-13` | 宣称导出“严格核对时间”、订阅查询“保留其它真实错误”，但 P2-4 和 P3-1 都不成立 | 修复后回写，或者注明例外 |
| D13 | [实现审核](organization-department-usage-implementation-audit-cn.md) | 3 项遗留问题在交付文档里没有标记为“仍未关闭” | 链接到本文 |
| D14 | `llm-wiki/wiki/backend.md:229,231` | 只列了 3 个权限码；`search-users` 的来源、排序和空查询行为描述错误 | 本轮已修正（见第 8 节） |
| D15 | §8.1 `:397` | 把“组织不匹配”列为 SetAccess 的校验项，但该接口没有组织参数，授权本身允许跨组织 | 删除该项 |
| D16 | §10 / `data-and-domain` | `239_departments.sql`、`240_department_email_change_audit.sql` 与上游 `239_channel_…`、`240_affiliate_…` 编号相同。运行器按文件名字典序执行、以文件名为主键，两组互不依赖，确认安全 | 补一句说明，避免以后误判 |

## 6. 本轮验证证据

环境：HEAD `273b76c35`，Windows 本机，Go toolchain 1.27.0，内嵌 PostgreSQL 16.9（CI 使用 PG 18.1，本轮没有验证 18）。日志在仓库外的 `E:/tmp/verify-10207/`。

| 命令（后端都带 `-p 1 -count=1`） | 结果 |
| --- | --- |
| `go build ./...`；`go vet`（默认 tag，service/repository/handler/admin/server） | 通过 |
| 默认 tag 的相关单测（`-run 'Department\|OrganizationUsage\|Subscription\|AdminPermission\|AdminAccess\|AdminUser\|AdminAuth\|SubAdmin\|UserService\|EmailChange\|Role'`） | 通过：service 93、repository 15、handler/admin 36、middleware 16、routes 2，0 失败 0 跳过 |
| 同上加 `-tags=unit` | **失败**：service 包因 B-1 构建失败，0 个测试执行；其余包（repository 16、handler/admin 37、middleware 20、routes 2）通过。临时屏蔽重复 helper 后，service 160 个全部通过，但这不计入原命令结果 |
| 真实 PG 专项（ops.md 列出的 6 组正则，`-tags=integration`） | 12/12 通过，0 跳过 |
| 自助重置 PG 专项 | 3/3 通过 |
| `TestDepartmentAdminPerformanceIntegration`（开启开关） | 通过 |
| `TestDepartmentUsagePerformanceIntegration`（开启开关） | 首跑 11/12 通过，`366/all` p95 为 3736 ms，超过 3000 ms 门槛而失败；单独重跑 p95 为 2114 ms，通过。属于环境敏感的阈值，建议在 CI 同规格环境复测 |
| Vitest：部门、报表、订阅、用户编辑、权限、路由相关 26 个 spec | 26 个文件 / 210 个测试全部通过 |
| `pnpm run typecheck` | 通过 |
| 未执行 | 浏览器、HTTP 和 Excel 人工验收，Go 与 Vitest 全量，lint |

## 7. 合理性评价

**设计合理、应保留的部分**
- 采用当前成员口径，适合日常团队管理；这个口径已在界面和 Excel 中明示。
- 权限按报表和订阅拆分，并与全站订阅权限互斥；每次请求都重新读库；空授权时拒绝访问，不回退全站。这是本功能最关键的安全底座，实现扎实。
- 邮箱跨组织变更由数据库触发器在同一事务里清空归属并写审计，覆盖了所有写入口，比在应用层逐个入口兜底更可靠。
- 三种版本职责分离，统一的权限 CAS 加上“先锁行、再重读”，避免了旧编辑窗口把权限写回去。

**可以商榷、建议调整的部分**
- **`scope_version` 的实现代价**：每个请求都把全部成员加载到 Go 里做哈希，成本是 O(成员数)。导出时每页都要重做一遍，而且完整管理员不带筛选时也不短路（P3-14）。建议管理员路径短路，负责人路径评估在 SQL 内计算摘要。
- **锁的粒度**：统一的 `FOR UPDATE` 加上管理员筛选重置会锁全部候选（P3-11），规模上去后会和计费写入、日志写入互相阻塞。可以保留行锁协议，但应降为 `FOR NO KEY UPDATE`，管理员路径改为不加成员锁。
- **与后续功能的耦合**：10208 在不改部门设计的情况下，改变了“部门报表 + 全站订阅”组合的实际能力（P2-3）。今后新增子管理员能力时，应同步检查部门设计里的权限组合表。
- **目录重复**：组织映射有 5 处实现加 1 个触发器，平台目录有 3 份列表（P3-9、D7），扩展时容易漏改。

## 8. 建议修复顺序

1. B-1：修复 `ptrFloat` 重复定义，恢复 `make test-unit`。
2. P2-1：部门模式的订阅响应改用精简 DTO。
3. P2-2：授权生命周期闭合（处理 nil、撤掉部门权限时清授权、弹窗列出全部授权部门）。
4. P2-3：更新权限文案和设计；为 10208 补充治理（禁止给自己分配、校验分组启用、考虑独立权限码）。
5. P2-4：导出沿用页面快照。
6. P3 中的错误语义批次：P3-1、P3-2、P3-3、P3-4、P3-6、P3-7、P3-8；并对 P3-5 做出取舍（报 409 还是冻结）。
7. P3 中的性能与锁批次：P3-11 到 P3-15，按生产规模压测后决定。
8. 按第 5 节同步设计、实施、验收、代码审核文档和 wiki。

本轮已修正的 wiki：`llm-wiki/wiki/backend.md`（权限码清单、`search-users` 的实际实现、批量重置对新候选的冻结语义）和 `llm-wiki/wiki/README.md`（合入状态和本审核入口）。其余文档按上表由后续修复轮次处理。
