import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { OrganizationUsageReportData } from '@/api/admin/organizationUsage'
import { processOrganizationUsageExport } from '../organizationUsageExport.worker'

const { write } = vi.hoisted(() => ({ write: vi.fn(() => new ArrayBuffer(8)) }))

vi.mock('xlsx', async () => {
  const actual = await vi.importActual<typeof import('xlsx')>('xlsx')
  return { ...actual, write }
})

const metrics = {
  requests: 0,
  input_tokens: 0,
  output_tokens: 0,
  cache_creation_tokens: 0,
  cache_read_tokens: 0,
  total_tokens: 0,
  actual_cost: 0
}

const reportData: OrganizationUsageReportData = {
  summary: {
    range: { start_date: '2026-07-01', end_date: '2026-07-31' },
    overview: { ...metrics, active_users: 0, used_users: 0 },
    organizations: [],
    champions: { day: null, week: null, month: null },
    items: [],
    pagination: { total: 0, page: 1, page_size: 500, pages: 1 }
  },
  periods: { day: [], week: [], month: [] }
}

describe('organization usage export worker entry', () => {
  beforeEach(() => {
    write.mockReset().mockReturnValue(new ArrayBuffer(8))
  })

  it('posts stages and transfers the serialized ArrayBuffer', async () => {
    const postMessage = vi.fn()

    await processOrganizationUsageExport(reportData, postMessage)

    expect(postMessage.mock.calls[0]).toEqual([{ type: 'stage', stage: 'building' }])
    expect(postMessage.mock.calls[1]).toEqual([{ type: 'stage', stage: 'serializing' }])
    const success = postMessage.mock.calls[2][0]
    expect(success).toMatchObject({ type: 'success', buffer: expect.any(ArrayBuffer) })
    expect(postMessage.mock.calls[2][1]).toEqual([success.buffer])
  })

  it('posts a serializable error instead of throwing out of the worker handler', async () => {
    write.mockImplementationOnce(() => { throw new Error('xlsx failed') })
    const postMessage = vi.fn()

    await processOrganizationUsageExport(reportData, postMessage)

    expect(postMessage).toHaveBeenLastCalledWith({ type: 'error', message: 'xlsx failed' })
  })
})
