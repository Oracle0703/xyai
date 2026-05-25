import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'

import ImageGenView from '../ImageGenView.vue'

const { fetchPublicSettings } = vi.hoisted(() => ({
  fetchPublicSettings: vi.fn(),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    cachedPublicSettings: null,
    siteName: 'Sub2API',
    siteLogo: '',
    docUrl: '',
    publicSettingsLoaded: true,
    fetchPublicSettings,
  }),
}))

describe('ImageGenView', () => {
  beforeEach(() => {
    fetchPublicSettings.mockReset()
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
})
