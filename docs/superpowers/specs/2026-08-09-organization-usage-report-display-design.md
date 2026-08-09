# 组织用量报表展示调整设计

## 目标

在不改变组织分组、筛选参数、API 字段和后端统计口径的前提下，统一调整组织用量报表页面与 XLSX 导出的展示文案，并将趋势图中的缓存 Token 曲线替换为总 Token 曲线。

## 展示合同

| 区域 | 当前展示 | 目标展示 | 底层数据 |
| --- | --- | --- | --- |
| 用量概览 | 活跃人数 | 注册人数 | `active_users`，语义与计算不变 |
| 用量概览 | 有用量人数 | 活跃人数 | `used_users`，语义与计算不变 |
| 组织汇总 | 活跃人数 | 注册人数 | `active_users`，语义与计算不变 |
| 组织汇总 | 有用量人数 | 活跃人数 | `used_users`，语义与计算不变 |
| 用量趋势 | 缓存 Token 曲线 | 总 Token 曲线 | `cache_read_tokens` 数据集替换为 `total_tokens` 数据集 |
| 组织名称 | `xunyou` / `xunyou.com` | 迅游 | 内部键仍为 `xunyou` |
| 组织名称 | `wsdashi` / `wsdashi.com` | 速宝 | 内部键仍为 `wsdashi` |
| 组织名称 | `other` | 其他 | 内部键仍为 `other` |

组织名称映射覆盖筛选器、组织汇总、人员汇总和 XLSX。品牌名在中英文界面均按用户指定显示为“迅游”和“速宝”。

## 实现边界

采用集中展示映射：在组织用量报表工具模块维护 `formatOrganizationUsageOrganization(value: string, otherLabel?: string): string`，兼容 API 当前可能返回的短键和域名形式；品牌名固定返回“迅游”或“速宝”，未知值返回调用方提供的本地化其他名称（默认“其他”）。组件和导出逻辑复用该函数。该函数不得反向参与 API 请求、筛选值或数据聚合。

趋势图继续使用现有双轴和交互配置，数据集顺序调整为：输入 Token、输出 Token、总 Token、请求数。总 Token 复用当前第三条 Token 曲线的配色和轴配置，仅替换 label 与数据来源。`total_tokens` 仍是 input、output、cache creation、cache read 四类之和，因此它不是另外两条可见 Token 曲线的简单相加；缓存创建/读取字段继续保留在 API 与 XLSX Token 明细中，人员表只保留既有的缓存读取 Token 明细，仅从趋势折线中移除缓存 Token。

## 影响文件

| 类别 | 文件 |
| --- | --- |
| 展示组件 | `frontend/src/components/admin/organization-usage/OrganizationUsageOverview.vue`、`OrganizationUsageSummary.vue`、`OrganizationUsageFilters.vue`、`OrganizationUsagePeopleTable.vue`、`OrganizationUsageTrendChart.vue` |
| 文案与映射 | `frontend/src/i18n/locales/{zh,en}/admin/organizationUsage.ts`（人数标题）、`frontend/src/utils/organizationUsageReport.ts`（唯一组织显示映射） |
| 测试 | 组件专项测试、`frontend/src/utils/__tests__/organizationUsageReport.spec.ts`、`organizationUsageWorkbook.spec.ts`、相关 View 测试 |
| 稳定文档 | `frontend/src/components/admin/organization-usage/README.md`、两份组织用量 feature 设计、`llm-wiki/wiki/frontend.md` |

## 验证

先写失败测试，再做最小实现。专项测试必须证明：人数文案正确、组织显示名覆盖所有页面入口和 XLSX、趋势图包含 `total_tokens` 且不包含缓存数据集、筛选请求仍发送原内部键。最后运行前端专项 Vitest、完整 typecheck、lint 检查，并检查 Git diff 与文档一致性。

## 非目标

- 不修改后端、数据库、API DTO 或组织分类 SQL。
- 不重命名 `active_users`、`used_users`、`xunyou`、`wsdashi` 等内部字段和值。
- 不改变 API 与 XLSX 中已有的缓存创建/读取 Token 明细，或人员表中既有的缓存读取 Token 明细。
- 不改变趋势请求、`as_of` 对齐、补零、粒度或导出分页逻辑。
