import { beforeEach, describe, expect, it, vi } from 'vitest'

const getGPTQuotaStatus = vi.hoisted(() => vi.fn())
const auth = vi.hoisted(() => ({ isAuthenticated: true }))
vi.mock('@/api/gptQuotaDisplay', () => ({ getGPTQuotaStatus }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))

async function load() {
  const { useGPTQuotaVisibility } = await import('../useGPTQuotaVisibility')
  return useGPTQuotaVisibility()
}

beforeEach(() => {
  vi.resetModules()
  getGPTQuotaStatus.mockReset()
  auth.isAuthenticated = true
})

describe('GPT quota menu visibility', () => {
  it.each([
    [false, false, false],
    [true, false, true],
    [false, true, true],
    [true, true, true],
  ])('quota=%s iq=%s shows the menu: %s', async (quota, iq, visible) => {
    getGPTQuotaStatus.mockResolvedValue({ enabled: quota, iq_test_enabled: iq })
    const visibility = await load()
    expect(visibility.gptQuotaMenuVisible.value).toBe(false)
    expect(await visibility.refreshGPTQuotaVisibility()).toBe(visible)
    expect(visibility.gptQuotaMenuVisible.value).toBe(visible)
  })

  it('stays hidden when the status request fails (opt-in)', async () => {
    getGPTQuotaStatus.mockRejectedValue(new Error('offline'))
    const visibility = await load()
    expect(await visibility.refreshGPTQuotaVisibility()).toBe(false)
    expect(visibility.gptQuotaMenuVisible.value).toBe(false)
  })

  it('treats an older backend without iq_test_enabled as IQ disabled', async () => {
    getGPTQuotaStatus.mockResolvedValue({ enabled: false })
    const visibility = await load()
    expect(await visibility.refreshGPTQuotaVisibility()).toBe(false)
  })

  it('turning quota display off keeps the menu while the IQ test is on', async () => {
    getGPTQuotaStatus.mockResolvedValue({ enabled: true, iq_test_enabled: true })
    const visibility = await load()
    await visibility.refreshGPTQuotaVisibility()
    visibility.setGPTQuotaDisplayVisibility(false)
    expect(visibility.gptQuotaMenuVisible.value).toBe(true)
    visibility.setGPTIQTestVisibility(false)
    expect(visibility.gptQuotaMenuVisible.value).toBe(false)
    visibility.setGPTIQTestVisibility(true)
    expect(visibility.gptQuotaMenuVisible.value).toBe(true)
  })

  it('each page writer only changes its own flag', async () => {
    getGPTQuotaStatus.mockResolvedValue({ enabled: false, iq_test_enabled: false })
    const visibility = await load()
    await visibility.refreshGPTQuotaVisibility()
    visibility.setGPTIQTestVisibility(true)
    visibility.setGPTQuotaDisplayVisibility(false)
    expect(visibility.gptQuotaMenuVisible.value).toBe(true)
  })

  it('caches a loaded status for five minutes and shares the cache between consumers', async () => {
    vi.useFakeTimers()
    try {
      getGPTQuotaStatus.mockResolvedValue({ enabled: false, iq_test_enabled: true })
      const first = await load()
      await first.refreshGPTQuotaVisibility()
      const { useGPTQuotaVisibility } = await import('../useGPTQuotaVisibility')
      expect(await useGPTQuotaVisibility().refreshGPTQuotaVisibility()).toBe(true)
      expect(getGPTQuotaStatus).toHaveBeenCalledTimes(1)
      vi.advanceTimersByTime(5 * 60 * 1000)
      await first.refreshGPTQuotaVisibility()
      expect(getGPTQuotaStatus).toHaveBeenCalledTimes(2)
    } finally {
      vi.useRealTimers()
    }
  })

  it('does not let a late status response overwrite a value written by a page meanwhile', async () => {
    let resolve!: (value: { enabled: boolean; iq_test_enabled: boolean }) => void
    getGPTQuotaStatus.mockReturnValue(new Promise((done) => { resolve = done }))
    const visibility = await load()
    const pending = visibility.refreshGPTQuotaVisibility()
    // 管理员刚保存关闭降智测试；迟到的旧状态（仍为开启）不得把它改回。
    visibility.setGPTIQTestVisibility(false)
    resolve({ enabled: false, iq_test_enabled: true })
    expect(await pending).toBe(false)
    expect(visibility.gptQuotaMenuVisible.value).toBe(false)
  })

  it('does not request or cache while signed out', async () => {
    auth.isAuthenticated = false
    const visibility = await load()
    expect(await visibility.refreshGPTQuotaVisibility()).toBe(false)
    expect(getGPTQuotaStatus).not.toHaveBeenCalled()
  })
})
