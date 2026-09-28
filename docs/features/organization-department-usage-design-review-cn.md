# 组织、部门与多平台用量报表设计审核报告

> 审核日期: 2026-09-20（第二轮修订，吸收 Codex 复核意见并重新核对源码）
> 被审文档: `docs/features/organization-department-usage-design-cn.md`（分支 `feature/hy/10207_department_usage`，HEAD `de5a3e383`；实现位于未提交工作区）
> 审核背景: 业务需求为"组织下增加部门维度，按部门统计用量；部门负责人可查看本部门报表、保留额度重置，不能分配订阅"。负责人职责已在设计 §6.1、§13 标注为业务确认项。
> 结论级别: **设计方向与范围成立，可在既定范围内收口；上线前必须补齐以下缺口**：权限双写路径的后端并发控制（P1-4）、`scope_version` 哈希范围收窄（P1-2）、完整管理员路径的范围解析短路与 SQL 侧过滤（P1-3/P2-2）、错误码区分（P2-3）、角色降级后的授权清理（P1-1）、锁与计费的并发验证（P2-5）。既有测试通过不代表这些新增发现已解决，最终验收需逐项补证。

所有代码引用基于 2026-09-20 工作区快照的行号；未运行测试、未连接数据库，结论来自静态阅读、`git` 状态核对与本机验收产物。

## 修订说明

第一轮报告存在两处事实错误，本轮更正：

1. **验收产物**：第一轮判定 `docs/delivery/2026-09-19-department-usage/` 为空。复核发现该目录含 `acceptance.md`（8,119 B）、`performance.md`（2,486 B）及 `artifacts/`（HTTP 验收 JSON、两张截图、一份 XLSX）。第一轮漏检原因：`.gitignore:155` 的 `docs/*` 规则忽略了该目录，依赖 gitignore 的文件搜索与 `git ls-files --others` 均不可见，而当时的目录列举命令输出丢失未被察觉。正确表述为"证据存在于本机，但未纳入版本库"（见 P0-2）。
2. **用户删除路径**：第一轮把 `User.Delete()` 认定为物理删除并推断外键故障。`users` 使用 `SoftDeleteMixin`（`backend/ent/schema/user.go:32`），删除钩子把 DELETE 改写为 `UPDATE deleted_at`（`backend/ent/schema/mixins/soft_delete.go:99-130`），仅 `SkipSoftDelete(ctx)` 才物理删除；`backend/internal` 中没有对 `users` 使用该 context 的删除调用。外键故障判断撤回（见 P1-1）。

此外按复核意见撤回"负责人特性属于范围膨胀"（P0-3）、"删除 `department_version`"（P2-6）、"审计触发器吞异常"（P1-5 的缓解建议）以及"用 `WHERE EXISTS` 替代行锁"（P2-5 的缓解建议）四项判断，理由见对照表。

## 复核结论对照表

| 复核意见 | 本轮结论 | 依据摘要 |
| --- | --- | --- |
| P1-4 权限两条写路径，旧编辑窗口可覆盖新授权甚至恢复全站订阅权限；需后端并发控制 | **确认**，并升级描述 | `admin_user.go:274-284` 以请求数组整体覆盖，无版本；`NormalizeAdminPermissions` 只在两码同时出现时拒绝（`admin_permission.go:127-131`），旧数组只含 `admin.subscriptions` 时可通过，grant 同时保留 |
| P1-2 `scope_version` 范围过宽，应限定实际查询范围并保留一致性检查 | **确认** | `department_repo.go:548,571` 对管理员哈希全站 active 用户及部门 `updated_at` |
| P1-3、P2-2 全量成员扫描、内存分页、逐部门追加查询存在；延迟回归幅度未实测 | **确认**（存在）／**待验证**（幅度） | 报表路径在 600 用户规模下已含范围解析计时（`performance.md:3-5`）；订阅路径与更大规模未测 |
| P2-3 错误码与提示需区分重名、范围变化、部门停用、跨组织 | **确认** | `department_repo.go:358-366,486-489`；`departmentErrors.ts:9-13` |
| P1-1 角色降级残留授权需处理；用户删除经 SoftDeleteMixin，不能认定物理删除与外键故障 | **部分成立**：降级残留确认；外键故障撤回 | `soft_delete.go:99-130`；`user_repo.go:517` 使用普通 ctx；`department_access_grants` 仅在 `department_repo.go` 出现 |
| P2-5 补充锁与计费并发验证并优化锁范围；不删除锁，`WHERE EXISTS` 不能替代撤权/转岗并发保护 | **部分成立**：锁序与耦合确认；替代方案撤回 | 撤权（`department_repo.go:450`）、转岗（`:333`）、重置（`department_subscription_scope.go:132`）共享 `users` 行锁形成串行点；READ COMMITTED 下 UPDATE 的子查询不会看到快照后提交的 grant 删除 |
| P0-3 负责人查看本部门、保留额度重置、不能分配订阅是明确需求，不属于范围膨胀 | **撤回** | 设计 §6.1 "已确认的业务边界"、§13 "负责人职责（已确认）" |
| P0-2 验收产物存在但未提交，应区分"文件不存在"与"证据未提交" | **撤回原表述，改为"证据未入库"** | `.gitignore:155-180` 对 `docs/*` 默认忽略且无该目录例外；`git status --ignored` 显示 `!! docs/delivery/2026-09-19-department-usage/`；其余 8 个 delivery 目录已被跟踪 |
| P0-1 历史漂移存在，但当前成员口径是设计选择；发生时快照属产品口径调整，且只加 `department_id` 无法解决全部漂移 | **部分成立**：漂移确认；"必须重做"撤回；快照方案覆盖面按下文限定 | 设计 §4.2/§7.2/§7.3 与 `acceptance.md:73` 已明示口径；快照只覆盖实施后日志的转岗/换邮箱归属 |
| 暂不采纳删除 `department_version`：行锁不能识别 A→B→A 往返调岗 | **撤回删除建议** | `expected_department_id` 比对无法区分往返调岗；设计 §5.2 明确"任何真正冲突返回 409" |
| 暂不采纳吞掉审计触发器异常 | **撤回缓解建议**，风险改为文档化依赖 | 吞异常会破坏"审计与清归属同事务"的保证；`audit_logs` 当前列均有默认值（`180_audit_logs.sql:8-26`），无故障证据 |
| P2-1、P2-4、P3-1、P3-2 作为文档完善或局部简化 | **确认**，降为文档项 | 不再建议扩大重构 |

## 审核范围

| 范围 | 说明 |
| --- | --- |
| 需求匹配 | 文档范围与"部门维度 + 部门统计 + 负责人查看/重置"三项需求的匹配程度 |
| 统计口径 | 部门用量在时间维度上的可复现性，历史归属规则是否在文档与界面中明示 |
| 逻辑与闭环 | 数据模型、权限、生命周期、并发、回退是否自洽并闭环 |
| 实现核对 | 文档标注"已实现并通过验收"，对照工作区实现与本机验收产物核对承诺是否成立 |
| 已读文件 | 设计文档全文；`docs/delivery/2026-09-19-department-usage/acceptance.md`、`performance.md`；`backend/migrations/239_departments.sql`、`240_department_email_change_audit.sql`、`180_audit_logs.sql`；`backend/ent/schema/user.go`、`department*.go`、`mixins/soft_delete.go`；`backend/internal/service/department_service.go`、`admin_permission.go`、`admin_user.go`（部分）、`organization_usage_department.go`、`gateway_usage_billing.go`（部分）；`backend/internal/repository/department_repo.go`、`department_subscription_scope.go`、`department_subscription_read.go`、`organization_usage_department_repo.go`、`user_subscription_repo.go`（diff）、`organization_usage_repo.go`（diff）、`user_repo.go`（删除路径）、`usage_billing_repo.go`；`backend/internal/server/middleware/admin_auth.go`、`routes/department.go`；`backend/internal/handler/admin/department_handler.go`；前端 `DepartmentsView.vue`、`DepartmentAssignmentDialog.vue`、`UserEditModal.vue`、`OrganizationUsageView.vue`（部分）、`api/admin/departments.ts`、`api/admin/organizationUsage.ts`（部分）、`utils/departmentErrors.ts`；`.gitignore`；`llm-wiki/wiki/README.md`、`department-report-design.md`、`backend.md`（部分）、`data-and-domain.md`、`security-and-reliability.md` |

## 总体结论

| 维度 | 结论 |
| --- | --- |
| 需求匹配 | 部门建模、成员归属、报表部门/平台维度、负责人只读报表与部门内额度重置均对应明确需求。范围成立，不建议拆期或延期。 |
| 统计口径 | 当前成员口径是设计选择并已在 §7.3 与验收边界中明示；四类历史漂移（转岗、停用/删除、跨组织换邮箱、平台配置变更）是该口径的必然结果。是否改为发生时快照属于产品决策，本报告只要求文档把漂移来源与界面标注写全，并确认业务方接受。 |
| 最大闭环缺口 | 权限数组双写路径：旧用户编辑窗口可用过期数组覆盖 department-scope 的结果，包括把已切换为部门订阅权限的负责人恢复为全站订阅权限，且 grant 仍在。必须由后端并发控制解决。 |
| 最大实现风险 | 完整管理员路径被纳入部门范围解析：每次订阅读写和报表请求都加载全站 active 用户；`scope_version` 对管理员哈希全站用户，任何注册或资料变更都会中断管理员导出。报表路径在 600 用户规模下仍在阈值内，订阅路径与更大规模未实测。 |
| 验收证据 | 验收与性能记录存在于本机并有实测产物，但目录被 `.gitignore` 忽略、未入库；`acceptance.md` 自身仍标注"功能提交待收口"、"图谱最终刷新待收口"，设计文档"已实现并通过验收"的表述略超前。 |
| 建议 | 在既定范围内修正 P1-1～P1-4、P2-2、P2-3、P2-5 的确认项，补齐对应验收；把验收产物纳入版本库；文档补"目标规模"、"grant 生命周期"、"漂移来源与业务确认"三节。 |

## 关键问题清单

严重级别：**高** = 上线前必须修正；**中** = 应修正或在文档中显式接受；**低** = 建议/文档项。状态列为本轮复核结论。

| ID | 严重级别 | 状态 | 问题 | 影响 |
| --- | --- | --- | --- | --- |
| P1-4 | 高（安全） | 确认 | `admin_permissions` 两条写路径，旧编辑窗口整数组覆盖且无版本；可把部门订阅权限还原为全站订阅权限并保留 grant | 违反 §6.1 "绝不能回退全站"；§8.1 的 CAS 承诺只在新接口成立 |
| P1-2 | 高 | 确认 | `scope_version` 对完整管理员哈希全站 active 用户（含 username/status/email）与部门 `updated_at` | 管理员多页导出/翻页易被无关变更中断；与 §7.4 表述矛盾 |
| P1-3 | 高 | 确认（存在）／待验证（幅度） | 所有管理员请求注入 actor，订阅读写与报表每次执行 `resolveDepartmentScope`；单项重置重复解析约 3 次并对管理员自身 `users` 行 `FOR UPDATE` | 完整管理员路径新增全表 active 用户扫描；订阅路径回归幅度未实测 |
| P1-1 | 高 | 部分成立 | 角色离开 `sub_admin` 时 `admin_permissions` 清空但 grant 保留，重新提权后旧范围复活；外键故障判断撤回 | 隐蔽的权限残留；负责人列表按角色过滤后不可见 |
| P0-1 | 中（产品决策） | 部分成立 | 当前成员口径带来四类历史漂移；文档已明示口径但未逐项列出漂移来源及业务确认 | 历史月份部门数字不可复现，需业务方书面接受或改口径 |
| P0-2 | 中 | 撤回原表述 | 验收与性能产物存在但被 `docs/*` 忽略未入库；设计状态"已通过验收"超前于 `acceptance.md` 的"待收口" | 克隆仓库无法追溯证据；wiki 三处引用在版本库中不可达 |
| P2-2 | 中 | 确认（存在）／待验证（规模） | 部门列表、成员列表内存过滤/分页；部门列表每行 2 条附加查询；成员集合以 `ANY($n)` 传入 SQL | 600 用户规模已通过阈值；更大规模与订阅路径未测 |
| P2-3 | 中 | 确认 | 冲突错误复用于"需确认替换全站订阅权限"；前端把任何 409 视为范围变化；跨组织/目标停用同为 400 | 管理员看到误导文案，无法按原因处理 |
| P2-5 | 中 | 部分成立 | 重置锁序（`users`→`user_subscriptions`）与计费（`user_subscriptions`→`users`）相反，当前不死锁依赖"单次计费只碰一张表"的未记录不变量；批量重置持有成员 `users` 行锁 | 需补并发验证与文档；行锁协议本身应保留 |
| P1-5 | 低 | 部分成立 | migration 240 在 `users` 触发器内写 `audit_logs`，形成核心表对审计表结构的依赖 | 当前无故障；需以测试或文档保护该依赖，不吞异常 |
| P2-1 | 低（文档） | 确认 | 组织识别至少四处硬编码外加 CHECK 约束，扩展成本未写入文档 | 新增组织时的改动面被低估 |
| P2-4 | 低（局部简化） | 确认 | 报表筛选要求"数字部门 ID 必须同时传组织"，导致服务层绕行代码 | 合同冗余；可局部简化 |
| P2-6 | — | 撤回 | 删除 `department_version` 的建议 | 行锁 + `expected_department_id` 无法识别 A→B→A 往返调岗 |
| P0-3 | — | 撤回 | "负责人特性属于范围膨胀、应延期"的判断 | 负责人查看/重置/不可分配订阅是业务确认需求 |
| P3-1 | 低（文档） | 确认 | 文档结构："实施补充"置顶、四种角色混排、状态口径不一 | 读者难以区分承诺与记录 |
| P3-2 | 低（文档） | 确认 | 组织 `other` 的"未分配"桶等于全部非公司邮箱用户 | 该桶对部门统计意义弱，需说明或隐藏 |

## 详细问题与证据

### P1-4: 权限数组双写路径（确认，升级描述）

文档依据：§6.1 "`admin.department_subscriptions` 是持久的受限权限类型……绝不能回退全站"；§8.1 "并发权限变更采用最新值合并或 CAS，不能用旧数组覆盖其他管理员的修改"。

现状：

- `PUT /admin/users/:id/department-scope`（`department_repo.go:444-527`）以 `loadDepartmentAccess` 的哈希做 CAS，并在 `:481-499` 按 `report/reset_quota/replace_global_subscriptions` 重写权限数组。
- `PUT /admin/users/:id` 走 `admin_user.go:274-284`：`requestedPermissions := user.AdminPermissions; if input.AdminPermissions != nil { requestedPermissions = *input.AdminPermissions }`，随后 `NormalizeAdminPermissions` 后整体写回，没有版本或差量合并。前端 `UserEditModal.vue:225` 对 `sub_admin` 总是提交完整数组。
- `NormalizeAdminPermissions`（`admin_permission.go:127-131`）只在 `admin.subscriptions` 与 `admin.department_subscriptions` 同时出现时拒绝。

复现路径：管理员 A 打开负责人 M 的编辑窗口（快照 `[admin.subscriptions]`）；管理员 B 经 department-scope 以 `reset_quota=true, replace_global_subscriptions=true` 把 M 切换为 `[admin.department_subscriptions, admin.organization_usage]` 并写入 grant；A 保存 → 提交 `[admin.subscriptions]` → 校验通过 → M 重新获得全站订阅权限，grant 仍在。`loadDepartmentActor` 的互斥检查（`department_repo.go:77-79`）只在两码并存时触发，此处不生效。

建议（后端）：`PUT /admin/users/:id` 对 `admin_permissions` 增加 `expected_admin_permissions` 或用户版本字段做 CAS；或在该路径拒绝写入两个部门相关权限码与 `admin.subscriptions` 的变更，要求经 department-scope 接口处理；同时在 grant 非空时拒绝写入 `admin.subscriptions`。仅隐藏前端勾选项不足以关闭该路径。

复核方法：`admin_user.go:274-284`；`admin_permission.go:109-140`；`department_repo.go:77-79,481-509`；`UserEditModal.vue:164-168,225`。

### P1-2: `scope_version` 哈希范围过宽（确认）

文档依据：§7.4 "所有影响报表成员或标签的相关字段纳入摘要，不包含范围外部门的数据，避免无关变化影响负责人"。

现状：

```go
// backend/internal/repository/department_repo.go:548,571
members, err := queryDepartmentMembers(ctx, q, &queryActor, true)
...
result.Version = service.HashDepartmentScope([]any{actor.ID, actor.Role, actor.AdminPermissions, departments, members})
```

- `queryDepartmentMembers`（`department_repo.go:262-281`）对 `role=admin` 返回全站 active 用户；每个成员序列化 `id/email/username/status/organization/department_id/department_name/department_version`（`department_service.go:74-83`）。
- `departments` 序列化包含 `created_at/updated_at/version`。
- 前端 `organizationUsage.ts:292-293` 对任一分页响应版本不一致抛 `REPORT_SCOPE_CHANGED`；`OrganizationUsageView.vue:398-420` 重试一次后提示刷新。验收 R4 覆盖了"变更后旧版本拒绝"，未覆盖"无关变更导致管理员导出中断"。

建议：保留一致性检查，但哈希只取"本次实际查询范围"：所选组织/部门内成员的 `id + department_id` 集合、所选部门 ID 与 `version`、操作者 grant 集合与权限码；排除 username/status/email/`updated_at`。完整管理员选定单部门时只哈希该部门。

### P1-3: 完整管理员路径纳入范围解析（确认；回归幅度待验证）

文档依据：§6 "首版不缓存负责人授权"；§9.1 "保留现有独立服务边界"。

现状：

- `admin_auth.go:223` 对所有 JWT 管理员请求注入 `WithDepartmentActor`，`:153` 对 Admin API Key 注入 `WithDepartmentAdminAPIKey`。
- 报表：`organization_usage_repo.go:26,60,109` 在 actor 存在时一律进入 scoped 路径 → `resolveDepartmentScope`（`organization_usage_department_repo.go:89`）。
- 订阅读：`department_subscription_scope.go:14-20` 只在 actor 为 0 时提前返回；`getByID/listByUserID/listByGroupID/listAdmin` 每次执行 `resolveDepartmentScope`（3 条查询，含全表 active 用户扫描）后才因 `Unrestricted` 返回 `nil`；`department_subscription_read.go:16-34` 另开 REPEATABLE READ 事务。
- 订阅写：`beginDepartmentSubscriptionWrite`（`department_subscription_scope.go:87-159`）在 `:108`、`:135`、`:154→:63` 三次解析范围，`:121-133` 对 `[管理员自身, 成员]` 行 `FOR UPDATE`，写后再 `loadDepartmentActor` 一次用于审计。

已有测量：`performance.md:3-5` 说明 Summary 计时"包含仓储完整 Summary 与权限范围解析"，600 用户/219,600 日志下 12 组 p95 均 < 3,000 ms；其中 366 天单平台 p95 为 2,846 ms（`performance.md:22`，10 个样本取第 10 个），接近阈值。订阅列表/详情/重置路径没有对应测量。

建议：`resolveDepartmentScope` 入口对 `role=admin` 与 Admin API Key 直接返回 `Unrestricted=true` 且不加载成员；报表 SQL 对管理员只追加 `u.department_id = $x` / `IS NULL` 谓词；订阅路径对管理员跳过范围逻辑；部门模式的一次写操作只在加锁后解析一次。

待验证：订阅列表/详情/单项重置在管理员身份下的前后耗时；成员规模 ≥ 5,000 时报表与订阅路径的耗时。

### P1-1: grant 生命周期（部分成立）

文档依据：§4.2 "用户停用、删除 → 报表排除"；§5.1 "成员与授权的部门外键禁止级联删除"。

**撤回部分**：第一轮认为 `User.Delete()` 会触发 `department_access_grants` 的 `ON DELETE RESTRICT`。核对：`users` 使用 `SoftDeleteMixin`（`user.go:32`），删除钩子把 DELETE 改写为 `UPDATE deleted_at`（`soft_delete.go:99-130`）；`user_repo.go:517` 以普通 ctx 调用 `exec.User.Delete()`，因此是软删除；`backend/internal` 中没有对 `users` 使用 `SkipSoftDelete` 的删除或 `ClearDeletedAt` 恢复调用。外键 RESTRICT 在现有路径下不会触发，该判断撤回。若未来引入物理清理任务，需同时处理 grant 与 `created_by`。

**确认部分**：角色离开 `sub_admin` 时 `NormalizeAdminPermissions` 返回空数组（`admin_permission.go:124-126`，经 `admin_user.go:278-284` 写回），但 `department_access_grants` 不在任何用户更新路径中清理（grep 显示该表只出现于 `department_repo.go`）。部门列表"负责人"列按 `u.role='sub_admin'` 过滤（`department_repo.go:187-188`），残留 grant 从界面消失；该用户重新成为子管理员并被赋予 `admin.organization_usage` 后，`queryDepartments`/`queryDepartmentMembers`（`:129-131,266-267`）直接按残留 grant 放行。

建议：角色离开 `sub_admin` 的更新与 grant 删除同事务完成；软删除用户时一并删除其 grant（`created_by` 保留即可）。文档 §4.2 增加"负责人降级/删除"行。

### P0-1: 历史漂移与统计口径（部分成立）

文档依据：§1 "首版按当前成员归属统计"；§4.2 停用/删除与跨组织邮箱规则；§7.2 平台配置不是历史快照；§7.3 调岗示例；`acceptance.md:73` "管理报表按当前 active 且未删除成员归属；调岗重分类历史，平台配置也不是发生时快照"。

现状：设计与验收对口径的声明是一致且诚实的。该口径下，同一部门同一历史月份的数字会随以下事件改变：成员转岗；成员停用/软删除（其历史整体退出报表）；成员邮箱跨组织变更（`239_departments.sql:46-49` 置空归属，历史滑入"未分配"）；分组/账号平台配置修改（重新分类旧日志）。

**撤回部分**：第一轮把该项列为"阻断、需重做"。口径选择属于产品决策，设计已明示，不能据此判定必须重做。

**限定说明**：第一轮提出的 `usage_logs.department_id` 写入时快照，只能固定实施之后日志的"转岗"与"换邮箱"归属；不能解决停用/删除用户被 active 过滤排除（这是报表成员口径问题）、平台重分类（需平台快照）以及实施前历史行。若业务只需要"日常团队管理"，当前口径可接受；若需要"月度部门对账"，需要同时改成员口径与平台归因，成本远超单列快照。

建议：文档 §7.3 增加"漂移来源清单"表并注明每种来源的界面表现；由业务方书面确认接受当前口径；界面与 Excel 已按 §7.3 标注"按当前成员及部门归属统计"，保持。

### P0-2: 验收证据未入库（撤回原表述）

文档依据：第 10 行 "状态：**已实现并通过验收**"；第 12 行链接 `../delivery/2026-09-19-department-usage/acceptance.md`、`performance.md`。

现状：

- 本机存在 `docs/delivery/2026-09-19-department-usage/acceptance.md`、`performance.md`、`artifacts/http-acceptance.json`、`artifacts/leader-report.png`、`artifacts/leader-subscriptions.png`、`artifacts/test-department-grok.xlsx`。
- `.gitignore:155` 为 `docs/*`，`:156-180` 列出例外，其中没有 `docs/delivery/`；`git status --short --ignored docs/delivery` 显示 `!! docs/delivery/2026-09-19-department-usage/`。其余 8 个 delivery 目录（如 `2026-09-18-sub2api-v0.2.6-sync`）已被跟踪，说明既有做法是强制添加。
- `acceptance.md:12` "功能提交 | 待收口"、`:43` "W1 ... 图谱最终刷新待收口"、`:3` "提交与图谱收口中"。设计文档的"已实现并通过验收"比验收文件自身的状态更前。
- `performance.md` 给出了数据规模（600 用户、219,600 条日志、每组 10 次采样、p95 取第 10 个样本），第一轮"未说明规模"的批评撤回；但样本量小、且 366 天单平台 p95 逼近阈值应在设计 §11 注明。
- `llm-wiki/wiki/README.md:6`、`department-report-design.md:3`、`backend.md:10` 引用的路径在版本库中不可达。

建议：以既有方式将该目录纳入版本库（`git add -f` 或在 `.gitignore` 增加 `!docs/delivery/` 例外并评估其他被忽略的 delivery 目录）；设计状态行改为"验收通过，提交与图谱收口中"，与 `acceptance.md` 一致。

### P2-2: 内存分页、逐部门查询与成员集合传参（确认；规模待验证）

现状：

- `department_repo.go:147-207` `List`：取全部部门后在 Go 内过滤、切片，再对当前页每个部门执行 2 条查询（成员计数、负责人）；页大小上限 200（`department_service.go:310`）。
- `department_repo.go:283-317` `Members`：`queryDepartmentMembers` 加载全部未删除用户后内存筛选、分页；成员弹窗与用户页分配弹窗每次打开都会触发。
- `organization_usage_department_repo.go:101-104` 把选中成员 ID 数组以 `ANY($n::bigint[])` 传入 SQL。

已有测量：600 用户规模下报表路径在阈值内（`performance.md`）。文档 §7.1 "服务端先做部门与权限过滤，再 count、排序和分页"的表述与"Go 内存过滤后分页"不一致。

建议：部门列表改为 SQL 侧 `COUNT` 子查询 + 一次性负责人查询；成员列表改为 SQL 谓词过滤与 `LIMIT/OFFSET`；文档补"目标规模"一节。

待验证：≥ 5,000 用户、≥ 200 部门时的部门列表、成员弹窗与 Summary 耗时。

### P2-3: 错误码与提示区分（确认）

现状：

- `department_repo.go:486-489`：目标已有全站 `admin.subscriptions` 且未设 `replace_global_subscriptions` 时返回 `ErrDepartmentConflict`（"department or membership changed; refresh and retry"）。
- `departmentErrors.ts:9-13`：`isDepartmentScopeChanged` 对任何 HTTP 409 返回 true，包括重名 `DEPARTMENT_DUPLICATE` 与转岗 CAS 冲突 `DEPARTMENT_CONFLICT`。
- `department_repo.go:358-366`：跨组织成员与目标部门停用同为 `ErrDepartmentInvalid`（400），与 §4.1 "显示错误或并发变更"的可区分要求不符。

建议：分别定义"需确认替换全站订阅权限"、"成员不属于目标组织"、"目标部门已停用"错误码；前端按 `code` 而非状态码判断范围变化。

### P2-5: 锁序、锁范围与计费并发（部分成立）

现状：

- 撤权 `SetAccess`（`department_repo.go:450`）锁 `[操作者, 负责人]`；转岗 `Assign`（`:333`）锁 `[操作者, 成员…]`；重置 `beginDepartmentSubscriptionWrite`（`department_subscription_scope.go:132`）锁 `[操作者, 成员]` 或 `[操作者, 授权成员…]`。三者都通过 `lockDepartmentUsers`（`department_repo.go:85-106`）对 `users` 行 `FOR UPDATE`，因此撤权与该负责人的重置在负责人行上串行，转岗与重置在成员行上串行。验收 A4 以 `pg_stat_activity` 确认了"撤权等待重置行锁"。
- 网关计费 `applyUsageBillingEffects`（`usage_billing_repo.go:174-188`）先更新 `user_subscriptions`（`:215` 起）再更新 `users`（`:243` 起），与重置路径相反；`gateway_usage_billing.go:313-318` 保证一次计费只走订阅或余额之一，因此当前不会形成死锁环。

**撤回部分**：第一轮建议"不锁 `users` 行，改用 `WHERE EXISTS(grants)` 写入 UPDATE"。在 READ COMMITTED 下，UPDATE 的子查询按语句快照评估，快照之后提交的 grant 删除或转岗不会被看到；只有当目标 `user_subscriptions` 行本身被并发修改时才会重新评估条件。该方案无法关闭"校验后撤权/转岗、随后仍重置"的窗口，行锁协议应保留。

**确认部分**：锁序与计费相反的事实、"单次计费只碰一张表"这一不变量未被记录或测试、批量重置期间成员 `users` 行锁阻塞其余额扣费。

建议：文档 §6.1 记录锁序与计费不变量；对 `role=admin` 跳过锁与范围解析（无授权需保护）；评估以 `pg_advisory_xact_lock(user_id)` 替代 `users` 行锁并让三条路径共用，避免阻塞余额更新；补充并发测试：重置 vs 余额扣费、重置 vs 撤权、重置 vs 转岗。

### P1-5: 触发器对 `audit_logs` 的依赖（部分成立）

现状：`240_department_email_change_audit.sql:7-10` 在 `users AFTER UPDATE OF email` 触发器内 `INSERT INTO audit_logs (action, auth_method, method, status_code, extra)`；`audit_logs` 全部列有默认值或可空（`180_audit_logs.sql:8-26`），当前可用，验收 D5 已覆盖。

**撤回部分**：第一轮建议用 `EXCEPTION WHEN OTHERS` 包裹审计插入。该做法会破坏"清归属与审计同事务"的原子保证，且当前没有故障证据。

**保留部分**：这是核心表对审计表结构的隐式依赖。建议在 `data-and-domain.md` 记录"修改 `audit_logs` 结构前需检查 migration 240 触发器"，并增加一条 migration 集成测试断言触发器插入成功。

### P2-1: 组织识别硬编码（确认，文档项）

现状：`department_service.go:160-173` `OrganizationForEmail`；报表 SQL `organizationUsageOrganizationExpression`；`239_departments.sql:36-39` 触发器 `CASE`；`user_subscription_repo.go:463-466` `subscriptionOrganizationDomain` 及 diff 中 `NOT IN ('xunyou.com','wsdashi.com')` 字面量；`239_departments.sql:4` CHECK 约束。设计 §5.1 只写"若以后增加第三家正式公司，再单独扩展"。

建议：文档列出扩展清单（一条 migration 替换触发器函数与约束 + 四处代码 + 前端选项）；不在本轮重构。

### P2-4: "组织 + 部门"双参数（确认，局部简化）

现状：部门到组织不可变（`239_departments.sql:71-81`）；`department_service.go:324-344` 为通过自身校验先把数字部门 ID 改为 `all` 再改回。建议：数字部门 ID 单独传即可，服务端反查组织；`unassigned` 才需要组织。可作为局部简化，不影响其他合同。

### P2-6: `department_version`（撤回删除建议）

第一轮建议删除该列。复核：`expected_department_id` 比对无法识别 A→B→A 的往返调岗，版本号是唯一能让预览失效的信号；设计 §5.2 明确"任何真正冲突返回 409，并要求刷新预览"。删除建议撤回。保留一条低优先级观察：前端每次分配前需重拉版本（`DepartmentAssignmentDialog.vue:59-61,85`），属于该设计的必要成本。

### P0-3: 负责人特性范围（撤回）

第一轮把负责人报表授权、额度重置隔离、权限互斥与落地页规则归为范围膨胀并建议延期。复核设计 §6.1 "此项为已确认的业务边界"、§13 "负责人职责（已确认）"，且业务方明确"负责人查看本部门、保留额度重置且不能分配订阅"。该判断撤回；第一轮"核心约 5–8 人日"的估算随之作废。对这些特性内部复杂度的意见已并入 P1-2、P1-3、P2-2、P2-5。

### P3-1、P3-2: 文档项（确认）

- P3-1：第 3–8 行"实施补充"位于第 10 行状态与 §1 之前；第 10 行"已实现并通过验收"与第 461 行"这是验收目标，不是当前实测"、第 469 行"当前方案未通过生产数据压测"口径不一。建议拆出"实施记录"章节并统一状态口径。
- P3-2：`organization_usage_department_repo.go:230-234` 对管理员在单组织视图追加"未分配"桶；`other` 组织下该桶等于全部非 `xunyou.com/wsdashi.com` 用户。建议在 §4.3 说明含义或在 `other` 下隐藏。

## 验收补齐清单

`acceptance.md` 的 T2/T3/T6 通过证明既有合同未被破坏，不覆盖本报告新增发现。最终验收需补以下条目：

| 对应问题 | 需补验收 | 形式 |
| --- | --- | --- |
| P1-4 | 两管理员并发：department-scope 切换为部门订阅权限后，旧编辑窗口保存不得恢复 `admin.subscriptions`；grant 非空时写入全站订阅权限被拒 | 服务单测 + HTTP 用例 |
| P1-2 | 管理员导出期间新用户注册/其他用户改用户名/其他部门改排序，不得触发 409；范围内成员转岗仍触发 | 仓储集成测试 + Vitest |
| P1-3 | `role=admin` 路径不加载成员集合、不加 `users` 行锁；订阅列表/详情/重置在管理员身份下的耗时前后对比 | 仓储单测 + 性能记录追加 |
| P1-1 | 子管理员降级为普通用户后 grant 为空；软删除用户后 grant 为空；重新提权不复活旧范围 | 仓储集成测试 |
| P2-2 | ≥ 5,000 用户、≥ 200 部门下部门列表、成员弹窗、Summary 的 p95 | 性能记录追加 |
| P2-3 | 重名、范围变化、部门停用、跨组织分配、需确认替换全站权限五种错误各自的错误码与前端提示 | 服务单测 + Vitest |
| P2-5 | 重置 vs 余额扣费、重置 vs 撤权、重置 vs 转岗的并发用例；锁序与计费不变量写入文档 | 真实 PG 并发测试 |
| P0-2 | 验收产物纳入版本库；设计与 wiki 状态行与 `acceptance.md` 一致 | 提交 |
| P0-1 | 业务方对漂移来源清单的书面确认 | 文档 |

## 建议（在既定范围内）

1. 后端并发控制关闭权限双写路径（P1-4）。
2. `scope_version` 只哈希实际查询范围，保留检查（P1-2）。
3. 完整管理员与 Admin API Key 在范围解析入口短路；部门模式一次写操作只解析一次；部门/成员列表改为 SQL 侧过滤分页（P1-3、P2-2）。
4. 角色降级与软删除同事务清理 grant（P1-1）。
5. 错误码按原因区分，前端按 `code` 判断（P2-3）。
6. 保留行锁协议，记录锁序不变量，评估 advisory lock 并补并发测试（P2-5）。
7. 保留 `department_version` 与审计触发器；为触发器依赖补测试与 wiki 记录（P2-6、P1-5）。
8. 文档：补目标规模、grant 生命周期、漂移来源清单与业务确认、组织扩展清单；状态行与 `acceptance.md` 对齐；验收产物入库（P0-1、P0-2、P2-1、P3-1、P3-2）。

## 需要业务方确认的问题

1. 历史归属：是否书面接受当前成员口径及其四类漂移；若不接受，需要重新评审成员口径与平台归因，而不只是增加一列快照。
2. 验收状态：设计与 wiki 的"已实现并通过验收"是否改为"验收通过，提交与图谱收口中"，并将 `docs/delivery/2026-09-19-department-usage/` 纳入版本库。
3. 上线门禁：是否接受"验收补齐清单"作为本功能上线前的必需条件。

## 复核清单（供第二审核方）

| 编号 | 验证方式 |
| --- | --- |
| P1-4 | `admin_user.go:274-284`；`admin_permission.go:109-140`；`department_repo.go:77-79,481-509`；`UserEditModal.vue:164-168,225` |
| P1-2 | `department_repo.go:262-281,530-573`；`department_service.go:74-83,154-158`；`organizationUsage.ts:292-293`；`OrganizationUsageView.vue:398-420`；`acceptance.md` R4 |
| P1-3 | `admin_auth.go:150-155,214-224`；`organization_usage_repo.go:25-27,59-61,108-110`；`department_subscription_scope.go` 全文；`department_subscription_read.go:16-34`；`performance.md:3-5,22` |
| P1-1 | `backend/ent/schema/user.go:32`；`mixins/soft_delete.go:75-77,99-130`；`user_repo.go:460-524`；`rg -n "department_access_grants" backend/internal`；`rg -n "SkipSoftDelete" backend/internal/repository/user_repo.go`；`admin_permission.go:124-126`；`department_repo.go:129-131,187-188,266-267` |
| P0-1 | 设计 §4.2、§7.2、§7.3、§12；`239_departments.sql:46-49`；`acceptance.md:73` |
| P0-2 | `Get-ChildItem -Force -Recurse docs\delivery\2026-09-19-department-usage`；`git check-ignore -v docs/delivery/2026-09-19-department-usage/acceptance.md`；`git status --short --ignored docs/delivery`；`git ls-files docs/delivery`；`.gitignore:155-180`；`acceptance.md:3,12,43` |
| P2-2 | `department_repo.go:147-207,283-317`；`department_service.go:310`；`organization_usage_department_repo.go:101-104` |
| P2-3 | `department_repo.go:358-366,486-489`；`departmentErrors.ts:9-13` |
| P2-5 | `department_repo.go:85-106,333,450`；`department_subscription_scope.go:121-135`；`usage_billing_repo.go:174-260`；`gateway_usage_billing.go:313-318`；`acceptance.md` A4 |
| P1-5 | `240_department_email_change_audit.sql`；`180_audit_logs.sql:8-26`；`acceptance.md` D5 |
| P2-1 | `department_service.go:160-173`；`239_departments.sql:4,36-39`；`user_subscription_repo.go:463-466` 及 diff |
| P2-4 | 设计 §8.2；`department_service.go:324-344` |
| P2-6 | `239_departments.sql:57-63`；`department_service.go:117-126`；`DepartmentAssignmentDialog.vue:59-61,85`；设计 §5.2 |
| P0-3 | 设计 §6.1 首段、§13 表格 |
| P3-1 | 设计第 3–12 行、第 461 行、第 469 行 |
| P3-2 | `organization_usage_department_repo.go:230-234` |
