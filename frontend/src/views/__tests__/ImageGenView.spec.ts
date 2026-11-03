import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { createPinia, getActivePinia, setActivePinia } from 'pinia'

import ImageGenView from '../ImageGenView.vue'
import { useImageGenStore } from '@/stores/imageGen'

const { fetchPublicSettings, authState, showSuccess, showError } = vi.hoisted(() => ({
  fetchPublicSettings: vi.fn(),
  authState: { isAuthenticated: false, isAdmin: false },
  showSuccess: vi.fn(),
  showError: vi.fn(),
}))

// 只 mock app/auth 两个文件模块（视图与 imageGen store 都从文件模块导入，
// 解析到同一 module ID），imageGen store 跑真实实现以覆盖跨卸载的状态保持。
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: null,
    siteName: 'Sub2API',
    siteLogo: '',
    docUrl: '',
    publicSettingsLoaded: true,
    backendModeEnabled: false,
    fetchPublicSettings,
    showSuccess,
    showError,
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState,
}))

// 用轻量桩替换平台布局，避免拉起侧边栏/引导等重依赖；仅验证是否被包裹。
vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: {
    name: 'AppLayout',
    template: '<div data-test="app-layout"><slot /></div>',
  },
}))

function mountView() {
  return mount(ImageGenView, {
    global: {
      stubs: {
        RouterLink: { template: '<a><slot /></a>' },
        Icon: true,
      },
    },
  })
}

function makeImageFile(name: string, sizeBytes?: number): File {
  const file = new File(['source image'], name, { type: 'image/png' })
  if (sizeBytes !== undefined) {
    Object.defineProperty(file, 'size', { configurable: true, value: sizeBytes })
  }
  return file
}

const MODEL_PREFERENCE_KEY = 'image-gen-model-v1'
const SAVED_KEY_KEY = 'image-gen-key-v1'
const HISTORY_KEY = 'image-gen-history-v1'

function seedModelPreference(model: string, modelSource: 'list' | 'manual' = 'list') {
  localStorage.setItem(MODEL_PREFERENCE_KEY, JSON.stringify({ version: 1, model, modelSource }))
}

function seedSavedKey(apiKey: string) {
  localStorage.setItem(SAVED_KEY_KEY, JSON.stringify({ version: 1, apiKey }))
}

function jsonResponse(body: unknown, status = 200, headers: Record<string, string> = {}) {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: new Headers(headers),
    json: async () => body,
  }
}

function imagesResponse() {
  return jsonResponse({ data: [{ b64_json: 'aGVsbG8=', revised_prompt: '' }] })
}

function catalog(...ids: string[]) {
  return jsonResponse({ object: 'list', data: ids.map((id) => ({ id, object: 'model' })) })
}

type FetchHandler = (init?: RequestInit) => unknown

// 按 URL 分流：模型目录交给 modelsHandler，生图请求默认返回一张图。
function routeFetch(modelsHandler: FetchHandler, imagesHandler: FetchHandler = imagesResponse) {
  const fetchMock = vi.fn((url: string, init?: RequestInit) =>
    Promise.resolve(url === '/v1/models' ? modelsHandler(init) : imagesHandler(init))
  )
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function modelCalls(fetchMock: ReturnType<typeof routeFetch>) {
  return fetchMock.mock.calls.filter(([url]) => url === '/v1/models')
}

function imageCalls(fetchMock: ReturnType<typeof routeFetch>) {
  return fetchMock.mock.calls.filter(([url]) => url !== '/v1/models')
}

function authHeader(init?: RequestInit): string {
  return (init?.headers as Record<string, string> | undefined)?.Authorization ?? ''
}

function optionValues(wrapper: ReturnType<typeof mountView>): string[] {
  return wrapper.findAll('[data-test="model-select"] option').map((option) => option.attributes('value') ?? '')
}

async function enterKey(wrapper: ReturnType<typeof mountView>, value: string) {
  await wrapper.get('[data-test="api-key-input"]').setValue(value)
}

async function blurKey(wrapper: ReturnType<typeof mountView>) {
  await wrapper.get('[data-test="api-key-input"]').trigger('blur')
  await flushPromises()
}

// 显式加载：点击“加载模型”按钮（回车路径在专门用例中断言）。
async function loadModels(wrapper: ReturnType<typeof mountView>) {
  await wrapper.get('[data-test="model-refresh-button"]').trigger('click')
  await flushPromises()
}

function savedKey(): string | null {
  const raw = localStorage.getItem(SAVED_KEY_KEY)
  return raw === null ? null : JSON.parse(raw).apiKey
}

function rememberChecked(wrapper: ReturnType<typeof mountView>): boolean {
  return (wrapper.get('[data-test="remember-key-checkbox"]').element as HTMLInputElement).checked
}

// 释放 store（移除 window storage 监听、取消请求），避免旧 store 跨用例或跨“刷新”存活。
function disposeImageGenStore() {
  const pinia = getActivePinia()
  if (pinia && 'imageGen' in pinia.state.value) {
    useImageGenStore(pinia).$dispose()
  }
}

// 模拟刷新页面：旧 store 释放后换新 pinia，新 store 重新读取已保存配置。
function reloadPage() {
  disposeImageGenStore()
  setActivePinia(createPinia())
}

// 直接派发可取消的事件以断言 defaultPrevented：jsdom 不会因 keydown 隐式提交表单，
// 只断言“没有生图请求”无法证明回车被拦截。
async function pressEnter(wrapper: ReturnType<typeof mountView>, selector: string): Promise<KeyboardEvent> {
  const event = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })
  wrapper.get(selector).element.dispatchEvent(event)
  await flushPromises()
  return event
}

function failSavedKeyWrites() {
  const originalSetItem = Storage.prototype.setItem
  return vi.spyOn(Storage.prototype, 'setItem').mockImplementation(function (
    this: Storage,
    key: string,
    value: string
  ) {
    if (key === SAVED_KEY_KEY) {
      throw new DOMException('quota exceeded', 'QuotaExceededError')
    }
    return originalSetItem.call(this, key, value)
  })
}

function failSavedKeyRemovals() {
  const originalRemoveItem = Storage.prototype.removeItem
  return vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(function (this: Storage, key: string) {
    if (key === SAVED_KEY_KEY) {
      throw new DOMException('storage denied', 'SecurityError')
    }
    return originalRemoveItem.call(this, key)
  })
}

async function selectSourceImages(wrapper: ReturnType<typeof mountView>, files: File[]) {
  const input = wrapper.get('[data-test="source-image-input"]').element as HTMLInputElement
  Object.defineProperty(input, 'files', {
    configurable: true,
    value: files,
  })
  await wrapper.get('[data-test="source-image-input"]').trigger('change')
}

describe('ImageGenView', () => {
  beforeEach(() => {
    // 每个用例一个全新 pinia：imageGen store 是真实单例，必须重建避免跨用例漏状态。
    setActivePinia(createPinia())
    fetchPublicSettings.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    authState.isAuthenticated = false
    authState.isAdmin = false
    localStorage.clear()
    // 页面不再写死模型：既有用例以恢复的模型偏好作为（未确认的）当前模型。
    seedModelPreference('gpt-image-2')

    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: vi.fn().mockReturnValue({ matches: false }),
    })

    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        created: 1779552000,
        data: [
          {
            b64_json: 'aGVsbG8=',
            revised_prompt: 'A clean product poster',
          },
        ],
      }),
    }))

    URL.createObjectURL = vi.fn(() => 'blob:image-preview')
    URL.revokeObjectURL = vi.fn()
  })

  afterEach(() => {
    disposeImageGenStore()
    vi.unstubAllGlobals()
  })

  it('calls the images gateway with the entered API key and prompt', async () => {
    const wrapper = mountView()

    await wrapper.get('[data-test="api-key-input"]').setValue('sk-valid-key')
    await wrapper.get('[data-test="prompt-input"]').setValue('给公众号生成一张配图')
    await wrapper.get('[data-test="size-select"]').setValue('1536x1024')
    await wrapper.get('[data-test="count-input"]').setValue(1)
    await wrapper.get('[data-test="generate-button"]').trigger('click')
    await flushPromises()
    await nextTick()

    const fetchMock = vi.mocked(fetch)
    expect(fetchMock).toHaveBeenCalledWith(
      '/v1/images/generations',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({
          Authorization: 'Bearer sk-valid-key',
          'Content-Type': 'application/json',
        }),
      })
    )
    expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body))).toEqual({
      model: 'gpt-image-2',
      prompt: '给公众号生成一张配图',
      size: '1536x1024',
      n: 1,
      response_format: 'b64_json',
    })
    expect(wrapper.find('[data-test="result-image"]').attributes('src')).toBe('data:image/png;base64,aGVsbG8=')
    expect(wrapper.text()).toContain('A clean product poster')

    wrapper.unmount()
  })

  it('stores generated images in local history without storing the API key', async () => {
    const wrapper = mountView()

    await wrapper.get('[data-test="api-key-input"]').setValue('sk-history-key')
    await wrapper.get('[data-test="prompt-input"]').setValue('历史记录测试')
    await wrapper.get('[data-test="generate-button"]').trigger('click')
    await flushPromises()
    await nextTick()

    const raw = localStorage.getItem('image-gen-history-v1')
    expect(raw).toBeTruthy()

    const history = JSON.parse(raw || '[]')
    expect(history).toHaveLength(1)
    expect(history[0]).toMatchObject({
      prompt: '历史记录测试',
      model: 'gpt-image-2',
      size: '1024x1024',
      images: ['data:image/png;base64,aGVsbG8='],
    })
    expect(JSON.stringify(history)).not.toContain('sk-history-key')
    expect(localStorage.getItem(SAVED_KEY_KEY)).toBeNull()
    expect(wrapper.findAll('[data-test="history-item"]')).toHaveLength(1)

    wrapper.unmount()
  })

  it('calls the image edits gateway with an uploaded source image', async () => {
    const wrapper = mountView()
    const sourceImage = makeImageFile('source.png')

    await wrapper.get('[data-test="mode-edit-button"]').trigger('click')
    await wrapper.get('[data-test="api-key-input"]').setValue('sk-edit-key')
    await wrapper.get('[data-test="prompt-input"]').setValue('把背景改成白色')
    await selectSourceImages(wrapper, [sourceImage])
    await wrapper.get('[data-test="generate-button"]').trigger('click')
    await flushPromises()
    await nextTick()

    const fetchMock = vi.mocked(fetch)
    expect(fetchMock).toHaveBeenCalledWith(
      '/v1/images/edits',
      expect.objectContaining({
        method: 'POST',
        headers: {
          Authorization: 'Bearer sk-edit-key',
        },
      })
    )
    const body = fetchMock.mock.calls[0][1]?.body
    expect(body).toBeInstanceOf(FormData)
    const formData = body as FormData
    expect(formData.get('model')).toBe('gpt-image-2')
    expect(formData.get('prompt')).toBe('把背景改成白色')
    expect(formData.get('size')).toBe('1024x1024')
    expect(formData.get('n')).toBe('1')
    expect(formData.get('response_format')).toBe('b64_json')
    expect(formData.getAll('image[]')).toEqual([sourceImage])

    wrapper.unmount()
  })

  it('uses a pasted image as the image edit source', async () => {
    const wrapper = mountView()
    const pastedImage = makeImageFile('paste.png')

    await wrapper.get('[data-test="mode-edit-button"]').trigger('click')
    await wrapper.get('[data-test="api-key-input"]').setValue('sk-paste-key')
    await wrapper.get('[data-test="prompt-input"]').setValue('把人物换成卡通风格')

    const pasteEvent = new Event('paste') as ClipboardEvent
    Object.defineProperty(pasteEvent, 'clipboardData', {
      value: {
        files: [pastedImage],
        items: [
          {
            kind: 'file',
            type: 'image/png',
            getAsFile: () => pastedImage,
          },
        ],
      },
    })
    window.dispatchEvent(pasteEvent)
    await nextTick()

    expect(wrapper.text()).toContain('paste.png')

    await wrapper.get('[data-test="generate-button"]').trigger('click')
    await flushPromises()
    await nextTick()

    const fetchMock = vi.mocked(fetch)
    const body = fetchMock.mock.calls[0][1]?.body as FormData
    expect(fetchMock.mock.calls[0][0]).toBe('/v1/images/edits')
    expect(body).toBeInstanceOf(FormData)
    expect(body.getAll('image[]')).toEqual([pastedImage])

    wrapper.unmount()
  })

  it('sends multiple source images as image[] entries and supports per-item removal', async () => {
    const wrapper = mountView()
    const first = makeImageFile('first.png')
    const second = makeImageFile('second.png')

    await wrapper.get('[data-test="mode-edit-button"]').trigger('click')
    await wrapper.get('[data-test="api-key-input"]').setValue('sk-multi-key')
    await wrapper.get('[data-test="prompt-input"]').setValue('把两张图拼成一张海报')
    await selectSourceImages(wrapper, [first, second])

    expect(wrapper.findAll('[data-test="source-image-item"]')).toHaveLength(2)

    await wrapper.get('[data-test="generate-button"]').trigger('click')
    await flushPromises()
    await nextTick()

    const fetchMock = vi.mocked(fetch)
    const body = fetchMock.mock.calls[0][1]?.body as FormData
    expect(body.getAll('image[]')).toEqual([first, second])

    await wrapper.findAll('[data-test="source-image-remove"]')[0].trigger('click')
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:image-preview')
    expect(wrapper.findAll('[data-test="source-image-item"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('second.png')

    wrapper.unmount()
  })

  it('rejects oversized files and enforces the total size cap', async () => {
    const wrapper = mountView()

    await wrapper.get('[data-test="mode-edit-button"]').trigger('click')

    // 单张超过 20MB：直接拒绝（网关会在 20MB 处静默截断分片，不能放行）。
    await selectSourceImages(wrapper, [makeImageFile('huge.png', 25 * 1024 * 1024)])
    expect(wrapper.findAll('[data-test="source-image-item"]')).toHaveLength(0)
    expect(wrapper.get('[data-test="error-message"]').text()).toContain('20MB')

    // 六张 18MB：前五张共 90MB 通过，第六张触发 100MB 总量护栏。
    const batch = Array.from({ length: 6 }, (_, index) =>
      makeImageFile(`batch-${index}.png`, 18 * 1024 * 1024)
    )
    await selectSourceImages(wrapper, batch)
    expect(wrapper.findAll('[data-test="source-image-item"]')).toHaveLength(5)
    expect(wrapper.get('[data-test="error-message"]').text()).toContain('100MB')

    wrapper.unmount()
  })

  it('clamps the requested image count to 10', async () => {
    const wrapper = mountView()

    await wrapper.get('[data-test="api-key-input"]').setValue('sk-count-key')
    await wrapper.get('[data-test="prompt-input"]').setValue('数量上限测试')
    await wrapper.get('[data-test="count-input"]').setValue(12)
    await wrapper.get('[data-test="generate-button"]').trigger('click')
    await flushPromises()

    const fetchMock = vi.mocked(fetch)
    expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body)).n).toBe(10)

    wrapper.unmount()
  })

  it('keeps form state, results and history across unmount/remount', async () => {
    const first = mountView()

    await first.get('[data-test="api-key-input"]').setValue('sk-persist-key')
    await first.get('[data-test="prompt-input"]').setValue('切页保留测试')
    await first.get('[data-test="generate-button"]').trigger('click')
    await flushPromises()
    first.unmount()

    const second = mountView()
    await nextTick()

    expect((second.get('[data-test="api-key-input"]').element as HTMLInputElement).value).toBe('sk-persist-key')
    expect((second.get('[data-test="prompt-input"]').element as HTMLTextAreaElement).value).toBe('切页保留测试')
    expect(second.find('[data-test="result-image"]').attributes('src')).toBe('data:image/png;base64,aGVsbG8=')
    expect(second.findAll('[data-test="history-item"]')).toHaveLength(1)

    second.unmount()
  })

  it('finishes an in-flight generation after unmount and shows the result on remount', async () => {
    let resolveFetch!: (value: { ok: boolean; json: () => Promise<unknown> }) => void
    vi.stubGlobal('fetch', vi.fn(() => new Promise((resolve) => {
      resolveFetch = resolve
    })))

    const first = mountView()
    await first.get('[data-test="api-key-input"]').setValue('sk-background-key')
    await first.get('[data-test="prompt-input"]').setValue('后台完成测试')
    await first.get('[data-test="generate-button"]').trigger('click')
    await nextTick()

    expect(first.get('[data-test="generate-button"]').attributes('disabled')).toBeDefined()
    first.unmount()

    // 组件已卸载，生成在 store 里继续跑完。
    resolveFetch({
      ok: true,
      json: async () => ({ data: [{ b64_json: 'aGVsbG8=', revised_prompt: '' }] }),
    })
    await flushPromises()

    expect(showSuccess).toHaveBeenCalledTimes(1)
    const history = JSON.parse(localStorage.getItem('image-gen-history-v1') || '[]')
    expect(history).toHaveLength(1)
    expect(history[0].prompt).toBe('后台完成测试')

    const second = mountView()
    await nextTick()

    expect(second.find('[data-test="result-image"]').attributes('src')).toBe('data:image/png;base64,aGVsbG8=')
    expect(second.get('[data-test="generate-button"]').attributes('disabled')).toBeUndefined()
    expect(second.findAll('[data-test="history-item"]')).toHaveLength(1)

    second.unmount()
  })

  it('renders standalone (no app layout) for unauthenticated visitors', () => {
    const wrapper = mountView()

    expect(wrapper.find('[data-test="app-layout"]').exists()).toBe(false)
    // 独立页保留自带的头部标题
    expect(wrapper.text()).toContain('图片生成工具')
    expect(wrapper.find('[data-test="api-key-input"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('embeds in the app layout for authenticated users', () => {
    authState.isAuthenticated = true

    const wrapper = mountView()

    // 嵌入平台布局（侧边栏 + 右侧内容区），不再渲染独立整页头部
    expect(wrapper.find('[data-test="app-layout"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('图片生成工具')
    // 生成器本体仍然渲染在内容区
    expect(wrapper.find('[data-test="api-key-input"]').exists()).toBe(true)

    wrapper.unmount()
  })

  describe('model discovery', () => {
    it('loads image models on Enter with only the API key and confirms the remembered model', async () => {
      const fetchMock = routeFetch(() =>
        catalog('gpt-5.4', 'gpt-image-2.5-flare', 'gpt-image-2', 'grok-imagine-video', 'gpt-image-1')
      )
      const wrapper = mountView()

      // 输入、粘贴、失焦都不发请求：只有回车或“加载模型”按钮会把 Key 发出。
      await enterKey(wrapper, 'sk-discover')
      await wrapper.get('[data-test="api-key-input"]').trigger('paste')
      await new Promise((resolve) => setTimeout(resolve, 0))
      await blurKey(wrapper)
      expect(modelCalls(fetchMock)).toHaveLength(0)
      expect(wrapper.get('[data-test="model-refresh-button"]').text()).toBe('加载模型')

      const enter = await pressEnter(wrapper, '[data-test="api-key-input"]')
      expect(enter.defaultPrevented).toBe(true)
      expect(imageCalls(fetchMock)).toHaveLength(0)
      expect(modelCalls(fetchMock)).toHaveLength(1)
      const [url, init] = modelCalls(fetchMock)[0]
      expect(url).toBe('/v1/models')
      expect(init).toMatchObject({ method: 'GET', cache: 'no-store', redirect: 'error' })
      // 精确比较：不得混入登录 JWT 等其他请求头。
      expect(init?.headers).toEqual({ Authorization: 'Bearer sk-discover', Accept: 'application/json' })
      expect(optionValues(wrapper)).toEqual(['', 'gpt-image-1', 'gpt-image-2', 'gpt-image-2.5-flare'])
      expect(wrapper.get('[data-test="model-badge"]').text()).toBe('gpt-image-2 · 已确认')
      expect(wrapper.get('[data-test="model-refresh-button"]').text()).toBe('刷新模型')

      // 回车总是显式刷新。
      await pressEnter(wrapper, '[data-test="api-key-input"]')
      expect(modelCalls(fetchMock)).toHaveLength(2)

      // 按住回车的连发不重复请求。
      const repeated = new KeyboardEvent('keydown', { key: 'Enter', repeat: true, bubbles: true, cancelable: true })
      wrapper.get('[data-test="api-key-input"]').element.dispatchEvent(repeated)
      await flushPromises()
      expect(modelCalls(fetchMock)).toHaveLength(2)

      wrapper.unmount()
    })

    it('keeps a stale catalog when a refresh fails and clears it on 401 for the same key', async () => {
      let calls = 0
      routeFetch(() => {
        calls++
        if (calls === 1) {
          return catalog('gpt-image-2', 'gpt-image-1')
        }
        if (calls === 2) {
          return jsonResponse({}, 502)
        }
        return jsonResponse({ code: 'API_KEY_DISABLED', message: 'API key is disabled' }, 401)
      })
      const wrapper = mountView()

      await enterKey(wrapper, 'sk-stale')
      await loadModels(wrapper)
      await wrapper.get('[data-test="model-refresh-button"]').trigger('click')
      await flushPromises()

      expect(optionValues(wrapper)).toEqual(['', 'gpt-image-1', 'gpt-image-2'])
      expect(wrapper.get('[data-test="model-status"]').text()).toContain('（当前为旧目录）')
      expect(wrapper.get('[data-test="model-badge"]').text()).toBe('gpt-image-2 · 未确认')
      // 从旧目录选择也不算已确认。
      await wrapper.get('[data-test="model-select"]').setValue('gpt-image-1')
      expect(wrapper.get('[data-test="model-badge"]').text()).toBe('gpt-image-1 · 未确认')

      await wrapper.get('[data-test="model-refresh-button"]').trigger('click')
      await flushPromises()
      expect(wrapper.get('[data-test="model-status"]').text()).toContain('API key is disabled')
      // 401 清空候选，当前选择保留为未确认项。
      expect(optionValues(wrapper)).toEqual(['', 'gpt-image-1'])
      expect(wrapper.findAll('[data-test="model-select"] option')[1].text()).toContain('未确认')

      wrapper.unmount()
    })

    it('reports HTML and missing-endpoint responses distinctly', async () => {
      routeFetch((init) => authHeader(init) === 'Bearer sk-html'
        ? {
          ok: true,
          status: 200,
          headers: new Headers(),
          json: async () => {
            throw new SyntaxError('Unexpected token <')
          },
        }
        : jsonResponse({ message: 'Not Found' }, 404))
      const wrapper = mountView()

      await enterKey(wrapper, 'sk-html')
      await loadModels(wrapper)
      expect(wrapper.get('[data-test="model-status"]').text()).toContain('响应格式异常')

      await enterKey(wrapper, 'sk-404')
      await loadModels(wrapper)
      expect(wrapper.get('[data-test="model-status"]').text()).toContain('请检查模型端点配置')

      wrapper.unmount()
    })

    // 只能证明这些 DOM 事件的处理；Chrome 是否自动填充、填充时触发哪些事件需浏览器验收。
    it('treats input, change, paste and blur as a draft: nothing is sent and the saved key is kept', async () => {
      seedSavedKey('sk-saved')
      const fetchMock = routeFetch(() => catalog('gpt-image-2'))
      const wrapper = mountView()
      await flushPromises()
      // 显式记住过的 Key 按约定在打开页面时恢复并加载。
      expect(modelCalls(fetchMock)).toHaveLength(1)

      const input = wrapper.get('[data-test="api-key-input"]')
      await input.setValue('login-password')
      await input.trigger('change')
      await input.trigger('paste')
      await new Promise((resolve) => setTimeout(resolve, 0))
      await blurKey(wrapper)

      expect(modelCalls(fetchMock)).toHaveLength(1)
      expect(fetchMock.mock.calls.some(([, init]) => authHeader(init) === 'Bearer login-password')).toBe(false)
      expect(savedKey()).toBe('sk-saved')
      expect(rememberChecked(wrapper)).toBe(true)
      expect(wrapper.get('[data-test="key-save-status"]').text()).toBe('浏览器中保存的是之前的 Key，当前 Key 加载模型成功后替换')
      wrapper.unmount()

      // 刷新后仍恢复已保存的 Key，而不是草稿。
      reloadPage()
      const second = mountView()
      await flushPromises()
      expect((second.get('[data-test="api-key-input"]').element as HTMLInputElement).value).toBe('sk-saved')
      expect(second.get('[data-test="key-save-status"]').text()).toBe('已保存')

      second.unmount()
    })

    it('replaces the saved key only after an explicit load; generating uses the typed key without saving it', async () => {
      seedSavedKey('sk-saved')
      const fetchMock = routeFetch(() => catalog('gpt-image-2'))
      const wrapper = mountView()
      await flushPromises()

      await enterKey(wrapper, 'sk-new')
      await wrapper.get('[data-test="prompt-input"]').setValue('不强制先加载模型')
      await wrapper.get('[data-test="generate-button"]').trigger('click')
      await flushPromises()

      // 生成直接使用输入框中的 Key（与改动前一致），但不发目录请求、不替换已保存的 Key。
      expect(imageCalls(fetchMock)).toHaveLength(1)
      expect(authHeader(imageCalls(fetchMock)[0][1])).toBe('Bearer sk-new')
      expect(modelCalls(fetchMock)).toHaveLength(1)
      expect(savedKey()).toBe('sk-saved')

      const enter = await pressEnter(wrapper, '[data-test="api-key-input"]')
      expect(enter.defaultPrevented).toBe(true)
      expect(modelCalls(fetchMock)).toHaveLength(2)
      expect(authHeader(modelCalls(fetchMock)[1][1])).toBe('Bearer sk-new')
      expect(savedKey()).toBe('sk-new')
      expect(wrapper.get('[data-test="key-save-status"]').text()).toBe('已保存')

      wrapper.unmount()
    })

    it('redacts the request key from catalog errors in both error shapes', async () => {
      routeFetch((init) => authHeader(init) === 'Bearer sk-echo-top'
        ? jsonResponse({ code: 'INVALID_API_KEY', message: 'Invalid API key: sk-echo-top' }, 401)
        : jsonResponse({ error: { message: 'Key sk-echo-nested is not allowed for this group' } }, 403))
      const wrapper = mountView()

      await enterKey(wrapper, 'sk-echo-top')
      await loadModels(wrapper)
      expect(wrapper.get('[data-test="model-status"]').text()).toBe('模型列表加载失败：Invalid API key: ***')

      await enterKey(wrapper, 'sk-echo-nested')
      await loadModels(wrapper)
      expect(wrapper.get('[data-test="model-status"]').text()).toBe('模型列表加载失败：Key *** is not allowed for this group')
      expect(wrapper.html()).not.toContain('sk-echo-')

      wrapper.unmount()
    })

    it('redacts with the request key snapshot after the key changed mid-generation', async () => {
      let resolveImages!: (value: unknown) => void
      routeFetch(() => catalog(), () => new Promise((resolve) => {
        resolveImages = resolve
      }))
      const wrapper = mountView()

      await enterKey(wrapper, 'sk-gen-old')
      await wrapper.get('[data-test="prompt-input"]').setValue('脱敏测试')
      await wrapper.get('[data-test="generate-button"]').trigger('click')
      await nextTick()
      // 请求发出后改 Key：脱敏必须用请求时的 Key，而不是当前输入值。
      await enterKey(wrapper, 'sk-gen-new')
      resolveImages(jsonResponse({ error: { message: 'Incorrect API key provided: sk-gen-old' } }, 401))
      await flushPromises()

      expect(wrapper.get('[data-test="error-message"]').text()).toBe('Incorrect API key provided: ***')
      expect(showError).toHaveBeenLastCalledWith('Incorrect API key provided: ***')

      // 顶层 message 结构同样脱敏。
      routeFetch(() => catalog(), () => jsonResponse({ code: 'INVALID_API_KEY', message: 'Invalid API key sk-gen-new' }, 401))
      await wrapper.get('[data-test="generate-button"]').trigger('click')
      await flushPromises()

      expect(wrapper.get('[data-test="error-message"]').text()).toBe('Invalid API key ***')
      expect(showError).toHaveBeenLastCalledWith('Invalid API key ***')
      for (const [message] of showError.mock.calls) {
        expect(String(message)).not.toContain('sk-gen-')
      }

      wrapper.unmount()
    })

    it('clears a remembered model missing from the catalog without picking another one', async () => {
      const fetchMock = routeFetch(() => catalog('grok-imagine-image'))
      const wrapper = mountView()

      await enterKey(wrapper, 'sk-grok')
      await loadModels(wrapper)

      expect(wrapper.get('[data-test="model-badge"]').text()).toBe('未选择模型')
      expect(wrapper.get('[data-test="model-notice"]').text()).toContain('原模型 gpt-image-2 不在当前 Key 的目录')
      expect((wrapper.get('[data-test="model-select"]').element as HTMLSelectElement).value).toBe('')

      await wrapper.get('[data-test="prompt-input"]').setValue('未选模型')
      await wrapper.get('[data-test="generate-button"]').trigger('click')
      await flushPromises()
      expect(wrapper.get('[data-test="error-message"]').text()).toContain('请选择或手动输入模型')
      expect(imageCalls(fetchMock)).toHaveLength(0)
      // 目录清除选择与校验失败的提交都不改写模型偏好。
      expect(JSON.parse(localStorage.getItem(MODEL_PREFERENCE_KEY) || '{}').model).toBe('gpt-image-2')

      wrapper.unmount()
    })

    it('requires a model on a first visit, auto-selects the only candidate and remembers the generated model', async () => {
      localStorage.removeItem(MODEL_PREFERENCE_KEY)
      const fetchMock = routeFetch(() => catalog('gpt-image-2.5-sunburst', 'gpt-5.4'))
      const wrapper = mountView()

      expect(wrapper.get('[data-test="model-badge"]').text()).toBe('未选择模型')
      await enterKey(wrapper, 'sk-single')
      await wrapper.get('[data-test="prompt-input"]').setValue('首次访问')
      // 首次访问没有默认模型：未加载目录前生成会提示选择模型，不再静默使用固定版本。
      await wrapper.get('[data-test="generate-button"]').trigger('click')
      await flushPromises()
      expect(wrapper.get('[data-test="error-message"]').text()).toContain('请选择或手动输入模型')
      expect(imageCalls(fetchMock)).toHaveLength(0)

      await loadModels(wrapper)
      expect(wrapper.get('[data-test="model-badge"]').text()).toBe('gpt-image-2.5-sunburst · 已确认')
      // 目录自动选中不写偏好；真正发出生图请求时才写入。
      expect(localStorage.getItem(MODEL_PREFERENCE_KEY)).toBeNull()

      await wrapper.get('[data-test="generate-button"]').trigger('click')
      await flushPromises()
      expect(JSON.parse(String(imageCalls(fetchMock)[0][1]?.body)).model).toBe('gpt-image-2.5-sunburst')
      expect(JSON.parse(localStorage.getItem(MODEL_PREFERENCE_KEY) || '{}')).toEqual({
        version: 1,
        model: 'gpt-image-2.5-sunburst',
        modelSource: 'list',
      })

      wrapper.unmount()
    })

    it('keeps the current model unconfirmed when the catalog fails and still allows generation', async () => {
      const fetchMock = routeFetch(() => jsonResponse({ error: { message: 'upstream secret detail' } }, 502))
      const wrapper = mountView()

      await enterKey(wrapper, 'sk-down')
      await loadModels(wrapper)

      const status = wrapper.get('[data-test="model-status"]').text()
      expect(status).toContain('HTTP 502')
      expect(status).not.toContain('upstream secret detail')
      expect(wrapper.get('[data-test="model-badge"]').text()).toBe('gpt-image-2 · 未确认')

      await wrapper.get('[data-test="prompt-input"]').setValue('目录失败仍可生成')
      await wrapper.get('[data-test="generate-button"]').trigger('click')
      await flushPromises()
      expect(JSON.parse(String(imageCalls(fetchMock)[0][1]?.body)).model).toBe('gpt-image-2')

      wrapper.unmount()
    })

    it('shows server messages for auth and billing failures', async () => {
      routeFetch((init) => authHeader(init) === 'Bearer sk-invalid'
        ? jsonResponse({ code: 'INVALID_API_KEY', message: 'Invalid API key' }, 401)
        : jsonResponse({ code: 'INSUFFICIENT_BALANCE', message: 'Insufficient account balance' }, 403))
      const wrapper = mountView()

      await enterKey(wrapper, 'sk-invalid')
      await loadModels(wrapper)
      expect(wrapper.get('[data-test="model-status"]').text()).toContain('Invalid API key')
      // 未勾选记住时不提示“未保存”。
      expect(wrapper.find('[data-test="key-storage-message"]').exists()).toBe(false)

      await enterKey(wrapper, 'sk-poor')
      await loadModels(wrapper)
      expect(wrapper.get('[data-test="model-status"]').text()).toContain('Insufficient account balance')

      wrapper.unmount()
    })

    it('shows Retry-After for invalid-auth rate limiting and loads again on an explicit retry', async () => {
      let calls = 0
      const fetchMock = routeFetch(() => {
        calls++
        return calls === 1
          ? jsonResponse(
            { code: 'INVALID_AUTH_RATE_LIMITED', message: 'Too many invalid authentication attempts; retry later' },
            429,
            { 'Retry-After': '30' }
          )
          : catalog('gpt-image-2')
      })
      const wrapper = mountView()

      await enterKey(wrapper, 'sk-limited')
      await loadModels(wrapper)
      expect(wrapper.get('[data-test="model-status"]').text()).toBe(
        'Too many invalid authentication attempts; retry later，请 30 秒后再试'
      )

      await loadModels(wrapper)
      expect(modelCalls(fetchMock)).toHaveLength(2)
      expect(wrapper.get('[data-test="model-badge"]').text()).toBe('gpt-image-2 · 已确认')

      wrapper.unmount()
    })

    it('ignores a late catalog response for a previous key', async () => {
      let resolveFirst!: (value: unknown) => void
      routeFetch((init) => authHeader(init) === 'Bearer sk-slow'
        ? new Promise((resolve) => {
          resolveFirst = resolve
        })
        : catalog('gpt-image-2'))
      const wrapper = mountView()

      const fetchMock = vi.mocked(fetch)

      await enterKey(wrapper, 'sk-slow')
      await loadModels(wrapper)
      // 同一 Key 在途时再次回车不重复请求。
      await pressEnter(wrapper, '[data-test="api-key-input"]')
      expect(modelCalls(fetchMock)).toHaveLength(1)
      await enterKey(wrapper, 'sk-fast')
      await loadModels(wrapper)
      expect(optionValues(wrapper)).toEqual(['', 'gpt-image-2'])

      resolveFirst(catalog('gpt-image-1'))
      await flushPromises()
      expect(optionValues(wrapper)).toEqual(['', 'gpt-image-2'])
      expect(wrapper.get('[data-test="model-badge"]').text()).toBe('gpt-image-2 · 已确认')

      wrapper.unmount()
    })

    it('ignores a late failure for a previous key while the next key is loading', async () => {
      let resolveOld!: (value: unknown) => void
      let resolveNew!: (value: unknown) => void
      const fetchMock = routeFetch((init) => new Promise((resolve) => {
        if (authHeader(init) === 'Bearer sk-old-req') {
          resolveOld = resolve
        } else {
          resolveNew = resolve
        }
      }))
      const wrapper = mountView()

      await enterKey(wrapper, 'sk-old-req')
      await loadModels(wrapper)
      await enterKey(wrapper, 'sk-new-req')
      await loadModels(wrapper)

      resolveOld(jsonResponse({ code: 'INVALID_API_KEY', message: 'Invalid API key sk-old-req' }, 401))
      await flushPromises()
      expect(wrapper.get('[data-test="model-status"]').text()).toContain('正在加载')
      expect(wrapper.html()).not.toContain('sk-old-req')
      // 旧请求的 finally 不能释放新请求的在途标记：同一 Key 再回车仍去重。
      await pressEnter(wrapper, '[data-test="api-key-input"]')
      expect(modelCalls(fetchMock)).toHaveLength(2)

      resolveNew(catalog('gpt-image-2'))
      await flushPromises()
      expect(wrapper.get('[data-test="model-badge"]').text()).toBe('gpt-image-2 · 已确认')

      wrapper.unmount()
    })

    it('generates with a manual model and remembers it as manual', async () => {
      const fetchMock = routeFetch(() => catalog())
      const wrapper = mountView()

      await wrapper.get('[data-test="model-manual-toggle"]').trigger('click')
      await wrapper.get('[data-test="model-manual-input"]').setValue('  my-image-alias  ')
      const enter = await pressEnter(wrapper, '[data-test="model-manual-input"]')

      expect(enter.defaultPrevented).toBe(true)
      expect(imageCalls(fetchMock)).toHaveLength(0)
      expect(wrapper.get('[data-test="model-badge"]').text()).toBe('my-image-alias · 手动')
      expect(JSON.parse(localStorage.getItem(MODEL_PREFERENCE_KEY) || '{}')).toEqual({
        version: 1,
        model: 'my-image-alias',
        modelSource: 'manual',
      })

      await enterKey(wrapper, 'sk-manual')
      await wrapper.get('[data-test="prompt-input"]').setValue('手动模型')
      await wrapper.get('[data-test="generate-button"]').trigger('click')
      await flushPromises()
      expect(JSON.parse(String(imageCalls(fetchMock)[0][1]?.body)).model).toBe('my-image-alias')

      wrapper.unmount()
    })

    it('records the request snapshot model when the selection changes mid-generation', async () => {
      let resolveImages!: (value: unknown) => void
      routeFetch(() => catalog(), () => new Promise((resolve) => {
        resolveImages = resolve
      }))
      const wrapper = mountView()

      await enterKey(wrapper, 'sk-snapshot')
      await wrapper.get('[data-test="prompt-input"]').setValue('快照测试')
      await wrapper.get('[data-test="generate-button"]').trigger('click')
      await nextTick()

      await wrapper.get('[data-test="model-manual-toggle"]').trigger('click')
      await wrapper.get('[data-test="model-manual-input"]').setValue('other-model')
      await wrapper.get('[data-test="model-manual-input"]').trigger('keydown', { key: 'Enter' })
      expect(wrapper.get('[data-test="active-request-model"]').text()).toContain('gpt-image-2')

      resolveImages(imagesResponse())
      await flushPromises()
      const history = JSON.parse(localStorage.getItem(HISTORY_KEY) || '[]')
      expect(history[0].model).toBe('gpt-image-2')
      expect(wrapper.get('[data-test="model-badge"]').text()).toBe('other-model · 手动')

      wrapper.unmount()
    })

    it('does not change the selected model when restoring a history item', async () => {
      localStorage.setItem(HISTORY_KEY, JSON.stringify([{
        id: 'h1',
        createdAt: '2026-11-02T00:00:00.000Z',
        prompt: '旧记录',
        model: 'gpt-image-1',
        size: '1024x1024',
        images: ['data:image/png;base64,aGVsbG8='],
        mode: 'generate',
      }]))
      const wrapper = mountView()

      await wrapper.get('[data-test="history-item"] button').trigger('click')

      expect((wrapper.get('[data-test="prompt-input"]').element as HTMLTextAreaElement).value).toBe('旧记录')
      expect(wrapper.get('[data-test="model-badge"]').text()).toBe('gpt-image-2 · 未确认')

      wrapper.unmount()
    })
  })

  describe('remembered API key', () => {
    it('saves the key only after the catalog succeeds and restores it on reload', async () => {
      let resolveCatalog: ((value: unknown) => void) | null = null
      const fetchMock = routeFetch(() => resolveCatalog === null
        ? new Promise((resolve) => {
          resolveCatalog = resolve
        })
        : catalog('gpt-image-2'))
      const first = mountView()

      expect(localStorage.getItem(SAVED_KEY_KEY)).toBeNull()
      await enterKey(first, 'sk-remember')
      // 勾选本身不发请求，等待用户显式加载。
      await first.get('[data-test="remember-key-checkbox"]').setValue(true)
      await flushPromises()
      expect(modelCalls(fetchMock)).toHaveLength(0)
      expect(first.get('[data-test="key-save-status"]').text()).toBe('待保存：当前 Key 加载模型成功后保存')

      await loadModels(first)
      expect(localStorage.getItem(SAVED_KEY_KEY)).toBeNull()
      resolveCatalog!(catalog('gpt-image-2'))
      await flushPromises()
      expect(modelCalls(fetchMock)).toHaveLength(1)
      expect(JSON.parse(localStorage.getItem(SAVED_KEY_KEY) || '{}')).toEqual({ version: 1, apiKey: 'sk-remember' })
      expect(first.get('[data-test="key-save-status"]').text()).toBe('已保存')
      first.unmount()

      // 模拟刷新页面：新 store 重新读取已保存配置并发起目录请求。
      reloadPage()
      const second = mountView()
      await flushPromises()

      expect((second.get('[data-test="api-key-input"]').element as HTMLInputElement).value).toBe('sk-remember')
      expect((second.get('[data-test="remember-key-checkbox"]').element as HTMLInputElement).checked).toBe(true)
      expect(JSON.parse(localStorage.getItem(SAVED_KEY_KEY) || '{}').apiKey).toBe('sk-remember')
      expect(modelCalls(fetchMock)).toHaveLength(2)
      expect(second.get('[data-test="model-badge"]').text()).toBe('gpt-image-2 · 已确认')

      second.unmount()
    })

    it('keeps the saved key when the catalog request fails during restore', async () => {
      seedSavedKey('sk-offline')
      routeFetch(() => jsonResponse({}, 503))
      const wrapper = mountView()
      await flushPromises()

      expect(wrapper.get('[data-test="model-status"]').text()).toContain('HTTP 503')
      expect(JSON.parse(localStorage.getItem(SAVED_KEY_KEY) || '{}').apiKey).toBe('sk-offline')
      expect(wrapper.get('[data-test="key-save-status"]').text()).toBe('已保存')

      wrapper.unmount()
    })

    it('keeps the saved key on edit and does not replace it with an invalid key', async () => {
      seedSavedKey('sk-old')
      routeFetch((init) => authHeader(init) === 'Bearer sk-typo'
        ? jsonResponse({ code: 'INVALID_API_KEY', message: 'Invalid API key' }, 401)
        : catalog('gpt-image-2'))
      const wrapper = mountView()
      await flushPromises()

      await enterKey(wrapper, 'sk-typo')
      expect(savedKey()).toBe('sk-old')
      expect(rememberChecked(wrapper)).toBe(true)

      await loadModels(wrapper)
      expect(wrapper.get('[data-test="key-storage-message"]').text()).toBe('Key 无效，未保存')
      expect(wrapper.get('[data-test="key-save-status"]').text()).toBe('浏览器中保存的是之前的 Key，当前 Key 加载模型成功后替换')
      expect(savedKey()).toBe('sk-old')

      // 改回原 Key 即与已保存值一致，无需重新写入。
      await enterKey(wrapper, 'sk-old')
      expect(wrapper.get('[data-test="key-save-status"]').text()).toBe('已保存')

      await enterKey(wrapper, 'sk-replacement')
      await loadModels(wrapper)
      expect(savedKey()).toBe('sk-replacement')
      expect(wrapper.get('[data-test="key-save-status"]').text()).toBe('已保存')

      wrapper.unmount()
    })

    it('does not write a key whose catalog response arrives after the key changed', async () => {
      let resolveFirst!: (value: unknown) => void
      routeFetch((init) => authHeader(init) === 'Bearer sk-first'
        ? new Promise((resolve) => {
          resolveFirst = resolve
        })
        : new Promise(() => {}))
      const wrapper = mountView()

      await enterKey(wrapper, 'sk-first')
      await wrapper.get('[data-test="remember-key-checkbox"]').setValue(true)
      await loadModels(wrapper)
      await enterKey(wrapper, 'sk-second')

      resolveFirst(catalog('gpt-image-2'))
      await flushPromises()
      expect(localStorage.getItem(SAVED_KEY_KEY)).toBeNull()

      wrapper.unmount()
    })

    it('removes the saved key when unchecked but keeps the in-memory key', async () => {
      seedSavedKey('sk-keep-memory')
      routeFetch(() => catalog('gpt-image-2'))
      const wrapper = mountView()
      await flushPromises()

      await wrapper.get('[data-test="remember-key-checkbox"]').setValue(false)

      expect(localStorage.getItem(SAVED_KEY_KEY)).toBeNull()
      expect((wrapper.get('[data-test="api-key-input"]').element as HTMLInputElement).value).toBe('sk-keep-memory')

      wrapper.unmount()
    })

    it('clears the saved key and model preference but keeps history', async () => {
      seedSavedKey('sk-clear')
      localStorage.setItem(HISTORY_KEY, JSON.stringify([{
        id: 'h1',
        createdAt: '2026-11-02T00:00:00.000Z',
        prompt: '保留历史',
        model: 'gpt-image-2',
        size: '1024x1024',
        images: ['data:image/png;base64,aGVsbG8='],
      }]))
      routeFetch(() => catalog('gpt-image-2'))
      const wrapper = mountView()
      await flushPromises()

      await wrapper.get('[data-test="clear-saved-config"]').trigger('click')

      expect(localStorage.getItem(SAVED_KEY_KEY)).toBeNull()
      expect(localStorage.getItem(MODEL_PREFERENCE_KEY)).toBeNull()
      expect((wrapper.get('[data-test="api-key-input"]').element as HTMLInputElement).value).toBe('')
      expect(wrapper.get('[data-test="model-badge"]').text()).toBe('未选择模型')
      expect(optionValues(wrapper)).toEqual([''])
      expect(wrapper.findAll('[data-test="history-item"]')).toHaveLength(1)
      expect(showSuccess).toHaveBeenCalledWith('已清除保存的 API Key 和模型')

      wrapper.unmount()
    })

    it('ignores a corrupted saved key without requesting models', async () => {
      localStorage.setItem(SAVED_KEY_KEY, '{broken')
      const fetchMock = routeFetch(() => catalog('gpt-image-2'))
      const wrapper = mountView()
      await flushPromises()

      expect((wrapper.get('[data-test="api-key-input"]').element as HTMLInputElement).value).toBe('')
      expect(modelCalls(fetchMock)).toHaveLength(0)
      expect(localStorage.getItem(SAVED_KEY_KEY)).toBeNull()
      expect(wrapper.get('[data-test="key-storage-message"]').text()).toContain('配置无效')
      expect(wrapper.find('[data-test="key-removal-warning"]').exists()).toBe(false)

      wrapper.unmount()
    })

    it('shows the leftover-key warning when a corrupted saved key cannot be removed', async () => {
      localStorage.setItem(SAVED_KEY_KEY, '{broken')
      const removeSpy = failSavedKeyRemovals()
      try {
        const fetchMock = routeFetch(() => catalog('gpt-image-2'))
        const wrapper = mountView()
        await flushPromises()

        expect(modelCalls(fetchMock)).toHaveLength(0)
        expect(localStorage.getItem(SAVED_KEY_KEY)).toBe('{broken')
        expect(wrapper.get('[data-test="key-storage-message"]').text()).toContain('配置无效')
        expect(wrapper.find('[data-test="key-removal-warning"]').exists()).toBe(true)
        expect(wrapper.find('[data-test="retry-key-removal"]').exists()).toBe(true)

        wrapper.unmount()
      } finally {
        removeSpy.mockRestore()
      }
    })

    it('reports a storage failure instead of claiming the key was saved', async () => {
      const setItemSpy = failSavedKeyWrites()
      try {
        routeFetch(() => catalog('gpt-image-2'))
        const wrapper = mountView()

        await enterKey(wrapper, 'sk-quota')
        await wrapper.get('[data-test="remember-key-checkbox"]').setValue(true)
        await loadModels(wrapper)

        expect(wrapper.get('[data-test="key-storage-message"]').text()).toBe('无法保存，当前仅本次页面有效')
        expect((wrapper.get('[data-test="remember-key-checkbox"]').element as HTMLInputElement).checked).toBe(false)
        expect(wrapper.find('[data-test="key-save-status"]').exists()).toBe(false)

        wrapper.unmount()
      } finally {
        setItemSpy.mockRestore()
      }
    })

    it('does not save a key whose latest catalog response failed', async () => {
      let calls = 0
      const fetchMock = routeFetch(() => {
        calls++
        return calls === 1
          ? catalog('gpt-image-2')
          : jsonResponse({ code: 'API_KEY_DISABLED', message: 'API key is disabled' }, 401)
      })
      const wrapper = mountView()

      await enterKey(wrapper, 'sk-revoked')
      await loadModels(wrapper)
      await wrapper.get('[data-test="model-refresh-button"]').trigger('click')
      await flushPromises()

      await wrapper.get('[data-test="remember-key-checkbox"]').setValue(true)
      await flushPromises()

      // 勾选不发请求，也不保存最近一次被拒绝的 Key。
      expect(modelCalls(fetchMock)).toHaveLength(2)
      expect(localStorage.getItem(SAVED_KEY_KEY)).toBeNull()
      expect(wrapper.get('[data-test="key-save-status"]').text()).toBe('待保存：当前 Key 加载模型成功后保存')

      wrapper.unmount()
    })

    it('saves immediately when the current key was just verified, and syncs the checkbox if that fails', async () => {
      const fetchMock = routeFetch(() => catalog('gpt-image-2'))
      const wrapper = mountView()

      // 空 Key 勾选只进入待保存，不写入也不请求。
      await wrapper.get('[data-test="remember-key-checkbox"]').setValue(true)
      expect(wrapper.get('[data-test="key-save-status"]').text()).toBe('待保存：输入 API Key 后保存')
      expect(localStorage.getItem(SAVED_KEY_KEY)).toBeNull()
      await wrapper.get('[data-test="remember-key-checkbox"]').setValue(false)

      await enterKey(wrapper, 'sk-verified')
      await loadModels(wrapper)
      expect(modelCalls(fetchMock)).toHaveLength(1)

      const setItemSpy = failSavedKeyWrites()
      try {
        // 已验证的 Key 同步写入；写入失败时勾选框必须同步回未勾选。
        await wrapper.get('[data-test="remember-key-checkbox"]').setValue(true)
        expect(modelCalls(fetchMock)).toHaveLength(1)
        expect((wrapper.get('[data-test="remember-key-checkbox"]').element as HTMLInputElement).checked).toBe(false)
        expect(wrapper.get('[data-test="key-storage-message"]').text()).toBe('无法保存，当前仅本次页面有效')
      } finally {
        setItemSpy.mockRestore()
      }

      await wrapper.get('[data-test="remember-key-checkbox"]').setValue(true)
      expect(JSON.parse(localStorage.getItem(SAVED_KEY_KEY) || '{}').apiKey).toBe('sk-verified')

      wrapper.unmount()
    })

    it('cancels an in-flight catalog request when saved config is cleared', async () => {
      let resolveCatalog!: (value: unknown) => void
      routeFetch(() => new Promise((resolve) => {
        resolveCatalog = resolve
      }))
      const wrapper = mountView()

      await enterKey(wrapper, 'sk-inflight')
      await loadModels(wrapper)
      await wrapper.get('[data-test="clear-saved-config"]').trigger('click')

      resolveCatalog(catalog('gpt-image-2'))
      await flushPromises()
      expect(optionValues(wrapper)).toEqual([''])
      expect(wrapper.get('[data-test="model-status"]').text()).toBe('输入 API Key 后按回车或点击“加载模型”。')

      wrapper.unmount()
    })

    it('ignores cross-tab changes to the model preference', async () => {
      seedSavedKey('sk-tab-model')
      routeFetch(() => catalog('gpt-image-2'))
      const wrapper = mountView()
      await flushPromises()

      window.dispatchEvent(new StorageEvent('storage', { key: MODEL_PREFERENCE_KEY }))
      await nextTick()

      expect((wrapper.get('[data-test="remember-key-checkbox"]').element as HTMLInputElement).checked).toBe(true)
      expect(wrapper.find('[data-test="key-storage-message"]').exists()).toBe(false)

      wrapper.unmount()
    })

    it.each([
      ['removes the saved key', false],
      ['clears all storage', true],
    ])('does not write the key back when another tab %s during an in-flight load', async (_label, clearAll) => {
      seedSavedKey('sk-shared')
      let resolveNext!: (value: unknown) => void
      routeFetch((init) => authHeader(init) === 'Bearer sk-shared'
        ? catalog('gpt-image-2')
        : new Promise((resolve) => {
          resolveNext = resolve
        }))
      const wrapper = mountView()
      await flushPromises()

      await enterKey(wrapper, 'sk-next')
      await loadModels(wrapper)

      // 另一标签页删除记录：本页收到 storage 事件（key 为 null 表示整体清空）。
      if (clearAll) {
        localStorage.clear()
        window.dispatchEvent(new StorageEvent('storage', { key: null }))
      } else {
        localStorage.removeItem(SAVED_KEY_KEY)
        window.dispatchEvent(new StorageEvent('storage', { key: SAVED_KEY_KEY, oldValue: '{}', newValue: null }))
      }
      await nextTick()
      expect(rememberChecked(wrapper)).toBe(false)

      resolveNext(catalog('gpt-image-2'))
      await flushPromises()
      expect(localStorage.getItem(SAVED_KEY_KEY)).toBeNull()
      expect(wrapper.find('[data-test="key-save-status"]').exists()).toBe(false)

      wrapper.unmount()
    })

    it('drops the old saved key when overwriting it fails', async () => {
      seedSavedKey('sk-old-saved')
      routeFetch(() => catalog('gpt-image-2'))
      const wrapper = mountView()
      await flushPromises()

      const setItemSpy = failSavedKeyWrites()
      try {
        await enterKey(wrapper, 'sk-unwritable')
        await loadModels(wrapper)
      } finally {
        setItemSpy.mockRestore()
      }

      // 退回未记住时，浏览器里不能还留着旧 Key。
      expect(wrapper.get('[data-test="key-storage-message"]').text()).toBe('无法保存，当前仅本次页面有效')
      expect(rememberChecked(wrapper)).toBe(false)
      expect(localStorage.getItem(SAVED_KEY_KEY)).toBeNull()
      expect(wrapper.find('[data-test="key-removal-warning"]').exists()).toBe(false)

      wrapper.unmount()
    })

    it('keeps a retry entry when clearing a key-only config fails to remove the key', async () => {
      localStorage.removeItem(MODEL_PREFERENCE_KEY)
      seedSavedKey('sk-stuck')
      routeFetch(() => catalog('gpt-image-2', 'gpt-image-1'))
      const wrapper = mountView()
      await flushPromises()
      // 无偏好且多个候选时不自动选择，也不写模型偏好：此时只有 Key 被保存。
      expect(wrapper.get('[data-test="model-badge"]').text()).toBe('未选择模型')
      expect(localStorage.getItem(MODEL_PREFERENCE_KEY)).toBeNull()

      const removeSpy = failSavedKeyRemovals()
      try {
        await wrapper.get('[data-test="clear-saved-config"]').trigger('click')

        expect(savedKey()).toBe('sk-stuck')
        expect(wrapper.get('[data-test="key-removal-warning"]').text()).toContain('可能仍有残留')
        expect(wrapper.find('[data-test="clear-saved-config"]').exists()).toBe(true)
        expect(showSuccess).not.toHaveBeenCalled()

        await wrapper.get('[data-test="retry-key-removal"]').trigger('click')
        expect(showError).toHaveBeenCalledWith('仍未能清除，请检查浏览器存储设置后重试')
        expect(wrapper.find('[data-test="key-removal-warning"]').exists()).toBe(true)
      } finally {
        removeSpy.mockRestore()
      }

      // 存储恢复后重试成功才解除残留状态。
      await wrapper.get('[data-test="retry-key-removal"]').trigger('click')
      expect(localStorage.getItem(SAVED_KEY_KEY)).toBeNull()
      expect(wrapper.find('[data-test="key-removal-warning"]').exists()).toBe(false)
      expect(wrapper.find('[data-test="clear-saved-config"]').exists()).toBe(false)
      expect(showSuccess).toHaveBeenCalledWith('已清除浏览器中保存的 API Key')

      wrapper.unmount()
    })

    it('keeps the removal warning through edits, re-checking and empty input until an overwrite succeeds', async () => {
      localStorage.removeItem(MODEL_PREFERENCE_KEY)
      seedSavedKey('sk-residue')
      routeFetch(() => catalog('gpt-image-2'))
      const wrapper = mountView()
      await flushPromises()

      const warning = () => wrapper.find('[data-test="key-removal-warning"]').exists()
      const removeSpy = failSavedKeyRemovals()
      try {
        await wrapper.get('[data-test="remember-key-checkbox"]').setValue(false)
        expect(warning()).toBe(true)
        expect(savedKey()).toBe('sk-residue')

        await enterKey(wrapper, 'sk-other')
        expect(warning()).toBe(true)
        await enterKey(wrapper, '')
        await wrapper.get('[data-test="remember-key-checkbox"]').setValue(true)
        expect(warning()).toBe(true)
        await wrapper.get('[data-test="remember-key-checkbox"]').setValue(false)
        expect(warning()).toBe(true)
        expect(wrapper.find('[data-test="clear-saved-config"]').exists()).toBe(true)
      } finally {
        removeSpy.mockRestore()
      }

      // 勾选未验证的新 Key 不算覆盖；加载成功写入新 Key 后才解除。
      await enterKey(wrapper, 'sk-final')
      await wrapper.get('[data-test="remember-key-checkbox"]').setValue(true)
      expect(warning()).toBe(true)
      await loadModels(wrapper)
      expect(savedKey()).toBe('sk-final')
      expect(warning()).toBe(false)

      wrapper.unmount()
    })

    it('leaves remember mode when another tab changes the saved key and keeps that tab\'s key', async () => {
      seedSavedKey('sk-tab')
      routeFetch(() => catalog('gpt-image-2'))
      const wrapper = mountView()
      await flushPromises()

      // 另一标签页改存了自己的 Key：本页退出记住，但不删除对方的值。
      seedSavedKey('sk-other-tab')
      window.dispatchEvent(new StorageEvent('storage', { key: SAVED_KEY_KEY, oldValue: '{}', newValue: '{}' }))
      await nextTick()

      expect(rememberChecked(wrapper)).toBe(false)
      expect(wrapper.get('[data-test="key-storage-message"]').text()).toContain('其他标签页')
      expect(savedKey()).toBe('sk-other-tab')

      wrapper.unmount()
    })

    it('removes its own late write when another tab deleted the key before its event arrived', async () => {
      seedSavedKey('sk-shared')
      let resolveNext!: (value: unknown) => void
      routeFetch((init) => authHeader(init) === 'Bearer sk-shared'
        ? catalog('gpt-image-2')
        : new Promise((resolve) => {
          resolveNext = resolve
        }))
      const wrapper = mountView()
      await flushPromises()

      await enterKey(wrapper, 'sk-next')
      await loadModels(wrapper)
      // 另一标签页已删除记录，但本页的目录成功先于它的 storage 事件到达并写入。
      localStorage.removeItem(SAVED_KEY_KEY)
      resolveNext(catalog('gpt-image-2'))
      await flushPromises()
      expect(savedKey()).toBe('sk-next')

      window.dispatchEvent(new StorageEvent('storage', { key: SAVED_KEY_KEY, oldValue: '{}', newValue: null }))
      await nextTick()

      // 两边都不再记住时，存储里不能留下 Key（否则下次打开会被恢复）。
      expect(savedKey()).toBeNull()
      expect(rememberChecked(wrapper)).toBe(false)
      expect(wrapper.find('[data-test="key-removal-warning"]').exists()).toBe(false)

      wrapper.unmount()
    })

    it('clears the leftover-key warning when another tab removes the record', async () => {
      seedSavedKey('sk-residue')
      routeFetch(() => catalog('gpt-image-2'))
      const wrapper = mountView()
      await flushPromises()

      const removeSpy = failSavedKeyRemovals()
      try {
        await wrapper.get('[data-test="remember-key-checkbox"]').setValue(false)
        expect(wrapper.find('[data-test="key-removal-warning"]').exists()).toBe(true)
      } finally {
        removeSpy.mockRestore()
      }

      localStorage.removeItem(SAVED_KEY_KEY)
      window.dispatchEvent(new StorageEvent('storage', { key: SAVED_KEY_KEY, oldValue: '{}', newValue: null }))
      await nextTick()

      expect(wrapper.find('[data-test="key-removal-warning"]').exists()).toBe(false)

      wrapper.unmount()
    })
  })
})
