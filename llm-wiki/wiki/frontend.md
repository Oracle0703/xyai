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

主入口:

- `frontend/src/main.ts`: 初始化主题, Pinia, 注入配置, i18n, router, mount。
- `frontend/src/App.vue`: 全局导航进度, RouterView, Toast, AnnouncementPopup, setup 检查, 公共设置加载。

## Vite 构建行为

`frontend/vite.config.ts`:

- `@` 指向 `frontend/src`。
- `vue-i18n` alias 到 runtime 版本, 避免 CSP unsafe-eval。
- dev server 默认端口来自 `VITE_DEV_PORT` 或 `3000`。
- dev proxy 转发 `/api`, `/v1`, `/setup` 到 `VITE_DEV_PROXY_TARGET` 或 `http://localhost:8080`。
- build 输出到 `../backend/internal/web/dist`, 供后端嵌入。
- dev 模式会尝试从后端 `/api/v1/settings/public` 注入 `window.__APP_CONFIG__`, 模拟生产 HTML 注入行为。

## 路由与守卫

`frontend/src/router/index.ts` 集中定义路由。

主要分组:

- setup: `/setup`
- public: `/home`, `/login`, `/register`, OAuth callback, `/key-usage`, `/image-gen`, `/legal/:documentId`
- batch image: `/batch-image`(alias `/docs/batch-image`) 使用 `BatchImageGuideView.vue`, 侧栏入口由 `useBatchImageAccess` 按用户/分组能力刷新显示。
- user: `/dashboard`, `/keys`, `/usage`, `/redeem`, `/affiliate`, `/available-channels`, `/profile`, `/subscriptions`, `/purchase`, `/orders`, payment 页面, `/custom/:id`
- admin: `/admin/dashboard`, `/admin/ops`, `/admin/users`, `/admin/groups`, `/admin/channels/*`, `/admin/accounts`, `/admin/settings`, `/admin/risk-control`, `/admin/request-intercept`, `/admin/usage`, `/admin/organization-usage`, `/admin/token-analysis`, payment admin, affiliate admin

守卫要点:

- 首次导航调用 `authStore.checkAuth()` 恢复 localStorage 会话。
- `requiresAuth` 默认 true, 显式 false 才是公开页。
- `requiresAdmin` 检查管理员角色。
- `requiresPayment` 依赖 public settings 中的 payment 开关。
- `requiresRiskControl` 依赖 risk control 开关。
- feature route guard 会先等待 `appStore.fetchPublicSettings()`; 只有 settings 已成功加载且开关显式为 `false` 才重定向。加载失败属于未知状态, 不能误判为功能关闭。`app.ts` 用单一 in-flight promise 合并并发 public-settings 请求, force refresh 也不能让旧请求覆盖新结果。
- simple mode 会限制部分 SaaS 页面。
- backend mode 下非管理员只能访问白名单公开路径。
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
- Channel Monitor 支持 Grok provider、模板和筛选; `GrokQuotaProbeCell.vue` 的 Free 配额显示按本地滚动 24 小时 Token 用量估算, 与上游 weekly header 分开展示。

## Pinia Store

`frontend/src/stores/`:

- `auth.ts`: 登录, 注册, 2FA, OAuth callback token, refresh token, pending auth session, localStorage 持久化。
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

订阅管理:

- 管理端订阅页在 `frontend/src/views/admin/SubscriptionsView.vue`。
- 操作列的“重置配额”调用 `adminAPI.subscriptions.resetQuota(id, { daily: true, weekly: true, monthly: true })`, 会同时归零日/周/月用量。
- 操作列的“重置日限”调用 `adminAPI.subscriptions.resetQuota(id, { daily: true, weekly: false, monthly: false })`, 只归零每日用量, 不修改周/月用量。
- 管理端订阅支持撤销/恢复: revoked 订阅在列表中保留历史, 操作列显示 restore; 恢复时后端会按当前过期时间决定 active/expired。用户侧和管理侧订阅卡展示 `expires_at` 剩余时长, one-time daily quota 会使用剩余时长文案。

管理端用量统计:

- `frontend/src/components/admin/usage/UsageStatsCards.vue` 总 token 卡片展示 input/output/cache 总量, cache tooltip 展示缓存创建 token 与缓存命中 token 明细; API 类型在 `frontend/src/api/admin/usage.ts` 暴露 `total_cache_creation_tokens` / `total_cache_read_tokens`。
- `frontend/src/components/admin/usage/UsageTable.vue` 的 IP 地址列可渲染 `IpGeoCell`, 并提供批量获取地区工具栏; `frontend/src/utils/ipGeoLookup.ts` 调用 geojs 单查/批量接口, 跳过内网 IP, 成功结果缓存到 localStorage `sub2api:ip-geo-cache:v1` 24 小时。用户侧 UsageView 复用同一表格事件处理。
- 管理端 UsageView 新增 `UserTokenRanking.vue`, 按筛选条件展示用户 Token 排行; `frontend/src/api/admin/dashboard.ts` 的 `UserBreakdownParams.request_type` 使用 `UsageRequestType`, 后端 `GetUserBreakdown` 通过 `ParseUsageRequestType` 解析, 不能退回普通 number 造成筛选口径漂移。用量表同时展示由 `latencyHealth.ts` 统一计算的延迟健康等级, 修改阈值或列设置时要同步 `UsageView.spec.ts`、`UserTokenRanking.spec.ts` 和 `latencyHealth.spec.ts`。

管理端组织用量报表:

- 完整设计见 `docs/features/organization-usage-report-design-cn.md`。
- 独立页面是 `frontend/src/views/admin/OrganizationUsageView.vue`, 路由 `/admin/organization-usage`; 月报、自然周报和最长 366 天自定义范围统一使用北京时间, 支持组织/邮箱筛选、服务端排序分页、三组织汇总和个人/团队日周月峰值。
- 前端合同在 `frontend/src/api/admin/organizationUsage.ts`; 正式导出会先固定候选 `as_of`, 再使用 Summary 首响应回显的 canonical `as_of` 继续后续 Summary 与日/周/月分页, 避免导出期间新增 usage 导致 offset 漂移。该值只固定用量查询上界, 不是密码学签名。
- Excel 构建在 `frontend/src/utils/organizationUsageReport.ts`, 固定生成“报表概览、组织汇总、人员汇总、月度明细、周度明细、日度明细”六个 Sheet。客户端四类数据合计最多 100,000 行; workbook 构建与 `XLSX.write` 在可终止的 `organizationUsageExport.worker.ts` 中执行, 页面卸载只清理任务, 不显示用户主动取消提示。
- 页面组件位于 `frontend/src/components/admin/organization-usage/`; 人员表始终保持宽表横向滚动, 不使用移动端卡片化 DataTable。修改筛选、组织汇总、峰值或导出交互时同步该目录 README、View/Worker 测试与中英文 `admin/organizationUsage.ts` locale。

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

- `frontend/src/components/common/README.md`
- `frontend/src/components/layout/README.md`
- `frontend/src/router/README.md`
- `frontend/src/stores/README.md`
- `frontend/src/views/auth/README.md`

修改 `frontend/src/components/` 下模块时, 必须同步更新该模块目录下 README; 缺失则新建。

## 账号与 Key 配置 UI

- `frontend/src/components/account/CreateAccountModal.vue` 和 `EditAccountModal.vue` 维护 OpenAI/Grok 账号创建编辑能力。OpenAI API Key 创建保留本地 compatible provider preset、endpoint capabilities、Responses WebSocket V2 mode、Codex CLI only 和 Claude Code allowlist; Grok API Key 默认 `https://api.x.ai/v1`、占位 `xai-...`。两条分支共享同一个 API Key 容器, 修改条件或 placeholder 时要同时跑 `CreateAccountModal.grok.spec.ts` 与 `credentialsBuilder.spec.ts`。
- OpenAI OAuth 编辑可手动覆盖 `credentials.plan_type`; 仅非 Spark 影子账号生效。空选项表示恢复自动识别, 提交时删除 stale `plan_type`; Plus/Pro/Free 预设之外的 canonical 值要保留。pool mode 的 `pool_mode_retry_count` 默认 3, 前后端都规范化到 `0..10`; 开启时提交规范化值, 关闭时必须和 retry status codes 一起删除。
- `frontend/src/components/keys/UseKeyModal.vue` 生成 Codex/OpenAI/Grok 使用示例。普通 Codex 模板继续使用本地 `model_provider = "xunyou"` 与 `[model_providers.xunyou]`; WebSocket v2 模板使用 `OpenAI` provider。两种模板都支持 Legacy Login(`requires_openai_auth=true`)与 API Key Mode(`requires_openai_auth=false` + `x-openai-actor-authorization`)切换并同时生成 `auth.json`; 修改 provider 名或认证模式时必须同步两种模板和 `UseKeyModal.spec.ts`。Grok 默认页签生成 `~/.grok/config.toml` / `%userprofile%\.grok\config.toml`, provider/model key 统一为 `grok`, 使用网关 API Key、Responses backend 和 `grok-4.5`; OpenCode 使用 `@ai-sdk/openai` 与显式 Grok 模型清单。
- `frontend/src/views/user/KeysView.vue` 的列显隐设置持久化在 localStorage: `api-key-hidden-columns` 与 `api-key-column-settings-version`; `name` 和 `actions` 始终可见。Key 列表支持按当前并发排序并展示 last used IP。编辑 quota exhausted / expired key 时, 只有用户明确改回 active 才提交 `status`, 防止无限额度 key 被误保持耗尽态。
- `frontend/src/views/admin/AccountsView.vue` 支持从 OpenAI OAuth 母账号创建 Spark 影子账号; 影子账号导出时会被排除, 后端返回 `skipped_shadows` 后前端提示。账号 action menu 的 create spark shadow 只应用于可作为母账号的 OpenAI OAuth 账号。
- 账号 action menu 对可复制的静态凭据账号提供一键复制, 调用 `POST /admin/accounts/:id/duplicate`; API client 为同一账号复用 session-scoped `Idempotency-Key`, 只有成功后才清理 key, 让网络重试可恢复同一个副本。复制成功后刷新列表; shadow 与旋转凭据账号不显示该入口。
- `AccountsView.vue` 的 `scheduler_score` 默认隐藏; 前端只在列可见时传 `include_scheduler_score=1`, 避免账号列表默认触发高成本调度分计算。
- `DataTable.vue` 默认仅在桌面行数大于 `virtualizeThreshold`(默认 100)时启用虚拟化, 小列表全量渲染以避免可变行高滚动补偿抖动; 虚拟行高缓存用 `rowKey` 而不是 index。分页/筛选换成不同 row identity 集合时必须清理旧 element/size cache, 仅同一组稳定 row key 或同一批对象的纯重排可复用缓存; duplicate/缺失 key 要保守失效。账号表显式使用阈值 50。修改虚拟化时要保持 mobile 非虚拟化、stable sort 和 exposed virtualizer/swipe selection 合同。
- 用户 Key 列表和管理端 Group 列表可选显示 ID 列, 默认隐藏并沿用各自列设置持久化；新增/调整列时不能覆盖用户现有显隐选择。

## 管理端用户筛选

- `/admin/users` 页面在 `frontend/src/views/admin/UsersView.vue`; 后端 `GET /api/v1/admin/users` 支持 `group_name` 按用户授权分组名模糊过滤, 也支持 `api_key_group_id` 按用户实际拥有的未软删除 API Key 绑定分组精确过滤。
- API Key 分组筛选选项由 `frontend/src/views/admin/apiKeyGroupFilterOptions.ts` 构建, 会包含停用分组, 便于排查仍绑定到停用分组的 key; 分节 header 使用负数 sentinel, 不要改成 `null` 以免 Select key 冲突。
- 管理端创建/编辑用户支持显式选择 `user` / `admin` 角色; 后端服务禁止误删最后一名管理员。角色字段或文案变化要同步 User DTO、`UserCreateModal.vue`、`UserEditModal.vue` 和 `admin_service_role_test.go`。

## Grok 与 Codex 管理端 UI

- Grok 平台已加入前端 platform 类型: `frontend/src/types/index.ts`, `frontend/src/api/admin/settings.ts`, `frontend/src/api/admin/users.ts`, `frontend/src/utils/platformColors.ts`, `PlatformIcon.vue`, `PlatformTypeBadge.vue`。
- Grok OAuth 管理 API 在 `frontend/src/api/admin/grok.ts`, 组合逻辑在 `frontend/src/composables/useGrokOAuth.ts`; `CreateAccountModal.vue` 和 `ReAuthAccountModal.vue` 复用 OAuth 授权流, 支持授权码、refresh token 校验和 OAuth credentials 构建。
- Grok 账号配额展示在 `AccountUsageCell.vue`; `GrokQuotaProbeCell.vue` 提供主动 probe, 只对 `platform === "grok" && type === "oauth"` 显示。xAI 不支持 reset 时前端显示 reset unsupported。账号测试 modal 使用 `buildApiUrl` 请求 `/admin/accounts/:id/test`, Grok OAuth 测试会走 xAI Responses 流。
- `useModelWhitelist.ts` 为 Grok 维护模型候选和常用映射 preset; 修改 Grok 模型名时要同步白名单 selector、平台颜色和 i18n 文案。
- Grok media 已接入 images/videos 路由后, 前端平台图标、颜色、Grok quota unknown/reset unsupported 文案要与后端 `allow_image_generation` gate 保持一致; 旧 Grok group 由后端 migration 自动回填图片能力。
- `GroupsView.vue` 的 Grok media 定价把图片与视频控制分离: 视频支持独立倍率以及 480p/720p/1080p 每秒单价, 表单归一化集中在 `groupsImagePricing.ts`; 不要把视频价格回填为图片价格。
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

`frontend/vitest.config.ts`:

- jsdom 环境。
- setup file: `src/__tests__/setup.ts`。
- include: `src/**/*.{test,spec}.{js,ts,jsx,tsx}`。
- 覆盖率阈值全局 80%。

根 Makefile 的 `test-frontend` 会运行 lint, typecheck 和 critical vitest 列表。
