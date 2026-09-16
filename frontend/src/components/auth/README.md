# 认证组件

- `EmailOAuthButtons.vue` 与登录/绑定场景共享服务端 OAuth 可用性；公开设置未知或加载失败时不要把 OAuth 按钮误当已授权入口。
- 变更按钮/回调状态时同步本目录 `__tests__` 与认证路由，不在组件存储第三方 token。
