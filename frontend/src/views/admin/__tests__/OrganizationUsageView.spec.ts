import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import Pagination from '@/components/common/Pagination.vue'
import UsageExportProgress from '@/components/admin/usage/UsageExportProgress.vue'
import OrganizationUsageOverview from '@/components/admin/organization-usage/OrganizationUsageOverview.vue'
import OrganizationUsageSummary from '@/components/admin/organization-usage/OrganizationUsageSummary.vue'
import type { OrganizationUsageSummaryResponse } from '@/api/admin/organizationUsage'
import OrganizationUsageView from '../OrganizationUsageView.vue'

const { fetchAll, generateWorkbook, getSummary, saveAs, showError, showInfo, showSuccess, write } = vi.hoisted(() => ({
  fetchAll: vi.fn(),
  generateWorkbook: vi.fn(() => Promise.resolve(new ArrayBuffer(8))),
  getSummary: vi.fn(),
  saveAs: vi.fn(),
  showError: vi.fn(),
  showInfo: vi.fn(),
  showSuccess: vi.fn(),
  write: vi.fn(() => new ArrayBuffer(8))
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    organizationUsage: { fetchAll, getSummary }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showInfo, showSuccess })
}))

vi.mock('file-saver', () => ({ saveAs }))

vi.mock('@/utils/organizationUsageExportWorker', () => ({
  generateOrganizationUsageWorkbook: generateWorkbook
}))

vi.mock('xlsx', async () => {
  const actual = await vi.importActual<typeof import('xlsx')>('xlsx')
  return { ...actual, write }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const metrics = {
  requests: 12,
  input_tokens: 100,
  output_tokens: 40,
  cache_creation_tokens: 20,
  cache_read_tokens: 10,
  total_tokens: 170,
  actual_cost: 1.25
}

const peak = {
  ...metrics,
  period_start: '2026-07-06',
  period_end: '2026-07-12',
  partial: true,
  user_id: 1,
  email: 'alice@xunyou.com',
  organization: 'xunyou'
}

function summary(items = [{
  ...metrics,
  user_id: 1,
  email: 'alice@xunyou.com',
  organization: 'xunyou',
  peak_day: peak,
  peak_week: peak,
  peak_month: peak
}]): OrganizationUsageSummaryResponse {
  return {
    range: { start_date: '2026-07-01', end_date: '2026-07-31' },
    overview: { ...metrics, active_users: 4, used_users: 3 },
    organizations: [
      { ...metrics, organization: 'xunyou', active_users: 2, used_users: 2 },
      { ...metrics, total_tokens: 85, organization: 'wsdashi', active_users: 1, used_users: 1 },
      { ...metrics, total_tokens: 0, organization: 'other', active_users: 1, used_users: 0 }
    ],
    champions: { day: peak, week: peak, month: null },
    items,
    pagination: { total: items.length, page: 1, page_size: 20, pages: items.length ? 1 : 0 }
  }
}

const emptySummary = (): OrganizationUsageSummaryResponse => ({
  range: { start_date: '2026-07-01', end_date: '2026-07-31' },
  overview: {
    active_users: 0,
    used_users: 0,
    requests: 0,
    input_tokens: 0,
    output_tokens: 0,
    cache_creation_tokens: 0,
    cache_read_tokens: 0,
    total_tokens: 0,
    actual_cost: 0
  },
  organizations: [],
  champions: { day: null, week: null, month: null },
  items: [],
  pagination: { total: 0, page: 1, page_size: 20, pages: 0 }
})

const reportData = () => ({
  summary: summary(),
  periods: { day: [peak], week: [peak], month: [peak] }
})

function mountView() {
  return mount(OrganizationUsageView, {
    global: {
      stubs: { AppLayout: { template: '<div><slot /></div>' } }
    }
  })
}

describe('OrganizationUsageView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-10T04:00:00.000Z'))
    getSummary.mockReset().mockResolvedValue(summary())
    fetchAll.mockReset().mockResolvedValue(reportData())
    generateWorkbook.mockReset().mockResolvedValue(new ArrayBuffer(8))
    saveAs.mockReset()
    showError.mockReset()
    showInfo.mockReset()
    showSuccess.mockReset()
    write.mockClear()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('loads the current Beijing natural month on first mount', async () => {
    mountView()
    await flushPromises()

    expect(getSummary).toHaveBeenCalledWith({
      start_date: '2026-07-01',
      end_date: '2026-07-31',
      organization: 'all',
      page: 1,
      page_size: 20,
      sort_by: 'total_tokens',
      sort_order: 'desc'
    }, { signal: expect.any(AbortSignal) })
  })

  it('switches all three date modes and blocks an invalid custom range', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="mode-week"]').trigger('click')
    expect(wrapper.get('[data-testid="week-range"]').text()).toContain('2026-07-06')
    expect(wrapper.get('[data-testid="week-range"]').text()).toContain('2026-07-12')

    await wrapper.get('[data-testid="mode-custom"]').trigger('click')
    await wrapper.get('[data-testid="custom-start"]').setValue('2025-01-01')
    await wrapper.get('[data-testid="custom-end"]').setValue('2026-12-31')
    await wrapper.get('[data-testid="apply-filters"]').trigger('click')

    expect(wrapper.get('[data-testid="date-error"]').text()).toContain('admin.organizationUsage.validation.rangeTooLong')
    expect(getSummary).toHaveBeenCalledTimes(1)

    await wrapper.get('[data-testid="mode-month"]').trigger('click')
    expect(wrapper.find('[data-testid="month-input"]').exists()).toBe(true)
  })

  it('queries the exact selected month and Monday-to-Sunday week ranges', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="month-input"]').setValue('2026-06')
    await wrapper.get('[data-testid="apply-filters"]').trigger('click')
    await flushPromises()
    expect(getSummary).toHaveBeenLastCalledWith(expect.objectContaining({
      start_date: '2026-06-01',
      end_date: '2026-06-30'
    }), expect.anything())

    await wrapper.get('[data-testid="mode-week"]').trigger('click')
    await wrapper.get('input[type="date"]').setValue('2026-07-08')
    await wrapper.get('[data-testid="apply-filters"]').trigger('click')
    await flushPromises()
    expect(getSummary).toHaveBeenLastCalledWith(expect.objectContaining({
      start_date: '2026-07-06',
      end_date: '2026-07-12'
    }), expect.anything())
  })

  it('applies a trimmed email query and reset restores the default query', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('input[type="search"]').setValue('  alice@xunyou.com  ')
    await wrapper.get('[data-testid="apply-filters"]').trigger('click')
    await flushPromises()
    expect(getSummary).toHaveBeenLastCalledWith(expect.objectContaining({
      q: 'alice@xunyou.com',
      page: 1
    }), expect.anything())

    const resetButton = wrapper.findAll('button').find((button) =>
      button.text().includes('admin.organizationUsage.actions.reset')
    )
    expect(resetButton).toBeDefined()
    await resetButton!.trigger('click')
    await flushPromises()

    const resetQuery = getSummary.mock.calls.at(-1)![0]
    expect(resetQuery).toMatchObject({
      start_date: '2026-07-01',
      end_date: '2026-07-31',
      organization: 'all',
      page: 1,
      page_size: 20,
      sort_by: 'total_tokens',
      sort_order: 'desc'
    })
    expect(resetQuery).not.toHaveProperty('q')
  })

  it('applies an organization row as a new page-one server query', async () => {
    const wrapper = mountView()
    await flushPromises()

    const organizationButton = wrapper.get('[data-organization="xunyou"]')
    expect(organizationButton.element.tagName).toBe('BUTTON')
    const organizationRow = organizationButton.element.closest('tr')!
    expect(organizationRow.getAttribute('role')).toBeNull()
    expect(organizationRow.getAttribute('tabindex')).toBeNull()
    expect(organizationRow.getAttribute('aria-pressed')).toBeNull()

    await organizationButton.trigger('click')
    await flushPromises()

    expect(getSummary).toHaveBeenLastCalledWith(expect.objectContaining({
      organization: 'xunyou',
      page: 1
    }), { signal: expect.any(AbortSignal) })
    expect(wrapper.get('[data-organization="xunyou"]').attributes('aria-pressed')).toBe('true')
  })

  it('performs sorting, paging and page-size changes on the server', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-sort-key="requests"]').trigger('click')
    await flushPromises()
    expect(getSummary).toHaveBeenLastCalledWith(expect.objectContaining({
      sort_by: 'requests',
      sort_order: 'asc',
      page: 1
    }), { signal: expect.any(AbortSignal) })

    wrapper.getComponent(Pagination).vm.$emit('update:page', 2)
    await flushPromises()
    expect(getSummary).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 }), expect.anything())

    wrapper.getComponent(Pagination).vm.$emit('update:pageSize', 50)
    await flushPromises()
    expect(getSummary).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, page_size: 50 }), expect.anything())
  })

  it('exposes server sorting through aria-sort on the sortable table headers', async () => {
    const wrapper = mountView()
    await flushPromises()

    const totalHeader = wrapper.get('[data-sort-key="total_tokens"]').element.closest('th')!
    const requestHeader = wrapper.get('[data-sort-key="requests"]').element.closest('th')!
    expect(totalHeader.getAttribute('aria-sort')).toBe('descending')
    expect(requestHeader.getAttribute('aria-sort')).toBe('none')

    await wrapper.get('[data-sort-key="requests"]').trigger('click')
    await flushPromises()
    expect(requestHeader.getAttribute('aria-sort')).toBe('ascending')
    expect(totalHeader.getAttribute('aria-sort')).toBe('none')
  })

  it('aborts the previous load and ignores its late response', async () => {
    let resolveStale!: (value: OrganizationUsageSummaryResponse) => void
    const staleRequest = new Promise<OrganizationUsageSummaryResponse>((resolve) => { resolveStale = resolve })
    const staleItem = { ...summary().items[0], email: 'stale@example.net' }
    const freshItem = { ...summary().items[0], email: 'fresh@example.net' }
    getSummary.mockReset()
      .mockReturnValueOnce(staleRequest)
      .mockResolvedValueOnce(summary([freshItem]))

    const wrapper = mountView()
    await wrapper.vm.$nextTick()
    const staleSignal = getSummary.mock.calls[0][1].signal as AbortSignal

    await wrapper.get('[data-sort-key="requests"]').trigger('click')
    await flushPromises()
    expect(staleSignal.aborted).toBe(true)
    expect(wrapper.find('[title="fresh@example.net"]').exists()).toBe(true)

    resolveStale(summary([staleItem]))
    await flushPromises()
    expect(wrapper.find('[title="fresh@example.net"]').exists()).toBe(true)
    expect(wrapper.find('[title="stale@example.net"]').exists()).toBe(false)
  })

  it('hides stale overview and organization totals while a filtered report is loading', async () => {
    let resolveFiltered!: (value: OrganizationUsageSummaryResponse) => void
    const filteredRequest = new Promise<OrganizationUsageSummaryResponse>((resolve) => { resolveFiltered = resolve })
    getSummary.mockReset()
      .mockResolvedValueOnce(summary())
      .mockReturnValueOnce(filteredRequest)

    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.findComponent(OrganizationUsageOverview).exists()).toBe(true)
    expect(wrapper.findComponent(OrganizationUsageSummary).exists()).toBe(true)

    await wrapper.get('[data-sort-key="requests"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-testid="people-loading"]').exists()).toBe(true)
    expect(wrapper.findComponent(OrganizationUsageOverview).exists()).toBe(false)
    expect(wrapper.findComponent(OrganizationUsageSummary).exists()).toBe(false)

    resolveFiltered(summary())
    await flushPromises()
    expect(wrapper.findComponent(OrganizationUsageOverview).exists()).toBe(true)
    expect(wrapper.findComponent(OrganizationUsageSummary).exists()).toBe(true)
  })

  it('aborts an in-flight load when unmounted', async () => {
    getSummary.mockReturnValueOnce(new Promise(() => {}))
    const wrapper = mountView()
    await wrapper.vm.$nextTick()
    const signal = getSummary.mock.calls[0][1].signal as AbortSignal

    wrapper.unmount()

    expect(signal.aborted).toBe(true)
  })

  it('renders loading, retryable error and empty states', async () => {
    let rejectRequest!: (error: Error) => void
    getSummary.mockReturnValueOnce(new Promise((_resolve, reject) => { rejectRequest = reject }))
    const wrapper = mountView()

    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-testid="people-loading"]').exists()).toBe(true)
    rejectRequest(new Error('network'))
    await flushPromises()
    expect(wrapper.find('[data-testid="load-error"]').exists()).toBe(true)

    getSummary.mockResolvedValueOnce(emptySummary())
    await wrapper.get('[data-testid="retry-load"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="people-empty"]').exists()).toBe(true)
  })

  it('exports the current range through the six-sheet workbook path', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="export-report"]').trigger('click')
    await flushPromises()

    expect(fetchAll).toHaveBeenCalledWith(expect.objectContaining({
      start_date: '2026-07-01',
      end_date: '2026-07-31',
      organization: 'all',
      sort_by: 'total_tokens',
      sort_order: 'desc'
    }), expect.objectContaining({ signal: expect.any(AbortSignal), onProgress: expect.any(Function) }))
    expect(generateWorkbook).toHaveBeenCalledWith(reportData(), expect.objectContaining({
      signal: expect.any(AbortSignal),
      onStage: expect.any(Function)
    }))
    expect(write).not.toHaveBeenCalled()
    expect(saveAs).toHaveBeenCalledOnce()
    expect(showSuccess).toHaveBeenCalledWith('admin.organizationUsage.feedback.exportSuccess')
  })

  it('keeps the export filename bound to the query snapshot taken at click time', async () => {
    let resolveExport!: (data: ReturnType<typeof reportData>) => void
    fetchAll.mockReturnValueOnce(new Promise((resolve) => { resolveExport = resolve }))
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="export-report"]').trigger('click')
    await wrapper.get('[data-testid="mode-custom"]').trigger('click')
    await wrapper.get('[data-testid="custom-start"]').setValue('2026-08-01')
    await wrapper.get('[data-testid="custom-end"]').setValue('2026-08-31')
    await wrapper.get('[data-testid="apply-filters"]').trigger('click')
    await flushPromises()

    resolveExport(reportData())
    await flushPromises()

    expect(saveAs).toHaveBeenCalledWith(
      expect.any(Blob),
      'organization_usage_2026-07-01_to_2026-07-31.xlsx'
    )
  })

  it('allows export cancellation and localizes capacity errors', async () => {
    fetchAll.mockImplementationOnce((_query, options: { signal: AbortSignal }) => new Promise((_resolve, reject) => {
      options.signal.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), { name: 'AbortError' })))
    }))
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="export-report"]').trigger('click')
    await flushPromises()
    wrapper.getComponent(UsageExportProgress).vm.$emit('cancel')
    await flushPromises()
    expect(showInfo).toHaveBeenCalledWith('admin.organizationUsage.feedback.exportCanceled')

    fetchAll.mockRejectedValueOnce(new Error('Organization usage export exceeds the client export row limit of 100000'))
    await wrapper.get('[data-testid="export-report"]').trigger('click')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('admin.organizationUsage.feedback.exportTooLarge')
  })

  it('reserves one progress step and remains cancelable while the worker is generating', async () => {
    fetchAll.mockImplementationOnce(async (_query, options: { onProgress: (value: { completed: number; total: number }) => void }) => {
      options.onProgress({ completed: 4, total: 4 })
      return reportData()
    })
    generateWorkbook.mockImplementationOnce((_data, options: { signal: AbortSignal; onStage: (stage: string) => void }) => {
      options.onStage('building')
      return new Promise((_resolve, reject) => {
        options.signal.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), { name: 'AbortError' })))
      })
    })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="export-report"]').trigger('click')
    await flushPromises()
    const progress = wrapper.getComponent(UsageExportProgress)
    expect(progress.props()).toMatchObject({
      show: true,
      current: 4,
      total: 5,
      progress: 80,
      estimatedTime: 'admin.organizationUsage.feedback.generatingWorkbook'
    })

    progress.vm.$emit('cancel')
    await flushPromises()
    expect(showInfo).toHaveBeenCalledWith('admin.organizationUsage.feedback.exportCanceled')
  })

  it('silently aborts an export when the view unmounts', async () => {
    fetchAll.mockImplementationOnce((_query, options: { signal: AbortSignal }) => new Promise((_resolve, reject) => {
      options.signal.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), { name: 'AbortError' })))
    }))
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="export-report"]').trigger('click')
    await flushPromises()

    wrapper.unmount()
    await flushPromises()

    expect(showInfo).not.toHaveBeenCalled()
  })

  it.each(['fetch', 'worker'] as const)(
    'keeps a user cancellation silent after the view unmounts while %s settles',
    async (stage) => {
      let rejectExport!: (error: Error) => void
      const pendingExport = new Promise<never>((_resolve, reject) => { rejectExport = reject })
      if (stage === 'fetch') {
        fetchAll.mockReturnValueOnce(pendingExport)
      } else {
        generateWorkbook.mockReturnValueOnce(pendingExport)
      }
      const wrapper = mountView()
      await flushPromises()
      await wrapper.get('[data-testid="export-report"]').trigger('click')
      await flushPromises()

      wrapper.getComponent(UsageExportProgress).vm.$emit('cancel')
      wrapper.unmount()
      rejectExport(Object.assign(new Error('aborted'), { name: 'AbortError' }))
      await flushPromises()

      expect(showInfo).not.toHaveBeenCalled()
    }
  )
})
