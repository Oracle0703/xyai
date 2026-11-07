/**
 * 图片生成页的尺寸选项：内置预设 + 用户自定义尺寸。
 *
 * 自定义尺寸只保存在浏览器 localStorage（CUSTOM_SIZES_STORAGE_KEY），不含 Key 等敏感信息。
 * 网关不校验尺寸（只按最长边分 1K/2K/4K 计费档，见 backend/internal/service/image_billing_size.go），
 * 这里只做基本格式与范围校验，能否出图由上游模型决定。
 */

export const CUSTOM_SIZES_STORAGE_KEY = 'image-gen-custom-sizes-v1'
const STORAGE_VERSION = 1
export const MAX_CUSTOM_SIZES = 20
export const MIN_IMAGE_EDGE = 64
export const MAX_IMAGE_EDGE = 4096
export const DEFAULT_IMAGE_SIZE = '1024x1024'

export const PRESET_IMAGE_SIZES: readonly string[] = [
  '1024x1024',
  '1536x1024',
  '1024x1536',
  '1792x1024',
  '1024x1792',
]

export type ImageSizeParseResult = { ok: true; size: string } | { ok: false; error: string }

function isValidEdge(value: number): boolean {
  return Number.isInteger(value) && value >= MIN_IMAGE_EDGE && value <= MAX_IMAGE_EDGE
}

// 宽、高来自数字输入框（v-model.number 留空时为 ''），统一规范成 "宽x高"。
export function parseImageSize(width: unknown, height: unknown): ImageSizeParseResult {
  const toNumber = (value: unknown) =>
    typeof value === 'number' ? value : typeof value === 'string' && value.trim() !== '' ? Number(value) : NaN
  const w = toNumber(width)
  const h = toNumber(height)
  if (Number.isNaN(w) || Number.isNaN(h)) {
    return { ok: false, error: '请填写宽和高' }
  }
  if (!isValidEdge(w) || !isValidEdge(h)) {
    return { ok: false, error: `宽和高须为 ${MIN_IMAGE_EDGE}~${MAX_IMAGE_EDGE} 之间的整数` }
  }
  return { ok: true, size: `${w}x${h}` }
}

// 存储里读回的值必须是本页能生成的规范格式，否则丢弃。
export function isCustomImageSize(value: unknown): value is string {
  if (typeof value !== 'string') {
    return false
  }
  const match = /^(\d{1,5})x(\d{1,5})$/.exec(value)
  if (!match || match[1].startsWith('0') || match[2].startsWith('0')) {
    return false
  }
  return isValidEdge(Number(match[1])) && isValidEdge(Number(match[2]))
}

export function isPresetImageSize(value: string): boolean {
  return PRESET_IMAGE_SIZES.includes(value)
}

export function formatImageSizeLabel(size: string): string {
  return size.replace('x', ' x ')
}

// 最新添加的在前；去重、剔除预设与非法值，最多 MAX_CUSTOM_SIZES 个。
export function normalizeCustomSizes(values: unknown[]): string[] {
  const result: string[] = []
  for (const value of values) {
    if (isCustomImageSize(value) && !isPresetImageSize(value) && !result.includes(value)) {
      result.push(value)
    }
    if (result.length >= MAX_CUSTOM_SIZES) {
      break
    }
  }
  return result
}

export function readCustomSizes(): string[] {
  try {
    const raw = localStorage.getItem(CUSTOM_SIZES_STORAGE_KEY)
    if (!raw) {
      return []
    }
    const parsed = JSON.parse(raw)
    if (!parsed || parsed.version !== STORAGE_VERSION || !Array.isArray(parsed.sizes)) {
      return []
    }
    return normalizeCustomSizes(parsed.sizes)
  } catch {
    return []
  }
}

export function writeCustomSizes(sizes: string[]): boolean {
  try {
    if (sizes.length === 0) {
      localStorage.removeItem(CUSTOM_SIZES_STORAGE_KEY)
    } else {
      localStorage.setItem(CUSTOM_SIZES_STORAGE_KEY, JSON.stringify({ version: STORAGE_VERSION, sizes }))
    }
    return true
  } catch {
    return false
  }
}
