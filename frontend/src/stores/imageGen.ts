/**
 * Image Generation Store
 *
 * 图片生成页的表单、原图列表、生成结果与历史记录全部放在 store 里，
 * 生成请求也由 store 发起：路由切换卸载视图后状态不丢，进行中的生成
 * 继续运行，完成后通过全局 toast 通知用户。
 *
 * 模型不写死：按当前 Key 请求 `/v1/models` 发现图片模型，也允许手动输入。
 * Key 只在用户回车或点击“加载模型”时发往目录端点；粘贴、失焦、输入（含密码管理器
 * 自动填充）都不发请求、不改动已保存的 Key。勾选“记住”后，Key 加载模型成功才写入 localStorage。
 * 设计见 docs/features/image-gen-design-cn.md。
 */
import { defineStore } from 'pinia'
import { computed, onScopeDispose, ref } from 'vue'
// 必须从文件模块导入而非 '@/stores' index，否则会形成 index -> imageGen -> index 循环依赖。
import { useAppStore } from './app'
import {
  SAVED_API_KEY_STORAGE_KEY,
  endpointsShareOrigin,
  isValidModelId,
  parseImageModelCatalog,
  readModelPreference,
  readSavedApiKey,
  redactCredential,
  removeModelPreference,
  removeSavedApiKey,
  writeModelPreference,
  writeSavedApiKey,
  type ModelPreference,
  type ModelSource,
} from '@/utils/imageGenModels'

const HISTORY_STORAGE_KEY = 'image-gen-history-v1'
const MAX_HISTORY_ITEMS = 20
const IMAGE_GENERATION_ENDPOINT = import.meta.env.VITE_IMAGE_GENERATION_ENDPOINT || '/v1/images/generations'
const IMAGE_EDIT_ENDPOINT = import.meta.env.VITE_IMAGE_EDIT_ENDPOINT || '/v1/images/edits'
const IMAGE_MODELS_ENDPOINT = import.meta.env.VITE_IMAGE_MODELS_ENDPOINT || '/v1/models'
const MODEL_DISCOVERY_TIMEOUT_MS = 10_000

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
export type ModelDiscoveryStatus = 'idle' | 'loading' | 'ready' | 'empty' | 'error' | 'disabled'

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
  // API Key 默认只在内存里；仅当用户勾选“记住”时写入 SAVED_API_KEY_STORAGE_KEY，
  // 不得写入历史或其他存储。用户编辑走 updateApiKey，恢复/清除等程序赋值直接改 ref。
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
  // 生成中的请求模型，用于标注当前任务（表单模型可能已被改动）。
  const activeRequestModel = ref('')

  // 模型选择：modelConfirmed 表示当前列表模型已被当前 Key 的目录确认；手动模型不参与确认。
  const model = ref('')
  const modelSource = ref<ModelSource>('list')
  const modelConfirmed = ref(false)
  const modelNotice = ref('')
  const preferredModel = ref<ModelPreference | null>(null)
  // 模型偏好是否确实在存储中（写入失败时只存在内存，不显示“清除已保存配置”）
  const preferenceStored = ref(false)

  // 模型发现：三个端点不同源时停用，避免把 Key 发给生图请求不会访问的 origin。
  const discoveryEnabled = endpointsShareOrigin(
    [IMAGE_MODELS_ENDPOINT, IMAGE_GENERATION_ENDPOINT, IMAGE_EDIT_ENDPOINT],
    window.location.origin
  )
  const discoveryStatus = ref<ModelDiscoveryStatus>(discoveryEnabled ? 'idle' : 'disabled')
  const discoveryError = ref('')
  // 同 Key 刷新失败时保留的旧目录
  const discoveryStale = ref(false)
  const modelCandidates = ref<string[]>([])

  // 记住 Key：rememberKey 为勾选状态；savedKey 为本页写入或恢复的已保存 Key（不导出），
  // 输入框改动后两者可以不同，直到新 Key 加载模型成功后覆盖。
  const rememberKey = ref(false)
  const savedKey = ref('')
  const keySaved = computed(() => savedKey.value !== '')
  const keySavedIsCurrent = computed(() => keySaved.value && savedKey.value === apiKey.value)
  // 删除已保存 Key 失败：浏览器可能仍残留凭据，保留警告与重试入口，直到删除或覆盖成功。
  const keyRemovalFailed = ref(false)
  const keyStorageMessage = ref('')

  // 请求时序（非响应式）：迟到的响应按序号与 Key 快照丢弃，不能只依赖 abort。
  let discoverySeq = 0
  let discoveryController: AbortController | null = null
  let inflightKey = ''
  // 当前 Key 最近一次获得结构合法的成功目录响应；之后任何失败或 Key 变化都会清除
  let keyVerified = false

  const totalSourceSize = computed(() =>
    sourceImages.value.reduce((sum, item) => sum + item.file.size, 0)
  )
  const hasSavedConfig = computed(() => keySaved.value || preferenceStored.value || keyRemovalFailed.value)

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
    const requestModel = model.value
    const requestModelSource = modelSource.value

    if (!key) {
      errorMessage.value = '请输入 API Key'
      return
    }
    if (!requestModel) {
      errorMessage.value = '请选择或手动输入模型'
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

    saveModelPreference({ model: requestModel, source: requestModelSource })
    activeRequestModel.value = requestModel
    isGenerating.value = true
    try {
      const response = mode === 'edit'
        ? await editImages(key, requestModel, text, imageCount, requestSize, files)
        : await createImages(key, requestModel, text, imageCount, requestSize)

      const payload = await response.json().catch(() => ({}))
      if (!response.ok) {
        throw new Error(extractErrorMessage(payload, `生成失败，网关返回 ${response.status}`))
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
        model: requestModel,
        size: requestSize,
        images: images.map((item) => item.src),
        mode,
      })
      appStore.showSuccess(`${mode === 'edit' ? '图改图' : '图片生成'}完成，共 ${images.length} 张`)
    } catch (error) {
      // 用发起请求时的 Key 快照脱敏：服务端文案可能回显凭据，且此时输入框可能已换成别的 Key。
      const message = redactCredential(error instanceof Error ? error.message : '生成失败，请稍后重试', key)
      errorMessage.value = message
      appStore.showError(message)
    } finally {
      isGenerating.value = false
      activeRequestModel.value = ''
    }
  }

  function createImages(
    key: string,
    requestModel: string,
    text: string,
    imageCount: number,
    requestSize: string
  ): Promise<Response> {
    return fetch(IMAGE_GENERATION_ENDPOINT, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${key}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        model: requestModel,
        prompt: text,
        size: requestSize,
        n: imageCount,
        response_format: 'b64_json',
      }),
    })
  }

  function editImages(
    key: string,
    requestModel: string,
    text: string,
    imageCount: number,
    requestSize: string,
    files: File[]
  ): Promise<Response> {
    const formData = new FormData()
    formData.set('model', requestModel)
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

  // ==================== API Key ====================

  // 用户编辑 Key（含密码管理器自动填充触发的输入事件）：只改内存中的当前 Key，
  // 不发请求，也不删除已保存的 Key；已保存的 Key 只在新 Key 加载模型成功后被覆盖。
  function updateApiKey(next: string): void {
    if (next === apiKey.value) {
      return
    }
    apiKey.value = next
    resetDiscoveryForKeyChange()
    keyStorageMessage.value = ''
  }

  // 勾选只保存已验证的当前 Key，不发请求；未验证的 Key 待用户回车或点击“加载模型”成功后保存。
  function setRememberKey(checked: boolean): void {
    keyStorageMessage.value = ''
    if (!checked) {
      rememberKey.value = false
      removeSavedKey()
      return
    }
    rememberKey.value = true
    const key = apiKey.value
    // 自动发现停用时没有目录可校验，勾选即视为确认。
    if (key && (!discoveryEnabled || keyVerified)) {
      persistApiKey(key)
    }
  }

  function persistApiKey(key: string): void {
    if (!key) {
      return
    }
    if (writeSavedApiKey(key)) {
      savedKey.value = key
      keyRemovalFailed.value = false
      keyStorageMessage.value = ''
      return
    }
    // 写入失败退回未记住；覆盖失败时旧 Key 仍在存储中，按取消记住处理一并删除。
    rememberKey.value = false
    removeSavedKey()
    keyStorageMessage.value = '无法保存，当前仅本次页面有效'
  }

  // 删除已保存的 Key。本页已知存在记录（或上次删除失败）时，失败要保留“可能残留”状态；
  // 从未保存过时只是尽力删除，存储不可用也不误报残留。
  function removeSavedKey(): void {
    const mayExist = keySaved.value || keyRemovalFailed.value
    savedKey.value = ''
    const removed = removeSavedApiKey()
    if (mayExist) {
      keyRemovalFailed.value = !removed
    }
  }

  function retrySavedKeyRemoval(): void {
    if (removeSavedApiKey()) {
      keyRemovalFailed.value = false
      appStore.showSuccess('已清除浏览器中保存的 API Key')
      return
    }
    appStore.showError('仍未能清除，请检查浏览器存储设置后重试')
  }

  // 删除已保存的 Key 与模型偏好，并清空当前 Key 和模型；提示词、结果与历史不动，
  // 已提交的生图任务继续完成。
  function clearSavedConfig(): void {
    removeSavedKey()
    const preferenceRemoved = removeModelPreference()
    apiKey.value = ''
    resetDiscoveryForKeyChange()
    model.value = ''
    modelSource.value = 'list'
    modelConfirmed.value = false
    preferredModel.value = null
    preferenceStored.value = !preferenceRemoved
    rememberKey.value = false
    keyStorageMessage.value = preferenceRemoved ? '' : '未能清除浏览器中保存的模型偏好'
    if (!keyRemovalFailed.value && preferenceRemoved) {
      appStore.showSuccess('已清除保存的 API Key 和模型')
    }
  }

  // 其他标签页改动已保存的 Key 时，本页退出记住状态且不自动导入，防止把已清除的 Key 写回。
  function handleStorageEvent(event: StorageEvent): void {
    if (event.key !== SAVED_API_KEY_STORAGE_KEY && event.key !== null) {
      return
    }
    // 记录已被其他标签页删除或覆盖，本页此前删除失败留下的旧值已不存在。
    keyRemovalFailed.value = false
    if (!rememberKey.value) {
      return
    }
    // 本页的写入可能与其他标签页的删除/改写交错（迟到的目录成功先于本事件写入）：
    // 退出记住时，存储里若仍是本页的值则一并删除，避免留下两边都不认的 Key。
    const ownKey = savedKey.value
    rememberKey.value = false
    savedKey.value = ''
    const stored = readSavedApiKey()
    if (ownKey && stored.kind === 'ok' && stored.apiKey === ownKey) {
      keyRemovalFailed.value = !removeSavedApiKey()
    }
    keyStorageMessage.value = '其他标签页修改了已保存的 API Key，本页已退出记住状态'
  }

  // ==================== Model selection ====================

  function selectModel(id: string): void {
    if (!id) {
      return
    }
    model.value = id
    modelSource.value = 'list'
    modelConfirmed.value = modelCandidates.value.includes(id) && !discoveryStale.value
    modelNotice.value = ''
    saveModelPreference({ model: id, source: 'list' })
  }

  // 手动模型：不限前缀，能否生图以接口结果为准。返回 false 表示输入不合法未采用。
  function setManualModel(raw: string): boolean {
    const id = raw.trim()
    if (!isValidModelId(id)) {
      return false
    }
    model.value = id
    modelSource.value = 'manual'
    modelConfirmed.value = false
    modelNotice.value = ''
    saveModelPreference({ model: id, source: 'manual' })
    return true
  }

  // 模型偏好不敏感，写入失败静默降级。
  function saveModelPreference(preference: ModelPreference): void {
    preferredModel.value = preference
    if (writeModelPreference(preference)) {
      preferenceStored.value = true
    }
  }

  // ==================== Model discovery ====================

  // 加载模型：只由用户显式操作（Key 框回车、“加载模型”按钮）调用，把当前 Key 发往目录端点。
  function triggerModelDiscovery(): void {
    const key = apiKey.value
    if (!key) {
      return
    }
    if (!discoveryEnabled) {
      // 无目录可校验 Key，回车即视为确认，按记住状态直接保存。
      if (rememberKey.value && savedKey.value !== key) {
        persistApiKey(key)
      }
      return
    }
    if (inflightKey === key) {
      return
    }
    void loadModels(key)
  }

  async function loadModels(key: string): Promise<void> {
    cancelDiscovery()
    const seq = discoverySeq
    const controller = new AbortController()
    discoveryController = controller
    inflightKey = key
    discoveryStatus.value = 'loading'
    discoveryError.value = ''
    let timedOut = false
    const timer = setTimeout(() => {
      timedOut = true
      controller.abort()
    }, MODEL_DISCOVERY_TIMEOUT_MS)
    const isCurrent = () => seq === discoverySeq && apiKey.value === key

    try {
      // 独立 fetch：不带登录 JWT，不带 client_version（否则会转入 Codex manifest 分支）。
      const response = await fetch(IMAGE_MODELS_ENDPOINT, {
        method: 'GET',
        headers: {
          Authorization: `Bearer ${key}`,
          Accept: 'application/json',
        },
        cache: 'no-store',
        redirect: 'error',
        signal: controller.signal,
      })
      const payload = await response.json().catch(() => undefined)
      if (!isCurrent()) {
        return
      }
      // 超时可能发生在读取响应体期间，json() 的失败已被吞掉，不能误报为格式异常。
      if (timedOut) {
        applyDiscoveryFailure('模型列表加载超时，可重试或手动输入模型', false)
        return
      }
      if (!response.ok) {
        applyDiscoveryHttpError(response, payload, key)
        return
      }
      const candidates = parseImageModelCatalog(payload)
      if (candidates === null) {
        applyDiscoveryFailure('模型列表响应格式异常，可重试或手动输入模型', false)
        return
      }
      applyCatalog(candidates)
      keyVerified = true
      // 覆盖保存前再读最新勾选状态：其他标签页清除或用户取消记住后，迟到的成功不能写回。
      if (rememberKey.value && savedKey.value !== key) {
        persistApiKey(key)
      }
    } catch {
      if (!isCurrent()) {
        return
      }
      applyDiscoveryFailure(
        timedOut ? '模型列表加载超时，可重试或手动输入模型' : '模型列表加载失败，可重试或手动输入模型',
        false
      )
    } finally {
      clearTimeout(timer)
      if (seq === discoverySeq) {
        discoveryController = null
        inflightKey = ''
      }
    }
  }

  function cancelDiscovery(): void {
    discoverySeq++
    discoveryController?.abort()
    discoveryController = null
    inflightKey = ''
  }

  function resetDiscoveryForKeyChange(): void {
    cancelDiscovery()
    keyVerified = false
    modelCandidates.value = []
    discoveryError.value = ''
    discoveryStale.value = false
    modelNotice.value = ''
    if (discoveryEnabled) {
      discoveryStatus.value = 'idle'
    }
    if (modelSource.value === 'list') {
      modelConfirmed.value = false
    }
  }

  // 目录成功返回：手动模型不受影响；列表模型不在目录时清除并提示，本次不自动改选；
  // 没有当前选择时依次取目录中的模型偏好、唯一候选。
  function applyCatalog(candidates: string[]): void {
    modelCandidates.value = candidates
    discoveryStatus.value = candidates.length > 0 ? 'ready' : 'empty'
    discoveryError.value = ''
    discoveryStale.value = false
    modelNotice.value = ''
    if (model.value && modelSource.value === 'manual') {
      return
    }
    if (model.value) {
      if (candidates.includes(model.value)) {
        modelConfirmed.value = true
        return
      }
      modelNotice.value = `原模型 ${model.value} 不在当前 Key 的目录，请重新选择`
      model.value = ''
      modelConfirmed.value = false
      return
    }
    const preferred = preferredModel.value
    let next = ''
    if (preferred && candidates.includes(preferred.model)) {
      next = preferred.model
    } else if (candidates.length === 1) {
      next = candidates[0]
    }
    if (next) {
      model.value = next
      modelSource.value = 'list'
      modelConfirmed.value = true
    }
  }

  // 目录请求失败不清除当前选择（以生成接口结果为准），只把列表模型标记为未确认。
  function applyDiscoveryFailure(message: string, clearCandidates: boolean): void {
    discoveryStatus.value = 'error'
    discoveryError.value = message
    keyVerified = false
    if (clearCandidates) {
      modelCandidates.value = []
    }
    discoveryStale.value = modelCandidates.value.length > 0
    if (modelSource.value === 'list') {
      modelConfirmed.value = false
    }
  }

  // /v1/models 与生图经过同一套鉴权/计费前置检查，403/429 同样会拒绝生图，展示服务端信息；
  // 展示前用请求 Key 快照脱敏。
  function applyDiscoveryHttpError(response: Response, payload: unknown, requestKey: string): void {
    const status = response.status
    if (status >= 500) {
      applyDiscoveryFailure(`模型列表加载失败（HTTP ${status}），可重试或手动输入模型`, false)
      return
    }
    const message = redactCredential(extractErrorMessage(payload, `HTTP ${status}`), requestKey)
    if (status === 401) {
      applyDiscoveryFailure(`模型列表加载失败：${message}`, true)
      if (rememberKey.value && savedKey.value !== requestKey) {
        keyStorageMessage.value = 'Key 无效，未保存'
      }
      return
    }
    if (status === 429 && extractErrorCode(payload) === 'INVALID_AUTH_RATE_LIMITED') {
      const retryAfter = Number.parseInt(response.headers.get('Retry-After') ?? '', 10)
      const wait = retryAfter > 0 ? `请 ${retryAfter} 秒后再试` : '请稍后再试'
      applyDiscoveryFailure(`${message}，${wait}`, false)
      return
    }
    const hint = status === 404 || status === 405 ? '，请检查模型端点配置' : ''
    applyDiscoveryFailure(`模型列表加载失败：${message}${hint}`, false)
  }

  function restorePreferences(): void {
    const preference = readModelPreference()
    if (preference) {
      preferredModel.value = preference
      preferenceStored.value = true
      model.value = preference.model
      modelSource.value = preference.source
    }
    const saved = readSavedApiKey()
    if (saved.kind === 'invalid') {
      keyStorageMessage.value = '已保存的 API Key 配置无效，已忽略'
      keyRemovalFailed.value = !saved.removed
      return
    }
    if (saved.kind === 'ok') {
      // 显式记住过的 Key 按约定恢复并加载模型；程序赋值，不走 updateApiKey。
      apiKey.value = saved.apiKey
      rememberKey.value = true
      savedKey.value = saved.apiKey
      if (discoveryEnabled) {
        void loadModels(saved.apiKey)
      }
    }
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
  restorePreferences()
  window.addEventListener('storage', handleStorageEvent)
  onScopeDispose(() => {
    window.removeEventListener('storage', handleStorageEvent)
    cancelDiscovery()
  })

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
    activeRequestModel,
    model,
    modelSource,
    modelConfirmed,
    modelNotice,
    modelCandidates,
    discoveryStatus,
    discoveryError,
    discoveryStale,
    rememberKey,
    keySaved,
    keySavedIsCurrent,
    keyRemovalFailed,
    keyStorageMessage,
    hasSavedConfig,
    // Actions
    generate,
    updateApiKey,
    setRememberKey,
    retrySavedKeyRemoval,
    clearSavedConfig,
    selectModel,
    setManualModel,
    triggerModelDiscovery,
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

// 兼容 OpenAI 风格 { error: { message } } 与鉴权中间件 { code, message } 两种错误体。
function extractErrorMessage(payload: unknown, fallback: string): string {
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
  return fallback
}

function extractErrorCode(payload: unknown): string {
  if (payload && typeof payload === 'object') {
    const record = payload as Record<string, any>
    if (typeof record.code === 'string') {
      return record.code
    }
    const nested = record.error
    if (nested && typeof nested === 'object' && typeof nested.code === 'string') {
      return nested.code
    }
  }
  return ''
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
