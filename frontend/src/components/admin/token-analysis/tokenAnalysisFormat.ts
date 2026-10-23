import { useI18n } from 'vue-i18n'

import type { TokenAnalysisIndexStatus } from '@/api/admin/tokenAnalysis'
import { formatCompactNumber } from '@/utils/format'

// 风险说明顺序与 ScoreTokenAnalysisRisk 中的扣分权重一致。
export const TOKEN_ANALYSIS_RISK_CODES = [
  'huge_input_tiny_output',
  'repeat_uncached_body',
  'low_cache_hit_large_input',
  'rapid_similar_requests',
  'oversized_system_prompt',
  'tool_heavy_short_output',
  'large_tool_history',
  'giant_tool_output'
] as const

export const MAX_SELECTED_TREND_USERS = 5
export const PROJECT_OUTPUT_INPUT_HEALTHY_RATIO = 0.05

// 用户趋势分类色(按选择时分配的槽位固定, 取消其他用户不重排颜色)。
// 已用 dataviz validate_palette 校验: 亮色在白色卡片、暗色在 dark-800 上均通过;
// 不含红色, 避免与本页风险语义混淆。
export const USER_TREND_COLORS_LIGHT = ['#3b82f6', '#10b981', '#f59e0b', '#8b5cf6', '#ec4899']
export const USER_TREND_COLORS_DARK = ['#3b82f6', '#059669', '#d97706', '#8b5cf6', '#ec4899']
// 与上面同序的 Tailwind 类, 供图表外的色点/标签复用。
export const USER_TREND_DOT_CLASSES = [
  'bg-blue-500',
  'bg-emerald-500 dark:bg-emerald-600',
  'bg-amber-500 dark:bg-amber-600',
  'bg-violet-500',
  'bg-pink-500'
]

export type TokenAnalysisTrendGranularity = 'day' | 'hour'
export type TokenAnalysisSelectedUser = { user_id: number; email: string; slot: number }
export type TokenAnalysisPagination = { page: number; page_size: number; total: number }

export function finiteNumber(value: number | null | undefined): number {
  const number = Number(value || 0)
  return Number.isFinite(number) ? number : 0
}

// 表格每行会调用多次, 复用同一个 formatter 实例(构造成本远高于 format)。
const integerFormat = new Intl.NumberFormat()

// 千分位整数(浏览器默认 locale), 与 utils/format 的缩写版 formatNumber 不同。
export function formatInteger(value: number | null | undefined): string {
  return integerFormat.format(Math.round(value || 0))
}

// Token 仅用 M/B(不用 K)。
export function formatTokenMetric(value: number | null | undefined): string {
  const number = finiteNumber(value)
  return Math.abs(number) >= 1_000_000 ? formatCompactNumber(number) : formatInteger(number)
}

export function formatRequestMetric(value: number | null | undefined): string {
  return formatCompactNumber(finiteNumber(value))
}

export function formatCost(value: number | null | undefined): string {
  return `$${Number(value || 0).toFixed(4)}`
}

// 用户排行专用: 费用达到 1000 后使用 K。
export function formatCompactCost(value: number | null | undefined): string {
  const number = finiteNumber(value)
  if (Math.abs(number) >= 1_000) return `$${(number / 1_000).toFixed(1)}K`
  return formatCost(number)
}

export function percent(value: number | null | undefined): string {
  return `${((value || 0) * 100).toFixed(1)}%`
}

// 时间列固定东八区展示(团队所在时区), 形如 2026-06-10 16:23:45;
// sv-SE locale 天然输出 YYYY-MM-DD HH:mm:ss, 不带 ISO 的 T 和时区尾巴。
const timeCNFormat = new Intl.DateTimeFormat('sv-SE', {
  timeZone: 'Asia/Shanghai',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false
})

export function formatTimeCN(value?: string | null): string {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return timeCNFormat.format(d)
}

// 行内占比条: 以当前页最大值为满格。
export function shareOfMax(values: Array<number | null | undefined>): (value: number) => number {
  const max = Math.max(1, ...values.map(finiteNumber))
  return (value) => Math.min(Math.max(finiteNumber(value) / max, 0), 1)
}

export type TokenAnalysisIndexState = 'running' | 'error' | 'idle'

export const TOKEN_ANALYSIS_INDEX_DOT: Record<TokenAnalysisIndexState, string> = {
  running: 'animate-pulse bg-amber-500',
  error: 'bg-red-500',
  idle: 'bg-emerald-500'
}

export function tokenAnalysisIndexState(status?: TokenAnalysisIndexStatus | null): TokenAnalysisIndexState {
  if (status?.running) return 'running'
  if (status?.last_error) return 'error'
  return 'idle'
}

export function riskScoreClass(score: number): string {
  if (score >= 60) return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (score >= 30) return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
}

// 风险原因 code 映射中文名称/解释(见 i18n riskCodes)。缺失映射时回退展示原始
// code, 保证后端新增风险类型而前端文案未补时仍可读, 不会显示成空白。
export function useTokenAnalysisRisk() {
  const { t } = useI18n()

  function riskLabel(code: string): string {
    const key = `admin.tokenAnalysis.riskCodes.${code}.label`
    const label = t(key)
    return label === key ? code : label
  }

  function riskDesc(code: string): string {
    const key = `admin.tokenAnalysis.riskCodes.${code}.desc`
    const desc = t(key)
    return desc === key ? '' : desc
  }

  return { riskLabel, riskDesc }
}
