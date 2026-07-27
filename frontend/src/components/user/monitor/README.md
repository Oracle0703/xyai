# 用户渠道监控组件

本目录组成用户侧渠道监控卡片、指标、provider icon、可用性行和最近状态时间线。

`MonitorTimeline.vue` 固定展示最多 `length` 个 bucket（默认 60），真实数据从 newest-first 反转为 oldest-first, 左侧用 empty bucket 补齐, 右端始终代表 now。状态同时使用高度和颜色编码；maintenance 时用固定高度占位替代时间线。

时间线容器是稳定的 flex track。每根 bar 使用 `flex-1 min-w-0`, 不能设置会让 60 根 bar 在窄卡片中溢出的固定最小宽度；标签使用 tabular number 并保持 past/now 两端对齐。

## 验证

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/components/user/monitor
cmd.exe /c pnpm --dir frontend run typecheck
```
