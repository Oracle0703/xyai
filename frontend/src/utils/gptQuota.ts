import type { GPTQuotaUserCard, GPTQuotaWindow } from '@/api/gptQuotaDisplay'
import { formatDate } from '@/utils/format'

export type GPTQuotaLevel = 'good' | 'low' | 'critical'
export type GPTQuotaWindowKey = 'five_hour' | 'seven_day'

/** 剩余比例分级：≤10% 紧张、≤30% 偏低，其余充足。 */
export function gptQuotaLevel(remaining: number): GPTQuotaLevel {
  if (remaining <= 10) return 'critical'
  if (remaining <= 30) return 'low'
  return 'good'
}

export const GPT_QUOTA_LEVEL_STYLES: Record<GPTQuotaLevel, {
  fill: string
  track: string
  ring: string
  ringTrack: string
  value: string
  badge: string
  icon: 'checkCircle' | 'exclamationTriangle' | 'exclamationCircle'
}> = {
  good: { fill: 'bg-emerald-500', track: 'bg-emerald-100 dark:bg-emerald-900/40', ring: 'text-emerald-500', ringTrack: 'text-emerald-100 dark:text-emerald-900/40', value: 'text-emerald-700 dark:text-emerald-400', badge: 'badge-success', icon: 'checkCircle' },
  low: { fill: 'bg-amber-500', track: 'bg-amber-100 dark:bg-amber-900/40', ring: 'text-amber-500', ringTrack: 'text-amber-100 dark:text-amber-900/40', value: 'text-amber-700 dark:text-amber-400', badge: 'badge-warning', icon: 'exclamationTriangle' },
  critical: { fill: 'bg-red-500', track: 'bg-red-100 dark:bg-red-900/40', ring: 'text-red-500', ringTrack: 'text-red-100 dark:text-red-900/40', value: 'text-red-600 dark:text-red-400', badge: 'badge-danger', icon: 'exclamationCircle' },
}

type QuotaWindows = Pick<GPTQuotaUserCard, 'five_hour' | 'seven_day'>

/**
 * 需要展示的额度窗口。OpenAI 目前已取消 5 小时限制，上游不再返回该窗口（快照为 null），
 * 此时不展示 5 小时；一旦上游重新返回数据会自动显示。7 天是主维度，缺失时仍显示"未提供"。
 */
export function visibleGPTQuotaWindows(card: QuotaWindows): Array<{ key: GPTQuotaWindowKey; window: GPTQuotaWindow | null }> {
  const rows: Array<{ key: GPTQuotaWindowKey; window: GPTQuotaWindow | null }> = []
  if (card.five_hour) rows.push({ key: 'five_hour', window: card.five_hour })
  rows.push({ key: 'seven_day', window: card.seven_day })
  return rows
}

/** 卡片整体剩余：取已提供窗口中的最小值，任一窗口用尽即视为紧张；均未提供返回 null。 */
export function effectiveGPTQuotaRemaining(card: QuotaWindows): number | null {
  const values = [card.five_hour, card.seven_day].filter((window): window is GPTQuotaWindow => !!window).map((window) => window.remaining_percent)
  return values.length ? Math.min(...values) : null
}

/** 7 天剩余的平均值（忽略未提供的账号）；没有数据返回 null。 */
export function averageSevenDayRemaining(cards: QuotaWindows[]): { average: number | null; count: number } {
  const values = cards.map((card) => card.seven_day?.remaining_percent).filter((value): value is number => typeof value === 'number')
  if (!values.length) return { average: null, count: 0 }
  return { average: values.reduce((sum, value) => sum + value, 0) / values.length, count: values.length }
}

export function formatGPTQuotaPercent(value: number | null | undefined): string {
  return typeof value === 'number' ? `${value.toFixed(1)}%` : '—'
}

/** 统计卡里的时间：当天只显示时:分，其他日期带月/日，避免完整日期在卡片里被截断。 */
export function formatGPTQuotaTileTime(value: string | null | undefined, now: Date = new Date()): string {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  const sameDay = date.toDateString() === now.toDateString()
  return formatDate(
    date,
    sameDay
      ? { hour: '2-digit', minute: '2-digit', hour12: false }
      : { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false },
  )
}
