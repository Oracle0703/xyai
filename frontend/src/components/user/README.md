# 用户通用组件

- `UserPlatformQuotaCell.vue` 仅展示至少一档已配置额度的平台；0.2.15 按 `constants/platformCatalog` 的具体平台顺序排序，未知平台排在末尾，不自行判断服务端额度准入。
- 资料、仪表盘、监控组件的详细合同见 `profile/README.md`、`dashboard/README.md`、`monitor/README.md`。
- 修改配额展示时运行 `src/components/admin/user/__tests__/UserPlatformQuotaModal.spec.ts` 及完整 frontend typecheck/Vitest，后端仍通过平台清单校验写入。
