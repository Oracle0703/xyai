import { describe, expect, it } from 'vitest'
import type { GPTIQCaseSet, GPTIQReason } from '@/api/gptIQTest'
import zh from '@/i18n/locales/zh/gptIQTest'
import {
  GPT_IQ_VERDICT_STYLES,
  formatGPTIQPercent,
  formatGPTIQSeconds,
  formatGPTIQTokens,
  formatGPTIQTps,
  gptIQCaseNameMap,
  gptIQChatPreview,
  gptIQCurrentRevisionIds,
  gptIQIsOldRevision,
  gptIQReasonText,
  gptIQRevisionMeta,
  gptIQRunInterrupted,
  gptIQRunMark,
  gptIQRunNoteClass,
  gptIQRunReasonLabel,
  gptIQRunStatusLabel,
  gptIQSetKind,
  gptIQTodayInZone,
  gptIQTrendCells,
  gptIQVerdictLabel,
  gptIQVerdictStyle,
  groupGPTIQSetsByBatch,
} from '../gptIQTest'

// 用真实中文文案做最小插值，校验拼出来的原因文本。
function lookup(key: string): string | undefined {
  let node: unknown = zh
  for (const part of key.split('.')) {
    if (!node || typeof node !== 'object') return undefined
    node = (node as Record<string, unknown>)[part]
  }
  return typeof node === 'string' ? node : undefined
}
const t = (key: string, params: Record<string, unknown> = {}) =>
  (lookup(key) ?? key).replace(/\{(\w+)\}/g, (_, name: string) => String(params[name] ?? `{${name}}`))
const te = (key: string) => lookup(key) !== undefined

const names = gptIQCaseNameMap([{ id: 1, name: '糖果题' }, { id: 2, name: '打招呼' }])

function reason(overrides: Partial<GPTIQReason>): GPTIQReason {
  return { code: 'low_accuracy', correct: 0, graded: 0, percent: 0, ...overrides }
}

function set(overrides: Partial<GPTIQCaseSet> = {}): GPTIQCaseSet {
  return {
    case_id: 1, batch_id: 1, trigger: 'schedule', run_date: '2026-11-23', tested_at: '2026-11-23T00:31:00Z',
    revision: { id: 30, revision: 3, kind: 'graded', model: 'gpt-6.1-sol', reasoning_effort: 'high', runs: 3 },
    runs: [], total: 0, ok: 0, errors: 0, skipped: 0, interrupted: 0, graded: 0, correct: 0,
    median_reasoning_tokens: null, avg_duration_ms: null, avg_tps: null,
    ...overrides,
  }
}

describe('verdict labels and styles', () => {
  it('labels every verdict in Chinese', () => {
    expect(gptIQVerdictLabel('normal', t, te)).toBe('正常')
    expect(gptIQVerdictLabel('connected', t, te)).toBe('连通正常')
    expect(gptIQVerdictLabel('suspect', t, te)).toBe('疑似降智')
    expect(gptIQVerdictLabel('failed', t, te)).toBe('失败')
    expect(gptIQVerdictLabel('insufficient', t, te)).toBe('样本不足')
    expect(gptIQVerdictLabel('skipped', t, te)).toBe('已跳过')
    expect(gptIQVerdictLabel('untested', t, te)).toBe('未测')
    expect(gptIQVerdictLabel('mystery', t, te)).toBe('mystery')
  })

  it('colors normal/connected green, suspect red, failed orange and the rest gray', () => {
    expect(GPT_IQ_VERDICT_STYLES.normal.badge).toBe('badge-success')
    expect(GPT_IQ_VERDICT_STYLES.connected.badge).toBe('badge-success')
    expect(GPT_IQ_VERDICT_STYLES.suspect.badge).toBe('badge-danger')
    expect(GPT_IQ_VERDICT_STYLES.failed.badge).toContain('orange')
    for (const verdict of ['insufficient', 'skipped', 'untested'] as const) expect(GPT_IQ_VERDICT_STYLES[verdict].badge).toBe('badge-gray')
    expect(gptIQVerdictStyle('unknown')).toBe(GPT_IQ_VERDICT_STYLES.untested)
  })
})

describe('verdict reason text', () => {
  it('builds the reason texts from i18n', () => {
    expect(gptIQReasonText(reason({ code: 'low_accuracy', case_id: 1, correct: 1, graded: 3 }), names, t, te)).toBe('糖果题 1/3')
    expect(gptIQReasonText(reason({ code: 'low_reasoning', case_id: 1, percent: 18 }), names, t, te)).toBe('糖果题 思考 token 仅为同批中位数的 18%')
    expect(gptIQReasonText(reason({ code: 'insufficient_samples', case_id: 1, correct: 1, graded: 2 }), names, t, te)).toBe('糖果题 样本不足（有效判分 2 次）')
  })

  it('translates the error category of failure reasons instead of showing raw codes', () => {
    expect(gptIQReasonText(reason({ code: 'all_failed', detail: 'timeout' }), names, t, te)).toBe('全部调用失败：请求超时')
    expect(gptIQReasonText(reason({ code: 'all_failed' }), names, t, te)).toBe('全部调用失败')
    expect(gptIQReasonText(reason({ code: 'chat_failed', case_id: 2, detail: 'rate_limited' }), names, t, te)).toBe('打招呼 调用失败：上游限流（429）')
    expect(gptIQReasonText(reason({ code: 'chat_failed', case_id: 9 }), names, t, te)).toBe('测试项 #9 调用失败')
  })

  it('labels interrupted samples separately from failures', () => {
    expect(gptIQReasonText(reason({ code: 'interrupted', case_id: 1, correct: 1, graded: 1 }), names, t, te)).toBe('糖果题 测试中断，样本不足')
    expect(gptIQReasonText(reason({ code: 'interrupted' }), names, t, te)).toBe('测试中断，样本不足')
    expect(gptIQRunStatusLabel('cancelled', t, te)).toBe('中断')
    expect(gptIQRunStatusLabel('abandoned', t, te)).toBe('中断')
    expect(gptIQRunStatusLabel('error', t, te)).toBe('失败')
    expect(gptIQRunInterrupted('cancelled')).toBe(true)
    expect(gptIQRunInterrupted('abandoned')).toBe(true)
    expect(gptIQRunInterrupted('error')).toBe(false)
    expect(gptIQRunNoteClass('error')).toContain('orange')
    expect(gptIQRunNoteClass('abandoned')).toContain('gray')
    expect(gptIQRunNoteClass('skipped')).toContain('gray')
    expect(gptIQRunReasonLabel('quota_unknown', t, te)).toBe('额度未知，已跳过')
  })

  it('labels every run/skip reason code the backend emits', () => {
    const codes = ['account_unavailable', 'not_supported', 'account_error', 'account_rate_limited', 'backoff', 'token_unavailable', 'low_quota', 'quota_unknown', 'cooldown', 'budget', 'after_auth_failure', 'after_rate_limit', 'unauthorized', 'forbidden', 'rate_limited', 'timeout', 'upstream_error', 'request_failed', 'parse_failed', 'cancelled', 'internal_error', 'stream_incomplete', 'empty_answer', 'response_failed', 'abandoned']
    for (const code of codes) expect(gptIQRunReasonLabel(code, t, te)).not.toBe(code)
    expect(gptIQRunReasonLabel('brand_new', t, te)).toBe('brand_new')
    expect(gptIQRunReasonLabel(undefined, t, te)).toBe('')
  })
})

describe('run marks and formatting', () => {
  it('maps correctness to ✓/✗/–', () => {
    expect(gptIQRunMark({ correct: true })).toBe('correct')
    expect(gptIQRunMark({ correct: false })).toBe('wrong')
    expect(gptIQRunMark({ correct: null })).toBe('none')
  })

  it('formats seconds, tokens, TPS and percent with null placeholders', () => {
    expect(formatGPTIQSeconds(12345)).toBe('12.3')
    expect(formatGPTIQSeconds(null)).toBe('—')
    expect(formatGPTIQTokens(12345)).toBe('12,345')
    expect(formatGPTIQTokens(null)).toBe('—')
    expect(formatGPTIQTps(45.67)).toBe('45.7')
    expect(formatGPTIQPercent(200 / 3)).toBe('66.7%')
    expect(formatGPTIQPercent(100)).toBe('100%')
    expect(formatGPTIQPercent(null)).toBe('—')
  })
})

describe('executed revision helpers', () => {
  it('uses the executed revision for kind and meta, not the current case', () => {
    expect(gptIQSetKind(set())).toBe('graded')
    expect(gptIQSetKind(set({ revision: { id: 31, revision: 4, kind: 'chat', model: 'm', runs: 1 }, graded: 2 }))).toBe('chat')
    expect(gptIQRevisionMeta(set().revision, t)).toBe('gpt-6.1-sol · 3 次 · 思考 high')
    expect(gptIQRevisionMeta({ model: 'gpt-6.1', runs: 1 }, t)).toBe('gpt-6.1 · 1 次')
  })

  it('marks results from an older revision against the current one', () => {
    const sets = [set({ revision: { ...set().revision, id: 30, revision: 3 } }), set({ revision: { ...set().revision, id: 25, revision: 2 } })]
    // 接口未给当前版本时，取已加载结果中最新执行的版本。
    const inferred = gptIQCurrentRevisionIds([{ id: 1 }], sets)
    expect(inferred.get(1)).toBe(30)
    expect(gptIQIsOldRevision(sets[0], inferred)).toBe(false)
    expect(gptIQIsOldRevision(sets[1], inferred)).toBe(true)
    // 接口给出的当前版本优先（题目改过但新版还没执行时，最新结果也算旧版）。
    const exact = gptIQCurrentRevisionIds([{ id: 1, revision_id: 33 }], sets)
    expect(gptIQIsOldRevision(sets[0], exact)).toBe(true)
    // 与上层映射合并取最新。
    expect(gptIQCurrentRevisionIds([], [sets[1]], new Map([[1, 30]])).get(1)).toBe(30)
  })
})

describe('trend and grouping helpers', () => {
  it('computes today in the server timezone', () => {
    // 2026-11-23 17:00 UTC 已是上海 11-24。
    expect(gptIQTodayInZone('2026-11-23T17:00:00Z', 'Asia/Shanghai')).toBe('2026-11-24')
    expect(gptIQTodayInZone('2026-11-23T15:00:00Z', 'Asia/Shanghai')).toBe('2026-11-23')
  })

  it('lays out one cell per day ending today and leaves untested days empty', () => {
    const cells = gptIQTrendCells([
      { date: '2026-11-10', verdict: 'normal', correct: 3, graded: 3 },
      { date: '2026-11-23', verdict: 'suspect', correct: 1, graded: 3 },
      { date: '2026-10-01', verdict: 'failed', correct: 0, graded: 0 },
    ], '2026-11-23', 14)
    expect(cells).toHaveLength(14)
    expect(cells[0].date).toBe('2026-11-10')
    expect(cells[0].point?.verdict).toBe('normal')
    expect(cells[13]).toEqual({ date: '2026-11-23', point: { date: '2026-11-23', verdict: 'suspect', correct: 1, graded: 3 } })
    expect(cells.filter((cell) => cell.point)).toHaveLength(2)
  })

  it('previews the first successful chat answer only', () => {
    const base = { index: 1, correct: null, input_tokens: null, output_tokens: null, reasoning_tokens: null, duration_ms: null, ttft_ms: null, tps: null, started_at: '' }
    expect(gptIQChatPreview(set({ runs: [{ ...base, status: 'error', reason: 'timeout' }, { ...base, index: 2, status: 'ok', answer_preview: 'Hello!' }] }))).toBe('Hello!')
    expect(gptIQChatPreview(set({ runs: [{ ...base, status: 'ok' }] }))).toBe('')
  })

  it('groups detail sets by batch, newest batch first as returned', () => {
    const groups = groupGPTIQSetsByBatch([
      set({ batch_id: 9, case_id: 1, trigger: 'manual', tested_at: '2026-11-23T06:00:00Z' }),
      set({ batch_id: 9, case_id: 2, trigger: 'manual', tested_at: '2026-11-23T06:05:00Z' }),
      set({ batch_id: 7, case_id: 1, run_date: '2026-11-22' }),
    ])
    expect(groups.map((group) => group.batch_id)).toEqual([9, 7])
    expect(groups[0].sets.map((item) => item.case_id)).toEqual([1, 2])
    expect(groups[0].tested_at).toBe('2026-11-23T06:05:00Z')
    expect(groups[1].run_date).toBe('2026-11-22')
  })
})
