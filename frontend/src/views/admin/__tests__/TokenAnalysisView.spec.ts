import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import TokenAnalysisView from '../TokenAnalysisView.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'
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
    // DateRangePicker 读取 locale 格式化日期显示。
    useI18n: () => ({ t: (key: string) => key, locale: { value: 'zh' } })
  }
})

const AppLayoutStub = { template: '<div><slot /></div>' }

function mountView() {
  return mount(TokenAnalysisView, { global: { stubs: { AppLayout: AppLayoutStub } } })
}

function pageOf<T>(items: T[], total: number) {
  return { items, total, page: 1, page_size: 20 }
}

async function setDateRange(wrapper: VueWrapper, startDate: string, endDate: string) {
  wrapper.findComponent(DateRangePicker).vm.$emit('change', { startDate, endDate, preset: null })
  await flushPromises()
}

function localToday(): string {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`
}

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
    human_input_count: 7,
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
    api.listProjects.mockResolvedValue(pageOf([], 0))
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
    api.listUsers.mockResolvedValue(pageOf([], 0))
    dashboardApi.getUserUsageTrend.mockResolvedValue({
      trend: [],
      start_date: '2026-07-01',
      end_date: '2026-07-01',
      granularity: 'day'
    })
    api.listRequests.mockResolvedValue(pageOf([
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
          usage_log_id: 11,
          usage_link_status: 'linked',
          usage_match_method: 'request_id',
          client_project: 'lag-killer',
          client_branch: 'main',
          has_input: true,
          input_truncated: false,
          request_body_truncated: true,
          duplicate_count: 3
        }
      ], 1))
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
    const wrapper = mountView()

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
    // 人工输入次数(估算)附在已归档请求卡片里, 说明放在 title。
    const requestsCard = wrapper.get('[data-testid="token-analysis-metric-requests"]')
    expect(requestsCard.text()).toContain('admin.tokenAnalysis.summary.humanInputs')
    expect(requestsCard.find('[title="admin.tokenAnalysis.help.humanInputs"]').text()).toBe('7')
    // 归档文件: 水位追平的文件进度显示"已读完", 并标注请求行合计。
    expect(wrapper.text()).toContain('100% · admin.tokenAnalysis.fullyRead')
    expect(wrapper.text()).toContain('admin.tokenAnalysis.requestRowsTotal')
  })

  it('shows request origins from validated server metadata and leaves legacy or invalid records unclassified', async () => {
    const cases = [
      { origin: { kind: 'human', reason: 'user_input', confidence: 'medium', version: 1 }, label: 'human', reason: 'user_input' },
      { origin: { kind: 'ai', reason: 'tool_result', confidence: 'high', version: 1 }, label: 'ai', reason: 'tool_result' },
      { origin: { kind: 'unknown', reason: 'no_signal', confidence: 'none', version: 1 }, label: null, reason: 'no_signal' },
      { origin: undefined, label: null, reason: null },
      // v2 keeps every v1 reason and adds compaction_resume as the only medium-confidence AI reason.
      { origin: { kind: 'ai', reason: 'tool_result', confidence: 'high', version: 2 }, label: 'ai', reason: 'tool_result' },
      { origin: { kind: 'ai', reason: 'compaction_resume', confidence: 'medium', version: 2 }, label: 'ai', reason: 'compaction_resume' },
      { origin: { kind: 'ai', reason: 'compaction_resume', confidence: 'high', version: 2 }, label: null, reason: null },
      { origin: { kind: 'ai', reason: 'compaction_resume', confidence: 'medium', version: 1 }, label: null, reason: null },
      { origin: { kind: 'ai', reason: 'tool_result', confidence: 'high', version: 3 }, label: null, reason: null },
      { origin: { kind: 'human', reason: 'user_input', confidence: 'high', version: 1 }, label: null, reason: null },
      { origin: { kind: 'ai', reason: 'future_reason', confidence: 'high', version: 1 }, label: null, reason: null },
      { origin: { kind: 'other', reason: 'tool_result', confidence: 'high', version: 1 }, label: null, reason: null },
      { origin: 'human', label: null, reason: null },
      { origin: null, label: null, reason: null }
    ]
    api.listRequests.mockResolvedValue({
      items: cases.map(({ origin }, index) => ({
        id: index + 1,
        archive_id: `origin-${index}`,
        event_time: '2026-05-19T01:00:00Z',
        user_email: `origin-${index}@example.com`,
        model: 'gpt-4.1',
        risk_score: 0,
        risk_reasons: [],
        last_user_preview: '<userRequest>human-looking text</userRequest>',
        duplicate_count: 8,
        summary_json: origin === undefined ? undefined : { request_origin: origin },
        has_input: false
      })),
      total: cases.length,
      page: 1,
      page_size: 20
    })
    const wrapper = mountView()
    await flushPromises()

    const table = wrapper.get('[data-testid="token-analysis-request-details"]')
    const badges = table.findAll('[data-testid="request-origin"]')
    expect(badges).toHaveLength(cases.length)
    cases.forEach(({ label, reason }, index) => {
      expect(badges[index].text()).toBe(label ? `admin.tokenAnalysis.requestOrigin.${label}` : '—')
      expect(badges[index].attributes('title')).toContain(reason
        ? `admin.tokenAnalysis.requestOrigin.reasons.${reason}`
        : 'admin.tokenAnalysis.requestOrigin.notIdentified')
    })
    expect(table.text()).toContain('admin.tokenAnalysis.requestOrigin.label')
    expect(table.text()).toContain('admin.tokenAnalysis.requestOrigin.hint')
    expect(api.listRequests).toHaveBeenCalledTimes(1)
    expect(api.getRequestInput).not.toHaveBeenCalled()
  })

  it.each([
    { kind: 'human', reason: 'user_input', confidence: 'medium' },
    { kind: 'ai', reason: 'tool_result', confidence: 'high' },
    { kind: 'unknown', reason: 'body_truncated', confidence: 'none' },
    { kind: 'unknown', reason: 'count_tokens', confidence: 'none' },
    { kind: 'ai', reason: 'compaction_resume', confidence: 'medium', version: 2 }
  ])('explains the $kind/$reason origin in the drawer without fetching input for classification', async (origin) => {
    api.listRequests.mockResolvedValue(pageOf([{
        id: 1,
        archive_id: 'origin-detail',
        event_time: '2026-05-19T01:00:00Z',
        user_email: 'origin-detail@example.com',
        model: 'gpt-4.1',
        risk_score: 0,
        risk_reasons: [],
        summary_json: { request_origin: { version: 1, ...origin } },
        has_input: false
      }], 1))
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="token-analysis-request-details"] tbody tr').trigger('click')
    await flushPromises()

    const detail = wrapper.get('[data-testid="request-origin-detail"]')
    expect(detail.text()).toContain(origin.kind === 'unknown' ? '—' : `admin.tokenAnalysis.requestOrigin.${origin.kind}`)
    expect(detail.text()).toContain('admin.tokenAnalysis.requestOrigin.inferred')
    expect(detail.text()).toContain(`admin.tokenAnalysis.requestOrigin.reasons.${origin.reason}`)
    expect(detail.text()).toContain(`admin.tokenAnalysis.requestOrigin.confidences.${origin.confidence}`)
    expect(detail.text()).toContain('admin.tokenAnalysis.requestOrigin.humanHint')
    expect(api.listRequests).toHaveBeenCalledTimes(1)
    expect(api.getRequestInput).not.toHaveBeenCalled()
  })

  it('shows unknown usage with the billing link status instead of zero tokens', async () => {
    const base = {
      event_time: '2026-05-19T01:00:00Z',
      user_email: 'link@example.com',
      model: 'gpt-6-astra',
      input_tokens: 0,
      output_tokens: 0,
      cache_read_tokens: 0,
      cache_creation_tokens: 0,
      actual_cost: 0,
      risk_score: 0,
      risk_reasons: [],
      match_confidence: 0,
      has_input: false
    }
    api.listRequests.mockResolvedValue(pageOf([
      { ...base, id: 1, archive_id: 'linked', usage_log_id: 9, usage_link_status: 'linked', usage_match_method: 'timing', input_tokens: 294, output_tokens: 547, cache_read_tokens: 156288, actual_cost: 0.186578, match_confidence: 3 },
      { ...base, id: 2, archive_id: 'unlinked', usage_link_status: 'unlinked' },
      { ...base, id: 3, archive_id: 'failed', usage_link_status: 'failed', response_status: 502 },
      { ...base, id: 4, archive_id: 'probe', usage_link_status: 'not_billable', endpoint: '/v1/messages/count_tokens' },
      // 旧接口没有 usage_link_status: 无 usage_log_id 按未关联处理。
      { ...base, id: 5, archive_id: 'old-api' }
    ], 5))
    const wrapper = mountView()
    await flushPromises()

    const rows = wrapper.findAll('[data-testid="token-analysis-request-details"] tbody tr')
    const usageCell = (index: number) => rows[index].findAll('td')[5]
    expect(usageCell(0).text()).toContain('157,129')
    expect(usageCell(0).text()).toContain('$0.1866')
    expect(usageCell(0).find('[data-testid="request-usage-link"]').exists()).toBe(false)
    ;(['unlinked', 'failed', 'not_billable', 'unlinked'] as const).forEach((status, offset) => {
      const cell = usageCell(offset + 1)
      expect(cell.text()).toContain('—')
      expect(cell.text()).not.toContain('$0')
      const tag = cell.get('[data-testid="request-usage-link"]')
      expect(tag.text()).toBe(`admin.tokenAnalysis.usageLink.statuses.${status}`)
      expect(tag.attributes('title')).toBe(`admin.tokenAnalysis.usageLink.statusHints.${status}`)
    })

    await rows[1].trigger('click')
    await flushPromises()
    let detail = wrapper.get('[data-testid="request-usage-link-detail"]')
    expect(detail.text()).not.toMatch(/\d/)
    expect(detail.text()).toContain('admin.tokenAnalysis.usageLink.statusHints.unlinked')
    const drawer = wrapper.get('[data-testid="token-analysis-request-drawer"]')
    expect(drawer.text()).toContain('admin.tokenAnalysis.usageLink.title')
    expect(drawer.text()).not.toContain('admin.tokenAnalysis.usageLink.methods')

    await rows[0].trigger('click')
    await flushPromises()
    detail = wrapper.get('[data-testid="request-usage-link-detail"]')
    expect(detail.text()).toContain('547')
    expect(detail.text()).not.toContain('admin.tokenAnalysis.usageLink.statusHints')
    expect(wrapper.get('[data-testid="token-analysis-request-drawer"]').text()).toContain('admin.tokenAnalysis.usageLink.methods.timing')
  })

  it('hides the manual index action for sub admins', async () => {
    authState.isAdmin = false
    authState.isSubAdmin = true

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).not.toContain('admin.tokenAnalysis.indexNow')
  })

  it.each([
    [999_999, '999,999'],
    [1_000_000, '1.0M'],
    [1_000_000_000, '1.0B']
  ])('formats total Token value %s with M/B only and preserves its exact title', async (value, expected) => {
    api.getSummary.mockResolvedValue(summaryFixture({ total_tokens: value }))

    const wrapper = mountView()
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

    const wrapper = mountView()
    await flushPromises()

    const card = wrapper.findAll('.card').find((item) => item.text().includes('admin.tokenAnalysis.summary.totalRequests'))
    const valueElement = card!.find('.mt-2')
    expect(valueElement.text()).toBe(expected)
    expect(valueElement.attributes('title')).toBe(new Intl.NumberFormat().format(value))
  })

  it('renders user ranking by user only without API key subtitle', async () => {
    api.listUsers.mockResolvedValue(pageOf([
        {
          user_id: 7,
          user_email: 'dev@example.com',
          api_key_id: 33,
          api_key_name: 'ranking-key',
          request_count: 5,
          risky_request_count: 1,
          human_input_count: 3,
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
      ], 1))

    const wrapper = mountView()
    await flushPromises()

    const userRankingCard = wrapper.findAll('.card').find((card) => card.text().includes('admin.tokenAnalysis.userRanking'))
    expect(userRankingCard).toBeTruthy()
    expect(userRankingCard!.text()).toContain('dev@example.com')
    expect(userRankingCard!.text()).not.toContain('ranking-key')
    expect(userRankingCard!.text()).toContain('admin.tokenAnalysis.humanInputs 3')

    const requestDetailsCard = wrapper.findAll('.card').find((card) => card.text().includes('admin.tokenAnalysis.requestDetails'))
    expect(requestDetailsCard).toBeTruthy()
    expect(requestDetailsCard!.text()).toContain('dev')
  })

  it('formats user ranking Token and cost thresholds while preserving exact titles', async () => {
    api.listUsers.mockResolvedValue(pageOf([
        { user_id: 7, user_email: 'raw@example.com', total_tokens: 1_200, actual_cost: 999.9999 },
        { user_id: 8, user_email: 'million@example.com', total_tokens: 1_000_000, actual_cost: 1_000 },
        { user_id: 9, user_email: 'billion@example.com', total_tokens: 1_000_000_000, actual_cost: 1_200_000 }
      ], 3))

    const wrapper = mountView()
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
    api.listUsers.mockResolvedValue(pageOf([userFixture(7, 'a@example.com'), userFixture(8, 'b@example.com')], 2))
    dashboardApi.getUserUsageTrend.mockResolvedValue({
      trend: [
        { date: '2026-07-01', user_id: 7, email: 'a@example.com', username: '', requests: 1, tokens: 100, cost: 1, actual_cost: 1 },
        { date: '2026-07-01', user_id: 8, email: 'b@example.com', username: '', requests: 1, tokens: 200, cost: 2, actual_cost: 2 }
      ],
      start_date: '2026-07-01',
      end_date: '2026-07-01',
      granularity: 'day'
    })
    const wrapper = mountView()
    await flushPromises()

    await setDateRange(wrapper, '2026-07-01', '2026-07-01')
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
    const wrapper = mountView()
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
      .mockResolvedValueOnce(pageOf([userFixture(7, 'page-one@example.com')], 40))
      .mockResolvedValueOnce({ items: [userFixture(8, 'page-two@example.com')], total: 40, page: 2, page_size: 20 })
    const wrapper = mountView()
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
    const today = localToday()
    api.listUsers.mockResolvedValue(pageOf([userFixture(7, 'a@example.com'), userFixture(8, 'b@example.com')], 2))
    const resolvers: Array<(value: unknown) => void> = []
    dashboardApi.getUserUsageTrend.mockImplementation(
      () => new Promise((resolve) => resolvers.push(resolve))
    )
    const wrapper = mountView()
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
    api.listUsers.mockResolvedValue(pageOf([userFixture(7, 'a@example.com')], 1))
    dashboardApi.getUserUsageTrend.mockResolvedValue({
      trend: [{ date: '2026-07-02', user_id: 7, email: 'a@example.com', username: '', requests: 1, tokens: 42, cost: 0, actual_cost: 0 }]
    })
    const wrapper = mountView()
    await flushPromises()

    await setDateRange(wrapper, '2026-07-01', '2026-07-03')
    await wrapper.find('input[data-user-trend-select]').setValue(true)
    await flushPromises()

    expect(wrapper.findComponent({ name: 'Line' }).props('data')).toMatchObject({
      labels: ['2026-07-01', '2026-07-02', '2026-07-03'],
      datasets: [{ data: [0, 42, 0] }]
    })
  })

  it('enables hourly mode only for a single selected date', async () => {
    api.listUsers.mockResolvedValue(pageOf([userFixture(7, 'a@example.com')], 1))
    const wrapper = mountView()
    await flushPromises()
    await setDateRange(wrapper, '2026-07-01', '2026-07-01')
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
    api.listUsers.mockResolvedValue(pageOf([userFixture(7, 'a@example.com')], 1))
    const wrapper = mountView()
    await flushPromises()
    await setDateRange(wrapper, '2026-07-01', '2026-07-02')
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
    api.listUsers.mockResolvedValue(pageOf([userFixture(7, 'a@example.com')], 1))
    dashboardApi.getUserUsageTrend.mockRejectedValueOnce(new Error('network'))
    const wrapper = mountView()
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
    api.listProjects.mockResolvedValue(pageOf([
        {
          project: 'lag-killer',
          user_id: 7,
          user_email: 'dev@example.com',
          request_count: 1234,
          matched_request_count: 1200,
          human_input_count: 17,
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
      ], 2))
    const wrapper = mountView()
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
    expect(wrapper.text()).toContain('admin.tokenAnalysis.humanInputs 17')
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
    const wrapper = mountView()
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
    const wrapper = mountView()
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
    api.listRequests.mockResolvedValue(pageOf([
        { ...baseItem, id: 1, archive_id: 'arch-1', user_email: 'user@example.com' },
        { ...baseItem, id: 2, archive_id: 'arch-2', user_email: 'second@example.com' }
      ], 2))

    const wrapper = mountView()
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
      const wrapper = mountView()
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
      const wrapper = mountView()
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

  it('keeps each trend user on its color slot when another user is removed', async () => {
    api.listUsers.mockResolvedValue(pageOf([userFixture(7, 'a@example.com'), userFixture(8, 'b@example.com'), userFixture(9, 'c@example.com')], 3))
    const wrapper = mountView()
    await flushPromises()

    const selectors = wrapper.findAll('input[data-user-trend-select]')
    await selectors[0].setValue(true)
    await selectors[1].setValue(true)
    await flushPromises()
    const colorOf = (label: string) => {
      const data = wrapper.findComponent({ name: 'Line' }).props('data') as { datasets: Array<{ label: string; borderColor: string }> }
      return data.datasets.find((dataset) => dataset.label === label)?.borderColor
    }
    const bColor = colorOf('b@example.com')

    // 取消第一个用户: 第二个用户保留原颜色; 新选用户补用空出的第一个槽位。
    await wrapper.findAll('input[data-user-trend-select]')[0].setValue(false)
    await flushPromises()
    expect(colorOf('b@example.com')).toBe(bColor)
    await wrapper.findAll('input[data-user-trend-select]')[2].setValue(true)
    await flushPromises()
    expect(colorOf('b@example.com')).toBe(bColor)
    expect(colorOf('c@example.com')).not.toBe(bColor)
  })

  it('removes a trend user from the chip list and clears all selections', async () => {
    api.listUsers.mockResolvedValue(pageOf([userFixture(7, 'a@example.com'), userFixture(8, 'b@example.com')], 2))
    const wrapper = mountView()
    await flushPromises()
    const selectors = wrapper.findAll('input[data-user-trend-select]')
    await selectors[0].setValue(true)
    await selectors[1].setValue(true)
    await flushPromises()

    const trend = wrapper.find('[data-testid="token-analysis-user-trend"]')
    await trend.find('button[aria-label="admin.tokenAnalysis.removeTrendUser"]').trigger('click')
    await flushPromises()
    expect(dashboardApi.getUserUsageTrend).toHaveBeenLastCalledWith(expect.objectContaining({ user_ids: [8] }))
    expect(wrapper.findAll('input[data-user-trend-select]')[0].element.checked).toBe(false)

    const clear = trend.findAll('button').find((button) => button.text() === 'admin.tokenAnalysis.clearTrendUsers')
    await clear!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="token-analysis-trend-empty"]').exists()).toBe(true)
  })

  it('shows clicked project and risk reason as removable filter chips', async () => {
    api.listProjects.mockResolvedValue(pageOf([{
        project: 'lag-killer',
        user_id: 7,
        user_email: 'dev@example.com',
        request_count: 1,
        matched_request_count: 1,
        total_tokens: 10,
        input_tokens: 5,
        output_tokens: 5,
        cache_read_tokens: 0,
        cache_creation_tokens: 0,
        actual_cost: 0,
        last_event_time: '2026-06-10T01:02:03Z'
      }], 1))
    const wrapper = mountView()
    await flushPromises()
    const chips = () => wrapper.find('[data-testid="token-analysis-active-filters"]')
    expect(chips().text()).toContain('admin.tokenAnalysis.noActiveFilters')

    const projectButton = wrapper.find('[data-testid="token-analysis-project-ranking"] tbody button')
    await projectButton.trigger('click')
    await flushPromises()
    expect(api.listRequests).toHaveBeenLastCalledWith(expect.objectContaining({ project: 'lag-killer' }))

    const riskButton = wrapper.find('[data-testid="token-analysis-risk-reasons"] button[aria-pressed]')
    await riskButton.trigger('click')
    await flushPromises()
    expect(api.getSummary).toHaveBeenLastCalledWith(expect.objectContaining({ risk_reason: 'huge_input_tiny_output' }))
    expect(chips().text()).toContain('admin.tokenAnalysis.project: lag-killer')
    expect(chips().text()).toContain('admin.tokenAnalysis.riskReason: huge_input_tiny_output')
    expect(wrapper.find('[data-testid="token-analysis-risk-reasons"] button[aria-pressed="true"]').exists()).toBe(true)

    await chips().find('button[aria-label="admin.tokenAnalysis.removeFilter"]').trigger('click')
    await flushPromises()
    const lastCall = api.listRequests.mock.calls.at(-1)![0]
    expect(lastCall.project).toBeUndefined()
    expect(lastCall.risk_reason).toBe('huge_input_tiny_output')
  })

  it('filters every archive view by call origin and clears it from the chip', async () => {
    const wrapper = mountView()
    await flushPromises()
    const originSelect = wrapper.get('[data-testid="token-analysis-origin-filter"]').findComponent(Select)
    expect((originSelect.props('options') as Array<{ value: string }>).map((option) => option.value)).toEqual(['', 'human', 'ai', 'unknown'])

    originSelect.vm.$emit('update:model-value', 'ai')
    await flushPromises()
    for (const call of [api.getSummary, api.listUsers, api.listProjects, api.listRequests]) {
      expect(call).toHaveBeenLastCalledWith(expect.objectContaining({ origin: 'ai' }))
    }
    const chips = wrapper.get('[data-testid="token-analysis-active-filters"]')
    expect(chips.text()).toContain('admin.tokenAnalysis.requestOrigin.label: admin.tokenAnalysis.requestOrigin.ai')

    originSelect.vm.$emit('update:model-value', 'unknown')
    await flushPromises()
    expect(api.listRequests).toHaveBeenLastCalledWith(expect.objectContaining({ origin: 'unknown' }))
    expect(chips.text()).toContain('admin.tokenAnalysis.requestOrigin.notIdentified')

    await chips.get('button[aria-label="admin.tokenAnalysis.removeFilter"]').trigger('click')
    await flushPromises()
    expect(api.listRequests.mock.calls.at(-1)![0].origin).toBeUndefined()
    expect(originSelect.props('modelValue')).toBe('')
  })

  it('resets dimension filters and dates back to today', async () => {
    const wrapper = mountView()
    await flushPromises()
    await setDateRange(wrapper, '2026-07-01', '2026-07-03')
    await wrapper.find('input[type="search"]').setValue('someone')
    await wrapper.find('input[type="search"]').trigger('keyup.enter')
    await flushPromises()
    expect(api.listRequests).toHaveBeenLastCalledWith(expect.objectContaining({ user_email: 'someone', start_date: '2026-07-01' }))

    const reset = wrapper.findAll('button').find((button) => button.text() === 'admin.tokenAnalysis.reset')
    await reset!.trigger('click')
    await flushPromises()
    const lastCall = api.listRequests.mock.calls.at(-1)![0]
    expect(lastCall.user_email).toBeUndefined()
    expect(lastCall.start_date).toBe(localToday())
    expect(lastCall.end_date).toBe(localToday())
  })

  it('keeps sorting and paging on the submitted filters while a draft is pending', async () => {
    api.listRequests.mockResolvedValue(pageOf([], 40))
    const wrapper = mountView()
    await flushPromises()
    const modelInput = wrapper.find('input[aria-label="admin.tokenAnalysis.model"]')
    await modelInput.setValue('model-a')
    await modelInput.trigger('keyup.enter')
    await flushPromises()
    // 改了输入框但未提交: 排序与翻页续查仍用已提交的 model-a。
    await modelInput.setValue('model-b')

    const riskSort = wrapper.findAll('button').find((button) => button.text() === 'admin.tokenAnalysis.sortByRisk')
    await riskSort!.trigger('click')
    await flushPromises()
    expect(api.listRequests).toHaveBeenLastCalledWith(expect.objectContaining({ model: 'model-a', sort_by: 'risk_score' }))

    const requestPagination = wrapper.find('[data-testid="token-analysis-request-details"]').findComponent(Pagination)
    requestPagination.vm.$emit('update:page', 2)
    await flushPromises()
    expect(api.listRequests).toHaveBeenLastCalledWith(expect.objectContaining({ model: 'model-a', page: 2 }))
    expect(wrapper.find('[data-testid="token-analysis-active-filters"]').text()).toContain('admin.tokenAnalysis.model: model-a')
  })

  it('switches to the index tab from the index status pill', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('[data-testid="token-analysis-tab-users"]').attributes('aria-selected')).toBe('true')

    await wrapper.find('[data-testid="token-analysis-index-pill"]').trigger('click')
    expect(wrapper.find('[data-testid="token-analysis-tab-index"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.find('#token-analysis-panel-index').isVisible()).toBe(true)
    expect(wrapper.find('#token-analysis-panel-users').isVisible()).toBe(false)
  })
})
