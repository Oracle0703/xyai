import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }),
}))

const SAVED_KEY_KEY = 'image-gen-key-v1'

let activeStore: { $dispose: () => void } | null = null

// 端点是模块加载时读取的构建变量：每个用例重置模块后重新导入 pinia 与 store。
async function loadStore() {
  vi.resetModules()
  const { createPinia, setActivePinia } = await import('pinia')
  setActivePinia(createPinia())
  const { useImageGenStore } = await import('@/stores/imageGen')
  const store = useImageGenStore()
  activeStore = store
  return store
}

async function flushMicrotasks() {
  for (let index = 0; index < 10; index++) {
    await Promise.resolve()
  }
}

function abortError() {
  return new DOMException('The operation was aborted.', 'AbortError')
}

describe('imageGen store model discovery', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  afterEach(() => {
    // 释放 store：移除 window storage 监听并取消请求，避免跨用例残留。
    activeStore?.$dispose()
    activeStore = null
    vi.useRealTimers()
    vi.unstubAllEnvs()
    vi.unstubAllGlobals()
  })

  it('disables discovery and saves the key only on explicit confirmation when endpoints differ in origin', async () => {
    vi.stubEnv('VITE_IMAGE_GENERATION_ENDPOINT', 'https://gateway.example.net/v1/images/generations')
    vi.stubEnv('VITE_IMAGE_EDIT_ENDPOINT', 'https://gateway.example.net/v1/images/edits')
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    const store = await loadStore()

    expect(store.discoveryStatus).toBe('disabled')

    store.updateApiKey('sk-external')
    store.triggerModelDiscovery()
    expect(localStorage.getItem(SAVED_KEY_KEY)).toBeNull()
    store.setRememberKey(true)
    expect(JSON.parse(localStorage.getItem(SAVED_KEY_KEY) || '{}').apiKey).toBe('sk-external')

    // 改 Key 不删除旧值；停用模式下回车即确认，覆盖为新 Key。
    store.updateApiKey('sk-external-2')
    expect(JSON.parse(localStorage.getItem(SAVED_KEY_KEY) || '{}').apiKey).toBe('sk-external')
    store.triggerModelDiscovery()
    expect(JSON.parse(localStorage.getItem(SAVED_KEY_KEY) || '{}').apiKey).toBe('sk-external-2')

    // 默认同源的 /v1/models 不能收到外部 Key。
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('reports a timeout when the catalog request hangs and lets the same key retry', async () => {
    const fetchMock = vi.fn((_url: string, init: RequestInit) => new Promise((_resolve, reject) => {
      init.signal?.addEventListener('abort', () => reject(abortError()))
    }))
    vi.stubGlobal('fetch', fetchMock)
    const store = await loadStore()
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })

    store.updateApiKey('sk-hang')
    store.triggerModelDiscovery()
    await vi.advanceTimersByTimeAsync(10_000)
    await flushMicrotasks()

    expect(store.discoveryStatus).toBe('error')
    expect(store.discoveryError).toContain('超时')

    // 超时释放在途标记，同一 Key 可显式重试。
    store.triggerModelDiscovery()
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(store.discoveryStatus).toBe('loading')
  })

  it('reports a timeout rather than a format error when the body stalls', async () => {
    vi.stubGlobal('fetch', vi.fn(async (_url: string, init: RequestInit) => ({
      ok: true,
      status: 200,
      headers: new Headers(),
      json: () => new Promise((_resolve, reject) => {
        init.signal?.addEventListener('abort', () => reject(abortError()))
      }),
    })))
    const store = await loadStore()
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })

    store.updateApiKey('sk-stall')
    store.triggerModelDiscovery()
    await vi.advanceTimersByTimeAsync(10_000)
    await flushMicrotasks()

    expect(store.discoveryError).toContain('超时')
    expect(store.discoveryError).not.toContain('格式异常')
  })
})
