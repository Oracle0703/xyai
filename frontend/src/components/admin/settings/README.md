# 管理设置组件

- `AdminAccessKeysSettings.vue`：受限管理员 API 密钥的操作/资源策略、有效期、IP 限制、创建、轮换、撤销和列表移除；写操作使用 step-up，明文只在创建/轮换后显示。
- `api/admin/adminAccessKeys.ts` 对接服务端权限目录与密钥 CRUD；页面不能替代服务端权限判定。旧全权密钥区域仍由 SettingsView 管理。
- 验证：`__tests__/AdminAccessKeysSettings.spec.ts`、完整 Vitest、lint:check、typecheck 和 build。
