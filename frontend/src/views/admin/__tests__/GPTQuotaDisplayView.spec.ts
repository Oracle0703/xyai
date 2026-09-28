import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { GPTQuotaAdminView } from '@/api/gptQuotaDisplay'
import GPTQuotaDisplayView from '../GPTQuotaDisplayView.vue'

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
      items: [{ account_id: 13, account_name: 'd-1', group: 'wsdashi', eligible: true, selected: false }],
      total: 1,
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
    await wrapper.find('[data-testid="gpt-quota-add"]').trigger('click')
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
