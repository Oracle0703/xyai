import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'

import ImageGenView from '../ImageGenView.vue'

const { fetchPublicSettings, authState } = vi.hoisted(() => ({
  fetchPublicSettings: vi.fn(),
  authState: { isAuthenticated: false, isAdmin: false },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    cachedPublicSettings: null,
    siteName: 'Sub2API',
    siteLogo: '',
    docUrl: '',
    publicSettingsLoaded: true,
    backendModeEnabled: false,
    fetchPublicSettings,
  }),
  useAuthStore: () => authState,
}))

// 用轻量桩替换平台布局，避免拉起侧边栏/引导等重依赖；仅验证是否被包裹。
vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: {
    name: 'AppLayout',
    template: '<div data-test="app-layout"><slot /></div>',
  },
}))

describe('ImageGenView', () => {
  beforeEach(() => {
    fetchPublicSettings.mockReset()
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
    const wrapper = mount(ImageGenView, {
      global: {
        stubs: {
          RouterLink: { template: '<a><slot /></a>' },
          Icon: true,
        },
      },
    })

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
    const wrapper = mount(ImageGenView, {
      global: {
        stubs: {
          RouterLink: { template: '<a><slot /></a>' },
          Icon: true,
        },
      },
    })

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
    const wrapper = mount(ImageGenView, {
      global: {
        stubs: {
          RouterLink: { template: '<a><slot /></a>' },
          Icon: true,
        },
      },
    })
    const sourceImage = new File(['source image'], 'source.png', { type: 'image/png' })

    await wrapper.get('[data-test="mode-edit-button"]').trigger('click')
    await wrapper.get('[data-test="api-key-input"]').setValue('sk-edit-key')
    await wrapper.get('[data-test="prompt-input"]').setValue('把背景改成白色')
    const input = wrapper.get('[data-test="source-image-input"]').element as HTMLInputElement
    Object.defineProperty(input, 'files', {
      configurable: true,
      value: [sourceImage],
    })
    await wrapper.get('[data-test="source-image-input"]').trigger('change')
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
    expect(formData.get('image')).toBe(sourceImage)

    wrapper.unmount()
  })

  it('uses a pasted image as the image edit source', async () => {
    const wrapper = mount(ImageGenView, {
      global: {
        stubs: {
          RouterLink: { template: '<a><slot /></a>' },
          Icon: true,
        },
      },
    })
    const pastedImage = new File(['pasted image'], 'paste.png', { type: 'image/png' })

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
    expect(body.get('image')).toBe(pastedImage)

    wrapper.unmount()
  })

  it('renders standalone (no app layout) for unauthenticated visitors', () => {
    const wrapper = mount(ImageGenView, {
      global: {
        stubs: {
          RouterLink: { template: '<a><slot /></a>' },
          Icon: true,
        },
      },
    })

    expect(wrapper.find('[data-test="app-layout"]').exists()).toBe(false)
    // 独立页保留自带的头部标题
    expect(wrapper.text()).toContain('图片生成工具')
    expect(wrapper.find('[data-test="api-key-input"]').exists()).toBe(true)

    wrapper.unmount()
  })

  it('embeds in the app layout for authenticated users', () => {
    authState.isAuthenticated = true

    const wrapper = mount(ImageGenView, {
      global: {
        stubs: {
          RouterLink: { template: '<a><slot /></a>' },
          Icon: true,
        },
      },
    })

    // 嵌入平台布局（侧边栏 + 右侧内容区），不再渲染独立整页头部
    expect(wrapper.find('[data-test="app-layout"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('图片生成工具')
    // 生成器本体仍然渲染在内容区
    expect(wrapper.find('[data-test="api-key-input"]').exists()).toBe(true)

    wrapper.unmount()
  })
})
