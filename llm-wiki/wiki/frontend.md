# 前端知识基线

## 技术栈与入口

- Vue 3 + Composition API + TypeScript。
- 构建: Vite 5。
- 状态管理: Pinia。
- 路由: Vue Router 4。
- 样式: TailwindCSS, 全局样式在 `frontend/src/style.css` 和 `frontend/src/styles/`。
- 国际化: `vue-i18n`, 入口 `frontend/src/i18n/index.ts`。
- `frontend/src/i18n/locales/en.ts` / `zh.ts` 已拆分为 `locales/{en,zh}/index.ts` + `common/dashboard/landing/misc` + `admin/*` 域模块; 新增文案应放入对应域模块, 并保留 `localesNoKeyCollision.spec.ts` 的 spread 键冲突守卫。
- 包管理器: pnpm, 不使用 npm/yarn。

## 0.1.171 合并增量

- 登录、注册、OAuth start 和 Passkey 登录可通过 `ActionCaptchaRequestProof` 传动作验证码。腾讯验证码组件为 `TencentCaptchaGate.vue` / `TencentCaptchaGate` 流程, 阿里云组件为 `AliyunCaptchaWidget.vue`; 阿里云 `captchaVerifyParam` 复用 `turnstile_token` 字段, 腾讯使用 `tencent_captcha_ticket` 与 `tencent_captcha_randstr`。
- `authStore.loginWithPasskey(proof?)` 会把动作验证码 proof 传给 `passkeyAPI.login`, 并继续复用 token/session 落盘流程；合并 `frontend/src/stores/auth.ts` 时必须同时保留 `ActionCaptchaRequestProof` 类型和本地 `AdminPermission` / 子管理员权限逻辑。
- `GroupsView.vue` 启用 OpenAI Live 时会调用 `adminAPI.groups.getLiveCapability()`。服务端不支持 Live attestation 时, UI 需要二次确认后才提交 `allow_live=true`; 单测 mock `@/api/admin` 时要补齐该方法, 否则会产生挂载后的 unhandled rejection。

## 0.1.170 合并增量

- OEM 设置新增 `compact_home_enabled`（默认 `false`）, 并随 public settings 返回。`HomeView.vue` 优先渲染 trim 后非空的 `home_content`; 只有内容为空时才按该开关显示内置简洁首页。简洁首页根据认证状态跳转 `/login`、`/dashboard` 或 `/admin/dashboard`。
- `GroupsView.vue` 只为 `openai`、`anthropic`、`gemini`、`grok`、`antigravity` 显示和提交利润控制。辅助逻辑在 `frontend/src/views/admin/groupsProfitControl.ts`, 界面用百分比编辑, API/数据库使用小数；启用时 margin、buffer 均在 `[0,1)` 且和小于 1, 并先拦截 `margin + buffer >= 100%` 与四舍五入后等于 1 的边界；切换到不支持的平台时清零并关闭。
- 所有 API Key 平台账号都可配置 upstream billing probe；启用 `upstream_billing_rate_sync_enabled` 后必须禁用人工 `rate_multiplier` 编辑。只有 OpenAI OAuth 账号显示 `extra.openai_responses_flatten_namespaces` 兼容开关, 缺省保持 namespace 原样。`ModelWhitelistSelector` 等跨平台组件需按 selected platforms 合并候选。
- 账号列表的“选择全部结果”按当前筛选遍历全部页收集 ID；批量删除调用 `POST /api/v1/admin/accounts/batch-delete`, 并按 `success_ids` / `failed_ids` 展示部分成功结果。
- 风险控制页 `RiskControlView.vue` 的内容审核保存与 API Key 测试都支持代理选择：省略表示沿用, `0` 表示强制直连/清除, 正数表示代理 ID。依赖 `ProxySelector` 与 `riskControlAPI` 的 `proxy_id` 字段；代理列表加载失败不能阻断页面其他配置。

## 0.1.168 合并增量

- `/model-plaza` 是公开声明但受 public settings 双门控的页面：`model_plaza_enabled` 控制入口和 API 是否存在, `model_plaza_require_auth` 决定匿名访问是否允许。页面由 `ModelPlazaView.vue` 与 `components/modelPlaza/` 组成, 登录用户从可选 JWT 取得个人倍率；未登录用户只看到可公开分组。
- Passkey client 位于 `frontend/src/api/passkey.ts`, 负责 WebAuthn JSON 编解码和 `navigator.credentials` 调用；`authStore.loginWithPasskey()` 复用正常 token/session 落盘流程。登录页只在服务端开关开启且浏览器支持时显示入口, Profile 的 `ProfilePasskeyCard` 支持注册、重命名和删除。
- `frontend/src/stores/auth.ts` 的合并合同同时包含上游 `passkeyAPI` 和本地 `AdminPermission` / 子管理员权限计算, 两者不能在冲突解决中互相覆盖。

主入口:

- `frontend/src/main.ts`: 初始化主题, Pinia, 注入配置, 首屏标题/favicon, i18n, router, mount。
- `frontend/src/App.vue`: 全局导航进度, RouterView, Toast, AnnouncementPopup, setup 检查, 公共设置加载, branding 热更新和子管理员权限拒绝恢复。

## Vite 构建行为

`frontend/vite.config.ts`:

- `@` 指向 `frontend/src`。
- `vue-i18n` alias 到 runtime 版本, 避免 CSP unsafe-eval。
- dev server 默认端口来自 `VITE_DEV_PORT` 或 `3000`。
- dev proxy 转发 `/api`, `/v1`, `/setup` 到 `VITE_DEV_PROXY_TARGET` 或 `http://localhost:8080`。
- build 输出到 `../backend/internal/web/dist`, 供后端嵌入。
- dev 模式会尝试从后端 `/api/v1/settings/public` 注入 `window.__APP_CONFIG__`, 并在 HTML 返回前注入安全转义的站点标题/favicon, 模拟生产 embedded HTML 注入行为。默认 favicon 与静态品牌资源已统一为 `/logo.svg`, README 使用 `assets/logo.svg`; 自定义 favicon 只接受相对路径、HTTP(S) 或 `data:image/*`, runtime 统一复用 `frontend/src/utils/branding.ts`。

## 路由与守卫

`frontend/src/router/index.ts` 集中定义路由。

主要分组:

- setup: `/setup`
- public: `/home`, `/login`, `/register`, OAuth callback, `/key-usage`, `/image-gen`, `/legal/:documentId`
- batch image: `/batch-image`(alias `/docs/batch-image`) 使用 `BatchImageGuideView.vue`, 侧栏入口由 `useBatchImageAccess` 按用户/分组能力刷新显示。
- user: `/dashboard`, `/keys`, `/usage`, `/redeem`, `/affiliate`, `/available-channels`, `/profile`, `/subscriptions`, `/purchase`, `/orders`, payment 页面, `/custom/:id`
- admin: `/admin/dashboard`, `/admin/ops`, `/admin/users`, `/admin/groups`, `/admin/channels/*`, `/admin/accounts`, `/admin/audit-logs`, `/admin/settings`, `/admin/risk-control`, `/admin/prompt-audit`, `/admin/request-intercept`, `/admin/usage`, `/admin/organization-usage`, `/admin/token-analysis`, payment admin, affiliate admin

守卫要点:

- 首次导航调用 `authStore.checkAuth()` 恢复 localStorage 会话。
- `requiresAuth` 默认 true, 显式 false 才是公开页。
- `requiresAdmin` 使用 `authStore.canAccessAdmin`; 子管理员路由还必须声明 `meta.adminPermission`, 缺失或不匹配时跳到安全落点。
- `requiresPayment` 依赖 public settings 中的 payment 开关。
- `requiresRiskControl` 依赖 risk control 开关。
- feature route guard 会先等待 `appStore.fetchPublicSettings()`; 只有 settings 已成功加载且开关显式为 `false` 才重定向。加载失败属于未知状态, 不能误判为功能关闭。`app.ts` 用单一 in-flight promise 合并并发 public-settings 请求, force refresh 也不能让旧请求覆盖新结果。
- simple mode 对用户侧受限页面一视同仁, 不能因裸 `sub_admin` 角色绕过。
- backend mode 下完整管理员进入管理总览; 至少有一项权限的子管理员进入首个授权页; 空权限子管理员只能停留在登录页。
- chunk load error 会触发一次页面 reload。
- 页面标题由 `frontend/src/router/title.ts` 的 `resolveRouteDocumentTitle` 统一生成; `CustomPage` 会优先使用公开自定义菜单项或管理员自定义菜单项 label, 语言切换、站点名变化、自定义菜单加载后都会重新解析标题。

## API Client

`frontend/src/api/client.ts`:

- Axios baseURL 由 `frontend/src/api/url.ts#getAPIBaseURL` 统一归一化: `VITE_API_BASE_URL` 或 `/api/v1`, 会去尾斜杠并支持绝对 URL。
- 默认 `withCredentials: true`, timeout 30s。
- 请求拦截:
  - 从 localStorage 读取 `auth_token` 写入 Authorization。
  - 写入 `Accept-Language`。
  - GET 请求追加 `timezone`。
  - 管理端页面请求继续按路径/页面附加 `X-Admin-UI-Request: 1`; allowlist 内的已认证用户 Web API 还会附加 `X-User-UI-Request: 1`, 供 opt-in Server-Timing 确定采集范围。两个 header 都不是授权凭据, 后端仍按认证角色和路径决定是否返回 timing; public payment/webhook、登录和网关请求不加用户标记。
- 响应拦截:
  - 自动解包 `{ code, message, data }`。
  - 401 时使用 `refresh_token` 调 `/auth/refresh`, 并重试原请求。
  - refresh 失败会清理 localStorage 并跳转 `/login`。
- 直连 fetch/WebSocket/setup 等不走 Axios 的请求必须使用 `buildApiUrl` 或 `buildGatewayUrl`, 避免部署在自定义 `VITE_API_BASE_URL` 时仍打到当前 origin; `buildGatewayUrl` 用于 `/setup`, `/api/v1/admin/ops/ws/qps` 等网关根路径。
  - 管理端可观测请求使用 `frontend/src/api/adminUIRequest.ts`, 统一附加 `X-Admin-UI-Request: 1`; 普通用户和第三方请求不要伪造该标记。后端仍会校验已认证 admin role 后才返回 Server-Timing。
  - ops disabled 的 404 会写缓存并跳转设置页。

API 模块分布:

- 用户侧: `frontend/src/api/auth.ts`, `keys.ts`, `usage.ts`, `user.ts`, `redeem.ts`, `payment.ts`, `groups.ts`, `channels.ts`, `totp.ts`, `channelMonitor.ts`。
- 管理侧: `frontend/src/api/admin/**`。
- 统一导出: `frontend/src/api/index.ts` 和 `frontend/src/api/admin/index.ts`。

管理端账号与监控:

- `CreateAccountModal.vue` 的 Grok OAuth 流支持 Web SSO key 批量导入, 每行一个 key, 通过 `adminAPI.grok.createFromSSO` 提交; SSO 模式允许账号名留空, 部分成功时保留失败明细。修改该流程时同步 `OAuthAuthorizationFlow.vue`、`useGrokOAuth.ts`、中英文 `admin/accounts.ts` 和 `CreateAccountModal.spec.ts`。
- `CreateAccountModal.vue` 的 Antigravity 批量 refresh-token 导入会保留管理员原始输入并传给 OAuth 组合逻辑, 不能只传解析后的 credential 结果, 否则手工 refresh token 会在后续流程中丢失。
- OpenAI OAuth/API Key 账号增加 `openai_long_context_billing_enabled` 开关, 默认关闭; Codex session/PAT 导入只有用户实际触碰开关时才覆盖服务端默认, 避免旧导入流程无意开启长上下文计费。
- API Key 账号创建默认提交 `upstream_billing_probe_enabled=true`, 账号创建成功后等待首次 probe 再刷新列表；用户可在创建弹窗关闭该开关。该能力不限 OpenAI 平台, 但非 OpenAI 官方根域账号会由后端标记为 `unsupported`。该状态与本地 compatible provider preset 独立, 两者必须同时进入最终 payload。
- 管理端 OpenAI 分组可通过 `allow_live` 显式开放 Live；前端保存前调用 `/api/v1/admin/groups/live-capability` 检查服务端 attestation 能力, 不支持时必须二次确认, 默认仍关闭。服务端平台和权限校验是最终边界。
- Channel Monitor 支持 Grok provider、模板和筛选; `GrokQuotaProbeCell.vue` 的 Free 配额显示按本地滚动 24 小时 Token 用量估算, 与上游 weekly header 分开展示。
- `AuditLogView.vue` 通过 `frontend/src/api/admin/audit.ts` 查询操作审计; 清空要求现场 TOTP。敏感导出/备份动作复用 `useStepUp.ts` + `TotpStepUpDialog.vue`, 收到 `STEP_UP_REQUIRED` 后取得短期 grant 并重试一次。`SettingsView.vue` 暴露默认关闭的 session binding 与 step-up 开关；关闭已启用的 step-up 本身需要二次验证, 开启前当前管理员必须已配置 TOTP。API Key ACL 的兼容开关开启时还可编辑最多 16 个自定义客户端 IP header；前端做 header token 校验、大小写规范化和去重, 后端仍是最终校验边界。
- `UsersView.vue` 的 `BulkEditUserModal.vue` 调用 `/admin/users/batch-limits`, 可更新选中用户或 `all=true` 全量用户的 concurrency/RPM; 0 必须原样提交, 不能被空值归一化丢失。

## Pinia Store

`frontend/src/stores/`:

- `auth.ts`: 登录, 注册, 2FA, OAuth callback token, refresh token, pending auth session, localStorage 持久化; 角色支持 `admin` / `sub_admin` / `user`, 并提供 `isSubAdmin`、`canAccessAdmin`、`hasAdminPermission`。
- `app.ts`: 全局 UI, theme, public settings, toast, backend mode 等。
- `subscriptions.ts`: 当前用户订阅轮询。
- `announcements.ts`: 公告拉取和已读状态。
- `adminSettings.ts`: 管理端设置缓存。
- `payment.ts`: 支付流程状态。
- `onboarding.ts`: 引导流程状态。
- `imageGen.ts`: 图片生成页全部状态与生成请求(表单/原图列表/结果/历史)。路由切换卸载视图后状态不丢, 进行中的生成在 store 里跑完并经 app store toast 通知。API Key 只在内存, 禁止持久化; 历史用 localStorage key `image-gen-history-v1`。图改图支持多图(`image[]` 最多 16 张, 单张 ≤20MB——网关 multipart 分片在 20MB 处静默截断, 总量 ≤100MB)。store 内部从 `./app` 文件模块导入 app store(测试按文件模块 mock, 见 ImageGenView.spec.ts)。原图预览用 `URL.createObjectURL`, 依赖 CSP `img-src` 放行 `blob:`(security_headers.go 的 requiredCSPDirectiveValues 会给旧配置补上)。

`auth.ts` localStorage key:

- `auth_token`
- `auth_user`
- `refresh_token`
- `token_expires_at`
- `pending_auth_session`

## 组件与页面约定

目录:

- `frontend/src/views`: 页面级组件。
- `frontend/src/components`: 业务和通用组件。
- `frontend/src/composables`: 可复用组合逻辑。
- `frontend/src/utils`: 纯工具函数。
- `frontend/src/types`: TS 类型。
- `frontend/src/constants`: 常量。

提示词审计管理端:

- `/admin/prompt-audit` 懒加载 `frontend/src/features/prompt-audit/PromptAuditView.vue`, 并通过 `requiresRiskControl` 与管理端权限守卫。侧栏把既有 Risk Control 与 Prompt Audit 放在 Security Audit 分组中, 本地 `/admin/request-intercept` 仍保持独立入口。
- `frontend/src/features/prompt-audit/api.ts` 维护配置、运行态、节点 probe 和事件删除合同；`components/` 下的 policy、endpoint pool、runtime、event workspace/detail 和 filter-delete dialog 组成页面。筛选删除必须先拿 preview 的 `snapshot_max_id` / `filter_hash`, 再显式确认。
- `blocking_latest_turn_only` 只有在 Prompt Audit 和 blocking 均已启用时才可编辑；配置保存和变更摘要都必须保留该字段, 便于审计阻断范围。
- `RiskControlView.vue` 的内容审核配置和 API Key 测试共用代理语义：`undefined` 不覆盖, `0` 强制直连, 正 ID 使用指定代理；前端控件不可改变后端对这些值的最终校验。
- `frontend/src/views/admin/BackupView.vue` 同时管理备份 S3 与异步生图/图片对象存储。生图卡片可复用备份 S3 的 endpoint/region/credentials 并单独指定 bucket/prefix, 或保存独立凭据；保存即让后端运行时缓存失效、无需重启。两类 S3 保存都必须通过 `backupStepUp.run`, 用户取消 step-up 只结束保存态, 不把取消显示成网络错误。测试连接与读配置使用 `frontend/src/api/admin/backup.ts` 的 `/backups/image-storage` 合同, secret 只显示 configured 状态而不回填明文。
- `frontend/src/views/admin/SettingsView.vue` 的安全设置暴露客户端 IP 兼容开关和有序自定义 header 列表。header 只在兼容模式开启时显示/提交, 前端去重并规范化, 服务端仍负责合法 header 名和最多 16 项的最终校验。
- `SettingsView.vue` 的 Ollama Cloud 自动刷新表单同时提交 `debounce_minutes` 与 `interval_minutes`: 前者表示最后一次模型请求后的安静等待, 后者表示持续请求下的最长等待；隐藏或缺失字段不能把后端已保存值意外清空。
- `SettingsView.vue` 的 Panel API 限流表单读写 `/admin/settings/panel-rate-limit`, 暴露总开关、普通用户 RPM、重查询 RPM、公开 IP RPM 和完整管理员豁免。RPM 为 0 表示该档不限；默认值与最终校验由后端负责, 前端保存成功后回填规范化响应。
- `frontend/src/api/admin/system.ts` 为在线更新与回退单独使用 15 分钟 axios timeout；修改该调用签名时必须同步 `frontend/src/api/__tests__/admin.system.rollback.spec.ts` 的第三参数断言。
- 公告编辑页复用 `AnnouncementPopup.vue` 做预览；preview 关闭只发 `close`, 不调用已读接口。公告铃与弹窗统一加载 `frontend/src/styles/announcement-markdown.css`, Markdown/内嵌 HTML 都先经 DOMPurify 后在同一 `.markdown-body` 规则下渲染。

订阅管理:

- 管理端订阅页在 `frontend/src/views/admin/SubscriptionsView.vue`。
- 操作列的“重置配额”调用 `adminAPI.subscriptions.resetQuota(id, { daily: true, weekly: true, monthly: true })`, 会同时归零日/周/月用量。
- 操作列的“重置日限”调用 `adminAPI.subscriptions.resetQuota(id, { daily: true, weekly: false, monthly: false })`, 只归零每日用量, 不修改周/月用量。
- 管理端订阅支持撤销/恢复: revoked 订阅在列表中保留历史, 操作列显示 restore; 恢复时后端会按当前过期时间决定 active/expired。用户侧和管理侧订阅卡展示 `expires_at` 剩余时长, one-time daily quota 会使用剩余时长文案。

管理端支付看板:

- `DashboardStats` 的 `today_amount`、`total_amount`、`avg_amount`、daily series 和 payment method amount 都是 `Record<currency, amount>`, `top_users` 也是按币种分组的独立排行。`OrderStatsCards.vue`、`DailyRevenueChart.vue`、`PaymentMethodChart.vue` 和 `TopUsersLeaderboard.vue` 不得把不同 ISO 4217 币种相加；展示顺序按币种码稳定排序并用对应币种格式化。

管理端用量统计:

- `frontend/src/components/admin/usage/UsageStatsCards.vue` 总 token 卡片展示 input/output/cache 总量, cache tooltip 展示缓存创建 token 与缓存命中 token 明细; API 类型在 `frontend/src/api/admin/usage.ts` 暴露 `total_cache_creation_tokens` / `total_cache_read_tokens`。
- `frontend/src/components/admin/usage/UsageTable.vue` 的 IP 地址列可渲染 `IpGeoCell`, 并提供批量获取地区工具栏; `frontend/src/utils/ipGeoLookup.ts` 调用 geojs 单查/批量接口, 跳过内网 IP, 成功结果缓存到 localStorage `sub2api:ip-geo-cache:v1` 24 小时。用户侧 UsageView 复用同一表格事件处理。
- `UsageRequestType`、管理端/用户侧 UsageView、`UsageFilters.vue` 和 `UsageTable.vue` 均包含独立 `live` 类型及绿色 badge；它与 `sync`/`stream`/`ws_v2`/`cyber` 并列, 不应回落为 legacy stream。UsageLog 的可选 `session_id` 只用于筛查客户端会话关联。
- 管理端 UsageView 新增 `UserTokenRanking.vue`, 按筛选条件展示用户 Token 排行; `frontend/src/api/admin/dashboard.ts` 的 `UserBreakdownParams.request_type` 使用 `UsageRequestType`, 后端 `GetUserBreakdown` 通过 `ParseUsageRequestType` 解析, 不能退回普通 number 造成筛选口径漂移。用量表同时展示由 `latencyHealth.ts` 统一计算的延迟健康等级, 修改阈值或列设置时要同步 `UsageView.spec.ts`、`UserTokenRanking.spec.ts` 和 `latencyHealth.spec.ts`。
- `/admin/usage?user_id=` 初始化时会异步读取用户邮箱回显筛选标签；`UsageFilters.vue#getUserSearchRevision` 用 revision 防止迟到 lookup 覆盖用户随后输入或新的 user ID。程序化调用 `setUserKeyword` 也会推进 revision, 修改筛选器 exposed API 时要保留该竞态合同。

管理端组织用量报表:

- 完整设计见 `docs/features/organization-usage-report-design-cn.md`；趋势图见 `docs/features/organization-usage-trend-chart-design-cn.md`。
- 独立页面是 `frontend/src/views/admin/OrganizationUsageView.vue`, 路由 `/admin/organization-usage`; 月报、自然周报和最长 366 天自定义范围统一使用北京时间, 支持组织/邮箱筛选、服务端排序分页、三组织汇总、用量趋势折线和个人/团队日周月峰值。
- 前端合同在 `frontend/src/api/admin/organizationUsage.ts`（含 `getTrend`）; 页面完整加载并行 Summary+Trend 并共享 candidate `as_of`, 以 Summary canonical 为权威必要时单次对齐 Trend; 人员翻页/排序只打 Summary 且不打断趋势。正式导出会先固定候选 `as_of`, 再使用 Summary 首响应回显的 canonical `as_of` 继续后续 Summary 与日/周/月分页; `fetchAll` 不调用 trend。该值只固定用量查询上界, 不是密码学签名。
- 趋势粒度由 `inferOrganizationUsageTrendGranularity`（`organizationUsageReport.ts`）按含首尾自然日自动推断; 组件 `OrganizationUsageTrendChart.vue` 使用 Chart.js 双轴（左 Token、右 requests）, 默认系列为输入/输出/总 Token 与请求数。`total_tokens` 包含缓存创建和缓存读取两类 Token，不能视为输入与输出两条可见曲线之和。
- 人数只改前端展示名：`active_users` 显示为“注册人数”，`used_users` 显示为“活跃人数”，后端统计条件和 API 字段不变。组织内部键/API 筛选值仍为 `xunyou` / `wsdashi` / `other`，页面在 Filters、组织汇总和人员表显示为“迅游”/“速宝”/“其他”。
- Excel 构建在 `frontend/src/utils/organizationUsageReport.ts`, 固定生成“报表概览、组织汇总、人员汇总、月度明细、周度明细、日度明细”六个 Sheet。客户端四类数据合计最多 100,000 行; workbook 构建与 `XLSX.write` 在可终止的 `organizationUsageExport.worker.ts` 中执行, 页面卸载只清理任务, 不显示用户主动取消提示。
- 页面组件位于 `frontend/src/components/admin/organization-usage/`; 人员表始终保持宽表横向滚动, 不使用移动端卡片化 DataTable。修改筛选、组织汇总、峰值、趋势或导出交互时同步该目录 README、View/Worker 测试与中英文 `admin/organizationUsage.ts` locale。

管理端 Token Analysis 计费用量趋势:

- `frontend/src/views/admin/TokenAnalysisView.vue` 的用户排行可跨分页选择最多 5 名存在 `user_id` 的用户; 选择状态保存用户 ID、邮箱和选择顺序, 达到上限后只禁用未选项, 已选项仍可取消。
- 趋势面板调用 `adminAPI.dashboard.getUserUsageTrend`, 数据源是后端 `usage_logs` 计费用量而不是归档摘要。`frontend/src/api/admin/dashboard.ts` 把 `number[]` 序列化为逗号分隔 `user_ids`; 空数组不发送该参数, 避免误进入后端严格选人模式。
- 日粒度按筛选范围生成完整日期轴; 小时粒度只在同一日期可选并生成 24 个北京时间整点。每名用户按选择顺序使用稳定颜色, 缺失周期补 0, 完全无用量仍渲染零值折线并显示空用量提示。
- 用户/粒度/筛选变化先清空旧点并递增请求序号, 迟到响应不得覆盖最新选择; 错误态在面板内重试。Chart.js 只在页面内注册并复用 `vue-chartjs` 的 `Line`, 不新增全局图表组件或页面路由。
- 回归测试位于 `frontend/src/views/admin/__tests__/TokenAnalysisView.spec.ts` 与 `frontend/src/api/__tests__/admin.dashboard.spec.ts`; 修改选择上限、分页、时间轴或竞态处理时同步中英文 `admin/tokenAnalysis.ts` 文案和这两组测试。

用户侧用量页:

- `frontend/src/views/user/UsageView.vue` 使用 `frontend/src/api/usage.ts#getDashboardSnapshotV2` 拉取 trend/group 图表, `getDashboardModels({ model_source: "requested" })` 拉取请求模型分布, 过滤项与后端共享 `api_key_id`、`group_id`、`model`、`request_type`、`billing_type`、`billing_mode` 和日期范围。
- 用户用量页的列显隐持久化 key 为 `user-usage-hidden-columns`; `created_at` 始终可见, `reasoning_effort` 和 `user_agent` 默认隐藏。CSV 导出沿用当前过滤条件, 并按 `billing_mode`/图片请求修正展示计费模式。
- `GroupDistributionChart.vue` / `ModelDistributionChart.vue` 可通过 `enableBreakdown=false` 和 `showAccountCost=false` 在用户侧复用, 避免暴露管理端 account cost 或用户拆分下钻。

项目已有组件 README:

- `frontend/src/components/admin/payment/README.md`
- `frontend/src/components/admin/usage/README.md`
- `frontend/src/components/channels/README.md`
- `frontend/src/components/common/README.md`
- `frontend/src/components/layout/README.md`
- `frontend/src/components/user/monitor/README.md`
- `frontend/src/router/README.md`
- `frontend/src/stores/README.md`
- `frontend/src/views/auth/README.md`

修改 `frontend/src/components/` 下模块时, 必须同步更新该模块目录下 README; 缺失则新建。

## 账号与 Key 配置 UI

- `frontend/src/views/admin/GroupsView.vue` 只为 OpenAI 分组展示 `ReasoningEffortPolicyFields.vue`, 可设置 `minimal/low/medium/high/xhigh/max` 上限和精确 from/to 映射。表单辅助逻辑集中在 `frontend/src/views/admin/groupsReasoningEffort.ts`, 创建/编辑提交前必须拒绝空值、平台不支持值和重复 source；切换到非 OpenAI platform 时清理不再有效的策略值。
- `GroupsView.vue` 支持创建 `composite` 分组、从具体平台分组复制账号, 并通过 Routes 操作维护 exact/prefix、endpoint、target platform、upstream model、priority 和 enabled。route modal 可 CRUD 并调用 preview 展示 route/detector 来源；API 类型与调用集中在 `frontend/src/api/admin/groups.ts`。具体平台的渠道映射/定价仍按 resolved platform 配置, 不在 composite 层伪造价格。
- `frontend/src/components/account/CreateAccountModal.vue` 和 `EditAccountModal.vue` 维护 OpenAI/Grok 账号创建编辑能力。OpenAI API Key 创建保留本地 compatible provider preset、endpoint capabilities、Responses WebSocket V2 mode、Codex CLI only 和 Claude Code allowlist; Grok API Key 默认 `https://api.x.ai/v1`、占位 `xai-...`。两条分支共享同一个 API Key 容器, 修改条件或 placeholder 时要同时跑 `CreateAccountModal.grok.spec.ts` 与 `credentialsBuilder.spec.ts`。
- OpenAI OAuth 编辑可手动覆盖 `credentials.plan_type`; 仅非 Spark 影子账号生效。空选项表示恢复自动识别, 提交时删除 stale `plan_type`; Plus/Pro/Free 预设之外的 canonical 值要保留。pool mode 的 `pool_mode_retry_count` 默认 3, 前后端都规范化到 `0..10`; 开启时提交规范化值, 关闭时必须和 retry status codes 一起删除。
- `frontend/src/components/keys/UseKeyModal.vue` 生成 Codex/OpenAI/Grok 使用示例。普通 Codex 模板继续使用本地 `model_provider = "xunyou"` 与 `[model_providers.xunyou]`; WebSocket v2 模板使用 `OpenAI` provider。两种模板都支持 Legacy Login(`requires_openai_auth=true`)与 API Key Mode(`requires_openai_auth=false` + `x-openai-actor-authorization`)切换并同时生成 `auth.json`; 修改 provider 名或认证模式时必须同步两种模板和 `UseKeyModal.spec.ts`。Grok 默认页签生成 `~/.grok/config.toml` / `%userprofile%\.grok\config.toml`, provider/model key 统一为 `grok`, 使用网关 API Key、Responses backend 和 `grok-4.5`; OpenCode 使用 `@ai-sdk/openai` 与显式 Grok 模型清单。
- `frontend/src/views/user/KeysView.vue` 的列显隐设置持久化在 localStorage: `api-key-hidden-columns` 与 `api-key-column-settings-version`; `name` 和 `actions` 始终可见。Key 列表支持按当前并发排序并展示 last used IP。编辑 quota exhausted / expired key 时, 只有用户明确改回 active 才提交 `status`, 防止无限额度 key 被误保持耗尽态。
- `frontend/src/views/admin/AccountsView.vue` 支持从 OpenAI OAuth 母账号创建 Spark 影子账号; 影子账号导出时会被排除, 后端返回 `skipped_shadows` 后前端提示。账号 action menu 的 create spark shadow 只应用于可作为母账号的 OpenAI OAuth 账号。
- 账号 action menu 对可复制的静态凭据账号提供一键复制, 调用 `POST /admin/accounts/:id/duplicate`; API client 为同一账号复用 session-scoped `Idempotency-Key`, 只有成功后才清理 key, 让网络重试可恢复同一个副本。复制成功后刷新列表; shadow 与旋转凭据账号不显示该入口。
- `AccountsView.vue` 的 `scheduler_score` 默认隐藏; 前端只在列可见时传 `include_scheduler_score=1`, 避免账号列表默认触发高成本调度分计算。
- `AccountsView.vue` 为 OpenAI API Key 账号展示 `UpstreamBillingRateCell.vue`, 显示账号自动探测状态、全局开关、最近倍率/时间、下次 probe 和 stale 状态, 并支持单个/批量探测；全局周期设置位于 `SettingsView.vue`。`CreateAccountModal.vue` 默认开启新账号自动探测并等待首次 probe, 但继续保留本地 OpenAI-compatible provider preset/base URL/endpoint capability 构建。账号名称仅在上游 URL 可安全解析时提供外链。Stripe 支付 SDK 改为在支付组件内动态 import, 不得重新放回首屏 vendor bundle。
- Ollama Cloud eligible 账号在 `AccountUsageCell.vue` / `OllamaCloudUsageCell.vue` 展示官方 5 小时、7 天、余额和模型窗口；`OllamaCloudUsageSettings.vue` 负责保存/删除 web session、账号级自动刷新和手工刷新。全局开关与 15-1440 分钟间隔位于 `SettingsView.vue`, 默认关闭；UI 不回显 session 明文。
- `PaymentStatusPanel.vue` 在 Alipay precreate 返回 QR/URL 时通过 `alipayDeepLink.ts` 生成支付宝 App/Universal Link, 移动端可直接拉起并保留二维码回退；只允许 `alipay:` 和受信 `https://qr.alipay.com` / `https://render.alipay.com/p/s/i` 目标, 其他值不生成深链。
- `DataTable.vue` 默认仅在桌面行数大于 `virtualizeThreshold`(默认 100)时启用虚拟化, 小列表全量渲染以避免可变行高滚动补偿抖动; 虚拟行高缓存用 `rowKey` 而不是 index。分页/筛选换成不同 row identity 集合时必须清理旧 element/size cache, 仅同一组稳定 row key 或同一批对象的纯重排可复用缓存; duplicate/缺失 key 要保守失效。账号表显式使用阈值 50。修改虚拟化时要保持 mobile 非虚拟化、stable sort 和 exposed virtualizer/swipe selection 合同。
- `Select.vue` 的 teleported dropdown 必须保留 8px viewport padding, `left`/`minWidth`/`maxWidth` 都按当前视口可用宽度收敛；窄屏不能因 200px 首选最小宽度溢出。`GroupOptionItem.vue` 的 description 保留换行、允许任意长词断行并最多显示 3 行。
- `AvailableChannelsTable.vue` 在 `lg` 及以上使用按渠道/platform 分组的 table, 较小视口改为无横向滚动的 channel section；分组 badge、峰值倍率和模型 chip 都必须 `min-w-0`/wrap。`MonitorTimeline.vue` 的 60 个 bar 使用 `flex-1 min-w-0`, 避免窄监控卡被固定最小宽度撑破。
- 用户 Key 列表和管理端 Group 列表可选显示 ID 列, 默认隐藏并沿用各自列设置持久化；新增/调整列时不能覆盖用户现有显隐选择。

## 管理端用户筛选

- `/admin/users` 页面在 `frontend/src/views/admin/UsersView.vue`; 后端 `GET /api/v1/admin/users` 支持 `group_name` 按用户授权分组名模糊过滤, 也支持 `api_key_group_id` 按用户实际拥有的未软删除 API Key 绑定分组精确过滤。
- API Key 分组筛选选项由 `frontend/src/views/admin/apiKeyGroupFilterOptions.ts` 构建, 会包含停用分组, 便于排查仍绑定到停用分组的 key; 分节 header 使用负数 sentinel, 不要改成 `null` 以免 Select key 冲突。
- 管理端创建/编辑用户支持 `user` / `sub_admin` / `admin`; 子管理员权限清单从 `GET /api/v1/admin/permissions/catalog` 获取, 不在弹窗内硬编码。后端服务禁止误删或降级最后一名管理员。角色字段或文案变化要同步 User DTO、两个用户弹窗、列表筛选和角色测试。
- 注册页在强制邀请码关闭但 affiliate 开启时显示可选 `aff_code` 输入；URL/localStorage referral 仍由 `syncAffiliateReferralCode` 预填。强制邀请码开启时只显示原校验输入, 不能同时渲染两个邀请码字段。

## 子管理员菜单与页面

- 权限路由顺序和 backend landing 的最小映射在 `frontend/src/utils/adminPermissions.ts`; 权限目录本身由后端 API 返回。
- `AppSidebar.vue` 为子管理员增加“管理功能”分区, 只显示已授权的订阅管理、使用记录、Token 分析; 账号、风控、请求拦截、设置和管理员自定义菜单不显示。
- `SubscriptionsView.vue` 对子管理员只显示全量配额重置和仅日限重置; 分配、延期、撤销、恢复及其弹窗仅完整管理员可见。
- `UsageView.vue` 对子管理员隐藏清理和用户余额详情入口, 保留查询、统计、排行、错误详情和导出; `UsageFilters.vue` 只调用 usage compact 账号/分组筛选接口。
- `TokenAnalysisView.vue` 对子管理员隐藏“立即索引”, 保留只读统计、项目、请求输入和索引状态。
- API client 收到 `ADMIN_PERMISSION_DENIED` 会触发用户信息刷新。标准模式回 `/dashboard`; backend 模式无剩余权限时先 logout 再回 `/login`。

## 渠道监控、注册与模型审计 UI

- `/monitor` 仍由 `ChannelStatusView.vue` 承载, 它通过 `utils/featureFlags.ts` 读取 public settings 的 `channel_monitor_enabled` 和 `channel_monitor_mode`。只有功能开启且 mode 为 `v2` 时渲染 `ChannelStatusV2View.vue`; 缺失、非法或 `v1` 都回落 `ChannelStatusV1View.vue`。
- 管理端 `/admin/channels/monitor` 在 `ChannelMonitorView.vue` 同时保留 V2 配置和 legacy V1 页签; 当前 mode 决定默认页签, 但管理员可在 V1 正在运行时先用 `features/channel-monitor-v2/MonitorSettingsPanel.vue` 预配 V2。V2 API client 集中在 `frontend/src/api/channelMonitorV2.ts`, admin 走 `/admin/channel-monitor-v2`, user 走 `/channel-monitor-v2`。
- `ChannelStatusV2View.vue` 不得通过前端重组恢复后端已脱敏的绝对请求/Token/样本/attempt 数。普通用户遵循 `channel_monitor_hide_throughput`, 管理员始终可见 RPM/TPM; 用户排行中只有本人可展示 identity 和 drilldown, 其他行必须使用匿名 label。
- `SettingsView.vue` 的 `channel_monitor_mode`、`channel_monitor_hide_throughput`、`registration_email_domain_quota_enabled` 和 `grok_cross_client_model_map_enabled` 都是 GET→form→PUT 的保真字段。后端 Grok 跨客户端映射默认 true, 而前端本地 form 初值可为 false; 必须等 GET 值覆盖初值后再保存, 并用 settings round-trip 测试防止与本次编辑无关的 true 被静默写成 false。
- `UsageView.vue` 和 CSV 导出同时展示 requested/model、`upstream_model`、`upstream_response_model` 及 mismatch。`upstream_model_mismatch` 筛选是 true/false/不筛选三态; 记录值为 `null` 时展示空白/未观测, 不得归入 false 的“一致”集合。Dashboard trend/models/groups 请求要传递同一筛选值。
- 注册和待完成 OAuth 邮箱页从 public settings 读取 `registration_email_suffix_whitelist` 与 `registration_email_domain_quota_enabled`。额度开启时前端可放行非白名单邮箱提交给后端做权威计数; 后端返回 `EMAIL_DOMAIN_REGISTRATION_LIMIT` 时统一映射为主域额度文案, 不在浏览器端猜测当前账户数。

## Grok 与 Codex 管理端 UI

- Grok 平台已加入前端 platform 类型: `frontend/src/types/index.ts`, `frontend/src/api/admin/settings.ts`, `frontend/src/api/admin/users.ts`, `frontend/src/utils/platformColors.ts`, `PlatformIcon.vue`, `PlatformTypeBadge.vue`。
- Grok OAuth 管理 API 在 `frontend/src/api/admin/grok.ts`, 组合逻辑在 `frontend/src/composables/useGrokOAuth.ts`; `CreateAccountModal.vue` 和 `ReAuthAccountModal.vue` 复用 OAuth 授权流, 支持授权码、refresh token 校验和 OAuth credentials 构建。
- 邮箱密码授权入口必须先请求 `/admin/grok/oauth/capabilities`; 只在 `password_auth_enabled=true` 时显示, capability 请求失败时 fail-closed。前端只把 `email----password` 传给当次授权 API, `buildCredentials` 不得保存密码或 raw SSO; 二次认证只可预填邮箱, 密码由管理员重新输入。
- `EditAccountModal.vue` 只为 Grok OAuth 账号显示“客户端工具缓存”开关, 持久化到 `extra.grok_client_tool_cache_enabled`; 缺失值在表单中显示为开启并在保存时显式落布尔值。后端只有已确认 Free OAuth 的缺失值默认开启, paid/API Key/unknown 保持 fail-closed；其他账号不显示也不提交该开关。
- Grok 账号配额展示在 `AccountUsageCell.vue`; `GrokQuotaProbeCell.vue` 提供主动 probe, 只对 `platform === "grok" && type === "oauth"` 显示。xAI 不支持 reset 时前端显示 reset unsupported。账号测试 modal 使用 `buildApiUrl` 请求 `/admin/accounts/:id/test`, Grok OAuth 测试会走 xAI Responses 流。
- `useModelWhitelist.ts` 为 Grok 维护模型候选和常用映射 preset; 修改 Grok 模型名时要同步白名单 selector、平台颜色和 i18n 文案。
- Grok media 已接入 images/videos 路由后, 前端平台图标、颜色、Grok quota unknown/reset unsupported 文案要与后端 `allow_image_generation` gate 保持一致; 旧 Grok group 由后端 migration 自动回填图片能力。
- `GroupsView.vue` 的 Grok media 定价把图片与视频控制分离: 视频支持独立倍率以及 480p/720p/1080p 每秒单价, 表单归一化集中在 `groupsImagePricing.ts`; 不要把视频价格回填为图片价格。
- `GroupsView.vue` 还维护 Grok 模型族视频价 `video_model_prices`、搜索价 `search_price_per_1k` 和 Voice 价 `audio_realtime_price_per_min` / `audio_tts_price_per_million_chars` / `audio_stt_price_per_hour`。这些可空字段必须区分 NULL（代码默认价）、0（显式免费）和正数（分组覆盖价）; 创建、编辑、复制和 API 类型需同步。
- OpenAI `codex_cli_only` 管理端新增全局 engine fingerprint signals 与 app-server 开关, 设置页入口在 `SettingsView.vue` + `codexFingerprintSignals.ts`; 账号创建/编辑/批量编辑里有账号级 `codex_cli_only_allow_app_server` 开关。
- OpenAI Fast/Flex policy 规则支持 `user_ids`; 设置页使用 `OpenAIFastPolicyUserSelector.vue` 按邮箱/ID 搜索并保留已删除用户的可识别标签, API 类型在 `frontend/src/api/admin/settings.ts`。新增文案必须同时补 `locales/en/admin/settings.ts` 与 `locales/zh/admin/settings.ts`, 并通过 `openaiFastPolicyLocales.spec.ts`。
- `VersionBadge.vue` 展示当前版本及最近 3 个历史版本, 管理员可通过 `frontend/src/api/admin/system.ts` 查询/触发在线回退; 回退按钮必须保留确认、运行状态和失败提示, 不能只改前端版本文本。
- Dashboard、Group/Model distribution 图表使用 `toFiniteNumber` 兜底, 避免后端返回字符串、null 或 NaN 时污染图表排序和格式化。`DataTable` sortable 表头使用双三角指示和 `aria-sort`, 修改排序 UI 时要保持可访问性语义。
- 日期筛选默认值必须通过 `frontend/src/utils/format.ts#formatDateLocalInput` 以本地年月日生成 `YYYY-MM-DD`, 不使用 UTC `toISOString().slice(0, 10)`; 用户 Dashboard 与 KeyUsageView 共用该 helper, 避免 UTC 正偏移时日期范围少一天。

## 测试与质量

`frontend/package.json`:

- `pnpm run build`: `vue-tsc -b && vite build`
- `pnpm run lint:check`: ESLint 检查。
- `pnpm run typecheck`: `vue-tsc --noEmit`
- `pnpm run test:run`: Vitest。
- locale message compile 测试直接使用 `@intlify/message-compiler@9.14.5`; 该依赖已在 `devDependencies` 和 `pnpm-lock.yaml` 显式声明。

`frontend/vitest.config.ts`:

- jsdom 环境。
- setup file: `src/__tests__/setup.ts`。
- include: `src/**/*.{test,spec}.{js,ts,jsx,tsx}`。
- 覆盖率阈值全局 80%。

根 Makefile 的 `test-frontend` 会运行 lint, typecheck 和 critical vitest 列表。

## 相关页面

- [[README]]
- [[backend]]
- [[frontend]]
- [[ops]]
- [[data-and-domain]]
- [[security-and-reliability]]
- [[ai-workflow]]
