<template>
  <component :is="wrapperComponent">
    <header v-if="!embedded" class="border-b border-gray-200 bg-white/90 px-6 py-4 dark:border-dark-800 dark:bg-dark-950/90">
      <nav class="mx-auto flex max-w-7xl items-center justify-between gap-4">
        <router-link to="/home" class="flex min-w-0 items-center gap-3">
          <div class="h-9 w-9 overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <div class="min-w-0">
            <p class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ siteName }}</p>
            <p class="text-xs text-gray-500 dark:text-dark-400">图片生成工具</p>
          </div>
        </router-link>
        <a
          v-if="docUrl"
          :href="docUrl"
          target="_blank"
          rel="noopener noreferrer"
          class="btn btn-secondary btn-sm"
        >
          <Icon name="book" size="sm" />
          <span>文档</span>
        </a>
      </nav>
    </header>

    <div :class="mainGridClass">
      <section class="space-y-4">
        <div class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-800 dark:bg-dark-900">
          <div class="mb-5 flex items-start justify-between gap-4">
            <div>
              <h1 class="text-xl font-semibold text-gray-950 dark:text-white">{{ formTitle }}</h1>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ formSubtitle }}</p>
            </div>
            <span class="rounded-md bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300">gpt-image-2</span>
          </div>

          <form class="space-y-4" @submit.prevent="generate">
            <div>
              <label class="input-label" for="image-gen-key">API Key</label>
              <div class="relative">
                <input
                  id="image-gen-key"
                  v-model.trim="apiKey"
                  data-test="api-key-input"
                  :type="showKey ? 'text' : 'password'"
                  class="input pr-11"
                  autocomplete="off"
                  placeholder="sk-..."
                />
                <button
                  type="button"
                  class="absolute right-2 top-1/2 inline-flex h-8 w-8 -translate-y-1/2 items-center justify-center rounded-md text-gray-500 hover:bg-gray-100 hover:text-gray-800 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
                  :title="showKey ? '隐藏 API Key' : '显示 API Key'"
                  @click="showKey = !showKey"
                >
                  <Icon :name="showKey ? 'eyeOff' : 'eye'" size="sm" />
                </button>
              </div>
              <p class="input-hint">页面不会保存 API Key；请求仍走现有鉴权、审核和计费。</p>
            </div>

            <div>
              <label class="input-label">生成方式</label>
              <div class="grid grid-cols-2 gap-2 rounded-lg border border-gray-200 bg-gray-50 p-1 dark:border-dark-800 dark:bg-dark-950">
                <button
                  data-test="mode-generate-button"
                  type="button"
                  class="inline-flex min-h-10 items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition"
                  :class="generationMode === 'generate'
                    ? 'bg-white text-gray-950 shadow-sm dark:bg-dark-800 dark:text-white'
                    : 'text-gray-600 hover:bg-white/70 dark:text-dark-300 dark:hover:bg-dark-800/70'"
                  @click="generationMode = 'generate'"
                >
                  <Icon name="sparkles" size="sm" />
                  <span>文生图</span>
                </button>
                <button
                  data-test="mode-edit-button"
                  type="button"
                  class="inline-flex min-h-10 items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition"
                  :class="generationMode === 'edit'
                    ? 'bg-white text-gray-950 shadow-sm dark:bg-dark-800 dark:text-white'
                    : 'text-gray-600 hover:bg-white/70 dark:text-dark-300 dark:hover:bg-dark-800/70'"
                  @click="generationMode = 'edit'"
                >
                  <Icon name="edit" size="sm" />
                  <span>图改图</span>
                </button>
              </div>
            </div>

            <div>
              <label class="input-label" for="image-gen-prompt">{{ generationMode === 'edit' ? '修改要求' : '提示词' }}</label>
              <textarea
                id="image-gen-prompt"
                v-model.trim="prompt"
                data-test="prompt-input"
                class="input min-h-36 resize-y leading-6"
                :placeholder="promptPlaceholder"
              />
            </div>

            <div v-if="generationMode === 'edit'">
              <label class="input-label">原图（可多张）</label>
              <div
                data-test="source-image-dropzone"
                class="rounded-lg border border-dashed border-gray-300 bg-gray-50 p-4 transition hover:border-blue-400 hover:bg-blue-50/60 dark:border-dark-700 dark:bg-dark-950 dark:hover:border-blue-500/70 dark:hover:bg-blue-950/20"
                :class="{ 'border-solid border-blue-300 bg-blue-50/50 dark:border-blue-600/70 dark:bg-blue-950/20': sourceImages.length > 0 }"
                @click="sourceImageInput?.click()"
                @dragover.prevent
                @drop.prevent="handleSourceImageDrop"
              >
                <input
                  ref="sourceImageInput"
                  data-test="source-image-input"
                  type="file"
                  accept="image/png,image/jpeg,image/webp"
                  multiple
                  class="hidden"
                  @change="handleSourceImageChange"
                />
                <div v-if="sourceImages.length > 0">
                  <div class="mb-2 flex items-center justify-between gap-3">
                    <p class="text-xs text-gray-500 dark:text-dark-400">
                      已选 {{ sourceImages.length }}/{{ MAX_SOURCE_IMAGES }} 张 · 共 {{ formatFileSize(totalSourceSize) }}
                    </p>
                    <button
                      type="button"
                      class="text-xs font-medium text-gray-500 hover:text-red-600 dark:text-dark-400 dark:hover:text-red-300"
                      @click.stop="clearSourceImages"
                    >
                      全部移除
                    </button>
                  </div>
                  <div class="grid grid-cols-3 gap-2 sm:grid-cols-4">
                    <div
                      v-for="image in sourceImages"
                      :key="image.id"
                      data-test="source-image-item"
                      class="relative"
                    >
                      <div class="aspect-square overflow-hidden rounded-md border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
                        <img :src="image.previewUrl" alt="" class="h-full w-full object-cover" />
                      </div>
                      <button
                        type="button"
                        data-test="source-image-remove"
                        class="absolute -right-1.5 -top-1.5 inline-flex h-5 w-5 items-center justify-center rounded-full bg-gray-900/80 text-white hover:bg-red-600 dark:bg-dark-700 dark:hover:bg-red-600"
                        title="移除这张原图"
                        @click.stop="removeSourceImage(image.id)"
                      >
                        <Icon name="x" size="xs" />
                      </button>
                      <p class="mt-1 truncate text-[11px] text-gray-500 dark:text-dark-400">{{ image.file.name }}</p>
                    </div>
                  </div>
                </div>
                <div v-else class="flex min-h-24 flex-col items-center justify-center text-center">
                  <Icon name="upload" size="xl" class="text-gray-400 dark:text-dark-500" />
                  <p class="mt-2 text-sm font-medium text-gray-700 dark:text-dark-200">点击选择、拖拽或 Ctrl+V 粘贴图片，可多选</p>
                  <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">最多 {{ MAX_SOURCE_IMAGES }} 张，单张不超过 20MB，总计不超过 100MB，支持 png / jpg / webp。</p>
                </div>
              </div>
              <p class="input-hint">页面会把这些原图按顺序作为 image[] 字段提交，第一张的细节保留度最高。</p>
            </div>

            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <label class="input-label" for="image-gen-size">尺寸</label>
                <select id="image-gen-size" v-model="size" data-test="size-select" class="input">
                  <option v-for="option in sizeOptions" :key="option.value" :value="option.value">
                    {{ option.label }}
                  </option>
                </select>
              </div>
              <div>
                <label class="input-label" for="image-gen-count">数量</label>
                <input
                  id="image-gen-count"
                  v-model.number="count"
                  data-test="count-input"
                  class="input"
                  type="number"
                  min="1"
                  max="10"
                />
              </div>
            </div>

            <button
              data-test="generate-button"
              type="button"
              class="btn btn-primary w-full"
              :disabled="isGenerating"
              @click="generate"
            >
              <Icon v-if="!isGenerating" name="sparkles" size="sm" />
              <span v-else class="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white"></span>
              <span>{{ isGenerating ? '处理中...' : submitLabel }}</span>
            </button>
          </form>
        </div>

        <div
          v-if="errorMessage"
          data-test="error-message"
          class="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300"
        >
          {{ errorMessage }}
        </div>
      </section>

      <section class="space-y-4">
        <div class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-800 dark:bg-dark-900">
          <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
            <div>
              <h2 class="text-base font-semibold text-gray-950 dark:text-white">生成结果</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">生成或修改成功后可预览、下载或复制图片 Data URL。</p>
            </div>
            <span v-if="results.length" class="text-xs text-gray-500 dark:text-dark-400">{{ results.length }} 张</span>
          </div>

          <div v-if="results.length" class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            <article
              v-for="image in results"
              :key="image.id"
              class="overflow-hidden rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-800 dark:bg-dark-950"
            >
              <div class="aspect-square bg-white dark:bg-dark-900">
                <img data-test="result-image" :src="image.src" :alt="image.revisedPrompt || prompt" class="h-full w-full object-contain" />
              </div>
              <div class="space-y-3 p-3">
                <p v-if="image.revisedPrompt" class="line-clamp-3 text-xs text-gray-600 dark:text-dark-300">
                  {{ image.revisedPrompt }}
                </p>
                <div class="flex gap-2">
                  <button class="btn btn-secondary btn-sm flex-1" type="button" @click="downloadImage(image.src, image.id)">
                    <Icon name="download" size="sm" />
                    <span>下载</span>
                  </button>
                  <button class="btn btn-secondary btn-sm flex-1" type="button" @click="copyText(image.src)">
                    <Icon name="copy" size="sm" />
                    <span>复制</span>
                  </button>
                </div>
              </div>
            </article>
          </div>

          <div v-else class="flex min-h-72 items-center justify-center rounded-lg border border-dashed border-gray-300 bg-gray-50 px-6 text-center dark:border-dark-700 dark:bg-dark-950">
            <div>
              <Icon name="sparkles" size="xl" class="mx-auto text-gray-400 dark:text-dark-500" />
              <p class="mt-3 text-sm font-medium text-gray-700 dark:text-dark-200">还没有结果</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ emptyResultHint }}</p>
            </div>
          </div>
        </div>

        <div class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-800 dark:bg-dark-900">
          <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
            <div>
              <h2 class="text-base font-semibold text-gray-950 dark:text-white">历史生图</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">仅保存在当前浏览器，最多保留最近 20 条。</p>
            </div>
            <button
              v-if="historyItems.length"
              class="btn btn-secondary btn-sm"
              type="button"
              @click="clearHistory"
            >
              <Icon name="trash" size="sm" />
              <span>清空</span>
            </button>
          </div>

          <div v-if="historyItems.length" class="grid grid-cols-1 items-start gap-3 md:grid-cols-2">
            <article
              v-for="item in historyItems"
              :key="item.id"
              data-test="history-item"
              class="rounded-lg border border-gray-200 p-3 dark:border-dark-800"
            >
              <div class="flex gap-3">
                <button type="button" class="grid shrink-0 grid-cols-2 gap-1" @click="restoreHistory(item)">
                  <img
                    v-for="src in item.images.slice(0, 4)"
                    :key="src"
                    :src="src"
                    alt=""
                    class="h-14 w-14 rounded-md border border-gray-200 object-cover dark:border-dark-700"
                  />
                </button>
                <div class="min-w-0 flex-1">
                  <div class="flex items-start justify-between gap-3">
                    <p class="line-clamp-2 text-sm font-medium text-gray-900 dark:text-white">{{ item.prompt }}</p>
                    <button
                      type="button"
                      class="rounded-md p-1.5 text-gray-400 hover:bg-gray-100 hover:text-red-600 dark:hover:bg-dark-800 dark:hover:text-red-300"
                      title="删除记录"
                      @click="deleteHistoryItem(item.id)"
                    >
                      <Icon name="x" size="sm" />
                    </button>
                  </div>
                  <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                    {{ formatTime(item.createdAt) }} · {{ modeLabel(item.mode) }} · {{ item.model }} · {{ item.size }} · {{ item.images.length }} 张
                  </p>
                </div>
              </div>
            </article>
          </div>

          <div v-else class="rounded-lg border border-dashed border-gray-300 p-6 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-dark-400">
            暂无历史记录
          </div>
        </div>
      </section>
    </div>
  </component>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onBeforeUnmount, onMounted, ref } from 'vue'
import { storeToRefs } from 'pinia'
// 从文件模块导入（而非 '@/stores' index），让测试可以只 mock app/auth 两个模块
// 而让 imageGen store 真实运行。
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useImageGenStore, MAX_SOURCE_IMAGES, type GenerationMode } from '@/stores/imageGen'
import Icon from '@/components/icons/Icon.vue'
import AppLayout from '@/components/layout/AppLayout.vue'

// 未登录访客（或后端模式下的非管理员）直接访问 /image-gen 时，渲染独立整页工具，
// 用这个轻量外壳提供整屏背景；登录用户走 AppLayout，保留左侧边栏 + 右侧内容区。
const StandaloneShell = defineComponent({
  name: 'ImageGenStandaloneShell',
  setup(_, { slots }) {
    return () =>
      h(
        'div',
        { class: 'min-h-screen bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-gray-100' },
        slots.default?.()
      )
  },
})

const appStore = useAppStore()
const authStore = useAuthStore()
// 表单、原图、结果、历史与生成逻辑都在 store 里：路由切换卸载本组件后状态不丢，
// 进行中的生成继续运行。
const imageGenStore = useImageGenStore()
const {
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
} = storeToRefs(imageGenStore)
const {
  generate,
  addSourceImages,
  removeSourceImage,
  clearSourceImages,
  deleteHistoryItem,
  clearHistory,
  restoreHistory,
} = imageGenStore

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')

// 是否嵌入平台布局展示。登录后的普通用户与管理员（即“我的账户”菜单的受众）都嵌入，
// 这样从侧边栏进入时停留在右侧内容区；后端模式下的非管理员（没有该菜单）与未登录访客
// 仍是独立整页工具，行为不变。
const embedded = computed(
  () => authStore.isAuthenticated && !(appStore.backendModeEnabled && !authStore.isAdmin)
)
const wrapperComponent = computed(() => (embedded.value ? AppLayout : StandaloneShell))
// 嵌入时铺满 AppLayout 内容区；独立页时居中限宽并自带内边距（与原整页布局一致）。
const mainGridClass = computed(() =>
  embedded.value
    ? 'grid gap-6 lg:grid-cols-[minmax(0,440px)_minmax(0,1fr)]'
    : 'mx-auto grid max-w-7xl gap-6 px-4 py-6 lg:grid-cols-[minmax(0,440px)_minmax(0,1fr)] lg:px-6'
)

const sizeOptions = [
  { value: '1024x1024', label: '1024 x 1024' },
  { value: '1536x1024', label: '1536 x 1024' },
  { value: '1024x1536', label: '1024 x 1536' },
  { value: '1792x1024', label: '1792 x 1024' },
  { value: '1024x1792', label: '1024 x 1792' },
]

const showKey = ref(false)
const sourceImageInput = ref<HTMLInputElement | null>(null)

const formTitle = computed(() => generationMode.value === 'edit' ? '图改图' : '文生图')
const formSubtitle = computed(() => generationMode.value === 'edit'
  ? '上传或粘贴原图，再用已验证 API Key 调用图片编辑网关。'
  : '粘贴已验证 API Key，直接调用图片生成网关。')
const promptPlaceholder = computed(() => generationMode.value === 'edit'
  ? '描述希望如何修改这些图片，例如：把背景改成白色，保持主体不变...'
  : '描述要生成的图片内容、风格、尺寸用途...')
const submitLabel = computed(() => generationMode.value === 'edit' ? '修改图片' : '生成图片')
const emptyResultHint = computed(() => generationMode.value === 'edit'
  ? '上传原图、填写 API Key 和修改要求后开始处理。'
  : '填写 API Key 和提示词后开始生成。')

onMounted(() => {
  window.addEventListener('paste', handleSourceImagePaste)
  if (!appStore.publicSettingsLoaded) {
    void appStore.fetchPublicSettings()
  }
})

onBeforeUnmount(() => {
  // 只移除本组件的监听器；原图预览的 objectURL 归 store 管，卸载时不得 revoke，
  // 否则切回页面后缩略图失效。
  window.removeEventListener('paste', handleSourceImagePaste)
})

function handleSourceImageChange(event: Event): void {
  const input = event.target as HTMLInputElement
  addSourceImages(Array.from(input.files ?? []))
  input.value = ''
}

function handleSourceImageDrop(event: DragEvent): void {
  const files = Array.from(event.dataTransfer?.files ?? []).filter((item) => item.type.startsWith('image/'))
  addSourceImages(files)
}

function handleSourceImagePaste(event: ClipboardEvent): void {
  if (generationMode.value !== 'edit') {
    return
  }
  const fromFiles = Array.from(event.clipboardData?.files ?? []).filter((file) => file.type.startsWith('image/'))
  const fromItems = Array.from(event.clipboardData?.items ?? [])
    .filter((item) => item.kind === 'file' && item.type.startsWith('image/'))
    .map((item) => item.getAsFile())
    .filter((file): file is File => file !== null)
  // files 与 items 指向同一批剪贴板文件，取其一即可，合并会重复添加。
  const images = fromFiles.length > 0 ? fromFiles : fromItems
  if (images.length === 0) {
    return
  }
  event.preventDefault()
  addSourceImages(images)
}

function modeLabel(mode?: GenerationMode): string {
  return mode === 'edit' ? '图改图' : '文生图'
}

function formatFileSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '0 KB'
  }
  if (bytes >= 1024 * 1024) {
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  }
  return `${Math.max(1, Math.round(bytes / 1024))} KB`
}

async function copyText(text: string): Promise<void> {
  try {
    await navigator.clipboard?.writeText(text)
  } catch {
    errorMessage.value = '复制失败，请手动复制'
  }
}

function downloadImage(src: string, id: string): void {
  const link = document.createElement('a')
  link.href = src
  link.download = `image-gen-${id}.png`
  document.body.appendChild(link)
  link.click()
  link.remove()
}

function formatTime(raw: string): string {
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) {
    return raw
  }
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}
</script>
