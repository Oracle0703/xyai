import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OrganizationUsagePeopleTable from '../OrganizationUsagePeopleTable.vue'

const { captureTablePng, saveAs, showError, showSuccess } = vi.hoisted(() => ({
  captureTablePng: vi.fn(), saveAs: vi.fn(), showError: vi.fn(), showSuccess: vi.fn()
}))
vi.mock('@/utils/tableScreenshot', () => ({ captureTablePng }))
vi.mock('file-saver', () => ({ saveAs }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))

function mountTable(count = 20, page = 1, pageSize = count) {
  return mount(OrganizationUsagePeopleTable, {
    props: {
      items: Array.from({ length: count }, (_, index) => ({
        user_id: 100 + index, email: `person${100 + index}@example.com`, organization: 'xunyou',
        department_name: '研发部', requests: index, input_tokens: 2, output_tokens: 3,
        cache_creation_tokens: 4, cache_read_tokens: 5, total_tokens: 14, actual_cost: 0.25,
        peak_day: null, peak_week: null, peak_month: null
      })),
      pagination: { total: 207, page, page_size: pageSize, pages: Math.ceil(207 / pageSize) },
      range: { start_date: '2026-09-01', end_date: '2026-09-30' },
      loading: false, sortBy: 'total_tokens', sortOrder: 'desc'
    },
    global: { stubs: { Pagination: true, Icon: true } }
  })
}

describe('OrganizationUsagePeopleTable screenshots', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    captureTablePng.mockResolvedValue(new Blob(['png'], { type: 'image/png' }))
  })

  it.each([20, 50, 100])('downloads exactly the current %i rendered rows and all columns', async count => {
    const wrapper = mountTable(count, 2)
    await wrapper.get('[data-testid="people-screenshot"]').trigger('click')
    await flushPromises()
    const table = captureTablePng.mock.calls[0][0] as HTMLTableElement
    expect(table).toBe(wrapper.get('table').element)
    expect(table.tBodies[0].rows).toHaveLength(count)
    expect(table.tHead!.rows[0].cells).toHaveLength(12)
    expect(table.tBodies[0].rows[0].textContent).toContain('person100@example.com')
    expect(saveAs).toHaveBeenCalledWith(expect.any(Blob), 'organization_usage_people_2026-09-01_to_2026-09-30_page_2.png')
    expect(showSuccess).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('captures only the remaining rows on the last page', async () => {
    const wrapper = mountTable(7, 5, 50)
    await wrapper.get('[data-testid="people-screenshot"]').trigger('click')
    await flushPromises()
    expect(captureTablePng.mock.calls[0][0].tBodies[0].rows).toHaveLength(7)
    expect(saveAs.mock.calls[0][1]).toContain('page_5.png')
    wrapper.unmount()
  })

  it('blocks empty/loading captures and duplicate clicks', async () => {
    const wrapper = mountTable()
    await wrapper.setProps({ loading: true })
    await wrapper.get('[data-testid="people-screenshot"]').trigger('click')
    expect(captureTablePng).not.toHaveBeenCalled()
    await wrapper.setProps({ loading: false, items: [] })
    expect(wrapper.get('[data-testid="people-screenshot"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()

    captureTablePng.mockReturnValue(new Promise(() => {}))
    const pending = mountTable()
    await pending.get('[data-testid="people-screenshot"]').trigger('click')
    await pending.get('[data-testid="people-screenshot"]').trigger('click')
    expect(captureTablePng).toHaveBeenCalledOnce()
    pending.unmount()
  })

  it.each(['page', 'loading', 'unmount'])('discards an in-flight image after %s changes', async change => {
    let finish!: (blob: Blob) => void
    captureTablePng.mockReturnValue(new Promise<Blob>(resolve => { finish = resolve }))
    const wrapper = mountTable()
    await wrapper.get('[data-testid="people-screenshot"]').trigger('click')
    const signal = captureTablePng.mock.calls[0][1] as AbortSignal
    if (change === 'unmount') wrapper.unmount()
    else if (change === 'loading') await wrapper.setProps({ loading: true })
    else await wrapper.setProps({ pagination: { total: 207, page: 2, page_size: 20, pages: 11 } })
    expect(signal.aborted).toBe(true)
    finish(new Blob(['stale']))
    await flushPromises()
    expect(saveAs).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()
    if (change !== 'unmount') wrapper.unmount()
  })

  it('reports rendering failure and lets the user retry', async () => {
    captureTablePng.mockRejectedValueOnce(new Error('render failed'))
    const wrapper = mountTable()
    await wrapper.get('[data-testid="people-screenshot"]').trigger('click')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('admin.organizationUsage.people.screenshotFailed')
    expect(saveAs).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="people-screenshot"]').trigger('click')
    await flushPromises()
    expect(saveAs).toHaveBeenCalledOnce()
    wrapper.unmount()
  })
})
