import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import TokenAnalysisView from '../TokenAnalysisView.vue'

const api = vi.hoisted(() => ({
  getSummary: vi.fn(),
  listUsers: vi.fn(),
  listRequests: vi.fn(),
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
    api.listRequests.mockReset()
    api.getIndexStatus.mockReset()
    api.triggerIndex.mockReset()
    api.getSummary.mockResolvedValue({
      total_requests: 12,
      total_tokens: 9000,
      total_actual_cost: 1.23,
      cache_read_tokens: 4000,
      cache_creation_tokens: 1000,
      cache_hit_rate: 0.4,
      risky_requests: 2,
      risky_cost: 0.6
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
          match_confidence: 3
        }
      ],
      total: 1,
      page: 1,
      page_size: 20
    })
    api.getIndexStatus.mockResolvedValue({ running: false, processed_rows: 10, failed_rows: 0, files: [] })
  })

  it('renders summary and suspicious request rows', async () => {
    const wrapper = mount(TokenAnalysisView, {
      global: { stubs: { AppLayout: AppLayoutStub } }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('9,000')
    expect(wrapper.text()).toContain('user@example.com')
    expect(wrapper.text()).toContain('gpt-4.1')
    expect(wrapper.text()).toContain('hello')
  })
})
