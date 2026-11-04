/**
 * 图片生成页的模型目录解析、候选判定与本地偏好存储。
 *
 * 全部为纯函数，store 负责状态与请求时序。设计见
 * docs/features/image-gen-design-cn.md。
 */

export const MODEL_PREFERENCE_STORAGE_KEY = 'image-gen-model-v1'
export const SAVED_API_KEY_STORAGE_KEY = 'image-gen-key-v1'
const STORAGE_VERSION = 1

export type ModelSource = 'list' | 'manual'

export interface ModelPreference {
  model: string
  source: ModelSource
}

export type SavedApiKeyReadResult =
  | { kind: 'none' }
  | { kind: 'ok'; apiKey: string }
  | { kind: 'invalid'; removed: boolean }

// 镜像后端 validateCompatibleImagesModel（backend/internal/service/openai_images.go）：
// 图片端点只接受这些模型名。后端判定变化时必须同步这里，否则候选列表会漏选或多选。
export function isImageModelCandidate(id: string): boolean {
  const model = id.toLowerCase()
  if (model.startsWith('gpt-image-')) {
    return true
  }
  if (model === 'grok-imagine' || model === 'grok-imagine-edit' || model.startsWith('grok-imagine-image')) {
    return true
  }
  return model.startsWith('gemini-') && (model.endsWith('-image') || model.includes('-image-'))
}

// 模型 ID 原样使用：不裁剪、不改写，含首尾空白或控制字符的直接拒绝。
export function isValidModelId(id: string): boolean {
  if (id.length === 0 || id !== id.trim()) {
    return false
  }
  for (let index = 0; index < id.length; index++) {
    const code = id.charCodeAt(index)
    // C0、DEL 与 C1 控制字符
    if (code < 0x20 || (code >= 0x7f && code <= 0x9f)) {
      return false
    }
  }
  return true
}

const modelIdCollator = new Intl.Collator('en', { numeric: true, sensitivity: 'variant' })

// 自然排序（数字段按数值比较），固定 locale 并以码位兜底，保证结果确定；只用于展示。
export function compareModelIds(a: string, b: string): number {
  const result = modelIdCollator.compare(a, b)
  if (result !== 0) {
    return result
  }
  return a < b ? -1 : a > b ? 1 : 0
}

/**
 * 从 `/v1/models` 响应中取出图片模型候选。
 * 顶层缺少 data 数组返回 null（响应异常），合法空数组返回 []（空目录）。
 */
export function parseImageModelCatalog(payload: unknown): string[] | null {
  if (!payload || typeof payload !== 'object') {
    return null
  }
  const data = (payload as { data?: unknown }).data
  if (!Array.isArray(data)) {
    return null
  }
  const ids = new Set<string>()
  for (const entry of data) {
    if (!entry || typeof entry !== 'object') {
      continue
    }
    const id = (entry as { id?: unknown }).id
    if (typeof id !== 'string' || !isValidModelId(id) || !isImageModelCandidate(id)) {
      continue
    }
    ids.add(id)
  }
  return Array.from(ids).sort(compareModelIds)
}

// 模型发现只向生图请求本来就会发送 Key 的 origin 发请求；端点不同源时停用自动发现。
export function endpointsShareOrigin(endpoints: string[], base: string): boolean {
  try {
    const origins = endpoints.map((endpoint) => new URL(endpoint, base).origin)
    return origins.every((origin) => origin === origins[0])
  } catch {
    return false
  }
}

export const REDACTED_CREDENTIAL = '***'

// 服务端错误文案可能回显请求凭据（如外部网关），展示前用发起请求时的 Key 快照逐处替换。
export function redactCredential(message: string, credential: string): string {
  if (!credential || !message.includes(credential)) {
    return message
  }
  return message.split(credential).join(REDACTED_CREDENTIAL)
}

// ==================== Storage ====================

function getStorage(): Storage | null {
  try {
    return window.localStorage
  } catch {
    return null
  }
}

function writeJSON(storage: Storage | null, key: string, value: unknown): boolean {
  if (!storage) {
    return false
  }
  try {
    storage.setItem(key, JSON.stringify(value))
    return true
  } catch {
    return false
  }
}

function removeItem(storage: Storage | null, key: string): boolean {
  if (!storage) {
    return false
  }
  try {
    storage.removeItem(key)
    return true
  } catch {
    return false
  }
}

// found=false 表示无记录或存储不可读；found=true 时 value 为解析结果（JSON 损坏为 undefined，内容可能不合法）。
function readJSON(storage: Storage | null, key: string): { found: false } | { found: true; value: unknown } {
  if (!storage) {
    return { found: false }
  }
  let raw: string | null
  try {
    raw = storage.getItem(key)
  } catch {
    return { found: false }
  }
  if (raw === null) {
    return { found: false }
  }
  try {
    return { found: true, value: JSON.parse(raw) }
  } catch {
    return { found: true, value: undefined }
  }
}

export function readModelPreference(storage: Storage | null = getStorage()): ModelPreference | null {
  const result = readJSON(storage, MODEL_PREFERENCE_STORAGE_KEY)
  if (!result.found) {
    return null
  }
  const value = result.value as { version?: unknown; model?: unknown; modelSource?: unknown } | null
  if (
    value
    && typeof value === 'object'
    && value.version === STORAGE_VERSION
    && typeof value.model === 'string'
    && isValidModelId(value.model)
    && (value.modelSource === 'list' || value.modelSource === 'manual')
  ) {
    return { model: value.model, source: value.modelSource }
  }
  // 模型偏好不敏感，损坏时静默尽力删除即可。
  removeItem(storage, MODEL_PREFERENCE_STORAGE_KEY)
  return null
}

export function writeModelPreference(preference: ModelPreference, storage: Storage | null = getStorage()): boolean {
  return writeJSON(storage, MODEL_PREFERENCE_STORAGE_KEY, {
    version: STORAGE_VERSION,
    model: preference.model,
    modelSource: preference.source,
  })
}

export function removeModelPreference(storage: Storage | null = getStorage()): boolean {
  return removeItem(storage, MODEL_PREFERENCE_STORAGE_KEY)
}

export function readSavedApiKey(storage: Storage | null = getStorage()): SavedApiKeyReadResult {
  const result = readJSON(storage, SAVED_API_KEY_STORAGE_KEY)
  if (!result.found) {
    return { kind: 'none' }
  }
  const value = result.value as { version?: unknown; apiKey?: unknown } | null
  if (
    value
    && typeof value === 'object'
    && value.version === STORAGE_VERSION
    && typeof value.apiKey === 'string'
    && value.apiKey.length > 0
    && value.apiKey === value.apiKey.trim()
  ) {
    return { kind: 'ok', apiKey: value.apiKey }
  }
  return { kind: 'invalid', removed: removeItem(storage, SAVED_API_KEY_STORAGE_KEY) }
}

export function writeSavedApiKey(apiKey: string, storage: Storage | null = getStorage()): boolean {
  return writeJSON(storage, SAVED_API_KEY_STORAGE_KEY, { version: STORAGE_VERSION, apiKey })
}

export function removeSavedApiKey(storage: Storage | null = getStorage()): boolean {
  return removeItem(storage, SAVED_API_KEY_STORAGE_KEY)
}
