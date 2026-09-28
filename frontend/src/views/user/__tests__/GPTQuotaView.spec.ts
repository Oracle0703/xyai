import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { GPTQuotaUserView } from '@/api/gptQuotaDisplay'
import GPTQuotaView from '../GPTQuotaView.vue'

const { getGPTQuotaDisplay, setGPTQuotaVisibility } = vi.hoisted(() => ({
  getGPTQuotaDisplay: vi.fn(),
  setGPTQuotaVisibility: vi.fn(),
}))

vi.mock('@/api/gptQuotaDisplay', () => ({ getGPTQuotaDisplay }))
vi.mock('@/composables/useGPTQuotaVisibility', () => ({
  useGPTQuotaVisibility: () => ({ setGPTQuotaVisibility }),
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

function setHidden(hidden: boolean) {
  Object.defineProperty(document, 'hidden', { configurable: true, get: () => hidden })
}

describe('GPTQuotaView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    setHidden(false)
    getGPTQuotaDisplay.mockReset()
    setGPTQuotaVisibility.mockReset()
    getGPTQuotaDisplay.mockResolvedValue(view())
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
    expect(setGPTQuotaVisibility).toHaveBeenCalledWith(true)
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
})
