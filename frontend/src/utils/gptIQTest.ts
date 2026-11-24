import type { GPTIQCaseRevision, GPTIQCaseSet, GPTIQPublicCase, GPTIQReason, GPTIQRun, GPTIQTrendPoint, GPTIQVerdict } from '@/api/gptIQTest'
import { formatDate } from '@/utils/format'

type Translate = (key: string, params?: Record<string, unknown>) => string
type TranslateExists = (key: string) => boolean

/** 判定的配色：正常/连通绿，疑似降智红，失败橙，样本不足/跳过/未测灰。 */
export const GPT_IQ_VERDICT_STYLES: Record<GPTIQVerdict, {
  badge: string
  cell: string
  icon: 'checkCircle' | 'exclamationTriangle' | 'exclamationCircle' | 'questionCircle' | 'clock'
}> = {
  normal: { badge: 'badge-success', cell: 'bg-emerald-500', icon: 'checkCircle' },
  connected: { badge: 'badge-success', cell: 'bg-emerald-400', icon: 'checkCircle' },
  suspect: { badge: 'badge-danger', cell: 'bg-red-500', icon: 'exclamationTriangle' },
  failed: { badge: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300', cell: 'bg-orange-500', icon: 'exclamationCircle' },
  insufficient: { badge: 'badge-gray', cell: 'bg-gray-400 dark:bg-dark-500', icon: 'questionCircle' },
  skipped: { badge: 'badge-gray', cell: 'bg-gray-300 dark:bg-dark-600', icon: 'clock' },
  untested: { badge: 'badge-gray', cell: 'bg-gray-200 dark:bg-dark-700', icon: 'clock' },
}

const KNOWN_VERDICTS = new Set<string>(Object.keys(GPT_IQ_VERDICT_STYLES))

export function gptIQVerdictStyle(verdict: string) {
  return GPT_IQ_VERDICT_STYLES[(KNOWN_VERDICTS.has(verdict) ? verdict : 'untested') as GPTIQVerdict]
}

export function gptIQVerdictLabel(verdict: string, t: Translate, te?: TranslateExists): string {
  const key = `gptIQTest.verdicts.${verdict}`
  return !te || te(key) ? t(key) : verdict
}

/** 调用失败/跳过原因（账号退避、超时、上游错误等）的文案；未知代码原样显示。 */
export function gptIQRunReasonLabel(reason: string | undefined | null, t: Translate, te?: TranslateExists): string {
  if (!reason) return ''
  const key = `gptIQTest.runReasons.${reason}`
  return !te || te(key) ? t(key) : reason
}

export function gptIQRunStatusLabel(status: string, t: Translate, te?: TranslateExists): string {
  const key = `gptIQTest.runStatus.${status}`
  return !te || te(key) ? t(key) : status
}

/** cancelled 与 abandoned 都是"中断"，不算失败。 */
export function gptIQRunInterrupted(status: string): boolean {
  return status === 'cancelled' || status === 'abandoned'
}

/** 非成功调用的说明文字颜色：只有 error 用失败色，中断/跳过用灰色。 */
export function gptIQRunNoteClass(status: string): string {
  return status === 'error' ? 'text-orange-600 dark:text-orange-400' : 'text-gray-500 dark:text-gray-400'
}

function caseLabel(caseId: number | undefined, caseNames: ReadonlyMap<number, string>, t: Translate): string {
  if (!caseId) return ''
  return caseNames.get(caseId) ?? t('gptIQTest.unknownCase', { id: caseId })
}

/**
 * 判定原因的展示文案，全部由 i18n 拼出（例："糖果题 1/3"、"思考 token 仅为同批中位数的 18%"、"样本不足"）。
 * detail 是错误类别代码，先翻译再填入；不展示任何上游原文。
 */
export function gptIQReasonText(reason: GPTIQReason, caseNames: ReadonlyMap<number, string>, t: Translate, te?: TranslateExists): string {
  const name = caseLabel(reason.case_id, caseNames, t)
  const detail = gptIQRunReasonLabel(reason.detail, t, te)
  switch (reason.code) {
    case 'low_accuracy':
      return t('gptIQTest.verdictReasons.low_accuracy', { case: name, correct: reason.correct, graded: reason.graded })
    case 'low_reasoning':
      return t('gptIQTest.verdictReasons.low_reasoning', { case: name, percent: reason.percent })
    case 'insufficient_samples':
      return t('gptIQTest.verdictReasons.insufficient_samples', { case: name, correct: reason.correct, graded: reason.graded })
    case 'all_failed':
      return detail ? t('gptIQTest.verdictReasons.all_failed_detail', { detail }) : t('gptIQTest.verdictReasons.all_failed')
    case 'chat_failed':
      return detail ? t('gptIQTest.verdictReasons.chat_failed_detail', { case: name, detail }) : t('gptIQTest.verdictReasons.chat_failed', { case: name })
    case 'interrupted':
      return name ? t('gptIQTest.verdictReasons.interrupted', { case: name }) : t('gptIQTest.verdictReasons.interrupted_all')
    default: {
      const key = `gptIQTest.verdictReasons.${String(reason.code)}`
      return te && !te(key) ? String(reason.code) : t(key, { case: name })
    }
  }
}

export function gptIQCaseNameMap(cases: ReadonlyArray<Pick<GPTIQPublicCase, 'id' | 'name'>>): Map<number, string> {
  return new Map(cases.map((item) => [item.id, item.name]))
}

/**
 * 每个测试项的"当前版本"：接口给出 revision_id 时以它为准，否则取已加载结果中最新执行的版本（ID 递增）。
 * base 为上层已算好的映射（例如列表页全部卡片），会与本次数据合并。
 */
export function gptIQCurrentRevisionIds(
  cases: ReadonlyArray<Pick<GPTIQPublicCase, 'id' | 'revision_id'>>,
  sets: ReadonlyArray<Pick<GPTIQCaseSet, 'case_id' | 'revision'>>,
  base?: ReadonlyMap<number, number>,
): Map<number, number> {
  const out = new Map(base ?? [])
  for (const set of sets) {
    const id = set.revision?.id
    if (typeof id === 'number') out.set(set.case_id, Math.max(out.get(set.case_id) ?? 0, id))
  }
  for (const item of cases) {
    if (typeof item.revision_id === 'number') out.set(item.id, item.revision_id)
  }
  return out
}

/** 该组结果是否由旧版本题目执行（需要显示"第 N 版"）。 */
export function gptIQIsOldRevision(set: Pick<GPTIQCaseSet, 'case_id' | 'revision'>, current: ReadonlyMap<number, number>): boolean {
  const id = set.revision?.id
  const latest = current.get(set.case_id)
  return typeof id === 'number' && typeof latest === 'number' && id !== latest
}

/** 实际执行版本的参数摘要：模型 · 次数 · 思考强度。 */
export function gptIQRevisionMeta(revision: Pick<GPTIQCaseRevision, 'model' | 'runs' | 'reasoning_effort'> | undefined, t: Translate): string {
  if (!revision) return ''
  const meta = t('gptIQTest.drawer.caseMeta', { model: revision.model, runs: revision.runs })
  return revision.reasoning_effort ? `${meta} · ${t('gptIQTest.drawer.effort', { effort: revision.reasoning_effort })}` : meta
}

/** 测试项类型以实际执行的版本为准；缺失时按结果推断。 */
export function gptIQSetKind(set: Pick<GPTIQCaseSet, 'revision' | 'graded' | 'runs'>): 'chat' | 'graded' {
  const kind = set.revision?.kind
  if (kind === 'chat' || kind === 'graded') return kind
  return set.graded > 0 || set.runs.some((run) => run.correct !== null) ? 'graded' : 'chat'
}

/** 单次调用的判分标记：✓ 对、✗ 错、– 未判分（失败、跳过、执行中或 chat）。 */
export type GPTIQRunMark = 'correct' | 'wrong' | 'none'

export function gptIQRunMark(run: Pick<GPTIQRun, 'correct'>): GPTIQRunMark {
  if (run.correct === true) return 'correct'
  if (run.correct === false) return 'wrong'
  return 'none'
}

export const GPT_IQ_RUN_MARK_STYLES: Record<GPTIQRunMark, { symbol: string; className: string }> = {
  correct: { symbol: '✓', className: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300' },
  wrong: { symbol: '✗', className: 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300' },
  none: { symbol: '–', className: 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-300' },
}

export function gptIQAccuracyPercent(correct: number, graded: number): number | null {
  return graded > 0 ? (correct * 100) / graded : null
}

export function formatGPTIQPercent(value: number | null | undefined): string {
  return typeof value === 'number' && Number.isFinite(value) ? `${Number(value.toFixed(1))}%` : '—'
}

/** 毫秒转秒，保留 1 位小数（与脚本 Time(s) 列一致）。 */
export function formatGPTIQSeconds(ms: number | null | undefined): string {
  return typeof ms === 'number' && Number.isFinite(ms) ? (ms / 1000).toFixed(1) : '—'
}

export function formatGPTIQTokens(value: number | null | undefined): string {
  return typeof value === 'number' && Number.isFinite(value) ? Math.round(value).toLocaleString('en-US') : '—'
}

export function formatGPTIQTps(value: number | null | undefined): string {
  return typeof value === 'number' && Number.isFinite(value) ? value.toFixed(1) : '—'
}

/** 当天只显示时:分，其他日期带月/日。 */
export function formatGPTIQTime(value: string | null | undefined, now: Date = new Date()): string {
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

export function formatGPTIQClock(value: string | null | undefined): string {
  if (!value) return '—'
  return formatDate(value, { hour: '2-digit', minute: '2-digit', hour12: false }) || '—'
}

function isoDate(date: Date): string {
  return date.toISOString().slice(0, 10)
}

/** 服务端时区（Asia/Shanghai）下的当天日期 YYYY-MM-DD；时区无效时退回本地日期。 */
export function gptIQTodayInZone(serverTime: string | null | undefined, timezone: string | null | undefined): string {
  const parsed = serverTime ? new Date(serverTime) : new Date()
  const base = Number.isNaN(parsed.getTime()) ? new Date() : parsed
  try {
    const parts = new Intl.DateTimeFormat('en-CA', { timeZone: timezone || undefined, year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(base)
    const get = (type: string) => parts.find((part) => part.type === type)?.value ?? ''
    const text = `${get('year')}-${get('month')}-${get('day')}`
    if (/^\d{4}-\d{2}-\d{2}$/.test(text)) return text
  } catch {
    // 无效时区：落到本地日期。
  }
  const local = new Date(base.getTime() - base.getTimezoneOffset() * 60000)
  return isoDate(local)
}

/** 趋势条：以 today 结尾的连续 days 天，每天一格；趋势只来自定时批次，没有定时结果的日期为 null。 */
export function gptIQTrendCells(trend: ReadonlyArray<GPTIQTrendPoint>, today: string, days = 14): Array<{ date: string; point: GPTIQTrendPoint | null }> {
  const byDate = new Map(trend.map((point) => [point.date, point]))
  const end = new Date(`${today}T00:00:00Z`)
  if (Number.isNaN(end.getTime())) return trend.slice(-days).map((point) => ({ date: point.date, point }))
  const cells: Array<{ date: string; point: GPTIQTrendPoint | null }> = []
  for (let offset = days - 1; offset >= 0; offset--) {
    const date = isoDate(new Date(end.getTime() - offset * 86400000))
    cells.push({ date, point: byDate.get(date) ?? null })
  }
  return cells
}

/** chat 类测试项在卡片上显示的回答预览：取第一条成功调用的预览，没有则返回空。 */
export function gptIQChatPreview(set: Pick<GPTIQCaseSet, 'runs'>): string {
  return set.runs.find((run) => run.status === 'ok' && run.answer_preview)?.answer_preview ?? ''
}

/** chat 失败时卡片显示的原因：第一条失败/跳过调用的原因代码。 */
export function gptIQFirstFailure(set: Pick<GPTIQCaseSet, 'runs'>): GPTIQRun | undefined {
  return set.runs.find((run) => run.status !== 'ok' && run.status !== 'running')
}

export interface GPTIQBatchGroup {
  batch_id: number
  run_date: string
  trigger: GPTIQCaseSet['trigger']
  tested_at: string
  sets: GPTIQCaseSet[]
}

/** 详情抽屉按批次分组（接口已按最新批次在前排序，组内保持接口顺序）。 */
export function groupGPTIQSetsByBatch(sets: ReadonlyArray<GPTIQCaseSet>): GPTIQBatchGroup[] {
  const groups: GPTIQBatchGroup[] = []
  const byBatch = new Map<number, GPTIQBatchGroup>()
  for (const set of sets) {
    let group = byBatch.get(set.batch_id)
    if (!group) {
      group = { batch_id: set.batch_id, run_date: set.run_date, trigger: set.trigger, tested_at: set.tested_at, sets: [] }
      byBatch.set(set.batch_id, group)
      groups.push(group)
    }
    group.sets.push(set)
    if (set.tested_at > group.tested_at) group.tested_at = set.tested_at
  }
  return groups
}
