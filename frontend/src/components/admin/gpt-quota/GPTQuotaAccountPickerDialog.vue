<template>
  <BaseDialog :show="show" :title="t('gptQuota.admin.pickerTitle')" width="wide" @close="emit('close')">
    <div class="space-y-4">
      <!-- 已删除或改为非 OpenAI OAuth 的已选账号不在候选列表里，只能在这里取消。 -->
      <div class="space-y-2">
        <div class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('gptQuota.admin.pickerSelected', { count: order.length }) }}</div>
        <div v-if="order.length" class="flex max-h-28 flex-wrap gap-2 overflow-y-auto">
          <span
            v-for="id in order"
            :key="id"
            class="badge inline-flex items-center gap-1"
            :class="meta[id]?.eligible === false ? 'badge-danger' : 'badge-primary'"
            :title="meta[id]?.eligible === false ? reasonLabel(meta[id]?.reason) : undefined"
            data-testid="gpt-quota-picker-chip"
          >
            {{ meta[id]?.account_name || `#${id}` }}
            <button type="button" class="opacity-70 hover:opacity-100" :aria-label="t('gptQuota.admin.remove')" @click="uncheck(id)">×</button>
          </span>
        </div>
        <p v-else class="text-xs text-gray-400">{{ t('gptQuota.admin.selectedEmpty') }}</p>
      </div>

      <div class="flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 pt-4 dark:border-dark-700">
        <p class="input-hint">{{ t('gptQuota.admin.candidatesHint') }}</p>
        <input
          v-model="search"
          class="input w-64 py-1.5 text-sm"
          :placeholder="t('gptQuota.admin.searchPlaceholder')"
          data-testid="gpt-quota-picker-search"
          @keyup.enter="searchCandidates"
        />
      </div>
      <ul class="max-h-[400px] divide-y divide-gray-100 overflow-y-auto pr-1 dark:divide-dark-700" :class="{ 'opacity-50': loading }">
        <li v-for="candidate in candidates.items" :key="candidate.account_id">
          <label
            class="flex items-center gap-3 py-2"
            :class="canToggle(candidate) ? 'cursor-pointer' : 'cursor-not-allowed opacity-60'"
          >
            <input
              type="checkbox"
              class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              :checked="checked.has(candidate.account_id)"
              :disabled="loading || !canToggle(candidate)"
              data-testid="gpt-quota-picker-item"
              @change="toggle(candidate)"
            />
            <div class="min-w-0">
              <div class="truncate text-sm text-gray-900 dark:text-white">{{ candidate.account_name }}</div>
              <div class="mt-0.5 flex gap-1">
                <span v-if="candidate.group" class="badge badge-gray">{{ t(`gptQuota.groups.${candidate.group}`) }}</span>
                <span v-if="!candidate.eligible" class="badge badge-danger">{{ reasonLabel(candidate.reason) }}</span>
              </div>
            </div>
          </label>
        </li>
        <li v-if="loadError" class="py-6 text-center text-sm text-red-600 dark:text-red-400" data-testid="gpt-quota-picker-error">
          {{ loadError }}
          <button type="button" class="btn btn-secondary btn-sm ml-2" :disabled="loading" data-testid="gpt-quota-picker-retry" @click="load">
            {{ t('gptQuota.admin.pickerRetry') }}
          </button>
        </li>
        <li v-else-if="loading && !candidates.items.length" class="py-6 text-center text-sm text-gray-500 dark:text-dark-400">
          {{ t('common.loading') }}
        </li>
        <li v-else-if="!loading && !candidates.items.length" class="py-6 text-center text-sm text-gray-500 dark:text-dark-400">
          {{ t('gptQuota.admin.pickerEmpty') }}
        </li>
      </ul>
      <Pagination
        v-if="candidates.total > PAGE_SIZE"
        :total="candidates.total"
        :page="page"
        :page-size="PAGE_SIZE"
        :show-page-size-selector="false"
        @update:page="changePage"
      />
    </div>
    <template #footer>
      <div class="flex flex-wrap items-center justify-between gap-3">
        <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('gptQuota.admin.pickerApplyHint') }}</span>
        <div class="flex gap-3">
          <button type="button" class="btn btn-secondary" @click="emit('close')">{{ t('common.cancel') }}</button>
          <button type="button" class="btn btn-primary" data-testid="gpt-quota-picker-confirm" @click="confirm">{{ t('common.save') }}</button>
        </div>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import { listGPTQuotaCandidates, type GPTQuotaCandidate, type GPTQuotaCandidatePage } from '@/api/gptQuotaDisplay'
import { extractApiErrorMessage } from '@/utils/apiError'

const PAGE_SIZE = 20

const props = defineProps<{
  show: boolean
  // 打开时默认勾选的账号（含未保存的新增），顺序即展示顺序；用于已选区展示名称与资格。
  selected: GPTQuotaCandidate[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
  // ids 为勾选结果（保留原顺序，新勾选追加在后）；meta 为勾选账号的候选信息，供未保存行展示。
  (e: 'confirm', ids: number[], meta: Record<number, GPTQuotaCandidate>): void
}>()

const { t, te } = useI18n()

const search = ref('')
const page = ref(1)
const loading = ref(false)
const loadError = ref('')
const emptyPage = (): GPTQuotaCandidatePage => ({ items: [], total: 0, page: 1, page_size: PAGE_SIZE })
const candidates = ref<GPTQuotaCandidatePage>(emptyPage())
// 勾选草稿跨分页与搜索保留；取消或关闭弹窗时丢弃。
const order = ref<number[]>([])
const meta = ref<Record<number, GPTQuotaCandidate>>({})
const checked = computed(() => new Set(order.value))

watch(
  () => props.show,
  (open) => {
    if (!open) return
    order.value = props.selected.map((item) => item.account_id)
    meta.value = Object.fromEntries(props.selected.map((item) => [item.account_id, item]))
    search.value = ''
    page.value = 1
    // 不沿用上次打开时的列表，避免首屏失败时展示过期搜索结果。
    candidates.value = emptyPage()
    void load()
  },
  { immediate: true },
)

let generation = 0
async function load() {
  const current = ++generation
  loading.value = true
  loadError.value = ''
  try {
    const result = await listGPTQuotaCandidates({ search: search.value.trim() || undefined, page: page.value, page_size: PAGE_SIZE })
    if (current === generation) candidates.value = result
  } catch (err) {
    if (current !== generation) return
    // 清掉与当前页码/搜索不一致的旧条目；保留总数让分页仍可切换，重试沿用当前条件。
    candidates.value = { ...candidates.value, items: [] }
    loadError.value = extractApiErrorMessage(err, t('gptQuota.admin.pickerLoadFailed'))
  } finally {
    if (current === generation) loading.value = false
  }
}

function searchCandidates() {
  page.value = 1
  void load()
}

function changePage(value: number) {
  page.value = value
  void load()
}

// 已失去资格的已选账号只能取消勾选，不能新增不合格账号。
function canToggle(candidate: GPTQuotaCandidate) {
  return candidate.eligible || checked.value.has(candidate.account_id)
}

function uncheck(id: number) {
  order.value = order.value.filter((item) => item !== id)
}

function toggle(candidate: GPTQuotaCandidate) {
  if (loading.value || !canToggle(candidate)) return
  const id = candidate.account_id
  if (checked.value.has(id)) {
    uncheck(id)
    return
  }
  order.value = [...order.value, id]
  meta.value = { ...meta.value, [id]: candidate }
}

function confirm() {
  emit('confirm', [...order.value], { ...meta.value })
}

function reasonLabel(reason?: string) {
  if (!reason) return ''
  const key = `gptQuota.admin.reasons.${reason}`
  return te(key) ? t(key) : reason
}
</script>
