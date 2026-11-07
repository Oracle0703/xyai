import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import type { OrganizationUsageTrendPoint } from '@/api/admin/organizationUsage'
import OrganizationUsageTrendChart from '../OrganizationUsageTrendChart.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) =>
        params?.value ? `${key}:${params.value}` : key
    })
  }
})

vi.mock('vue-chartjs', () => ({
  Line: {
    name: 'Line',
    props: ['data', 'options'],
    template: '<div data-testid="line-chart" />'
  }
}))

const metrics = {
  requests: 3,
  input_tokens: 10,
  output_tokens: 20,
  cache_creation_tokens: 1,
  cache_read_tokens: 5,
  total_tokens: 36,
  actual_cost: 0.1
}

function point(overrides: Partial<OrganizationUsageTrendPoint> = {}): OrganizationUsageTrendPoint {
  return {
    period_start: '2026-07-01',
    period_end: '2026-07-01',
    partial: false,
    ...metrics,
    ...overrides
  }
}

function mountChart(props: Record<string, unknown> = {}) {
  return mount(OrganizationUsageTrendChart, {
    props: {
      points: [point()],
      granularity: 'day',
      loading: false,
      error: '',
      range: { start_date: '2026-07-01', end_date: '2026-07-31', as_of: '2026-07-10T04:00:00.000Z' },
      ...props
    }
  })
}

describe('OrganizationUsageTrendChart', () => {
  it('shows loading, error with retry, and emits granularity changes', async () => {
    const loading = mountChart({ loading: true, points: [] })
    expect(loading.find('[data-testid="trend-loading"]').exists()).toBe(true)

    const errored = mountChart({ error: 'boom', points: [] })
    expect(errored.find('[data-testid="trend-error"]').exists()).toBe(true)
    await errored.get('[data-testid="trend-retry"]').trigger('click')
    expect(errored.emitted('retry')).toBeTruthy()

    const ready = mountChart()
    await ready.get('[data-testid="trend-granularity-week"]').trigger('click')
    expect(ready.emitted('update:granularity')?.[0]).toEqual(['week'])
  })

  it('exposes total tokens and requests on the correct axes with monotone interpolation', () => {
    const wrapper = mountChart()
    const exposed = wrapper.vm as unknown as {
      chartData: {
        datasets: Array<{
          label: string
          data: number[]
          yAxisID: string
          cubicInterpolationMode: string
        }>
      }
      lineOptions: { scales: Record<string, { beginAtZero?: boolean; ticks?: { maxTicksLimit?: number } }> }
    }
    const datasets = exposed.chartData.datasets
    expect(datasets).toHaveLength(2)
    expect(datasets.map((dataset) => dataset.label)).toEqual([
      'admin.organizationUsage.metrics.totalTokens',
      'admin.organizationUsage.metrics.requests'
    ])
    expect(datasets.map((dataset) => dataset.data)).toEqual([[36], [3]])
    expect(datasets.some((dataset) => dataset.label === 'admin.organizationUsage.metrics.inputTokens')).toBe(false)
    expect(datasets.some((dataset) => dataset.label === 'admin.organizationUsage.metrics.outputTokens')).toBe(false)
    expect(datasets.some((dataset) => dataset.label === 'admin.organizationUsage.metrics.cacheTokens')).toBe(false)
    expect(datasets.map((dataset) => dataset.yAxisID)).toEqual(['y', 'yRequests'])
    expect(datasets.every((dataset) => dataset.cubicInterpolationMode === 'monotone')).toBe(true)
    expect(exposed.lineOptions.scales.y.beginAtZero).toBe(true)
    expect(exposed.lineOptions.scales.yRequests.beginAtZero).toBe(true)
    expect(exposed.lineOptions.scales.x.ticks?.maxTicksLimit).toBe(14)
    expect(wrapper.find('[data-testid="trend-chart"]').exists()).toBe(true)
  })

  it('adds one total-token line per organization between total and requests', () => {
    const split = (xunyou: number, wsdashi: number, other: number) => [
      { organization: 'xunyou', ...metrics, total_tokens: xunyou },
      { organization: 'wsdashi', ...metrics, total_tokens: wsdashi },
      { organization: 'other', ...metrics, total_tokens: other }
    ]
    const wrapper = mountChart({
      points: [
        point({ total_tokens: 30, organizations: split(20, 10, 0) }),
        point({ period_start: '2026-07-02', period_end: '2026-07-02', total_tokens: 5, organizations: split(0, 5, 0) })
      ]
    })
    const datasets = (wrapper.vm as unknown as {
      chartData: { datasets: Array<{ label: string; data: number[]; yAxisID: string; borderColor: string }> }
    }).chartData.datasets
    // "other" has no usage in range, so it is not drawn.
    expect(datasets.map((dataset) => dataset.label)).toEqual([
      'admin.organizationUsage.metrics.totalTokens',
      '迅游 admin.organizationUsage.metrics.totalTokens',
      '速宝 admin.organizationUsage.metrics.totalTokens',
      'admin.organizationUsage.metrics.requests'
    ])
    expect(datasets.map((dataset) => dataset.data)).toEqual([[30, 5], [20, 0], [10, 5], [3, 3]])
    expect(datasets.map((dataset) => dataset.yAxisID)).toEqual(['y', 'y', 'y', 'yRequests'])
    expect(new Set(datasets.map((dataset) => dataset.borderColor)).size).toBe(4)

    const withOther = mountChart({ points: [point({ organizations: split(20, 10, 6) })] })
    const otherDatasets = (withOther.vm as unknown as {
      chartData: { datasets: Array<{ label: string; borderDash?: number[] }> }
    }).chartData.datasets
    expect(otherDatasets).toHaveLength(5)
    expect(otherDatasets[3].label).toBe('admin.organizationUsage.organizations.other admin.organizationUsage.metrics.totalTokens')
    expect(otherDatasets[3].borderDash).toEqual([6, 4])
  })

  it('skips organization lines when only one organization is visible', () => {
    const wrapper = mountChart({
      points: [point({ organizations: [{ organization: 'xunyou', ...metrics }] })]
    })
    const exposed = wrapper.vm as unknown as {
      organizationSeries: unknown[]
      chartData: { datasets: unknown[] }
    }
    expect(exposed.organizationSeries).toEqual([])
    expect(exposed.chartData.datasets).toHaveLength(2)
  })

  it('uses zero point radius for dense series and still charts all-zero data', () => {
    const densePoints = Array.from({ length: 61 }, (_, index) =>
      point({
        period_start: `2026-01-${String((index % 28) + 1).padStart(2, '0')}`,
        period_end: `2026-01-${String((index % 28) + 1).padStart(2, '0')}`,
        requests: 0,
        input_tokens: 0,
        output_tokens: 0,
        cache_read_tokens: 0,
        total_tokens: 0
      })
    )
    const wrapper = mountChart({ points: densePoints })
    const exposed = wrapper.vm as unknown as {
      pointRadius: number
      densePoints: boolean
      chartData: { datasets: Array<{ pointRadius: number; data: number[] }> }
    }
    expect(exposed.densePoints).toBe(true)
    expect(exposed.pointRadius).toBe(0)
    expect(exposed.chartData.datasets.every((d) => d.pointRadius === 0)).toBe(true)
    expect(wrapper.find('[data-testid="trend-chart"]').exists()).toBe(true)
  })

  it('includes partial and final-bucket as_of text in tooltip callbacks', () => {
    const wrapper = mountChart({
      points: [
        point({ period_start: '2026-07-01', period_end: '2026-07-01', partial: true }),
        point({ period_start: '2026-07-02', period_end: '2026-07-02', partial: false })
      ]
    })
    const exposed = wrapper.vm as unknown as {
      lineOptions: {
        plugins: {
          tooltip: {
            callbacks: {
              afterTitle: (items: Array<{ dataIndex: number }>) => string | string[]
            }
          }
        }
      }
    }
    const first = exposed.lineOptions.plugins.tooltip.callbacks.afterTitle([{ dataIndex: 0 }])
    const last = exposed.lineOptions.plugins.tooltip.callbacks.afterTitle([{ dataIndex: 1 }])
    expect(String(first)).toContain('admin.organizationUsage.common.partial')
    expect(String(last)).toContain('admin.organizationUsage.trend.asOf')
  })
})
