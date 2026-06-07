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

  it('loads project usage with project filter', async () => {
    get.mockResolvedValue({
      data: {
        items: [
          {
            project: 'lag-killer',
            user_id: 7,
            user_email: 'dev@example.com',
            request_count: 12,
            matched_request_count: 11,
            total_tokens: 5000,
            input_tokens: 3000,
            output_tokens: 1000,
            cache_read_tokens: 800,
            cache_creation_tokens: 200,
            actual_cost: 0.5
          }
        ],
        total: 1,
        page: 1,
        page_size: 20
      }
    })

    const result = await tokenAnalysisAPI.listProjects({
      start_date: '2026-06-05',
      end_date: '2026-06-05',
      project: 'lag-killer'
    })

    expect(get).toHaveBeenCalledWith('/admin/token-analysis/projects', {
      params: {
        start_date: '2026-06-05',
        end_date: '2026-06-05',
        project: 'lag-killer'
      },
      signal: undefined
    })
    expect(result.items[0]?.project).toBe('lag-killer')
    expect(result.items[0]?.total_tokens).toBe(5000)
  })

  it('loads full user input by archive id', async () => {
    get.mockResolvedValue({
      data: {
        id: 1,
        archive_id: 'arch-1',
        event_time: '2026-06-07T01:00:00Z',
        content: 'line one\nline two',
        content_sha256: 'abc',
        chars: 17,
        truncated: false,
        quality_version: ''
      }
    })

    const result = await tokenAnalysisAPI.getRequestInput('arch-1')

    expect(get).toHaveBeenCalledWith('/admin/token-analysis/requests/input', {
      params: { archive_id: 'arch-1' }
    })
    expect(result.content).toContain('line two')
    expect(result.truncated).toBe(false)
  })

  it('lists archive files with deletable status', async () => {
    get.mockResolvedValue({
      data: [
        {
          name: '2026-05-20.jsonl',
          size_bytes: 2048,
          mod_time: '2026-05-20T23:59:00Z',
          indexed_offset: 2048,
          processed_rows: 20,
          failed_rows: 0,
          last_error: '',
          status: 'deletable'
        }
      ]
    })

    const files = await tokenAnalysisAPI.listArchiveFiles()

    expect(get).toHaveBeenCalledWith('/admin/token-analysis/archive-files')
    expect(files[0]?.status).toBe('deletable')
    expect(files[0]?.indexed_offset).toBe(2048)
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
