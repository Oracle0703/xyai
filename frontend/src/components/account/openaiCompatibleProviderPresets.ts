import type { AccountPlatform } from '@/types'

export type OpenAICompatibleProviderId =
  | 'openai'
  | 'volcengine'
  | 'qwen'
  | 'deepseek'
  | 'moonshot'
  | 'custom'

export interface OpenAICompatibleProviderPreset {
  id: OpenAICompatibleProviderId
  label: string
  platform: AccountPlatform
  defaultBaseUrl: string
  apiKeyPlaceholder: string
}

export const OPENAI_COMPATIBLE_PROVIDER_PRESETS: OpenAICompatibleProviderPreset[] = [
  {
    id: 'openai',
    label: 'OpenAI',
    platform: 'openai',
    defaultBaseUrl: 'https://api.openai.com',
    apiKeyPlaceholder: 'sk-proj-...'
  },
  {
    id: 'volcengine',
    label: '火山方舟',
    platform: 'openai',
    defaultBaseUrl: 'https://ark.cn-beijing.volces.com/api/v3',
    apiKeyPlaceholder: 'your-volcengine-api-key'
  },
  {
    id: 'qwen',
    label: 'Qwen / 阿里云百炼',
    platform: 'openai',
    defaultBaseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    apiKeyPlaceholder: 'sk-...'
  },
  {
    id: 'deepseek',
    label: 'DeepSeek',
    platform: 'openai',
    defaultBaseUrl: 'https://api.deepseek.com',
    apiKeyPlaceholder: 'sk-...'
  },
  {
    id: 'moonshot',
    label: 'Moonshot / Kimi',
    platform: 'openai',
    defaultBaseUrl: 'https://api.moonshot.cn/v1',
    apiKeyPlaceholder: 'sk-...'
  },
  {
    id: 'custom',
    label: '自定义兼容平台',
    platform: 'openai',
    defaultBaseUrl: '',
    apiKeyPlaceholder: 'sk-...'
  }
]

export function getOpenAICompatibleProviderPreset(
  id: OpenAICompatibleProviderId
): OpenAICompatibleProviderPreset {
  return (
    OPENAI_COMPATIBLE_PROVIDER_PRESETS.find((preset) => preset.id === id) ??
    OPENAI_COMPATIBLE_PROVIDER_PRESETS[0]
  )
}

export function applyOpenAICompatibleProviderPreset(
  credentials: Record<string, unknown>,
  id: OpenAICompatibleProviderId
): Record<string, unknown> {
  const preset = getOpenAICompatibleProviderPreset(id)
  const next = { ...credentials }
  const baseUrl = typeof next.base_url === 'string' ? next.base_url.trim() : ''

  if (!baseUrl && preset.defaultBaseUrl) {
    next.base_url = preset.defaultBaseUrl
  }

  next.openai_compatible_provider = preset.id
  return next
}
