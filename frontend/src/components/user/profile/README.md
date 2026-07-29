# User Profile Components

本目录维护用户资料、头像、密码、身份绑定、TOTP、余额通知和 Passkey 管理卡片。页面级公开设置由 `ProfileView.vue` 获取后通过 props 传入；组件不应把设置加载失败误判为功能开启。

## Passkey

`ProfilePasskeyCard.vue` 仅在 `enabled=true` 且浏览器支持 WebAuthn 时允许操作。它通过 `passkeyAPI` 列出、注册、重命名和删除凭据；注册与删除必须提交当前账号密码，凭据 ceremony 的 session token 由 API client 传递，不写入全局 auth store。

开关关闭时不得请求凭据列表，也不应弹出加载失败 toast。Passkey 登录仍由 `LoginView.vue` 和 `authStore.loginWithPasskey()` 负责，不在 Profile 组件中建立会话。

## 验证

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/api/__tests__/passkey.spec.ts
cmd.exe /c pnpm --dir frontend run typecheck
```
