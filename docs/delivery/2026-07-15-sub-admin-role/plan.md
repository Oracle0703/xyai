# 子管理员角色与菜单权限实施计划

> 按 TDD 执行：先补失败测试并记录 RED，再完成最小实现，最后运行回归验证。

## 任务清单

| 编号 | 状态 | 任务 | 主要改动 | 验证 |
| --- | --- | --- | --- | --- |
| 1 | 已完成 | 创建分支 | 从本地 `main` 创建目标分支 | 工作区干净、`main` 为祖先 |
| 2 | 已完成 | 数据模型 | 角色常量、JSONB 字段、迁移、Ent 生成代码、DTO 映射 | 迁移与映射测试 |
| 3 | 已完成 | 权限目录 | 权限码、菜单目录、方法/路由白名单 | 目录和路由匹配单测 |
| 4 | 已完成 | 管理鉴权 | `AdminAuth` 接受子管理员并执行白名单 | middleware、WebSocket、Admin API Key 测试 |
| 5 | 已完成 | 用户管理 | 创建/更新角色和权限、清理 stale 权限、审计与最后管理员保护 | service、handler、API 契约测试 |
| 6 | 已完成 | 只读接口收口 | 使用记录和订阅精简筛选接口、三类页面依赖白名单 | handler、route、响应字段测试 |
| 7 | 已完成 | 前端权限状态 | 类型、Auth store、路由元信息、专用 403 刷新流程 | auth、router、client 测试 |
| 8 | 已完成 | 管理配置界面 | 子管理员角色、服务端权限目录、列表筛选和 i18n | 弹窗、列表和 i18n 测试 |
| 9 | 已完成 | 菜单和页面限制 | 菜单过滤、订阅重置、使用记录/Token 分析只读 | Sidebar 和页面测试 |
| 10 | 已完成 | 文档与验收 | 组件 README、`llm-wiki`、完整验证和浏览器验收 | Go/Vitest/typecheck/lint/浏览器 |

## 文件所有权

| 范围 | 主要路径 |
| --- | --- |
| 后端模型 | `backend/ent/schema/user.go`、`backend/migrations/177_add_sub_admin_permissions.sql`、Ent 生成文件 |
| 后端权限 | `backend/internal/domain`、`backend/internal/server/middleware`、`backend/internal/server/routes` |
| 用户管理 | `backend/internal/service/admin_user.go`、`backend/internal/repository/user_repo.go`、`backend/internal/handler` |
| 前端状态 | `frontend/src/types`、`frontend/src/stores/auth.ts`、`frontend/src/router`、API client |
| 前端界面 | `AppSidebar.vue`、用户管理组件、`SubscriptionsView.vue`、`UsageView.vue`、`TokenAnalysisView.vue` |
| 文档 | `docs/delivery/2026-07-15-sub-admin-role`、`llm-wiki/wiki`、相关 README |

## 实施约束

- 新迁移使用 `177_add_sub_admin_permissions.sql`，不修改历史 migration。
- 修改 Ent schema 后运行 `go generate ./ent`；依赖签名变化时运行 `go generate ./cmd/server`。
- 权限检查以后端数据库中的最新用户数据为准，不信任 JWT 内旧角色或前端状态。
- 子管理员接口采用白名单，不允许“只要是 GET 就放行”。
- 页面按钮隐藏只是体验控制，后端独立拒绝对应写接口。
- 新增管理功能时必须同时登记权限目录、路由元信息、后端白名单和测试。

## 验证命令

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run
cmd.exe /c pnpm --dir frontend run typecheck
cmd.exe /c pnpm --dir frontend run lint:check

# 后端使用 llm-wiki/wiki/ops.md 的仓库内 GOCACHE/GOMODCACHE 和 fresh GOTMPDIR
go test -tags=unit -p 1 -count=1 ./internal/server/middleware ./internal/service ./internal/handler ./internal/server/routes
go test -p 1 -count=1 ./migrations

git diff --check
git status --short
```

## 审查关卡

| 关卡 | 状态 | 证明 |
| --- | --- | --- |
| 规格符合性 | 已完成 | AC-1 至 AC-8 已对照实现与 `docs/reviews/2026-07-15-sub-admin-role-code-review.md` |
| 代码质量与安全 | 已完成 | compact DTO、simple/backend 模式、服务端 catalog、白名单写接口集合已复审 |
| 完整验证 | 已完成 | 7 个 Go 包与 migration、178 个 Vitest 文件/1167 测试、typecheck、lint 和浏览器角色矩阵均已记录 |

## 回滚

回滚功能提交并移除新增迁移注册即可恢复旧行为；数据库列为向后兼容的非空默认空数组，保留列不会影响旧版本读取。生产环境若已执行迁移，不执行破坏性降级 SQL。
