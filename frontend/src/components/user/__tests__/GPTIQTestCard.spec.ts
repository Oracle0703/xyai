import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { GPTIQCaseSet, GPTIQPublicCase, GPTIQRun, GPTIQUserCard } from '@/api/gptIQTest'
import GPTIQTestCard from '../GPTIQTestCard.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => (params ? `${key}:${JSON.stringify(params)}` : key),
      te: () => true,
    }),
  }
})

const NOW = Date.parse('2026-11-23T04:00:00Z')

const CASES: GPTIQPublicCase[] = [
  { id: 1, name: '打招呼', kind: 'chat', model: 'gpt-6.1', runs: 1, public_answer: true },
  { id: 2, name: '糖果题', kind: 'graded', model: 'gpt-6.1-sol', reasoning_effort: 'high', runs: 3, public_answer: true },
]

function run(overrides: Partial<GPTIQRun> = {}): GPTIQRun {
  return {
    index: 1, status: 'ok', correct: null, input_tokens: 10, output_tokens: 20, reasoning_tokens: 0,
    duration_ms: 1500, ttft_ms: 300, tps: 13.3, started_at: '2026-11-23T00:30:00Z',
    ...overrides,
  }
}

function caseSet(overrides: Partial<GPTIQCaseSet> = {}): GPTIQCaseSet {
  return {
    case_id: 1, batch_id: 5, trigger: 'schedule', run_date: '2026-11-23', tested_at: '2026-11-23T00:31:00Z',
    revision: { id: 11, revision: 1, kind: 'chat', model: 'gpt-6.1', runs: 1 },
    runs: [run()], total: 1, ok: 1, errors: 0, skipped: 0, interrupted: 0, graded: 0, correct: 0,
    median_reasoning_tokens: null, avg_duration_ms: 1500, avg_tps: 13.3,
    ...overrides,
  }
}

function card(overrides: Partial<GPTIQUserCard> = {}): GPTIQUserCard {
  return {
    id: 7, display_name: 'c-01', verdict: 'normal', reasons: [], tested_at: '2026-11-23T00:31:00Z',
    cases: [caseSet()], trend: [], ...overrides,
  }
}

function mountCard(value: GPTIQUserCard, currentRevisions: Map<number, number> | null = null) {
  return mount(GPTIQTestCard, { props: { card: value, cases: CASES, today: '2026-11-23', now: NOW, currentRevisions } })
}

describe('GPTIQTestCard', () => {
  it('renders model answers as plain text without injecting HTML', () => {
    const malicious = '<img src=x onerror="alert(1)"><b>bold</b>\nline 2'
    const wrapper = mountCard(card({ display_name: '<script>x</script>', cases: [caseSet({ runs: [run({ answer_preview: malicious })] })] }))
    const preview = wrapper.find('[data-testid="gpt-iq-preview"]')
    expect(preview.exists()).toBe(true)
    expect(preview.text()).toContain('<img src=x onerror="alert(1)"><b>bold</b>')
    expect(preview.element.querySelector('img')).toBeNull()
    expect(preview.element.querySelector('b')).toBeNull()
    expect(preview.classes()).toContain('whitespace-pre-wrap')
    expect(wrapper.element.querySelector('script')).toBeNull()
    expect(wrapper.text()).toContain('<script>x</script>')
  })

  it('hides the chat preview when the answer is not provided', () => {
    const wrapper = mountCard(card({ cases: [caseSet({ runs: [run({ answer_preview: undefined })] })] }))
    expect(wrapper.find('[data-testid="gpt-iq-preview"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="gpt-iq-failure"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('gptIQTest.latency')
  })

  it('shows the failure category for a failed chat case instead of a preview', () => {
    const wrapper = mountCard(card({
      verdict: 'failed',
      reasons: [{ code: 'all_failed', correct: 0, graded: 0, percent: 0, detail: 'timeout' }],
      cases: [caseSet({ runs: [run({ status: 'error', reason: 'timeout' })], ok: 0, errors: 1 })],
    }))
    expect(wrapper.find('[data-testid="gpt-iq-preview"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="gpt-iq-failure"]').text()).toContain('gptIQTest.runReasons.timeout')
    expect(wrapper.find('[data-testid="gpt-iq-reasons"]').text()).toContain('gptIQTest.verdictReasons.all_failed_detail')
  })

  it('renders graded runs as ✓/✗ marks with the score and a manual retest label', () => {
    const wrapper = mountCard(card({
      verdict: 'suspect',
      reasons: [{ code: 'low_accuracy', case_id: 2, correct: 1, graded: 3, percent: 0 }],
      cases: [caseSet({
        case_id: 2,
        revision: { id: 21, revision: 2, kind: 'graded', model: 'gpt-6.1-sol', reasoning_effort: 'high', runs: 3 },
        trigger: 'manual',
        tested_at: '2026-11-23T06:05:00Z',
        runs: [run({ index: 1, correct: true }), run({ index: 2, correct: false }), run({ index: 3, status: 'error', reason: 'timeout', correct: null })],
        total: 3, ok: 2, errors: 1, graded: 2, correct: 1, median_reasoning_tokens: 1234,
      })],
    }))
    expect(wrapper.find('[data-testid="gpt-iq-run-marks"]').text()).toContain('✓✗–')
    expect(wrapper.text()).toContain('gptIQTest.graded:{"correct":1,"graded":2}')
    expect(wrapper.text()).toContain('"value":"1,234"')
    expect(wrapper.find('[data-testid="gpt-iq-retest"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="gpt-iq-verdict"]').classes()).toContain('badge-danger')
    expect(wrapper.find('[data-testid="gpt-iq-reasons"]').text()).toContain('"case":"糖果题"')
  })

  it('draws a 14-day trend strip and emits open on click', async () => {
    const wrapper = mountCard(card({ trend: [{ date: '2026-11-22', verdict: 'suspect', correct: 1, graded: 3 }] }))
    const cells = wrapper.findAll('[data-testid="gpt-iq-trend"] [role="listitem"]')
    expect(cells).toHaveLength(14)
    expect(cells[12].attributes('data-verdict')).toBe('suspect')
    expect(cells[13].attributes('data-verdict')).toBe('none')
    await wrapper.find('[data-testid="gpt-iq-card"]').trigger('click')
    expect(wrapper.emitted('open')?.[0]?.[0]).toMatchObject({ id: 7 })
  })

  it('takes kind and model from the executed revision and labels an older version', () => {
    // 当前测试项已改成 chat，但这组结果由第 2 版判分题执行，应按判分展示。
    const wrapper = mountCard(card({
      cases: [caseSet({
        case_id: 1,
        revision: { id: 9, revision: 2, kind: 'graded', model: 'gpt-old', reasoning_effort: 'low', runs: 2 },
        runs: [run({ index: 1, correct: true }), run({ index: 2, correct: true })],
        total: 2, ok: 2, graded: 2, correct: 2,
      })],
    }), new Map([[1, 11]]))
    expect(wrapper.find('[data-testid="gpt-iq-run-marks"]').text()).toContain('✓✓')
    const version = wrapper.find('[data-testid="gpt-iq-version"]')
    expect(version.text()).toBe('gptIQTest.versionLabel:{"revision":2}')
    expect(version.attributes('title')).toContain('gpt-old')
    expect(wrapper.find('[data-testid="gpt-iq-case-row"] span[title]').attributes('title')).toContain('gpt-old')
  })

  it('does not label the current revision', () => {
    const wrapper = mountCard(card(), new Map([[1, 11]]))
    expect(wrapper.find('[data-testid="gpt-iq-version"]').exists()).toBe(false)
  })

  it('shows an interrupted chat call as interrupted, not as a failure', () => {
    const wrapper = mountCard(card({
      verdict: 'insufficient',
      reasons: [{ code: 'interrupted', case_id: 1, correct: 0, graded: 0, percent: 0 }],
      cases: [caseSet({ runs: [run({ status: 'abandoned', reason: 'abandoned' })], ok: 0, interrupted: 1 })],
    }))
    const note = wrapper.find('[data-testid="gpt-iq-failure"]')
    expect(note.text()).toContain('gptIQTest.runStatus.abandoned')
    expect(note.classes()).not.toContain('text-orange-600')
    expect(wrapper.find('[data-testid="gpt-iq-reasons"]').text()).toContain('gptIQTest.verdictReasons.interrupted')
  })
})
