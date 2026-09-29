import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { GPTQuotaAdminView } from '@/api/gptQuotaDisplay'
import Pagination from '@/components/common/Pagination.vue'
import GPTQuotaDisplayView from '../GPTQuotaDisplayView.vue'

const PICKER_NAMES: Record<number, string> = { 11: 'c-1', 13: 'd-1', 15: 'x-1' }

function pickerItem(wrapper: ReturnType<typeof mount>, accountId: number) {
  const items = wrapper.findAll('[data-testid="gpt-quota-picker-item"]')
  const item = items.find((input) => input.element.closest('li')?.textContent?.includes(PICKER_NAMES[accountId]))
  if (!item) throw new Error(`picker item ${accountId} not found`)
  return item
}

const api = vi.hoisted(() => ({
  getAdminGPTQuota: vi.fn(),
  listGPTQuotaCandidates: vi.fn(),
  saveGPTQuotaConfig: vi.fn(),
  refreshGPTQuotaEntry: vi.fn(),
  refreshAllGPTQuota: vi.fn(),
}))
const { showError, showSuccess } = vi.hoisted(() => ({ showError: vi.fn(), showSuccess: vi.fn() }))

vi.mock('@/api/gptQuotaDisplay', () => api)
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess }) }))
vi.mock('@/composables/useGPTQuotaVisibility', () => ({ useGPTQuotaVisibility: () => ({ setGPTQuotaVisibility: vi.fn() }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('@/components/common/BaseDialog.vue', () => ({
  default: { props: ['show'], template: '<div v-if="show" data-testid="dialog"><slot /><slot name="footer" /></div>' },
}))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key, te: () => true }) }
})

function adminView(): GPTQuotaAdminView {
  const base = { five_hour: null, seven_day: null, sampled_at: null, stale: false, last_attempt_at: null, last_attempt_status: 'never', retry_after: null }
  return {
    config: { enabled: true, interval_minutes: 30, start_time: '09:30', end_time: '18:00', version: 3, updated_at: '2026-09-24T00:00:00Z' },
    server_time: '2026-09-24T04:00:00Z',
    schedule: { start: '09:30', end: '18:00', interval_minutes: 30, timezone: 'Asia/Shanghai' },
    next_scheduled_at: '2026-09-24T04:30:00Z',
    in_schedule_window: true,
    batch: { running: false, total: 0, succeeded: 0, failed: 0, skipped: 0 },
    entries: [
      { ...base, id: 1, account_id: 11, account_name: 'c-1', group: 'xunyou', display_name: '迅游一号', effective_display_name: '迅游一号', eligible: true },
      { ...base, id: 2, account_id: 12, account_name: '', group: '', display_name: '', effective_display_name: 'gpt-#2', eligible: false, reason: 'account_not_found' },
    ],
  }
}

describe('GPTQuotaDisplayView', () => {
  beforeEach(() => {
    Object.values(api).forEach((fn) => fn.mockReset())
    showError.mockReset()
    showSuccess.mockReset()
    api.getAdminGPTQuota.mockResolvedValue(adminView())
    api.listGPTQuotaCandidates.mockResolvedValue({
      items: [
        { account_id: 11, account_name: 'c-1', group: 'xunyou', eligible: true, selected: true },
        { account_id: 13, account_name: 'd-1', group: 'wsdashi', eligible: true, selected: false },
        { account_id: 15, account_name: 'x-1', group: '', eligible: false, reason: 'no_display_prefix', selected: false },
      ],
      total: 3,
      page: 1,
      page_size: 20,
    })
    api.saveGPTQuotaConfig.mockResolvedValue({ config: { ...adminView().config, version: 4 } })
  })

  it('lets admins remove an entry that lost eligibility and saves the remaining selection', async () => {
    const wrapper = mount(GPTQuotaDisplayView)
    await flushPromises()

    const rows = wrapper.findAll('[data-testid="gpt-quota-selected-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[1].text()).toContain('gptQuota.admin.reasons.account_not_found')
    await rows[1].find('[data-testid="gpt-quota-remove"]').trigger('click')
    await wrapper.find('[data-testid="gpt-quota-pick"]').trigger('click')
    await flushPromises()
    await pickerItem(wrapper, 13).trigger('change')
    await wrapper.find('[data-testid="gpt-quota-picker-confirm"]').trigger('click')
    await wrapper.find('[data-testid="gpt-quota-save"]').trigger('click')
    await flushPromises()

    expect(api.saveGPTQuotaConfig).toHaveBeenCalledWith({
      enabled: true,
      interval_minutes: 30,
      expected_version: 3,
      entries: [
        { account_id: 11, display_name: '迅游一号' },
        { account_id: 13, display_name: '' },
      ],
    })
    expect(showSuccess).toHaveBeenCalled()
  })

  it('pre-checks selected accounts in the picker and applies the ticks only on save config', async () => {
    const wrapper = mount(GPTQuotaDisplayView)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-quota-pick"]').trigger('click')
    await flushPromises()

    expect((pickerItem(wrapper, 11).element as HTMLInputElement).checked).toBe(true)
    expect((pickerItem(wrapper, 13).element as HTMLInputElement).checked).toBe(false)
    expect(pickerItem(wrapper, 15).attributes('disabled')).toBeDefined()

    await pickerItem(wrapper, 11).trigger('change')
    await pickerItem(wrapper, 13).trigger('change')
    await wrapper.find('[data-testid="gpt-quota-picker-confirm"]').trigger('click')
    expect(wrapper.find('[data-testid="dialog"]').exists()).toBe(false)
    expect(api.saveGPTQuotaConfig).not.toHaveBeenCalled()
    // 12 不在候选页上，也保留勾选。
    const rows = wrapper.findAll('[data-testid="gpt-quota-selected-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[1].text()).toContain('d-1')

    await wrapper.find('[data-testid="gpt-quota-save"]').trigger('click')
    await flushPromises()
    expect(api.saveGPTQuotaConfig).toHaveBeenCalledWith(expect.objectContaining({
      entries: [
        { account_id: 12, display_name: '' },
        { account_id: 13, display_name: '' },
      ],
    }))
  })

  it('discards picker ticks on cancel and restores the saved alias when an account is re-checked', async () => {
    const wrapper = mount(GPTQuotaDisplayView)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-quota-pick"]').trigger('click')
    await flushPromises()
    await pickerItem(wrapper, 13).trigger('change')
    await wrapper.find('[data-testid="dialog"] .btn-secondary').trigger('click')
    expect(wrapper.findAll('[data-testid="gpt-quota-selected-row"]')).toHaveLength(2)

    await wrapper.findAll('[data-testid="gpt-quota-remove"]')[0].trigger('click')
    await wrapper.find('[data-testid="gpt-quota-pick"]').trigger('click')
    await flushPromises()
    expect((pickerItem(wrapper, 13).element as HTMLInputElement).checked).toBe(false)
    await pickerItem(wrapper, 11).trigger('change')
    await wrapper.find('[data-testid="gpt-quota-picker-confirm"]').trigger('click')
    await wrapper.find('[data-testid="gpt-quota-save"]').trigger('click')
    await flushPromises()
    expect(api.saveGPTQuotaConfig).toHaveBeenCalledWith(expect.objectContaining({
      entries: [
        { account_id: 12, display_name: '' },
        { account_id: 11, display_name: '迅游一号' },
      ],
    }))
  })

  it('lets admins uncheck a selected account that the candidate list no longer returns', async () => {
    const wrapper = mount(GPTQuotaDisplayView)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-quota-pick"]').trigger('click')
    await flushPromises()

    const chips = wrapper.findAll('[data-testid="gpt-quota-picker-chip"]')
    expect(chips.map((chip) => chip.text())).toEqual([expect.stringContaining('c-1'), expect.stringContaining('#12')])
    expect(chips[1].classes()).toContain('badge-danger')
    await chips[1].find('button').trigger('click')
    await wrapper.find('[data-testid="gpt-quota-picker-confirm"]').trigger('click')
    await wrapper.find('[data-testid="gpt-quota-save"]').trigger('click')
    await flushPromises()
    expect(api.saveGPTQuotaConfig).toHaveBeenCalledWith(expect.objectContaining({ entries: [{ account_id: 11, display_name: '迅游一号' }] }))
  })

  it('clears stale candidates when a page load fails and retries the same page', async () => {
    const page1 = { items: [{ account_id: 13, account_name: 'd-1', group: 'wsdashi', eligible: true, selected: false }], total: 25, page: 1, page_size: 20 }
    const page2 = { items: [{ account_id: 16, account_name: 'd-21', group: 'wsdashi', eligible: true, selected: false }], total: 25, page: 2, page_size: 20 }
    api.listGPTQuotaCandidates.mockReset()
    api.listGPTQuotaCandidates.mockResolvedValueOnce(page1).mockRejectedValueOnce({ message: 'boom' }).mockResolvedValueOnce(page2)
    const wrapper = mount(GPTQuotaDisplayView)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-quota-pick"]').trigger('click')
    await flushPromises()

    wrapper.findComponent(Pagination).vm.$emit('update:page', 2)
    await flushPromises()
    expect(wrapper.findAll('[data-testid="gpt-quota-picker-item"]')).toHaveLength(0)
    expect(wrapper.find('[data-testid="gpt-quota-picker-error"]').text()).toContain('boom')
    expect(wrapper.findComponent(Pagination).props('page')).toBe(2)

    await wrapper.find('[data-testid="gpt-quota-picker-retry"]').trigger('click')
    await flushPromises()
    expect(api.listGPTQuotaCandidates).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 }))
    expect(wrapper.find('[data-testid="gpt-quota-picker-error"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('d-21')
  })

  it('does not show the previous search results when reopening fails to load', async () => {
    const wrapper = mount(GPTQuotaDisplayView)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-quota-pick"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="gpt-quota-picker-item"]')).toHaveLength(3)
    await wrapper.find('[data-testid="dialog"] .btn-secondary').trigger('click')

    api.listGPTQuotaCandidates.mockRejectedValueOnce({ message: 'down' })
    await wrapper.find('[data-testid="gpt-quota-pick"]').trigger('click')
    await flushPromises()
    expect(wrapper.findAll('[data-testid="gpt-quota-picker-item"]')).toHaveLength(0)
    expect(wrapper.find('[data-testid="gpt-quota-picker-error"]').exists()).toBe(true)
  })

  it('keeps the edit base version while polling a running batch', async () => {
    vi.useFakeTimers()
    try {
      api.getAdminGPTQuota.mockResolvedValueOnce({ ...adminView(), batch: { running: true, total: 1, succeeded: 0, failed: 0, skipped: 0 } })
      api.getAdminGPTQuota.mockResolvedValueOnce({ ...adminView(), config: { ...adminView().config, version: 9 } })
      const wrapper = mount(GPTQuotaDisplayView)
      await flushPromises()
      await vi.advanceTimersByTimeAsync(5000)
      await flushPromises()
      expect(api.getAdminGPTQuota).toHaveBeenCalledTimes(2)
      await wrapper.find('[data-testid="gpt-quota-save"]').trigger('click')
      await flushPromises()
      expect(api.saveGPTQuotaConfig).toHaveBeenCalledWith(expect.objectContaining({ expected_version: 3 }))
    } finally {
      vi.useRealTimers()
    }
  })

  it('maps a version conflict to a reload hint', async () => {
    api.saveGPTQuotaConfig.mockRejectedValue({ status: 409, reason: 'GPT_QUOTA_CONFIG_CONFLICT', message: 'changed' })
    const wrapper = mount(GPTQuotaDisplayView)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-quota-save"]').trigger('click')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('gptQuota.admin.conflict')
  })

  it('reports a running batch instead of starting another one', async () => {
    api.refreshAllGPTQuota.mockRejectedValue({ status: 409, reason: 'GPT_QUOTA_BATCH_RUNNING', message: 'running' })
    const wrapper = mount(GPTQuotaDisplayView)
    await flushPromises()
    await wrapper.find('[data-testid="gpt-quota-refresh-all"]').trigger('click')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('gptQuota.admin.batchRunning')
  })
})
