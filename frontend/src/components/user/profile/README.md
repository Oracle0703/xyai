# User Profile Components

## 0.2.6 合并增量

- `TotpSetupModal.vue` 的六个输入显式绑定 `code[index]`，验证失败重置状态时同步清空 DOM；设置/禁用流程统一用 `extractApiErrorMessage` 展示规范化错误。验证为 `Totp.errors.spec.ts` 与 `TotpSetupModal.inputs.spec.ts`。

本目录维护用户资料、头像、密码、身份绑定、TOTP、余额通知和 Passkey 管理卡片。页面级公开设置由 `ProfileView.vue` 获取后通过 props 传入；组件不应把设置加载失败误判为功能开启。

## Passkey

`ProfilePasskeyCard.vue` 仅在 `enabled=true` 且浏览器支持 WebAuthn 时允许操作。它通过 `passkeyAPI` 列出、注册、重命名和删除凭据；注册与删除必须提交当前账号密码，凭据 ceremony 的 session token 由 API client 传递，不写入全局 auth store。

开关关闭时不得请求凭据列表，也不应弹出加载失败 toast。Passkey 登录仍由 `LoginView.vue` 和 `authStore.loginWithPasskey()` 负责，不在 Profile 组件中建立会话。

## 验证

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/api/__tests__/passkey.spec.ts
cmd.exe /c pnpm --dir frontend run typecheck
```

# 0.2.5 资料表单

- `ProfileEditForm.vue`、`ProfilePasswordForm.vue` 和余额提醒组件的表单值仍由用户资料 API 确认；更新编辑/密码/通知交互时同步同目录 `__tests__`，不让未保存的表单状态改写服务端用户资料。
