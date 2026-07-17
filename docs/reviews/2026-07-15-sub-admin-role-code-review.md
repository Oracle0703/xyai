# 子管理员角色实现审查报告

| 字段 | 内容 |
| --- | --- |
| 日期 | 2026-07-15 |
| 分支 | `feature/hy/10156_新增子管理员角色` |
| 对照文档 | `docs/delivery/2026-07-15-sub-admin-role/{requirements,spec,plan}.md` |
| 审查范围 | 当前工作区未提交改动（约 56 个已跟踪文件 + 12 个新增文件） |
| 审查目标 | 设计不合理、死代码、非最佳实践、设计矛盾 |

## 1. 总体结论

实现整体方向正确，核心安全模型也站得住：

- 后端以数据库最新用户角色/权限为准，不信任 JWT 内旧权限。
- 管理接口采用 **方法 + Gin `FullPath()` 白名单**，默认拒绝。
- 前端菜单/按钮隐藏与后端鉴权分离，写操作（清理、索引、分配/撤销订阅）后端未放行。
- 权限撤销后下次管理请求立即 403，并触发前端刷新跳转。

**但还不能视为可直接合并的成品。** 主要问题集中在：

1. 数据最小化不一致：usage 做了精简筛选接口，subscriptions 却开放完整 `AdminGroup`。
2. 简易模式下“空权限子管理员”会获得额外能力，角色语义被打穿。
3. 权限目录前后端双份维护，catalog API 实际成为死接口。
4. 文案/规格/实现有多处表述矛盾。
5. 项目知识库与交付状态未收尾，测试对若干关键守卫仍停留在旧 `isAdmin` 语义。

建议按 **P0 → P1 → P2** 修完后再做完整验证与交付。

---

## 2. 实现地图（便于对照）

| 层级 | 关键路径 | 作用 |
| --- | --- | --- |
| 数据 | `backend/migrations/177_add_sub_admin_permissions.sql`、`backend/ent/schema/user.go` | `users.admin_permissions JSONB` |
| 权限核心 | `backend/internal/service/admin_permission.go` | 权限码、目录、路由白名单 |
| 鉴权 | `backend/internal/server/middleware/admin_auth.go` | admin 全量 / sub_admin 白名单 |
| 用户管理 | `backend/internal/service/admin_user.go`、`handler/admin/user_handler.go` | 创建/更新角色与权限、清理 stale 权限 |
| 精简筛选 | `handler/admin/usage_handler.go` `SearchAccounts/SearchGroups` | 避免开放账号/分组管理列表 |
| 前端状态 | `frontend/src/stores/auth.ts`、`utils/adminPermissions.ts` | `isSubAdmin` / `canAccessAdmin` / landing path |
| 前端守卫 | `frontend/src/router/index.ts`、`App.vue`、`api/client.ts` | 路由 meta、403 刷新 |
| 页面收口 | `SubscriptionsView` / `UsageView` / `TokenAnalysisView` / `AppSidebar` | 按钮与菜单 |

---

## 3. 问题清单

### 3.1 P0 — 建议合并前处理

#### P0-1. 订阅权限复用完整 `GET /admin/groups/all`，破坏“最小暴露”设计

**现象**

- 规格明确：usage 要用精简筛选接口，避免开放账号管理列表。
- 实现也对 usage 新增了 `search-accounts` / `search-groups`，只返回 `{id,name}`。
- 但 `admin.subscriptions` 白名单直接放行：

```text
GET /api/v1/admin/groups/all
```

该接口返回的是完整 `dto.AdminGroup`，源码注释写明：

> AdminGroup 是管理员接口使用的 group DTO（包含敏感/内部字段）。

其中包含倍率、价格、模型路由、账号数量、调度配置等内部运营信息。

**为什么不合理**

同一需求里对 usage 做了数据最小化，对 subscriptions 却放行完整管理 DTO，属于**设计自相矛盾**。
子管理员只要有订阅权限，就能读到接近分组管理页的配置面，而规格写的是“不能管理账号/系统设置/自定义菜单”，并未授权查看完整分组运营配置。

**建议**

- 仿照 usage，新增 `GET /admin/subscriptions/search-groups`（或通用 compact groups）。
- 仅返回筛选所需字段：`id/name/platform/subscription_type/limits` 等。
- 将 `groups/all` 从 sub_admin 白名单移除。
- 前端订阅页筛选改为走精简接口。

---

#### P0-2. 简易模式下，任意 `sub_admin`（含空权限）绕过用户侧限制

**现象**

```ts
if (authStore.isSimpleMode && !authStore.isSubAdmin) {
  // 限制 /subscriptions、/redeem、/admin/groups 等
}
```

只要 `role === 'sub_admin'`，无论 `admin_permissions` 是否为空，都跳过简易模式限制。

**设计矛盾**

| 角色 | 简易模式下 `/subscriptions`、`/redeem` |
| --- | --- |
| `user` | 不可访问 |
| `sub_admin` + 空权限 | **可访问** |
| `sub_admin` + 仅 token_analysis | **可访问**（与订阅权限无关） |

规格说：

- 空权限子管理员在标准模式仍可进用户首页；
- 子管理员保留普通用户能力；
- 未说明“升级为简易模式特权用户”。

结果是：管理员只要把某人设成空权限 `sub_admin`，就等于绕过 simple mode 对普通用户的产品限制。这是角色语义泄漏，不只是文案问题。

**建议**

1. simple mode 判断改为基于能力，而不是裸角色：
   - 管理页：`canAccessAdmin && hasAdminPermission(...)`
   - 用户页限制：不要因 `isSubAdmin` 一律放行
2. 或禁止创建/保存“空权限 sub_admin”，强制至少 1 个权限；空权限一律归一成 `user`。
3. 补单测：`simple mode + empty sub_admin` 不得访问 `/subscriptions`、`/redeem`。

---

#### P0-3. 权限目录三处维护，catalog API 对前端是死代码

**现象**

| 位置 | 内容 |
| --- | --- |
| 后端 `admin_permission.go` | 权限码 + 路由白名单 + catalog |
| 前端 `utils/adminPermissions.ts` | 权限码 + landing path |
| 前端用户弹窗 | 直接读前端 catalog 渲染复选框 |

后端已提供：

```text
GET /api/v1/admin/permissions/catalog
```

但前端用户创建/编辑弹窗**完全没有调用**，目录被硬编码。

**问题**

- 规格把 catalog API 写成契约的一部分，实现却让它变成“只有手工 curl 才用到”的接口。
- 新增权限时要同时改后端白名单、后端 catalog、前端 catalog、路由 meta、侧边栏、i18n；任何一处漏改都会前后端漂移。
- 这不是安全漏洞，但是高概率的长期维护事故源。

**建议**

二选一，不要半吊子：

1. **推荐**：前端管理配置 UI 调用 catalog API；前端只保留路由 meta / landing 所需最小映射。
2. 或者删除/降级 catalog API，明确“权限码为前后端编译期常量”，并加 CI 校验两边常量一致。

---

### 3.2 P1 — 明显不合理 / 易踩坑

#### P1-1. 文案声称“只读”，实现明确允许写操作

中文提示：

> 子管理员只能进入已勾选的管理页面，并使用对应的**只读接口**。

但：

- 规格与实现都允许 `POST /admin/subscriptions/:id/reset-quota`
- 权限描述本身也写了“可重置全部配额或仅重置日限”

这是**产品文案与真实授权模型矛盾**，会误导管理员授权。

**建议**

改成类似：

> 子管理员只能访问已勾选菜单；默认只读，订阅权限额外允许配额重置。

---

#### P1-2. `admin.usage` 权限面明显大于“使用记录页”

白名单除 usage 列表/统计外，还包括：

- dashboard 聚合：`snapshot-v2`、`models`、`user-breakdown`、`users-ranking`
- ops 错误链路：`ops/errors`、`request-errors`、`upstream-errors` 及其详情

这与规格一致，但和权限名 `admin.usage`、菜单名“使用记录”的直觉范围不一致。
拿到 usage 权限的人，实际上接近只读运营分析员，而不是“只能看 usage 表”。

**建议**

- 若业务接受：在权限描述/管理员提示中写清“含仪表盘聚合与错误详情”。
- 若业务不接受：拆权限，或把 dashboard/ops 从 usage 中移出。

当前最大风险不是“能不能跑”，而是**管理员误以为授权很窄**。

---

#### P1-3. Handler 层权限校验语义不正确

`user_handler.go` 创建/更新时固定：

```go
NormalizeAdminPermissions(service.RoleSubAdmin, req.AdminPermissions)
```

不管目标角色是 `admin` / `user` / `sub_admin`。

Service 层才会用真实 role 再归一化一次。

**问题**

- 双次校验，职责重复。
- Handler 用错误角色做校验，语义漂移；今天碰巧能用，是因为未知码在任何角色下都报错，非 sub_admin 最终会被 service 清空。
- 非法角色组合的错误信息不够贴近真实意图。

**建议**

- Handler 只做 transport 校验（JSON/binding）。
- 角色+权限归一化只留在 service。
- 或 handler 传入真实 `req.Role`（更新时还要考虑空 role 表示不改）。

---

#### P1-4. 前端多处仍按“只有 admin 才是管理端用户”编码

已改成 `canAccessAdmin` 的地方：

- 登录跳转、requiresAdmin 入口、合规弹窗、backend mode 登录门闸

仍只认 `isAdmin` 的地方：

| 位置 | 影响 |
| --- | --- |
| `router/index.ts` payment/risk 回退跳转 | 子管理员被送到 `/dashboard` 或 `/admin/settings`（后者其无权限） |
| `setupRedirect.ts` | setup 完成后子管理员不进授权管理页 |
| `App.vue` / `i18n/index.ts` 自定义菜单 | 子管理员永远看不到 admin 自定义菜单（规格范围外，可接受，但要确认） |
| `router/__tests__/guards.spec.ts` | 测试仍复制旧 isAdmin 逻辑，**不能回归 sub_admin 守卫** |

这不是单一 bug，而是“角色模型升级不彻底”。

---

#### P1-5. 权限撤销后 backend mode 可能停在“已登录的 /login”脏状态

`onAdminPermissionDenied` 在 backend mode 下：

1. `refreshUser()`
2. `router.replace('/login')`

但**没有 logout / 清 token**。

配合守卫：

```ts
if (backendModeEnabled && !canAccessAdmin) next() // 留在 login
```

会出现：本地仍持有 JWT，页面在 `/login`，用户已登录但不能进任何 backend 页面。
功能上未必坏，但体验和会话语义很怪；刷新 token 时也会被 backend mode 拒绝。

**建议**

权限被清空时：backend mode 直接 `logout()` 再去登录页；标准模式保留用户会话并回 `/dashboard`。

---

#### P1-6. 白名单与页面契约缺少“防漂移”测试

现有测试覆盖了：

- 若干允许/拒绝样例
- 权限撤销后 DB 生效
- 前端三个路由 meta
- 部分按钮隐藏

但没有：

- “订阅/使用/Token 分析页面实际会调用的 API 全集 ⊆ 白名单”
- “白名单中的写接口 ⊆ 规格允许的写接口”
- “前端 catalog ⊆ 后端 catalog”

后续加一个筛选 API 时，很容易出现“页面 403 但菜单还在”的回归。

---

### 3.3 P2 — 死代码 / 非最佳实践 / 收尾缺口

#### P2-1. `AdminOnly` 中间件是死代码

`backend/internal/server/middleware/admin_only.go` 只允许 `RoleAdmin`，且全仓库路由未引用。
本次也没有把它接入 sub_admin 模型。

建议删除或改造成可复用的 role guard，并补测试；否则后续有人误用会与 `AdminAuth` 双轨冲突。

---

#### P2-2. 前端硬编码权限，后端 catalog 无客户端消费者

见 P0-3。从“可运行代码”角度看，catalog handler 近似死接口；从“规格完成度”看，属于实现不完整。

---

#### P2-3. `delivery-status.md` 与真实进度不符

状态文件仍写：

- 实现进行中
- QA/验证未开始
- 下一检查点还是 RED/GREEN

但代码已大面积落地。交付文档失真，后续审查者会被误导。

---

#### P2-4. 未更新 `llm-wiki`

按 `Agents.md` / `llm-wiki/wiki/ai-workflow.md`：

> 新增或修改认证、权限……必须更新 llm-wiki

当前 `llm-wiki` 仍只有 admin/user 二元模型描述，没有 `sub_admin`、权限码、白名单机制。
这会让后续 AI/开发者继续按旧权限模型改代码。

建议至少更新：

- `llm-wiki/wiki/security-and-reliability.md`
- 必要时补 `frontend.md` 路由 meta / auth store 说明

---

#### P2-5. 空权限 `sub_admin` 角色价值存疑

标准模式：

- `canAccessAdmin = false`
- 无管理菜单
- 行为几乎等于 `user`
- 但多了 simple mode 旁路（P0-2）和特殊侧边栏分支

建议产品上明确：

- 不允许空权限 sub_admin；或
- 空权限自动降级为 user。

---

#### P2-6. 小实现味道

| 项 | 说明 |
| --- | --- |
| 权限匹配线性扫描 | 目前 3 个权限可接受；权限变多后应改为 `map[method]map[route]struct{}` |
| `SearchGroups` 固定 `pageSize=1000` | 与旧实现一致，大数据量时仍粗暴 |
| `SearchAccounts` 固定 30 条 | 可接受，但无分页/排序说明 |
| 多处 `data: any` / 弱类型更新 payload | 用户编辑弹窗仍有 `const data: any` |
| 审计日志用 `LegacyPrintf` | 能追，但没有结构化 audit event，检索成本高 |
| `notesHint: 仅管理员可见` | 现在系统已有 sub_admin，文案未同步（次要） |

---

## 4. 设计矛盾对照表

| 主题 | 规格/文案 | 实现 | 判定 |
| --- | --- | --- | --- |
| 默认拒绝 | 未登记接口拒绝 | `CanAccessAdminRoute` 默认 false | 一致 |
| 最新权限生效 | 以 DB 为准 | `AdminAuth` 每次 `GetByID` | 一致 |
| usage 最小暴露 | 精简账号/分组筛选 | 已做 compact API | 一致 |
| subscriptions 最小暴露 | 未明确要求 compact，但同需求强调避免管理列表 | 仍开放完整 `groups/all` | **矛盾** |
| 只读为主 | 文案写只读 | 允许 reset-quota | **文案矛盾** |
| 保留用户能力 | 标准模式保留 | 基本成立；simple mode 被放大 | **部分矛盾** |
| catalog API | 规格要求 | 后端有、前端不用 | **契约半实现** |
| 空权限 sub_admin | 标准模式可进用户首页 | 成立，但带来 simple mode 旁路 | **语义不干净** |
| 文档沉淀 | Agents 要求更新 wiki | 未更新 | **流程缺口** |

---

## 5. 做得好的地方

1. **安全默认值正确**：未知路由拒绝，不按 GET 泛化放行。
2. **权限撤销即时生效**：middleware 测到 DB 权限被清空后立即 403。
3. **离开 sub_admin 清权限**：避免 stale permissions 复用。
4. **最后一名 admin 保护**扩展到“降级为 sub_admin”路径，而不是只防降为 user。
5. **usage 精简筛选**方向正确，值得作为其他权限依赖接口的模板。
6. **前端 403 专用码** `ADMIN_PERMISSION_DENIED` + 事件刷新，体验闭环完整。
7. **页面写操作收口**较完整：清理、立即索引、分配/延期/撤销/恢复都按 admin 隐藏。
8. **测试不是零**：middleware / service / migration / 部分 Vue 组件都有覆盖。

---

## 6. 建议修复顺序

### 第一批（合并前）

1. 订阅分组筛选改为 compact API，移出 `groups/all`。
2. 修复 simple mode 对 `isSubAdmin` 的一刀切放行；处理空权限角色语义。
3. 统一权限目录来源（接 catalog API 或删 API + 加一致性校验）。
4. 修正“只读”文案与权限描述。
5. backend mode 权限清空时执行 logout。

### 第二批（合并前最好完成）

1. 清理 handler 双重 `NormalizeAdminPermissions`。
2. 补齐 router 中 payment/risk/setup 等对 sub_admin 的跳转语义。
3. 增加“页面 API ⊆ 白名单”契约测试。
4. 更新 `llm-wiki` 与 `delivery-status.md`。

### 第三批（可后续）

1. 处理 `AdminOnly` 死代码。
2. 结构化审计日志。
3. 评估是否拆分 `admin.usage` 的 dashboard/ops 权限。

---

## 7. 建议补充的验证

```powershell
# 前端
cmd.exe /c pnpm --dir frontend exec vitest run src/stores/__tests__/auth.spec.ts src/router/__tests__/subAdminRoutes.spec.ts src/components/admin/user/__tests__/UserRolePermissions.spec.ts src/views/admin/__tests__/UsageView.spec.ts src/views/admin/__tests__/SubscriptionsView.spec.ts src/views/admin/__tests__/TokenAnalysisView.spec.ts
cmd.exe /c pnpm --dir frontend run typecheck

# 后端（按仓库 ops 约定设置 GOCACHE/GOMODCACHE/GOTMPDIR）
go test -tags=unit -p 1 -count=1 ./internal/server/middleware ./internal/service ./internal/handler ./internal/handler/admin
go test -p 1 -count=1 ./migrations
```

手工验收矩阵：

| 账号 | 模式 | 期望 |
| --- | --- | --- |
| admin | 任意 | 全菜单全接口 |
| sub_admin: subscriptions | 标准 | 仅订阅页；可 reset；不可 assign/revoke；不可读完整 groups 管理字段 |
| sub_admin: usage | 标准 | 可筛选用量；不可 cleanup；不可点用户进余额管理 |
| sub_admin: token_analysis | 标准 | 可读分析；无立即索引 |
| sub_admin: 空权限 | 标准 | 仅用户首页 |
| sub_admin: 空权限 | simple | **不应**比普通 user 多权限 |
| sub_admin: 任意权限 | backend | 可登录；只能进授权管理页 |
| sub_admin: 空权限 | backend | 不可登录 |
| 权限中途撤销 | 任意 | 下一次管理 API 403，前端回到安全页 |

---

## 8. 审查结论

| 维度 | 评级 | 说明 |
| --- | --- | --- |
| 安全骨架 | 良好 | DB 权威 + 白名单默认拒绝是对的 |
| 最小权限/最小暴露 | 中等偏弱 | usage 做好了，subscriptions 的 `groups/all` 明显过宽 |
| 角色语义一致性 | 中等偏弱 | simple mode 与空权限 sub_admin 有旁路 |
| 可维护性 | 中等 | 三处目录、缺少防漂移测试 |
| 规格符合度 | 中高 | 主路径基本落地，catalog/文案/文档有缺口 |
| 可合并性 | **暂不建议直接合并** | 先处理 P0 与关键 P1 |

**一句话**：这是一个方向正确、主链路基本可用的 RBAC 雏形；真正拖后腿的不是“能不能做子管理员”，而是**权限边界没有在所有依赖接口上贯彻最小暴露，以及 sub_admin 角色在 simple mode / 空权限场景下语义不干净**。

---

## 9. 实施处理记录

| 审查项 | 处理 | 结果 |
| --- | --- | --- |
| P0-1 订阅读取完整分组 | 已修复 | 新增 `/admin/subscriptions/search-groups` compact 入口，移除子管理员对 `/admin/groups/all` 的白名单 |
| P0-2 simple mode 角色旁路 | 已修复 | simple mode 用户侧受限路径不再因 `isSubAdmin` 绕过；空权限与有权限子管理员均有真实 guard 测试 |
| P0-3 catalog API 无消费者 | 已修复 | 创建/编辑弹窗改从 `GET /admin/permissions/catalog` 加载，前端只保留 backend landing 最小路由顺序 |
| P1-1 只读文案矛盾 | 已修复 | 中英文提示明确“默认只读，订阅权限额外允许配额重置” |
| P1-2 usage 能力范围不直观 | 已修复文案 | 权限描述明确包含排行、Dashboard 聚合和错误详情；授权范围保持已批准规格 |
| P1-3 handler 双重校验 | 已修复 | 权限归一化只留 service，未知权限返回 typed 400 `UNKNOWN_ADMIN_PERMISSION` |
| P1-4 旧 `isAdmin` 语义 | 已核对 | payment/risk 回退在子管理员权限守卫之后不可达；管理员自定义菜单属于规格明确拒绝范围。实际 router harness 已补子管理员模式测试 |
| P1-5 backend 脏登录态 | 已修复 | 权限刷新后无剩余授权时先 `logout()` 再跳 `/login`; refresh token 同样按最新权限判断 |
| P1-6 防漂移测试 | 已补充 | 增加 compact/完整接口允许拒绝、写白名单集合、catalog client 与页面路由/按钮测试 |
| P2-1 `AdminOnly` 死代码 | 已处理 | 删除未被任何路由引用的旧中间件，避免与 `AdminAuth` 双轨 |
| P2-3/P2-4 状态与 wiki | 已处理 | 更新 delivery 状态、backend/frontend/data/security wiki 和管理组件 README |
| P2-5 空权限角色 | 保留设计 | 按已批准规格，标准模式允许空权限子管理员作为普通用户使用；backend 模式仍拒绝登录，不自动改写角色 |
| P2-6 小实现味道 | 部分收口 | 编辑 payload 去除 `any` 并补 `rpm_limit` 类型；3 项固定权限的线性匹配保留，待权限规模扩大后再改索引结构 |
| 补充审查：shallow 用户 DTO 暴露权限 | 已修复 | `admin_permissions` 移到完整 `UserFromService` 映射；API Key、订阅、兑换码和用量日志等嵌套 shallow 用户不再携带权限数组，并补 RED/GREEN 测试 |
