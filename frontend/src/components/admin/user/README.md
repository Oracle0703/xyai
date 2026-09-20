# 用户管理弹窗

- `UserCreateModal.vue`、`UserEditModal.vue` 从服务端权限目录生成子管理员权限选项；`admin.subscriptions` 与 `admin.department_subscriptions` 互斥选择，后端仍独立验证。
- 部门报表与部门订阅权限分别授权；仅勾选权限不会自动获得全站或部门数据，需在部门管理页维护负责人部门集合。
- 用户页部门归属通过 `../department/DepartmentAssignmentDialog.vue` 单独维护，提交显式成员 ID、旧部门和版本；不混入普通用户编辑或已有分组/订阅写入。
- 部门调整不改变 API Key 分组或多平台订阅。完整管理员负责成员归属与订阅分配，部门负责人仅查询、导出和额度重置。
