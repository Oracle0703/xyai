import { beforeEach, describe, expect, it, vi } from 'vitest'
import tokenAnalysisAPI from '../admin/tokenAnalysis'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn()
}))

vi.mock('../client', () => ({
  apiClient: { get, post }
}))

describe('admin tokenAnalysis API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  })

  it('loads request list with filters', async () => {
    get.mockResolvedValue({ data: { items: [], total: 0, page: 1, page_size: 20 } })

    await tokenAnalysisAPI.listRequests({
      start_date: '2026-05-19',
      end_date: '2026-05-19',
      risk_min: 30,
      include_unmatched: true
    })

    expect(get).toHaveBeenCalledWith('/admin/token-analysis/requests', {
      params: {
        start_date: '2026-05-19',
        end_date: '2026-05-19',
        risk_min: 30,
        include_unmatched: true
      },
      signal: undefined
    })
  })

  it('triggers index with date range', async () => {
    post.mockResolvedValue({ data: { indexed_rows: 1, failed_rows: 0 } })

    const result = await tokenAnalysisAPI.triggerIndex({ start_date: '2026-05-19', end_date: '2026-05-19' })

    expect(post).toHaveBeenCalledWith('/admin/token-analysis/index', {
      start_date: '2026-05-19',
      end_date: '2026-05-19'
    })
    expect(result.indexed_rows).toBe(1)
  })

  it('loads summary risk distribution', async () => {
    get.mockResolvedValue({
      data: {
        total_requests: 10,
        matched_requests: 7,
        unmatched_requests: 3,
        total_tokens: 1000,
        total_actual_cost: 1.2,
        cache_read_tokens: 200,
        cache_creation_tokens: 100,
        cache_hit_rate: 0.2,
        risky_requests: 4,
        risky_cost: 0.6,
        unmatched_rate: 0.3,
        risk_request_rate: 0.4,
        risk_reasons: [{ code: 'huge_input_tiny_output', count: 3 }]
      }
    })

    const result = await tokenAnalysisAPI.getSummary({ risk_min: 20 })

    expect(get).toHaveBeenCalledWith('/admin/token-analysis/summary', { params: { risk_min: 20 } })
    expect(result.unmatched_requests).toBe(3)
    expect(result.risk_reasons?.[0]?.code).toBe('huge_input_tiny_output')
  })
})
