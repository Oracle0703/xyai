import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import type {
  OrganizationUsageChampions,
  OrganizationUsageOverview as OrganizationUsageOverviewData,
  OrganizationUsageRange
} from '@/api/admin/organizationUsage'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import { formatDateTime } from '@/utils/format'
import OrganizationUsageOverview from '../OrganizationUsageOverview.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, values?: Record<string, string>) => {
        if (key === 'admin.organizationUsage.organizations.other') return '其他'
        return values?.value ? `${key}:${values.value}` : key
      }
    })
  }
})

const overview: OrganizationUsageOverviewData = {
  active_users: 10,
  used_users: 4,
  requests: 1234,
  input_tokens: 4321,
  output_tokens: 2100,
  cache_creation_tokens: 300,
  cache_read_tokens: 400,
  total_tokens: 6789,
  actual_cost: 12.34
}

const range: OrganizationUsageRange = {
  start_date: '2026-08-01',
  end_date: '2026-08-10',
  as_of: '2026-08-10T09:30:00+08:00'
}

const champions: OrganizationUsageChampions = {
  day: {
    ...overview,
    user_id: 1,
    email: 'day@example.com',
    organization: 'xunyou.com',
    period_start: '2026-08-10',
    period_end: '2026-08-10',
    partial: false
  },
  week: {
    ...overview,
    user_id: 2,
    email: 'week@example.com',
    organization: 'wsdashi',
    period_start: '2026-08-04',
    period_end: '2026-08-10',
    partial: false
  },
  month: {
    ...overview,
    user_id: 3,
    email: 'month@example.com',
    organization: 'other',
    period_start: '2026-08-01',
    period_end: '2026-08-10',
    partial: true
  }
}

function mountOverview(overviewData = overview, rangeData = range) {
  return mount(OrganizationUsageOverview, {
    props: { overview: overviewData, range: rangeData, champions }
  })
}

describe('OrganizationUsageOverview', () => {
  it('renders the six operational KPIs without input tokens', () => {
    const wrapper = mountOverview()
    const text = wrapper.text()

    expect(wrapper.findAll('dl > div')).toHaveLength(6)
    expect(text).toContain('admin.organizationUsage.metrics.activeUsers')
    expect(text).toContain('admin.organizationUsage.metrics.usedUsers')
    expect(text).toContain('admin.organizationUsage.metrics.activeRate')
    expect(text).toContain('admin.organizationUsage.metrics.requests')
    expect(text).toContain('admin.organizationUsage.metrics.totalTokens')
    expect(text).toContain('admin.organizationUsage.metrics.actualCost')
    expect(text).not.toContain('admin.organizationUsage.metrics.inputTokens')
    expect(text).toContain('10')
    expect(text).toContain('4')
    expect(text).toContain('40.0%')
    expect(text).toContain('1,234')
    expect(text).toContain('6,789')
    expect(text).toContain('$12.3400')
    expect(text).not.toContain('4,321')
  })

  it('renders a zero active rate when there are no registered users', () => {
    const wrapper = mountOverview({ ...overview, active_users: 0, used_users: 0 })

    expect(wrapper.text()).toContain('0.0%')
  })

  it('renders a zero active rate when registered users is negative', () => {
    const wrapper = mountOverview({ ...overview, active_users: -1 })

    expect(wrapper.findAll('dd').map((metric) => metric.text())).toContain('0.0%')
  })

  it('uses the date formatter fallback instead of displaying an invalid report cutoff', () => {
    const wrapper = mountOverview(overview, { ...range, as_of: 'not-a-timestamp' })

    expect(wrapper.text()).toContain('admin.organizationUsage.trend.asOf:-')
    expect(wrapper.text()).not.toContain('not-a-timestamp')
  })

  it('omits the cutoff line when as_of is missing', () => {
    const wrapper = mountOverview(overview, { ...range, as_of: undefined })

    expect(wrapper.text()).not.toContain('admin.organizationUsage.trend.asOf')
  })

  it('renders the report cutoff, metric definitions and formatted champion organizations', () => {
    const wrapper = mountOverview()

    expect(wrapper.text()).toContain(`admin.organizationUsage.trend.asOf:${formatDateTime(range.as_of)}`)
    expect(wrapper.findAllComponents(HelpTooltip).map((tooltip) => tooltip.props('content'))).toEqual([
      'admin.organizationUsage.metrics.registeredUsersHelp',
      'admin.organizationUsage.metrics.activeUsersHelp',
      'admin.organizationUsage.metrics.totalTokensHelp'
    ])
    expect(wrapper.text()).toContain('迅游')
    expect(wrapper.text()).toContain('速宝')
    expect(wrapper.text()).toContain('其他')
  })
})
