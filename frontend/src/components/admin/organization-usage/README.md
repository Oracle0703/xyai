# Organization Usage Components

该目录承载组织/部门用量报表；完整管理员可选全部范围，负责人只查询和导出获授权部门。

| 组件 | 职责 |
| --- | --- |
| `OrganizationUsageFilters.vue` | 北京时间周期、授权组织/部门、平台及邮箱筛选；组织变化清部门，未应用草稿时禁用导出 |
| `OrganizationUsageOverview.vue` | 成员人数、活跃人数/率、请求、Token、费用和日周月 champion；`active_users` 为成员人数，`used_users` 为有记录人数 |
| `OrganizationUsageTrendChart.vue` | Chart.js 双轴、日周月粒度、独立 loading/error/retry；总 Token 包含缓存创建/读取 |
| `OrganizationUsageSummary.vue` | 仅渲染服务端返回的组织，不补范围外组织行；按钮提交原内部键 |
| `OrganizationUsageBreakdowns.vue` | 同范围部门/平台汇总，平台活跃人数不能相加作为总人数 |
| `OrganizationUsagePeopleTable.vue` | 带组织和部门的人员表，服务端排序分页，保留宽表横向滚动；每页选择器前可下载当前页 PNG |

组织内部键 `xunyou / wsdashi / other` 显示为迅游/速宝/其他，部门名称动态配置。平台筛选只过滤用量，零用量成员仍保留。

## 页面请求与导出

`views/admin/OrganizationUsageView.vue` 先请求 scope 目录，再由首个 Summary 确定查询 `scope_version` 和 canonical `as_of`，随后加载 Trend；目录 `catalog_version` 不作为查询版本。必要时每周期单次对齐 Trend；人员翻页/排序只刷新 Summary。

完整查询递增 `reportCycleId`，取消旧控制器并清数据；403/范围冲突清除保护结果及导出任务。仅业务码 `REPORT_SCOPE_CHANGED`（reason 或字符串 code）最多重新获取 scope 并重试一次，持续变化停止；空授权显示联系管理员，不回退全站。迟到响应不能覆盖新筛选。

Excel 使用 `organizationUsageReport.ts` 与可终止 Worker，按报表概览、人员、组织、部门、平台、月、周、日顺序生成八个 Sheet。人员汇总包含筛选范围内全部人员（不受页面 20/50/100 条限制）、零用量成员、组织/部门、数值指标及日周月峰值，设置列宽和自动筛选。所有数据 Sheet 合计上限 100,000 行；元信息包含实际筛选、当前成员归属口径、生成时间、`as_of/scope_version`。各页及 Sheet 版本不一致立即中止，`fetchAll` 不请求 Trend。

PNG 截图仅包含当前页已渲染的表头与人员行，末页按实际行数，覆盖横向滚动隐藏的列；通过 `utils/tableScreenshot.ts` 按需加载 `html2canvas`，仅解除克隆 DOM 的滚动裁剪，不改页面布局。文件名含报表日期范围与页码；加载中、空数据及截图中禁用按钮。分页/查询数据变化或卸载会作废在途截图，避免下载旧范围数据。

设计：`docs/features/organization-department-usage-design-cn.md`。专项覆盖 View 403/409/迟到响应、scope 汇总、分页导出/取消/超限及工作簿字符串安全；真实浏览器与 Excel 对账见 `docs/delivery/2026-09-19-department-usage/acceptance.md`。

- 2026-09-21 精简：Summary 建立快照后再启动 Trend，移除旧并行对齐函数及请求时间状态；人员续页不允许更换 as_of。导出必须取得完整 as_of/scope_version，并校验每页/Sheet 一致，删除缺字段继续导出的兼容兜底；保留取消、总行数和分页上限。
