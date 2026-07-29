# 可用渠道组件

本目录渲染用户可见的渠道、平台、分组和模型价格信息。

## 响应式布局

- `AvailableChannelsTable.vue` 在 `lg` 及以上使用五列表格；渠道名和描述跨 platform row 合并。
- 小于 `lg` 时使用按 channel 分节、按 platform 分块的移动布局, 不依赖横向滚动。
- channel/description、GroupBadge、峰值倍率和模型 chip 都必须允许 wrap；容器保留 `min-w-0` 与 `overflow-x-hidden`, 长模型名或长分组名不能撑破视口。
- loading、empty、exclusive/public group 和 pricing/no-pricing 语义在桌面与移动布局中必须一致。

`SupportedModelChip.vue` 和 `PricingRow.vue` 负责模型能力与价格明细；`AvailableChannelsTable.vue` 只组织分组和响应式呈现, 不重新计算后端定价。

## 验证

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/components/channels
cmd.exe /c pnpm --dir frontend run typecheck
```
