# 管理端账号组件

- `AccountActionMenu.vue` 使用 `supportsAccountQuota` 为 apikey/bedrock/service_account 判断配额重置能力；菜单仍经过本地 `useAccountOperatorAccess`，子管理员只能执行已授权动作。能力判断不替代后端权限校验。
- 0.2.18 仅同步上游 Vertex 配额判断，本地子管理员限制保留；验证入口为账号操作权限相关 Vitest 与完整前端测试。
