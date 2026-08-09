# Organization Usage Components

该目录承载管理端组织用量报表的页面子组件。

- `OrganizationUsageFilters.vue`: 报表周期、组织、邮箱筛选与导出入口；日期范围计算复用 `utils/organizationUsageReport.ts`。组织选项把内部键 `xunyou` / `wsdashi` 显示为“迅游”/“速宝”，但提交给 API 的筛选值不变。
- `OrganizationUsageOverview.vue`: 当前筛选范围的概览指标与日/周/月 Champion；`active_users` 显示为“注册人数”，`used_users` 显示为“活跃人数”，仅为前端展示别名。
- `OrganizationUsageTrendChart.vue`: 用量趋势折线图（Chart.js 双轴）；粒度切换与局部 loading/error/retry；默认系列为输入 Token、输出 Token、总 Token 与请求数。总 Token 包含缓存创建和缓存读取两类 Token，不能视为另外两条可见 Token 曲线之和。
- `OrganizationUsageSummary.vue`: 固定三组组织汇总；将 `xunyou` / `wsdashi` / `other` 显示为“迅游”/“速宝”/“其他”，首列使用原生按钮按原内部筛选键切换组织筛选，表格行保留标准 `tr` 语义；人数列沿用“注册人数”=`active_users`、“活跃人数”=`used_users` 的展示别名。
- `OrganizationUsagePeopleTable.vue`: 服务端排序、分页的人员汇总表；组织列沿用“迅游”/“速宝”/“其他”展示映射而不改内部键，移动端保持横向滚动表格。

## 页面编排（View）

页面级请求由 `views/admin/OrganizationUsageView.vue` 统一编排：

| 控制器 | 用途 |
| --- | --- |
| `reportController` + `loading` | Summary；控制 Overview / 组织汇总隐藏与人员 skeleton |
| `trendController` + `trendLoading` | Trend；**仅**图表区 skeleton，翻页/排序不 abort |

完整加载（mount / apply / reset / 组织行 / 页面 Retry）会：

1. 递增 `reportCycleId`，生成同一 candidate `as_of`
2. 并行请求 Summary 与 Trend
3. 以 Summary 返回的 canonical `as_of` 为权威；若 Trend 不一致则**每周期最多重拉一次** Trend

人员 sort/page/page_size 只打 Summary 并复用 `snapshotAsOf`，保持已成功的趋势曲线。

粒度：默认按日期跨度自动推断（≤31 day / ≤120 week / 否则 month，见 `inferOrganizationUsageTrendGranularity`）；手动切换只打 Trend。

导出数据拉取完成后，通过 `utils/organizationUsageExportWorker.ts` 协调 Worker 构建工作簿。`fetchAll` **不**调用 trend。

设计文档：`docs/features/organization-usage-trend-chart-design-cn.md`。
