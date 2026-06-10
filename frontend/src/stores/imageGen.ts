/**
 * Image Generation Store
 *
 * 图片生成页的表单、原图列表、生成结果与历史记录全部放在 store 里，
 * 生成请求也由 store 发起：路由切换卸载视图后状态不丢，进行中的生成
 * 继续运行，完成后通过全局 toast 通知用户。
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
// 必须从文件模块导入而非 '@/stores' index，否则会形成 index -> imageGen -> index 循环依赖。
import { useAppStore } from './app'

const HISTORY_STORAGE_KEY = 'image-gen-history-v1'
const MAX_HISTORY_ITEMS = 20
const DEFAULT_MODEL = 'gpt-image-2'
const IMAGE_GENERATION_ENDPOINT = import.meta.env.VITE_IMAGE_GENERATION_ENDPOINT || '/v1/images/generations'
const IMAGE_EDIT_ENDPOINT = import.meta.env.VITE_IMAGE_EDIT_ENDPOINT || '/v1/images/edits'

export const MAX_SOURCE_IMAGES = 16
// 网关对单个 multipart 分片在 20MB 处静默截断（openai_images.go openAIImageMaxUploadPartSize），
// 超限文件会变成损坏图片而不是报错，这个前端上限不能放宽。
export const MAX_SOURCE_IMAGE_SIZE = 20 * 1024 * 1024
// 网关请求体上限默认 256MB（gateway.max_body_size），留余量避免 413。
export const MAX_TOTAL_SOURCE_SIZE = 100 * 1024 * 1024
// 官方 images API 的 n 取值范围是 1~10。
export const MAX_IMAGE_COUNT = 10
const ALLOWED_SOURCE_TYPES = ['image/png', 'image/jpeg', 'image/webp']

export type GenerationMode = 'generate' | 'edit'

export interface GeneratedImage {
  id: string
  src: string
  revisedPrompt: string
}

export interface HistoryItem {
  id: string
  createdAt: string
  prompt: string
  model: string
  size: string
  images: string[]
  mode?: GenerationMode
}

export interface SourceImage {
  id: string
  file: File
  previewUrl: string
}

interface ImagesResponseItem {
  b64_json?: string
  url?: string
  revised_prompt?: string
}

interface ImagesResponse {
  data?: ImagesResponseItem[]
  output_format?: string
}

export const useImageGenStore = defineStore('imageGen', () => {
  const appStore = useAppStore()

  // ==================== State ====================
  // API Key 只保留在内存里，页面承诺“不会保存 API Key”，不得写入任何持久化存储。
  const apiKey = ref('')
  const prompt = ref('')
  const size = ref('1024x1024')
  const count = ref(1)
  const generationMode = ref<GenerationMode>('generate')
  const isGenerating = ref(false)
  const errorMessage = ref('')
  const results = ref<GeneratedImage[]>([])
  const historyItems = ref<HistoryItem[]>([])
  const sourceImages = ref<SourceImage[]>([])

  const totalSourceSize = computed(() =>
    sourceImages.value.reduce((sum, item) => sum + item.file.size, 0)
  )

  // ==================== Generation ====================

  async function generate(): Promise<void> {
    if (isGenerating.value) {
      return
    }
    errorMessage.value = ''
    const key = apiKey.value.trim()
    const text = prompt.value.trim()
    const imageCount = normalizeCount(count.value)
    // 在第一个 await 之前快照所有请求参数：生成期间用户可能切页或继续编辑表单，
    // 异步尾部不能再读这些活引用。
    const mode = generationMode.value
    const requestSize = size.value
    const files = sourceImages.value.map((item) => item.file)

    if (!key) {
      errorMessage.value = '请输入 API Key'
      return
    }
    if (!text) {
      errorMessage.value = '请输入提示词'
      return
    }
    if (mode === 'edit' && files.length === 0) {
      errorMessage.value = '请先上传或粘贴原图'
      return
    }

    isGenerating.value = true
    try {
      const response = mode === 'edit'
        ? await editImages(key, text, imageCount, requestSize, files)
        : await createImages(key, text, imageCount, requestSize)

      const payload = await response.json().catch(() => ({}))
      if (!response.ok) {
        throw new Error(extractErrorMessage(payload, response.status))
      }

      const images = normalizeImagesResponse(payload as ImagesResponse)
      if (images.length === 0) {
        throw new Error('接口未返回可展示的图片')
      }

      results.value = images
      saveHistory({
        id: createID(),
        createdAt: new Date().toISOString(),
        prompt: text,
        model: DEFAULT_MODEL,
        size: requestSize,
        images: images.map((item) => item.src),
        mode,
      })
      appStore.showSuccess(`${mode === 'edit' ? '图改图' : '图片生成'}完成，共 ${images.length} 张`)
    } catch (error) {
      const message = error instanceof Error ? error.message : '生成失败，请稍后重试'
      errorMessage.value = message
      appStore.showError(message)
    } finally {
      isGenerating.value = false
    }
  }

  function createImages(key: string, text: string, imageCount: number, requestSize: string): Promise<Response> {
    return fetch(IMAGE_GENERATION_ENDPOINT, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${key}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        model: DEFAULT_MODEL,
        prompt: text,
        size: requestSize,
        n: imageCount,
        response_format: 'b64_json',
      }),
    })
  }

  function editImages(
    key: string,
    text: string,
    imageCount: number,
    requestSize: string,
    files: File[]
  ): Promise<Response> {
    const formData = new FormData()
    formData.set('model', DEFAULT_MODEL)
    formData.set('prompt', text)
    formData.set('size', requestSize)
    formData.set('n', String(imageCount))
    formData.set('response_format', 'b64_json')
    // 不传 filename 参数：File 自带 name，显式传参会按规范重新包一层 File 对象。
    for (const file of files) {
      formData.append('image[]', file)
    }

    return fetch(IMAGE_EDIT_ENDPOINT, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${key}`,
      },
      body: formData,
    })
  }

  // ==================== Source images ====================

  function addSourceImages(files: File[]): void {
    if (files.length === 0) {
      return
    }
    let rejectReason = ''
    for (const file of files) {
      if (!ALLOWED_SOURCE_TYPES.includes(file.type)) {
        rejectReason = rejectReason || '仅支持 png / jpg / webp 图片'
        continue
      }
      if (file.size > MAX_SOURCE_IMAGE_SIZE) {
        rejectReason = rejectReason || '原图单张不能超过 20MB'
        continue
      }
      if (sourceImages.value.length >= MAX_SOURCE_IMAGES) {
        rejectReason = rejectReason || `最多选择 ${MAX_SOURCE_IMAGES} 张原图`
        continue
      }
      if (totalSourceSize.value + file.size > MAX_TOTAL_SOURCE_SIZE) {
        rejectReason = rejectReason || '原图总大小不能超过 100MB'
        continue
      }
      sourceImages.value.push({
        id: createID(),
        file,
        previewUrl: URL.createObjectURL(file),
      })
    }
    errorMessage.value = rejectReason
  }

  function removeSourceImage(id: string): void {
    const item = sourceImages.value.find((entry) => entry.id === id)
    if (!item) {
      return
    }
    URL.revokeObjectURL(item.previewUrl)
    sourceImages.value = sourceImages.value.filter((entry) => entry.id !== id)
  }

  // objectURL 只在移除/清空时 revoke，组件卸载时不得 revoke——预览要在路由切换后继续可用。
  function clearSourceImages(): void {
    for (const item of sourceImages.value) {
      URL.revokeObjectURL(item.previewUrl)
    }
    sourceImages.value = []
  }

  // ==================== History ====================

  function loadHistory(): void {
    try {
      const raw = localStorage.getItem(HISTORY_STORAGE_KEY)
      if (!raw) {
        historyItems.value = []
        return
      }
      const parsed = JSON.parse(raw)
      if (!Array.isArray(parsed)) {
        historyItems.value = []
        return
      }
      historyItems.value = parsed
        .filter(isHistoryItem)
        .slice(0, MAX_HISTORY_ITEMS)
    } catch {
      historyItems.value = []
    }
  }

  function saveHistory(item: HistoryItem): void {
    const next = [item, ...historyItems.value].slice(0, MAX_HISTORY_ITEMS)
    historyItems.value = next
    try {
      localStorage.setItem(HISTORY_STORAGE_KEY, JSON.stringify(next))
    } catch {
      // Generation result should remain usable even if browser storage is unavailable.
    }
  }

  function deleteHistoryItem(id: string): void {
    historyItems.value = historyItems.value.filter((item) => item.id !== id)
    try {
      localStorage.setItem(HISTORY_STORAGE_KEY, JSON.stringify(historyItems.value))
    } catch {
      // ignore localStorage failures
    }
  }

  function clearHistory(): void {
    historyItems.value = []
    try {
      localStorage.removeItem(HISTORY_STORAGE_KEY)
    } catch {
      // ignore localStorage failures
    }
  }

  function restoreHistory(item: HistoryItem): void {
    prompt.value = item.prompt
    size.value = item.size
    generationMode.value = item.mode || 'generate'
    // 历史记录不保存原图，restore 后清掉残留的上传，避免切回 edit 时出现
    // 与当前历史不匹配的上次会话的原图。
    clearSourceImages()
    results.value = item.images.map((src, index) => ({
      id: `${item.id}-${index}`,
      src,
      revisedPrompt: '',
    }))
  }

  loadHistory()

  return {
    // State
    apiKey,
    prompt,
    size,
    count,
    generationMode,
    isGenerating,
    errorMessage,
    results,
    historyItems,
    sourceImages,
    totalSourceSize,
    // Actions
    generate,
    addSourceImages,
    removeSourceImage,
    clearSourceImages,
    loadHistory,
    deleteHistoryItem,
    clearHistory,
    restoreHistory,
  }
})

function normalizeImagesResponse(payload: ImagesResponse): GeneratedImage[] {
  const outputFormat = payload.output_format || 'png'
  return (payload.data || [])
    .map((item, index) => {
      const src = imageItemToSource(item, outputFormat)
      if (!src) {
        return null
      }
      return {
        id: `${Date.now()}-${index}`,
        src,
        revisedPrompt: item.revised_prompt || '',
      }
    })
    .filter((item): item is GeneratedImage => item !== null)
}

function imageItemToSource(item: ImagesResponseItem, outputFormat: string): string {
  if (item.url) {
    return item.url
  }
  if (!item.b64_json) {
    return ''
  }
  return `data:${mimeTypeForOutputFormat(outputFormat)};base64,${item.b64_json}`
}

function mimeTypeForOutputFormat(format: string): string {
  switch (format.toLowerCase()) {
    case 'jpg':
    case 'jpeg':
      return 'image/jpeg'
    case 'webp':
      return 'image/webp'
    case 'png':
    default:
      return 'image/png'
  }
}

function normalizeCount(value: number): number {
  if (!Number.isFinite(value)) {
    return 1
  }
  return Math.min(MAX_IMAGE_COUNT, Math.max(1, Math.trunc(value)))
}

function extractErrorMessage(payload: unknown, status: number): string {
  if (payload && typeof payload === 'object') {
    const record = payload as Record<string, any>
    const nested = record.error
    if (nested && typeof nested === 'object' && typeof nested.message === 'string') {
      return nested.message
    }
    if (typeof record.message === 'string') {
      return record.message
    }
  }
  return `生成失败，网关返回 ${status}`
}

function isHistoryItem(value: unknown): value is HistoryItem {
  if (!value || typeof value !== 'object') {
    return false
  }
  const item = value as Partial<HistoryItem>
  return typeof item.id === 'string'
    && typeof item.createdAt === 'string'
    && typeof item.prompt === 'string'
    && typeof item.model === 'string'
    && typeof item.size === 'string'
    && (item.mode === undefined || item.mode === 'generate' || item.mode === 'edit')
    && Array.isArray(item.images)
    && item.images.every((src) => typeof src === 'string')
}

function createID(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID()
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`
}
