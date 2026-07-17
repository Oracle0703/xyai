# 子管理员角色与菜单权限设计规格

## 摘要

新增 `sub_admin` 角色。权限按账号保存，管理员可配置子管理员能够进入的管理菜单。子管理员默认保留普通用户能力；管理端采用后端白名单鉴权，未登记页面和接口默认拒绝。

## 当前状态

- 用户角色仅支持 `admin`、`user`。
- 管理菜单在 `frontend/src/components/layout/AppSidebar.vue` 静态维护。
- 前端管理路由统一依赖 `requiresAdmin`。
- 后端 `/api/v1/admin/**` 统一要求 `admin`。
- 菜单隐藏与后端接口没有细粒度权限映射。

## 目标行为

| 角色 | 用户菜单 | 管理菜单 | 管理接口 |
| --- | --- | --- | --- |
| `admin` | 可见 | 全部可见 | 全部允许 |
| `sub_admin` | 标准模式全部可见 | 仅账号已授权菜单 | 仅白名单接口 |
| `user` | 可见 | 不可见 | 全部拒绝 |

标准模式下子管理员默认进入 `/dashboard`。backend 模式下至少有一个管理权限才能登录，并进入第一个授权页面。

## 数据与接口契约

- 新增 `users.admin_permissions JSONB NOT NULL DEFAULT '[]'::jsonb`。
- 角色联合类型扩展为 `admin | sub_admin | user`。
- 用户 DTO、创建请求、更新请求增加 `admin_permissions: string[]`。
- 新增 `GET /api/v1/admin/permissions/catalog`，仅管理员可访问。
- 未知权限码返回 400；未授权接口返回 `403 ADMIN_PERMISSION_DENIED`。
- 用户离开 `sub_admin` 角色时清空权限；新增权限不会自动授予存量子管理员。

| 权限码 | 菜单 | 子管理员能力 |
| --- | --- | --- |
| `admin.subscriptions` | 订阅管理 | 查看、筛选、重置全部配额、仅重置日限 |
| `admin.usage` | 使用记录 | 查询、统计、排行、错误详情、导出 |
| `admin.token_analysis` | Token 分析 | 查看统计、项目、请求、请求输入和索引状态 |

## 后端授权规则

管理员和 Admin API Key 绕过权限检查。子管理员按 HTTP 方法与 Gin 路由模板匹配白名单，未知路由默认拒绝。

- 订阅权限允许订阅读接口、compact 用户/分组筛选及 `POST /admin/subscriptions/:id/reset-quota`; 分组筛选不得开放完整 `/admin/groups/all`。
- 使用记录权限允许使用统计、Dashboard 聚合、排行和 Ops 错误只读接口；使用精简账号/分组筛选接口，避免开放账号管理列表。
- Token 分析权限允许相关 GET 接口及用户趋势查询。
- `/admin/compliance` 允许已认证子管理员完成管理端合规确认。
- 使用记录清理、Token 立即索引、订阅分配/延期/撤销/恢复/删除全部拒绝。
- 账号、风控、请求拦截、系统设置、权限配置和管理员自定义菜单全部拒绝。

## 数据流

1. 管理员弹窗从 `GET /admin/permissions/catalog` 加载权限目录并创建或更新用户。
2. 服务层校验角色与权限码，非 `sub_admin` 角色清空权限。
3. Repository 将权限写入 `users.admin_permissions`，DTO 返回最新权限。
4. 每次管理请求由 `AdminAuth` 从数据库加载最新用户并确认角色、状态和 TokenVersion。
5. 管理员/API Key 直接放行；子管理员使用请求方法、Gin `FullPath()` 和权限白名单判断。
6. 前端从当前用户权限过滤管理菜单和路由；专用 403 触发用户刷新并跳转到仍可访问的首页。

## 失败处理与边界

| 场景 | 行为 |
| --- | --- |
| 未知权限码 | 创建/更新返回 400，不写入数据库 |
| 普通用户访问管理接口 | 返回既有管理员访问 403 |
| 子管理员访问未登记接口 | 返回 `403 ADMIN_PERMISSION_DENIED` |
| 权限被撤销 | 下一次后端请求立即拒绝，前端刷新权限并跳转 |
| 子管理员权限为空 | 标准模式仍可进入用户首页；backend 模式拒绝登录 |
| 用户离开 `sub_admin` | 权限数组清空，避免 stale 权限复用 |

## 前端行为

- 用户创建/编辑弹窗增加“子管理员”和权限复选清单。
- 用户列表支持子管理员筛选、角色徽章和中英文文案。
- Auth store 增加 `isSubAdmin`、`canAccessAdmin`、`hasAdminPermission`。
- 路由元信息增加 `adminPermission`，子管理员直链访问同样受限。
- 子管理员侧边栏增加“管理功能”分区，只显示已授权菜单。
- 订阅页隐藏除两种配额重置外的写操作。
- 使用记录页隐藏清理功能和用户后台详情跳转。
- Token 分析页隐藏“立即索引”。
- 权限撤销后，前端刷新用户权限并跳回可访问首页。

## 验收标准

验收标准以 `requirements.md` 的 AC-1 至 AC-8 为准，并通过后端单元测试、迁移测试、前端 Vitest、类型检查、lint 和浏览器角色组合验收提供证据。
