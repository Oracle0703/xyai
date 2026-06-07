<template>
  <AppLayout>
    <div class="space-y-4">
      <div class="card p-4">
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-8">
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('common.startDate') }}</span>
            <input v-model="filters.start_date" type="date" class="input h-9 text-sm" @change="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('common.endDate') }}</span>
            <input v-model="filters.end_date" type="date" class="input h-9 text-sm" @change="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">User ID</span>
            <input v-model.number="filters.user_id" type="number" min="1" class="input h-9 text-sm" @keyup.enter="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">API Key ID</span>
            <input v-model.number="filters.api_key_id" type="number" min="1" class="input h-9 text-sm" @keyup.enter="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">Model</span>
            <input v-model.trim="filters.model" class="input h-9 text-sm" @keyup.enter="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.tokenAnalysis.project') }}</span>
            <input v-model.trim="filters.project" class="input h-9 text-sm" :placeholder="t('admin.tokenAnalysis.projectFilterHint')" @keyup.enter="reloadAll" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.tokenAnalysis.riskMin') }}</span>
            <select v-model.number="filters.risk_min" class="input h-9 text-sm" @change="reloadAll">
              <option :value="0">0</option>
              <option :value="20">20</option>
              <option :value="40">40</option>
              <option :value="60">60</option>
            </select>
          </label>
          <label class="flex items-end gap-2 pb-2 text-sm text-gray-600 dark:text-gray-300">
            <input v-model="filters.include_unmatched" type="checkbox" class="h-4 w-4 rounded border-gray-300" @change="reloadAll" />
            <span>{{ t('admin.tokenAnalysis.includeUnmatched') }}</span>
          </label>
          <div class="flex items-end gap-2">
            <button type="button" class="btn btn-secondary h-9 flex-1 text-sm" @click="reloadAll">
              {{ t('common.refresh') }}
            </button>
            <button type="button" class="btn btn-primary h-9 flex-1 text-sm" :disabled="indexing" @click="triggerIndex">
              {{ indexing ? t('common.loading') : t('admin.tokenAnalysis.indexNow') }}
            </button>
          </div>
        </div>
      </div>

      <div class="grid grid-cols-2 gap-3 lg:grid-cols-6">
        <div v-for="card in summaryCards" :key="card.label" class="card p-4">
          <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ card.label }}</div>
          <div class="mt-2 truncate text-xl font-semibold text-gray-900 dark:text-white">{{ card.value }}</div>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div class="card p-4">
          <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.matchQuality') }}</h2>
          <div class="mt-3 grid grid-cols-3 gap-3 text-sm">
            <div>
              <div class="text-xs text-gray-500">{{ t('admin.tokenAnalysis.matched') }}</div>
              <div class="mt-1 font-semibold text-gray-900 dark:text-white">{{ formatNumber(summary?.matched_requests ?? 0) }}</div>
            </div>
            <div>
              <div class="text-xs text-gray-500">{{ t('admin.tokenAnalysis.unmatched') }}</div>
              <div class="mt-1 font-semibold text-gray-900 dark:text-white">{{ formatNumber(summary?.unmatched_requests ?? 0) }}</div>
            </div>
            <div>
              <div class="text-xs text-gray-500">{{ t('admin.tokenAnalysis.unmatchedRate') }}</div>
              <div class="mt-1 font-semibold text-gray-900 dark:text-white">{{ percent(summary?.unmatched_rate ?? 0) }}</div>
            </div>
          </div>
        </div>

        <div class="card p-4 lg:col-span-2">
          <div class="mb-3 flex items-center justify-between gap-3">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.riskReasons') }}</h2>
            <span class="text-xs text-gray-500">{{ t('admin.tokenAnalysis.riskRate') }} {{ percent(summary?.risk_request_rate ?? 0) }}</span>
          </div>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="reason in summary?.risk_reasons || []"
              :key="reason.code"
              type="button"
              class="rounded border px-2.5 py-1 text-xs"
              :class="filters.risk_reason === reason.code ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'border-gray-200 bg-white text-gray-600 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300'"
              @click="toggleRiskReason(reason.code)"
            >
              {{ reason.code }} · {{ formatNumber(reason.count) }}
            </button>
            <span v-if="!(summary?.risk_reasons || []).length" class="text-sm text-gray-500">{{ t('common.noData') }}</span>
          </div>
        </div>
      </div>

      <div class="card p-4">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.tokenAnalysis.indexStatus') }}:
            <span class="font-medium text-gray-800 dark:text-gray-100">{{ indexStatusText }}</span>
          </div>
          <div class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.tokenAnalysis.processed') }} {{ indexStatus?.processed_rows ?? 0 }} / {{ t('admin.tokenAnalysis.failed') }} {{ indexStatus?.failed_rows ?? 0 }}
          </div>
        </div>
        <div v-if="indexStatus?.files?.length" class="mt-3 overflow-x-auto">
          <table class="table text-xs">
            <thead>
              <tr>
                <th>{{ t('admin.tokenAnalysis.file') }}</th>
                <th>{{ t('admin.tokenAnalysis.offset') }}</th>
                <th>{{ t('admin.tokenAnalysis.processed') }}</th>
                <th>{{ t('admin.tokenAnalysis.failed') }}</th>
                <th>{{ t('admin.tokenAnalysis.updated') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="file in indexStatus.files.slice(0, 5)" :key="file.source_file">
                <td class="max-w-[320px] truncate">{{ file.source_file }}</td>
                <td>{{ formatNumber(file.last_offset) }}</td>
                <td>{{ formatNumber(file.processed_rows) }}</td>
                <td>{{ formatNumber(file.failed_rows) }}</td>
                <td>{{ file.updated_at || '-' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="card p-4">
        <div class="mb-3 flex items-center justify-between">
          <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.projectRanking') }}</h2>
          <span class="text-xs text-gray-500">{{ projectsPagination.total }}</span>
        </div>
        <div class="overflow-x-auto">
          <table class="table text-sm">
            <thead>
              <tr>
                <th>{{ t('admin.tokenAnalysis.project') }}</th>
                <th>{{ t('admin.tokenAnalysis.user') }}</th>
                <th>{{ t('admin.tokenAnalysis.requests') }}</th>
                <th>{{ t('admin.tokenAnalysis.tokens') }}</th>
                <th>Input / Output</th>
                <th>{{ t('admin.tokenAnalysis.cacheTokens') }}</th>
                <th>{{ t('admin.tokenAnalysis.cost') }}</th>
                <th>{{ t('admin.tokenAnalysis.lastActive') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in projects" :key="`${row.project}-${row.user_id || 0}`">
                <td>
                  <button
                    type="button"
                    class="max-w-[200px] truncate font-medium hover:text-primary-600"
                    :class="row.project === 'unattributed' ? 'italic text-gray-400' : ''"
                    :title="row.project"
                    @click="toggleProjectFilter(row.project)"
                  >
                    {{ row.project === 'unattributed' ? t('admin.tokenAnalysis.unattributed') : row.project }}
                  </button>
                </td>
                <td>
                  <div class="max-w-[180px] truncate">{{ row.user_email || '-' }}</div>
                </td>
                <td>
                  <div>{{ formatNumber(row.request_count) }}</div>
                  <div class="text-xs text-gray-500">{{ t('admin.tokenAnalysis.matched') }} {{ formatNumber(row.matched_request_count) }}</div>
                </td>
                <td class="font-medium">{{ formatNumber(row.total_tokens) }}</td>
                <td class="text-xs text-gray-500">{{ formatNumber(row.input_tokens) }} / {{ formatNumber(row.output_tokens) }}</td>
                <td class="text-xs text-gray-500">{{ formatNumber(row.cache_read_tokens + row.cache_creation_tokens) }}</td>
                <td>{{ formatCost(row.actual_cost) }}</td>
                <td class="text-xs text-gray-500">{{ row.last_event_time || '-' }}</td>
              </tr>
              <tr v-if="!projectsLoading && projects.length === 0">
                <td colspan="8" class="py-8 text-center text-gray-500">{{ t('common.noData') }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <Pagination
          v-if="projectsPagination.total > 0"
          :page="projectsPagination.page"
          :page-size="projectsPagination.page_size"
          :total="projectsPagination.total"
          class="mt-3"
          @update:page="changeProjectPage"
          @update:pageSize="changeProjectPageSize"
        />
      </div>

      <div class="grid grid-cols-1 gap-4 xl:grid-cols-3">
        <div class="card p-4 xl:col-span-1">
          <div class="mb-3 flex items-center justify-between">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.userRanking') }}</h2>
            <span class="text-xs text-gray-500">{{ usersPagination.total }}</span>
          </div>
          <div class="overflow-x-auto">
            <table class="table text-sm">
              <thead>
                <tr>
                  <th>{{ t('admin.tokenAnalysis.user') }}</th>
                  <th>{{ t('admin.tokenAnalysis.tokens') }}</th>
                  <th>{{ t('admin.tokenAnalysis.cost') }}</th>
                  <th>{{ t('admin.tokenAnalysis.risk') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="user in users" :key="`${user.user_id || 0}-${user.api_key_id || 0}`">
                  <td>
                    <div class="max-w-[180px] truncate font-medium">{{ user.user_email || '-' }}</div>
                    <div class="text-xs text-gray-500">{{ user.api_key_name || '-' }}</div>
                  </td>
                  <td>{{ formatNumber(user.total_tokens) }}</td>
                  <td>{{ formatCost(user.actual_cost) }}</td>
                  <td>{{ percent(user.risk_ratio) }}</td>
                </tr>
                <tr v-if="!usersLoading && users.length === 0">
                  <td colspan="4" class="py-8 text-center text-gray-500">{{ t('common.noData') }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <div class="card p-4 xl:col-span-2">
          <div class="mb-3 flex items-center justify-between">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.suspiciousRequests') }}</h2>
            <span class="text-xs text-gray-500">{{ requestsPagination.total }}</span>
          </div>
          <div class="overflow-x-auto">
            <table class="table text-sm">
              <thead>
                <tr>
                  <th>{{ t('admin.tokenAnalysis.risk') }}</th>
                  <th>{{ t('admin.tokenAnalysis.user') }}</th>
                  <th>Model</th>
                  <th>{{ t('admin.tokenAnalysis.usage') }}</th>
                  <th>{{ t('admin.tokenAnalysis.preview') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="item in requests" :key="item.id" class="cursor-pointer" @click="selectedRequest = item">
                  <td>
                    <span class="inline-flex min-w-10 justify-center rounded-full px-2 py-0.5 text-xs font-semibold" :class="riskClass(item.risk_score)">
                      {{ item.risk_score }}
                    </span>
                  </td>
                  <td>
                    <div class="max-w-[180px] truncate font-medium">{{ item.user_email || '-' }}</div>
                    <div class="text-xs text-gray-500">{{ item.api_key_name || '-' }}</div>
                  </td>
                  <td>
                    <div class="max-w-[180px] truncate">{{ item.model }}</div>
                    <div class="text-xs text-gray-500">{{ item.endpoint }}</div>
                  </td>
                  <td>
                    <div>{{ formatNumber(item.input_tokens + item.output_tokens + item.cache_read_tokens + item.cache_creation_tokens) }}</div>
                    <div class="text-xs text-gray-500">{{ formatCost(item.actual_cost) }}</div>
                  </td>
                  <td>
                    <div class="max-w-[360px] truncate">{{ item.last_user_preview }}</div>
                    <div class="mt-1 flex flex-wrap gap-1">
                      <span v-for="reason in item.risk_reasons || []" :key="reason.code" class="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                        {{ reason.code }}
                      </span>
                    </div>
                  </td>
                </tr>
                <tr v-if="!requestsLoading && requests.length === 0">
                  <td colspan="5" class="py-8 text-center text-gray-500">{{ t('common.noData') }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <Pagination
            v-if="requestsPagination.total > 0"
            :page="requestsPagination.page"
            :page-size="requestsPagination.page_size"
            :total="requestsPagination.total"
            class="mt-3"
            @update:page="changeRequestPage"
            @update:pageSize="changeRequestPageSize"
          />
        </div>
      </div>
    </div>

    <div v-if="selectedRequest" class="fixed inset-0 z-40 bg-black/20" @click="selectedRequest = null"></div>
    <aside v-if="selectedRequest" class="fixed right-0 top-0 z-50 h-full w-full max-w-xl overflow-y-auto border-l border-gray-200 bg-white p-5 shadow-xl dark:border-dark-700 dark:bg-dark-900">
      <div class="mb-4 flex items-start justify-between gap-3">
        <div>
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ selectedRequest.archive_id }}</h2>
          <p class="text-sm text-gray-500">{{ selectedRequest.event_time }}</p>
        </div>
        <button type="button" class="btn btn-ghost btn-sm" @click="selectedRequest = null">{{ t('common.close') }}</button>
      </div>
      <dl class="grid grid-cols-2 gap-3 text-sm">
        <template v-for="field in detailFields" :key="field.label">
          <dt class="text-gray-500">{{ field.label }}</dt>
          <dd class="min-w-0 break-words font-medium text-gray-900 dark:text-gray-100">{{ field.value }}</dd>
        </template>
      </dl>
      <div class="mt-5">
        <div class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.preview') }}</div>
        <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200">
          {{ selectedRequest.last_user_preview || '-' }}
        </div>
      </div>
    </aside>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  TokenAnalysisIndexStatus,
  TokenAnalysisProjectUsage,
  TokenAnalysisQueryParams,
  TokenAnalysisRequestItem,
  TokenAnalysisSummary,
  TokenAnalysisUserUsage
} from '@/api/admin/tokenAnalysis'
import AppLayout from '@/components/layout/AppLayout.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()

const today = new Date().toISOString().slice(0, 10)
const filters = reactive<TokenAnalysisQueryParams>({
  start_date: today,
  end_date: today,
  timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
  risk_min: 20,
  include_unmatched: false
})

const summary = ref<TokenAnalysisSummary | null>(null)
const users = ref<TokenAnalysisUserUsage[]>([])
const projects = ref<TokenAnalysisProjectUsage[]>([])
const requests = ref<TokenAnalysisRequestItem[]>([])
const indexStatus = ref<TokenAnalysisIndexStatus | null>(null)
const selectedRequest = ref<TokenAnalysisRequestItem | null>(null)
const usersLoading = ref(false)
const projectsLoading = ref(false)
const requestsLoading = ref(false)
const indexing = ref(false)

const usersPagination = reactive({ page: 1, page_size: 20, total: 0 })
const projectsPagination = reactive({ page: 1, page_size: 20, total: 0 })
const requestsPagination = reactive({ page: 1, page_size: 20, total: 0 })

const cleanFilters = computed(() => {
  const out: TokenAnalysisQueryParams = {}
  for (const [key, value] of Object.entries(filters)) {
    if (value !== '' && value !== undefined && value !== null) {
      (out as Record<string, unknown>)[key] = value
    }
  }
  return out
})

const summaryCards = computed(() => [
  { label: t('admin.tokenAnalysis.summary.totalTokens'), value: formatNumber(summary.value?.total_tokens ?? 0) },
  { label: t('admin.tokenAnalysis.summary.totalCost'), value: formatCost(summary.value?.total_actual_cost ?? 0) },
  { label: t('admin.tokenAnalysis.summary.cacheRead'), value: formatNumber(summary.value?.cache_read_tokens ?? 0) },
  { label: t('admin.tokenAnalysis.summary.cacheHitRate'), value: percent(summary.value?.cache_hit_rate ?? 0) },
  { label: t('admin.tokenAnalysis.summary.riskyRequests'), value: formatNumber(summary.value?.risky_requests ?? 0) },
  { label: t('admin.tokenAnalysis.summary.riskyCost'), value: formatCost(summary.value?.risky_cost ?? 0) }
])

const indexStatusText = computed(() => {
  if (indexStatus.value?.running) return t('common.loading')
  if (indexStatus.value?.last_error) return indexStatus.value.last_error
  return indexStatus.value?.updated_at || '-'
})

const detailFields = computed(() => {
  const item = selectedRequest.value
  if (!item) return []
  return [
    { label: 'Endpoint', value: item.endpoint },
    { label: 'Model', value: item.model },
    { label: 'Match', value: String(item.match_confidence) },
    { label: 'Messages', value: String(item.message_count ?? 0) },
    { label: 'System chars', value: formatNumber(item.system_chars ?? 0) },
    { label: 'User chars', value: formatNumber(item.user_chars ?? 0) },
    { label: 'Tools', value: String(item.tools_count ?? 0) },
    { label: 'Images', value: String(item.image_count ?? 0) },
    { label: 'Input tokens', value: formatNumber(item.input_tokens ?? 0) },
    { label: 'Output tokens', value: formatNumber(item.output_tokens ?? 0) },
    { label: 'Cache read', value: formatNumber(item.cache_read_tokens ?? 0) },
    { label: 'Cost', value: formatCost(item.actual_cost ?? 0) },
    { label: t('admin.tokenAnalysis.project'), value: item.client_project || '-' },
    { label: t('admin.tokenAnalysis.workdir'), value: item.client_workdir || '-' },
    { label: t('admin.tokenAnalysis.branch'), value: item.client_branch || '-' },
    { label: t('admin.tokenAnalysis.attributionSource'), value: item.attribution_source || '-' }
  ]
})

function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(Math.round(value || 0))
}

function formatCost(value: number): string {
  return `$${Number(value || 0).toFixed(4)}`
}

function percent(value: number): string {
  return `${((value || 0) * 100).toFixed(1)}%`
}

function riskClass(score: number): string {
  if (score >= 60) return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (score >= 30) return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
  return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
}

async function loadSummary() {
  summary.value = await adminAPI.tokenAnalysis.getSummary(cleanFilters.value)
}

async function loadUsers() {
  usersLoading.value = true
  try {
    const result = await adminAPI.tokenAnalysis.listUsers({
      ...cleanFilters.value,
      page: usersPagination.page,
      page_size: usersPagination.page_size
    })
    users.value = result.items
    usersPagination.total = result.total
    usersPagination.page = result.page
    usersPagination.page_size = result.page_size
  } finally {
    usersLoading.value = false
  }
}

async function loadProjects() {
  projectsLoading.value = true
  try {
    const result = await adminAPI.tokenAnalysis.listProjects({
      ...cleanFilters.value,
      page: projectsPagination.page,
      page_size: projectsPagination.page_size
    })
    projects.value = result.items
    projectsPagination.total = result.total
    projectsPagination.page = result.page
    projectsPagination.page_size = result.page_size
  } finally {
    projectsLoading.value = false
  }
}

async function loadRequests() {
  requestsLoading.value = true
  try {
    const result = await adminAPI.tokenAnalysis.listRequests({
      ...cleanFilters.value,
      page: requestsPagination.page,
      page_size: requestsPagination.page_size,
      sort_by: 'risk_score',
      sort_order: 'desc'
    })
    requests.value = result.items
    requestsPagination.total = result.total
    requestsPagination.page = result.page
    requestsPagination.page_size = result.page_size
  } finally {
    requestsLoading.value = false
  }
}

async function loadIndexStatus() {
  indexStatus.value = await adminAPI.tokenAnalysis.getIndexStatus()
}

async function reloadAll() {
  requestsPagination.page = 1
  projectsPagination.page = 1
  try {
    await Promise.all([loadSummary(), loadUsers(), loadProjects(), loadRequests(), loadIndexStatus()])
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  }
}

async function triggerIndex() {
  indexing.value = true
  try {
    const result = await adminAPI.tokenAnalysis.triggerIndex({
      start_date: filters.start_date,
      end_date: filters.end_date,
      timezone: filters.timezone
    })
    appStore.showSuccess(`${t('admin.tokenAnalysis.indexed')}: ${result.indexed_rows}`)
    await reloadAll()
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    indexing.value = false
  }
}

function changeRequestPage(page: number) {
  requestsPagination.page = page
  void loadRequests()
}

function changeRequestPageSize(pageSize: number) {
  requestsPagination.page_size = pageSize
  requestsPagination.page = 1
  void loadRequests()
}

function toggleRiskReason(code: string) {
  filters.risk_reason = filters.risk_reason === code ? undefined : code
  void reloadAll()
}

function changeProjectPage(page: number) {
  projectsPagination.page = page
  void loadProjects()
}

function changeProjectPageSize(pageSize: number) {
  projectsPagination.page_size = pageSize
  projectsPagination.page = 1
  void loadProjects()
}

function toggleProjectFilter(project: string) {
  filters.project = filters.project === project ? undefined : project
  void reloadAll()
}

onMounted(() => {
  void reloadAll()
})
</script>
