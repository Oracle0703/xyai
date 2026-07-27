import { apiClient } from '../client'
import { MAX_CLIENT_EXPORT_ROWS, MAX_XLSX_DATA_ROWS } from '@/constants/organizationUsage'

export { MAX_CLIENT_EXPORT_ROWS, MAX_XLSX_DATA_ROWS } from '@/constants/organizationUsage'

export interface OrganizationUsageMetrics {
  requests: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  total_tokens: number
  actual_cost: number
}

export interface OrganizationUsageRange {
  start_date: string
  end_date: string
  as_of?: string
}

export interface OrganizationUsagePagination {
  total: number
  page: number
  page_size: number
  pages: number
}

export interface OrganizationUsagePeriod extends OrganizationUsageMetrics {
  period_start: string
  period_end: string
  partial: boolean
  user_id: number
  email: string
  organization: string
}

export interface OrganizationUsageSummaryItem extends OrganizationUsageMetrics {
  user_id: number
  email: string
  organization: string
  peak_day: OrganizationUsagePeriod | null
  peak_week: OrganizationUsagePeriod | null
  peak_month: OrganizationUsagePeriod | null
}

export interface OrganizationUsageOverview extends OrganizationUsageMetrics {
  active_users: number
  used_users: number
}

export interface OrganizationUsageOrganization extends OrganizationUsageMetrics {
  organization: string
  active_users: number
  used_users: number
}

export interface OrganizationUsageChampions {
  day: OrganizationUsagePeriod | null
  week: OrganizationUsagePeriod | null
  month: OrganizationUsagePeriod | null
}

export interface OrganizationUsageSummaryResponse {
  range: OrganizationUsageRange
  overview: OrganizationUsageOverview
  organizations: OrganizationUsageOrganization[]
  champions: OrganizationUsageChampions
  items: OrganizationUsageSummaryItem[]
  pagination: OrganizationUsagePagination
}

export interface OrganizationUsagePeriodsResponse {
  range: OrganizationUsageRange
  granularity: OrganizationUsageGranularity
  items: OrganizationUsagePeriod[]
  pagination: OrganizationUsagePagination
}

export interface OrganizationUsageTrendPoint extends OrganizationUsageMetrics {
  period_start: string
  period_end: string
  partial: boolean
}

export interface OrganizationUsageTrendResponse {
  range: OrganizationUsageRange
  data_through?: string
  granularity: OrganizationUsageGranularity
  points: OrganizationUsageTrendPoint[]
}

export interface OrganizationUsageTrendQuery {
  start_date: string
  end_date: string
  as_of?: string
  organization?: OrganizationUsageOrganizationFilter
  q?: string
  granularity: OrganizationUsageGranularity
}

export type OrganizationUsageOrganizationFilter = 'all' | 'xunyou' | 'wsdashi' | 'other'
export type OrganizationUsageGranularity = 'day' | 'week' | 'month'
export type OrganizationUsageSortOrder = 'asc' | 'desc'
export const ORGANIZATION_USAGE_SORT_FIELDS = [
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
] as const
export type OrganizationUsageSortBy = (typeof ORGANIZATION_USAGE_SORT_FIELDS)[number]
export interface OrganizationUsageQuery extends OrganizationUsageRange {
  organization?: OrganizationUsageOrganizationFilter
  q?: string
  page?: number
  page_size?: number
}

export interface OrganizationUsageSummaryQuery extends OrganizationUsageQuery {
  sort_by?: OrganizationUsageSortBy
  sort_order?: OrganizationUsageSortOrder
}

export interface OrganizationUsagePeriodsQuery extends OrganizationUsageQuery {
  granularity: OrganizationUsageGranularity
}

export interface OrganizationUsageRequestOptions {
  signal?: AbortSignal
}

export interface OrganizationUsageFetchProgress {
  completed: number
  total: number
}

export interface OrganizationUsageFetchAllOptions extends OrganizationUsageRequestOptions {
  onProgress?: (progress: OrganizationUsageFetchProgress) => void
}

export interface OrganizationUsageReportData {
  summary: OrganizationUsageSummaryResponse
  periods: Record<OrganizationUsageGranularity, OrganizationUsagePeriod[]>
}

export async function getOrganizationUsageSummary(
  params: OrganizationUsageSummaryQuery,
  options?: OrganizationUsageRequestOptions
): Promise<OrganizationUsageSummaryResponse> {
  const { data } = await apiClient.get<OrganizationUsageSummaryResponse>(
    '/admin/usage/organization-report/summary',
    { params, signal: options?.signal }
  )
  return data
}

export async function getOrganizationUsagePeriods(
  params: OrganizationUsagePeriodsQuery,
  options?: OrganizationUsageRequestOptions
): Promise<OrganizationUsagePeriodsResponse> {
  const { data } = await apiClient.get<OrganizationUsagePeriodsResponse>(
    '/admin/usage/organization-report/periods',
    { params, signal: options?.signal }
  )
  return data
}

export async function getOrganizationUsageTrend(
  params: OrganizationUsageTrendQuery,
  options?: OrganizationUsageRequestOptions
): Promise<OrganizationUsageTrendResponse> {
  const { data } = await apiClient.get<OrganizationUsageTrendResponse>(
    '/admin/usage/organization-report/trend',
    { params, signal: options?.signal }
  )
  return data
}

const EXPORT_PAGE_SIZE = 500
const EXPORT_INITIAL_REQUESTS = 4
const EXPORT_MAX_PAGES = Math.ceil(MAX_CLIENT_EXPORT_ROWS / EXPORT_PAGE_SIZE) + 1

function getAbortError(signal: AbortSignal): Error | DOMException {
  const reason = signal.reason
  if (reason instanceof Error) return reason
  if (typeof DOMException !== 'undefined' && reason instanceof DOMException) return reason
  return new DOMException('The operation was aborted', 'AbortError')
}

function throwIfAborted(signal?: AbortSignal): void {
  if (signal?.aborted) throw getAbortError(signal)
}

function estimatedPages(pagination: OrganizationUsagePagination): number {
  const reportedPages = Number.isSafeInteger(pagination.pages) && pagination.pages > 0
    ? pagination.pages
    : 1
  const pagesByTotal = Number.isSafeInteger(pagination.total) && pagination.total >= 0
    ? Math.max(1, Math.ceil(pagination.total / EXPORT_PAGE_SIZE))
    : 1
  return Math.max(reportedPages, pagesByTotal)
}

function completePagination(total: number): OrganizationUsagePagination {
  return {
    total,
    page: 1,
    page_size: EXPORT_PAGE_SIZE,
    pages: Math.max(1, Math.ceil(total / EXPORT_PAGE_SIZE))
  }
}

export async function fetchAllOrganizationUsageData(
  query: OrganizationUsageSummaryQuery,
  options: OrganizationUsageFetchAllOptions = {}
): Promise<OrganizationUsageReportData> {
  const { signal, onProgress } = options
  let snapshotAsOf = query.as_of ?? new Date().toISOString()
  const { page: _page, page_size: _pageSize, ...queryWithoutPagination } = {
    ...query,
    as_of: snapshotAsOf
  }
  let completed = 0
  let totalRequests = EXPORT_INITIAL_REQUESTS
  let collectedRows = 0

  const reportProgress = () => onProgress?.({ completed, total: totalRequests })
  const requestPages = async <T extends { pagination: OrganizationUsagePagination; items: unknown[] }>(
    request: (page: number) => Promise<T>,
    onFirst?: (response: T) => void
  ): Promise<{ first: T; items: T['items'] }> => {
    let page = 1
    let expectedPages = 1
    const items: T['items'] = [] as unknown as T['items']
    let first: T | undefined

    while (true) {
      throwIfAborted(signal)
      let response: T
      try {
        response = await request(page)
      } catch (error) {
        if (signal?.aborted) throw getAbortError(signal)
        throw error
      }
      throwIfAborted(signal)
      if (!first && collectedRows + response.pagination.total > MAX_CLIENT_EXPORT_ROWS) {
        throw new Error(`Organization usage export exceeds the client export row limit of ${MAX_CLIENT_EXPORT_ROWS}`)
      }
      if (collectedRows + response.items.length > MAX_CLIENT_EXPORT_ROWS) {
        throw new Error(`Organization usage export exceeds the client export row limit of ${MAX_CLIENT_EXPORT_ROWS}`)
      }
      if (response.pagination.total > MAX_XLSX_DATA_ROWS) {
        throw new Error(`Organization usage export exceeds the Excel row limit of ${MAX_XLSX_DATA_ROWS}`)
      }
      if (items.length + response.items.length > MAX_XLSX_DATA_ROWS) {
        throw new Error(`Organization usage export exceeds the Excel row limit of ${MAX_XLSX_DATA_ROWS}`)
      }
      if (!first) {
        first = response
        onFirst?.(response)
        expectedPages = estimatedPages(response.pagination)
        totalRequests += expectedPages - 1
      }
      items.push(...response.items)
      collectedRows += response.items.length
      completed += 1

      const isShortPage = response.items.length < EXPORT_PAGE_SIZE
      if (isShortPage && expectedPages > page) {
        totalRequests -= expectedPages - page
        expectedPages = page
      } else if (!isShortPage && page >= expectedPages) {
        expectedPages = page + 1
        totalRequests += 1
      }
      reportProgress()

      if (isShortPage) break
      if (page >= EXPORT_MAX_PAGES) {
        throw new Error(`Organization usage export exceeded ${EXPORT_MAX_PAGES} pages`)
      }
      page += 1
    }

    if (!first) throw new Error('Organization usage pagination returned no response')
    return { first, items }
  }

  const summaryPages = await requestPages(
    (page) =>
      getOrganizationUsageSummary(
        { ...queryWithoutPagination, as_of: snapshotAsOf, page, page_size: EXPORT_PAGE_SIZE },
        { signal }
      ),
    (response) => {
      if (response.range.as_of) snapshotAsOf = response.range.as_of
    }
  )

  const periods = {} as Record<OrganizationUsageGranularity, OrganizationUsagePeriod[]>
  const { sort_by: _sortBy, sort_order: _sortOrder, ...periodQuery } = queryWithoutPagination
  for (const granularity of ['day', 'week', 'month'] as const) {
    const periodPages = await requestPages((page) =>
      getOrganizationUsagePeriods(
        { ...periodQuery, as_of: snapshotAsOf, granularity, page, page_size: EXPORT_PAGE_SIZE },
        { signal }
      )
    )
    periods[granularity] = periodPages.items as OrganizationUsagePeriod[]
  }

  return {
    summary: {
      ...summaryPages.first,
      items: summaryPages.items as OrganizationUsageSummaryItem[],
      pagination: completePagination(summaryPages.items.length)
    },
    periods
  }
}

export const organizationUsageAPI = {
  getSummary: getOrganizationUsageSummary,
  getPeriods: getOrganizationUsagePeriods,
  getTrend: getOrganizationUsageTrend,
  fetchAll: fetchAllOrganizationUsageData
}

export default organizationUsageAPI
