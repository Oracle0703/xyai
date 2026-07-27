# 管理端支付组件

本目录承载管理端订单列表、详情、退款和支付统计看板。金额展示必须使用订单或统计响应携带的 ISO 4217 币种, 不能假设所有 provider 都使用同一币种。

## 主要文件

| 文件 | 职责 |
| --- | --- |
| `AdminOrderTable.vue` / `AdminOrderDetail.vue` | 订单列表与详情, 按订单自身 currency 格式化金额 |
| `AdminRefundDialog.vue` | 管理员退款输入与状态交互 |
| `OrderStatsCards.vue` | 今日、累计和平均金额, 每个币种独立展示 |
| `DailyRevenueChart.vue` | 每日收入按币种拆分为独立 series |
| `PaymentMethodChart.vue` | 支付方式数量与分币种金额 |
| `TopUsersLeaderboard.vue` | 每个币种独立的用户支付排行 |

## 多币种合同

- `DashboardStats.today_amount`、`total_amount`、`avg_amount` 和 daily/method amount 都是 `Record<string, number>`。
- `DashboardStats.top_users` 是 `Record<string, TopUserPaymentStats[]>`; 不生成跨币种总榜。
- 币种列表按代码稳定排序, 金额通过 `Intl.NumberFormat` 使用对应 currency 格式化。
- 不得把 CNY、USD 等金额相加后显示单一 total、占比或排行；图表也必须为每个币种建立独立 dataset。

## 验证

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/components/admin/payment
cmd.exe /c pnpm --dir frontend run typecheck
```
