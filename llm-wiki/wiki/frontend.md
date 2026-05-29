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
