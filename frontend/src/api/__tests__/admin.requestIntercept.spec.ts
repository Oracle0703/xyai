import { beforeEach, describe, expect, it, vi } from 'vitest'
import requestInterceptAPI from '../admin/requestIntercept'

const { get, put } = vi.hoisted(() => ({
  get: vi.fn(),
  put: vi.fn()
}))

vi.mock('../client', () => ({
  apiClient: { get, put }
}))

describe('admin requestIntercept API', () => {
  beforeEach(() => {
    get.mockReset()
    put.mockReset()
  })

  it('loads global config', async () => {
    get.mockResolvedValue({ data: { enabled: true } })

    const result = await requestInterceptAPI.getConfig()

    expect(get).toHaveBeenCalledWith('/admin/request-intercept/config')
    expect(result.enabled).toBe(true)
  })

  it('updates global config', async () => {
    put.mockResolvedValue({ data: { enabled: false } })

    const result = await requestInterceptAPI.updateConfig({ enabled: false })

    expect(put).toHaveBeenCalledWith('/admin/request-intercept/config', { enabled: false })
    expect(result.enabled).toBe(false)
  })
})
