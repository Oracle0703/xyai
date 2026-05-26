<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="card p-4">
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-8">
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('common.startDate') }}</span>
            <input v-model="dateRange.start" type="date" class="input h-9 text-sm" @change="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('common.endDate') }}</span>
            <input v-model="dateRange.end" type="date" class="input h-9 text-sm" @change="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.promptMetrics.project') }}</span>
            <input v-model.trim="filters.project" class="input h-9 text-sm" @keyup.enter="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.promptMetrics.branch') }}</span>
            <input v-model.trim="filters.branch" class="input h-9 text-sm" @keyup.enter="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.promptMetrics.client') }}</span>
            <input v-model.trim="filters.client" class="input h-9 text-sm" @keyup.enter="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">Model</span>
            <input v-model.trim="filters.model" class="input h-9 text-sm" @keyup.enter="reloadAll" />
          </label>
          <label class="flex items-end gap-2 pb-2 text-sm text-gray-600 dark:text-gray-300">
            <input v-model="filters.only_low_quality" type="checkbox" class="h-4 w-4 rounded border-gray-300" @change="reloadAll" />
            <span>{{ t('admin.promptMetrics.onlyLowQuality') }}</span>
          </label>
          <div class="flex items-end">
            <button type="button" class="btn btn-primary h-9 w-full text-sm" :disabled="loading" @click="reloadAll">
              {{ loading ? t('common.loading') : t('common.refresh') }}
            </button>
          </div>
        </div>
      </div>

      <div class="grid grid-cols-2 gap-3 lg:grid-cols-8">
        <div v-for="card in summaryCards" :key="card.label" class="card p-4">
          <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ card.label }}</div>
          <div class="mt-2 truncate text-xl font-semibold text-gray-900 dark:text-white">{{ card.value }}</div>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-4 xl:grid-cols-3">
        <div class="card p-4 xl:col-span-2">
          <div class="mb-3 flex items-center justify-between">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.promptMetrics.trend') }}</h2>
            <select v-model="trendBucket" class="input h-8 w-28 text-sm" @change="loadTrend">
              <option value="day">{{ t('admin.promptMetrics.day') }}</option>
              <option value="hour">{{ t('admin.promptMetrics.hour') }}</option>
            </select>
          </div>
          <div class="h-56">
            <div v-if="trend.length === 0" class="flex h-full items-center justify-center text-sm text-gray-500">{{ t('common.noData') }}</div>
            <div v-else class="flex h-full items-end gap-1">
              <div v-for="point in trend" :key="point.bucket" class="group flex min-w-0 flex-1 flex-col items-center gap-1">
                <div class="w-full rounded-t bg-primary-500/80" :style="{ height: `${barHeight(point.events)}%` }"></div>
                <div class="max-w-full truncate text-[10px] text-gray-500">{{ point.bucket }}</div>
              </div>
            </div>
          </div>
        </div>

        <div class="card p-4">
          <div class="mb-3 flex items-center justify-between">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.promptMetrics.rank') }}</h2>
            <select v-model="rankDimension" class="input h-8 w-28 text-sm" @change="loadRank">
              <option value="project">{{ t('admin.promptMetrics.project') }}</option>
              <option value="branch">{{ t('admin.promptMetrics.branch') }}</option>
              <option value="client">{{ t('admin.promptMetrics.client') }}</option>
              <option value="model">Model</option>
              <option value="user">{{ t('admin.promptMetrics.user') }}</option>
            </select>
          </div>
          <div class="space-y-2">
            <div v-for="item in ranks" :key="item.key || item.label" class="rounded border border-gray-100 p-2 dark:border-dark-700">
              <div class="flex items-center justify-between gap-2 text-sm">
                <span class="truncate font-medium text-gray-900 dark:text-white">{{ item.label || '-' }}</span>
                <span class="text-xs text-gray-500">{{ formatNumber(item.events) }}</span>
              </div>
              <div class="mt-1 text-xs text-gray-500">{{ formatNumber(item.tokens) }} tokens · {{ formatCost(item.cost) }}</div>
            </div>
            <div v-if="ranks.length === 0" class="py-8 text-center text-sm text-gray-500">{{ t('common.noData') }}</div>
          </div>
        </div>
      </div>

      <div class="card p-4">
        <div class="mb-3 flex items-center justify-between">
          <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.promptMetrics.events') }}</h2>
          <span class="text-xs text-gray-500">{{ eventsPagination.total }}</span>
        </div>
        <div class="overflow-x-auto">
          <table class="table text-sm">
            <thead>
              <tr>
                <th>{{ t('admin.promptMetrics.time') }}</th>
                <th>{{ t('admin.promptMetrics.user') }}</th>
                <th>{{ t('admin.promptMetrics.context') }}</th>
                <th>{{ t('admin.promptMetrics.prompt') }}</th>
                <th>{{ t('admin.promptMetrics.quality') }}</th>
                <th>{{ t('admin.promptMetrics.usage') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="event in events" :key="event.id" class="cursor-pointer" @click="openEvent(event.id)">
                <td class="whitespace-nowrap">{{ formatDateTime(event.created_at) }}</td>
                <td>
                  <div class="max-w-[180px] truncate font-medium">{{ event.user_email || event.user_id || '-' }}</div>
                  <div class="text-xs text-gray-500">{{ event.api_key_name || event.api_key_id || '-' }}</div>
                </td>
                <td>
                  <div class="max-w-[220px] truncate">{{ event.project_name || '-' }}</div>
                  <div class="text-xs text-gray-500">{{ event.git_branch || event.client_name || '-' }}</div>
                </td>
                <td>
                  <div class="max-w-[420px] truncate">{{ event.prompt_excerpt }}</div>
                  <div class="text-xs text-gray-500">{{ event.endpoint }} · {{ event.requested_model || event.model || '-' }}</div>
                </td>
                <td>
                  <span class="inline-flex min-w-10 justify-center rounded-full px-2 py-0.5 text-xs font-semibold" :class="qualityClass(event.analysis?.quality_score)">
                    {{ event.analysis?.quality_score ?? event.analysis_status }}
                  </span>
                </td>
                <td>
                  <div>{{ formatNumber(event.total_tokens || 0) }}</div>
                  <div class="text-xs text-gray-500">{{ formatCost(event.actual_cost || 0) }}</div>
                </td>
              </tr>
              <tr v-if="!loading && events.length === 0">
                <td colspan="6" class="py-8 text-center text-gray-500">{{ t('common.noData') }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <Pagination
          v-if="eventsPagination.total > 0"
          :page="eventsPagination.page"
          :page-size="eventsPagination.page_size"
          :total="eventsPagination.total"
          class="mt-3"
          @update:page="changePage"
          @update:pageSize="changePageSize"
        />
      </div>
    </div>

    <BaseDialog
      :show="detailOpen"
      :title="t('admin.promptMetrics.detail')"
      width="extra-wide"
      @close="detailOpen = false"
    >
      <div v-if="selectedEvent" class="space-y-4">
        <div class="grid grid-cols-2 gap-3 text-sm lg:grid-cols-4">
          <div><div class="text-xs text-gray-500">ID</div><div class="font-medium">{{ selectedEvent.id }}</div></div>
          <div><div class="text-xs text-gray-500">{{ t('admin.promptMetrics.quality') }}</div><div class="font-medium">{{ selectedEvent.analysis?.quality_score ?? '-' }}</div></div>
          <div><div class="text-xs text-gray-500">{{ t('admin.promptMetrics.project') }}</div><div class="font-medium">{{ selectedEvent.project_name || '-' }}</div></div>
          <div><div class="text-xs text-gray-500">{{ t('admin.promptMetrics.branch') }}</div><div class="font-medium">{{ selectedEvent.git_branch || '-' }}</div></div>
        </div>
        <div>
          <div class="mb-1 text-xs font-medium text-gray-500">{{ t('admin.promptMetrics.prompt') }}</div>
          <pre class="max-h-72 overflow-auto rounded bg-gray-50 p-3 text-xs text-gray-800 dark:bg-dark-800 dark:text-gray-100">{{ selectedEvent.prompt_text || selectedEvent.prompt_excerpt }}</pre>
        </div>
        <div v-if="selectedEvent.analysis" class="rounded border border-gray-100 p-3 dark:border-dark-700">
          <div class="text-sm font-medium text-gray-900 dark:text-white">{{ selectedEvent.analysis.summary }}</div>
          <div class="mt-2 flex flex-wrap gap-2">
            <span v-for="item in selectedEvent.analysis.improvement_suggestions" :key="item" class="rounded bg-gray-100 px-2 py-1 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">{{ item }}</span>
          </div>
        </div>
      </div>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="detailOpen = false">{{ t('common.close') }}</button>
        <button type="button" class="btn btn-primary" :disabled="reanalyzing || !selectedEvent" @click="reanalyzeSelected">
          {{ reanalyzing ? t('common.loading') : t('admin.promptMetrics.reanalyze') }}
        </button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import promptMetricsAPI, { type PromptMetricsEvent, type PromptMetricsFilters, type PromptMetricsOverview, type PromptMetricsRankItem, type PromptMetricsTrendPoint } from '@/api/admin/promptMetrics'

const { t } = useI18n()

const loading = ref(false)
const reanalyzing = ref(false)
const overview = ref<PromptMetricsOverview | null>(null)
const trend = ref<PromptMetricsTrendPoint[]>([])
const ranks = ref<PromptMetricsRankItem[]>([])
const events = ref<PromptMetricsEvent[]>([])
const selectedEvent = ref<PromptMetricsEvent | null>(null)
const detailOpen = ref(false)
const trendBucket = ref<'day' | 'hour'>('day')
const rankDimension = ref('project')
const eventsPagination = reactive({ page: 1, page_size: 20, total: 0 })
const dateRange = reactive({ start: defaultStartDate(), end: defaultEndDate() })
const filters = reactive<PromptMetricsFilters>({
  project: '',
  branch: '',
  client: '',
  model: '',
  only_low_quality: false
})

const summaryCards = computed(() => [
  { label: t('admin.promptMetrics.totalEvents'), value: formatNumber(overview.value?.total_events || 0) },
  { label: t('admin.promptMetrics.activeUsers'), value: formatNumber(overview.value?.active_users || 0) },
  { label: t('admin.promptMetrics.lowQuality'), value: formatNumber(overview.value?.low_quality || 0) },
  { label: t('admin.promptMetrics.truncated'), value: formatNumber(overview.value?.truncated || 0) },
  { label: t('admin.promptMetrics.pending'), value: formatNumber(overview.value?.pending_analysis || 0) },
  { label: 'Tokens', value: formatNumber(overview.value?.total_tokens || 0) },
  { label: t('admin.promptMetrics.cost'), value: formatCost(overview.value?.total_cost || 0) },
  { label: t('admin.promptMetrics.avgQuality'), value: formatNumber(Math.round(overview.value?.average_quality || 0)) }
])

function buildParams(): PromptMetricsFilters {
  return {
    ...filters,
    from: dateToRFC3339(dateRange.start, false),
    to: dateToRFC3339(dateRange.end, true)
  }
}

async function reloadAll() {
  loading.value = true
  eventsPagination.page = 1
  try {
    await Promise.all([loadOverview(), loadTrend(), loadRank(), loadEvents()])
  } finally {
    loading.value = false
  }
}

async function loadOverview() {
  overview.value = await promptMetricsAPI.getOverview(buildParams())
}

async function loadTrend() {
  trend.value = await promptMetricsAPI.getTrend({ ...buildParams(), bucket: trendBucket.value })
}

async function loadRank() {
  ranks.value = await promptMetricsAPI.getRank({ ...buildParams(), dimension: rankDimension.value, limit: 10 })
}

async function loadEvents() {
  const data = await promptMetricsAPI.listEvents({ ...buildParams(), page: eventsPagination.page, page_size: eventsPagination.page_size })
  events.value = data.items
  eventsPagination.total = data.total
  eventsPagination.page = data.page
  eventsPagination.page_size = data.page_size
}

async function openEvent(id: number) {
  selectedEvent.value = await promptMetricsAPI.getEvent(id)
  detailOpen.value = true
}

async function reanalyzeSelected() {
  if (!selectedEvent.value) return
  reanalyzing.value = true
  try {
    const analysis = await promptMetricsAPI.reanalyze(selectedEvent.value.id)
    selectedEvent.value.analysis = analysis
    await loadEvents()
  } finally {
    reanalyzing.value = false
  }
}

async function changePage(page: number) {
  eventsPagination.page = page
  await loadEvents()
}

async function changePageSize(pageSize: number) {
  eventsPagination.page = 1
  eventsPagination.page_size = pageSize
  await loadEvents()
}

function barHeight(value: number): number {
  const max = Math.max(...trend.value.map(item => item.events), 1)
  return Math.max(4, Math.round((value / max) * 100))
}

function qualityClass(score?: number) {
  if (score === undefined) return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  if (score >= 80) return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  if (score >= 60) return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
}

function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(value || 0)
}

function formatCost(value: number): string {
  return `$${(value || 0).toFixed(4)}`
}

function formatDateTime(value: string): string {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

function defaultStartDate(): string {
  const date = new Date()
  date.setDate(date.getDate() - 6)
  return formatDateInputValue(date)
}

function defaultEndDate(): string {
  return formatDateInputValue(new Date())
}

function dateToRFC3339(value: string, endOfDay: boolean): string | undefined {
  if (!value) return undefined
  const date = new Date(`${value}T${endOfDay ? '23:59:59' : '00:00:00'}`)
  return date.toISOString()
}

// formatDateInputValue 按浏览器本地日期生成 input[type="date"] 需要的 yyyy-mm-dd 值, 避免 UTC 转换导致日期偏移。
function formatDateInputValue(date: Date): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

onMounted(() => {
  reloadAll()
})
</script>
