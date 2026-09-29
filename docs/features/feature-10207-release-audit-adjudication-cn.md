# `feature/hy/10207_department_usage` 两份审核的对比裁决与修复建议

日期：2026-09-29。基线：`main` = `feature/hy/10207_department_usage` = `273b76c35`（版本 0.2.8）。

对比对象：
- **Codex 报告**：`docs/reviews/2026-09-29-feature-10207-release-audit.md`，整分支上线门禁审核，结论为 NO-GO。该文件位于被 `.gitignore` 排除的目录，只存在于本机。
- **Claude 报告**：[设计落地审核](organization-department-usage-design-implementation-audit-cn.md)，逐条对照部门设计与源码。

裁决时补充了以下复核证据：GitHub Actions 运行记录、本机前端 Vitest 全量复跑、带 `-overlay` 的 unit 定向复跑，以及上游 0.2.9 的差异统计。

## 1. 最终裁决

**维持 NO-GO：当前不部署生产。** 结论与 Codex 相同，但阻断理由不同：

1. **真正的阻断是 CI 测试任务为红，且真实数据库集成测试从未在 CI 上执行。**
   - `main` 的 CI 从 09-21 起持续失败。最新一次（run `36439528136`，`273b76c35`）中，`Unit tests` 因 `ptrFloat redeclared` 编译失败，于是 `Integration tests` 被跳过。
   - 部门功能的真实 PostgreSQL 用例只在本机 PG16 上跑通过，CI 上从未执行。
   - 修掉 `ptrFloat` 后还剩一个既有失败 `TestApplyDefaultOpenAIReasoningEffort`，本轮用 overlay 复跑，HEAD 上仍然失败。
2. Codex 列出的 P0 以及三个 P1 中，**有两项不成立或已满足**：
   - 前端 unhandled error 本轮全量复跑没有复现；
   - govulncheck 已在 CI Security Scan 中通过。
   另有两项是“特定场景门禁”，不是部署阻断：
   - GPT 真实上游验收：该功能默认关闭；
   - `sh.exe`：属于本机环境问题。
3. Codex **漏掉了**这些问题：`-tags=unit` 编译失败（它只跑了默认 tag），以及 Claude 报告中的 P2-1、P2-2 的主体部分、P2-3、P2-4。这些问题经三轮复核均成立。

## 2. 两份报告的定位差异

| 维度 | Codex | Claude |
| --- | --- | --- |
| 范围 | 整分支上线：部门、自助重置、GPT 额度、上游 0.2.8 | 部门设计（§1–§15）逐条对照实现 |
| 方法 | 构建、lint、全量测试、门禁清单 | 4 个区域审核、对抗复核、逐条复核源码，加定向测试和本机 PG |
| 长处 | 覆盖 GPT 真实上游、安全扫描、上游版本策略等流程门禁 | 发现设计合同偏差、数据暴露、授权生命周期、合并后权限漂移 |
| 盲区 | 没跑 `-tags=unit`，没查 CI 实际结果，没做设计符合性审查 | 没覆盖 GPT 额度真实上游和上游版本策略 |

## 3. 共识项（双方都提出，裁决：需要修复）

| 项目 | Codex | Claude | 裁决 |
| --- | --- | --- | --- |
| `SetAccess` 省略 `department_ids` 时不清授权（`department_repo.go:499`） | P2 | P2-2 的一部分 | **修复**。Claude 另外指出界面路径也会复活授权（见 4.2），应合并处理 |
| 进度接口把所有错误改写成 404（`subscription_service.go:1257-1260`） | P3 | P3-1 | **修复**。越权应返回 403，数据库错误应原样返回 |
| 组织用量分页乘法溢出（`organization_usage_service.go:472-477`、`organization_usage_repo.go:181`） | P3 | P3-2 | **修复**。复用部门列表的溢出守卫 |
| 分配订阅没有过滤停用分组：`assignable-groups` 只判断订阅类型（`subscription_assignment_options.go:53`），`assignSubscriptionWithReuse` 也不查 status（`subscription_service.go:509-515`） | P3 | P2-3 的子项 | **修复**，P3。只限管理端分配：`assignable-groups` 和 admin assign/bulk-assign 拒绝非 active 分组；兑换码、支付履约、默认订阅共用的 `assignOrExtendSubscription` **不能**加此校验（见 8.1 回归 R2） |
| 真实数据库集成测试缺少有效证据 | P1 | 本机 PG16 部门 12/12、自助重置 3/3 通过，但 CI 从未执行 | **发布门禁**。以 CI 集成测试为绿作为通过标准 |

## 4. 分歧项裁决

### 4.1 Codex 提出、经复核调整的项

| Codex 结论 | 复核证据 | 裁决 |
| --- | --- | --- |
| **P0**：前端全量 Vitest 有 1 个 unhandled error（`AccountsView.selectAllResults.spec.ts` 触发 `useTableLoader.ts:59`） | 本轮全量复跑：372 个文件 / 2789 个测试，exit 0，没有 Errors（日志 `E:/tmp/verify-10207/vitest-full.log`）；单独跑该 spec 为 5/5。spec 来自上游提交 `2a871ec85`、`7f0f579bb`，不在 CI 的 `FRONTEND_CRITICAL_VITEST` 列表中；CI 的 frontend 任务为绿 | **降为 P3（偶发测试卫生问题），不阻断**。成因是用例结束后仍有 `load()` 未完成，而 mock 返回了 undefined。建议在 spec 的 afterEach 中等待或中止在途加载，或者让 `useTableLoader` 在响应为空时安全返回。连续两次全量复现再升级 |
| **P1**：后端全量有 3 个 `backup_pg_dumper` 失败 | 原因是 Windows 本机没有 `sh.exe`，wiki `ops.md` 已登记为环境边界；CI（Linux）中这 3 个用例没有出现在失败列表里 | **不是代码问题**。以 CI 结果为准 |
| **P1**：集成测试被跳过 | 见第 3 节共识项。补充一点：CI 跳过集成测试的直接原因是单测步骤失败 | **发布门禁**，与 G1 合并 |
| **P1**：GPT 额度真实上游采集未验收 | `backend/migrations/243_gpt_quota_display.sql:6` 中 `enabled BOOLEAN NOT NULL DEFAULT FALSE`，功能默认关闭 | **不是部署阻断**，是“开启 GPT 额度展示前”的门禁。部署后保持关闭，用隔离账号验收后再开启 |
| **P1**：govulncheck 未执行 | CI Security Scan 在 `main@273b76c35` 上已成功（run `36439528255`，包含 govulncheck、pnpm audit 和 audit exception 检查） | **已满足**，不需要处理 |
| 基线风险：上游已到 0.2.9 | `upstream/main@9a62841fd`：比 0.2.8 多 70 个提交、117 个文件，**没有新迁移**，提交标题中没有安全修复 | **不阻断**。本次上线锁定 0.2.8，0.2.9 按常规同步流程放到下一轮 |
| 已通过项：“后端重点测试通过”“工作区干净”“图谱 READY” | Codex 只跑了默认 tag，没有发现 `-tags=unit` 下 service 包编译失败；工作区现在有本轮 wiki 和文档改动，尚未提交 | **不成立或已过期**。以 CI 为准；提交文档后再刷新图谱 |

### 4.2 Claude 提出、Codex 未覆盖的项

以下都经过三轮复核，证据见 [设计落地审核](organization-department-usage-design-implementation-audit-cn.md)。

| Claude 结论 | 裁决 | 理由 |
| --- | --- | --- |
| **B-1**：`-tags=unit` 下 `ptrFloat` 重复定义 | **修复，阻断** | CI run `36439528136` 的注解就是 `ptrFloat redeclared in this block`。来源是 10209 的 `88afc6589` |
| **P2-1**：部门模式的订阅响应返回完整用户 DTO（成员和分配人的余额、累计充值、通知邮箱、角色） | **修复，开放负责人试点前必须完成** | `mappers.go:12-36,882-892`；`user_subscription_repo.go:76,374`。分配人通常不在负责人的授权范围内 |
| **P2-2**（Codex 只覆盖了 nil 部分）：撤掉两项部门权限后授权仍保留；授权弹窗按“已有授权 ∪ 当前部门”提交，并默认两项都勾选 | **修复，开放负责人试点前必须完成** | `user_repo.go:367-371`；`DepartmentAccessDialog.vue:91-108`。正常界面操作就会让已撤销的部门范围复活 |
| **P2-3**：10208 之后，全站订阅权限包含分配能力，但权限描述、设计和负责人弹窗都没有体现；子管理员可以给自己分配 | **修复**。文案部分部署前完成（低成本）；自我分配和停用分组校验作为 10208 治理项 | `admin_permission.go:65-68`；`zh/admin/overview.ts:636` |
| **P2-4**：导出没有沿用页面快照的 `as_of` | **修复，上线前完成（约 1 行改动）** | `OrganizationUsageView.vue:534`；`organizationUsage.ts:262` |
| P3-3 到 P3-21、D1–D16 | 按原报告处理，不阻断上线 | 均不扩大访问范围 |

### 4.3 双方都没有写明、本轮新确认的门禁

| 项目 | 证据 | 裁决 |
| --- | --- | --- |
| `TestApplyDefaultOpenAIReasoningEffort/config_none_normalizes_to_empty_->_disabled` 失败 | CI run `36388753123`（0.2.8 合并时）已失败；本轮用 overlay 在 HEAD 复跑仍失败：配置 `"none"` 时仍会注入 `reasoning_effort`。测试来自本地 `7c70a7a72` | **修复，阻断 CI**。需要先确认预期：配置 `none` 应表示“不注入”，还是改为向上游显式传 `none`。据此修实现或修测试 |
| golangci-lint 报 29 个问题 | CI lint 任务失败。问题分布在 `apicompat`、`redis.go`、`large_chat_tool_compaction.go` 等处；`organization_usage_repo.go` 的 3 处是 7 月报表功能遗留，**不是部门提交引入的** | **既有 lint 债，不阻断本次**。按既有清理约定处理；本次改动不得新增问题（增量 lint 为 0） |
| `TestAPIContracts`（`/auth/me` golden） | 0.2.8 合并时 CI 失败；本轮 HEAD `-tags=unit` 复跑通过 | 已恢复，不需要处理 |

## 5. 修复建议（按上线阶段）

### 阶段 A：部署生产前（阻断）

| 编号 | 事项 | 通过标准 |
| --- | --- | --- |
| A1 | 修 `ptrFloat` 重复定义（重命名 `gpt_quota_display_test.go:287` 的 helper） | `go test -tags=unit ./internal/service` 能编译 |
| A2 | 确认 `none` 语义后，修复 `TestApplyDefaultOpenAIReasoningEffort` 或对应实现 | 该用例通过 |
| A3 | 推送后 CI `test` 任务全绿（包括 `make test-integration`） | 部门、订阅、组织用量相关的 integration 用例都出现 PASS，没有 skip |
| A4 | 更新 `admin.subscriptions` 的中英文描述（写明包含分配），在负责人弹窗里写明“含全站分配订阅” | 界面文案与白名单一致 |
| A5 | 导出传入页面快照的 `as_of`；报表未加载成功时禁用导出 | 补一个页面级断言 |
| A6 | 修复共识中的 P3 小项：进度接口错误码、分页溢出、停用分组分配 | 各补一个单测 |

### 阶段 B：给部门负责人开放试点权限前（设计 §10 第 5 步）

| 编号 | 事项 | 通过标准 |
| --- | --- | --- |
| B1 | P2-1：部门模式的订阅响应改用精简 DTO | handler 测试断言响应中没有余额、充值、通知邮箱等字段 |
| B2 | P2-2：`department_ids` 等字段必须显式提供；两项部门权限都撤销时清空授权；弹窗列出全部已授权部门，无部门权限时默认不勾选 | PG 用例覆盖省略、null、`[]`、撤权后再授权四种情况 |
| B3 | 前端重置遇 403/409 的处理（P3-6） | Vitest 覆盖 403 清数据、409 清快照和幂等键 |

### 阶段 C：开启 GPT 额度展示前

| 编号 | 事项 |
| --- | --- |
| C1 | 用隔离的真实 OpenAI OAuth 账号完成单账号采集和页面读取，并确认不改写 `accounts.extra`、不触发 reset-credit、不改变调度状态（Codex 门禁 7） |

### 阶段 D：常规迭代（不阻断）

- Claude 报告中其余的 P3（锁粒度、`ANY` 数组、成员加载、平台目录、审计改前值、界面缺口、测试缺口、回退工具）和 D1–D16 文档同步。
- 前端 `AccountsView` 用例的偶发 unhandled rejection。
- golangci-lint 存量问题。
- 上游 0.2.9 同步。

## 6. 裁决汇总

| 类别 | 条目 |
| --- | --- |
| 需要修复，且阻断部署 | B-1 `ptrFloat`；`TestApplyDefaultOpenAIReasoningEffort`；CI 集成测试为绿；P2-3 文案；P2-4 导出快照；共识中的 3 个 P3 |
| 需要修复，且阻断负责人试点 | P2-1 精简 DTO；P2-2 授权生命周期（含 Codex 的 nil 问题）；P3-6 |
| 需要修复，但只阻断对应功能开启 | GPT 额度真实上游验收 |
| 不是问题，或已满足 | `backup_pg_dumper`（环境问题）；govulncheck（CI 已通过）；`TestAPIContracts`（已恢复） |
| 降级处理 | 前端 unhandled error：P0 降为 P3（没有复现，不在 CI 门禁内） |
| 需要决策，不阻断 | 锁定 0.2.8，0.2.9 放到下一轮；lint 存量按既有约定处理 |

裁决复核阶段只做了只读复核和测试复跑，复跑日志在仓库外的 `E:/tmp/verify-10207/`。后续修复状态见下节。

## 7. 2026-09-29 修复启动记录

经当前源码复核，裁决中的 B-1、`none` 默认注入、停用分组分配、订阅进度错误映射、组织用量分页溢出、授权缺省/撤权清理、部门模式订阅精简 DTO、导出 `as_of`、子管理员自我分配和相关中英文权限文案已开始修复。当前已通过后端定向测试、部门/组织用量前端定向测试和 ESLint；尚未完成 PostgreSQL 集成验证、CI 全量验证、完整前端 Vitest、完整后端 unit/integration 和负责人试点验收，因此不能据此改变本文件的 NO-GO 结论。

## 8. 2026-09-29 修复终审与补修

本节对第 7 节 Codex 修复做逐项复核，并补修所发现的问题。终审基线仍是 `273b76c35` 加上未提交的工作区改动。

### 8.1 复核发现的回归与缺陷（已补修）

| 编号 | Codex 修复中的问题 | 证据 | 补修 |
| --- | --- | --- | --- |
| R1 | 精简 DTO 的开关用的是 `h.departments != nil`，但 `NewSubscriptionHandler` 总会注入非 nil 的 DepartmentService，所以完整管理员、Admin API Key 和全站子管理员也都拿到了精简 DTO，丢失 `assigned_by_user`、`notes` 等字段。此外精简分组只有 `id/name/platform`，订阅页的额度进度条依赖的 `daily/weekly/monthly_limit_usd`、`subscription_type`、`rate_multiplier` 全部缺失，**所有角色的进度条都会消失** | `subscription_handler.go:62-68`；`SubscriptionsView.vue:268-396` | 按请求角色选择投影：`sub_admin`（全站或部门）用 `dto.SubAdminUserSubscription`，完整管理员和 Admin API Key 保持完整 DTO。精简分组补上状态、类型、倍率和三档限额。子管理员 bulk-assign 的结果不再返回订阅明细 |
| R2 | 停用分组校验同时加进了 `assignOrExtendSubscription`。它是兑换码（`redeem_service.go:528`）、支付履约（`payment_fulfillment.go:584`）和注册/首绑默认订阅共用的路径，分组停用后，**已付款订单会履约失败** | `subscription_service.go:224-234` | 撤回该处校验，只保留在管理端的 `assignSubscriptionWithReuse` 和 `assignable-groups`。新增回归测试：管理端拒绝停用分组；`AssignOrExtendSubscription` 仍能为停用分组履约 |
| R3 | 用户编辑去掉部门权限时，先读取 `after`、再删除 grants，所以返回的 `admin_access_version` 仍包含已删除的 grant。下一次保存会误报 409 `ADMIN_ACCESS_CHANGED`，审计中的 after 也是错的 | `user_repo.go:373-392` | 清理后重新 `loadAdminAccess`。真实 PG 用例断言返回的版本等于数据库当前版本；用 `-overlay` 去掉重读后该用例失败，证明它能捕获这个缺陷 |
| R4 | 授权弹窗的 `retainExistingGrants = hasDepartmentPermission \|\| report \|\| resetQuota`：管理员一勾选权限，休眠 grant 就被带回提交。Codex 把测试预期也改成了 `[7, 8]`（8 是休眠授权），等于把复活行为写进了测试 | `DepartmentAccessDialog.vue:104`；spec 中账号 99 只有 `admin.subscriptions` 和 grant `[8]` | 只在加载时账号已持有部门权限的情况下保留其他授权。测试改为预期 `[7]`，另增“现有负责人的授权范围被保留”（`[7, 8]`）用例 |
| R5 | Codex 记录的“后端定向测试通过”不成立：停用分组校验导致 10 个既有分配用例失败（`GROUP_NOT_ACTIVE`）；且没有任何新增代码配测试 | `go test ./internal/service -run 'TestAssign\|TestBulkAssign'` 失败 10 个 | 分配用例夹具补上 `Status: StatusActive`。新增精简投影、自我分配、停用分组、进度错误透传、SetAccess 替换集、分页溢出、授权清理（PG）、导出 `as_of` 等测试 |
| R6 | 导出没有快照时静默返回，按钮却仍可点击 | `OrganizationUsageView.vue:15,521` | 加入 `export-disabled` 条件；页面导出测试断言 `as_of` 等于页面快照 |
| R7 | 新错误码 `SUBSCRIPTION_SELF_ASSIGN_DENIED`、`GROUP_NOT_ACTIVE` 在前端只显示“分配订阅失败” | `SubscriptionsView.vue` 分配 catch | 按业务码显示中英文提示 |
| B3 | （原阶段 B 待办，Codex 未处理）单项和筛选批量重置遇到 403/409 时只提示通用失败：确认框、旧快照和幂等键都保留，重试会反复 409；403 时也不清受保护数据 | `SubscriptionsView.vue` 两个重置的 catch | 403/409 时关闭确认框，清空快照和幂等键，按业务码提示，并重新加载列表（列表遇 403 走既有清数据路径）；网络错误仍保留幂等键以便安全重试。新增 2 个 Vitest |

### 8.1a 终审后自审补修

| 编号 | 问题 | 补修 |
| --- | --- | --- |
| S1 | R3 只在“本次编辑去掉部门权限”时清 grants；上线前遗留的休眠 grant（账号本来就没有部门权限）在之后从用户编辑重新开启权限时仍会复活 | 改为不变式：`sub_admin` 只要不持有任何部门权限，任何一次权限编辑都会清掉残留 grants 并重读版本。PG 用例新增遗留 grant 场景 |
| S2 | 授权弹窗提示仍写“其他部门的授权会保留”，但两项权限都取消时后端会清空全部授权 | 中英文提示补充“两项权限都取消时，将清除该负责人的全部部门授权” |
| S3 | Codex 把新负责人的“允许本部门”默认改为不勾选：管理员勾了报表却漏勾部门时，会保存出“有权限、无部门”的无效配置 | 恢复原默认（本部门启用时预选）；报表、重置两项仍需显式勾选。新增 Vitest 覆盖 |

已确认无需改动：配置 `openai_default_reasoning_effort` 的文档写明“空=关闭，合法值 low/medium/high/xhigh”，原测试预期 `none` 等于关闭，因此把 `none` 视为关闭符合原功能意图。进度接口的响应只含分组名、到期时间和用量窗口，用户侧调用方遇到错误会跳过，改为透传错误没有副作用。订阅页读取的字段都在精简投影内。

### 8.2 确认成立、原样保留的 Codex 修复

| 项目 | 结论 |
| --- | --- |
| `ptrFloat` 改名为 `ptrQuotaFloat` | 成立，`-tags=unit` 下 service 包可以编译 |
| 配置 `"none"` 不注入 `reasoning_effort` | 成立，`TestApplyDefaultOpenAIReasoningEffort` 通过；语义为“显式关闭站点默认值” |
| 进度接口原样返回仓储错误 | 成立。仓储已把“无记录”映射为 `ErrSubscriptionNotFound`，越权返回 403，数据库错误不再被伪装成 404 |
| 组织用量分页溢出守卫 | 成立，已补 `page=MaxInt` 用例 |
| `SetAccess`：nil 视为 `[]`；两项部门权限都为 false 时清空 | 成立，已补单测和真实 PG 用例 |
| 子管理员不能给自己分配（assign/bulk-assign） | 成立，已补 handler 用例；完整管理员不受影响 |
| 权限描述、负责人弹窗文案写明全站分配能力 | 成立 |
| 导出带上页面快照的 `as_of` | 成立（按钮状态见 R6） |

### 8.3 终审验证（本机，2026-09-29）

| 命令 | 结果 |
| --- | --- |
| `go build ./...`；`go vet`（service、handler、repository，含 integration tag） | 通过 |
| `go test -tags=unit -p 1 -count=1 ./...`（与 CI `make test-unit` 同口径，PATH 包含 Git `usr/bin`） | **exit 0，60 个包全部 ok**，含 `backup_pg_dumper`、`TestAPIContracts`、`TestApplyDefaultOpenAIReasoningEffort` |
| 受影响包在最终改动后重跑：`-tags=unit` 的 service、handler/*、repository、server/*，以及默认 tag 的同组包 | 全部 ok |
| 真实 PG16（内嵌）：部门 6 组、组织用量 6 个、自助重置 3 个，加新增 `TestUserAdminAccessIntegration_RemovingDepartmentPermissionsClearsGrants` | 16/16 PASS，0 SKIP |
| golangci-lint v2.14 `--new-from-rev=HEAD`（默认 tag 和 integration tag） | 0 issues |
| 前端 `lint:check`、`typecheck` | 通过 |
| 前端 Vitest 全量（B3 之前） | 372 个文件 / 2790 个测试，exit 0，没有 unhandled error |
| 前端相关 Vitest（B3 之后：`src/views/admin`、部门、用户、i18n、订阅和报表 API） | 86 个文件 / 586 个测试通过 |

### 8.4 仍未完成的门禁（维持 NO-GO 的理由）

| 门禁 | 状态 |
| --- | --- |
| 提交并推送后 CI `test` 任务（unit + Docker integration）全绿 | 未执行：本轮不提交、不推送。本机已按 CI 口径通过，但 CI 从未执行过集成测试步骤 |
| CI golangci-lint 存量 29 个问题 | 既有债务，不是本次引入（增量为 0）；是否阻断发布由团队决定 |
| 负责人试点验收（设计 §10 第 5 步） | 未执行 |
| GPT 额度真实上游验收 | 未执行，只阻断开启该功能 |
| 设计文档状态与 D1–D16 同步 | 未执行；Codex 本轮没有改设计文档 |

结论：代码层面的阶段 A（部署前）和阶段 B（B1–B3，负责人试点前）已全部完成，并有测试证据。**CI 转绿之前仍维持 NO-GO**；提交并推送后若 CI 的 test 任务（含集成测试）全绿，即可转为有条件 GO：按设计 §10 分步上线，GPT 额度展示保持关闭。
