# Organization Usage Components

该目录承载管理端组织用量报表的页面子组件。

- `OrganizationUsageFilters.vue`: 报表周期、组织、邮箱筛选与导出入口；日期范围计算复用 `utils/organizationUsageReport.ts`。
- `OrganizationUsageOverview.vue`: 当前筛选范围的概览指标与日/周/月 Champion。
- `OrganizationUsageSummary.vue`: 固定三组组织汇总；首列使用原生按钮切换组织筛选，表格行保留标准 `tr` 语义。
- `OrganizationUsagePeopleTable.vue`: 服务端排序、分页的人员汇总表；移动端保持横向滚动表格。

页面级请求、AbortController 竞态保护和导出流程由 `views/admin/OrganizationUsageView.vue` 统一编排。导出数据拉取完成后，通过 `utils/organizationUsageExportWorker.ts` 协调 `organizationUsageExport.worker.ts` 在 Worker 内构建并序列化工作簿，避免阻塞主线程。
