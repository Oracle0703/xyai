import { afterEach, beforeEach, describe, expect, expectTypeOf, it, vi } from 'vitest'

import type {
  OrganizationUsageSortBy,
  OrganizationUsageSummaryQuery
} from '@/api/admin/organizationUsage'

const { get } = vi.hoisted(() => ({
  get: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get }
}))

import {
  MAX_CLIENT_EXPORT_ROWS,
  MAX_XLSX_DATA_ROWS,
  ORGANIZATION_USAGE_SORT_FIELDS,
  fetchAllOrganizationUsageData,
  getOrganizationUsagePeriods,
  getOrganizationUsageSummary,
  getOrganizationUsageTrend
} from '@/api/admin/organizationUsage'

const metrics = {
  requests: 1,
  input_tokens: 2,
  output_tokens: 3,
  cache_creation_tokens: 4,
  cache_read_tokens: 5,
  total_tokens: 14,
  actual_cost: 0.25
}

const EXPECTED_MAX_XLSX_DATA_ROWS = 1_048_575
const EXPECTED_MAX_CLIENT_EXPORT_ROWS = 100_000

const range = { start_date: '2026-07-01', end_date: '2026-07-10' }

const pagination = (page: number, pages: number, total: number) => ({
  total,
  page,
  page_size: 500,
  pages
})

const summaryItem = (userId: number) => ({
  user_id: userId,
  email: `user${userId}@example.com`,
  organization: 'xunyou',
  ...metrics,
  peak_day: null,
  peak_week: null,
  peak_month: null
})

const periodItem = (userId: number, periodStart: string) => ({
  period_start: periodStart,
  period_end: periodStart,
  partial: false,
  user_id: userId,
  email: `user${userId}@example.com`,
  organization: 'xunyou',
  ...metrics
})

const summaryResponse = (page: number, pages: number, total: number, items: ReturnType<typeof summaryItem>[]) => ({
  range,
  overview: { active_users: 2, used_users: 2, ...metrics },
  organizations: [{ organization: 'xunyou', active_users: 2, used_users: 2, ...metrics }],
  champions: { day: null, week: null, month: null },
  items,
  pagination: pagination(page, pages, total)
})

const periodsResponse = (
  granularity: 'day' | 'week' | 'month',
  page: number,
  pages: number,
  total: number,
  items: ReturnType<typeof periodItem>[]
) => ({
  range,
  granularity,
  items,
  pagination: pagination(page, pages, total)
})

describe('organization usage API', () => {
  beforeEach(() => {
    get.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('restricts summary sort_by to the backend-supported field union', () => {
    expect(MAX_XLSX_DATA_ROWS).toBe(EXPECTED_MAX_XLSX_DATA_ROWS)
    expect(MAX_CLIENT_EXPORT_ROWS).toBe(EXPECTED_MAX_CLIENT_EXPORT_ROWS)
    expect(ORGANIZATION_USAGE_SORT_FIELDS).toEqual([
      'email',
      'requests',
      'input_tokens',
      'output_tokens',
      'cache_creation_tokens',
      'cache_read_tokens',
      'total_tokens',
      'actual_cost',
      'peak_day_tokens',
      'peak_week_tokens',
      'peak_month_tokens'
    ])
    expectTypeOf<OrganizationUsageSummaryQuery['sort_by']>().toEqualTypeOf<
      OrganizationUsageSortBy | undefined
    >()
  })

  it('requests summary with exact URL, params and AbortSignal', async () => {
    const response = summaryResponse(1, 1, 0, [])
    const controller = new AbortController()
    const params = {
      ...range,
      organization: 'xunyou' as const,
      q: 'alice',
      page: 2,
      page_size: 50,
      sort_by: 'total_tokens',
      sort_order: 'desc' as const
    }
    get.mockResolvedValueOnce({ data: response })

    await expect(getOrganizationUsageSummary(params, { signal: controller.signal })).resolves.toBe(response)
    expect(get).toHaveBeenCalledWith('/admin/usage/organization-report/summary', {
      params,
      signal: controller.signal
    })
  })

  it('requests periods with exact URL, params and AbortSignal', async () => {
    const response = periodsResponse('week', 1, 1, 0, [])
    const controller = new AbortController()
    const params = {
      ...range,
      organization: 'all' as const,
      granularity: 'week' as const,
      page: 1,
      page_size: 100
    }
    get.mockResolvedValueOnce({ data: response })

    await expect(getOrganizationUsagePeriods(params, { signal: controller.signal })).resolves.toBe(response)
    expect(get).toHaveBeenCalledWith('/admin/usage/organization-report/periods', {
      params,
      signal: controller.signal
    })
  })

  it('requests trend with exact URL, params and AbortSignal', async () => {
    const response = {
      range: { ...range, as_of: '2026-07-10T04:00:00.000Z' },
      data_through: '2026-07-10',
      granularity: 'day' as const,
      points: []
    }
    const controller = new AbortController()
    const params = {
      ...range,
      organization: 'xunyou' as const,
      granularity: 'day' as const,
      as_of: '2026-07-10T04:00:00.000Z'
    }
    get.mockResolvedValueOnce({ data: response })

    await expect(getOrganizationUsageTrend(params, { signal: controller.signal })).resolves.toBe(response)
    expect(get).toHaveBeenCalledWith('/admin/usage/organization-report/trend', {
      params,
      signal: controller.signal
    })
  })

  it('continues past under-reported pages when total requires another page', async () => {
    const firstPageItems = Array.from({ length: 500 }, (_, index) => summaryItem(index + 1))
    const firstSummary = summaryResponse(1, 1, 501, firstPageItems)
    const progress = vi.fn()

    get.mockImplementation(async (url: string, config: { params: Record<string, unknown> }) => {
      const page = Number(config.params.page)
      if (url.endsWith('/summary')) {
        return { data: page === 1 ? firstSummary : summaryResponse(2, 1, 501, [summaryItem(501)]) }
      }
      const granularity = config.params.granularity as 'day' | 'week' | 'month'
      return {
        data: periodsResponse(granularity, page, 1, 1, [periodItem(1, '2026-07-01')])
      }
    })

    const result = await fetchAllOrganizationUsageData(
      { ...range, organization: 'all', sort_by: 'total_tokens', sort_order: 'desc' },
      { onProgress: progress }
    )

    expect(result.summary.items).toHaveLength(501)
    expect(result.summary.items.at(-1)).toEqual(summaryItem(501))
    expect(result.summary.pagination).toEqual({ total: 501, page: 1, page_size: 500, pages: 2 })
    expect(result.periods.day).toHaveLength(1)
    expect(result.periods.week).toHaveLength(1)
    expect(result.periods.month).toHaveLength(1)
    expect(get).toHaveBeenCalledTimes(5)
    for (const call of get.mock.calls) {
      expect(call[1].params.page_size).toBe(500)
    }
    expect(progress).toHaveBeenLastCalledWith({ completed: 5, total: 5 })
  })

  it('stops immediately on a non-empty short page and corrects an over-reported total', async () => {
    const progress = vi.fn()
    get.mockImplementation(async (url: string, config: { params: Record<string, unknown> }) => {
      if (url.endsWith('/summary')) {
        return { data: summaryResponse(1, 999, 999, [summaryItem(1)]) }
      }
      const granularity = config.params.granularity as 'day' | 'week' | 'month'
      return { data: periodsResponse(granularity, 1, 999, 999, []) }
    })

    const result = await fetchAllOrganizationUsageData({ ...range }, { onProgress: progress })

    expect(result.summary.items).toEqual([summaryItem(1)])
    expect(result.summary.pagination).toEqual({ total: 1, page: 1, page_size: 500, pages: 1 })
    expect(get).toHaveBeenCalledTimes(4)
    expect(progress).toHaveBeenLastCalledWith({ completed: 4, total: 4 })
  })

  it('probes beyond under-reported total and pages while pages remain full', async () => {
    const fullPage = Array.from({ length: 500 }, (_, index) => summaryItem(index + 1))
    get.mockImplementation(async (url: string, config: { params: Record<string, unknown> }) => {
      const page = Number(config.params.page)
      if (url.endsWith('/summary')) {
        return {
          data: page === 1
            ? summaryResponse(1, 1, 1, fullPage)
            : summaryResponse(2, 1, 1, [summaryItem(501)])
        }
      }
      const granularity = config.params.granularity as 'day' | 'week' | 'month'
      return { data: periodsResponse(granularity, 1, 1, 0, []) }
    })

    const result = await fetchAllOrganizationUsageData({ ...range })

    expect(result.summary.items).toHaveLength(501)
    expect(result.summary.pagination).toEqual({ total: 501, page: 1, page_size: 500, pages: 2 })
    expect(get).toHaveBeenCalledTimes(5)
  })

  it('normalizes an in-flight request cancellation to AbortError', async () => {
    const controller = new AbortController()
    let requestStarted!: () => void
    const started = new Promise<void>((resolve) => {
      requestStarted = resolve
    })
    get.mockImplementationOnce((_url: string, config: { signal: AbortSignal }) => {
      requestStarted()
      return new Promise((_resolve, reject) => {
        config.signal.addEventListener('abort', () => {
          reject(Object.assign(new Error('canceled by transport'), { name: 'CanceledError' }))
        })
      })
    })

    const result = fetchAllOrganizationUsageData(range, { signal: controller.signal })
    await started
    controller.abort()

    await expect(result).rejects.toMatchObject({ name: 'AbortError' })
  })

  it('preserves ordinary request errors when the signal is not aborted', async () => {
    const networkError = new Error('network failed')
    get.mockRejectedValueOnce(networkError)

    await expect(fetchAllOrganizationUsageData(range)).rejects.toBe(networkError)
  })

  it('rejects server totals above the client export row limit before collecting rows', async () => {
    get.mockResolvedValueOnce({
      data: summaryResponse(1, 1, EXPECTED_MAX_CLIENT_EXPORT_ROWS + 1, [])
    })

    await expect(fetchAllOrganizationUsageData(range)).rejects.toThrow(/client export row limit/)
  })

  it('rejects actual page items above the client export row limit before appending them', async () => {
    const sparseItems = new Array(EXPECTED_MAX_CLIENT_EXPORT_ROWS + 1) as ReturnType<typeof summaryItem>[]
    get.mockResolvedValueOnce({
      data: summaryResponse(1, 1, 0, sparseItems)
    })

    await expect(fetchAllOrganizationUsageData(range)).rejects.toThrow(/client export row limit/)
  })

  it('rejects a period from its first server total when prior datasets consume the client budget', async () => {
    const fullSummaryPage = Array.from({ length: 500 }, (_, index) => summaryItem(index + 1))
    get.mockImplementation(async (url: string, config: { params: Record<string, unknown> }) => {
      const page = Number(config.params.page)
      if (url.endsWith('/summary')) {
        return {
          data: summaryResponse(page, 120, 60_000, page <= 120 ? fullSummaryPage : [])
        }
      }
      const granularity = config.params.granularity as 'day' | 'week' | 'month'
      return { data: periodsResponse(granularity, 1, 100, 50_000, []) }
    })

    await expect(fetchAllOrganizationUsageData(range)).rejects.toThrow(/client export row limit/)
    expect(get).toHaveBeenCalledTimes(122)
  })

  it('uses actual accumulated rows when metadata under-reports across datasets', async () => {
    const fullSummaryPage = Array.from({ length: 500 }, (_, index) => summaryItem(index + 1))
    const fullPeriodPage = Array.from({ length: 500 }, (_, index) => periodItem(index + 1, '2026-07-01'))
    get.mockImplementation(async (url: string, config: { params: Record<string, unknown> }) => {
      const page = Number(config.params.page)
      if (url.endsWith('/summary')) {
        return { data: summaryResponse(page, 1, 1, page <= 120 ? fullSummaryPage : []) }
      }
      const granularity = config.params.granularity as 'day' | 'week' | 'month'
      const items = granularity === 'day' && page <= 81 ? fullPeriodPage : []
      return { data: periodsResponse(granularity, page, 1, 1, items) }
    })

    await expect(fetchAllOrganizationUsageData(range)).rejects.toThrow(/client export row limit/)
    expect(get).toHaveBeenCalledTimes(202)
  })

  it('rejects a pre-aborted export before issuing any request', async () => {
    const controller = new AbortController()
    controller.abort()

    await expect(fetchAllOrganizationUsageData(range, { signal: controller.signal })).rejects.toMatchObject({
      name: 'AbortError'
    })
    expect(get).not.toHaveBeenCalled()
  })

  it('switches from the generated candidate to the signed summary snapshot before page 2', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2030-01-02T03:04:05.000Z'))
    const fullSummaryPage = Array.from({ length: 500 }, (_, index) => summaryItem(index + 1))
    const fullDayPage = Array.from({ length: 500 }, (_, index) => periodItem(index + 1, '2026-07-01'))
    const seenSnapshots: unknown[] = []
    const signedAsOf = '2026-07-10T08:00:00Z'

    get.mockImplementation(async (url: string, config: { params: Record<string, unknown> }) => {
      seenSnapshots.push(config.params.as_of)
      if (seenSnapshots.length === 1) {
        vi.setSystemTime(new Date('2026-07-12T12:34:56.000Z'))
      }
      const page = Number(config.params.page)
      if (url.endsWith('/summary')) {
        const response = summaryResponse(page, 2, 501, page === 1 ? fullSummaryPage : [summaryItem(501)])
        return { data: { ...response, range: { ...response.range, as_of: signedAsOf } } }
      }
      const granularity = config.params.granularity as 'day' | 'week' | 'month'
      const items = granularity === 'day' && page === 1 ? fullDayPage : []
      return { data: periodsResponse(granularity, page, 1, items.length, items) }
    })

    const result = await fetchAllOrganizationUsageData(range)

    expect(seenSnapshots).toHaveLength(6)
    expect(seenSnapshots[0]).toBe('2030-01-02T03:04:05.000Z')
    expect(seenSnapshots.slice(1)).toEqual(Array(5).fill(signedAsOf))
    expect(result.summary.range.as_of).toBe(signedAsOf)
  })

  it('switches an explicit candidate to the server-signed snapshot', async () => {
    const candidateAsOf = '2030-01-02T03:04:05.123456789+08:00'
    const signedAsOf = '2026-07-10T08:00:00Z'
    const seenSnapshots: unknown[] = []
    get.mockImplementation(async (url: string, config: { params: Record<string, unknown> }) => {
      seenSnapshots.push(config.params.as_of)
      if (url.endsWith('/summary')) {
        const response = summaryResponse(1, 1, 0, [])
        return { data: { ...response, range: { ...response.range, as_of: signedAsOf } } }
      }
      const granularity = config.params.granularity as 'day' | 'week' | 'month'
      return { data: periodsResponse(granularity, 1, 1, 0, []) }
    })

    await fetchAllOrganizationUsageData({ ...range, as_of: candidateAsOf })

    expect(seenSnapshots).toEqual([candidateAsOf, signedAsOf, signedAsOf, signedAsOf])
  })

  it('keeps the candidate as_of when the summary response does not echo one', async () => {
    const explicitAsOf = '2026-07-10T16:20:30.123456789+08:00'
    get.mockImplementation(async (url: string, config: { params: Record<string, unknown> }) => {
      if (url.endsWith('/summary')) {
        return { data: summaryResponse(1, 1, 0, []) }
      }
      const granularity = config.params.granularity as 'day' | 'week' | 'month'
      return { data: periodsResponse(granularity, 1, 1, 0, []) }
    })

    await fetchAllOrganizationUsageData({ ...range, as_of: explicitAsOf })

    expect(get).toHaveBeenCalledTimes(4)
    for (const call of get.mock.calls) {
      expect(call[1].params.as_of).toBe(explicitAsOf)
    }
  })
})
