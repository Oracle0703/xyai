# 子管理员账号管理（受限运维）设计

更新：2026-11-04。状态：**已实现并合入 main（分支 `feature/hy/10220_sub_admin_accounts`，提交 `25bedc620`）。实现审核的两个 P1（F1 导入改写已有账号、F2 读取清错误/清限流）已修复并通过独立复核；P2（F3 Gemini 旧 `project_id` 残留）按用户决定暂缓。真实浏览器、真实 SMTP、真实 OAuth 提供商端到端验收尚未执行（§11）。**

本文是受限账号运维 `admin.accounts_operator` 的唯一文档，包含目标、权限白名单、副作用收口、前端收口、操作通知、易错点和验证方式。实现审核原文（含审核发现、修复记录和独立复核）已移出仓库，最后存在于提交 `26f6363af`（`git show 26f6363af:docs/reviews/sub-admin-accounts-operator-implementation-audit-cn.md`）；凡是对后续修改有约束的结论都已写入正文。前置机制：[子管理员角色设计](sub-admin-role-design-cn.md)（角色、权限码、`AdminAuth` 白名单）。

阅读顺序建议：
- 开放新的账号接口或改白名单：先读 §3、§4 和 §9；
- 改账号页面或弹窗：读 §5；
- 改通知：读 §6；
- 跑测试：读 §10。

## 1. 目标

让指定账号进入 `/admin/accounts`：

- 可看：列表全部列、账号详情、用量/今日统计、配额、状态、统计弹窗（凭据仍按现有 DTO 脱敏）。
- 可做：新增账号、测试连接、重新授权。
- 不可做：编辑、删除、复制、调度开关、批量操作、刷新 Token、清错误/恢复状态、重置配额、隐私设置、影子账号、CRS 同步、导入导出、定时测试、上游计费探测、模型同步、各类设置。
- 授权人可配置，支持多人。

## 2. 方案取舍

| 问题 | 结论 | 理由 |
| --- | --- | --- |
| 原页面改 vs 新页面 | 原页面改 | `AccountsView.vue` 约 2600 行、`CreateAccountModal.vue` 7000+ 行，且是上游高频变更文件。复制新页面会长期漏掉上游新平台、新列和修复；原页面只加少量 `v-if`，合并冲突小。同类先例：`SubscriptionsView`、`UsageView`、`TokenAnalysisView` 都是原页面对子管理员收口。 |
| 授权人如何配置 | 复用 `sub_admin` + 新权限码 | 在用户管理里把账号设为 `sub_admin` 并勾选"账号管理（受限）"，可多人、即时生效（每次请求读数据库最新权限）、无需迁移和重启。已否决：在 `config.yaml` 写邮箱列表，原因：需重启，且与权限目录形成两套来源。 |

## 3. 权限白名单

权限码 `admin.accounts_operator`（菜单 `accounts`，路由 `/admin/accounts`），白名单在 `backend/internal/service/admin_permission.go` 的 `adminPermissionRouteRules`。仍是 HTTP 方法 + Gin 路由模板白名单，未登记即拒绝（`403 ADMIN_PERMISSION_DENIED`）。

| 分组 | 路由（省略 `/api/v1/admin` 前缀） |
| --- | --- |
| 只读 | `GET /accounts`、`GET /accounts/form-options`、`GET /accounts/:id`、`GET /accounts/upstream-billing-rates`、`GET /accounts/:id/stats`、`/usage`、`/today-stats`、`/temp-unschedulable`、`/models`、`/claude/reset-credits`；`GET /grok/accounts/:id/quota`、`GET /cn-providers/accounts/:id/quota`、`/balance`；`GET /gemini/oauth/capabilities`、`GET /grok/oauth/capabilities`；只读 POST：`/accounts/usage/batch`、`/accounts/today-stats/batch`、`/accounts/check-mixed-channel`、`/accounts/models/sync-upstream-preview` |
| 新增 | `POST /accounts`、`POST /accounts/import/codex-session`、`POST /openai/create-from-codex-pat`、`POST /grok/sso-to-oauth` |
| OAuth（新增与重新授权共用） | Claude：`/accounts/generate-auth-url`、`generate-setup-token-url`、`exchange-code`、`exchange-setup-token-code`、`cookie-auth`、`setup-token-cookie-auth`；OpenAI：`/openai/generate-auth-url`、`exchange-code`、`refresh-token`；Gemini：`/gemini/oauth/auth-url`、`exchange-code`；Antigravity：`/antigravity/oauth/auth-url`、`exchange-code`、`refresh-token`；Grok：`/grok/oauth/auth-url`、`exchange-code`、`refresh-token`、`sso-token`、`password` |
| 测试连接 | `POST /accounts/:id/test` |
| 重新授权落库 | `POST /accounts/:id/apply-oauth-credentials` |

其余账号写接口（`PUT /accounts/:id`、删除、复制、调度开关、刷新 token、清错误、批量、CRS、导入导出、模型同步、计费探测）及 `/admin/groups/all`、`/admin/proxies/all`、`/admin/settings/*`、`/admin/tls-fingerprint-profiles` 均不放行。前端未使用的 `openai/create-from-oauth`、`grok/oauth/create-from-oauth` 不放行。

说明：
- `GET /accounts/:id` 与列表同样经 `RedactCredentials` 脱敏；测试连接和查看统计依赖它加载详情。
- `GET /accounts/openai-request-timezones` 也是账号表单所需的只读白名单接口，仅返回固定默认值和允许的时区名，不读账号/设置或请求上游；只给 `admin.accounts_operator` 子管理员放行。前端 GET 自动附带的 `timezone` 查询参数不参与权限判断。
- `GET /accounts/antigravity/default-model-mapping` 同属只读白名单，仅返回代码常量映射；新建表单切换 Antigravity 和关闭后的表单重置都会加载它。
- `import/codex-session` 对子管理员只新增，规则见 §4。

## 4. 副作用收口（读取与新增附带的写入）

白名单只决定"能不能调"。被放行的既有接口里藏着状态变更，需要按操作者角色再收口。`AdminAuth` 给所有 `sub_admin` 请求打上 `service.WithSubAdminAccountRestrictions` 标记（来源是数据库最新角色，客户端参数不能去掉；幂等 helper 和 errgroup 子 context 会保留该值）。服务端据此收口：

1. **Codex 导入命中已有账号一律跳过。** 子管理员调用 `import/codex-session` 时，命中已有账号的条目返回 `skipped` 并附已有账号 ID，不改写凭据、分组、代理、并发、优先级、倍率、负载因子、过期时间和 `extra`，也不另建重复账号；同批次中的新账号照常新增。需要换凭据请对该账号使用重新授权。完整管理员行为不变（`update_existing` 缺省或 `true` 时更新已有账号，`false` 保持原创建行为）。
2. **查看用量不清错误。** 单条 `GET /accounts/:id/usage` 与批量 `POST /accounts/usage/batch` 对子管理员不调用 `AccountUsageService.tryClearRecoverableAccountError`，`force=true` 也不能绕过。
3. **Grok 额度探测不清限流。** `GET /grok/accounts/:id/quota` 及用量查询内部触发的探测，对子管理员不清除限流。探测在 singleflight 的独立 context 中运行，标记必须显式带入，并作为 singleflight key 的一部分与管理员探测分开合并。真实 429 仍会写入限流，这是保护性标记。
4. **账号恢复只能经测试连接或重新授权。** 测试连接成功走 `RecoverAccountAfterSuccessfulTest`，重新授权走清错误与 token 缓存失效；这两条路径都会发邮件（§6）。完整管理员查看页面的既有自动恢复行为不变。

仍保留的读取副作用（产品已接受，不视为管理状态变更）：用量/额度快照写入 `extra` 中的快照字段；OpenAI 路径给"自动重置"服务发信号（与网关调度热路径相同，是否消耗重置次数由已配置策略决定）；真实 429 的保护性限流。因此本节的结论是"读取不直接清错误/清限流"，不能概括为"所有查询严格只读"。

## 5. 前端

统一由 `frontend/src/composables/useAccountOperatorAccess.ts` 的 `canManageAccounts`（= 完整管理员）判断。子管理员触发一次越权请求，`App.vue` 就会因 `ADMIN_PERMISSION_DENIED` 把他重定向出当前页，所以受限模式下页面必须做到零越权请求：

- 路由 `meta.adminPermission: 'admin.accounts_operator'`；`AppSidebar.vue` "管理功能"分区加入账号管理；`utils/adminPermissions.ts` 加落地页；类型与中英文权限文案。用户编辑弹窗的权限勾选项来自后端目录。
- `AccountsView.vue`：隐藏行内编辑/删除、勾选列和批量操作栏、"更多"菜单里的数据/工具项（保留列设置）、回退代理按钮；调度开关只读；优先级纯展示；管理员专用弹窗不挂载；分组/代理改走 `form-options`；不读取计费探测全局设置。
- `AccountActionMenu.vue`：只显示"测试连接""查看统计""重新授权"。
- `CreateAccountModal.vue`：不加载 Web Search 设置、额度通知全局设置和 TLS 指纹；创建后跳过 `sync-upstream` 与计费探测（管理员可事后补）。
- 单元格与弹窗内的写按钮隐藏或禁用：OpenAI 配额查询/重置、推荐邀请、Ollama/OpenCode 用量刷新、Claude 重置兑换、上游计费探测、代理测试、临时不可调度"恢复状态"。
- `ReAuthAccountModal.vue`：所有平台（含 Gemini、Antigravity）统一调用 `apply-oauth-credentials`。此前 Gemini/Antigravity 分支调用 `PUT /accounts/:id` + `clear-error`；改为服务端按 key 合并（`MergeCredentials`），保留 `model_mapping` 等配置并内置清错误和 token 缓存失效，对完整管理员也是修正。代价见 §9 第 4 条。
- 分组/代理下拉：`GET /api/v1/admin/accounts/form-options`（`AccountOperatorHandler.FormOptions`），完整管理员维持原调用，仅子管理员走新接口。
  - `groups`：`id, name, description, platform, status, subscription_type, rate_multiplier, account_count, long_context_pricing_enabled`
  - `proxies`：`id, name, protocol, host, port, status, account_count`（不含用户名/密码）

前端隐藏只是体验，安全边界以后端白名单和 §4 收口为准。

## 6. 操作通知

原则：子管理员造成的账号变动都要暴露出来，宁多勿漏。收件人写死为代码常量 `service.AccountOperatorNotifyRecipient`。

- 中间件：`AccountOperatorHandler.ActivityTracker()` 挂在整个 admin 路由组上，只对 `sub_admin` 的非 GET 请求生效。每个非 GET 白名单路由必须在 `accountOperatorActivityRoutes` 声明通知方式（`create` / `test` / `reauth` / `none`，查询函数 `AccountOperatorActivityKindForRoute`），`TestAccountOperatorWritesAreClassifiedForNotification` 强制两者一致。
  - 新增类：捕获成功响应，提取账号 DTO（`id+platform+type`）或导入结果项（`account_id+action`），重新读取后发"新增账号"邮件；一次请求多个账号合并成一封。
  - 重新授权：执行前后各读一次账号，成功后始终发邮件，附变更对比。
  - 测试连接：前后比对，只有状态、错误信息、可调度、限流/过载/临时不可调度、上游身份、凭据字段、分组、代理等显著字段变化时才发；只有 `extra` 运行时字段变化不发。
- 发送：`backend/internal/service/account_operator_activity.go` 直接调用 `EmailService.SendEmail` 发送内置中文 HTML，不新增通知模板事件（避免改动上游通知模块）。异步、30 秒超时、失败只记日志，不影响业务结果。没有持久队列、重试或送达保证，不能承诺"每次变更一定收到邮件"；操作留痕以 admin 审计中间件为准。
- 只通知子管理员的操作，完整管理员的操作不发。
- 邮件内容：操作人、时间、接口、来源 IP、账号 ID/名称/平台/类型、状态、错误信息、运行状态、上游身份、上游地址（非官方地址标红）、分组、代理、优先级/并发/负载因子/倍率、过期时间，以及变更前后对比（凭据和附加配置只列字段名，凭据比较用摘要，正文 HTML 转义）；附 `/admin/accounts` 链接（需配置前端地址）。不包含任何凭据值。依赖生产环境已配置 SMTP。

## 7. 风险与边界

| 项 | 处理 |
| --- | --- |
| 测试接口可自定义 prompt、跑生图/生视频 | 沿用现有逻辑，不额外限制（已确认） |
| 测试成功会自动恢复账号状态 | 保留，状态变化会发邮件 |
| 重新授权可作用于任意 OAuth 账号，可换成另一个上游账号 | 允许（已确认），每次都发邮件，上游身份变化在邮件里标出 |
| 新增 API Key 类账号可填任意 `base_url`，用户请求会转发过去 | 允许（已确认），邮件标红非官方地址 |
| 新增时可设分组、优先级、并发、倍率，立即参与调度 | 不限制，邮件列出这些字段 |
| 借重新授权顺带修改 `extra` | 不过滤 key，变更的附加配置字段名会列在邮件里 |
| 借 Codex 导入改写已有账号 | 子管理员命中已有账号一律跳过（§4 第 1 条） |
| 查看用量/额度顺带恢复错误或限流 | 子管理员请求不触发恢复（§4 第 2、3 条） |
| Gemini 重新授权切换 OAuth 类型残留旧 `project_id` | 暂缓，见 §9 第 4 条 |
| 子管理员填错不能改、不能删 | 第一版不开放，由管理员根据邮件修正 |
| 子管理员不能新建代理/分组 | 需要时先找管理员建 |
| 上游后续新增账号路由或创建方式 | 新路由默认拒绝；白名单路由必须真实注册且声明通知方式，测试强制 |

## 8. 决策记录（2026-10-25 用户确认）

1. "测试、连接"即行菜单"测试连接"一个按钮。
2. 新增账号不审核，直接进入调度；改为邮件通知（收件人写死）。
3. 第一版不做"查看详情"。
4. 测试连接沿用现有逻辑。
5. 重新授权不限制账号范围，每次发邮件。
6. 自定义 `base_url` 允许，邮件标红。
7. 通知原则：大的变动都发邮件，宁多勿漏。
8. 子管理员 Codex 导入命中已有账号时跳过，不更新（F1）。
9. 子管理员查看用量/额度不触发错误或限流恢复，恢复只走测试连接/重新授权（F2）。
10. Gemini 旧 `project_id` 残留暂不处理（F3，当前无 Gemini 订阅，页面不可触发）。

## 9. 关键约束与易错点

1. **白名单不能证明接口无副作用，开放既有接口前要沿调用链查状态变更。** 曾复现的失败场景（F2）：Gemini 账号错误为 `Token refresh failed: invalid_client`，用真实 `GetUsage`/`GetUsageBatch` 和内存仓储查询，单条与批量各调用一次 `ClearError`，账号被恢复为 active；而通知中间件跳过所有 GET、批量用量归类为 `none`、审计中间件的敏感 GET 列表也不含 usage，这次恢复没有任何留痕。`tryClearRecoverableAccountError` 对含 `token refresh failed`、`invalid_client`、`missing_project_id`、`unauthenticated` 的错误都会清除，Antigravity 等分支也有类似恢复路径。
2. **"新增"接口可能带"更新已有"分支。** 曾复现的失败场景（F1）：Codex 导入前端提交 `update_existing: true` 和整张创建表单；命中已有账号后构造的 `UpdateAccountInput` 含 `concurrency=99, priority=1, rate=0.01, proxy=77, groups=[88]`，绕开了对 `PUT /accounts/:id` 的拒绝。边界必须在服务端按角色实现，不能依赖前端不传。
3. **依赖请求 context 的限制在 singleflight / `context.Background()` 处会断链。** Grok 探测在独立共享 context 中执行，限制标记要显式带入并进入 singleflight key；否则管理员与子管理员并发探测同一账号时会共用先到者的行为。复核用例：同一账号并发探测、两种角色分别先进入，只有完整管理员触发清限流。新增类似限制时同样检查。
4. **重新授权按 key 合并会残留与新 OAuth 类型不兼容的身份字段（暂缓）。** 接口合同问题：Code Assist 账号改按 AI Studio 授权时，换码结果无 `project_id`（`GeminiTokenInfo.ProjectID` 为 `omitempty`），`ApplyOAuthCredentials` 的 `MergeCredentials` 会把旧 `project_id` 补回，而 Gemini 转发以 `project_id` 是否非空选择 Code Assist 并使用旧项目，影响实际调用。直接构造接口请求可复现（响应 200，落库凭据仍含旧项目）；页面不可触发，因为弹窗的 Gemini 类型只读、取自原 `oauth_type`，Code Assist 换码无项目会报错。完整管理员同样受影响。启用 Gemini 订阅前要加固：类型切换时清理旧 `project_id`、保留 `model_mapping`，反向切换使用新项目。
5. **每个非 GET 白名单路由都要归类通知方式。** 漏归类会被 `TestAccountOperatorWritesAreClassifiedForNotification` 拦下；白名单路由还必须真实注册（`TestSubAdminPermissionWhitelistMatchesRegisteredAdminRoutes`）。注册一致性测试只能检查“白名单路由是否存在”，无法发现新表单接口未加入白名单；`openai-request-timezones` 曾因此对子管理员返回 403，新增依赖还需角色允许/拒绝回归。
6. **分组/代理下拉不得改回完整接口。** `/admin/proxies/all` 返回含密码的 `AdminProxy`；子管理员只走 `form-options`。
7. **受限页面必须零越权请求。** 页面任何一处对未放行接口的请求都会触发 403 恢复、把用户踢出账号页；改 `AccountsView`、`CreateAccountModal` 或单元格组件时，管理员专用加载请求要按 `canManageAccounts` 判断。检查需覆盖首次加载、手动刷新、平台切换与弹窗关闭重置；计费探测设置曾只在 `onMounted` 检查权限，手动刷新仍发出越权请求。守卫现置于 `loadUpstreamBillingProbeGlobalState` 内，该设置接口仍不对子管理员放行；时区选项、Antigravity 默认映射则按 §3 放行只读接口。

## 10. 验证

在 `backend/` 下（`admin_permission_test.go` 等只在 unit 标签下编译）：

```bash
go test -tags=unit -p 1 -count=1 ./internal/service ./internal/handler/admin ./internal/server/middleware ./internal/server/routes -run 'AccountOperator|AccountActivity|SubAdmin|AdminAuth|AdminPermission|CanAccessAdmin|ImportCodex|AccountUsage|GrokQuota'
```

测试清单：
- `internal/service/account_operator_restrictions_test.go`：子管理员单条/批量用量不清错误，管理员保持原恢复。
- `internal/handler/admin/account_operator_import_test.go`：子管理员导入跳过已有账号。
- `internal/service/account_operator_activity_test.go`：响应解析、测试无变化不发信、状态恢复发信、重新授权不泄露凭据、批量合并与非官方地址标红、Grok 探测不清限流。
- `internal/server/middleware/admin_auth_test.go`：限制标记只给子管理员；真实 JWT 鉴权链覆盖时区选项与默认模型映射接口带/不带查询参数、所有子管理员权限及同一 JWT 撤权后的拒绝。
- `internal/service/admin_permission_test.go`：允许/拒绝表、写路由收窄、通知归类。
- `internal/handler/admin/account_operator_handler_test.go`：中间件只跟踪子管理员已归类写操作、`form-options` 不含代理认证信息。
- `internal/server/routes/admin_permission_routes_test.go`：白名单路由都已注册。
- 前端：`AccountsView.accountsOperator.spec.ts`（含子管理员刷新跳过设置、完整管理员刷新保留设置）、`AccountActionMenu.accountsOperator.spec.ts`、`ReAuthAccountModal.applyCredentials.spec.ts`、`adminPermissions.spec.ts`。

最近结果：
- 2026-11-09（本机）：时区选项、Antigravity 默认映射补入只读白名单，手动刷新跳过子管理员的计费探测设置请求；修复前 JWT/前端回归均复现遗漏，修复后 `-tags=unit -p 1 -count=1` 下 service、server/middleware、server/routes 的 `AdminPermission|CanAccessAdmin|SubAdmin|AdminAuth|AccountOperatorWrites` 专项全部通过，覆盖查询参数、各权限隔离与撤权。前端账号页/菜单/重新授权/新建/时区/模型选择 6 文件 104 用例、typecheck、改动文件 ESLint 通过。核对了页面加载/刷新、各平台新增与重新授权、测试、统计、用量及管理员专用入口；未做线上或浏览器验收。
- 2026-11-04（HEAD `26f6363af`，本机）：`-tags=unit` 下 `AccountOperator` 与权限白名单定向测试在 service、handler/admin、server/routes 通过。
- 2026-10-25 实现审核（修复前）：上述 Go 四包在 `-tags=unit` 下通过（默认标签下 service、middleware 无匹配测试，不算有效覆盖）；本功能相关前端 Vitest 25 个文件 / 279 个用例通过；typecheck、变更文件 ESLint、`git diff --check` 通过。另用 Go overlay 写的 3 个反例测试按预期失败，分别证实 F1、F2（单条/批量）、F3。
- 2026-10-25 修复后独立复核：四包通过；独立 overlay 测试 16 个子场景全部通过（导入 JWT 链 6：覆盖 `update_existing` 缺省/true/false × 两种角色；用量读取 8：角色 × 单条/批量 × `force`；Grok 并发 2）。只使用合成 JWT、内存仓储和 mock 上游，未访问生产账号、未发真实邮件。修复无前端增量，未重跑前端。

## 11. 状态与待办

- [ ] 真实浏览器验收（完整管理员 / 子管理员双角色），重点确认受限页面零越权请求。
- [ ] 真实 SMTP 发送一次新增、重新授权、测试连接状态变化三类邮件。
- [ ] 真实 OAuth 提供商下的重新授权验收。
- [ ] 启用 Gemini 订阅前修复 §9 第 4 条（类型切换清理旧 `project_id`），完整管理员和子管理员都覆盖，并用数据合同测试替代当前只扫描源码的前端测试。
- 未做（第一版范围外）：子管理员编辑/删除自己新增的账号、新建代理/分组、查看详情页。
