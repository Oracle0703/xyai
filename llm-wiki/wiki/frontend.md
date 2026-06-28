# 前端知识基线

## 技术栈与入口

- Vue 3 + Composition API + TypeScript。
- 构建: Vite 5。
- 状态管理: Pinia。
- 路由: Vue Router 4。
- 样式: TailwindCSS, 全局样式在 `frontend/src/style.css` 和 `frontend/src/styles/`。
- 国际化: `vue-i18n`, 入口 `frontend/src/i18n/index.ts`。
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
- user: `/dashboard`, `/keys`, `/usage`, `/redeem`, `/affiliate`, `/available-channels`, `/profile`, `/subscriptions`, `/purchase`, `/orders`, payment 页面, `/custom/:id`
- admin: `/admin/dashboard`, `/admin/ops`, `/admin/users`, `/admin/groups`, `/admin/channels/*`, `/admin/accounts`, `/admin/settings`, `/admin/risk-control`, `/admin/request-intercept`, `/admin/token-analysis`, payment admin, affiliate admin

守卫要点:

- 首次导航调用 `authStore.checkAuth()` 恢复 localStorage 会话。
- `requiresAuth` 默认 true, 显式 false 才是公开页。
- `requiresAdmin` 检查管理员角色。
- `requiresPayment` 依赖 public settings 中的 payment 开关。
- `requiresRiskControl` 依赖 risk control 开关。
- simple mode 会限制部分 SaaS 页面。
- backend mode 下非管理员只能访问白名单公开路径。
- chunk load error 会触发一次页面 reload。
- 页面标题由 `frontend/src/router/title.ts` 的 `resolveRouteDocumentTitle` 统一生成; `CustomPage` 会优先使用公开自定义菜单项或管理员自定义菜单项 label, 语言切换、站点名变化、自定义菜单加载后都会重新解析标题。

## API Client

`frontend/src/api/client.ts`:

- Axios baseURL: `VITE_API_BASE_URL` 或 `/api/v1`。
- 默认 `withCredentials: true`, timeout 30s。
- 请求拦截:
  - 从 localStorage 读取 `auth_token` 写入 Authorization。
  - 写入 `Accept-Language`。
  - GET 请求追加 `timezone`。
- 响应拦截:
  - 自动解包 `{ code, message, data }`。
  - 401 时使用 `refresh_token` 调 `/auth/refresh`, 并重试原请求。
  - refresh 失败会清理 localStorage 并跳转 `/login`。
  - ops disabled 的 404 会写缓存并跳转设置页。

API 模块分布:

- 用户侧: `frontend/src/api/auth.ts`, `keys.ts`, `usage.ts`, `user.ts`, `redeem.ts`, `payment.ts`, `groups.ts`, `channels.ts`, `totp.ts`, `channelMonitor.ts`。
- 管理侧: `frontend/src/api/admin/**`。
- 统一导出: `frontend/src/api/index.ts` 和 `frontend/src/api/admin/index.ts`。

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

管理端用量统计:

- `frontend/src/components/admin/usage/UsageStatsCards.vue` 总 token 卡片展示 input/output/cache 总量, cache tooltip 展示缓存创建 token 与缓存命中 token 明细; API 类型在 `frontend/src/api/admin/usage.ts` 暴露 `total_cache_creation_tokens` / `total_cache_read_tokens`。

项目已有组件 README:

- `frontend/src/components/common/README.md`
- `frontend/src/components/layout/README.md`
- `frontend/src/router/README.md`
- `frontend/src/stores/README.md`
- `frontend/src/views/auth/README.md`

修改 `frontend/src/components/` 下模块时, 必须同步更新该模块目录下 README; 缺失则新建。

## 账号与 Key 配置 UI

- `frontend/src/components/account/CreateAccountModal.vue` 和 `EditAccountModal.vue` 维护 OpenAI 账号创建/编辑能力, 包括 OpenAI-compatible provider preset, endpoint capabilities, Responses WebSocket V2 mode, Codex CLI only 和 Claude Code allowlist。
- `frontend/src/components/keys/UseKeyModal.vue` 生成 Codex/OpenAI 使用示例。本地 Codex 模板使用 `model_provider = "xunyou"` 与 `[model_providers.xunyou]` 配套, 修改 provider 名时必须同步配置段名称。

## 管理端用户筛选

- `/admin/users` 页面在 `frontend/src/views/admin/UsersView.vue`; 后端 `GET /api/v1/admin/users` 支持 `group_name` 按用户授权分组名模糊过滤, 也支持 `api_key_group_id` 按用户实际拥有的未软删除 API Key 绑定分组精确过滤。
- API Key 分组筛选选项由 `frontend/src/views/admin/apiKeyGroupFilterOptions.ts` 构建, 会包含停用分组, 便于排查仍绑定到停用分组的 key; 分节 header 使用负数 sentinel, 不要改成 `null` 以免 Select key 冲突。

## Grok 与 Codex 管理端 UI

- Grok 平台已加入前端 platform 类型: `frontend/src/types/index.ts`, `frontend/src/api/admin/settings.ts`, `frontend/src/api/admin/users.ts`, `frontend/src/utils/platformColors.ts`, `PlatformIcon.vue`, `PlatformTypeBadge.vue`。
- Grok OAuth 管理 API 在 `frontend/src/api/admin/grok.ts`, 组合逻辑在 `frontend/src/composables/useGrokOAuth.ts`; `CreateAccountModal.vue` 和 `ReAuthAccountModal.vue` 复用 OAuth 授权流, 支持授权码、refresh token 校验和 OAuth credentials 构建。
- Grok 账号配额展示在 `AccountUsageCell.vue`; `GrokQuotaProbeCell.vue` 提供主动 probe, 只对 `platform === "grok" && type === "oauth"` 显示。xAI 不支持 reset 时前端显示 reset unsupported。
- `useModelWhitelist.ts` 为 Grok 维护模型候选和常用映射 preset; 修改 Grok 模型名时要同步白名单 selector、平台颜色和 i18n 文案。
- OpenAI `codex_cli_only` 管理端新增全局 engine fingerprint signals 与 app-server 开关, 设置页入口在 `SettingsView.vue` + `codexFingerprintSignals.ts`; 账号创建/编辑/批量编辑里有账号级 `codex_cli_only_allow_app_server` 开关。
- Dashboard、Group/Model distribution 图表使用 `toFiniteNumber` 兜底, 避免后端返回字符串、null 或 NaN 时污染图表排序和格式化。

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
