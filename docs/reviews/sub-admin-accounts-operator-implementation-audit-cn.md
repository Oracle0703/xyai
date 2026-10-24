# 子管理员账号受限运维实现审核

审核日期：2026-10-25（Asia/Shanghai）。范围：`feature/hy/10220_sub_admin_accounts` 相对 `HEAD=7b270c108` 的未提交改动及新增文件。设计依据：`docs/features/sub-admin-accounts-operator-design-cn.md`。本轮只审核，未修复业务代码、提交或部署。

## 当前复核结论（2026-10-25）

两个 P1 的修复已通过本机复核，本轮未发现新的阻断项。F1 按新合同“子管理员导入命中已有账号一律跳过”验收；F2 按“读取不直接清错误/清限流，保留设计列出的快照和自动策略”验收。P2 根据用户明确决定暂不处理，不阻断本次验收。详细证据见文末“独立复核记录”。下文保留首轮发现及当时证据。

## 首轮结论

设计方向合理，但当前逻辑尚未闭合，建议修复下面 3 项后再验收。两个权限问题来自将原完整管理员接口开放给子管理员时，没有按接口的真实副作用收窄；另一个是重新授权合并语义带来的兼容性回归。

| 级别 | 问题 | 影响 | 修复方向 |
| --- | --- | --- | --- |
| P1 | Codex 新增导入可编辑已有账号的管理配置 | 子管理员可改变已有账号分组、代理、并发、优先级、倍率等，超出“只新增/测试/重新授权”合同 | 服务端按操作者角色限制命中已有账号后的可写字段；保留允许的凭据更新，拒绝或忽略其他管理字段 |
| P1 | 用量读取可以清除账号错误，却未纳入通知 | 查看页面或查询用量可能恢复错误账号，且 GET 请求既不发本次邮件，也不进入现有敏感读取审计 | 子管理员查询使用不改变管理状态的读取路径；若明确允许状态恢复，则按真实副作用追踪单条/批量变化，补齐通知与审计 |
| P2 | Gemini 切换 OAuth 模式残留旧 project_id | Code Assist 切换 AI Studio 后仍可能按旧项目走 Code Assist 转发；完整管理员同样受影响 | 区分保留的配置字段和必须替换/清理的身份字段，在服务端清理与新 OAuth 类型不兼容的字段 |

## F1 / P1：新增导入放开了对已有账号的编辑能力

变更入口：`backend/internal/service/admin_permission.go:116`。

实际链路：

1. `CreateAccountModal.vue:6835` 的 Codex 导入提交 `update_existing: true`，同时提交分组、代理、并发、优先级、倍率等表单值。
2. `account_codex_import.go:256` 按上游身份命中已有账号后进入更新分支。
3. `account_codex_import.go:279` 构造 `UpdateAccountInput`，不仅包含凭据，还包含 `Concurrency/Priority/RateMultiplier/LoadFactor/ExpiresAt/AutoPauseOnExpired`，并追加 `ProxyID/GroupIDs`。
4. 该路径没有子管理员字段边界检查。通用 `PUT /accounts/:id` 被拒绝不影响这里的更新。

触发不需要绕过前端：重复导入一个已有账号，创建表单的配置值就会进入已有账号更新。设计只解释了“导入可能更新已有凭据”，没有解释这些管理字段也能被修改。`RecordCreated` 对 updated 项只展示更新后的快照，没有这些字段的前后对比。

本机复现：实际调用导入 handler 内部业务链，命中同一个账号后捕获到 `concurrency=99, priority=1, rate=0.01, proxy=77, groups=[88]` 的更新输入。测试先确认该路由对 `admin.accounts_operator` 放行；仓储用内存 stub，没有操作真实账号。

验收要求：子管理员重复导入后，已有账号上述配置保持不变；合法凭据更新仍成功并通知；完整管理员原导入行为保持可用。边界必须在服务端实现。

## F2 / P1：把 GET 或 usage/batch 等同于无副作用，造成漏通知

变更入口：`backend/internal/service/admin_permission.go:101`、`:111`；通知分类：`backend/internal/service/account_operator_activity.go:45`；GET 跳过：`backend/internal/handler/admin/account_operator_handler.go:117`。

实际链路：

1. `GET /accounts/:id/usage` 默认 `source=active`，进入 `AccountUsageService.GetUsage`；`POST /accounts/usage/batch` 也对非 Anthropic 账号调用同一个服务路径。
2. Gemini 分支在 `account_usage_service.go:367` 查询本地用量成功后调用 `tryClearRecoverableAccountError`。
3. 该函数在 `:1608` 对包含 `token refresh failed`、`invalid_client`、`missing_project_id`、`unauthenticated` 的错误执行 `accountRepo.ClearError`，把账号恢复 active。此过程不要求重新授权或测试成功。Antigravity 等分支也有类似状态恢复路径。
4. 新通知中间件直接跳过所有 GET；批量用量又被明确归类为 `none`。现有 `audit_log.go` 的敏感 GET 列表不含 usage，所以单条读取连管理操作审计也没有。

本机复现：使用真实 `GetUsage` / `GetUsageBatch`、真实 Gemini quota service 和内存账号/用量仓储，初始错误为 `Token refresh failed: invalid_client`。单条和批量两种查询都调用了 `ClearError` 一次，预期“只读不得清错误”的断言均失败。测试没有访问上游。

这不是要求改写所有原管理员查询行为，而是本次授权必须识别既有服务的状态变更。其他被放行的 quota、模型、凭据相关读取也应按真实副作用核对，不能仅凭 HTTP 方法证明安全。OpenAI 主动用量路径还存在 token provider、快照写入和 auto-reset notifier 调用，应一并审清。

验收要求：单条和批量读取都覆盖错误状态、缓存命中/未命中、主动/被动参数。若产品维持“不能清错误”，则读取不改变管理状态；若允许自动恢复，则该例外须明确写入设计并完整留痕。

## F3 / P2：Gemini 重新授权保留了会改变转发协议的旧身份字段

变更入口：`frontend/src/components/admin/account/ReAuthAccountModal.vue:454`。

实际链路：

1. 账号原为 Code Assist，凭据中有 `project_id`；在重新授权弹窗选择 AI Studio。
2. AI Studio 换码没有项目时，服务端 `GeminiTokenInfo.ProjectID` 使用 `omitempty`，前端 `buildCredentials` 对应值为 undefined，JSON 请求不包含 `project_id`。
3. 新调用的 `ApplyOAuthCredentials` 在 `account_handler.go:1636` 使用 `MergeCredentials`，把旧 `project_id` 补回；以前这条 Gemini 分支走通用更新，不会保留这个非敏感字段。
4. `gemini_messages_compat_service.go:620` 仍以 `project_id` 是否非空选择 Code Assist 转发，在 `:630` 使用旧项目构造请求。因此残留不是单纯的显示问题。

本机复现：向实际 Gin handler 提交 `oauth_type=ai_studio`、新合成 access token、无 project_id，响应 200，捕获的更新凭据仍包含 `project_id=old-code-assist-project`。这与设计第 4 节已记录的残留风险一致，但风险影响实际调用，不能作为无害差异接受。

验收要求：Code Assist → AI Studio 时清理旧 project_id，保留 model_mapping 等运行配置；反向切换使用新项目。完整管理员、子管理员均覆盖。现有前端测试只扫描源码是否调用 applyOAuthCredentials，无法验证该数据合同。

## 已确认合理的部分与设计边界

| 项 | 评估 |
| --- | --- |
| 复用页面和 sub_admin 权限体系 | 合理，避免复制多平台账号管理页面；菜单、路由、landing、类型、文案和后端 catalog 已接线 |
| HTTP 方法 + 路由模板精确白名单 | 合理，未知接口默认拒绝；但还需补上上述参数/字段/副作用边界 |
| 精简 form-options | 合理，新响应不返回代理用户名和密码，完整管理员保留原目录调用 |
| 前端权限收口 | 已覆盖常见行操作、批量入口、管理弹窗、设置加载和多种单元格操作；相关回归测试通过，尚未完成真实浏览器端到端验收 |
| 统一重新授权入口 | 方向合理，避免额外 update/clear-error 请求并复用缓存失效；需要修复 Gemini 模式转换合同 |
| 邮件服务与变更内容 | 常见新增、重新授权、测试变更路径已有跟踪，凭据比较使用摘要、正文 HTML 转义；没有向真实收件人发送邮件 |
| 文档列出的允许行为 | 任意 OAuth 账号重新授权、自定义 base_url、新账号立即调度和 extra 不收窄，按设计声明的决策评估，不重复列为代码缺陷 |
| 通知可靠性 | 当前明确是异步 best-effort，失败只记日志，无持久队列/重试/送达保证；与设计写法一致，但不能承诺“每次变更一定收到邮件” |

## 验证与证据

| 验证 | 结果 |
| --- | --- |
| Go 专项，default build tags | handler/admin、routes 通过；service、middleware 在该筛选下无匹配测试，不计为有效覆盖 |
| Go 专项，`-tags=unit` | service、handler/admin、routes、middleware 四包通过 |
| 本次涉及的前端 Vitest | 25 个文件、279 个用例通过；有既有 mock/错误分支 stderr，不影响用例结果 |
| 前端 typecheck | 通过 |
| 变更 Vue/TS 文件 ESLint | 通过 |
| `git diff --check` | 通过 |
| 独立合同反例 | 3 个测试函数、4 个场景均按预期失败，分别证实 F1、F2 单条/批量、F3 |

Go 专项命令（backend 目录，按 wiki 使用 review-cache/review-gopath 和 fresh GOTMPDIR）：

```powershell
go test -tags=unit -p 1 -count=1 ./internal/service ./internal/handler/admin ./internal/server/routes ./internal/server/middleware -run 'AccountOperator|AccountActivity|SubAdminPermission|AdminPermission|CanAccessAdmin|AdminAuth|ApplyOAuthCredentials'
```

反例采用 Go overlay，没有往业务源码目录添加测试或修改实现。本机证据位于 `backend/.gocache/account-operator-review/`，属忽略的临时产物，不随 Git 交付：`service_contract_test.go`、`admin_contract_test.go`、`overlay.json`、`repro.log`。运行命令：

```powershell
go test -overlay .gocache/account-operator-review/overlay.json -tags=unit -p 1 -count=1 ./internal/service ./internal/handler/admin -run '^TestReview(AccountOperator|Gemini)' -v
```

未做生产、真实 SMTP、真实 OAuth 提供商、真实浏览器或数据库端到端验证。前三项业务闭环缺陷应先修复，再补完整管理员/子管理员双角色的页面验收。

## 修复记录（2026-10-25）

| 编号 | 结论 | 处理 |
| --- | --- | --- |
| F1 | 属实，实现遗漏 | 子管理员 Codex 导入命中已有账号时服务端跳过（`skipped` + 账号 ID），不改写已有账号、不建重复账号；完整管理员行为不变。回归：`internal/handler/admin/account_operator_import_test.go` |
| F2 | 属实，实现遗漏（按 HTTP 方法而非真实副作用归类） | 产品决定"查看不清错误、不清限流"（不等于查询严格只读）：`AdminAuth` 为子管理员打 `WithSubAdminAccountRestrictions`，用量单条/批量不再清可恢复错误，Grok 额度探测不再清限流（singleflight 共享 context 显式带入标记并单独合并）。恢复只走会发邮件的测试连接/重新授权；用量快照缓存写入与 OpenAI 自动重置信号不视为管理状态变更。回归：`internal/service/account_operator_restrictions_test.go`、`account_operator_activity_test.go` 的 Grok 用例、`admin_auth_test.go` |
| F3 | 接口合同问题属实，但页面不可触发：重新授权弹窗的 Gemini 类型只读、取自原 `oauth_type`；Code Assist 换码无项目会直接报错 | 当前无 Gemini 订阅，按产品决定暂不加固，记录在设计文档风险表 |

## 独立复核记录（2026-10-25）

复核范围为 Claude 对 F1/F2 的修复及相邻调用链，基线仍为 `HEAD=7b270c108` 的未提交工作区。本轮未修改业务代码、提交或部署。

| 项目 | 复核结果 | 证据 |
| --- | --- | --- |
| F1：子管理员重复导入 | 通过。已有账号不进入更新/创建分支，返回 skipped 与已有账号 ID；混合批次的新账号仍正常新增 | 实际 JWT → AdminAuth → ImportCodexSession → 幂等 helper → 导入业务链，覆盖 update_existing 缺省/true/false |
| F1：完整管理员兼容 | 通过。缺省/true 仍更新已有账号；false 保留原创建行为 | 同一 JWT 路由测试的 admin 对照组，实际检查更新调用和并发字段 |
| F2：单条/批量用量 | 通过。子管理员不再清除可恢复错误，force=true 也不能绕过；完整管理员保持原有恢复 | 使用真实 AccountUsageService、GeminiQuotaService 和内存仓储，覆盖角色 × 单条/批量 × force，共 8 个场景 |
| F2：Grok 共享探测 | 通过。限制标记被带入独立共享 context，角色使用不同 singleflight key | 同一账号并发探测，分别以完整管理员/子管理员先进入；两次 mock 上游请求各自保留正确标记，只有完整管理员触发清限流 |
| 标记来源和传递 | 通过。标记来自数据库最新角色；幂等和 errgroup 子 context 保留值；不能通过客户端参数自行去掉 | AdminAuth 源码、现有鉴权回归和独立 JWT 路由测试 |
| 恢复入口 | 未发现这次限制误伤已允许的恢复路径 | 限制落在用量的 tryClearRecoverableAccountError 和 Grok 额度探测；测试连接的 RecoverAccountAfterSuccessfulTest、重新授权的 ClearAccountError 仍走原路径，邮件追踪保留 |
| P2 | 按用户决定暂缓 | 同时更正首轮描述：当前 Gemini 重新授权弹窗的类型是只读展示，初始化取原账号 oauth_type；首轮“在弹窗选择 AI Studio”不成立。直接构造接口请求的合并残留仍是潜在合同问题，本轮不修改 |

F2 的通过不代表所有 GET 都没有副作用：设计仍保留用量快照缓存、真实 429 的保护性限流及 OpenAI 自动重置策略信号。本次验收关闭的是读取路径直接清错误/清限流的问题，不能对外概括为“全部查询严格只读”。

### 本轮验证

| 命令/检查 | 结果 |
| --- | --- |
| Go 专项：service、handler/admin、middleware、routes，包含子管理员、导入、用量、Grok 配额与权限测试 | 四包通过；完整命令见下方 |
| 独立 Go overlay，`-run '^TestRecheckOperator'` | 3 个测试函数、16 个子场景全部通过：导入 JWT 链 6、用量读取 8、Grok 并发 2 |
| `git diff --check` | 通过 |

独立测试只使用合成 JWT、内存仓储和 mock 上游，没有访问生产账号或发送真实邮件。源码与日志在本机忽略目录 `backend/.gocache/account-operator-recheck/`：`admin_recheck_test.go`、`service_recheck_test.go`、`overlay.json`、`recheck.log`。按 wiki 的 Go 缓存设置，在 backend 目录运行：

```powershell
go test -tags=unit -p 1 -count=1 ./internal/service ./internal/handler/admin ./internal/server/middleware ./internal/server/routes -run 'AccountOperator|AccountActivity|SubAdmin|AdminAuth|AdminPermission|CanAccessAdmin|ImportCodex|AccountUsage|GrokQuota'
go test -overlay .gocache/account-operator-recheck/overlay.json -tags=unit -p 1 -count=1 ./internal/service ./internal/handler/admin -run '^TestRecheckOperator' -v
```

本轮两个修复没有前端代码增量，未重复前端测试；首轮前端验证结果仍作为历史证据。真实浏览器、SMTP、真实 OAuth 提供商和生产验收仍未执行。
