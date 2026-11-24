import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { GPTIQAdminCase, GPTIQAdminEntry, GPTIQBatch, GPTIQBatchResult } from '@/api/admin/gptIQTest'
import RunPanel from '../RunPanel.vue'

const api = vi.hoisted(() => ({
  startGPTIQTestRun: vi.fn(),
  getGPTIQTestRun: vi.fn(),
  cancelGPTIQTestRun: vi.fn(),
}))
const { showError, showSuccess } = vi.hoisted(() => ({ showError: vi.fn(), showSuccess: vi.fn() }))
const auth = vi.hoisted(() => ({ user: { id: 42 } }))

vi.mock('@/api/admin/gptIQTest', () => api)
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key, te: () => true }) }
})

const ENTRIES: GPTIQAdminEntry[] = [
  { id: 1, display_name: 'c-01', group: 'xunyou', runnable: true },
  { id: 2, display_name: 'd-01', group: 'wsdashi', runnable: false },
]
const CASES = [
  { id: 10, name: '糖果题', kind: 'graded', runs: 2, enabled: true },
  { id: 11, name: '试运行', kind: 'chat', runs: 1, enabled: false },
] as GPTIQAdminCase[]

function batch(overrides: Partial<GPTIQBatch> = {}): GPTIQBatch {
  return {
    id: 5, trigger: 'manual', run_date: '2026-11-23', triggered_by: 42, scope_groups: [], entry_ids: [1], case_ids: [10],
    status: 'running', failure_reason: '', cancel_requested: false, heartbeat_at: '', total_calls: 2, started_at: '2026-11-23T06:00:00Z',
    ...overrides,
  }
}

function result(overrides: Partial<GPTIQBatchResult> = {}): GPTIQBatchResult {
  return {
    id: 100, batch_id: 5, run_date: '2026-11-23', trigger: 'manual', entry_id: 1, case_id: 10, revision_id: 1, run_index: 1,
    attempts: 1, status: 'ok', correct: true, model_requested: 'gpt-6.1-sol', answer_text: '最终答案：21', answer_truncated: false,
    input_tokens: 100, cached_tokens: 0, output_tokens: 50, reasoning_tokens: 400, ttft_ms: 900, duration_ms: 12000,
    started_at: '', entry_name: 'c-01', group: 'xunyou', tps: 4.2,
    ...overrides,
  }
}

function mountPanel(props: Record<string, unknown> = {}) {
  return mount(RunPanel, {
    props: { entries: ENTRIES, cases: CASES, batches: [], runningBatchId: null, scope: { full_scope: true, groups: [], can_configure: true }, ...props },
  })
}

describe('RunPanel', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    Object.values(api).forEach((fn) => fn.mockReset())
    showError.mockReset()
    showSuccess.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('only lets runnable entries be selected and omits empty selections from the request', async () => {
    api.startGPTIQTestRun.mockResolvedValue(batch())
    api.getGPTIQTestRun.mockResolvedValue({ batch: batch({ status: 'finished' }), results: [] })
    const wrapper = mountPanel()
    const boxes = wrapper.findAll('[data-testid="gpt-iq-run-entry"]')
    expect((boxes[0].element as HTMLInputElement).disabled).toBe(false)
    expect((boxes[1].element as HTMLInputElement).disabled).toBe(true)

    await wrapper.find('[data-testid="gpt-iq-run-start"]').trigger('click')
    await flushPromises()
    expect(api.startGPTIQTestRun).toHaveBeenCalledWith({})

    api.startGPTIQTestRun.mockClear()
    await boxes[0].setValue(true)
    await wrapper.findAll('[data-testid="gpt-iq-run-case"]')[1].setValue(true)
    // 上一个批次已结束后才能再次开始。
    await vi.advanceTimersByTimeAsync(3000)
    await wrapper.find('[data-testid="gpt-iq-run-start"]').trigger('click')
    await flushPromises()
    expect(api.startGPTIQTestRun).toHaveBeenCalledWith({ entry_ids: [1], case_ids: [11] })
    wrapper.unmount()
  })

  it('polls a running batch every 3 seconds, renders cells and stops after it finishes', async () => {
    api.getGPTIQTestRun
      .mockResolvedValueOnce({ batch: batch(), results: [result({ status: 'running', correct: null, answer_text: '', duration_ms: null, input_tokens: null, output_tokens: null })] })
      .mockResolvedValueOnce({ batch: batch(), results: [result(), result({ id: 101, run_index: 2, status: 'error', correct: null, reason: 'timeout', attempts: 2, error_summary: '<b>upstream</b> timed out', answer_text: '' })] })
      .mockResolvedValueOnce({ batch: batch({ status: 'finished', finished_at: '2026-11-23T06:05:00Z' }), results: [result(), result({ id: 101, run_index: 2, status: 'error', correct: null, reason: 'timeout', attempts: 2, error_summary: '<b>upstream</b> timed out', answer_text: '' })] })
    const wrapper = mountPanel({ runningBatchId: 5 })
    await flushPromises()
    expect(api.getGPTIQTestRun).toHaveBeenCalledTimes(1)
    let cells = wrapper.findAll('[data-testid="gpt-iq-result-cell"]')
    expect(cells.map((cell) => cell.attributes('data-status'))).toEqual(['running', 'pending'])
    expect(wrapper.find('[data-testid="gpt-iq-run-cancel"]').exists()).toBe(true)

    await vi.advanceTimersByTimeAsync(3000)
    cells = wrapper.findAll('[data-testid="gpt-iq-result-cell"]')
    expect(cells.map((cell) => cell.attributes('data-status'))).toEqual(['ok', 'error'])
    expect(cells[0].text()).toContain('✓')
    expect(cells[0].find('[data-testid="gpt-iq-result-answer"]').text()).toBe('最终答案：21')
    const error = cells[1].find('[data-testid="gpt-iq-result-error"]')
    expect(error.text()).toBe('<b>upstream</b> timed out')
    expect(error.element.querySelector('b')).toBeNull()
    expect(cells[1].text()).toContain('gptIQTest.admin.results.attempts')
    expect(wrapper.emitted('refresh')).toBeUndefined()

    await vi.advanceTimersByTimeAsync(3000)
    expect(wrapper.find('[data-testid="gpt-iq-run-status"]').text()).toBe('gptIQTest.admin.batchStatus.finished')
    expect(wrapper.emitted('refresh')).toHaveLength(1)
    expect(wrapper.find('[data-testid="gpt-iq-run-cancel"]').exists()).toBe(false)

    await vi.advanceTimersByTimeAsync(30000)
    expect(api.getGPTIQTestRun).toHaveBeenCalledTimes(3)
    wrapper.unmount()
  })

  it('reports a running batch conflict and asks the page to refresh', async () => {
    api.startGPTIQTestRun.mockRejectedValue({ status: 409, reason: 'GPT_IQ_TEST_BATCH_RUNNING', message: 'busy' })
    const wrapper = mountPanel()
    await wrapper.find('[data-testid="gpt-iq-run-start"]').trigger('click')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('gptIQTest.admin.run.batchRunning')
    expect(wrapper.emitted('refresh')).toHaveLength(1)
    wrapper.unmount()
  })

  it('hides cancel from organization leaders for batches they did not start', async () => {
    api.getGPTIQTestRun.mockResolvedValue({ batch: batch({ trigger: 'schedule', triggered_by: null }), results: [] })
    const wrapper = mountPanel({ runningBatchId: 5, scope: { full_scope: false, groups: ['xunyou'], can_configure: false } })
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-iq-run-cancel"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('uses the scoped total so a leader sees a site-wide batch reach 100%', async () => {
    api.getGPTIQTestRun.mockResolvedValue({
      batch: batch({ trigger: 'schedule', triggered_by: undefined, total_calls: 4, entry_ids: [1, 2] }),
      results: [result(), result({ id: 101, run_index: 2 })],
      visible_total_calls: 2,
    })
    const wrapper = mountPanel({ runningBatchId: 5, scope: { full_scope: false, groups: ['xunyou'], can_configure: false } })
    await flushPromises()
    expect(wrapper.find('.bg-primary-500').attributes('style')).toContain('width: 100%')
    wrapper.unmount()
  })

  it('loads a recent batch into the table when clicked', async () => {
    api.getGPTIQTestRun.mockResolvedValue({ batch: batch({ id: 3, status: 'finished' }), results: [result({ batch_id: 3 })] })
    const summary = { ...batch({ id: 3, status: 'finished' }), completed: 2, ok: 2, errors: 0, skipped: 0, interrupted: 0, graded: 2, correct: 2 }
    const wrapper = mountPanel({ batches: [summary] })
    await wrapper.find('[data-testid="gpt-iq-batch-item"]').trigger('click')
    await flushPromises()
    expect(api.getGPTIQTestRun).toHaveBeenCalledWith(3, expect.any(AbortSignal))
    expect(wrapper.findAll('[data-testid="gpt-iq-result-cell"]')).toHaveLength(2)
    await vi.advanceTimersByTimeAsync(10000)
    expect(api.getGPTIQTestRun).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('shows why a batch ended abnormally and counts interrupted calls separately', async () => {
    api.getGPTIQTestRun.mockResolvedValue({
      batch: batch({ id: 4, status: 'aborted', failure_reason: 'timeout' }),
      results: [
        result({ batch_id: 4, status: 'abandoned', reason: 'abandoned', correct: null, answer_text: '' }),
        result({ id: 101, batch_id: 4, run_index: 2, status: 'cancelled', reason: 'cancelled', correct: null, answer_text: '' }),
      ],
    })
    const summary = { ...batch({ id: 4, status: 'aborted', failure_reason: 'persist_failed' }), completed: 2, ok: 0, errors: 0, skipped: 0, interrupted: 2, graded: 0, correct: 0 }
    const wrapper = mountPanel({ batches: [summary] })
    const row = wrapper.find('[data-testid="gpt-iq-batch-item"]')
    expect(row.find('[data-testid="gpt-iq-batch-failure"]').text()).toBe('gptIQTest.admin.batchFailure.persist_failed')
    await row.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-iq-run-failure"]').text()).toBe('gptIQTest.admin.batchFailure.timeout')
    const cells = wrapper.findAll('[data-testid="gpt-iq-result-cell"]')
    expect(cells.map((cell) => cell.attributes('data-status'))).toEqual(['abandoned', 'cancelled'])
    expect(cells[0].text()).toContain('gptIQTest.runStatus.abandoned')
    // 中断不是失败：不使用失败色。
    expect(cells[0].find('.text-orange-600').exists()).toBe(false)
    wrapper.unmount()
  })

  it('omits the failure note for a normally finished batch', () => {
    const summary = { ...batch({ id: 3, status: 'finished' }), completed: 2, ok: 2, errors: 0, skipped: 0, interrupted: 0, graded: 2, correct: 2 }
    const wrapper = mountPanel({ batches: [summary] })
    expect(wrapper.find('[data-testid="gpt-iq-batch-failure"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
