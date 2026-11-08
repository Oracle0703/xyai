# Sub2API 0.2.20 上游同步记录（固定提交）

合并日期 2026-11-08 ｜ 工作分支 `feature/hy/10231_merge_sub2api_220`（第一父 = 本地 `main@bb52b73b10143a2478d66009349c8a52535a027f`）｜ 上游 `Wei-Shaw/sub2api main@6b4b37db43443b5e88e424fe3c69602cc3810dfb`（VERSION 0.2.20）｜ 合并提交 `4acd8d9e49ebc16f92896fd2b30bf45bdac7098e` ｜ 状态：经 PR #49（merge commit）合入 main

本文是该轮同步的唯一记录，复核结论直接写入正文。正文中"尚未创建/等待审核/未 commit"等字样是提交前审核快照，实际合并提交见上一行。台账条目见 `docs/upstream-sync/merge-log.md` 的 `2026-11-08 — 0.2.20 固定提交合并（待审核、未 commit）` 节。

## 1 范围与状态

| 项目 | 结果 |
| --- | --- |
| 位置 | 原仓库工作目录，不使用 worktree |
| 工作分支 / 第一父 | `feature/hy/10231_merge_sub2api_220` / `bb52b73b10143a2478d66009349c8a52535a027f`（本地 main = github/main，fetch 后确认为最新；origin/main 是 2026-05 的旧内网分支，不作基线） |
| 固定上游 | `6b4b37db43443b5e88e424fe3c69602cc3810dfb`（`chore: sync VERSION to 0.2.20`），fetch 时即 upstream/main；tag `v0.2.20` 指向的 `dab829a4e` 仍是 VERSION 0.2.19，以 VERSION 同步提交为边界 |
| merge base | `a9240d5016c226b341242a8c54a01a8605cbbc72`（0.2.19） |
| 上游增量 | 112 commits（72 非合并 + 40 合并）/ 459 paths（146 A / 312 M / 1 D） |
| 双方修改路径 | 83 个；文本冲突 12 个 |
| 合并方式 | `git merge --no-commit --no-ff 6b4b37db4` |
| 边界 | 只解决文本冲突和合并后才出现的语义冲突；不修上游 bug，不 commit / push / 合回 main / 部署 |

## 2 冲突与处理

### 2.1 文本冲突

| 文件 | 双方内容与处理 |
| --- | --- |
| `.gitignore` | 本地 docs 目录白名单 + 上游 `agent-resource-safety.md`、`moego-json-batching.md` 白名单，全部保留 |
| `backend/go.mod` | 本地把 x/sys、x/text 列为直接依赖，上游在 indirect 段重复列出并新增 x/oauth2；保留本地直接依赖，只加 `golang.org/x/oauth2 v0.36.0 // indirect`；上游 go-sdk/brotli/compress 升级自动合入 |
| `backend/cmd/server/wire.go` | cleanup 并列保留本地 GPTQuotaDisplay/OrganizationUsageRollup 与上游 DigestSessionStore/BalanceNotify 四步 |
| `backend/cmd/server/wire_gen.go` | 不选边，源图合好后运行 `go run -mod=mod github.com/google/wire/cmd/wire` 重新生成；`ProvidePluginManager` 仍调用 `SetAccountDirectory`；生成写入的 subcommands checksum 已从 go.sum 还原 |
| `backend/internal/handler/handler.go`、`handler/wire.go` | `Handlers` 字段并集：上游 `MCPOAuth` + 本地 `SubscriptionSelfReset`、`GPTQuotaDisplay` |
| `backend/internal/server/router.go` | 本地 `registerPromptMetricsAdminRoutes` 与上游 `RegisterMCPRoutes`/`RegisterMCPOAuthRoutes` 并存 |
| `backend/internal/server/routes/gateway.go` | 上游 `BodyMemoryBudget` 版 `RequestBodyLimit` + 本地 RequestArchive/RequestIntercept 构造并存；本轮上游无新网关入口，中间件顺序不变 |
| `backend/internal/handler/admin/scoped_admin_subscription_response.go` | 保留本地 switch（受限密钥 / JWT 子管理员 / 完整 DTO 三分支），受限分支判断按上游改为 `adminaccess.RestrictedFromContext`，全权密钥走完整 DTO |
| `backend/internal/service/openai_gateway_responses_chat_fallback.go` | 保留本地把 billing/upstream model 提前计算的顺序，调用改为上游更名后的 `validateGPT61CompatRequest` |
| `frontend/src/api/admin/settings.ts` | 本地归档设置导出与上游注册黑名单导出并存 |
| `frontend/src/api/client.ts` | 本地 `retryAfter`（原始头，`useSubscriptionSelfReset` 使用）与上游 `retry_after_seconds`（整数秒，用量导出使用）并存。两者部分重叠，本轮不改本地 composable，后续统一时改用上游字段 |

### 2.2 合并后语义适配

| 位置 | 问题与处理 |
| --- | --- |
| `backend/internal/managementmcp/auth.go#DispatchAuth` | 上游新管理 MCP 在私有路由里复用 `/admin/subscriptions*`，鉴权只设置 user/role。本地 `SubscriptionHandler` 生产接线带 `DepartmentService`，缺 `DepartmentActor` 时返回 `ErrDepartmentScopeDenied`，所以合并后 5 个 MCP 订阅工具（list/get/assign/extend/revoke）会全部被拒，上游单独运行时正常。处理：admin 分发时追加 `service.WithDepartmentActor(principal.UserID)`，与 0.2.19 受限密钥适配同类；admin scope 只给 RoleAdmin，仓储仍按库内角色复核，工具集不变 |
| 回归 | 新增本地测试 `backend/internal/managementmcp/department_actor_test.go`：admin 分发带 actor、user 分发不带；改前失败（actual 0），改后通过 |
| `backend/internal/repository/mcp_oauth_integration_test.go`、`user_repo_email_blacklist_integration_test.go`（仅测试） | 上游两个 PG 集成用例用 `users.Update(..., Role: true)` 把用户提升/降级为管理员，没带本地仓储要求的 admin access 版本，在隔离 PG 上报 `ADMIN_ACCESS_VERSION_REQUIRED`。处理：改用 `GetByIDWithAdminAccess` 读取并传 `ExpectedAdminAccessVersion`（与 0.2.19 测试适配同类），断言不变；改后 6 个 MCP/黑名单 PG 用例全部通过。上游生产代码没有不带版本的角色变更 |

这些都是本地约束（fail-closed 部门身份、角色 CAS）和上游新代码叠加后才出现的冲突，不是修上游 bug。

### 2.3 双方修改路径与 Git 硬门

| 检查 | 结果 |
| --- | --- |
| 376 个仅上游路径 | 合并时逐 blob 等于目标；之后只有 2.2 的三个文件不同（`managementmcp/auth.go` 生产适配 + 两个上游 PG 集成测试的 CAS 适配） |
| 725 个仅本地路径 | 逐 blob 等于第一父；之后只改本轮文档（llm-wiki 六页与 wiki 图谱、`docs/README.md`、`playbook.md`、台账）。新增文件只有本报告和 `department_actor_test.go` |
| 83 个双方修改路径 | 79 个自动合并文件中，merge-base→第一父、merge-base→目标两组增删行序列在合并结果中都完整保留；剩余 4 个（wire_gen、handler.go、handler/wire.go、订阅投影）是冲突文件，按 2.1 处理 |
| `git ls-files -u` / 冲突标记 / `git diff --cached --check` | 0 / 无 / 通过 |
| `docs/features/` | 26 个文件零修改零删除 |
| migration | 新增 `249_mcp_oauth.sql`，与本地 `249_token_analysis_client_identity.sql` 按完整文件名共存；旧迁移零修改 |

## 3 保留的本地能力

| 范围 | 核对结论 |
| --- | --- |
| 网关链路 | RequestArchive → RequestIntercept、Responses guard、Prompt Metrics/Risk、Token Analysis、默认 effort、compatible cache usage、大请求保护、空响应兜底保留 |
| 管理/数据 | 组织/部门/组织负责人/分组组织归属（D13）、日汇总、订阅自助重置与批量重置、GPT 额度、并发预设、子管理员与账号运维通知、插件账号目录接线保留 |
| 权限 | 上游新增 `/admin/users/batch-disable`、`/admin/users/blacklist-matches`、`/admin/settings/registration-email-blacklist` 未加入 `admin_permission.go`，子管理员默认拒绝；MCP admin scope 仅 RoleAdmin，子管理员无法经 MCP 进入 `/admin` |
| disabled_source | 上游批量禁用不写 `disabled_source`，由本地 migration 261/262 触发器补成 `other`，部门/组织负责人不能解除 |
| 前端 | 生图（含 2026-11-08 自定义尺寸）、Token Analysis、组织报表、子管理员菜单过滤、用量页默认今天保留；上游 MCP 页/授权页是普通用户路由 |
| vue-i18n v11 | 本地代码没有用已移除的 `tc/$tc/v-t`；`localesMessageCompile.spec.ts` 用 11.4.16 编译器校验全部本地文案 |

## 4 采用 / 未采用的上游行为

| 事项 | 结论 |
| --- | --- |
| 管理 MCP + OAuth | 采用；`mcp.public_url` 为空时关闭。新表见 migration 249 |
| 管理密钥全权兼容模式 | 采用；全权密钥可访问全部 `/api/v1/admin/*`（不含密钥管理），包括本地部门/Token Analysis 等路由，和旧版全权密钥一致 |
| 注册邮箱精确黑名单 + 批量禁用 | 采用；只拦截注册（含邮箱 OAuth 自动注册） |
| 网关资源边界 | 采用实例请求体内存预算（1 GiB，超出 503）、单次读/写 idle 超时（120s）、SSE 行/事件/队列上限。本地归档/拦截用 `io.ReadAll` 读请求体，受读超时约束但不计入内存预算，记为边界，不修补 |
| OpenAI / 生图 | 生图账号 1K/2K/4K 档位分流、GPT Image 2.6 原生链路、可关闭传输错误冷却、cyber transcript 长会话误拦修复、空 instructions 透传、Codex WS 指纹收敛 |
| 其他平台 | Antigravity 配额窗口/Pro Argon/INTERNAL 500 惩罚范围、Gemini SSE 行上限与 function response 媒体、Grok Voice 读失败分类、Bedrock 帧长校验、Kimi 并发 403 短冷却 |
| 依赖 | MCP go-sdk v1.4.1、brotli v1.2.7、klauspost/compress v1.20.2；前端 vue-i18n v11、decode-uri-component 与 follow-redirects override |
| 重叠实现 | 只有 Retry-After 一处部分重叠（2.1 client.ts），两者并存；未发现需要整项删除的本地功能 |

## 5 验证

| 验证 | 实际结果 |
| --- | --- |
| pnpm 9.15.4 frozen install | 通过；本机默认 registry 缺 `@intlify/*@11.4.16`（404），本次命令临时加 `--registry=https://registry.npmmirror.com/`，锁文件和 package.json 零漂移 |
| 前端 lint:check / typecheck | 均通过 |
| 完整 Vitest | 438 files / 3599 tests 全部通过（含 vue-i18n v11 文案编译校验） |
| 前端 build | 通过（保留原有大 chunk 提示） |
| Go default 全量 `-p 1 -count=1 ./...` | 退出 0，56 个包 ok，0 FAIL |
| Go unit 全量 `-tags=unit`（CI 门禁） | 退出 0，62 个包通过；13288 顶层 / 26997 含子用例 PASS，0 FAIL，41 显式 skip（repository 16 个是 `SCOPED_ADMIN_TEST_DSN` 门控，见下行；其余 service 10、securityaudit 9、tlsfingerprint 3 等沿既有 skip 条件，本轮未逐项复核原因） |
| scoped admin PG 用例（`SCOPED_ADMIN_TEST_DSN`） | 上面 16 个 DSN 门控用例及相关 `-run` 匹配：26 顶层 / 50 含子用例 PASS，0 skip；handler/admin `TestLegacyAdminKeyRotationPostgres` 通过 |
| Go integration 全量 `-tags=integration` | 退出 0，56 个包通过；7314 顶层 / 14952 含子用例 PASS，18 显式 skip。repository 包因无 Docker 整包跳过，不算数据库集成通过 |
| 隔离 PG 升级模拟 | PostgreSQL 18.3 新库 `merge220_upgrade`：先用第一父快照跑迁移（329 个，含本地 249），再用合并结果跑，只新增 `249_mcp_oauth.sql`（330 个），三张 mcp_oauth 表存在；随后 MCP/黑名单 6 个 PG 用例通过（2.2 测试适配后） |
| repository PG 全套（postgres-only harness） | 新空库对照：合并结果 531 顶层 PASS / 11 FAIL / 5 skip，第一父快照 524 / 11 / 5，失败集合完全相同（10 个 Redis 依赖套件在无 Redis 时 nil 指针 + 既有 `TestIdempotencyCoordinator_AtomicSuccessRollsBack...`），多出的 7 个通过是上游新用例 |
| MCP / routes 专项 | `managementmcp`、`server/routes` default 通过；新增 `TestMCPAdminDispatchSetsDepartmentActor` 改前红、改后绿 |
| Ent / Wire 生成 | 各跑两次，ent 与 cmd/server 无漂移，wire_gen 哈希两次一致；工具写入 go.sum 的 12 行 checksum 已还原 |
| Go normal / embed build | 均通过，embed 使用本轮前端 build 产物 |
| golangci-lint 2.14 `--new-from-rev=HEAD ./...` | 0 issues；已用 `-v` 确认真实加载分析（不带 new-from-rev 时能报出 TD-004 既有问题） |
| go mod tidy -diff | 退出 0，无差异 |
| Git 硬门 | 见 2.3，全部通过 |
| Wiki 图谱 | `toolsefresh-understand-wiki.cmd`：35 nodes / 76 edges，62 wikilinks / 0 unresolved |

Go 1.27.3；三套 Go 全量依次运行，每套独立 GOTMPDIR。第一父对照用 `git archive HEAD backend` 导出到忽略目录 `backend/.gocache/merge220-main-baseline`，不建分支、不改工作区。嵌入式 PG 只监听 127.0.0.1，使用新建库 `merge220_upgrade` / `merge220_fresh` / `merge220_basefresh` / `merge220_scoped`，未读取生产配置，结束后已停止。原始日志在 `E:	mp\merge220-*`（不入库）。

## 6 遗留问题

| 问题 | 归属与处理 |
| --- | --- |
| repository PG 全套 11 个失败 | 环境限制 + 第一父既有：合并结果与第一父在新空库上失败集合相同；10 个是 postgres-only harness 无 Redis，1 个是既有 `TestIdempotencyCoordinator_AtomicSuccessRollsBack...`。不在本轮处理 |
| 完整集成覆盖 | 本机无 Docker，repository 在 integration 全量里整包跳过；PG 专项不替代 Docker/Redis 全套。外部模型、真实 MCP 客户端接入、浏览器端到端、真实 SMTP/支付、生产均未验收 |
| 归档/拦截与请求体内存预算 | 本地 RequestArchive/RequestIntercept 用 `io.ReadAll` 读取请求体，不计入上游新增的 1 GiB 实例内存预算（仍受读超时约束）。记为边界，不在合并中修改 |
| Retry-After 双字段 | 前端 `retryAfter` 与 `retry_after_seconds` 并存；以后统一时把 `useSubscriptionSelfReset` 改到上游字段 |
| 部署前检查 | 新库 249 迁移只建表；若要启用 MCP 需配置 `mcp.public_url`（HTTPS）。内网构建机需要能拉到 vue-i18n 11.4.16 相关包 |
| 未发现的上游问题 | 本轮测试未暴露上游自身缺陷，没有登记上游 bug |

## 7 提交前状态

- `HEAD=main=bb52b73b10143a2478d66009349c8a52535a027f`；`MERGE_HEAD=6b4b37db43443b5e88e424fe3c69602cc3810dfb`。未 commit、未 push、未合回 main、未部署；全部改动已暂存，等待用户审核。
