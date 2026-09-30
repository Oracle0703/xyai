# 子管理员角色：合并前小修建议

| 字段 | 内容 |
| --- | --- |
| 日期 | 2026-07-17 |
| 分支 | `feature/hy/10156_新增子管理员角色` |
| 对照 | `docs/delivery/2026-07-15-sub-admin-role/{requirements,spec}.md` |
| 前置审查 | 实现整体可合；本文只列**合并前建议小修**，不含大范围 RBAC 重构 |
| 结论 | **无阻塞硬伤**；下列项按优先级择修，修完再合更稳 |

---

## 1. 合并门槛判断

| 判断 | 说明 |
| --- | --- |
| 能否合 | 可以。安全模型（DB 权威 + 方法/路由白名单默认拒绝 + 写操作极窄）正确，与已批准规格一致。 |
| 建议合前做 | P0 建议尽量做完；P1 视工期；P2 可合后跟。 |
| 明确不做 | 资源级/组织级 RBAC、自定义权限码、拆全站菜单权限体系——属范围外。 |

---

## 2. P0 — 建议合并前处理

### P0-1. 禁止或收口「空权限 sub_admin」

**问题**

- 允许 `role=sub_admin` 且 `admin_permissions=[]`。
- 标准模式下行为接近普通用户，但角色徽章/筛选仍显示「子管理员」，语义含糊。
- backend 模式虽已拒绝登录，列表与运维认知仍易混淆。

**建议（二选一，推荐 A）**

| 方案 | 行为 |
| --- | --- |
| A（推荐） | 创建/更新为 `sub_admin` 时强制至少 1 个合法权限码；否则 400。 |
| B | 保存时空权限自动归一为 `user`，并清空 `admin_permissions`。 |

**改动范围（示意）**

- `backend/internal/service/admin_permission.go`：`NormalizeAdminPermissions` 或 Create/Update 路径校验。
- `backend/internal/service/admin_permission_test.go` / `admin_service_role_test.go`：空权限拒绝或降级用例。
- 前端创建/编辑弹窗：提交前提示「至少勾选一项」；与后端错误码对齐。

**验收**

- 创建空权限 sub_admin → 400（或降为 user，按选定方案）。
- 已有空权限存量若存在：迁移/运维说明，或启动时一次性清洗脚本（可选）。

---

### P0-2. 补「页面实际 API ⊆ 白名单」防漂移测试

**问题**

- 路由白名单手写在 `admin_permission.go`。
- 已有「写接口 ⊆ 允许写集合」测试，但**没有**「订阅/使用/Token 页面会打的接口全集 ⊆ 白名单」。
- 后续加筛选/统计接口时，易出现「菜单可见、按钮可点、请求 403」。

**建议**

在后端单测中维护三份只读清单（或从现有前端 API 模块静态提取注释清单）：

| 权限 | 期望允许的 method+route 至少覆盖 |
| --- | --- |
| `admin.subscriptions` | 列表/详情/进度、search-users、search-groups、reset-quota |
| `admin.usage` | usage 列表/stats/search-*、dashboard 聚合、ops 错误只读 |
| `admin.token_analysis` | summary/users/projects/requests/input/index status/archive-files、users-trend |

断言：清单中每一项 `CanAccessAdminRoute(sub_admin, method, route) == true`；\
并保留既有：非 GET 写接口仅 `reset-quota`。

**改动范围**

- `backend/internal/service/admin_permission_test.go`（或新建 `admin_permission_contract_test.go`）。
- 可选：在 `docs/delivery/2026-07-15-sub-admin-role/spec.md` 附「页面 API 契约」表，与测试同源。

**验收**

- 故意从白名单删一条页面依赖路由 → 测试失败。
- 故意加入非规格写接口 → 写白名单测试失败。

---

### P0-3. 合并前真实环境 smoke（非代码，但建议卡点）

**问题**

- 交付报告中的浏览器矩阵依赖 mock API；真实 PostgreSQL + 真实 JWT 路径未作为合并门禁。

**建议最小 smoke**

| 账号 | 操作 | 期望 |
| --- | --- | --- |
| admin | 创建三种单权限 sub_admin | 成功；权限目录来自 catalog |
| sub_admin(subscriptions) | 列表、筛选、两种 reset-quota；尝试 assign | 前两者成功；assign 403 |
| sub_admin(usage) | 查询/导出；尝试 cleanup | 查询成功；cleanup 403 |
| sub_admin(token) | 查看；尝试立即索引 | 查看成功；索引 403 |
| 撤权后同会话 | 再调管理接口 | 403 + 前端跳转/登出符合模式 |

记录结果到交付目录或本文件附录即可。

---

## 3. P1 — 建议合前或合后立刻跟

### P1-1. 理顺 compact 筛选 handler 归属

**问题**

- `GET /admin/subscriptions/search-groups` 挂在 subscription 路由，实现却是 `UsageHandler.SearchGroups`。
- 可读性差，后续改 usage 易误伤订阅筛选。

**建议**

- 抽公共 `SearchGroups` / compact option 到共享 handler 或 admin service；
- 或订阅侧薄封装调用同一 service，避免跨域 handler 复用。

**优先级**：不阻塞合并；改动小、可读性收益高。

---

### P1-2. 前端守卫测试与真实路由语义对齐

**问题**

- `frontend/src/router/__tests__/guards.spec.ts` 仍大量按旧 `isAdmin` 语义断言。
- 无法回归：`sub_admin` + `adminPermission`、空权限、backend 撤权 logout。

**建议**

- 至少补 4 个用例：
  1. sub_admin 有权限 → 可进对应 admin 页
  2. sub_admin 无该权限 → 落到 landing / dashboard
  3. simple mode 下 sub_admin 不能因角色绕过用户侧 `/subscriptions`、`/redeem`
  4. backend 模式空权限 / 撤权后应 logout 或留在 login 的约定行为

**优先级**：合前更好；至少合后第一周补齐。

---

### P1-3. setup / 功能开关回退路径识别 sub_admin

**问题**

| 位置 | 现状 |
| --- | --- |
| `setupRedirect.ts` | 只认 `isAdmin`，setup 完成后 sub_admin 进 `/dashboard` |
| payment/risk 关闭时的回退 | `isAdmin ? admin... : /dashboard`，sub_admin 一律用户首页 |

**建议**

- setup 完成：`canAccessAdmin` 时用 `getAdminLandingPath`。
- 功能开关关闭：sub_admin 回 landing，而不是误进无权限的 `/admin/settings`。

**优先级**：边缘路径，P1。

---

### P1-4. 授权文案与权限面再确认（产品，可不改代码）

**问题**

- `admin.usage` 实际含 Dashboard 聚合与 Ops 错误详情；i18n 已写明，但运营侧仍可能按「只能看使用记录表」理解。

**建议**

- 合并说明 / 发布说明中写清三权实际边界。
- 若产品不接受 usage 含 ops：合后单独立项拆权限，**不要**在本分支临时砍白名单导致页面半残。

---

## 4. P2 — 可合后迭代

| ID | 项 | 说明 |
| --- | --- | --- |
| P2-1 | 侧边栏/landing 也消费 catalog | 减少「后端加权限、前端菜单漏改」；当前仅 3 项可暂缓 |
| P2-2 | 订阅只读 vs 可重置拆权 | 需要「只能看不能重置」时再拆 `admin.subscriptions.reset_quota` |
| P2-3 | 白名单结构优化 | 权限增多后改为 `map[method]map[route]struct{}`；现 3 权线性扫描可接受 |
| P2-4 | SearchGroups pageSize=1000 | 分组规模上来后改为服务端搜索分页；与本需求无强绑定 |
| P2-5 | 路由注册与白名单同源 | 长期用代码生成或注册时声明 `AllowSubAdmin(permission)`，消灭双份字符串 |

---

## 5. 明确不建议在本分支做的事

| 项 | 原因 |
| --- | --- |
| 引入完整 Casbin/组织 RBAC | 范围外，延误合并 |
| 子管理员开放账号/设置/自定义菜单 | 规格明确拒绝 |
| 为方便调试临时「GET 全放行」 | 破坏默认拒绝模型 |
| 大面积重写 AdminAuth | 现模型正确，只做增量小修 |

---

## 6. 建议执行顺序

```text
1. P0-1 空权限收口（产品拍板 A 或 B）
2. P0-2 白名单契约测试
3. P0-3 测试环境真实账号 smoke
4. （有余力）P1-2 前端守卫回归 + P1-3 setup/回退
5. 合并
6. 合后排期 P1-1、P2-*
```

---

## 7. 建议验证命令（修完后）

```bash
# 后端权限与鉴权
go test -tags=unit -count=1 ./internal/service -run 'AdminPermission|NormalizeAdmin|SubAdmin'
go test -tags=unit -count=1 ./internal/server/middleware -run 'AdminAuth|SubAdmin'
go test -tags=unit -count=1 ./internal/handler/admin -run 'Permission|Compact|SubAdmin'
go test -count=1 ./migrations -run SubAdmin

# 前端
pnpm --dir frontend exec vitest run src/utils/__tests__/adminPermissions.spec.ts src/stores/__tests__/auth.spec.ts src/router/__tests__/feature-access.spec.ts src/router/__tests__/subAdminRoutes.spec.ts
pnpm --dir frontend run typecheck
```

（路径以仓库 `backend/` 为工作目录时，service/middleware 包路径按现有模块调整。）

---

## 8. 一句话清单（给合并负责人）

| 优先级 | 要不要挡合并 | 项 |
| --- | --- | --- |
| P0-1 | 建议挡 | 空权限 sub_admin 收口 |
| P0-2 | 建议挡 | 页面 API ⊆ 白名单契约测试 |
| P0-3 | 建议挡 | 真实环境 5 角色 smoke |
| P1-1 | 不挡 | SearchGroups 归属清理 |
| P1-2 | 尽量做 | 前端 guards 回归 sub_admin |
| P1-3 | 不挡 | setup/功能开关回退 |
| P1-4 | 不挡 | 发布说明写清 usage 权限面 |
| P2-* | 合后 | 目录统一、拆权、生成白名单等 |

**总评**：实现可合；合并前最值得做的三件小事是 **空权限语义、契约测试、真实 smoke**。其余属于可维护性与边缘路径，适合合后快速跟进。
