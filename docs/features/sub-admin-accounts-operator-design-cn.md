# 子管理员账号管理（受限运维）设计

状态: 已实现，待审核与上线验证
分支: `feature/hy/10220_sub_admin_accounts`

## 1. 目标

让指定账号进入 `/admin/accounts`：

- 可看：列表全部列、账号详情、用量/今日统计、配额、状态、统计弹窗（凭据仍按现有 DTO 脱敏）。
- 可做：新增账号、测试连接、重新授权。
- 不可做：编辑、删除、复制、调度开关、批量操作、刷新 Token、清错误/恢复状态、重置配额、隐私设置、影子账号、CRS 同步、导入导出、定时测试、上游计费探测、模型同步、各类设置。
- 授权人可配置，支持多人。

## 2. 结论

| 问题 | 结论 | 理由 |
| --- | --- | --- |
| 原页面改 vs 新页面 | 原页面改 | `AccountsView.vue` 约 2600 行、`CreateAccountModal.vue` 7000+ 行，且是上游高频变更文件。复制新页面会长期漏掉上游新平台、新列和修复；原页面只加少量 `v-if`，合并冲突小且可读。项目已有同类先例：`SubscriptionsView` / `UsageView` / `TokenAnalysisView` 都是原页面对子管理员收口。 |
| 授权人如何配置 | 复用 `sub_admin` + 新权限码 | 在用户管理里把账号设为 `sub_admin` 并勾选"账号管理（受限）"即可，可多人、即时生效（每次请求读数据库最新权限）、无需迁移和重启。不在 `config.yaml` 写邮箱列表：需重启，且与现有权限目录形成两套来源。 |

## 3. 权限模型

新增权限码 `admin.accounts_operator`（菜单 `accounts`，路由 `/admin/accounts`），白名单在 `backend/internal/service/admin_permission.go`。后端仍是 HTTP 方法 + Gin 路由模板白名单，未登记即拒绝（`403 ADMIN_PERMISSION_DENIED`）。

| 分组 | 路由（`/api/v1/admin` 前缀） |
| --- | --- |
| 只读 | `GET /accounts`、`GET /accounts/form-options`、`GET /accounts/:id`、`GET /accounts/upstream-billing-rates`、`GET /accounts/:id/stats`、`/usage`、`/today-stats`、`/temp-unschedulable`、`/models`、`/claude/reset-credits`；`GET /grok/accounts/:id/quota`、`GET /cn-providers/accounts/:id/quota`、`/balance`；`GET /gemini/oauth/capabilities`、`GET /grok/oauth/capabilities`；只读 POST：`/accounts/usage/batch`、`/accounts/today-stats/batch`、`/accounts/check-mixed-channel`、`/accounts/models/sync-upstream-preview` |
| 新增 | `POST /accounts`、`POST /accounts/import/codex-session`、`POST /openai/create-from-codex-pat`、`POST /grok/sso-to-oauth` |
| OAuth（新增与重新授权共用） | Claude：`/accounts/generate-auth-url`、`generate-setup-token-url`、`exchange-code`、`exchange-setup-token-code`、`cookie-auth`、`setup-token-cookie-auth`；OpenAI：`/openai/generate-auth-url`、`exchange-code`、`refresh-token`；Gemini：`/gemini/oauth/auth-url`、`exchange-code`；Antigravity：`/antigravity/oauth/auth-url`、`exchange-code`、`refresh-token`；Grok：`/grok/oauth/auth-url`、`exchange-code`、`refresh-token`、`sso-token`、`password` |
| 测试连接 | `POST /accounts/:id/test` |
| 重新授权落库 | `POST /accounts/:id/apply-oauth-credentials` |

其余账号写接口及 `/admin/groups/all`、`/admin/proxies/all`、`/admin/settings/*`、`/admin/tls-fingerprint-profiles` 均不放行。

说明：

- `GET /accounts/:id` 与列表同样经 `RedactCredentials` 脱敏，测试连接和查看统计依赖它加载详情。
- `import/codex-session` 对子管理员只新增：命中已有账号时服务端直接跳过（结果项 `skipped` 并附账号 ID），不改写已有账号的凭据、分组、代理、并发、优先级、倍率、过期时间和 extra，也不另建重复账号；需要换凭据请对该账号使用重新授权。完整管理员的导入行为不变。
- 前端未使用的 `openai/create-from-oauth`、`grok/oauth/create-from-oauth` 不放行。

子管理员请求由 `AdminAuth` 打上 `service.WithSubAdminAccountRestrictions` 标记，服务端据此收口两类"读取/新增附带的写入"：

- 查看用量（单条 `GET /accounts/:id/usage` 与批量 `POST /accounts/usage/batch`）不再调用 `tryClearRecoverableAccountError` 把错误账号恢复为 active。
- Grok 额度探测（`GET /grok/accounts/:id/quota`，以及用量查询内部触发的探测）不再清除限流；探测走 singleflight 共享 context，标记显式带入并与管理员探测分开合并。真实 429 仍会写入限流，这是保护性标记。
- 账号恢复只能通过会发邮件的测试连接或重新授权完成。完整管理员查看页面的既有自动恢复行为不变。
- 用量查询仍会写入用量/额度快照缓存（`extra` 中的快照字段），OpenAI 路径会给"自动重置"服务发信号（与网关调度热路径相同，是否消耗重置次数由已配置策略决定），这些不视为管理状态变更。

## 4. 实施中处理的两个缺口

1. **Gemini / Antigravity 重新授权原先走通用编辑接口。** `frontend/src/components/admin/account/ReAuthAccountModal.vue` 中这两条分支调用 `PUT /accounts/:id` + `clear-error`。已改为与其他平台一致调用 `apply-oauth-credentials`：服务端合并保留 `model_mapping` 等配置，并内置清错误和 token 缓存失效，对完整管理员也是修正。差异：凭据由整体覆盖变为按 key 合并，重新授权时切换 Gemini OAuth 类型可能残留旧的 `project_id`。
2. **分组/代理下拉依赖完整管理接口。** `/admin/proxies/all` 返回含密码的 `AdminProxy`。新增 `GET /api/v1/admin/accounts/form-options`（`backend/internal/handler/admin/account_operator_handler.go`）：
   - `groups`: `id, name, description, platform, status, subscription_type, rate_multiplier, account_count, long_context_pricing_enabled`
   - `proxies`: `id, name, protocol, host, port, status, account_count`（不含用户名/密码）

   完整管理员维持原调用不变，仅子管理员走新接口。

## 5. 前端改动

统一由 `frontend/src/composables/useAccountOperatorAccess.ts` 的 `canManageAccounts`（= 完整管理员）判断。子管理员只要触发一次越权请求，`App.vue` 就会把他重定向出当前页，所以受限模式下页面必须做到零越权请求：

- 路由 `meta.adminPermission: 'admin.accounts_operator'`；`AppSidebar.vue` "管理功能"分区加入账号管理；`utils/adminPermissions.ts` 加 landing；类型与中英文权限文案。用户编辑弹窗的权限勾选项来自后端目录，自动出现。
- `AccountsView.vue`：隐藏行内编辑/删除、勾选列和批量操作栏、"更多"菜单里的数据/工具项（保留列设置）、回退代理按钮；调度开关只读；优先级纯展示；管理员专用弹窗不挂载；分组/代理改走 `form-options`；不读取计费探测全局设置。
- `AccountActionMenu.vue`：只显示"测试连接""查看统计""重新授权"。
- `CreateAccountModal.vue`：不加载 Web Search 设置、额度通知全局设置和 TLS 指纹；创建后跳过 `sync-upstream` 与计费探测（管理员可事后补）。
- 单元格与弹窗内的写按钮隐藏或禁用：OpenAI 配额查询/重置、推荐邀请、Ollama/OpenCode 用量刷新、Claude 重置兑换、上游计费探测、代理测试、临时不可调度"恢复状态"。

前端隐藏只是体验，安全边界以后端白名单为准。

## 6. 操作通知

原则：子管理员造成的账号变动都要暴露出来，宁多勿漏。收件人固定 `huangyu@xunyou.com`（`service.AccountOperatorNotifyRecipient`）。

- 中间件：`AccountOperatorHandler.ActivityTracker()` 挂在整个 admin 路由组上，只对 `sub_admin` 的非 GET 请求生效；每个非 GET 白名单路由在 `accountOperatorActivityRoutes` 中声明通知方式（`create` / `test` / `reauth` / `none`），测试强制两者一致。
  - 新增类：捕获成功响应，提取账号 DTO（`id+platform+type`）或导入结果项（`account_id+action`），重新读取后发"新增账号"邮件；一次请求多个账号合并成一封，导入更新的已有账号单独标注。
  - 重新授权：执行前后各读一次账号，成功后始终发邮件，附变更对比。
  - 测试连接：同样前后比对，只有状态、错误信息、可调度、限流/过载/临时不可调度、上游身份、凭据字段、分组、代理等显著字段变化时才发；只有 `extra` 运行时字段变化不发。
- 发送：`backend/internal/service/account_operator_activity.go` 直接调用 `EmailService.SendEmail` 发送内置中文 HTML，不新增通知模板事件（避免改动上游通知模块）。异步发送，30 秒超时，失败只记日志，不影响业务结果。
- 只通知子管理员的操作，完整管理员的操作不发。
- 邮件内容：操作人、时间、接口、来源 IP、账号 ID/名称/平台/类型、状态、错误信息、运行状态、上游身份、上游地址（非官方地址标红）、分组、代理、优先级/并发/负载因子/倍率、过期时间，以及变更前后对比（凭据和附加配置只列字段名）；附 `/admin/accounts` 链接（需配置前端地址）。不包含任何凭据值。
- 操作记录同时由现有 admin 审计中间件留存，邮件不是唯一凭证。依赖生产环境已配置 SMTP。

## 7. 风险与边界

| 项 | 处理 |
| --- | --- |
| 测试接口可自定义 prompt、跑生图/生视频 | 沿用现有逻辑，不额外限制（已确认） |
| 测试成功会自动恢复账号状态 | 保留，状态变化会发邮件 |
| 重新授权可作用于任意 OAuth 账号，可换成另一个上游账号 | 允许（已确认），每次都发邮件，上游身份变化在邮件里标出 |
| 新增 API Key 类账号可填任意 `base_url`，用户请求会转发过去 | 允许（已确认），邮件标红非官方地址 |
| 新增时可设分组、优先级、并发、倍率，立即参与调度 | 不限制，邮件列出这些字段 |
| 借重新授权顺带修改 `extra` | 不过滤 key，变更的附加配置字段名会列在邮件里 |
| 借 Codex 导入改写已有账号 | 子管理员命中已有账号一律跳过（审核 F1） |
| 查看用量/额度顺带恢复错误或限流 | 子管理员请求不触发恢复（审核 F2） |
| Gemini 重新授权切换 OAuth 类型残留旧 `project_id` | 弹窗中类型只读、Code Assist 换码无项目会报错，页面无法触发；暂无 Gemini 订阅，暂不加固（审核 F3） |
| 子管理员填错不能改、不能删 | 第一版不开放，由管理员根据邮件修正 |
| 子管理员不能新建代理/分组 | 需要时先找管理员建 |
| 上游后续新增账号路由或创建方式 | 新路由默认拒绝；白名单路由必须真实注册且声明通知方式，测试强制 |

## 8. 测试

- 后端：`internal/service/account_operator_restrictions_test.go`（子管理员单条/批量用量不清错误，管理员保持原恢复）；`internal/handler/admin/account_operator_import_test.go`（子管理员导入跳过已有账号）；`account_operator_activity_test.go` 中的 Grok 探测不清限流；`admin_auth_test.go` 校验标记只给子管理员；`internal/service/admin_permission_test.go`（允许/拒绝表、写路由收窄、通知归类）；`internal/service/account_operator_activity_test.go`（响应解析、测试无变化不发信、状态恢复发信、重新授权不泄露凭据、批量合并与非官方地址标红）；`internal/handler/admin/account_operator_handler_test.go`（中间件只跟踪子管理员已归类写操作、`form-options` 不含代理认证信息）；`internal/server/routes/admin_permission_routes_test.go`（白名单路由都已注册）。
- 前端：`AccountsView.accountsOperator.spec.ts`、`AccountActionMenu.accountsOperator.spec.ts`、`ReAuthAccountModal.applyCredentials.spec.ts`、`adminPermissions.spec.ts`。

## 9. 决策记录

已确认（2026-10-25）：

1. "测试、连接"即行菜单"测试连接"一个按钮。
2. 新增账号不审核，直接进入调度；改为邮件通知 `huangyu@xunyou.com`（写死）。
3. 第一版不做"查看详情"。
4. 测试连接沿用现有逻辑。
5. 重新授权不限制账号范围，每次发邮件。
6. 自定义 `base_url` 允许，邮件标红。
7. 通知原则：大的变动都发邮件，宁多勿漏。
8. 审核 F1：子管理员 Codex 导入命中已有账号时跳过，不更新。
9. 审核 F2：子管理员查看用量/额度不触发错误或限流恢复，恢复只走测试连接/重新授权。
10. 审核 F3：暂不处理（当前无 Gemini 订阅，页面不可触发）。
