# 用户管理弹窗

- 0.2.15：`UserPlatformQuotaModal.vue` 使用 `platformQuotaPlatforms()` 从内置清单构建额度行，包含 Command Code/Cline；`BulkEditUserModal.vue` 通过 `extractApiErrorMessage` 展示后端规范错误。本地角色、权限目录、部门归属和 CAS 合同继续独立保留。

- `UserCreateModal.vue`、`UserEditModal.vue` 从服务端权限目录生成子管理员权限选项；`admin.subscriptions` 与 `admin.department_subscriptions` 互斥选择，后端仍独立验证。
- 部门报表、部门订阅与成员管理权限分别授权；仅勾选权限不会自动获得全站或部门数据，需在部门管理页维护负责人部门集合。
- 用户页部门归属通过 `../department/DepartmentAssignmentDialog.vue` 单独维护，提交显式成员 ID、旧部门和版本；不混入普通用户编辑或已有分组/订阅写入。
- 部门调整不改变 API Key 分组或多平台订阅；归属实际变化会清空负责人备注。完整管理员负责转岗与订阅分配，具备 `admin.department_members` 的负责人可经专用页面加入同组织未分配普通用户、移出、启停用、改用户名与负责人备注。
- `UsersView.vue` 在部门列下只读显示 `department_note`，状态列标注 `disabled_source='department_manager'`；`UserCreateModal.vue`、`UserEditModal.vue` 和通用编辑请求不增加负责人备注字段。管理员 `notes` 与负责人备注相互独立。

- 编辑打开/切换用户先获取管理详情与 `admin_access_version`；详情失败禁止保存，取消/切换忽略迟到响应。普通资料只发差量；角色/规范化权限变化才发送两字段和 `expected_admin_access_version`，409 后刷新并人工确认。
- 权限目录 code 包含点，i18n 使用方括号字面键路径读取 label/description。
