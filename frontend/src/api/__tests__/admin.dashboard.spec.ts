import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getUserUsageTrend } from '../admin/dashboard'

const get = vi.hoisted(() => vi.fn())

vi.mock('../client', () => ({
  apiClient: { get }
}))

describe('admin dashboard user trend API', () => {
  beforeEach(() => {
    get.mockReset()
    get.mockResolvedValue({ data: { trend: [] } })
  })

  it('serializes selected user IDs as a stable comma-separated query value', async () => {
    await getUserUsageTrend({
      user_ids: [8, 7],
      start_date: '2026-07-01',
      end_date: '2026-07-01',
      granularity: 'hour'
    })

    expect(get).toHaveBeenCalledWith('/admin/dashboard/users-trend', {
      params: {
        user_ids: '8,7',
        start_date: '2026-07-01',
        end_date: '2026-07-01',
        granularity: 'hour'
      }
    })
  })

  it('omits user_ids when no selected users are supplied', async () => {
    await getUserUsageTrend({
      user_ids: [],
      start_date: '2026-07-01',
      end_date: '2026-07-02',
      granularity: 'day',
      limit: 12
    })

    expect(get).toHaveBeenCalledWith('/admin/dashboard/users-trend', {
      params: {
        start_date: '2026-07-01',
        end_date: '2026-07-02',
        granularity: 'day',
        limit: 12
      }
    })
  })
})
