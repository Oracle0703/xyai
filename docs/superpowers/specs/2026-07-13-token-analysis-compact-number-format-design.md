# Token Analysis Compact Number Format Design

## 状态

| 项目 | 说明 |
| --- | --- |
| 状态 | 已实现，本文档按 2026-07-14 代码审核结论补全为可验收规格 |
| 页面 | `frontend/src/views/admin/TokenAnalysisView.vue` |
| 测试 | `frontend/src/views/admin/__tests__/TokenAnalysisView.spec.ts` |
| 变更性质 | 管理端 Token 分析页面的局部展示优化 |

需求发生冲突时，按“用户最新确认的业务合同 -> 本设计规格 -> 当前实现”的顺序判断。实现与规格不一致时，应修正实现或明确记录差异，不能直接以当前代码覆盖已确认需求。

## 目标

优化管理端 Token 分析页面的大数展示，使概览卡片和用户排行更易扫描，并通过原生 `title` 保留完整数值供桌面端悬停查看。

## 修改范围

- 修改 `TokenAnalysisView.vue` 内的页面专用数字格式化与展示映射。
- 修改 `TokenAnalysisView.spec.ts`，覆盖代表字段的阈值和精确值。
- 不修改 API、后端聚合、数据库或其他页面的全局格式化行为。
- 不新增共享 formatter，不改变共享 formatter 的现有调用方。

## 输入约束

- API 返回的 Token、请求数和消费均应为非负、有限的 JSON 数值。
- `null`、`undefined` 和空值按页面现有行为显示为 `0`。
- 页面专用紧凑 formatter 使用 `finiteNumber` 防御运行时异常值；`NaN` 和 `Infinity` 的紧凑展示回退为 `0`。
- `NaN` 和 `Infinity` 不是合法 API 数据，也不作为精确 `title` formatter 的验收输入。
- 单位选择使用格式化前的绝对值，之后再执行 `toFixed(1)`；舍入后不跨单位晋级。
- 聚合指标业务上不应为负数。formatter 的阈值按绝对值判断并保留数值符号，但这不表示负用量或负消费属于合法业务数据。

## 数值合同

### Token 指标

适用范围：概览卡 Token 字段和用户排行 `total_tokens`。

| 原始绝对值 | 展示规则 |
| --- | --- |
| `abs < 1_000_000` | 页面本地 `formatNumber` 输出完整整数，不使用 K |
| `1_000_000 <= abs < 1_000_000_000` | `value / 1_000_000`，保留 1 位小数并添加 `M` |
| `abs >= 1_000_000_000` | `value / 1_000_000_000`，保留 1 位小数并添加 `B` |

示例：

| 输入 | 输出 |
| ---: | ---: |
| `1_200` | `1,200` |
| `999_999` | `999,999` |
| `1_000_000` | `1.0M` |
| `1_200_000` | `1.2M` |
| `999_999_999` | `1000.0M` |
| `1_000_000_000` | `1.0B` |

### 请求数指标

适用范围：概览卡请求数字段。

| 原始绝对值 | 展示规则 |
| --- | --- |
| `abs < 1_000` | 页面本地 `formatNumber` 输出完整整数 |
| `1_000 <= abs < 1_000_000` | `value / 1_000`，保留 1 位小数并添加 `K` |
| `1_000_000 <= abs < 1_000_000_000` | `value / 1_000_000`，保留 1 位小数并添加 `M` |
| `abs >= 1_000_000_000` | `value / 1_000_000_000`，保留 1 位小数并添加 `B` |

示例：`999 -> 999`、`1_000 -> 1.0K`、`999_999 -> 1000.0K`、`1_000_000 -> 1.0M`、`1_000_000_000 -> 1.0B`。

### 用户排行费用

适用范围：用户排行 `actual_cost`，货币符号固定为 `$`。

| 原始绝对值 | 展示规则 |
| --- | --- |
| `abs < 1_000` | 页面本地 `formatCost`，保留 4 位小数 |
| `abs >= 1_000` | `value / 1_000`，保留 1 位小数并添加 `K` |

费用永不切换到 `M` 或 `B`。因此 `1_200_000` 显示为 `$1200.0K`，这是用户已确认的页面专属规则，不得擅自改成 `$1.2M`。

负消费不属于业务输入合同。若异常数据为 `-1_500`，当前 formatter 保留现有货币前缀顺序并输出 `$-1.5K`；不使用 `-$1.5K` 作为验收断言。

### 单位与精确值

- 紧凑单位固定使用英文大写 `K`、`M`、`B`，不随语言环境切换为“万/亿”。
- 紧凑值统一保留 1 位小数，包括边界值 `1.0K`、`1.0M`、`1.0B`。
- 页面本地 `formatNumber` 使用 `Intl.NumberFormat().format(Math.round(value || 0))`，用于完整整数和精确 `title`。
- 页面本地 `formatCost` 使用 `$${Number(value || 0).toFixed(4)}`，用于原金额和精确 `title`。
- `Intl.NumberFormat()` 的分组符随浏览器运行环境变化；本规格要求值完整，不要求固定为某一种 locale 分隔符。

## 页面映射

### 概览卡片

| 字段 | 展示函数 | `title` |
| --- | --- | --- |
| `total_requests` | `formatRequestMetric` | `formatNumber` |
| `billed_requests` | `formatRequestMetric` | `formatNumber` |
| `risky_requests` | `formatRequestMetric` | `formatNumber` |
| `total_tokens` | `formatTokenMetric` | `formatNumber` |
| `total_input_tokens` | `formatTokenMetric` | `formatNumber` |
| `total_output_tokens` | `formatTokenMetric` | `formatNumber` |
| `cache_read_tokens` | `formatTokenMetric` | `formatNumber` |
| `archive_coverage` | `percent` | 与展示值相同 |
| `cache_hit_rate` | `percent` | 与展示值相同 |
| `total_actual_cost` | `formatCost` | `formatCost` |
| `risky_cost` | `formatCost` | `formatCost` |

### 用户排行

| 字段 | 展示函数 | `title` |
| --- | --- | --- |
| `total_tokens` | `formatUserRankingTokens`，内部委托 `formatTokenMetric` | `formatNumber` |
| `actual_cost` | `formatUserRankingCost` | `formatCost` |

测试可以使用 `total_tokens` 和 `total_requests` 作为代表字段，但同类字段必须继续共用表中指定函数。

## 实现边界

页面内保留以下专用纯函数：

- `finiteNumber`：为紧凑 formatter 归一化运行时数值。
- `formatTokenMetric`：完整整数或 M/B。
- `formatRequestMetric`：完整整数或 K/M/B。
- `formatUserRankingTokens`：委托 `formatTokenMetric`。
- `formatUserRankingCost`：原金额或 K。

不扩展共享 `frontend/src/utils/format.ts#formatCompactNumber`，因为 Token 禁用 K、用户排行费用只使用 K 都是本页专属规则。

## 防回归约束

1. 本页精确整数和金额继续使用页面本地 `formatNumber`、`formatCost`。
2. 不使用共享 `@/utils/format#formatNumber` 生成精确值；该函数在 `abs >= 10_000` 时启用 locale compact，中文环境可能显示“万/亿”。
3. 不把本页规则并入共享 `formatCompactNumber`，避免影响其他页面。
4. 用户排行 Token 小于 1M 时必须显示完整整数，不能恢复为 `0.0M`。
5. 用户排行超大费用继续使用 K，不能自动晋级 M/B。
6. 页面展示规则冲突时先核对用户合同和本规格，不能默认以当前实现覆盖规格。

## 已知取舍

- 用户排行 Token 禁用 K，只在 M/B 量级紧凑展示。
- 项目排行 Token 继续使用共享 `formatCompactNumber`，保留现有 K/M/B。
- 这是同一页面内有意保留的差异，不在本次修改范围内。
- 概览金额卡 `total_actual_cost`、`risky_cost` 保持 `$x.xxxx`，不做紧凑化。
- 原生 `title` 满足当前桌面端悬停查看精确值的需求，不新增移动端弹层或自定义 Tooltip。

## 测试合同

### 必须覆盖

- Token：`999_999 -> 999,999`、`1_000_000 -> 1.0M`、`1_000_000_000 -> 1.0B`。
- 请求数：`999 -> 999`、`1_000 -> 1.0K`、`1_000_000 -> 1.0M`、`1_000_000_000 -> 1.0B`。
- 用户排行费用：`999.9999 -> $999.9999`、`1_000 -> $1.0K`、`1_200_000 -> $1200.0K`。
- 用户排行 Token 小于 1M 时不得出现 `0.0M`。
- 紧凑值对应的 `title` 必须包含完整整数或 4 位小数金额。
- 测试只覆盖代表字段时，代码映射仍需确认同类字段共用同一 formatter。

### 非阻断补充覆盖

- 可增加单位舍入前边界测试：Token `999_999_999 -> 1000.0M`、请求数 `999_999 -> 1000.0K`。
- 只有在产品允许负消费后，才补充负金额的展示合同和测试。
- 不为 API 无法合法返回的 `NaN`、`Infinity` 增加精确 `title` 验收测试。

## 验证

按以下顺序执行：

```powershell
pnpm --dir frontend test:run src/views/admin/__tests__/TokenAnalysisView.spec.ts
pnpm --dir frontend test:run src/utils/__tests__/formatCompactNumber.spec.ts
pnpm --dir frontend run test:run
pnpm --dir frontend run typecheck
pnpm --dir frontend run lint:check
git diff --check
```

- Token Analysis 定向测试、共享 formatter 定向测试、全量 Vitest、typecheck、lint:check 和差异格式检查均为交付前必检项。
- 不使用 `pnpm --dir frontend run lint` 做验证，因为该脚本包含 `--fix`，会修改工作区。
- 若全量测试失败，必须定位是否由本次改动引入；当前仓库不存在已确认可忽略的相关预存红灯。

## 非目标

- 不改变数值、排序、分页、查询条件或后端返回结构。
- 不改变项目排行现有 K/M/B 格式。
- 不将用户排行费用切换为 M/B。
- 不修改匹配质量、风险原因、归档文件、请求明细或其他用量页面。
- 不因这项局部展示改动更新 `llm-wiki`；若实施时发现 wiki 与源码冲突，再单独修正文档事实。
