import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { GPTIQAdminView } from '@/api/admin/gptIQTest'
import GPTIQTestConfigView from '../GPTIQTestConfigView.vue'

const api = vi.hoisted(() => ({
  getAdminGPTIQTest: vi.fn(),
  updateGPTIQTestConfig: vi.fn(),
  createGPTIQTestCase: vi.fn(),
  updateGPTIQTestCase: vi.fn(),
  deleteGPTIQTestCase: vi.fn(),
  rejudgeGPTIQTestCase: vi.fn(),
  startGPTIQTestRun: vi.fn(),
  getGPTIQTestRun: vi.fn(),
  cancelGPTIQTestRun: vi.fn(),
}))
const { showError, showSuccess, setGPTIQTestVisibility } = vi.hoisted(() => ({ showError: vi.fn(), showSuccess: vi.fn(), setGPTIQTestVisibility: vi.fn() }))

vi.mock('@/api/admin/gptIQTest', () => api)
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: { id: 1 } }) }))
vi.mock('@/composables/useGPTQuotaVisibility', () => ({ useGPTQuotaVisibility: () => ({ setGPTIQTestVisibility }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('@/components/common/BaseDialog.vue', () => ({
  default: { props: ['show', 'title'], template: '<div v-if="show" data-testid="dialog"><slot /><slot name="footer" /></div>' },
}))
vi.mock('vue-router', () => ({ onBeforeRouteLeave: vi.fn() }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key, te: () => true }) }
})

function adminView(): GPTIQAdminView {
  return {
    config: {
      enabled: true, schedule_enabled: true, run_time: '08:30', min_accuracy_percent: 60, min_graded_runs: 3, reasoning_ratio_percent: 30,
      min_five_hour_percent: 10, min_seven_day_percent: 5, daily_call_budget: 300, retention_days: 90, version: 4, updated_at: '2026-11-23T00:00:00Z',
    },
    server_time: '2026-11-23T04:00:00Z',
    timezone: 'Asia/Shanghai',
    next_run_at: '2026-11-24T00:30:00Z',
    scope: { full_scope: true, groups: [], can_configure: true },
    cases: [{
      id: 10, name: '糖果题', enabled: true, sort_order: 20, public_prompt: true, public_answer: true, kind: 'graded', prompt: '题\n面',
      model: 'gpt-6.1-sol', reasoning_effort: 'high', runs: 3, timeout_seconds: 300, scoring_mode: 'final_line', scoring_expected: '21',
      version: 7, revision_id: 3, revision: 2, prompt_sha256: 'x', updated_at: '2026-11-23T00:00:00Z',
    }],
    templates: [{ key: 'hi', input: { name: '打招呼', enabled: true, sort_order: 10, public_prompt: true, public_answer: true, kind: 'chat', prompt: 'hi', model: 'gpt-6.1', reasoning_effort: '', runs: 1, timeout_seconds: 60, scoring_mode: 'none', scoring_expected: '' } }],
    entries: [{ id: 1, display_name: 'c-01', group: 'xunyou', runnable: true }],
    batches: [],
    running_batch_id: null,
    dispatched_today: 12,
  }
}

describe('GPTIQTestConfigView', () => {
  beforeEach(() => {
    Object.values(api).forEach((fn) => fn.mockReset())
    showError.mockReset()
    showSuccess.mockReset()
    setGPTIQTestVisibility.mockReset()
    api.getAdminGPTIQTest.mockResolvedValue(adminView())
  })

  it('shows today\'s dispatched calls against the daily budget', async () => {
    const wrapper = mount(GPTIQTestConfigView)
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-iq-admin-budget"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="gpt-iq-case-row"]')).toHaveLength(1)
    wrapper.unmount()
  })

  it('saves the config with expected_version and syncs the menu visibility', async () => {
    api.updateGPTIQTestConfig.mockResolvedValue({ ...adminView().config, enabled: false, version: 5 })
    const wrapper = mount(GPTIQTestConfigView)
    await flushPromises()
    const save = wrapper.find('[data-testid="gpt-iq-config-save"]')
    expect((save.element as HTMLButtonElement).disabled).toBe(true)
    await wrapper.find('[data-testid="gpt-iq-config-enabled"]').trigger('click')
    await wrapper.find('[data-testid="gpt-iq-config-daily_call_budget"]').setValue('500')
    expect((save.element as HTMLButtonElement).disabled).toBe(false)
    await save.trigger('click')
    await flushPromises()
    expect(api.updateGPTIQTestConfig).toHaveBeenCalledWith(expect.objectContaining({ enabled: false, daily_call_budget: 500, expected_version: 4 }))
    expect(setGPTIQTestVisibility).toHaveBeenCalledWith(false)
    wrapper.unmount()
  })

  it('blocks saving a run time outside 05:00–11:00 and maps a version conflict to a reload hint', async () => {
    api.updateGPTIQTestConfig.mockRejectedValue({ status: 409, reason: 'GPT_IQ_TEST_CONFIG_CONFLICT', message: 'conflict' })
    const wrapper = mount(GPTIQTestConfigView)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-iq-config-run-time"]').setValue('12:00')
    expect((wrapper.find('[data-testid="gpt-iq-config-save"]').element as HTMLButtonElement).disabled).toBe(true)
    await wrapper.find('[data-testid="gpt-iq-config-run-time"]').setValue('09:00')
    await wrapper.find('[data-testid="gpt-iq-config-save"]').trigger('click')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('gptIQTest.admin.conflict')
    expect(setGPTIQTestVisibility).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('deletes a case with its expected version after confirmation', async () => {
    api.deleteGPTIQTestCase.mockResolvedValue({ deleted: true })
    const wrapper = mount(GPTIQTestConfigView)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-iq-case-delete"]').trigger('click')
    const confirm = wrapper.findAll('[data-testid="dialog"] button').find((button) => button.text() === 'common.confirm')
    expect(confirm).toBeTruthy()
    await confirm!.trigger('click')
    await flushPromises()
    expect(api.deleteGPTIQTestCase).toHaveBeenCalledWith(10, 7)
    wrapper.unmount()
  })

  it('re-scores a graded case and shows the counts', async () => {
    api.rejudgeGPTIQTestCase.mockResolvedValue({ checked: 9, updated: 2, skipped_truncated: 1 })
    const wrapper = mount(GPTIQTestConfigView)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-iq-case-rejudge"]').trigger('click')
    await flushPromises()
    // 带上当前版本 ID，只重算该版本的结果。
    expect(api.rejudgeGPTIQTestCase).toHaveBeenCalledWith(10, 3)
    expect(wrapper.find('[data-testid="gpt-iq-rejudge-result"]').text()).toBe('gptIQTest.admin.cases.rejudgeDone')
    wrapper.unmount()
  })

  it('asks to reload when the case changed before re-scoring', async () => {
    api.rejudgeGPTIQTestCase.mockRejectedValue({ status: 409, reason: 'GPT_IQ_TEST_CASE_CONFLICT', message: 'conflict' })
    const wrapper = mount(GPTIQTestConfigView)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-iq-case-rejudge"]').trigger('click')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('gptIQTest.admin.cases.rejudgeConflict')
    expect(wrapper.find('[data-testid="gpt-iq-rejudge-result"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('renders settings and cases read-only for an organization leader without the config permission', async () => {
    api.getAdminGPTIQTest.mockResolvedValue({ ...adminView(), scope: { full_scope: false, groups: ['xunyou'], can_configure: false }, templates: [] })
    const wrapper = mount(GPTIQTestConfigView)
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-iq-readonly"]').exists()).toBe(true)
    const fields = wrapper.find('[data-testid="gpt-iq-config-fields"]')
    expect((fields.element as HTMLFieldSetElement).disabled).toBe(true)
    expect(fields.find('[data-testid="gpt-iq-config-daily_call_budget"]').element.matches(':disabled')).toBe(true)
    expect(fields.find('[data-testid="gpt-iq-config-enabled"]').element.matches(':disabled')).toBe(true)
    expect(wrapper.find('[data-testid="gpt-iq-config-bar"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="gpt-iq-config-save"]').exists()).toBe(false)
    for (const id of ['gpt-iq-template-menu', 'gpt-iq-case-add', 'gpt-iq-case-edit', 'gpt-iq-case-delete', 'gpt-iq-case-rejudge']) {
      expect(wrapper.find(`[data-testid="${id}"]`).exists()).toBe(false)
    }
    // 测试项列表与在页面上测试仍可用。
    expect(wrapper.findAll('[data-testid="gpt-iq-case-row"]')).toHaveLength(1)
    expect(wrapper.find('[data-testid="gpt-iq-run-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="gpt-iq-run-start"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('prefills the editor from a template and creates the case keeping prompt newlines', async () => {
    api.createGPTIQTestCase.mockResolvedValue({ ...adminView().cases[0], id: 11 })
    const wrapper = mount(GPTIQTestConfigView)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-iq-template-menu"]').trigger('click')
    await wrapper.find('[data-testid="gpt-iq-template-hi"]').trigger('click')
    expect((wrapper.find('[data-testid="gpt-iq-case-name"]').element as HTMLInputElement).value).toBe('打招呼')
    await wrapper.find('[data-testid="gpt-iq-case-prompt"]').setValue('hi\n  there')
    await wrapper.find('[data-testid="gpt-iq-case-form"]').trigger('submit')
    await flushPromises()
    expect(api.createGPTIQTestCase).toHaveBeenCalledWith(expect.objectContaining({ name: '打招呼', kind: 'chat', prompt: 'hi\n  there', scoring_mode: 'none', sort_order: 30 }))
    wrapper.unmount()
  })

  it('updates a case with its expected version and shows a conflict inside the dialog', async () => {
    api.updateGPTIQTestCase.mockRejectedValue({ status: 409, reason: 'GPT_IQ_TEST_CASE_CONFLICT', message: 'conflict' })
    const wrapper = mount(GPTIQTestConfigView)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-iq-case-edit"]').trigger('click')
    await wrapper.find('[data-testid="gpt-iq-case-form"]').trigger('submit')
    await flushPromises()
    expect(api.updateGPTIQTestCase).toHaveBeenCalledWith(10, expect.objectContaining({ scoring_mode: 'final_line', scoring_expected: '21', prompt: '题\n面' }), 7)
    expect(wrapper.find('[data-testid="gpt-iq-case-error"]').text()).toBe('gptIQTest.admin.cases.conflict')
    wrapper.unmount()
  })
})
