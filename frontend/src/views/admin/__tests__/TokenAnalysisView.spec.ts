import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import TokenAnalysisView from '../TokenAnalysisView.vue'

const api = vi.hoisted(() => ({
  getSummary: vi.fn(),
  listUsers: vi.fn(),
  listProjects: vi.fn(),
  listRequests: vi.fn(),
  getRequestInput: vi.fn(),
  getIndexStatus: vi.fn(),
  triggerIndex: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    tokenAnalysis: api
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const AppLayoutStub = { template: '<div><slot /></div>' }

describe('TokenAnalysisView', () => {
  beforeEach(() => {
    api.getSummary.mockReset()
    api.listUsers.mockReset()
    api.listProjects.mockReset()
    api.listRequests.mockReset()
    api.getRequestInput.mockReset()
    api.getIndexStatus.mockReset()
    api.triggerIndex.mockReset()
    api.listProjects.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
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
    api.getSummary.mockResolvedValue({
      total_requests: 12,
      matched_requests: 10,
      unmatched_requests: 2,
      total_tokens: 9000,
      total_actual_cost: 1.23,
      cache_read_tokens: 4000,
      cache_creation_tokens: 1000,
      cache_hit_rate: 0.4,
      risky_requests: 2,
      risky_cost: 0.6,
      unmatched_rate: 0.1667,
      risk_request_rate: 0.1667,
      risk_reasons: [{ code: 'huge_input_tiny_output', count: 2 }]
    })
    api.listUsers.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
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
          input_truncated: false
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
  })
})
