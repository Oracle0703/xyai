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
})
