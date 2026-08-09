# Organization Usage Report Display Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变组织用量 API、统计口径和内部筛选键的前提下，统一页面与 XLSX 的人数/组织展示文案，并用总 Token 曲线替换趋势图的缓存 Token 曲线。

**Architecture:** 在 `organizationUsageReport.ts` 增加唯一的纯展示映射函数，页面组件与 XLSX 共同消费该函数，原始 response 和 request value 保持不变。人数标题继续复用现有 i18n keys，仅替换中英文值；趋势组件继续使用现有四数据集、双轴和快照状态机，只替换第三个数据集的 label 与数据来源。

**Tech Stack:** Vue 3、TypeScript、vue-i18n、Chart.js、Vue Test Utils、Vitest、SheetJS、pnpm 9、PowerShell / `cmd.exe`。

## Global Constraints

- 工作分支固定为 `feature/hy/10174_org_usage_report_display`。
- `active_users` 数值与计算不变，只显示为“注册人数 / Registered users”。
- `used_users` 数值与计算不变，只显示为“活跃人数 / Active users”。
- 组织筛选值继续使用 `all|xunyou|wsdashi|other`；不得修改 API DTO、后端、SQL 或数据库。
- `xunyou|xunyou.com` 显示为“迅游”，`wsdashi|wsdashi.com` 显示为“速宝”，未知值回落调用方传入的“其他 / Other”。
- 趋势数据集顺序固定为 input、output、total、requests；`total_tokens` 仍包含 input、output、cache creation、cache read。
- 缓存创建/读取字段继续保留在 API 与 XLSX 明细中；人员表只保留既有的缓存读取字段；两类字段均从趋势折线移除。
- 页面与 XLSX 必须同步；不修改 Sheet 名、分页、导出 Worker、`as_of`、粒度或补零逻辑。
- 未经用户进一步授权，不 push、不创建 PR、不合并。

---

## 文件结构

| 责任 | 文件 |
| --- | --- |
| 唯一组织展示映射与 XLSX | `frontend/src/utils/organizationUsageReport.ts` |
| 映射与日期工具测试 | `frontend/src/utils/__tests__/organizationUsageReport.spec.ts` |
| XLSX 合同测试 | `frontend/src/utils/__tests__/organizationUsageWorkbook.spec.ts` |
| 页面组织显示 | `frontend/src/components/admin/organization-usage/OrganizationUsageFilters.vue`、`OrganizationUsageSummary.vue`、`OrganizationUsagePeopleTable.vue` |
| 页面回归测试 | `frontend/src/components/admin/organization-usage/__tests__/OrganizationUsageFilters.spec.ts`、`frontend/src/views/admin/__tests__/OrganizationUsageView.spec.ts` |
| 人数文案 | `frontend/src/i18n/locales/{zh,en}/admin/organizationUsage.ts` |
| 双语文案测试 | `frontend/src/i18n/__tests__/organizationUsageLocale.spec.ts` |
| 趋势曲线 | `frontend/src/components/admin/organization-usage/OrganizationUsageTrendChart.vue` 及其专项测试 |
| 稳定知识 | 组件 README、两份 organization usage feature 设计、`llm-wiki/wiki/frontend.md` |

## 规格覆盖

| 成功标准 | 实施任务 | 最终证据 |
| --- | --- | --- |
| SC-1 用量概览人数标题 | Task 3 | 双语 locale 精确断言 + View 回归 |
| SC-2 组织汇总人数标题 | Task 3 | 两个组件共用同一 locale keys + View 回归 |
| SC-3 总 Token 替换缓存曲线 | Task 4 | dataset label/data/axis 专项断言 |
| SC-4 页面组织显示名 | Task 1 | helper、Filters、Summary、People 与请求键断言 |
| SC-5 XLSX 同步 | Task 2 | Champion、组织、人员、月/周/日逐项断言 |
| SC-6 内部合同不变 | Tasks 1、6 | `xunyou` 请求断言、无 backend/API diff、typecheck |

### Task 1: 建立唯一组织显示映射并接入页面

**Files:**
- Modify: `frontend/src/utils/__tests__/organizationUsageReport.spec.ts`
- Modify: `frontend/src/components/admin/organization-usage/__tests__/OrganizationUsageFilters.spec.ts`
- Modify: `frontend/src/views/admin/__tests__/OrganizationUsageView.spec.ts`
- Modify: `frontend/src/utils/organizationUsageReport.ts`
- Modify: `frontend/src/components/admin/organization-usage/OrganizationUsageFilters.vue`
- Modify: `frontend/src/components/admin/organization-usage/OrganizationUsageSummary.vue`
- Modify: `frontend/src/components/admin/organization-usage/OrganizationUsagePeopleTable.vue`

**Interfaces:**
- Consumes: `OrganizationUsageOrganizationFilter` 的现有 `all|xunyou|wsdashi|other` 值。
- Produces: `formatOrganizationUsageOrganization(value: string, otherLabel?: string): string`，供页面和 Task 2 的 XLSX 使用。

- [ ] **Step 1: 为纯映射函数写可收集的失败测试**

在 `organizationUsageReport.spec.ts` 增加 namespace import。通过可选函数类型读取尚不存在的 export，先断言它必须是函数；缺失时测试在行为断言处失败，不产生模块收集错误：

```ts
import * as organizationUsageReport from '@/utils/organizationUsageReport'

it.each([
  ['xunyou', undefined, '迅游'],
  ['xunyou.com', undefined, '迅游'],
  ['wsdashi', undefined, '速宝'],
  ['wsdashi.com', undefined, '速宝'],
  ['other', undefined, '其他'],
  ['unknown', 'Other', 'Other']
] as const)('formats organization %s for display', (value, otherLabel, expected) => {
  const formatter = (organizationUsageReport as unknown as {
    formatOrganizationUsageOrganization?: (organization: string, fallback?: string) => string
  }).formatOrganizationUsageOrganization
  expect(formatter).toBeTypeOf('function')
  if (!formatter) return
  expect(formatter(value, otherLabel)).toBe(expected)
})
```

- [ ] **Step 2: 为筛选器、组织汇总和人员表写失败断言**

在 `OrganizationUsageFilters.spec.ts` 引入 `Select`，断言 option 的 label 改变而 value 不变：

```ts
import Select from '@/components/common/Select.vue'

it('shows brand names while preserving organization filter values', () => {
  const { wrapper } = mountFilters(baseDraft)
  expect(wrapper.getComponent(Select).props('options')).toEqual([
    { value: 'all', label: 'admin.organizationUsage.organizations.all' },
    { value: 'xunyou', label: '迅游' },
    { value: 'wsdashi', label: '速宝' },
    { value: 'other', label: 'admin.organizationUsage.organizations.other' }
  ])
})
```

扩展 `OrganizationUsageView.spec.ts` 中现有的组织行测试：

```ts
const xunyouButton = wrapper.get('[data-organization="xunyou"]')
const wsdashiButton = wrapper.get('[data-organization="wsdashi"]')
expect(xunyouButton.text()).toBe('迅游')
expect(wsdashiButton.text()).toBe('速宝')

const personEmail = wrapper.get('tbody [title="alice@xunyou.com"]')
expect(personEmail.element.closest('tr')?.querySelector('td')?.textContent?.trim()).toBe('迅游')

await xunyouButton.trigger('click')
await flushPromises()
expect(getSummary).toHaveBeenLastCalledWith(
  expect.objectContaining({ organization: 'xunyou', page: 1 }),
  { signal: expect.any(AbortSignal) }
)
```

- [ ] **Step 3: 运行 RED 验证**

Run:

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/utils/__tests__/organizationUsageReport.spec.ts src/components/admin/organization-usage/__tests__/OrganizationUsageFilters.spec.ts src/views/admin/__tests__/OrganizationUsageView.spec.ts
```

Expected: FAIL；工具测试在 `expected undefined to be type of 'function'` 断言处失败，组件断言仍看到 `xunyou.com` / `wsdashi.com`。不得出现模块收集、named export 或 TypeScript 语法错误。

- [ ] **Step 4: 写最小展示映射**

在 `organizationUsageReport.ts` 增加：

```ts
export function formatOrganizationUsageOrganization(value: string, otherLabel = '其他'): string {
  if (value === 'xunyou' || value === 'xunyou.com') return '迅游'
  if (value === 'wsdashi' || value === 'wsdashi.com') return '速宝'
  return otherLabel
}
```

三个组件都从该工具模块导入函数：

```ts
import { formatOrganizationUsageOrganization } from '@/utils/organizationUsageReport'
```

筛选器使用：

```ts
const organizationOptions = computed(() => ([
  { value: 'all', label: t('admin.organizationUsage.organizations.all') },
  { value: 'xunyou', label: formatOrganizationUsageOrganization('xunyou') },
  { value: 'wsdashi', label: formatOrganizationUsageOrganization('wsdashi') },
  { value: 'other', label: t('admin.organizationUsage.organizations.other') }
]))
```

组织汇总使用：

```ts
label: formatOrganizationUsageOrganization(
  filter,
  t('admin.organizationUsage.organizations.other')
)
```

人员表删除本地品牌判断，保留一个本地化回落包装：

```ts
function organizationLabel(value: string) {
  return formatOrganizationUsageOrganization(
    value,
    t('admin.organizationUsage.organizations.other')
  )
}
```

- [ ] **Step 5: 运行 GREEN 验证**

Run: 与 Step 3 相同。

Expected: PASS；现有组织行点击测试仍证明请求发送 `xunyou`。

- [ ] **Step 6: 提交本任务**

```powershell
git add frontend/src/utils/organizationUsageReport.ts frontend/src/utils/__tests__/organizationUsageReport.spec.ts frontend/src/components/admin/organization-usage/OrganizationUsageFilters.vue frontend/src/components/admin/organization-usage/OrganizationUsageSummary.vue frontend/src/components/admin/organization-usage/OrganizationUsagePeopleTable.vue frontend/src/components/admin/organization-usage/__tests__/OrganizationUsageFilters.spec.ts frontend/src/views/admin/__tests__/OrganizationUsageView.spec.ts
git commit -m "feat(frontend): centralize organization usage display names"
```

### Task 2: 将组织显示名和人数标题同步到 XLSX

**Files:**
- Modify: `frontend/src/utils/__tests__/organizationUsageWorkbook.spec.ts`
- Modify: `frontend/src/utils/organizationUsageReport.ts`

**Interfaces:**
- Consumes: Task 1 的 `formatOrganizationUsageOrganization(value, otherLabel)`。
- Produces: 六个 Sheet 结构不变、展示文案已转换的 workbook。

- [ ] **Step 1: 写覆盖全部 XLSX 组织列的失败测试**

在 `organizationUsageWorkbook.spec.ts` 增加一个测试，构造两种键形式并逐 Sheet 断言：

```ts
it('uses display names and headcount labels across every workbook organization column', () => {
  const xunyouPeriod = period({ organization: 'xunyou' })
  const wsdashiPeriod = period({ organization: 'wsdashi.com' })
  const summary: OrganizationUsageSummaryResponse = {
    ...emptySummary,
    overview: { ...zeroMetrics, active_users: 2, used_users: 1 },
    organizations: [
      { ...zeroMetrics, organization: 'xunyou', active_users: 1, used_users: 1 },
      { ...zeroMetrics, organization: 'wsdashi', active_users: 1, used_users: 0 }
    ],
    champions: { day: wsdashiPeriod, week: null, month: null },
    items: [{
      ...zeroMetrics,
      user_id: 7,
      email: 'alice@xunyou.com',
      organization: 'xunyou.com',
      peak_day: null,
      peak_week: null,
      peak_month: null
    }]
  }
  const workbook = roundTrip(buildOrganizationUsageWorkbook({
    summary,
    periods: { month: [xunyouPeriod], week: [wsdashiPeriod], day: [xunyouPeriod] }
  }))

  const overviewRows = rows(workbook, '报表概览')
  expect(overviewRows.find((row) => row[0] === '注册人数')?.slice(0, 2)).toEqual(['注册人数', 2])
  expect(overviewRows.find((row) => row[0] === '活跃人数')?.slice(0, 2)).toEqual(['活跃人数', 1])
  expect(overviewRows.some((row) => row[0] === '总活跃人数' || row[0] === '有用量人数')).toBe(false)
  expect(overviewRows.find((row) => row[0] === '日度 Champion')?.[6]).toBe('速宝')

  const organizationRows = rows(workbook, '组织汇总')
  expect(organizationRows[0]?.slice(0, 3)).toEqual(['组织', '注册人数', '活跃人数'])
  expect(organizationRows.slice(1).map((row) => row[0])).toEqual(['迅游', '速宝'])
  expect(rows(workbook, '人员汇总')[1]?.[2]).toBe('迅游')

  for (const [sheet, expected] of [
    ['月度明细', '迅游'],
    ['周度明细', '速宝'],
    ['日度明细', '迅游']
  ] as const) {
    const detailRows = rows(workbook, sheet)
    const headers = detailRows[0] as string[]
    expect(detailRows[1]?.[headers.indexOf('组织')]).toBe(expected)
  }
})
```

同时把现有 Champion `other` 期望从原始键改为“其他”，证明未知/other 回落也被导出覆盖。

- [ ] **Step 2: 运行 RED 验证**

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/utils/__tests__/organizationUsageWorkbook.spec.ts
```

Expected: FAIL；旧 workbook 仍输出旧人数标题和内部组织键。

- [ ] **Step 3: 在四类 XLSX 行中复用唯一映射**

在 `organizationUsageReport.ts` 做以下精确替换：

```ts
// championRow
formatOrganizationUsageOrganization(champion.organization),

// buildOverviewRows
['注册人数', overview.active_users, '', '', '', '', '', ''],
['活跃人数', overview.used_users, '', '', '', '', '', ''],

// buildOrganizationRows header and organization cell
['组织', '注册人数', '活跃人数', ...METRIC_HEADERS],
formatOrganizationUsageOrganization(organization.organization),

// buildPeopleRows
formatOrganizationUsageOrganization(item.organization),

// buildPeriodRows
formatOrganizationUsageOrganization(period.organization),
```

- [ ] **Step 4: 运行 GREEN 验证**

Run: 与 Step 2 相同。

Expected: PASS；六个 Sheet 的数量、名称、数值类型和行上限测试继续通过。

- [ ] **Step 5: 提交本任务**

```powershell
git add frontend/src/utils/organizationUsageReport.ts frontend/src/utils/__tests__/organizationUsageWorkbook.spec.ts
git commit -m "feat(frontend): align organization usage workbook labels"
```

### Task 3: 更新人数双语文案

**Files:**
- Modify: `frontend/src/i18n/__tests__/organizationUsageLocale.spec.ts`
- Modify: `frontend/src/i18n/locales/zh/admin/organizationUsage.ts`
- Modify: `frontend/src/i18n/locales/en/admin/organizationUsage.ts`

**Interfaces:**
- Consumes: Overview 和 Organization Summary 已使用的 `metrics.activeUsers` / `metrics.usedUsers` keys。
- Produces: 中文“注册人数/活跃人数”和英文“Registered users/Active users”。

- [ ] **Step 1: 写双语失败测试**

在 `organizationUsageLocale.spec.ts` 增加：

```ts
it.each([
  ['zh', zh, '注册人数', '活跃人数'],
  ['en', en, 'Registered users', 'Active users']
] as const)('uses the requested headcount labels in %s', (_locale, messages, registered, active) => {
  expect(messages.admin.organizationUsage.metrics.activeUsers).toBe(registered)
  expect(messages.admin.organizationUsage.metrics.usedUsers).toBe(active)
})
```

- [ ] **Step 2: 运行 RED 验证**

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/i18n/__tests__/organizationUsageLocale.spec.ts
```

Expected: FAIL；locale 仍返回“活跃人数/有用量人数”和“Active users/Users with usage”。

- [ ] **Step 3: 最小修改 locale 值**

中文：

```ts
activeUsers: '注册人数',
usedUsers: '活跃人数',
```

英文：

```ts
activeUsers: 'Registered users',
usedUsers: 'Active users',
```

不得改 key 名，Overview 与 Organization Summary 会自动同步。

- [ ] **Step 4: 运行 GREEN 与组件回归**

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/i18n/__tests__/organizationUsageLocale.spec.ts src/views/admin/__tests__/OrganizationUsageView.spec.ts
```

Expected: PASS。

- [ ] **Step 5: 提交本任务**

```powershell
git add frontend/src/i18n/__tests__/organizationUsageLocale.spec.ts frontend/src/i18n/locales/zh/admin/organizationUsage.ts frontend/src/i18n/locales/en/admin/organizationUsage.ts
git commit -m "feat(frontend): rename organization usage headcounts"
```

### Task 4: 用总 Token 替换趋势缓存 Token 曲线

**Files:**
- Modify: `frontend/src/components/admin/organization-usage/__tests__/OrganizationUsageTrendChart.spec.ts`
- Modify: `frontend/src/components/admin/organization-usage/OrganizationUsageTrendChart.vue`

**Interfaces:**
- Consumes: `OrganizationUsageTrendPoint.total_tokens`，已由现有 API 返回。
- Produces: input/output/total/requests 四数据集；轴、tooltip、粒度、loading/error 和 dense-point 行为保持不变。

- [ ] **Step 1: 强化现有四曲线测试，使 total 与 cache 数值可区分**

把现有 `exposes four default series...` 测试中的 datasets 类型和断言扩展为：

```ts
const datasets = exposed.chartData.datasets as Array<{
  label: string
  data: number[]
  yAxisID: string
  cubicInterpolationMode: string
}>

expect(datasets.map((dataset) => dataset.label)).toEqual([
  'admin.organizationUsage.metrics.inputTokens',
  'admin.organizationUsage.metrics.outputTokens',
  'admin.organizationUsage.metrics.totalTokens',
  'admin.organizationUsage.metrics.requests'
])
expect(datasets.map((dataset) => dataset.data)).toEqual([[10], [20], [36], [3]])
expect(datasets.some((dataset) => dataset.label === 'admin.organizationUsage.metrics.cacheTokens')).toBe(false)
expect(datasets.map((dataset) => dataset.yAxisID)).toEqual(['y', 'y', 'y', 'yRequests'])
```

fixture 已有 `cache_read_tokens: 5` 和 `total_tokens: 36`，可直接证明第三条线不再读取缓存值。

- [ ] **Step 2: 运行 RED 验证**

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/components/admin/organization-usage/__tests__/OrganizationUsageTrendChart.spec.ts
```

Expected: FAIL；第三个数据集仍是 cache label 和 `[5]`。

- [ ] **Step 3: 最小替换第三个数据集**

在 `OrganizationUsageTrendChart.vue` 中把颜色 key 改为 `total`，保留当前 cyan 色值：

```ts
const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb',
  input: '#3b82f6',
  output: '#10b981',
  total: '#06b6d4',
  requests: '#8b5cf6'
}))
```

第三个 dataset 改为：

```ts
{
  label: t('admin.organizationUsage.metrics.totalTokens'),
  data: props.points.map((point) => point.total_tokens),
  borderColor: colors.total,
  backgroundColor: `${colors.total}20`,
  fill: false,
  yAxisID: 'y',
  cubicInterpolationMode: 'monotone' as const,
  pointRadius: radius,
  pointHoverRadius: 4,
  tension: 0.3
},
```

- [ ] **Step 4: 运行 GREEN 验证**

Run: 与 Step 2 相同。

Expected: PASS；loading/error/tooltip/dense-point 测试也继续通过。

- [ ] **Step 5: 提交本任务**

```powershell
git add frontend/src/components/admin/organization-usage/OrganizationUsageTrendChart.vue frontend/src/components/admin/organization-usage/__tests__/OrganizationUsageTrendChart.spec.ts
git commit -m "feat(frontend): show total tokens in organization trend"
```

### Task 5: 同步组件说明、feature 设计和 llm-wiki

**Files:**
- Modify: `frontend/src/components/admin/organization-usage/README.md`
- Modify: `docs/features/organization-usage-report-design-cn.md`
- Modify: `docs/features/organization-usage-trend-chart-design-cn.md`
- Modify: `llm-wiki/wiki/frontend.md`

**Interfaces:**
- Consumes: Tasks 1-4 的最终展示合同。
- Produces: 后续 AI 可直接读取、与源码一致的稳定知识。

- [ ] **Step 1: 更新组件 README**

把趋势默认系列改为“输入/输出/总 Token + 请求数”；在 Filters/Summary/People 描述中写明 `xunyou/wsdashi` 内部键分别显示“迅游/速宝”，筛选值不变；写明人数 labels 与字段映射。

- [ ] **Step 2: 更新原报表设计**

在 `organization-usage-report-design-cn.md` 保留 `active_users` / `used_users` 的真实后端语义，并补充前端标签：

```markdown
前端展示中，`active_users` 标为“注册人数”，`used_users` 标为“活跃人数”；这只是业务展示名，不改变上述统计条件。
```

组织归属表的页面显示改为“迅游/速宝/其他”，API 值不变。

- [ ] **Step 3: 更新趋势设计**

将目标、K8、前端 dataset 表和验收标准中的默认系列统一改为 requests + input + output + total；明确 `total_tokens` 包含两类缓存 Token，因此不等于另外两条可见 Token 曲线之和。保留 API 返回 cache creation/read 的说明和后端聚合字段。

- [ ] **Step 4: 更新 frontend wiki**

在组织用量段落记录：人数展示别名、组织显示映射、趋势默认 input/output/total/requests，内部键和 API 字段不变。不要修改后端/domain wiki 中的真实统计定义。

- [ ] **Step 5: 扫描过期展示合同**

```powershell
rg -n '默认系列.*缓存 Token|默认系列.*cache_read|requests \+ input \+ output \+ cache_read|默认展示指标.*cache_read' docs/features/organization-usage-trend-chart-design-cn.md frontend/src/components/admin/organization-usage/README.md llm-wiki/wiki/frontend.md
```

Expected: 无过期默认系列描述。

```powershell
rg -n '\| 精确等于 `xunyou\.com` \| `xunyou` \| `xunyou\.com` \||\| 精确等于 `wsdashi\.com` \| `wsdashi` \| `wsdashi\.com` \|' docs/features/organization-usage-report-design-cn.md
```

Expected: 无旧页面显示域名表格行。

- [ ] **Step 6: 提交本任务**

```powershell
git add frontend/src/components/admin/organization-usage/README.md docs/features/organization-usage-report-design-cn.md docs/features/organization-usage-trend-chart-design-cn.md llm-wiki/wiki/frontend.md
git commit -m "docs: align organization usage display contract"
```

### Task 6: 审查、完整验证与交付记录

**Files:**
- Modify: `docs/delivery/2026-08-09-organization-usage-report-display/plan.md`
- Create: `docs/delivery/2026-08-09-organization-usage-report-display/test-review.md`
- Create: `docs/delivery/2026-08-09-organization-usage-report-display/delivery-report.md`
- Modify: `docs/delivery/2026-08-09-organization-usage-report-display/delivery-status.md`
- Verify: Tasks 1-5 的全部文件。

**Interfaces:**
- Consumes: 六项成功标准 SC-1 至 SC-6 和 Tasks 1-5 的提交。
- Produces: 当前状态的测试证据、review findings 与最终交付说明。

- [ ] **Step 1: 运行专项回归矩阵**

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/utils/__tests__/organizationUsageReport.spec.ts src/utils/__tests__/organizationUsageWorkbook.spec.ts src/components/admin/organization-usage/__tests__/OrganizationUsageFilters.spec.ts src/components/admin/organization-usage/__tests__/OrganizationUsageTrendChart.spec.ts src/i18n/__tests__/organizationUsageLocale.spec.ts src/views/admin/__tests__/OrganizationUsageView.spec.ts
```

Expected: 全部 PASS；记录文件数和测试数。

- [ ] **Step 2: 运行完整前端测试**

```powershell
cmd.exe /c pnpm --dir frontend run test:run
```

Expected: 全部 PASS；若存在基线失败，必须用独立复跑和 `github/main` 对照区分，不能写成通过。

- [ ] **Step 3: 运行静态验证**

```powershell
cmd.exe /c pnpm --dir frontend run typecheck
cmd.exe /c pnpm --dir frontend run lint:check
```

Expected: 两条命令均 exit 0。

- [ ] **Step 4: 执行规格符合性和代码质量审查**

逐项核对 SC-1 至 SC-6；确认没有后端或 API 类型改动、没有重复组织映射、没有把显示名写回请求参数。代码质量审查重点检查未知组织回落、XLSX 六 Sheet、Chart dataset 轴与 tooltip、双语 locale、现有 `as_of` 状态机未被触碰。 findings 按严重度写入 `test-review.md`；无 findings 时明确记录“无问题”。

- [ ] **Step 5: 检查差异与范围**

```powershell
git diff --check github/main...HEAD
git diff --name-only github/main...HEAD
git status --short --branch
```

Expected: diff check 无输出；变更仅落在本计划列出的前端、测试和文档路径；工作区无未提交文件。

额外确认后端没有变化：

```powershell
git diff --name-only github/main...HEAD -- backend frontend/src/api/admin/organizationUsage.ts
```

Expected: 无输出。

- [ ] **Step 6: 启动前端并做桌面/移动端可视检查**

```powershell
cmd.exe /c pnpm --dir frontend run dev -- --host 127.0.0.1 --port 5174
```

保持 dev server 运行，检查 `/admin/organization-usage` 在桌面与移动 viewport：人数标题不截断、组织中文名不改变筛选行为、图例显示总 Token 且无缓存 Token、表格横向滚动不产生重叠。若本机没有可用管理员会话或后端数据，明确记录视觉检查受阻，但仍提供 `http://127.0.0.1:5174/` 给用户试用。

- [ ] **Step 7: 完成交付文档并提交**

把实际命令、结果、review findings、视觉验证和残余风险写入 `test-review.md` / `delivery-report.md`，将 `plan.md` 和 `delivery-status.md` 的已完成项更新为“已完成”。

```powershell
git add -f docs/delivery/2026-08-09-organization-usage-report-display
git commit -m "docs: finalize organization usage report delivery"
```

不得 push、创建 PR 或合并。
