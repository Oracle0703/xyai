# Token Analysis Compact Number Format Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 按已确认的 M/B、K/M/B 和金额 K 规则优化 Token 分析页面，并通过 `title` 保留精确值。

**Architecture:** 仅在 `TokenAnalysisView.vue` 内保留页面专用纯格式化函数，不改变 API、后端或共享 formatter。概览卡在计算属性中同时生成紧凑展示值和完整 `title`，用户排行单元格分别绑定紧凑值与精确 `title`。

**Tech Stack:** Vue 3、TypeScript、Vue Test Utils、Vitest、pnpm

**Status:** 已实现；本计划按 2026-07-14 审核结论同步最终实施合同和验证结果。

---

### Task 1: 用页面测试锁定数值展示合同

**Files:**
- Modify: `frontend/src/views/admin/__tests__/TokenAnalysisView.spec.ts`
- Test: `frontend/src/views/admin/__tests__/TokenAnalysisView.spec.ts`

- [x] **Step 1: 提取稳定的概览数据 fixture**

新增 `summaryFixture`，让每个边界测试只覆盖目标字段，避免重复构造完整 API 响应：

```ts
function summaryFixture(overrides: Record<string, unknown> = {}) {
  return {
    total_requests: 12,
    total_input_tokens: 5000,
    total_output_tokens: 3000,
    total_tokens: 9000,
    total_actual_cost: 1.23,
    cache_read_tokens: 4000,
    cache_hit_rate: 0.4,
    risky_requests: 2,
    risky_cost: 0.6,
    billed_requests: 48,
    archive_coverage: 0.421,
    ...overrides
  }
}
```

- [x] **Step 2: 增加概览卡代表字段边界测试**

Token 代表字段 `total_tokens` 覆盖：

```ts
it.each([
  [999_999, '999,999'],
  [1_000_000, '1.0M'],
  [1_000_000_000, '1.0B']
])('formats total Token value %s with M/B only and preserves its exact title', async (value, expected) => {
  api.getSummary.mockResolvedValue(summaryFixture({ total_tokens: value }))

  const wrapper = mount(TokenAnalysisView, {
    global: { stubs: { AppLayout: AppLayoutStub } }
  })
  await flushPromises()

  const card = wrapper.findAll('.card').find((item) => item.text().includes('admin.tokenAnalysis.summary.totalTokens'))
  const valueElement = card!.find('.mt-2')
  expect(valueElement.text()).toBe(expected)
  expect(valueElement.attributes('title')).toBe(new Intl.NumberFormat().format(value))
})
```

请求数代表字段 `total_requests` 使用同样的挂载方式，并验证完整 `title`：

```ts
it.each([
  [999, '999'],
  [1_000, '1.0K'],
  [1_000_000, '1.0M'],
  [1_000_000_000, '1.0B']
])('formats total request value %s with K/M/B and preserves its exact title', async (value, expected) => {
  api.getSummary.mockResolvedValue(summaryFixture({ total_requests: value }))

  const wrapper = mount(TokenAnalysisView, {
    global: { stubs: { AppLayout: AppLayoutStub } }
  })
  await flushPromises()

  const card = wrapper.findAll('.card').find((item) => item.text().includes('admin.tokenAnalysis.summary.totalRequests'))
  const valueElement = card!.find('.mt-2')
  expect(valueElement.text()).toBe(expected)
  expect(valueElement.attributes('title')).toBe(new Intl.NumberFormat().format(value))
})
```

- [x] **Step 3: 增加用户排行 Token、费用和精确 title 测试**

```ts
expect(userRankingCard!.text()).toContain('1,200')
expect(userRankingCard!.text()).not.toContain('0.0M')
expect(userRankingCard!.text()).toContain('1.0M')
expect(userRankingCard!.text()).toContain('1.0B')
expect(userRankingCard!.text()).toContain('$999.9999')
expect(userRankingCard!.text()).toContain('$1.0K')
expect(userRankingCard!.text()).toContain('$1200.0K')
expect(userRankingCard!.find('[title="1,200"]').exists()).toBe(true)
expect(userRankingCard!.find('[title="$1000.0000"]').exists()).toBe(true)
expect(userRankingCard!.find('[title="$1200000.0000"]').exists()).toBe(true)
```

- [x] **Step 4: 确认测试在实现前为 RED**

Run:

```powershell
pnpm --dir frontend test:run src/views/admin/__tests__/TokenAnalysisView.spec.ts
```

Expected before implementation: FAIL；旧实现会把 `1,200` Token 显示为 `0.0M`，把所有用户费用除以 1000，且概览卡没有紧凑值和精确 `title`。

### Task 2: 实现页面专用紧凑格式化

**Files:**
- Modify: `frontend/src/views/admin/TokenAnalysisView.vue`
- Test: `frontend/src/views/admin/__tests__/TokenAnalysisView.spec.ts`

- [x] **Step 1: 新增运行时数值归一化与专用 formatter**

```ts
function finiteNumber(value: number): number {
  const number = Number(value || 0)
  return Number.isFinite(number) ? number : 0
}

function formatTokenMetric(value: number): string {
  const number = finiteNumber(value)
  const abs = Math.abs(number)
  if (abs >= 1_000_000_000) return `${(number / 1_000_000_000).toFixed(1)}B`
  if (abs >= 1_000_000) return `${(number / 1_000_000).toFixed(1)}M`
  return formatNumber(number)
}

function formatRequestMetric(value: number): string {
  const number = finiteNumber(value)
  const abs = Math.abs(number)
  if (abs >= 1_000_000_000) return `${(number / 1_000_000_000).toFixed(1)}B`
  if (abs >= 1_000_000) return `${(number / 1_000_000).toFixed(1)}M`
  if (abs >= 1_000) return `${(number / 1_000).toFixed(1)}K`
  return formatNumber(number)
}
```

单位选择按未舍入的绝对值完成，随后使用 `toFixed(1)`。因此 Token `999_999_999` 保持 `1000.0M`，请求数 `999_999` 保持 `1000.0K`，不会因显示舍入提前晋级。

- [x] **Step 2: 收口用户排行 Token 和费用规则**

```ts
function formatUserRankingTokens(value: number): string {
  return formatTokenMetric(value)
}

function formatUserRankingCost(value: number): string {
  const number = finiteNumber(value)
  if (Math.abs(number) >= 1_000) return `$${(number / 1_000).toFixed(1)}K`
  return formatCost(number)
}
```

费用达到 1000 后始终使用 K，不晋级 M/B。负费用不是合法业务输入；当前异常值格式为 `$-1.5K`，计划不增加 `-$1.5K` 断言。

- [x] **Step 3: 映射全部概览卡并提供精确 title**

请求类字段 `total_requests`、`billed_requests`、`risky_requests` 使用 `formatRequestMetric`；Token 类字段 `total_tokens`、`total_input_tokens`、`total_output_tokens`、`cache_read_tokens` 使用 `formatTokenMetric`；百分比和概览金额保持原展示函数。

```vue
<div class="mt-2 truncate text-xl font-semibold text-gray-900 dark:text-white" :title="card.title">
  {{ card.value }}
</div>
```

- [x] **Step 4: 为用户排行增加精确 title**

```vue
<td class="tabular-nums" :title="formatNumber(user.total_tokens)">
  {{ formatUserRankingTokens(user.total_tokens) }}
</td>
<td class="tabular-nums" :title="formatCost(user.actual_cost)">
  {{ formatUserRankingCost(user.actual_cost) }}
</td>
```

- [x] **Step 5: 运行定向测试并确认 GREEN**

Run:

```powershell
pnpm --dir frontend test:run src/views/admin/__tests__/TokenAnalysisView.spec.ts
```

Expected: PASS。2026-07-14 当前页面测试结果为 `24/24`；其中新增的选中用户趋势用例与本任务共享同一测试文件，紧凑数字边界用例继续通过。

### Task 3: 固化设计和防回归边界

**Files:**
- Modify: `docs/superpowers/specs/2026-07-13-token-analysis-compact-number-format-design.md`
- Modify: `docs/superpowers/plans/2026-07-13-token-analysis-compact-number-format.md`
- Verify: `frontend/src/utils/format.ts`

- [x] **Step 1: 写明阈值、小数位和来源优先级**

设计规格必须明确：

- Token 使用 M/B，不使用 K。
- 请求数使用 K/M/B。
- 用户排行费用小于 1000 保留 4 位，达到 1000 后始终使用 K。
- 紧凑值统一保留 1 位小数，单位选择发生在舍入前。
- 发生冲突时按“用户最新确认合同 -> 规格 -> 实现”处理。

- [x] **Step 2: 写明页面本地 formatter 边界**

精确值继续使用页面本地 `formatNumber`、`formatCost`。禁止切换到共享 `@/utils/format#formatNumber`，因为共享函数在 `abs >= 10_000` 时启用 locale compact，中文环境可能输出“万/亿”。不扩展共享 `formatCompactNumber` 承载本页的专属规则。

- [x] **Step 3: 记录有意保留的同页差异**

项目排行继续使用共享 `formatCompactNumber` 的 K/M/B；用户排行 Token 继续禁用 K。概览金额卡保持 4 位小数，用户排行超大费用继续显示如 `$1200.0K`。

- [x] **Step 4: 收窄异常输入合同**

API 合法输入必须是非负有限 JSON 数值。紧凑 formatter 防御性地将非有限值归零，但 `NaN`、`Infinity` 不作为精确 `title` 的验收输入；不把当前代码尚未支持的行为写成已实现事实。

### Task 4: 回归验证与知识库判断

**Files:**
- Verify: `frontend/src/views/admin/TokenAnalysisView.vue`
- Verify: `frontend/src/views/admin/__tests__/TokenAnalysisView.spec.ts`
- Verify: `frontend/src/utils/__tests__/formatCompactNumber.spec.ts`
- Verify: `llm-wiki/wiki/frontend.md`

- [x] **Step 1: 运行页面和共享 formatter 定向测试**

```powershell
pnpm --dir frontend test:run src/views/admin/__tests__/TokenAnalysisView.spec.ts src/utils/__tests__/formatCompactNumber.spec.ts
```

Expected: exit code 0。2026-07-14 当前复核结果为 `2` 个测试文件、`27/27` 通过（Token Analysis 页面 `24/24`，共享 formatter `3/3`）。

- [x] **Step 2: 运行前端全量测试**

```powershell
pnpm --dir frontend run test:run
```

Expected: exit code 0。2026-07-14 当前复核结果为 `167` 个测试文件、`1085/1085` 通过。全量 Vitest 是交付必检项，不降级为可选。

- [x] **Step 3: 运行类型和只读 lint 检查**

```powershell
pnpm --dir frontend run typecheck
pnpm --dir frontend run lint:check
```

Expected: 两条命令均 exit code 0。不得使用包含 `--fix` 的 `pnpm --dir frontend run lint` 作为只读验证命令。

- [x] **Step 4: 检查补丁格式和修改范围**

```powershell
git diff --check
git diff -- frontend/src/views/admin/TokenAnalysisView.vue frontend/src/views/admin/__tests__/TokenAnalysisView.spec.ts docs/superpowers/specs/2026-07-13-token-analysis-compact-number-format-design.md docs/superpowers/plans/2026-07-13-token-analysis-compact-number-format.md
```

Expected: `git diff --check` 无输出；代码差异仅影响约定页面及测试，文档差异仅为设计规格和计划。本轮不创建提交、不推送。

- [x] **Step 5: 判断 llm-wiki 是否需要更新**

该改动仅为局部展示规则，不改变路由、组件边界、API 或数据流，按 `AGENTS.md` 不更新 `llm-wiki`。若后续发现 wiki 与源码冲突，再只修正文档事实。
