import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { GPTQuotaUserView } from '@/api/gptQuotaDisplay'
import type { GPTIQUserView } from '@/api/gptIQTest'
import GPTQuotaView from '../GPTQuotaView.vue'

const { getGPTQuotaDisplay, getGPTIQTests, getGPTIQTestEntry, setGPTQuotaDisplayVisibility, setGPTIQTestVisibility } = vi.hoisted(() => ({
  getGPTQuotaDisplay: vi.fn(),
  getGPTIQTests: vi.fn(),
  getGPTIQTestEntry: vi.fn(),
  setGPTQuotaDisplayVisibility: vi.fn(),
  setGPTIQTestVisibility: vi.fn(),
}))

vi.mock('@/api/gptQuotaDisplay', () => ({ getGPTQuotaDisplay }))
vi.mock('@/api/gptIQTest', () => ({ getGPTIQTests, getGPTIQTestEntry }))
vi.mock('@/composables/useGPTQuotaVisibility', () => ({
  useGPTQuotaVisibility: () => ({ setGPTQuotaDisplayVisibility, setGPTIQTestVisibility }),
}))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const POLL_MS = 15 * 60 * 1000

function view(overrides: Partial<GPTQuotaUserView> = {}): GPTQuotaUserView {
  return {
    enabled: true,
    server_time: '2026-09-24T04:00:00Z',
    poll_interval_seconds: 900,
    schedule: { start: '09:30', end: '18:00', interval_minutes: 30, timezone: 'Asia/Shanghai' },
    next_scheduled_at: '2026-09-24T04:30:00Z',
    in_schedule_window: true,
    groups: {
      xunyou: [{ id: 1, display_name: 'c-01', five_hour: { remaining_percent: 80 }, seven_day: null, sampled_at: '2026-09-24T03:30:00Z', stale: false }],
      wsdashi: [],
    },
    ...overrides,
  }
}

function iqView(overrides: Partial<GPTIQUserView> = {}): GPTIQUserView {
  return {
    enabled: true,
    server_time: '2026-09-24T04:00:00Z',
    poll_interval_seconds: 900,
    schedule: { enabled: true, run_time: '08:30', timezone: 'Asia/Shanghai' },
    next_run_at: '2026-09-25T00:30:00Z',
    thresholds: { min_accuracy_percent: 60, min_graded_runs: 3, reasoning_ratio_percent: 30 },
    cases: [{ id: 1, name: '打招呼', kind: 'chat', model: 'gpt-6.1', runs: 1, public_answer: true }],
    groups: {
      xunyou: [{ id: 1, display_name: 'c-01', verdict: 'connected', reasons: [], tested_at: '2026-09-24T00:31:00Z', cases: [], trend: [] }],
      wsdashi: [],
    },
    ...overrides,
  }
}

function setHidden(hidden: boolean) {
  Object.defineProperty(document, 'hidden', { configurable: true, get: () => hidden })
}

describe('GPTQuotaView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    setHidden(false)
    getGPTQuotaDisplay.mockReset()
    getGPTIQTests.mockReset()
    getGPTIQTestEntry.mockReset()
    setGPTQuotaDisplayVisibility.mockReset()
    setGPTIQTestVisibility.mockReset()
    getGPTQuotaDisplay.mockResolvedValue(view())
    getGPTIQTests.mockResolvedValue(iqView({ enabled: false, groups: { xunyou: [], wsdashi: [] } }))
  })

  afterEach(() => {
    vi.useRealTimers()
    setHidden(false)
  })

  it('renders xunyou and subao columns and syncs menu visibility', async () => {
    const wrapper = mount(GPTQuotaView)
    await flushPromises()
    expect(wrapper.find('[data-testid="gpt-quota-column-xunyou"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="gpt-quota-column-wsdashi"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('80.0%')
    expect(setGPTQuotaDisplayVisibility).toHaveBeenCalledWith(true)
    expect(setGPTIQTestVisibility).toHaveBeenCalledWith(false)
    // 降智测试关闭时不显示该区。
    expect(wrapper.find('[data-testid="gpt-iq-section"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('summarizes accounts, the 7-day average and low quota in dashboard tiles', async () => {
    const sampled = '2026-09-24T03:30:00Z'
    getGPTQuotaDisplay.mockResolvedValue(view({
      groups: {
        xunyou: [
          { id: 1, display_name: 'c-01', five_hour: null, seven_day: { remaining_percent: 90 }, sampled_at: sampled, stale: false },
          { id: 2, display_name: 'c-02', five_hour: null, seven_day: { remaining_percent: 8 }, sampled_at: sampled, stale: false },
        ],
        wsdashi: [
          { id: 3, display_name: 'd-01', five_hour: null, seven_day: { remaining_percent: 25 }, sampled_at: sampled, stale: false },
          { id: 4, display_name: 'd-02', five_hour: null, seven_day: null, sampled_at: null, stale: false },
        ],
      },
    }))
    const wrapper = mount(GPTQuotaView)
    await flushPromises()
    const summary = wrapper.find('[data-testid="gpt-quota-summary"]').text()
    expect(summary).toContain('4')
    expect(summary).toContain('41.0%')
    expect(wrapper.find('[data-testid="gpt-quota-summary-low"]').text()).toContain('2')
    wrapper.unmount()
  })

  it('shows the disabled state without columns', async () => {
    getGPTQuotaDisplay.mockResolvedValue(view({ enabled: false, groups: { xunyou: [], wsdashi: [] } }))
    const wrapper = mount(GPTQuotaView)
    await flushPromises()
    expect(wrapper.text()).toContain('gptQuota.disabled')
    expect(wrapper.find('[data-testid="gpt-quota-column-xunyou"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows the off-hours notice outside the schedule window', async () => {
    getGPTQuotaDisplay.mockResolvedValue(view({ in_schedule_window: false }))
    const wrapper = mount(GPTQuotaView)
    await flushPromises()
    expect(wrapper.text()).toContain('gptQuota.outsideWindow')
    wrapper.unmount()
  })

  it('polls every 15 minutes, pauses while hidden and catches up once when visible', async () => {
    const wrapper = mount(GPTQuotaView)
    await flushPromises()
    expect(getGPTQuotaDisplay).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(POLL_MS)
    expect(getGPTQuotaDisplay).toHaveBeenCalledTimes(2)

    setHidden(true)
    await vi.advanceTimersByTimeAsync(POLL_MS)
    expect(getGPTQuotaDisplay).toHaveBeenCalledTimes(2)

    setHidden(false)
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(getGPTQuotaDisplay).toHaveBeenCalledTimes(3)

    // 刚读取过，再次切回可见不补读。
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(getGPTQuotaDisplay).toHaveBeenCalledTimes(3)

    // 补读后重新计时，定时器不会紧接着再读一次。
    await vi.advanceTimersByTimeAsync(POLL_MS - 1000)
    expect(getGPTQuotaDisplay).toHaveBeenCalledTimes(3)
    await vi.advanceTimersByTimeAsync(1000)
    expect(getGPTQuotaDisplay).toHaveBeenCalledTimes(4)
    wrapper.unmount()
  })

  it('keeps the current screen on read failure and stops polling after unmount', async () => {
    const wrapper = mount(GPTQuotaView)
    await flushPromises()
    getGPTQuotaDisplay.mockRejectedValueOnce(new Error('boom'))
    await vi.advanceTimersByTimeAsync(POLL_MS)
    expect(wrapper.text()).toContain('gptQuota.loadFailed')
    expect(wrapper.text()).toContain('80.0%')

    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(POLL_MS * 2)
    expect(getGPTQuotaDisplay).toHaveBeenCalledTimes(2)
  })

  it('renders the IQ test section below the quota area when only the IQ test is enabled', async () => {
    getGPTQuotaDisplay.mockResolvedValue(view({ enabled: false, groups: { xunyou: [], wsdashi: [] } }))
    getGPTIQTests.mockResolvedValue(iqView())
    const wrapper = mount(GPTQuotaView)
    await flushPromises()
    expect(wrapper.text()).toContain('gptQuota.disabled')
    expect(wrapper.find('[data-testid="gpt-iq-section"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="gpt-iq-column-xunyou"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="gpt-iq-column-wsdashi"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="gpt-iq-card"]')).toHaveLength(1)
    expect(setGPTQuotaDisplayVisibility).toHaveBeenCalledWith(false)
    expect(setGPTIQTestVisibility).toHaveBeenCalledWith(true)
    wrapper.unmount()
  })

  it('keeps the quota area when the IQ test request fails, and vice versa', async () => {
    getGPTIQTests.mockRejectedValue(new Error('iq down'))
    const wrapper = mount(GPTQuotaView)
    await flushPromises()
    expect(wrapper.text()).toContain('80.0%')
    expect(wrapper.text()).not.toContain('gptQuota.loadFailed')
    expect(wrapper.find('[data-testid="gpt-iq-load-error"]').exists()).toBe(true)
    expect(setGPTIQTestVisibility).not.toHaveBeenCalled()
    wrapper.unmount()

    getGPTQuotaDisplay.mockRejectedValue(new Error('quota down'))
    getGPTIQTests.mockResolvedValue(iqView())
    const second = mount(GPTQuotaView)
    await flushPromises()
    expect(second.text()).toContain('gptQuota.loadFailed')
    expect(second.find('[data-testid="gpt-iq-load-error"]').exists()).toBe(false)
    expect(second.findAll('[data-testid="gpt-iq-card"]')).toHaveLength(1)
    second.unmount()
  })

  it('polls the IQ results on the same 15-minute timer with its own request', async () => {
    const wrapper = mount(GPTQuotaView)
    await flushPromises()
    expect(getGPTIQTests).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(POLL_MS)
    expect(getGPTQuotaDisplay).toHaveBeenCalledTimes(2)
    expect(getGPTIQTests).toHaveBeenCalledTimes(2)
    setHidden(true)
    await vi.advanceTimersByTimeAsync(POLL_MS)
    expect(getGPTIQTests).toHaveBeenCalledTimes(2)
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(POLL_MS * 2)
    expect(getGPTIQTests).toHaveBeenCalledTimes(2)
  })

  it('aborts the in-flight IQ request on unmount', async () => {
    let signal: AbortSignal | undefined
    getGPTIQTests.mockImplementation((value: AbortSignal) => {
      signal = value
      return new Promise(() => {})
    })
    const wrapper = mount(GPTQuotaView)
    await flushPromises()
    expect(signal?.aborted).toBe(false)
    wrapper.unmount()
    expect(signal?.aborted).toBe(true)
  })

  it('opens the detail drawer for a card and loads the entry detail', async () => {
    getGPTIQTests.mockResolvedValue(iqView())
    getGPTIQTestEntry.mockResolvedValue({ enabled: true, id: 1, display_name: 'c-01', days: 14, cases: iqView().cases, sets: [] })
    const wrapper = mount(GPTQuotaView, { attachTo: document.body })
    await flushPromises()
    await wrapper.find('[data-testid="gpt-iq-card"]').trigger('click')
    await flushPromises()
    expect(getGPTIQTestEntry).toHaveBeenCalledWith(1, { days: 14 }, expect.any(AbortSignal))
    expect(document.body.querySelector('[data-testid="gpt-iq-drawer"]')).not.toBeNull()
    wrapper.unmount()
  })
})
