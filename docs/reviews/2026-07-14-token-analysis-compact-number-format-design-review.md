# Token Analysis 紧凑数字格式设计文档修改说明

| 字段 | 值 |
| --- | --- |
| 复核日期 | 2026-07-14 |
| 审核人 | Copilot |
| 目标文档 | `docs/superpowers/specs/2026-07-13-token-analysis-compact-number-format-design.md` |
| 对照实现 | `frontend/src/views/admin/TokenAnalysisView.vue` |
| 对照测试 | `frontend/src/views/admin/__tests__/TokenAnalysisView.spec.ts` |
| 共享工具 | `frontend/src/utils/format.ts#formatCompactNumber` |
| 本轮目标 | **只改设计文档**，不改业务代码；把规格补到可直接验收、可防回归 |

---

## 1. 结论

设计方向正确，范围克制，且与当前实现高度一致：

- 页面内已有 `formatTokenMetric` / `formatRequestMetric` / `formatUserRankingCost`
- 概览卡已有精确值 `title`
- 用户排行 Token 禁用 K、费用只到 K 的规则已落地
- 对应边界测试已存在

当前文档的主要问题不是方向错误，而是：

1. 数值合同仍偏“举例”，阈值/小数位/兜底未写死
2. 表格把 Token/费用硬塞进完整 K/M/B 阶梯，容易误读
3. 未写清“页面本地 `formatNumber` vs 共享 `formatNumber`”的防回归约束
4. 同页项目排行仍用 K 的有意不一致未显式声明
5. 测试与验证命令写得偏理想，覆盖面和失败归因不够硬

**给 Codex 的任务：**

> 按本文 P0/P1 修改目标设计文档，使其成为可执行验收规格。默认不改前端实现；若发现文档与实现冲突，以当前实现为准，并在文档中写明事实。

---

## 2. 修改范围

### 2.1 允许修改

- `docs/superpowers/specs/2026-07-13-token-analysis-compact-number-format-design.md`

### 2.2 默认不修改

- `frontend/src/views/admin/TokenAnalysisView.vue`
- `frontend/src/views/admin/__tests__/TokenAnalysisView.spec.ts`
- `frontend/src/utils/format.ts`
- 其他页面、API、后端、数据库

只有在用户明确要求“同步改实现/测试”时，才允许越出本文范围。

---

## 3. 必须补进文档的内容（P0）

### 3.1 增加文档状态

在文首增加：

```md
## 状态

- 状态：已实现（以 `TokenAnalysisView.vue` 当前代码为准）
- 目标：把页面专属紧凑数字规则固化为可验收规格
```

### 3.2 把“数值合同”改成可编码规则

不要只给示例。至少写死：

| 规则项 | 要求 |
| --- | --- |
| Token 阈值 | `abs >= 1_000_000_000 -> B`；`abs >= 1_000_000 -> M`；否则原整数。**不使用 K** |
| 请求数阈值 | `abs >= 1_000_000_000 -> B`；`abs >= 1_000_000 -> M`；`abs >= 1_000 -> K`；否则原整数 |
| 用户排行费用阈值 | `abs >= 1_000 -> $x.xK`；否则 `$x.xxxx`。**永不升到 M/B** |
| 紧凑小数位 | 统一 `toFixed(1)`，例如 `1.0M`、`1.2K`、`$1.0K` |
| 精确整数 | 页面本地 `formatNumber`：`Intl.NumberFormat().format(Math.round(value || 0))` |
| 精确金额 | 页面本地 `formatCost`：`` `$${Number(value || 0).toFixed(4)}` `` |
| 符号 | 阈值按绝对值判断，输出保留原符号 |
| 非有限值 | `NaN` / `Infinity` / `null` / `undefined` 兜底为 `0` |
| 单位语言 | 固定英文大写 `K` / `M` / `B`，不随 locale 变成“万/亿” |

建议把原四列表格改成“按指标分行”的规则表，避免 Token/费用被误读成完整 K/M/B 阶梯。

推荐结构：

```md
## 数值合同

### Token 指标（概览卡 + 用户排行 Token）

- 函数：`formatTokenMetric`
- 规则：...
- 示例：...

### 请求数指标（概览卡）

- 函数：`formatRequestMetric`
- 规则：...
- 示例：...

### 用户排行费用

- 函数：`formatUserRankingCost`
- 规则：...
- 示例：...
```

### 3.3 明确页面本地 formatNumber 约束

必须新增一节，类似：

```md
## 防回归约束

1. 本页精确值必须使用页面本地 `formatNumber` / `formatCost`。
2. 禁止直接复用共享 `@/utils/format#formatNumber` 作为精确值展示，
   因为共享实现在 `>=10000` 时会走 locale compact，中文环境可能变成“万/亿”。
3. 不扩展共享 `formatCompactNumber` 来承载本页规则：
   - Token 禁用 K
   - 费用只使用 K
   这两条是页面专属合同，不应污染全局工具。
```

### 3.4 显式声明同页不一致

在“非目标”或单独“已知取舍”中写明：

```md
## 已知取舍

- 用户排行 Token：禁用 K，只在 M/B 紧凑。
- 项目排行 Token：继续使用共享 `formatCompactNumber`，保留现有 K/M/B。
- 这是有意保留的同页不一致，不在本次规格修复范围内。
- 概览金额卡（`total_actual_cost` / `risky_cost`）保持 `$x.xxxx`，不做紧凑化。
- 用户排行超大费用继续用 `$x.xK`（例如 `$1200.0K`），不升 M/B。
```

### 3.5 页面映射写成“代表字段 + 共用函数”

当前映射基本正确，但要补成验收口径：

```md
## 页面映射

### 概览卡片

| 字段 | 展示函数 | 精确 title |
| --- | --- | --- |
| `total_requests` | `formatRequestMetric` | `formatNumber` |
| `billed_requests` | `formatRequestMetric` | `formatNumber` |
| `risky_requests` | `formatRequestMetric` | `formatNumber` |
| `total_tokens` | `formatTokenMetric` | `formatNumber` |
| `total_input_tokens` | `formatTokenMetric` | `formatNumber` |
| `total_output_tokens` | `formatTokenMetric` | `formatNumber` |
| `cache_read_tokens` | `formatTokenMetric` | `formatNumber` |
| `archive_coverage` | `percent` | 同 value |
| `cache_hit_rate` | `percent` | 同 value |
| `total_actual_cost` | `formatCost` | `formatCost` |
| `risky_cost` | `formatCost` | `formatCost` |

### 用户排行

| 字段 | 展示函数 | 精确 title |
| --- | --- | --- |
| `total_tokens` | `formatUserRankingTokens`（= `formatTokenMetric`） | `formatNumber` |
| `actual_cost` | `formatUserRankingCost` | `formatCost` |

### 不在本次范围

- 匹配质量
- 风险原因
- 归档文件
- 项目排行
- 请求明细
```

说明：

- 测试可以只覆盖代表字段（如 `total_tokens`、`total_requests`）
- 但规格必须写明其余字段共用同一函数，避免只改一张卡

---

## 4. 建议补进文档的内容（P1）

### 4.1 测试合同补边界

在“测试”一节追加：

```md
## 测试合同

### 代表字段边界（已有/必须保留）

- Token：`999_999 -> 999,999`；`1_000_000 -> 1.0M`；`1_000_000_000 -> 1.0B`
- 请求数：`999 -> 999`；`1_000 -> 1.0K`；`1_000_000 -> 1.0M`；`1_000_000_000 -> 1.0B`
- 用户排行费用：`999.9999 -> $999.9999`；`1_000 -> $1.0K`；`1_200_000 -> $1200.0K`
- 用户排行 Token 小于 1M 时不得出现 `0.0M`
- 紧凑值对应 `title` 必须是完整精确值

### 建议补强（若实现侧后续补测）

- 负数：`-1_200_000 -> -1.2M`；`-$1_500 -> -$1.5K`
- 非有限值：`NaN` / `Infinity` -> `0`
- 共用函数覆盖说明：
  - Token 卡代表字段测 `total_tokens` 即可，但文档声明 input/output/cache_read 共用
  - 请求卡代表字段测 `total_requests` 即可，但文档声明 billed/risky 共用
```

### 4.2 验证命令改为可执行

把“前端全量 Vitest 必过”改成分层验证：

```md
## 验证

1. 定向：
   `pnpm --dir frontend test:run src/views/admin/__tests__/TokenAnalysisView.spec.ts`
2. 共享工具未被误改：
   `pnpm --dir frontend test:run src/utils/__tests__/formatCompactNumber.spec.ts`
3. `pnpm --dir frontend run typecheck`
4. `pnpm --dir frontend run lint`（若仓库脚本存在）
5. `git diff --check`
6. 全量 Vitest 可选；若失败，必须区分：
   - 本次改动引入
   - 仓库预存红（例如历史图片用量用例）
```

### 4.3 示例与实现口径对齐

文档示例允许使用 `1.2M` 这类非边界值，但必须同时写：

- 边界值输出是 `1.0M` / `1.0K` / `1.0B`
- 紧凑单位统一保留 1 位小数

避免读者以为实现应输出不带小数的 `1M`。

---

## 5. 不要求改动的内容

以下内容当前设计/实现已对齐，无需为了“再设计一遍”而改：

1. 不修改 API、后端聚合、数据库
2. 不把规则下沉到共享 `formatCompactNumber`
3. 不把项目排行一并改成 Token 禁用 K
4. 不把概览金额卡改成紧凑格式
5. 不把用户排行费用升级到 M/B

---

## 6. 建议的文档目录（改完后）

Codex 改完后，目标文档建议接近：

```md
# Token Analysis Compact Number Format Design

## 状态
## 目标
## 修改范围
## 数值合同
### Token 指标
### 请求数指标
### 用户排行费用
## 页面映射
## 实现方式
## 防回归约束
## 已知取舍
## 测试合同
## 验证
## 非目标
```

---

## 7. Codex 执行清单

按顺序做：

1. 只读打开：
   - `docs/superpowers/specs/2026-07-13-token-analysis-compact-number-format-design.md`
   - `frontend/src/views/admin/TokenAnalysisView.vue`（`summaryCards` 与 format 函数）
   - `frontend/src/views/admin/__tests__/TokenAnalysisView.spec.ts`
2. 按本文 P0 全量改设计文档。
3. 按本文 P1 尽量补齐测试合同与验证命令。
4. 若发现文档与实现不一致：
   - 以当前实现为准修正文档
   - 在回复中列出“文档原表述 -> 修正后表述”
5. 不要改业务代码。
6. 完成后输出：
   - 改了哪些章节
   - 哪些 P0/P1 已覆盖
   - 是否仍有意保留“项目排行使用 K”的同页不一致

---

## 8. 验收标准

设计文档修改完成后，应满足：

| 检查项 | 通过标准 |
| --- | --- |
| 阈值可编码 | Token/请求数/费用阈值无需猜示例即可实现 |
| 小数位明确 | 紧凑单位统一 1 位小数 |
| 防回归明确 | 禁止误用共享 `formatNumber` / 扩展共享 compact 工具 |
| 同页取舍明确 | 项目排行继续 K/M/B 被写成已知取舍 |
| 映射可验收 | 每个概览卡字段都有函数归属 |
| 验证可执行 | 有定向测试命令，不全量 Vitest 一刀切 |

---

## 9. 参考实现锚点

改文档时以这些实现事实为准：

```ts
// TokenAnalysisView.vue
function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(Math.round(value || 0))
}

function formatTokenMetric(value: number): string {
  // abs >= 1e9 -> B; abs >= 1e6 -> M; else formatNumber
}

function formatRequestMetric(value: number): string {
  // abs >= 1e9 -> B; abs >= 1e6 -> M; abs >= 1e3 -> K; else formatNumber
}

function formatCost(value: number): string {
  return `$${Number(value || 0).toFixed(4)}`
}

function formatUserRankingCost(value: number): string {
  // abs >= 1e3 -> $x.xK; else formatCost
}
```

项目排行仍使用：

```ts
formatCompactNumber(row.total_tokens) // 共享 K/M/B
```

这与用户排行 Token 规则不同，必须在文档中保留为已知取舍。
