# 子管理员角色设计

更新：2026-11-04。状态：**已上线（2026-07-15 首版，分支 `feature/hy/10156_新增子管理员角色`），此后权限码从 3 项扩到 8 项；本文按 HEAD `26f6363af` 的源码校对。**

本文是子管理员 `sub_admin` 角色基础机制的唯一文档：角色模型、权限码、后端白名单鉴权、用户管理接口、前端菜单与路由守卫、易错点和验证方式。2026-07 的需求/规格/计划/测试记录/交付报告和两份代码审查原文已移出仓库，最后存在于提交 `26f6363af`（可用 `git show 26f6363af:docs/delivery/2026-07-15-sub-admin-role/spec.md`、`git show 26f6363af:docs/reviews/2026-07-15-sub-admin-role-code-review.md` 等查看）；凡是对后续修改有约束的结论都已写入正文。

建立在本机制上的专项功能各有独立文档，本文只给入口，不复制其规则：
- 组织负责人与部门权限：[组织负责人权限设计](organization-leader-design-cn.md) §2（角色体系）、§3（权限模型与范围计算）；
- 受限账号运维 `admin.accounts_operator`：[子管理员受限账号运维设计](sub-admin-accounts-operator-design-cn.md)；
- 订阅按筛选批量日重置：[订阅每日剩余排序与批量重置设计](subscription-daily-remaining-sort-bulk-reset-design-cn.md)。

阅读顺序建议：
- 新增或修改权限码：先读 §3、§4，再读 §9 的检查清单；
- 修改用户创建/编辑：读 §5；
- 修改前端菜单、路由或 403 处理：读 §7；
- 定位代码：读 §8；跑测试：读 §10。

## 1. 需求与范围

原始需求（2026-07-15）：在不改变完整管理员和普通用户行为的前提下，新增账号级 `sub_admin` 角色。完整管理员可以给子管理员勾选部分管理菜单；子管理员只能调用对应接口，菜单之外一律拒绝。

| 使用者 | 需求 |
| --- | --- |
| 完整管理员 | 创建或编辑子管理员，配置其管理菜单权限 |
| 子管理员 | 保留普通用户能力，并访问获授权的管理页面 |
| 普通用户 | 行为不变，不显示管理菜单 |

验收标准（首版 AC-1～AC-8，均已通过，仍是回归口径）：

| 编号 | 结果 |
| --- | --- |
| AC-1 | 完整管理员始终可见并可访问全部管理菜单和接口 |
| AC-2 | 子管理员只显示、只能访问已授权菜单；直链和手工调接口同样受限 |
| AC-3 | 订阅权限只开放 §3 列出的写接口，其他订阅写操作拒绝 |
| AC-4 | 使用记录权限只读，不开放清理 |
| AC-5 | Token 分析权限只读，不开放"立即索引" |
| AC-6 | 普通用户行为不变，所有管理接口拒绝 |
| AC-7 | 撤销权限在下一次请求立即生效；未知权限码返回 400 |
| AC-8 | 迁移、角色、权限、鉴权、审计和前端权限行为有自动化测试 |

范围外（至今仍成立）：子管理员访问系统设置、权限配置、风控、请求拦截、管理员自定义菜单；自定义权限码、角色组、Casbin 式通用 RBAC。账号管理只经 `admin.accounts_operator` 受限开放；组织/部门级范围由组织负责人功能另行实现。

## 2. 角色模型

`users.role` 取值：`admin`、`sub_admin`、`user`（`backend/internal/domain/constants.go`，前端 `UserRole`）。

| 角色 | 用户侧菜单 | 管理菜单 | 管理接口 |
| --- | --- | --- | --- |
| `admin` | 可见 | 全部 | 全部允许 |
| `sub_admin` | 标准模式全部可见 | 仅已授权菜单（侧边栏"管理功能"分区） | 仅白名单（§3、§4） |
| `user` | 可见 | 不可见 | 全部拒绝（`403 FORBIDDEN`） |

- 能否进入管理端：`CanAccessAdmin` = `admin`，或 `sub_admin` 且权限数组非空。前端 `canAccessAdmin` 同口径。
- 空权限 `sub_admin` 合法：标准模式下等同普通用户（进 `/dashboard`），backend 模式下不能登录。已否决：强制"子管理员至少勾选一项"或"空权限自动降为 `user`"，原因：首版规格已批准空权限作为普通用户使用；其带来的 simple mode 旁路已在前端守卫按能力判断后关闭（§9 第 6 条）。
- 只有 `sub_admin` 可以持有权限码；角色改为 `admin` 或 `user` 时服务端把权限清空（`NormalizeAdminPermissions`），同时清掉部门授权（`clearUserDepartmentGrants`，原因码 `role_changed`）。
- 新增权限码不会自动授予存量子管理员。

## 3. 权限码

权限码与白名单只在 `backend/internal/service/admin_permission.go` 定义（`adminPermissionCatalog`、`adminPermissionRouteRules`）。目录顺序即 `NormalizeAdminPermissions` 输出顺序。当前 8 项：

| 权限码 | 菜单 key / 前端路由 | 能力 | 允许的写接口 |
| --- | --- | --- | --- |
| `admin.subscriptions` | `subscriptions` / `/admin/subscriptions` | 全站订阅：列表、详情、进度、按用户/分组查询；compact 筛选 `search-users`、`search-groups`；分配弹窗 `assignable-groups`；历史用户筛选 `usage/search-users`；`subscriptions/scope` | `POST subscriptions/assign`、`bulk-assign`、`:id/reset-quota`、`reset-daily-filtered` |
| `admin.usage` | `usage` / `/admin/usage` | 使用记录列表、统计、精简筛选（用户/API Key/账号/分组）、导出；Dashboard 聚合 `snapshot-v2`、`models`、`user-breakdown`、`users-ranking`；Ops 错误只读（`ops/errors`、`request-errors`、`upstream-errors` 及详情） | 无 |
| `admin.token_analysis` | `token_analysis` / `/admin/token-analysis` | summary、users、projects、requests、requests/input、index/status、archive-files、`dashboard/users-trend` | 无 |
| `admin.organization_usage` | `organization_usage` / `/admin/organization-usage` | 获授权部门用量报表（scope、summary、periods、trend） | 无 |
| `admin.department_subscriptions` | `department_subscriptions` / `/admin/subscriptions` | 获授权部门成员的订阅查询；仓储层按部门范围收口 | `POST subscriptions/:id/reset-quota`、`reset-daily-filtered` |
| `admin.department_members` | `department_members` / `/admin/my-department` | 部门成员管理专用五路由（`/admin/department-members/*`） | `POST department-members/assign`、`PUT department-members/:id` |
| `admin.organization_manage` | `organization_manage` / `/admin/departments` | 组织负责人：维护本组织部门、任命部门负责人（规则见组织负责人文档 §2/§3） | `POST departments`、`PUT departments/:id`、`POST departments/assign-members`、`PUT organization/leaders/:id` |
| `admin.accounts_operator` | `accounts` / `/admin/accounts` | 受限账号运维：账号全量查看 + 新增、测试连接、重新授权（规则见受限账号运维文档） | 新增、OAuth 换码、`:id/test`、`:id/apply-oauth-credentials` 等，见该文档 §2 |

表中路径均省略前缀 `/api/v1/admin/`。所有子管理员额外共享 `subAdminCommonRouteRules`：`GET /admin/compliance`、`POST /admin/compliance/accept`（完成管理端合规确认，不代表任何业务权限）。

权限组合规则（`NormalizeAdminPermissions`）：
- 未知权限码 → `400 UNKNOWN_ADMIN_PERMISSION`，不写库；空串和重复项被丢弃。
- `admin.subscriptions` 与 `admin.department_subscriptions` 互斥 → `400 CONFLICTING_ADMIN_PERMISSIONS`。前端勾选一项时自动取消另一项。
- 持有 `admin.organization_manage` 必须同时持有三项部门权限（`OrganizationLeaderDepartmentPermissions`），否则 `400 ORGANIZATION_SCOPE_ENDPOINT_REQUIRED`；三项部门权限本身不代表组织负责人。

各权限的边界说明：
- `admin.usage` 的实际权限面大于"使用记录页"：含 Dashboard 聚合与 Ops 错误详情，i18n 描述已写明。已否决：在首版把 Dashboard/Ops 从 `admin.usage` 拆出，原因：已批准规格如此、页面依赖这些接口；若产品要收窄，单独立项拆权限，不要临时删白名单导致页面半残。
- `admin.subscriptions` 首版只允许重置；2026-09 随订阅自助日重置一起开放了分配（`assign`、`bulk-assign`、`assignable-groups`）。延期、撤销、恢复、删除和 `bulk-action` 仍拒绝。
- 以下接口不在任何白名单（只给完整管理员）：使用记录清理、Token 立即索引、`/admin/permissions/catalog`、`/admin/users*`、`/admin/groups/all`、`/admin/proxies/all`、订阅自助策略 `self-reset-policy`（GET/PUT）与 `self-reset-events`、`/admin/gpt-quota*`、系统设置与自定义菜单。

## 4. 后端鉴权

`AdminAuth`（`backend/internal/server/middleware/admin_auth.go`）对 `/api/v1/admin/**` 统一生效：

1. Admin API Key（`x-api-key`）：传统全权 key 映射到第一个管理员，绕过子管理员检查；`adm_sc_` 前缀的受限管理密钥走 `authenticateScopedAdminRequest` 和 `internal/pkg/adminaccess` 的独立白名单，与本文的 JWT 子管理员白名单互不相通。
2. JWT（`Authorization: Bearer`，WebSocket 用 `Sec-WebSocket-Protocol: jwt.<token>`）：`validateJWTForAdmin` 每次从数据库按 ID 读最新用户，依次检查 active 状态、`TokenVersion`、会话绑定。
3. 角色既不是 `admin` 也不是 `sub_admin` → `403 FORBIDDEN`（`Admin access required`）。
4. `CanAccessAdminRoute(user, method, c.FullPath())`：`admin` 直接通过；`sub_admin` 按 HTTP 方法 + Gin 路由模板逐条匹配公共规则和其持有权限的规则；未命中 → `403 ADMIN_PERMISSION_DENIED`。
5. 通过后在 context 写入用户、角色、`WithDepartmentActor`；子管理员额外写入 `WithSubAdminAccountRestrictions`（受限账号运维的副作用收口，见该文档）。

响应收口（白名单只管"能不能调"，不管"返回什么"）：
- 订阅接口经 `adminSubscriptionResponse` 投影：JWT 子管理员（全站或部门）只拿订阅用量字段，不返回 `assigned_by`、`assigned_at`、`notes`；嵌入 `user` 只留 `id/email/username/status`，`group` 只留标签和额度字段。
- `admin_permissions` 只在完整用户 DTO（`UserFromService`、`UserFromServiceAdmin`）里映射；`UserFromServiceShallow` 被 API Key、订阅、兑换码、用量日志等嵌套对象复用，不得带权限数组。
- 筛选依赖必须用 compact 接口：订阅页 `search-users`/`search-groups`（`DepartmentHandler.SubscriptionUsers/SubscriptionGroups`，全站权限为全站、部门权限限授权部门）、`assignable-groups`（`GroupHandler.SubscriptionAssignmentGroups`，仅全站订阅权限）；使用记录页 `usage/search-*`。

## 5. 用户管理接口合同

| 接口 | 权限 | 说明 |
| --- | --- | --- |
| `GET /api/v1/admin/permissions/catalog` | 仅完整管理员 | 返回 `[{code, menu_key, route}]`，用户创建/编辑弹窗从这里加载复选项 |
| `POST /api/v1/admin/users` | 仅完整管理员 | 请求增加 `role`、`admin_permissions: string[]`；不能直接创建带 `admin.organization_manage` 的账号 |
| `PUT /api/v1/admin/users/:id` | 仅完整管理员 | 修改 `role` 或 `admin_permissions` 时必须带 `expected_admin_access_version` |
| `PUT /api/v1/admin/users/:id/organization-scope` | 仅完整管理员 | 组织负责人唯一写入入口，见组织负责人文档 §5.1 |

规则：
- 角色与权限的归一化只在 service 层做（`CreateUser`、`UpdateUser` 调 `NormalizeAdminPermissions`），handler 只做绑定校验。
- 修改角色/权限需要乐观版本：缺少版本 → `400 ADMIN_ACCESS_VERSION_REQUIRED`；版本已变 → `409 ADMIN_ACCESS_CHANGED`（`prepareUserAdminAccessWrite`）。
- `admin.organization_manage` 的增删只能走 organization-scope；在普通更新里增减它 → `400 ORGANIZATION_SCOPE_ENDPOINT_REQUIRED`。前端创建弹窗不显示该项，编辑弹窗只读显示。
- 防锁死：管理员不能把自己降级（handler 返回 `cannot demote yourself from admin`）；降级最后一个管理员（含降为 `sub_admin`）→ `400 LAST_ADMIN_DEMOTE_FORBIDDEN`；`admin` 不能被禁用（`ADMIN_DISABLE_FORBIDDEN`）。
- 把非管理员提升为 `admin` 需要 step-up 2FA；提升为 `sub_admin` 不需要。
- 创建特权用户、变更角色、变更权限写审计日志（`audit: privileged user created`、`user role changed`、`user admin permissions changed`）；角色/权限变化同时失效认证缓存（`InvalidateAuthCacheByUserID`），部门授权版本变化写 `department.user_access_changed`。

## 6. 数据模型与迁移

- `backend/migrations/177_add_sub_admin_permissions.sql`：`users.admin_permissions JSONB NOT NULL DEFAULT '[]'::jsonb`，存量用户为空数组。
- Ent：`backend/ent/schema/user.go` 字段 `admin_permissions`；服务层 `User.AdminPermissions []string`。
- 回滚：回退代码即可；列为非空默认空数组，旧版本读取不受影响；已执行迁移的生产库不做破坏性降级。

## 7. 前端

- 类型：`frontend/src/types/index.ts` 的 `UserRole`、`AdminPermission`（8 个字面量，必须与后端目录一致）。
- Auth store（`frontend/src/stores/auth.ts`）：`isSubAdmin`、`canAccessAdmin`、`hasAdminPermission`（`admin` 恒为 true）。
- 路由 meta（`frontend/src/router/index.ts`、`meta.d.ts`）：`requiresAdmin` + `adminPermission`，或 `adminPermissionsAny`（订阅页同时接受 `admin.subscriptions` 与 `admin.department_subscriptions`）。子管理员进入没有 `adminPermission` 的管理路由（如 GPT 额度展示、`/admin/users`）时被送回落地页。
- 落地页 `getAdminLandingPath`（`frontend/src/utils/adminPermissions.ts`）：按 `ADMIN_PERMISSION_LANDING_ROUTES` 顺序取第一个已授权页面（组织负责人优先 `/admin/departments`）。标准模式下，只持有部门管理类权限（组织用量、部门订阅、部门成员、组织负责人）的子管理员进第一个授权页，其他子管理员进 `/dashboard`；backend 模式进第一个授权页，没有则 `/login`。`departmentLoginDestination` 在登录完成后套用这个规则（显式 `redirect` 优先）。
- 侧边栏（`AppSidebar.vue` 的 `subAdminNavItems`）：只列已授权菜单。
- 用户管理（`UserCreateModal.vue`、`UserEditModal.vue`）：角色下拉含"子管理员"，权限复选框来自 catalog API；列表支持按角色筛选和角色徽章。
- 专用 403 恢复：`api/client.ts` 收到 `403 ADMIN_PERMISSION_DENIED` 时派发 `admin-permission-denied` 事件；`App.vue` 的 `onAdminPermissionDenied` 刷新用户后调用 `resolveAdminPermissionDeniedRecovery`：子管理员回落地页；backend 模式下已无任何权限则先 `logout()` 再去 `/login`。
- simple mode 的受限路径（`/subscriptions`、`/redeem` 等）对所有人生效，不因 `isSubAdmin` 放行。
- 页面收口：订阅页只显示允许的写操作；使用记录页隐藏清理和用户后台详情跳转；Token 分析页隐藏"立即索引"。按钮隐藏只是体验，后端独立拒绝。

## 8. 代码地图

| 层 | 位置 |
| --- | --- |
| 角色常量 | `backend/internal/domain/constants.go`（`RoleSubAdmin`），`backend/internal/service/user.go`（`User.IsSubAdmin`） |
| 权限目录与白名单 | `backend/internal/service/admin_permission.go`：`adminPermissionCatalog`、`adminPermissionRouteRules`、`subAdminCommonRouteRules`、`OrganizationLeaderDepartmentPermissions`、`NormalizeAdminPermissions`、`HasAdminPermission`、`CanAccessAdmin`、`CanAccessAdminRoute`、`AdminPermissionRouteTemplates` |
| 鉴权 | `backend/internal/server/middleware/admin_auth.go`：`adminAuth`、`validateJWTForAdmin`、`authenticateScopedAdminRequest` |
| 用户管理 | `backend/internal/service/admin_user.go`：`normalizeUserRole`、`CreateUser`、`UpdateUser`、`ensureNotLastAdmin`；`backend/internal/handler/admin/user_handler.go`：`Update`、`GetPermissionCatalog` |
| 持久化 | `backend/internal/repository/user_repo.go`（`Update` 中的角色/权限写入）、`user_admin_access.go`（`prepareUserAdminAccessWrite`、`clearUserDepartmentGrants`）、`department_repo.go`（`loadAdminAccess`、`departmentAudit`） |
| 响应投影 | `backend/internal/handler/admin/scoped_admin_subscription_response.go`（`adminSubscriptionResponse`）；`backend/internal/handler/dto/mappers.go`（`UserFromServiceShallow`、`UserFromService`、`UserFromServiceAdmin`） |
| 路由 | `backend/internal/server/routes/admin.go`（`/permissions/catalog`、`registerSubscriptionRoutes`）、`routes/department.go` |
| 前端 | `frontend/src/utils/adminPermissions.ts`、`stores/auth.ts`、`router/index.ts`、`router/meta.d.ts`、`api/client.ts`、`App.vue`、`components/layout/AppSidebar.vue`、`components/admin/user/UserCreateModal.vue`、`UserEditModal.vue`、`api/admin/users.ts`（`getPermissionCatalog`） |

## 9. 关键约束与易错点

1. **以数据库为准，不信任 JWT 或前端状态。** `validateJWTForAdmin` 每次查库；撤权后下一次管理请求即 403。依据：首版 middleware 测试覆盖"DB 权限被清空后立即拒绝"。
2. **默认拒绝，按方法 + 路由模板匹配。** 已否决：对子管理员"GET 一律放行"，原因：GET 也会暴露完整管理 DTO（见第 3 条）。匹配键必须是 `c.FullPath()` 模板，不是实际 URL。
3. **筛选依赖只用 compact 接口。** 曾复现：首版 `admin.subscriptions` 放行 `GET /admin/groups/all`，子管理员能读到含倍率、价格、模型路由、调度配置的完整 `AdminGroup`；已改为 `subscriptions/search-groups`。`/admin/proxies/all` 含代理密码，同样不得为筛选放行。
4. **白名单只保证"能调"，不保证"无副作用、不泄露"。** 新开放的接口要同时检查响应 DTO（参照 `adminSubscriptionResponse` 投影）和隐藏副作用（参照 `WithSubAdminAccountRestrictions`）。
5. **`admin_permissions` 只在完整用户 DTO。** 曾复现：`admin_permissions` 映射在 `UserFromServiceShallow` 时，API Key、订阅等嵌套用户都带出权限数组（RED 测试得到 `[]string{"admin.usage"}`）；现由 `TestUserFromServiceShallowOmitsAdminPermissions` 守护。
6. **simple mode 限制按能力判断，不按角色放行。** 曾复现：首版守卫写成 `isSimpleMode && !isSubAdmin`，空权限或只有 Token 分析权限的子管理员能进 `/subscriptions`、`/redeem`；`feature-access.spec.ts` 已有回归用例。
7. **backend 模式撤权要结束会话。** 曾复现：权限清空后只 `refreshUser()` 再跳 `/login`，本地仍持有 JWT，形成"已登录却只能停在登录页"的脏状态；现由 `resolveAdminPermissionDeniedRecovery` 返回 `logout: true`。
8. **权限目录只有一个来源。** 用户弹窗必须从 catalog API 取选项；前端只保留类型字面量、路由 meta、落地顺序、侧边栏和 i18n。曾复现：首版前端硬编码目录，catalog API 无消费者。
9. **归一化只在 service 层。** 曾复现：handler 固定用 `RoleSubAdmin` 预校验权限，与 service 二次校验语义不一致；已删除 handler 预校验，未知码统一 `UNKNOWN_ADMIN_PERMISSION`。
10. **新增管理接口或权限码的检查清单**：后端 `adminPermissionCatalog`/`adminPermissionRouteRules`；`admin_permission_test.go` 的目录项数（当前 8）与写白名单 `TestSubAdminWriteWhitelistStaysNarrow`；路由注册一致性 `TestSubAdminPermissionWhitelistMatchesRegisteredAdminRoutes`；前端 `AdminPermission` 类型、路由 meta、`ADMIN_PERMISSION_LANDING_ROUTES`、`subAdminNavItems`、i18n（`admin/overview.ts` 的权限描述）；账号运维写路由还要在 `accountOperatorActivityRoutes` 归类。漏掉任何一处会出现"菜单可见、请求 403"或前后端漂移。
11. **i18n 文案不能写"只读"。** 首版曾写"只读接口"误导授权；现 `permissionsHint` 为"默认只读，订阅权限额外允许配额重置"，但订阅权限 2026-09 起还允许分配，文案未跟上（§11 待办）。改写权限面时同步改这条提示和各权限描述。
12. **旧 `AdminOnly` 中间件已删除**，不要恢复第二套管理鉴权；所有管理路由只走 `AdminAuth`。

## 10. 验证

在 `backend/` 下：

```bash
# 权限目录、白名单、写接口集合（admin_permission_test.go 只在 unit 标签下编译）
go test -tags=unit -count=1 ./internal/service -run 'NormalizeAdminPermissions|CanAccessAdmin|AdminPermissionCatalog|SubAdminWriteWhitelist|OrganizationManageRouteWhitelist|AccountOperatorWrites'
# 白名单路由必须已注册（无构建标签）
go test -count=1 ./internal/server/routes -run TestSubAdminPermissionWhitelistMatchesRegisteredAdminRoutes
# 鉴权、DTO
go test -tags=unit -count=1 ./internal/server/middleware ./internal/handler/dto
go test -count=1 ./migrations
```

在 `frontend/` 下：

```bash
pnpm exec vitest run src/utils/__tests__/adminPermissions.spec.ts src/stores/__tests__/auth.spec.ts src/router/__tests__/subAdminRoutes.spec.ts src/router/__tests__/feature-access.spec.ts src/components/admin/user src/components/layout
pnpm typecheck
```

最近结果：2026-11-04（HEAD `26f6363af`，本机，`-tags=unit`）：service、repository、server/routes、handler/admin、handler/dto 五包的权限与订阅定向测试通过（筛选含 `NormalizeAdminPermissions|CanAccessAdmin|AdminPermissionCatalog|SubAdminWriteWhitelist|OrganizationManageRouteWhitelist|AccountOperator|TestSubAdminPermissionWhitelistMatchesRegisteredAdminRoutes|TestUserFromServiceShallowOmitsAdminPermissions` 等）；前端未重跑。

首版验收结果（2026-07-15）：7 个 Go 包与 migration 通过；前端 178 个 Vitest 文件 / 1167 个测试、typecheck、lint 通过；浏览器角色矩阵（admin、三种单权限、空权限）用本机 mock API 通过。后续权限扩展的验证结果见各专项文档。

## 11. 状态与待办

- [ ] 测试环境真实账号 smoke（首版浏览器矩阵用的是 mock API，订阅夹具为空列表，未实际点击重置按钮）：按订阅、使用记录、Token 分析各建一个子管理员，验证允许与拒绝动作，以及同会话撤权后的 403 与跳转。
- [ ] `/setup` 完成后的跳转（`resolveCompletedSetupRedirectPath`）和 Model Plaza 关闭时的回退仍只认 `isAdmin`，子管理员会进 `/dashboard`；边缘路径，未修。
- [ ] "页面实际调用的 API ⊆ 白名单"的契约测试未建；目前只有"白名单 ⊆ 已注册路由"和"写接口集合"两项防漂移测试。
- [ ] 白名单仍是线性扫描；权限规模继续扩大时改为 `map[method]map[route]` 索引，或在路由注册处声明权限、消除双份字符串。
- [ ] 审计日志仍用 `LegacyPrintf` 文本行，未结构化（部门/组织授权变更已经由 `departmentAudit` 结构化记录）。
- [ ] 用户弹窗提示 `admin.users.permissionsHint` 只写"订阅权限额外允许配额重置"，漏了分配；文案待改。
- 未做：订阅"只读"与"可写"拆成两个权限码；需要"只能看不能改"的订阅子管理员时再拆。
