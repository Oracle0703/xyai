# 测试与审查

## 测试矩阵

| 验收标准 | 结果 | 证据 |
| --- | --- | --- |
| AC-1 管理员全量访问 | 已通过 | middleware 管理员/API Key 绕过测试；浏览器侧边栏包含全部管理入口 |
| AC-2 子管理员按账号授权 | 已通过 | DB 最新权限 middleware 测试、router meta/guard 测试、三种单权限浏览器矩阵 |
| AC-3 订阅仅允许查看与两种配额重置 | 已通过 | 写白名单集合测试、`SubscriptionsView.spec.ts` 两种 payload 与按钮测试、既有 `subscription_reset_quota_test.go` |
| AC-4 使用记录只读 | 已通过 | compact 筛选 handler/页面测试；浏览器无清理入口，保留查询、排行与导出 |
| AC-5 Token 分析只读 | 已通过 | 白名单及页面测试；浏览器无“立即索引”入口 |
| AC-6 普通用户不变 | 已通过 | middleware 拒绝、simple mode guard 和完整前端回归 |
| AC-7 撤权即时生效、未知码 400 | 已通过 | middleware DB 权限撤销测试、service typed 400、前端 403 恢复测试 |
| AC-8 迁移、审计、缓存失效 | 已通过 | migration、repository 回读、service 审计/缓存失效相关测试 |

## 规格符合性审查

| 项目 | 状态 | 结论 |
| --- | --- | --- |
| `docs/reviews/2026-07-15-sub-admin-role-code-review.md` P0/P1 | 已处理 | compact 分组、simple mode、catalog 消费者、文案、handler、backend logout 和防漂移测试均已收口 |
| 默认拒绝与 DB 权威 | 已通过 | `AdminAuth` 使用最新用户和 Gin `FullPath()`，未知路由拒绝 |
| DTO 数据最小化 | 已修复 | `admin_permissions` 只在完整用户 DTO 映射，shallow 嵌套用户不再暴露 |
| 范围控制 | 已通过 | 未开放账号、风控、请求拦截、系统设置、自定义菜单或额外写操作 |

## 代码质量审查

| 严重程度 | 项目 | 状态 |
| --- | --- | --- |
| 无阻塞发现 | 角色转换、最后管理员保护、权限清权和缓存失效路径 | 已复核 |
| 无阻塞发现 | 管理员/Admin API Key 绕过与子管理员默认拒绝白名单 | 已复核 |
| 无阻塞发现 | 前端直链守卫、权限撤销恢复和 backend 模式登出 | 已复核 |

## 验证日志

| 命令 / 检查 | 结果 | 备注 |
| --- | --- | --- |
| DTO shallow 权限测试 RED | 按预期失败 | 修复前得到 `[]string{"admin.usage"}` |
| DTO 完整/shallow 权限测试 GREEN | 通过 | 完整 DTO 保留权限，shallow 为 `nil` |
| `go generate ./ent` | 通过 | Ent 生成文件与 schema 对齐 |
| `go generate ./cmd/server` | 通过 | Wire 生成成功，无实质 `wire_gen.go` diff |
| `go test -tags=unit -p 1 -count=1`（middleware/service/repository/handler/dto/admin/routes） | 通过 | 7 个包全部通过；service 113.892s |
| `go test -p 1 -count=1 ./migrations` | 通过 | 首次遇 Windows `.test.exe` 占用；确认无残留进程并换 fresh `GOTMPDIR` 后 0.626s 通过 |
| `pnpm --dir frontend exec vitest run` | 通过 | 178 个测试文件、1167 个测试 |
| `pnpm --dir frontend run typecheck` | 通过 | `vue-tsc --noEmit`，exit 0 |
| `pnpm --dir frontend run lint:check` | 通过 | ESLint，exit 0 |
| 内置浏览器角色矩阵 | 通过 | admin、subscriptions、usage、token、empty 五种状态；URL/标题/DOM/交互均核对 |
| `git diff --check` | 通过 | 无空白错误 |

## 残余风险

| 风险 | 影响 | 处理 |
| --- | --- | --- |
| 浏览器矩阵使用本机内存 mock API | 未覆盖真实 PostgreSQL 登录与真实业务数据渲染 | DB、middleware、service、handler 和 migration 由自动化测试覆盖；上线前仍建议在测试环境做真实账号 smoke |
| 订阅浏览器夹具为空列表 | 浏览器未实际点击两种重置按钮 | 组件测试验证两种请求 payload 和成功反馈，service 既有测试验证重置逻辑，白名单测试验证子管理员可调用 |
| 部分页面截图接口超时 | 订阅/用量/Token 状态只有 DOM/URL 证据 | 管理员首屏截图成功；其他页面无框架错误覆盖层，DOM 完整且关键入口/按钮已核对 |
