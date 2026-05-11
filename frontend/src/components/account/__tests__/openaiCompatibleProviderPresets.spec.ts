import { describe, expect, it } from 'vitest'
import {
  OPENAI_COMPATIBLE_PROVIDER_PRESETS,
  applyOpenAICompatibleProviderPreset,
  getOpenAICompatibleProviderPreset
} from '../openaiCompatibleProviderPresets'

describe('openaiCompatibleProviderPresets', () => {
  it('defines Volcengine and Qwen presets as OpenAI-compatible providers', () => {
    expect(getOpenAICompatibleProviderPreset('volcengine')).toMatchObject({
      id: 'volcengine',
      platform: 'openai',
      defaultBaseUrl: 'https://ark.cn-beijing.volces.com/api/v3'
    })
    expect(getOpenAICompatibleProviderPreset('qwen')).toMatchObject({
      id: 'qwen',
      platform: 'openai',
      defaultBaseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1'
    })
  })

  it('keeps provider ids unique', () => {
    const ids = OPENAI_COMPATIBLE_PROVIDER_PRESETS.map((preset) => preset.id)
    expect(new Set(ids).size).toBe(ids.length)
  })

  it('applies provider metadata to credentials while keeping backend platform openai', () => {
    const credentials = applyOpenAICompatibleProviderPreset(
      { api_key: 'sk-test', base_url: '' },
      'qwen'
    )

    expect(credentials).toMatchObject({
      api_key: 'sk-test',
      base_url: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
      openai_compatible_provider: 'qwen'
    })
  })

  it('does not replace a user-provided base URL', () => {
    const credentials = applyOpenAICompatibleProviderPreset(
      { api_key: 'sk-test', base_url: 'https://proxy.example.com/v1' },
      'volcengine'
    )

    expect(credentials.base_url).toBe('https://proxy.example.com/v1')
    expect(credentials.openai_compatible_provider).toBe('volcengine')
  })
})
