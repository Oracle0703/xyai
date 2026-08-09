# 设计规格

## 摘要

| 字段 | 内容 |
| --- | --- |
| 需求来源 | `requirements.md` |
| 交付目标 | 用统一展示映射完成页面和 XLSX 文案调整，不改变数据合同 |
| 主要验收标准 | 人数标题、趋势数据集、组织显示名和内部键兼容性均有自动化证据 |

## 当前状态

| 区域 | 当前行为 / 证据 |
| --- | --- |
| 页面入口 | `frontend/src/views/admin/OrganizationUsageView.vue` 编排筛选、概览、趋势、组织和人员组件 |
| 人数 | `active_users` 显示“活跃人数”，`used_users` 显示“有用量人数” |
| 趋势 | `OrganizationUsageTrendChart.vue` 展示输入、输出、缓存读取和请求数 |
| 组织名称 | 筛选、汇总和人员表分别硬编码 `xunyou.com` / `wsdashi.com` |
| XLSX | `organizationUsageReport.ts` 直接写内部组织键，概览和组织表使用旧人数标题 |
| 数据 / 接口 | API 已提供 `active_users`、`used_users`、`total_tokens`；组织筛选类型为 `all/xunyou/wsdashi/other` |
| 测试 / 命令 | 现有组件、View、工具和 API Vitest；前端提供 `typecheck`、`lint:check` |

## 目标行为

| ID | 行为 | 用户 / 系统影响 |
| --- | --- | --- |
| TB-1 | `active_users` 的显示标签统一为“注册人数 / Registered users” | 仅显示变化 |
| TB-2 | `used_users` 的显示标签统一为“活跃人数 / Active users” | 仅显示变化 |
| TB-3 | 趋势 Token 轴包含 input、output、total，不包含 cache 数据集 | 图表更直接展示总量 |
| TB-4 | 迅游和速宝组织在页面及 XLSX 显示指定中文名称 | 内部键仍可稳定筛选和聚合 |

## 接口

| 接口 | 变更 | 兼容性说明 |
| --- | --- | --- |
| 后端 API / DTO | 无 | 不修改 Go 代码或 JSON 字段 |
| `OrganizationUsageOrganizationFilter` | 无 | 仍为 `all/xunyou/wsdashi/other` |
| i18n | 更新人数标题 | 中英文 locale 都提供完整键；品牌映射不在 locale 中重复维护 |
| 展示工具函数 | 新增 `formatOrganizationUsageOrganization(value, otherLabel)` | 接受短键和 `.com` 形式，不用于请求构造；未知值回落 `otherLabel` |
| Chart.js datasets | `cache_read_tokens` 数据集替换为 `total_tokens` | 双轴、tooltip、粒度和状态逻辑不变 |
| XLSX | 调用统一组织展示函数并更新人数行/列标题 | 文件结构和工作表名称不变 |

## 数据契约

| 字段 / 值 | 必需规则 | 兼容性 / 消费方说明 |
| --- | --- | --- |
| `active_users` | 值不变，显示为注册人数 | Overview、Organization Summary、XLSX |
| `used_users` | 值不变，显示为活跃人数 | Overview、Organization Summary、XLSX |
| `total_tokens` | 作为第三条 Token 趋势数据集；仍等于 input + output + cache creation + cache read | API 已返回，无后端变更；不应解释为两条可见 input/output 曲线之和 |
| `cache_read_tokens` | 不再进入趋势 datasets | 人员表仍保留既有 cache read；API DTO 和 XLSX 明细保留 cache creation/read |
| `xunyou` / `xunyou.com` | 显示“迅游” | 请求与筛选仍发送 `xunyou` |
| `wsdashi` / `wsdashi.com` | 显示“速宝” | 请求与筛选仍发送 `wsdashi` |
| `other` / 未识别值 | 显示本地化“其他 / Other” | 不创建新分组 |

## 数据流

| 步骤 | 输入 | 处理 | 输出 |
| --- | --- | --- | --- |
| 1 | API response 的组织键和指标 | 保持原对象不变 | 组件 props |
| 2 | 组织键 | 纯展示函数规范化短键或域名 | “迅游”“速宝”或本地化其他 |
| 3 | Trend points | 从每个点读取 input/output/total/requests | Chart.js 四个 datasets |
| 4 | 导出 Champion、组织、人员和周期行 | 复用组织展示函数，替换人数标题 | 保持原 sheet 结构的 XLSX |
| 5 | 筛选选择 | UI 显示中文品牌名，option value 保持短键 | 原 API query |

## 失败模式与边界

| 场景 | 预期处理 | 需要的证据 |
| --- | --- | --- |
| API 返回 `.com` 兼容形式 | 与短键映射到同一中文品牌名 | 工具函数/组件测试 |
| API 返回未知组织值 | 回落本地化“其他 / Other” | 工具函数测试 |
| 趋势点的 `total_tokens` 为 0 | 正常绘制 0，不回落缓存值 | Chart 数据集测试 |
| 切换组织筛选 | 显示名改变但 emit/request value 不变 | Filters 或 View 测试 |
| 空数据 | 延续现有空态，不新增异常路径 | 现有测试继续通过 |

## 验收标准映射

| 成功标准 | 规格覆盖 | 验证方式 |
| --- | --- | --- |
| SC-1 | TB-1、TB-2 | Overview 挂载测试与 i18n 断言 |
| SC-2 | TB-1、TB-2 | Summary 表头测试 |
| SC-3 | TB-3 | TrendChart 暴露数据集断言 |
| SC-4 | TB-4、数据流 2/5 | Filters、Summary、PeopleTable 测试 |
| SC-5 | TB-1、TB-2、TB-4、数据流 4 | workbook 单元测试 |
| SC-6 | 接口与数据契约 | API 请求断言、typecheck、Git diff 审查 |
