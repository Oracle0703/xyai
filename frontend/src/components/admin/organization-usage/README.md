# Organization Usage Components

该目录承载组织/部门用量报表；完整管理员可选全部范围，负责人只查询和导出获授权部门。

| 组件 | 职责 |
| --- | --- |
| `OrganizationUsageFilters.vue` | 北京时间周期、授权组织/部门、平台及邮箱筛选；组织变化清部门，未应用草稿时禁用导出 |
| `OrganizationUsageOverview.vue` | 成员人数、活跃人数/率、请求、Token、费用和日周月 champion；`active_users` 为成员人数，`used_users` 为有记录人数 |
| `OrganizationUsageTrendChart.vue` | Chart.js 双轴、日周月粒度、独立 loading/error/retry；总 Token 包含缓存创建/读取 |
| `OrganizationUsageSummary.vue` | 仅渲染服务端返回的组织，不补范围外组织行；按钮提交原内部键 |
| `OrganizationUsageBreakdowns.vue` | 同范围部门/平台汇总，平台活跃人数不能相加作为总人数 |
| `OrganizationUsagePeopleTable.vue` | 带组织和部门的人员表，服务端排序分页，保留宽表横向滚动 |

组织内部键 `xunyou / wsdashi / other` 显示为迅游/速宝/其他，部门名称动态配置。平台筛选只过滤用量，零用量成员仍保留。

## 页面请求与导出

`views/admin/OrganizationUsageView.vue` 先请求 scope，再并行加载 Summary/Trend，共用 `scope_version` 和 candidate `as_of`。Summary 的 canonical `as_of` 为权威，必要时每周期单次对齐 Trend；人员翻页/排序只刷新 Summary。

完整查询递增 `reportCycleId`，取消旧控制器并清数据；403/范围冲突清除保护结果及导出任务。409 最多重新获取 scope 并重试一次，持续变化停止；空授权显示联系管理员，不回退全站。迟到响应不能覆盖新筛选。

Excel 使用 `organizationUsageReport.ts` 与可终止 Worker，生成报表概览、组织、部门、平台、人员、月、周、日八个 Sheet。所有数据 Sheet 合计上限 100,000 行；元信息包含实际筛选、当前成员归属口径、生成时间、`as_of/scope_version`。各页及 Sheet 版本不一致立即中止，`fetchAll` 不请求 Trend。

设计：`docs/features/organization-department-usage-design-cn.md`。专项覆盖 View 403/409/迟到响应、scope 汇总、分页导出/取消/超限及工作簿字符串安全；真实浏览器与 Excel 对账见 `docs/delivery/2026-09-19-department-usage/acceptance.md`。
