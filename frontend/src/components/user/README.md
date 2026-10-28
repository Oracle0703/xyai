# 用户通用组件

- `UserPlatformQuotaCell.vue` 仅展示至少一档已配置额度的平台；0.2.15 按 `constants/platformCatalog` 的具体平台顺序排序，未知平台排在末尾，不自行判断服务端额度准入。
- 资料、仪表盘、监控组件的详细合同见 `profile/README.md`、`dashboard/README.md`、`monitor/README.md`。
- `GPTQuotaMeter.vue` 复用 `utils/gptQuota.ts` 的额度分级：>30% 绿色、≤30% 黄色、≤10% 红色。默认 `variant="bar"` 保持管理列表紧凑展示；用户看板 `GPTQuotaColumn.vue` 显式使用 `variant="ring"`，SVG 圆环从 12 点起、额度下降时末端逆时针回退，100% 闭合、0% 无残点，中心显示百分比。缺数据用灰色空环与“未提供”，不是 0%；支持深色及减少动态效果。账号为自适应网格，数据/重置/过期标识、缺失 5 小时窗口隐藏及两组顺序保持。
- 修改配额展示时运行 `src/components/admin/user/__tests__/UserPlatformQuotaModal.spec.ts` 及完整 frontend typecheck/Vitest，后端仍通过平台清单校验写入。
