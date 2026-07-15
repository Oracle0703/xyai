# 交付报告

## 摘要

| 字段 | 内容 |
| --- | --- |
| 结果 | `sub_admin` 账号级菜单权限、后端默认拒绝白名单和三类授权页面已实现 |
| 分支 | `feature/hy/10156_新增子管理员角色` |
| 状态 | 已完成，未 commit、未 push |

## 已交付变更

| 区域 | 变更 |
| --- | --- |
| 数据 | `users.admin_permissions` JSONB、迁移 177、Ent 与 Repository 映射 |
| 后端 | 三项权限目录、DB 最新权限鉴权、方法/路由模板白名单、compact 筛选、typed 400/403 |
| 用户管理 | 创建/编辑子管理员、权限目录、离开角色清权、最后管理员保护、审计和缓存失效 |
| 前端 | Auth store、路由 meta/直链守卫、授权菜单、专用 403 恢复、创建/编辑权限配置 |
| 页面 | 订阅仅保留两种配额重置；使用记录和 Token 分析只读 |
| 安全收口 | shallow 嵌套用户 DTO 不暴露 `admin_permissions`；账号、风控、请求拦截等保持拒绝 |
| 文档 | code review 处理记录、组件 README、backend/frontend/data/security wiki 已同步 |

## 验证证据

| 检查 | 结果 |
| --- | --- |
| 后端 unit | middleware、service、repository、handler、dto、admin handler、routes 全部通过 |
| migration | fresh `GOTMPDIR` 重跑通过 |
| 前端 | 178 个文件、1167 个 Vitest 测试通过；typecheck/lint exit 0 |
| 生成一致性 | Ent/Wire 生成通过，Wire 无实质漂移 |
| 浏览器 | 管理员全菜单；三种单权限只显示授权菜单；空权限直链回 `/dashboard` |

## 已知限制

| 限制 | 影响 |
| --- | --- |
| 浏览器使用内存 mock API | 真实数据库端到端 smoke 留给测试/预发布环境；核心授权链已有分层自动化覆盖 |
| 未执行 commit/push | 保留工作区供用户审核 |

## 后续动作

| 优先级 | 动作 |
| --- | --- |
| 建议 | 在测试环境创建真实子管理员，按 `subscriptions` / `usage` / `token_analysis` 组合做一次 smoke |
| 用户决定 | 审核后提交并推送当前分支 |
