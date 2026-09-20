import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post }
}))

import { list, resetDailyFiltered } from '@/api/admin/subscriptions'

describe('admin subscriptions API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  })

  it('passes organization and secondary sort fields to the list endpoint', async () => {
    get.mockResolvedValue({
      data: { items: [], total: 0, page: 2, page_size: 20, pages: 0 }
    })

    await list(2, 20, {
      organization: 'xunyou',
      sort_by: 'expires_at',
      sort_order: 'asc'
    })

    expect(get).toHaveBeenCalledWith('/admin/subscriptions', {
      params: {
        page: 2,
        page_size: 20,
        organization: 'xunyou',
        sort_by: 'expires_at',
        sort_order: 'asc'
      },
      signal: undefined
    })
  })

  it('sends only reset filters with the supplied idempotency key', async () => {
    post.mockResolvedValue({ data: { reset_count: 4 } })

    const result = await resetDailyFiltered(
      {
        status: 'active',
        user_id: 11,
        group_id: 21,
        platform: 'openai',
        organization: 'xunyou',
        sort_by: 'expires_at',
        sort_order: 'asc'
      },
      'subscription-reset-11111111-1111-4111-8111-111111111111'
    )

    expect(post).toHaveBeenCalledWith(
      '/admin/subscriptions/reset-daily-filtered',
      {
        status: 'active',
        user_id: 11,
        group_id: 21,
        platform: 'openai',
        organization: 'xunyou'
      },
      {
        headers: {
          'Idempotency-Key': 'subscription-reset-11111111-1111-4111-8111-111111111111'
        }
      }
    )
    expect(result).toEqual({ reset_count: 4 })
  })
  it('preserves the department and successful query scope version for a filtered reset', async () => {
    post.mockResolvedValue({ data: { reset_count: 1 } })
    await resetDailyFiltered({ organization: 'xunyou', department_id: '7', scope_version: 'scope-v1', sort_by: 'status' }, 'scoped-reset')
    expect(post).toHaveBeenCalledWith('/admin/subscriptions/reset-daily-filtered', expect.objectContaining({ organization: 'xunyou', department_id: '7', scope_version: 'scope-v1' }), { headers: { 'Idempotency-Key': 'scoped-reset' } })
    expect(post.mock.calls[0][1]).not.toHaveProperty('sort_by')
  })

})
