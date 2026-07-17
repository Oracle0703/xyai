import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import TokenAnalysisView from '../TokenAnalysisView.vue'
import Pagination from '@/components/common/Pagination.vue'

const api = vi.hoisted(() => ({
  getSummary: vi.fn(),
  listUsers: vi.fn(),
  listProjects: vi.fn(),
  listRequests: vi.fn(),
  getRequestInput: vi.fn(),
  getIndexStatus: vi.fn(),
  listArchiveFiles: vi.fn(),
  triggerIndex: vi.fn()
}))

const dashboardApi = vi.hoisted(() => ({
  getUserUsageTrend: vi.fn()
}))

const authState = vi.hoisted(() => ({
  isAdmin: true,
  isSubAdmin: false
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    tokenAnalysis: api,
    dashboard: dashboardApi
  }
}))

vi.mock('vue-chartjs', () => ({
  Line: {
    name: 'Line',
    props: ['data', 'options'],
    template: '<div data-testid="selected-user-trend-chart" />'
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const AppLayoutStub = { template: '<div><slot /></div>' }

function summaryFixture(overrides: Record<string, unknown> = {}) {
  return {
    total_requests: 12,
    matched_requests: 10,
    unmatched_requests: 2,
    total_input_tokens: 5000,
    total_output_tokens: 3000,
    total_tokens: 9000,
    total_actual_cost: 1.23,
    cache_read_tokens: 4000,
    cache_creation_tokens: 1000,
    cache_hit_rate: 0.4,
    risky_requests: 2,
    risky_cost: 0.6,
    unmatched_rate: 0.1667,
    risk_request_rate: 0.1667,
    risk_reasons: [{ code: 'huge_input_tiny_output', count: 2 }],
    billed_requests: 48,
    archive_coverage: 0.421,
    ...overrides
  }
}

function userFixture(userId: number, email: string) {
  return {
    user_id: userId,
    user_email: email,
    total_tokens: userId * 100,
    actual_cost: userId
  }
}

describe('TokenAnalysisView', () => {
  beforeEach(() => {
    api.getSummary.mockReset()
    api.listUsers.mockReset()
    api.listProjects.mockReset()
    api.listRequests.mockReset()
    api.getRequestInput.mockReset()
    api.getIndexStatus.mockReset()
    api.listArchiveFiles.mockReset()
    api.triggerIndex.mockReset()
    dashboardApi.getUserUsageTrend.mockReset()
    authState.isAdmin = true
    authState.isSubAdmin = false
    api.listProjects.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
    api.listArchiveFiles.mockResolvedValue([
      {
        name: '2026-05-20.jsonl',
        size_bytes: 1024,
        mod_time: '2026-05-20T23:59:00Z',
        indexed_offset: 1024,
        processed_rows: 10,
        failed_rows: 0,
        last_error: '',
        status: 'deletable'
      }
    ])
    api.getRequestInput.mockResolvedValue({
      id: 1,
      archive_id: 'arch-1',
      event_time: '2026-05-19T01:00:00Z',
      content: 'full input line one\nline two',
      content_sha256: 'abc',
      chars: 28,
      truncated: false,
      quality_version: ''
    })
    api.getSummary.mockResolvedValue(summaryFixture())
    api.listUsers.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
    dashboardApi.getUserUsageTrend.mockResolvedValue({
      trend: [],
      start_date: '2026-07-01',
      end_date: '2026-07-01',
      granularity: 'day'
    })
    api.listRequests.mockResolvedValue({
      items: [
        {
          id: 1,
          archive_id: 'arch-1',
          event_time: '2026-05-19T01:00:00Z',
          user_email: 'user@example.com',
          api_key_name: 'dev',
          model: 'gpt-4.1',
          input_tokens: 220000,
          output_tokens: 32,
          cache_read_tokens: 0,
          cache_creation_tokens: 0,
          actual_cost: 1.23,
          risk_score: 45,
          risk_reasons: [{ code: 'huge_input_tiny_output', message: 'large input tiny output', score: 45 }],
          last_user_preview: 'hello',
          match_confidence: 3,
          client_project: 'lag-killer',
          client_branch: 'main',
          has_input: true,
          input_truncated: false,
          request_body_truncated: true,
          duplicate_count: 3
        }
      ],
      total: 1,
      page: 1,
      page_size: 20
    })
    api.getIndexStatus.mockResolvedValue({
      running: false,
      processed_rows: 10,
      failed_rows: 0,
      files: [{
        source_file: '2026-05-21.jsonl',
        last_offset: 1234,
        last_archive_id: 'arch-1',
        processed_rows: 10,
        failed_rows: 0,
        last_error: '',
        updated_at: '2026-05-21T01:00:00Z'
      }]
    })
  })

  it('renders summary and request detail rows', async () => {
    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('9,000')
    expect(wrapper.text()).toContain('user@example.com')
    expect(wrapper.text()).toContain('gpt-4.1')
    expect(wrapper.text()).toContain('hello')
    expect(wrapper.text()).toContain('lag-killer')
    expect(wrapper.text()).toContain('main')
    expect(wrapper.text()).toContain('16.7%')
    expect(wrapper.text()).toContain('huge_input_tiny_output')
    expect(wrapper.text()).toContain('2026-05-21.jsonl')
    // 归档文件卡片: 已入库文件带可删除标签。
    expect(wrapper.text()).toContain('2026-05-20.jsonl')
    expect(wrapper.text()).toContain('admin.tokenAnalysis.archiveFileStatus.deletable')
    // 新增展示: 重复计数徽标 ×N、归档截断徽标、概览 input/output 拆分卡。
    expect(wrapper.text()).toContain('×3')
    expect(wrapper.text()).toContain('admin.tokenAnalysis.bodyTruncated')
    expect(wrapper.text()).toContain('admin.tokenAnalysis.summary.totalRequests')
    expect(wrapper.text()).toContain('admin.tokenAnalysis.summary.inputTokens')
    expect(wrapper.text()).toContain('5,000')
    // 口径卡片: 同期计费请求数 + 归档覆盖率(matched/billed)。
    expect(wrapper.text()).toContain('admin.tokenAnalysis.summary.billedRequests')
    expect(wrapper.text()).toContain('48')
    expect(wrapper.text()).toContain('admin.tokenAnalysis.summary.archiveCoverage')
    expect(wrapper.text()).toContain('42.1%')
    // 归档文件: 水位追平的文件进度显示"已读完", 并标注请求行合计。
    expect(wrapper.text()).toContain('100% · admin.tokenAnalysis.fullyRead')
    expect(wrapper.text()).toContain('admin.tokenAnalysis.requestRowsTotal')
  })

  it('hides the manual index action for sub admins', async () => {
    authState.isAdmin = false
    authState.isSubAdmin = true

    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })
    await flushPromises()

    expect(wrapper.text()).not.toContain('admin.tokenAnalysis.indexNow')
  })

  it.each([
    [999_999, '999,999'],
    [1_000_000, '1.0M'],
    [1_000_000_000, '1.0B']
  ])('formats total Token value %s with M/B only and preserves its exact title', async (value, expected) => {
    api.getSummary.mockResolvedValue(summaryFixture({ total_tokens: value }))

    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })
    await flushPromises()

    const card = wrapper.findAll('.card').find((item) => item.text().includes('admin.tokenAnalysis.summary.totalTokens'))
    const valueElement = card!.find('.mt-2')
    expect(valueElement.text()).toBe(expected)
    expect(valueElement.attributes('title')).toBe(new Intl.NumberFormat().format(value))
  })

  it.each([
    [999, '999'],
    [1_000, '1.0K'],
    [1_000_000, '1.0M'],
    [1_000_000_000, '1.0B']
  ])('formats total request value %s with K/M/B and preserves its exact title', async (value, expected) => {
    api.getSummary.mockResolvedValue(summaryFixture({ total_requests: value }))

    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })
    await flushPromises()

    const card = wrapper.findAll('.card').find((item) => item.text().includes('admin.tokenAnalysis.summary.totalRequests'))
    const valueElement = card!.find('.mt-2')
    expect(valueElement.text()).toBe(expected)
    expect(valueElement.attributes('title')).toBe(new Intl.NumberFormat().format(value))
  })

  it('renders user ranking by user only without API key subtitle', async () => {
    api.listUsers.mockResolvedValue({
      items: [
        {
          user_id: 7,
          user_email: 'dev@example.com',
          api_key_id: 33,
          api_key_name: 'ranking-key',
          request_count: 5,
          risky_request_count: 1,
          total_tokens: 1200,
          input_tokens: 700,
          output_tokens: 100,
          cache_read_tokens: 300,
          cache_creation_tokens: 100,
          actual_cost: 1.5,
          risky_cost: 0.7,
          cache_hit_rate: 0.2727,
          risk_ratio: 0.2,
          last_event_time: '2026-06-12T10:00:00Z'
        }
      ],
      total: 1,
      page: 1,
      page_size: 20
    })

    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })
    await flushPromises()

    const userRankingCard = wrapper.findAll('.card').find((card) => card.text().includes('admin.tokenAnalysis.userRanking'))
    expect(userRankingCard).toBeTruthy()
    expect(userRankingCard!.text()).toContain('dev@example.com')
    expect(userRankingCard!.text()).not.toContain('ranking-key')

    const requestDetailsCard = wrapper.findAll('.card').find((card) => card.text().includes('admin.tokenAnalysis.requestDetails'))
    expect(requestDetailsCard).toBeTruthy()
    expect(requestDetailsCard!.text()).toContain('dev')
  })

  it('formats user ranking Token and cost thresholds while preserving exact titles', async () => {
    api.listUsers.mockResolvedValue({
      items: [
        { user_id: 7, user_email: 'raw@example.com', total_tokens: 1_200, actual_cost: 999.9999 },
        { user_id: 8, user_email: 'million@example.com', total_tokens: 1_000_000, actual_cost: 1_000 },
        { user_id: 9, user_email: 'billion@example.com', total_tokens: 1_000_000_000, actual_cost: 1_200_000 }
      ],
      total: 3,
      page: 1,
      page_size: 20
    })

    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })
    await flushPromises()

    const userRankingCard = wrapper.findAll('.card').find((card) => card.text().includes('admin.tokenAnalysis.userRanking'))
    expect(userRankingCard).toBeTruthy()
    expect(userRankingCard!.text()).toContain('1,200')
    expect(userRankingCard!.text()).not.toContain('0.0M')
    expect(userRankingCard!.text()).toContain('1.0M')
    expect(userRankingCard!.text()).toContain('1.0B')
    expect(userRankingCard!.text()).toContain('$999.9999')
    expect(userRankingCard!.text()).toContain('$1.0K')
    expect(userRankingCard!.text()).toContain('$1200.0K')
    expect(userRankingCard!.find('[title="1,200"]').exists()).toBe(true)
    expect(userRankingCard!.find('[title="$1000.0000"]').exists()).toBe(true)
    expect(userRankingCard!.find('[title="$1200000.0000"]').exists()).toBe(true)
  })

  it('selects ranking users and loads authoritative daily usage trend', async () => {
    api.listUsers.mockResolvedValue({
      items: [userFixture(7, 'a@example.com'), userFixture(8, 'b@example.com')],
      total: 2,
      page: 1,
      page_size: 20
    })
    dashboardApi.getUserUsageTrend.mockResolvedValue({
      trend: [
        { date: '2026-07-01', user_id: 7, email: 'a@example.com', username: '', requests: 1, tokens: 100, cost: 1, actual_cost: 1 },
        { date: '2026-07-01', user_id: 8, email: 'b@example.com', username: '', requests: 1, tokens: 200, cost: 2, actual_cost: 2 }
      ],
      start_date: '2026-07-01',
      end_date: '2026-07-01',
      granularity: 'day'
    })
    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })
    await flushPromises()

    const dateInputs = wrapper.findAll('input[type="date"]')
    await dateInputs[0].setValue('2026-07-01')
    await dateInputs[1].setValue('2026-07-01')
    const selectors = wrapper.findAll('input[data-user-trend-select]')
    await selectors[0].setValue(true)
    await selectors[1].setValue(true)
    await flushPromises()

    expect(dashboardApi.getUserUsageTrend).toHaveBeenLastCalledWith({
      user_ids: [7, 8],
      start_date: '2026-07-01',
      end_date: '2026-07-01',
      granularity: 'day'
    })
    const chart = wrapper.findComponent({ name: 'Line' })
    expect(chart.exists()).toBe(true)
    expect(chart.props('data')).toMatchObject({
      labels: ['2026-07-01'],
      datasets: [
        { label: 'a@example.com', data: [100] },
        { label: 'b@example.com', data: [200] }
      ]
    })
  })

  it('disables unselected users after five selections', async () => {
    api.listUsers.mockResolvedValue({
      items: Array.from({ length: 6 }, (_, index) => userFixture(index + 1, `user-${index + 1}@example.com`)),
      total: 6,
      page: 1,
      page_size: 20
    })
    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })
    await flushPromises()

    let selectors = wrapper.findAll('input[data-user-trend-select]')
    for (const selector of selectors.slice(0, 5)) {
      await selector.setValue(true)
    }
    await flushPromises()
    selectors = wrapper.findAll('input[data-user-trend-select]')

    expect(selectors[5].attributes('disabled')).toBeDefined()
    for (const selector of selectors.slice(0, 5)) {
      expect(selector.attributes('disabled')).toBeUndefined()
    }
    expect(wrapper.text()).toContain('admin.tokenAnalysis.selectedUsersCount')
  })

  it('keeps selected users when the ranking page changes', async () => {
    api.listUsers
      .mockResolvedValueOnce({ items: [userFixture(7, 'page-one@example.com')], total: 40, page: 1, page_size: 20 })
      .mockResolvedValueOnce({ items: [userFixture(8, 'page-two@example.com')], total: 40, page: 2, page_size: 20 })
    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })
    await flushPromises()

    await wrapper.find('input[data-user-trend-select]').setValue(true)
    await flushPromises()
    const userPagination = wrapper.findAllComponents(Pagination)[0]
    expect(userPagination).toBeTruthy()
    userPagination.vm.$emit('update:page', 2)
    await flushPromises()

    expect(api.listUsers).toHaveBeenLastCalledWith(expect.objectContaining({ page: 2 }))
    expect(wrapper.text()).toContain('page-two@example.com')
    expect(dashboardApi.getUserUsageTrend).toHaveBeenLastCalledWith(expect.objectContaining({ user_ids: [7] }))
    expect(wrapper.find('input[data-user-trend-select]').element.checked).toBe(false)
  })

  it('ignores an older trend response after the selection changes and clears stale chart data', async () => {
    const today = new Date().toISOString().slice(0, 10)
    api.listUsers.mockResolvedValue({
      items: [userFixture(7, 'a@example.com'), userFixture(8, 'b@example.com')],
      total: 2,
      page: 1,
      page_size: 20
    })
    const resolvers: Array<(value: unknown) => void> = []
    dashboardApi.getUserUsageTrend.mockImplementation(
      () => new Promise((resolve) => resolvers.push(resolve))
    )
    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })
    await flushPromises()

    const selectors = wrapper.findAll('input[data-user-trend-select]')
    await selectors[0].setValue(true)
    await selectors[1].setValue(true)
    expect(resolvers).toHaveLength(2)
    expect(wrapper.find('[data-testid="selected-user-trend-chart"]').exists()).toBe(false)

    resolvers[1]({
      trend: [
        { date: today, user_id: 7, email: 'a@example.com', username: '', requests: 1, tokens: 10, cost: 0, actual_cost: 0 },
        { date: today, user_id: 8, email: 'b@example.com', username: '', requests: 1, tokens: 20, cost: 0, actual_cost: 0 }
      ]
    })
    await flushPromises()
    const latestData = wrapper.findComponent({ name: 'Line' }).props('data') as { datasets: Array<{ data: number[] }> }
    expect(latestData.datasets.map((dataset) => dataset.data.reduce((sum, value) => sum + value, 0))).toEqual([10, 20])

    resolvers[0]({
      trend: [{ date: today, user_id: 7, email: 'a@example.com', username: '', requests: 1, tokens: 999, cost: 0, actual_cost: 0 }]
    })
    await flushPromises()
    const settledData = wrapper.findComponent({ name: 'Line' }).props('data') as { datasets: Array<{ data: number[] }> }
    expect(settledData.datasets.map((dataset) => dataset.data.reduce((sum, value) => sum + value, 0))).toEqual([10, 20])
  })

  it('fills missing user periods with zero', async () => {
    api.listUsers.mockResolvedValue({ items: [userFixture(7, 'a@example.com')], total: 1, page: 1, page_size: 20 })
    dashboardApi.getUserUsageTrend.mockResolvedValue({
      trend: [{ date: '2026-07-02', user_id: 7, email: 'a@example.com', username: '', requests: 1, tokens: 42, cost: 0, actual_cost: 0 }]
    })
    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })
    await flushPromises()

    const dateInputs = wrapper.findAll('input[type="date"]')
    await dateInputs[0].setValue('2026-07-01')
    await dateInputs[1].setValue('2026-07-03')
    await wrapper.find('input[data-user-trend-select]').setValue(true)
    await flushPromises()

    expect(wrapper.findComponent({ name: 'Line' }).props('data')).toMatchObject({
      labels: ['2026-07-01', '2026-07-02', '2026-07-03'],
      datasets: [{ data: [0, 42, 0] }]
    })
  })

  it('enables hourly mode only for a single selected date', async () => {
    api.listUsers.mockResolvedValue({ items: [userFixture(7, 'a@example.com')], total: 1, page: 1, page_size: 20 })
    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })
    await flushPromises()
    const dateInputs = wrapper.findAll('input[type="date"]')
    await dateInputs[0].setValue('2026-07-01')
    await dateInputs[1].setValue('2026-07-01')
    await wrapper.find('input[data-user-trend-select]').setValue(true)
    await flushPromises()

    const hourButton = wrapper.findAll('button').find((button) => button.text().includes('admin.tokenAnalysis.trendHour'))
    expect(hourButton).toBeTruthy()
    expect(hourButton!.attributes('disabled')).toBeUndefined()
    await hourButton!.trigger('click')
    await flushPromises()

    expect(dashboardApi.getUserUsageTrend).toHaveBeenLastCalledWith(expect.objectContaining({ granularity: 'hour' }))
    const chartData = wrapper.findComponent({ name: 'Line' }).props('data') as { labels: string[]; datasets: Array<{ data: number[] }> }
    expect(chartData.labels).toHaveLength(24)
    expect(chartData.labels[0]).toBe('2026-07-01 00:00')
    expect(chartData.labels[23]).toBe('2026-07-01 23:00')
    expect(chartData.datasets[0].data).toEqual(Array(24).fill(0))
  })

  it('does not request hourly trend for a multi-day range', async () => {
    api.listUsers.mockResolvedValue({ items: [userFixture(7, 'a@example.com')], total: 1, page: 1, page_size: 20 })
    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })
    await flushPromises()
    const dateInputs = wrapper.findAll('input[type="date"]')
    await dateInputs[0].setValue('2026-07-01')
    await dateInputs[1].setValue('2026-07-02')
    await wrapper.find('input[data-user-trend-select]').setValue(true)
    await flushPromises()

    const callsBefore = dashboardApi.getUserUsageTrend.mock.calls.length
    const hourButton = wrapper.findAll('button').find((button) => button.text().includes('admin.tokenAnalysis.trendHour'))
    expect(hourButton).toBeTruthy()
    expect(hourButton!.attributes('disabled')).toBeDefined()
    await hourButton!.trigger('click')
    expect(dashboardApi.getUserUsageTrend).toHaveBeenCalledTimes(callsBefore)
  })

  it('clears stale chart data and exposes retry after a load error', async () => {
    api.listUsers.mockResolvedValue({ items: [userFixture(7, 'a@example.com')], total: 1, page: 1, page_size: 20 })
    dashboardApi.getUserUsageTrend.mockRejectedValueOnce(new Error('network'))
    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })
    await flushPromises()
    await wrapper.find('input[data-user-trend-select]').setValue(true)
    await flushPromises()

    expect(wrapper.text()).toContain('admin.tokenAnalysis.trendLoadFailed')
    expect(wrapper.find('[data-testid="selected-user-trend-chart"]').exists()).toBe(false)

    dashboardApi.getUserUsageTrend.mockResolvedValueOnce({ trend: [] })
    const retry = wrapper.findAll('button').find((button) => button.text().includes('admin.tokenAnalysis.trendRetry'))
    expect(retry).toBeTruthy()
    await retry!.trigger('click')
    await flushPromises()

    expect(dashboardApi.getUserUsageTrend).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="selected-user-trend-chart"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('admin.tokenAnalysis.trendNoUsage')
  })
  it('renders project ranking with compact tokens, UTC+8 time and sortable headers', async () => {
    api.listProjects.mockResolvedValue({
      items: [
        {
          project: 'lag-killer',
          user_id: 7,
          user_email: 'dev@example.com',
          request_count: 1234,
          matched_request_count: 1200,
          total_tokens: 1_234_000_000,
          input_tokens: 56_700_000,
          output_tokens: 890_000,
          cache_read_tokens: 12_000_000,
          cache_creation_tokens: 3_000_000,
          actual_cost: 12.3456,
          last_event_time: '2026-06-10T01:02:03Z'
        },
        {
          project: 'chat-heavy',
          user_id: 8,
          user_email: 'writer@example.com',
          request_count: 42,
          matched_request_count: 40,
          total_tokens: 12_000,
          input_tokens: 100_000,
          output_tokens: 8_000,
          cache_read_tokens: 0,
          cache_creation_tokens: 0,
          actual_cost: 98.7654,
          last_event_time: '2026-06-10T02:03:04Z'
        }
      ],
      total: 2,
      page: 1,
      page_size: 20
    })
    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })
    await flushPromises()

    // 默认按 total_tokens 降序请求。
    expect(api.listProjects).toHaveBeenCalledWith(
      expect.objectContaining({ sort_by: 'total_tokens', sort_order: 'desc' })
    )
    // Token / Input / Output / 缓存三列以 K/M/B 紧凑展示。
    expect(wrapper.text()).toContain('1.2B')
    expect(wrapper.text()).toContain('56.7M')
    expect(wrapper.text()).toContain('890.0K')
    expect(wrapper.text()).toContain('15.0M')
    // 项目排行不再展示费用列, 改为输出/输入比例; 5% 以下红色, 5% 及以上绿色。
    expect(wrapper.text()).not.toContain('$12.3456')
    expect(wrapper.text()).not.toContain('$98.7654')
    expect(wrapper.text()).toContain('1.6%')
    expect(wrapper.text()).toContain('8.0%')
    const lowRatio = wrapper.find('[data-test="project-io-ratio-low"]')
    const healthyRatio = wrapper.find('[data-test="project-io-ratio-healthy"]')
    expect(lowRatio.exists()).toBe(true)
    expect(lowRatio.text()).toContain('1.6%')
    expect(lowRatio.classes()).toContain('text-red-600')
    expect(healthyRatio.exists()).toBe(true)
    expect(healthyRatio.text()).toContain('8.0%')
    expect(healthyRatio.classes()).toContain('text-emerald-600')
    // 最近活动按东八区展示, 不带 ISO 的 T/时区尾巴。
    expect(wrapper.text()).toContain('2026-06-10 09:02:03')
    expect(wrapper.text()).not.toContain('2026-06-10T01:02:03Z')

    // 点击"请求数"表头 → 改列默认降序; 再点同列翻转为升序。
    const th = wrapper.findAll('th button').find((b) => b.text().includes('admin.tokenAnalysis.requests'))
    expect(th).toBeTruthy()
    await th!.trigger('click')
    await flushPromises()
    expect(api.listProjects).toHaveBeenLastCalledWith(
      expect.objectContaining({ sort_by: 'request_count', sort_order: 'desc' })
    )
    await th!.trigger('click')
    await flushPromises()
    expect(api.listProjects).toHaveBeenLastCalledWith(
      expect.objectContaining({ sort_by: 'request_count', sort_order: 'asc' })
    )
  })

  it('lazy loads full user input when a request row is opened', async () => {
    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })
    await flushPromises()

    const row = wrapper.findAll('tbody tr').find((r) => r.text().includes('user@example.com'))
    expect(row).toBeTruthy()
    await row!.trigger('click')
    await flushPromises()

    expect(api.getRequestInput).toHaveBeenCalledWith('arch-1')
    expect(wrapper.text()).toContain('full input line one')
    // 抽屉新增: 内容哈希行 + 字符数。
    expect(wrapper.text()).toContain('admin.tokenAnalysis.contentHash')
    expect(wrapper.text()).toContain('abc')
  })

  it('shows userRequest content before the full input when present', async () => {
    api.getRequestInput.mockResolvedValue({
      id: 1,
      archive_id: 'arch-1',
      event_time: '2026-05-19T01:00:00Z',
      content: '<context>ignored</context>\n<userRequest>Put this first\nwith line two</userRequest>\n<notes>keep below</notes>',
      content_sha256: 'abc',
      chars: 96,
      truncated: false,
      quality_version: ''
    })
    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })
    await flushPromises()

    const row = wrapper.findAll('tbody tr').find((r) => r.text().includes('user@example.com'))
    expect(row).toBeTruthy()
    await row!.trigger('click')
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('admin.tokenAnalysis.userRequest')
    expect(text).toContain('Put this first')
    expect(text.indexOf('Put this first')).toBeLessThan(text.indexOf('<context>ignored</context>'))
  })
  it('keeps loading state for the newly opened request when an older input request resolves late', async () => {
    // 竞态回归: 连续点击 A→B, A 的旧 promise 晚到时不得关闭 B 的 loading,
    // 更不能把 A 的全文展示在 B 的抽屉里。
    const resolvers = new Map<string, (input: unknown) => void>()
    api.getRequestInput.mockImplementation(
      (archiveId: string) =>
        new Promise((resolve) => {
          resolvers.set(archiveId, resolve)
        })
    )
    const baseItem = {
      id: 1,
      event_time: '2026-05-19T01:00:00Z',
      api_key_name: 'dev',
      model: 'gpt-4.1',
      input_tokens: 1000,
      output_tokens: 32,
      cache_read_tokens: 0,
      cache_creation_tokens: 0,
      actual_cost: 0.1,
      risk_score: 0,
      risk_reasons: [],
      last_user_preview: 'preview',
      match_confidence: 3,
      client_project: 'lag-killer',
      client_branch: 'main',
      has_input: true,
      input_truncated: false
    }
    api.listRequests.mockResolvedValue({
      items: [
        { ...baseItem, id: 1, archive_id: 'arch-1', user_email: 'user@example.com' },
        { ...baseItem, id: 2, archive_id: 'arch-2', user_email: 'second@example.com' }
      ],
      total: 2,
      page: 1,
      page_size: 20
    })

    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })
    await flushPromises()

    const rows = wrapper.findAll('tbody tr')
    const rowA = rows.find((r) => r.text().includes('user@example.com'))
    const rowB = rows.find((r) => r.text().includes('second@example.com'))
    expect(rowA).toBeTruthy()
    expect(rowB).toBeTruthy()
    await rowA!.trigger('click')
    await rowB!.trigger('click')

    // 旧请求 A 先返回: B 的抽屉应继续 loading, 不显示 A 的内容。
    resolvers.get('arch-1')!({
      id: 1,
      archive_id: 'arch-1',
      event_time: '2026-05-19T01:00:00Z',
      content: 'stale input from A',
      content_sha256: 'aaa',
      chars: 18,
      truncated: false,
      quality_version: ''
    })
    await flushPromises()
    expect(wrapper.text()).toContain('common.loading')
    expect(wrapper.text()).not.toContain('stale input from A')

    // B 自己的响应到达后正常展示。
    resolvers.get('arch-2')!({
      id: 2,
      archive_id: 'arch-2',
      event_time: '2026-05-19T01:00:00Z',
      content: 'fresh input from B',
      content_sha256: 'bbb',
      chars: 18,
      truncated: false,
      quality_version: ''
    })
    await flushPromises()
    expect(wrapper.text()).not.toContain('common.loading')
    expect(wrapper.text()).toContain('fresh input from B')
  })

  it('triggers index asynchronously and polls status until finished', async () => {
    // 索引触发已改异步(202): POST 立即返回, 之后每 3s 轮询状态,
    // running 翻 false 时停止轮询并刷新页面数据。
    vi.useFakeTimers()
    try {
      api.triggerIndex.mockResolvedValue(undefined)
      const wrapper = mount(TokenAnalysisView, {
        global: { stubs: { AppLayout: AppLayoutStub } }
      })
      await flushPromises()
      const mountCalls = api.getIndexStatus.mock.calls.length

      const button = wrapper.findAll('button').find((b) => b.text().includes('admin.tokenAnalysis.indexNow'))
      expect(button).toBeTruthy()

      // 第一轮轮询返回 running=true, 之后回落到默认 mock(running=false)。
      api.getIndexStatus.mockResolvedValueOnce({ running: true, processed_rows: 0, failed_rows: 0, files: [] })
      await button!.trigger('click')
      await flushPromises()
      expect(api.triggerIndex).toHaveBeenCalledTimes(1)
      // 轮询期间按钮保持禁用。
      expect(button!.attributes('disabled')).toBeDefined()

      await vi.advanceTimersByTimeAsync(3000)
      await flushPromises()
      expect(api.getIndexStatus.mock.calls.length).toBe(mountCalls + 1)
      expect(button!.attributes('disabled')).toBeDefined()

      // 第二轮 running=false: 停止轮询并 reloadAll(其中再拉一次状态)。
      await vi.advanceTimersByTimeAsync(3000)
      await flushPromises()
      expect(button!.attributes('disabled')).toBeUndefined()

      const settled = api.getIndexStatus.mock.calls.length
      await vi.advanceTimersByTimeAsync(15000)
      await flushPromises()
      expect(api.getIndexStatus.mock.calls.length).toBe(settled)
    } finally {
      vi.useRealTimers()
    }
  })

  it('follows an already running index on mount and stops polling on unmount', async () => {
    vi.useFakeTimers()
    try {
      api.getIndexStatus.mockResolvedValue({ running: true, processed_rows: 0, failed_rows: 0, files: [] })
      const wrapper = mount(TokenAnalysisView, {
        global: { stubs: { AppLayout: AppLayoutStub } }
      })
      await flushPromises()
      const mountCalls = api.getIndexStatus.mock.calls.length

      // 进页面时已有索引在跑(如自动索引): 自动开始轮询。
      await vi.advanceTimersByTimeAsync(3000)
      await flushPromises()
      expect(api.getIndexStatus.mock.calls.length).toBe(mountCalls + 1)

      // 卸载后定时器必须清理, 不再发请求。
      wrapper.unmount()
      const settled = api.getIndexStatus.mock.calls.length
      await vi.advanceTimersByTimeAsync(15000)
      await flushPromises()
      expect(api.getIndexStatus.mock.calls.length).toBe(settled)
    } finally {
      vi.useRealTimers()
    }
  })
})
