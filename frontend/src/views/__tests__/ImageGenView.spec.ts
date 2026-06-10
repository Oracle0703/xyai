import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'

import ImageGenView from '../ImageGenView.vue'

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
})
