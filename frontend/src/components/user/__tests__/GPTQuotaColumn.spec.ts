import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { GPTQuotaUserCard } from '@/api/gptQuotaDisplay'
import GPTQuotaColumn from '../GPTQuotaColumn.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => (params ? `${key}:${JSON.stringify(params)}` : key) }),
  }
})

const NOW = Date.parse('2026-09-24T04:00:00Z')

function card(overrides: Partial<GPTQuotaUserCard> = {}): GPTQuotaUserCard {
  return {
    id: 1,
    display_name: 'c-01',
    five_hour: { remaining_percent: 75, reset_at: '2026-09-24T05:00:00Z', reset_time_source: 'reset_at' },
    seven_day: { remaining_percent: 40.25, reset_at: '2026-09-30T00:00:00Z', reset_time_source: 'reset_at' },
    sampled_at: '2026-09-24T03:30:00Z',
    stale: false,
    ...overrides,
  }
}

function mountColumn(items: GPTQuotaUserCard[]) {
  return mount(GPTQuotaColumn, { props: { groupKey: 'xunyou', title: '迅游', items, now: NOW } })
}

describe('GPTQuotaColumn', () => {
  it('renders both windows with one decimal and the sample time', () => {
    const text = mountColumn([card()]).text()
    expect(text).toContain('迅游')
    expect(text).toContain('75.0%')
    expect(text).toContain('40.3%')
    expect(text).toContain('gptQuota.sampledAt')
    expect(text).not.toContain('gptQuota.stale')
  })

  it('distinguishes missing windows, no data and stale data', () => {
    const wrapper = mountColumn([
      card({ id: 1, seven_day: null, stale: true }),
      card({ id: 2, display_name: 'c-02', five_hour: null, seven_day: null, sampled_at: null }),
    ])
    const cards = wrapper.findAll('[data-testid="gpt-quota-card"]')
    expect(cards[0].text()).toContain('gptQuota.notProvided')
    expect(cards[0].text()).toContain('gptQuota.stale')
    expect(cards[1].text()).toContain('gptQuota.noData')
    expect(cards[1].text()).not.toContain('%')
  })

  it('shows reset pending instead of assuming a full quota after the reset time', () => {
    const text = mountColumn([card({ five_hour: { remaining_percent: 3, reset_at: '2026-09-24T03:59:00Z' } })]).text()
    expect(text).toContain('gptQuota.resetPending')
    expect(text).toContain('3.0%')
    expect(text).not.toContain('100.0%')
  })

  it('marks a missing reset time as unknown', () => {
    const text = mountColumn([card({ five_hour: { remaining_percent: 50 } })]).text()
    expect(text).toContain('gptQuota.resetUnknown')
  })

  it('hides the 5-hour window when it is not provided and shows it again once reported', () => {
    const wrapper = mountColumn([
      card({ id: 1, five_hour: null }),
      card({ id: 2, display_name: 'c-02' }),
    ])
    const cards = wrapper.findAll('[data-testid="gpt-quota-card"]')
    expect(cards[0].findAll('[data-testid="gpt-quota-meter"]')).toHaveLength(1)
    expect(cards[0].text()).not.toContain('gptQuota.fiveHour')
    expect(cards[1].findAll('[data-testid="gpt-quota-meter"]')).toHaveLength(2)
    expect(cards[1].text()).toContain('gptQuota.fiveHour')
  })

  it('labels each account by its tightest window, not by color alone', () => {
    const wrapper = mountColumn([
      card({ id: 1, five_hour: null, seven_day: { remaining_percent: 80 } }),
      card({ id: 2, display_name: 'c-02', five_hour: { remaining_percent: 8 }, seven_day: { remaining_percent: 60 } }),
      card({ id: 3, display_name: 'c-03', five_hour: null, seven_day: { remaining_percent: 25 } }),
    ])
    const levels = wrapper.findAll('[data-testid="gpt-quota-level"]').map((badge) => badge.text())
    expect(levels).toEqual(['gptQuota.levels.good', 'gptQuota.levels.critical', 'gptQuota.levels.low'])
    expect(wrapper.find('[data-testid="gpt-quota-group-average"]').text()).toContain('55.0%')
  })

  it('keeps an empty column visible', () => {
    const wrapper = mount(GPTQuotaColumn, { props: { groupKey: 'wsdashi', title: '速宝', items: [], now: NOW } })
    expect(wrapper.text()).toContain('速宝')
    expect(wrapper.text()).toContain('gptQuota.emptyGroup')
  })
})
