# 部门功能实现审核（隔离与逻辑）

日期：2026-09-21。基线：`feature/hy/10207_department_usage@927c164d9`，相对 `main`（merge-base `de5a3e383`）。

本轮只读审核实际实现，查找较大漏洞和逻辑错误，不改代码。对照源码核对调用方、迁移、仓储 SQL、管理路由与前端权限入口。生成 Ent、wiki 图谱、验收截图/工作簿，以及 `feat:暂存` 中无关的 shared-compute-pool 文档不在审核范围。

相对 `main` 共 3 个实现提交、192 个文件（`+20290/-743`）；深审 126 个手写实现文件。

## 结论

没有发现会跨组织、跨部门越权，或在空授权/撤权后回退全站的较大漏洞。空 grant、省略/`all` 筛选、报表与重置权限拆分、锁内重新鉴权、降级/软删除清 grant、停用部门禁新增、Admin API Key 全量管理员语义、子管理员写白名单，这些高风险路径都是 fail-closed。

Ent 空 `UserIDIn` 会变成 `FALSE`，SQL `ANY('{}')` 不会退化成全站。2026-09-21 精简复审宣称的加固大体成立（版本化管理详情读取、导出首页快照、actor/lookup 的 DB 错误、部门列表分页溢出、授权增量锁）。剩余 3 项是加固未覆盖完全，当前不会扩大访问范围。

| 统计 | 数量 |
| --- | ---: |
| bug（可越权、回退全站、写错目标） | 0 |
| suggestion（口径/合同缺口） | 3 |
| nit | 0 |

## 已核对仍成立的隔离合同

| 路径 | 结果 |
| --- | --- |
| 负责人无 grant 读目录 / 读数据 / 重置 | 目录可空；数据与重置拒绝；不回退全站 |
| 省略筛选或 `all` | 仅表示授权范围内全部 |
| 撤销报表权限或清空 grants 后再查订阅、重置、幂等重放 | 不回退全站 |
| `admin.organization_usage` 与 `admin.department_subscriptions` 拆分 | 共用 `department_access_grants`，权限独立 |
| 重置 | 操作者/成员行锁下重新鉴权；`scope_version` 只做变化检测 |
| 降级 / 软删除 | 同事务清其持有 grants；再提权不复活 |
| 停用部门 | 可查、可移出、可重置；不能新增成员或新授权 |
| 完整管理员与 Admin API Key | 保持全量管理员语义 |
| 子管理员写白名单 | 仍仅单条重置与筛选日限重置；成员管理、订阅分配等拒绝 |
| 前端 403 / 范围变化 | 清受保护数据并作废迟到请求；已返回/已下载内容不追回 |

计费 command 费用二选一及潜在混合扣费锁序（`users` → `user_subscriptions` 与反向）仍按既有设计处理，本分支未新开混合路径，不记为新缺陷。

## 剩余问题

### 1. 订阅进度把所有错误改写成 404

- 优先级：P3（fail-closed，不扩大访问）
- 文件：`backend/internal/service/subscription_service.go:1259`
- 说明：`SubscriptionHandler.GetProgress` 已改为 `response.ErrorFrom`，但 `GetSubscriptionProgress` 仍把 `GetByID` 的全部错误改写成 `ErrSubscriptionNotFound`。仓储 `checkDepartmentSubscription` 对越权返回 `ErrDepartmentScopeDenied`，查询失败返回真实 DB 错误。部门负责人探测范围外 ID，或管理员遇到 DB 中断，进度接口一律 404。同一 allowlist 上的 `GetByID` 会保留越权错误。
- 建议：`GetSubscriptionProgress` 原样返回 `GetByID` 错误，仅在确实无记录时映射 404。补单元测试：scoped `ErrDepartmentScopeDenied` 与包装后的 DB 错误不得被改写。

### 2. 组织用量分页溢出只修了部门列表

- 优先级：P3（仍在授权范围内的错页，不是跨租户泄漏）
- 文件：`backend/internal/service/organization_usage_service.go:472`；OFFSET 计算在 `backend/internal/repository/organization_usage_repo.go:181`
- 说明：部门列表 `normalizeDepartmentList` 已拒绝 `page-1 > math.MaxInt/pageSize`。组织用量 Summary/Periods 仍接受任意 `page >= 1`（`page_size <= 1000`），再计算 `(page-1)*pageSize`。amd64 上 `strconv.Atoi` 可达 `math.MaxInt`，乘法回绕后：负 OFFSET 被 PostgreSQL 拒绝；回绕成较小正 OFFSET 则返回错误页的仍在范围内的行，报表/导出会不一致。这是 2026-09-21 宣称已关闭的同类缺口。
- 建议：在 `normalizeOrganizationUsageQuery` 使用与部门列表相同的溢出守卫，进入仓储前拒绝。用 `page=math.MaxInt` 的服务测试覆盖，且不得打到 SQL。

### 3. `SetAccess` 省略 `department_ids` 不会清 grant

- 优先级：P2（Admin API Key / 裸 JSON 合同，UI 路径安全）
- 文件：`backend/internal/repository/department_repo.go:499`；服务层 `backend/internal/service/department_service.go:368`
- 说明：`department_ids` 按全量替换语义，但 JSON 省略该字段时是 `nil`。服务层只拒绝 `len > 200` 和非法 ID，不把省略规范化为空切片。`pq.Array(nil)` 绑成 SQL `NULL`，`NOT (department_id = ANY(NULL))` 匹配不到行，已有 grant 保留，同时 `admin_permissions` 仍可能被改写。Vue 弹窗总会发送数组（含 `[]`），界面路径安全。Admin API Key 或裸 JSON 若省略 `department_ids`、只关掉报表/重置权限，grant 会留下；以后再打开 `admin.organization_usage` 或 `admin.department_subscriptions`（用户编辑或再次 SetAccess）会静默恢复旧部门。这与降级“grant 不复活”的生命周期相反。
- 建议：在 `DepartmentService.SetAccess` 把省略的 `DepartmentIDs` 规范化为空切片（替换集语义），或仅在字段显式缺省（指针）时跳过 DELETE。用真实 SQL 覆盖省略、`[]`、非空保留列表，并包含 Admin API Key 上下文。

## 建议处理顺序

1. 第 3 条：补齐替换集语义，避免 Admin API 留下可复活的 grant。
2. 第 1 条：进度接口错误口径与详情/列表对齐。
3. 第 2 条：组织用量分页与部门列表对齐。

本轮未改代码、未补测试、未提交、未推送、未部署。

## 相关文档

- 设计：`docs/features/organization-department-usage-design-cn.md`
- 执行记录：`docs/features/organization-department-usage-implementation-plan-cn.md`
- 2026-09-21 精简复审（已处理项）：`docs/features/organization-department-usage-code-review-cn.md`
- 验收与性能：`docs/delivery/2026-09-19-department-usage/acceptance.md`、`performance.md`
