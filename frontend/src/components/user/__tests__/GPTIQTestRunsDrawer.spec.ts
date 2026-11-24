import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { GPTIQCaseRevision, GPTIQCaseSet, GPTIQPublicCase, GPTIQRun, GPTIQUserCard } from '@/api/gptIQTest'
import GPTIQTestRunsDrawer from '../GPTIQTestRunsDrawer.vue'

const getGPTIQTestEntry = vi.hoisted(() => vi.fn())
vi.mock('@/api/gptIQTest', () => ({ getGPTIQTestEntry }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => (params ? `${key}:${JSON.stringify(params)}` : key), te: () => true }),
  }
})

// 当前测试项（第 3 版）；抽屉里的模型/次数/题面必须取结果实际执行的版本。
const CASES: GPTIQPublicCase[] = [
  { id: 2, name: '糖果题', kind: 'graded', model: 'gpt-current', reasoning_effort: 'xhigh', runs: 5, prompt: '当前题面', public_answer: true },
]
const CURRENT_REVISION: GPTIQCaseRevision = { id: 30, revision: 3, kind: 'graded', model: 'gpt-current', reasoning_effort: 'xhigh', runs: 5, prompt: '当前题面' }
const OLD_REVISION: GPTIQCaseRevision = { id: 20, revision: 2, kind: 'graded', model: 'gpt-6.1-sol', reasoning_effort: 'high', runs: 2, prompt: '第一行\n<i>第二行</i>' }

function run(overrides: Partial<GPTIQRun> = {}): GPTIQRun {
  return {
    index: 1, status: 'ok', correct: true, answer_preview: '答案 21', answer: '推理过程\n<script>alert(1)</script>\n最终答案：21',
    input_tokens: 1200, output_tokens: 800, reasoning_tokens: 600, duration_ms: 20000, ttft_ms: 1000, tps: 40, started_at: '',
    ...overrides,
  }
}

function set(overrides: Partial<GPTIQCaseSet> = {}): GPTIQCaseSet {
  return {
    case_id: 2, batch_id: 9, trigger: 'schedule', run_date: '2026-11-23', tested_at: '2026-11-23T00:31:00Z',
    revision: OLD_REVISION,
    runs: [run(), run({ index: 2, status: 'error', reason: 'timeout', correct: null, answer: undefined, answer_preview: undefined })],
    total: 2, ok: 1, errors: 1, skipped: 0, interrupted: 0, graded: 1, correct: 1, median_reasoning_tokens: 600, avg_duration_ms: 20000, avg_tps: 40,
    ...overrides,
  }
}

const CARD: GPTIQUserCard = { id: 3, display_name: 'c-03', verdict: 'insufficient', reasons: [], tested_at: null, cases: [], trend: [] }

function drawerEl() {
  return document.body.querySelector('[data-testid="gpt-iq-drawer"]') as HTMLElement
}

describe('GPTIQTestRunsDrawer', () => {
  beforeEach(() => {
    getGPTIQTestEntry.mockReset()
    document.body.innerHTML = ''
  })

  it('shows the script-style table per batch with a summary row and plain-text answers', async () => {
    getGPTIQTestEntry.mockResolvedValue({
      enabled: true, id: 3, display_name: 'c-03', days: 14, cases: CASES,
      sets: [set(), set({ batch_id: 8, run_date: '2026-11-22', runs: [run()], total: 1, ok: 1, errors: 0 })],
    })
    const wrapper = mount(GPTIQTestRunsDrawer, { props: { card: CARD, cases: CASES, today: '2026-11-23' }, attachTo: document.body })
    await flushPromises()
    expect(getGPTIQTestEntry).toHaveBeenCalledWith(3, { days: 14 }, expect.any(AbortSignal))

    const drawer = drawerEl()
    expect(drawer.querySelectorAll('[data-testid="gpt-iq-drawer-batch"]')).toHaveLength(2)
    const summary = drawer.querySelector('[data-testid="gpt-iq-drawer-summary"]') as HTMLElement
    expect(summary.textContent).toContain('"graded":1')
    expect(summary.textContent).toContain('"percent":"100%"')
    const firstRows = drawer.querySelectorAll('[data-testid="gpt-iq-drawer-batch"]')[0].querySelectorAll('[data-testid="gpt-iq-drawer-run"]')
    expect(firstRows[0].textContent).toContain('1,200')
    expect(firstRows[0].textContent).toContain('20.0')
    expect(firstRows[1].textContent).toContain('gptIQTest.runReasons.timeout')

    // 题面来自执行版本，纯文本。
    const prompt = drawer.querySelector('[data-testid="gpt-iq-prompt"] pre') as HTMLElement
    expect(prompt.textContent).toBe('第一行\n<i>第二行</i>')
    expect(prompt.querySelector('i')).toBeNull()

    ;(drawer.querySelector('[data-testid="gpt-iq-expand"]') as HTMLButtonElement).click()
    await flushPromises()
    const full = drawer.querySelector('[data-testid="gpt-iq-full-answer"]') as HTMLElement
    expect(full.textContent).toBe('推理过程\n<script>alert(1)</script>\n最终答案：21')
    expect(full.querySelector('script')).toBeNull()
    expect(full.className).toContain('whitespace-pre-wrap')
    wrapper.unmount()
  })

  it('shows model, runs and prompt of the executed revision and labels older versions', async () => {
    getGPTIQTestEntry.mockResolvedValue({
      enabled: true, id: 3, display_name: 'c-03', days: 14, cases: CASES,
      sets: [
        set({ batch_id: 12, revision: CURRENT_REVISION, runs: [run({ status: 'cancelled', reason: 'cancelled', correct: null })], total: 1, ok: 0, errors: 0, interrupted: 1, graded: 0, correct: 0 }),
        set({ batch_id: 9 }),
        set({ batch_id: 8, run_date: '2026-11-22' }),
      ],
    })
    const wrapper = mount(GPTIQTestRunsDrawer, {
      props: { card: CARD, cases: CASES, today: '2026-11-23', currentRevisions: new Map([[2, 30]]) },
      attachTo: document.body,
    })
    await flushPromises()
    const batches = drawerEl().querySelectorAll('[data-testid="gpt-iq-drawer-batch"]')
    expect(batches).toHaveLength(3)

    // 最新批次用当前版本：无版本标记，参数来自第 3 版；中断计数单列，调用显示为中断。
    expect(batches[0].querySelector('[data-testid="gpt-iq-drawer-version"]')).toBeNull()
    expect(batches[0].querySelector('[data-testid="gpt-iq-drawer-meta"]')?.textContent).toContain('"model":"gpt-current"')
    expect(batches[0].querySelector('[data-testid="gpt-iq-drawer-summary"]')?.textContent).toContain('gptIQTest.drawer.interrupted:{"count":1}')
    const interruptedRow = batches[0].querySelector('[data-testid="gpt-iq-drawer-run"]') as HTMLElement
    expect(interruptedRow.textContent).toContain('gptIQTest.runStatus.cancelled')
    expect(interruptedRow.querySelector('.text-orange-600')).toBeNull()

    // 旧批次显示"第 2 版"，模型、次数、思考强度取第 2 版而非当前测试项。
    expect(batches[1].querySelector('[data-testid="gpt-iq-drawer-version"]')?.textContent?.trim()).toBe('gptIQTest.versionLabel:{"revision":2}')
    const oldMeta = batches[1].querySelector('[data-testid="gpt-iq-drawer-meta"]')?.textContent ?? ''
    expect(oldMeta).toContain('"model":"gpt-6.1-sol"')
    expect(oldMeta).toContain('"runs":2')
    expect(oldMeta).toContain('"effort":"high"')
    expect(oldMeta).not.toContain('gpt-current')

    // 题面按版本去重：当前版与第 2 版各一份，旧版带版本标记。
    const prompts = [...drawerEl().querySelectorAll('[data-testid="gpt-iq-prompt"]')]
    expect(prompts).toHaveLength(2)
    expect(prompts[0].querySelector('pre')?.textContent).toBe('当前题面')
    expect(prompts[0].querySelector('summary')?.textContent).not.toContain('gptIQTest.versionLabel')
    expect(prompts[1].querySelector('pre')?.textContent).toBe('第一行\n<i>第二行</i>')
    expect(prompts[1].querySelector('summary')?.textContent).toContain('gptIQTest.versionLabel')
    wrapper.unmount()
  })

  it('omits the prompt when the executed revision does not carry one', async () => {
    getGPTIQTestEntry.mockResolvedValue({
      enabled: true, id: 3, display_name: 'c-03', days: 14, cases: CASES,
      sets: [set({ revision: { ...OLD_REVISION, prompt: undefined } })],
    })
    const wrapper = mount(GPTIQTestRunsDrawer, { props: { card: CARD, cases: CASES, today: '2026-11-23' }, attachTo: document.body })
    await flushPromises()
    expect(document.body.querySelector('[data-testid="gpt-iq-prompt"]')).toBeNull()
    wrapper.unmount()
  })

  it('reloads with a different day range and closes on Escape', async () => {
    getGPTIQTestEntry.mockResolvedValue({ enabled: true, id: 3, display_name: 'c-03', days: 7, cases: CASES, sets: [] })
    const wrapper = mount(GPTIQTestRunsDrawer, { props: { card: CARD, cases: CASES, today: '2026-11-23' }, attachTo: document.body })
    await flushPromises()
    const buttons = [...document.body.querySelectorAll('[data-testid="gpt-iq-drawer"] [role="group"] button')] as HTMLButtonElement[]
    buttons[0].click()
    await flushPromises()
    expect(getGPTIQTestEntry).toHaveBeenLastCalledWith(3, { days: 7 }, expect.any(AbortSignal))
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('shows a retryable error when the detail request fails', async () => {
    getGPTIQTestEntry.mockRejectedValueOnce({ status: 404, reason: 'GPT_IQ_TEST_ENTRY_NOT_FOUND' })
    const wrapper = mount(GPTIQTestRunsDrawer, { props: { card: CARD, cases: CASES, today: '2026-11-23' }, attachTo: document.body })
    await flushPromises()
    expect(document.body.textContent).toContain('gptIQTest.drawer.loadFailed')
    wrapper.unmount()
  })
})
