# Organization Usage Components

该目录承载管理端组织用量报表的页面子组件。

- `OrganizationUsageFilters.vue`: 报表周期、组织、邮箱筛选与导出入口；日期范围计算复用 `utils/organizationUsageReport.ts`。
- `OrganizationUsageOverview.vue`: 当前筛选范围的概览指标与日/周/月 Champion。
- `OrganizationUsageTrendChart.vue`: 用量趋势折线图（Chart.js 双轴）；粒度切换与局部 loading/error/retry；默认系列为输入/输出/缓存 Token + 请求数。
- `OrganizationUsageSummary.vue`: 固定三组组织汇总；首列使用原生按钮切换组织筛选，表格行保留标准 `tr` 语义。
- `OrganizationUsagePeopleTable.vue`: 服务端排序、分页的人员汇总表；移动端保持横向滚动表格。

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
