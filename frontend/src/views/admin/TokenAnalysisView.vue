<template>
  <AppLayout>
    <!-- 页面标题与描述由 AppHeader(路由元信息)展示, 页面内不重复。 -->
    <div class="mx-auto w-full max-w-[1800px] space-y-6" data-testid="token-analysis-view">
      <section class="card p-4" :aria-label="t('admin.tokenAnalysis.filters')">
        <div class="flex flex-wrap items-center gap-3">
          <DateRangePicker
            :start-date="filters.start_date || ''"
            :end-date="filters.end_date || ''"
            @change="onDateRangeChange"
          />
          <div class="relative min-w-[220px] flex-1">
            <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input
              v-model.trim="filters.user_email"
              type="search"
              class="input py-2 pl-9"
              :aria-label="t('admin.tokenAnalysis.userEmail')"
              :placeholder="t('admin.tokenAnalysis.userEmailHint')"
              @keyup.enter="reloadAll"
            />
          </div>
          <input
            v-model.trim="filters.project"
            class="input w-full py-2 sm:w-44"
            :aria-label="t('admin.tokenAnalysis.project')"
            :title="t('admin.tokenAnalysis.projectFilterHint')"
            :placeholder="t('admin.tokenAnalysis.projectPlaceholder')"
            @keyup.enter="reloadAll"
          />
          <input
            v-model.trim="filters.model"
            class="input w-full py-2 sm:w-44"
            :aria-label="t('admin.tokenAnalysis.model')"
            :placeholder="t('admin.tokenAnalysis.modelFilterHint')"
            @keyup.enter="reloadAll"
          />
          <div class="w-full sm:w-36">
            <Select
              :model-value="filters.risk_min ?? 0"
              :options="riskMinOptions"
              :searchable="false"
              :aria-label="t('admin.tokenAnalysis.riskMin')"
              @update:model-value="onRiskMinChange"
            />
          </div>
          <button
            type="button"
            class="btn btn-secondary inline-flex items-center gap-1.5"
            :aria-expanded="showAdvanced"
            data-testid="token-analysis-advanced-toggle"
            @click="showAdvanced = !showAdvanced"
          >
            <Icon name="filter" size="sm" />
            {{ t('admin.tokenAnalysis.moreFilters') }}
            <span v-if="advancedActiveCount > 0" class="rounded-full bg-primary-100 px-1.5 text-xs font-semibold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">
              {{ advancedActiveCount }}
            </span>
            <Icon name="chevronDown" size="xs" class="transition-transform" :class="{ 'rotate-180': showAdvanced }" />
          </button>
          <div class="flex items-center gap-2 lg:ml-auto">
            <button type="button" class="btn btn-secondary" @click="resetFilters">
              {{ t('admin.tokenAnalysis.reset') }}
            </button>
            <button type="button" class="btn btn-primary inline-flex items-center gap-1.5" @click="reloadAll">
              <Icon name="search" size="sm" />
              {{ t('admin.tokenAnalysis.query') }}
            </button>
          </div>
        </div>

        <div v-if="showAdvanced" class="mt-4 grid grid-cols-2 gap-3 border-t border-gray-100 pt-4 dark:border-dark-700 md:grid-cols-3 xl:grid-cols-5">
          <label v-for="field in idFilterFields" :key="field.key" class="block">
            <span class="mb-1 block text-xs font-medium text-gray-500 dark:text-dark-400">{{ field.label }}</span>
            <input v-model.number="filters[field.key]" type="number" min="1" class="input py-2" @keyup.enter="reloadAll" />
          </label>
          <label class="col-span-2 flex items-center gap-2 self-end pb-2 text-sm text-gray-700 dark:text-gray-300 md:col-span-1">
            <input v-model="filters.include_unmatched" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" @change="reloadAll" />
            <span>{{ t('admin.tokenAnalysis.includeUnmatched') }}</span>
            <HelpTooltip :content="t('admin.tokenAnalysis.includeUnmatchedHelp')" />
          </label>
        </div>

        <div class="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 pt-3 dark:border-dark-700">
          <div class="flex min-w-0 flex-wrap items-center gap-2" data-testid="token-analysis-active-filters">
            <template v-if="activeFilterChips.length">
              <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.tokenAnalysis.activeFilters') }}</span>
              <span
                v-for="chip in activeFilterChips"
                :key="chip.key"
                class="inline-flex max-w-[280px] items-center gap-1 rounded-full bg-primary-50 py-0.5 pl-2.5 pr-1 text-xs font-medium text-primary-700 dark:bg-primary-900/20 dark:text-primary-300"
              >
                <span class="truncate" :title="chip.label">{{ chip.label }}</span>
                <button
                  type="button"
                  class="rounded-full p-0.5 hover:bg-primary-100 dark:hover:bg-primary-900/40"
                  :aria-label="t('admin.tokenAnalysis.removeFilter', { name: chip.label })"
                  @click="clearFilter(chip.key)"
                >
                  <Icon name="x" size="xs" />
                </button>
              </span>
              <button type="button" class="text-xs font-medium text-gray-500 hover:text-gray-700 dark:text-dark-400 dark:hover:text-gray-200" @click="clearAllFilters">
                {{ t('admin.tokenAnalysis.clearFilters') }}
              </button>
            </template>
            <span v-else class="text-xs text-gray-400 dark:text-dark-500">{{ t('admin.tokenAnalysis.noActiveFilters') }}</span>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button
              type="button"
              class="inline-flex max-w-[360px] items-center gap-1.5 rounded-full border border-gray-200 px-2.5 py-1 text-xs text-gray-600 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
              :title="indexPill.title"
              data-testid="token-analysis-index-pill"
              @click="activeTab = 'index'"
            >
              <span class="h-2 w-2 shrink-0 rounded-full" :class="indexPill.dot" />
              <span class="truncate">{{ indexPill.text }}</span>
            </button>
            <button
              v-if="canTriggerIndex"
              type="button"
              class="btn btn-secondary btn-sm inline-flex items-center gap-1.5"
              :disabled="indexing"
              :title="t('admin.tokenAnalysis.indexRangeHint')"
              @click="triggerIndex"
            >
              <Icon name="refresh" size="xs" :class="{ 'animate-spin': indexing }" />
              {{ indexing ? t('common.loading') : t('admin.tokenAnalysis.indexNow') }}
            </button>
          </div>
        </div>
      </section>

      <TokenAnalysisOverview
        :summary="summary"
        :active-risk-reason="appliedFilters.risk_reason"
        @toggle-risk-reason="toggleRiskReason"
      />

      <div class="space-y-4">
        <div
          class="inline-flex max-w-full overflow-x-auto rounded-xl border border-gray-200 bg-white p-1 shadow-sm dark:border-dark-700 dark:bg-dark-800/60"
          role="tablist"
          :aria-label="t('admin.tokenAnalysis.title')"
        >
          <button
            v-for="tab in tabs"
            :id="`token-analysis-tab-${tab.key}`"
            :key="tab.key"
            :ref="(el) => setTabRef(tab.key, el)"
            type="button"
            role="tab"
            :aria-selected="activeTab === tab.key"
            :aria-controls="`token-analysis-panel-${tab.key}`"
            :tabindex="activeTab === tab.key ? 0 : -1"
            class="inline-flex shrink-0 items-center gap-2 whitespace-nowrap rounded-lg px-3.5 py-2 text-sm font-medium transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
            :class="activeTab === tab.key
              ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
              : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:text-gray-300 dark:hover:bg-dark-700 dark:hover:text-white'"
            :data-testid="`token-analysis-tab-${tab.key}`"
            @click="activeTab = tab.key"
            @keydown="onTabKeydown($event, tab.key)"
          >
            <Icon :name="tab.icon" size="sm" />
            {{ tab.label }}
            <span
              class="rounded-full px-1.5 text-xs tabular-nums"
              :class="activeTab === tab.key ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/50 dark:text-primary-200' : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'"
            >
              {{ tab.count }}
            </span>
            <span v-if="tab.alert" class="h-1.5 w-1.5 rounded-full bg-red-500" :aria-label="t('admin.tokenAnalysis.indexHasError')" />
          </button>
        </div>

        <div
          v-show="activeTab === 'users'"
          id="token-analysis-panel-users"
          role="tabpanel"
          aria-labelledby="token-analysis-tab-users"
          class="grid grid-cols-1 gap-4 xl:grid-cols-2"
        >
          <TokenAnalysisUserRanking
            :users="users"
            :pagination="usersPagination"
            :loading="usersLoading"
            :selected-users="selectedTrendUsers"
            @toggle="toggleTrendUser"
            @page="changeUserPage"
            @page-size="changeUserPageSize"
          />
          <TokenAnalysisUserTrend
            :selected-users="selectedTrendUsers"
            :points="userTrendPoints"
            :granularity="userTrendGranularity"
            :loading="userTrendLoading"
            :error="userTrendError"
            :can-use-hourly="canUseHourlyTrend"
            :start-date="appliedFilters.start_date || ''"
            :end-date="appliedFilters.end_date || ''"
            @granularity="setUserTrendGranularity"
            @retry="loadSelectedUserTrend"
            @remove="removeTrendUser"
            @clear="clearTrendUsers"
          />
        </div>

        <div v-show="activeTab === 'projects'" id="token-analysis-panel-projects" role="tabpanel" aria-labelledby="token-analysis-tab-projects">
          <TokenAnalysisProjectTable
            :projects="projects"
            :pagination="projectsPagination"
            :loading="projectsLoading"
            :sort-by="projectSort"
            :sort-order="projectSortOrder"
            :active-project="appliedFilters.project"
            @sort="changeProjectSort"
            @filter-project="toggleProjectFilter"
            @page="changeProjectPage"
            @page-size="changeProjectPageSize"
          />
        </div>

        <div v-show="activeTab === 'requests'" id="token-analysis-panel-requests" role="tabpanel" aria-labelledby="token-analysis-tab-requests">
          <TokenAnalysisRequestTable
            :requests="requests"
            :pagination="requestsPagination"
            :loading="requestsLoading"
            :sort="requestSort"
            :selected-id="selectedRequest?.id"
            @sort="changeRequestSort"
            @select="selectRequest"
            @page="changeRequestPage"
            @page-size="changeRequestPageSize"
          />
        </div>

        <div v-show="activeTab === 'index'" id="token-analysis-panel-index" role="tabpanel" aria-labelledby="token-analysis-tab-index">
          <TokenAnalysisIndexPanel :index-status="indexStatus" :archive-files="archiveFiles" />
        </div>
      </div>
    </div>

    <TokenAnalysisRequestDrawer
      :request="selectedRequest"
      :input="requestInput"
      :input-loading="requestInputLoading"
      @close="selectedRequest = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, type ComponentPublicInstance } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  TokenAnalysisArchiveFile,
  TokenAnalysisIndexStatus,
  TokenAnalysisProjectUsage,
  TokenAnalysisQueryParams,
  TokenAnalysisRequestInput,
  TokenAnalysisRequestItem,
  TokenAnalysisSummary,
  TokenAnalysisUserUsage
} from '@/api/admin/tokenAnalysis'
import AppLayout from '@/components/layout/AppLayout.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import TokenAnalysisIndexPanel from '@/components/admin/token-analysis/TokenAnalysisIndexPanel.vue'
import TokenAnalysisOverview from '@/components/admin/token-analysis/TokenAnalysisOverview.vue'
import TokenAnalysisProjectTable, {
  type TokenAnalysisProjectSortField
} from '@/components/admin/token-analysis/TokenAnalysisProjectTable.vue'
import TokenAnalysisRequestDrawer from '@/components/admin/token-analysis/TokenAnalysisRequestDrawer.vue'
import TokenAnalysisRequestTable, {
  type TokenAnalysisRequestSort
} from '@/components/admin/token-analysis/TokenAnalysisRequestTable.vue'
import TokenAnalysisUserRanking from '@/components/admin/token-analysis/TokenAnalysisUserRanking.vue'
import TokenAnalysisUserTrend from '@/components/admin/token-analysis/TokenAnalysisUserTrend.vue'
import {
  MAX_SELECTED_TREND_USERS,
  TOKEN_ANALYSIS_INDEX_DOT,
  formatRequestMetric,
  formatTimeCN,
  tokenAnalysisIndexState,
  useTokenAnalysisRisk,
  type TokenAnalysisSelectedUser,
  type TokenAnalysisTrendGranularity
} from '@/components/admin/token-analysis/tokenAnalysisFormat'
import { formatDateLocalInput } from '@/utils/format'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import type { UserUsageTrendPoint } from '@/types'

const { t } = useI18n()
const { riskLabel } = useTokenAnalysisRisk()
const appStore = useAppStore()
const authStore = useAuthStore()
const canTriggerIndex = computed(() => authStore.isAdmin)

function defaultFilters(): TokenAnalysisQueryParams {
  // 默认日期按浏览器本地日(与请求携带的 timezone 一致), 不用 UTC 日期。
  const today = formatDateLocalInput(new Date())
  return {
    start_date: today,
    end_date: today,
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    // 请求明细默认展示全部请求(风险排查时再调高风险分过滤)。
    risk_min: 0,
    include_unmatched: false
  }
}

const filters = reactive<TokenAnalysisQueryParams>(defaultFilters())
// 最近一次提交的筛选快照: 所有列表/概览/趋势请求、翻页排序续查、已应用筛选
// 标签和行内高亮都以它为准; 输入框里尚未提交的草稿不生效, 只有 reloadAll 更新它。
const appliedFilters = ref<TokenAnalysisQueryParams>({})
const showAdvanced = ref(false)

type TabKey = 'users' | 'projects' | 'requests' | 'index'
const activeTab = ref<TabKey>('users')

const summary = ref<TokenAnalysisSummary | null>(null)
const users = ref<TokenAnalysisUserUsage[]>([])
const projects = ref<TokenAnalysisProjectUsage[]>([])
const requests = ref<TokenAnalysisRequestItem[]>([])
const indexStatus = ref<TokenAnalysisIndexStatus | null>(null)
const archiveFiles = ref<TokenAnalysisArchiveFile[]>([])
const selectedRequest = ref<TokenAnalysisRequestItem | null>(null)
const requestSort = ref<TokenAnalysisRequestSort>('event_time')
// 项目排行支持按请求数/token/最近活动服务端排序(列表是分页的, 前端排序无意义)。
const projectSort = ref<TokenAnalysisProjectSortField>('total_tokens')
const projectSortOrder = ref<'asc' | 'desc'>('desc')
const requestInput = ref<TokenAnalysisRequestInput | null>(null)
const requestInputLoading = ref(false)
const usersLoading = ref(false)
const projectsLoading = ref(false)
const requestsLoading = ref(false)
const indexing = ref(false)

const usersPagination = reactive({ page: 1, page_size: 20, total: 0 })
const projectsPagination = reactive({ page: 1, page_size: 20, total: 0 })
const requestsPagination = reactive({ page: 1, page_size: 20, total: 0 })

const selectedTrendUsers = ref<TokenAnalysisSelectedUser[]>([])
const userTrendPoints = ref<UserUsageTrendPoint[]>([])
const userTrendGranularity = ref<TokenAnalysisTrendGranularity>('day')
const userTrendLoading = ref(false)
const userTrendError = ref(false)
let userTrendLoadSeq = 0

function compactFilters(source: TokenAnalysisQueryParams): TokenAnalysisQueryParams {
  return Object.fromEntries(
    Object.entries(source).filter(([, value]) => value !== '' && value !== undefined && value !== null)
  ) as TokenAnalysisQueryParams
}

const riskMinOptions = computed(() => [
  { value: 0, label: t('admin.tokenAnalysis.riskMinAll') },
  { value: 20, label: t('admin.tokenAnalysis.riskMinAtLeast', { n: 20 }) },
  { value: 40, label: t('admin.tokenAnalysis.riskMinAtLeast', { n: 40 }) },
  { value: 60, label: t('admin.tokenAnalysis.riskMinAtLeast', { n: 60 }) }
])

type IdFilterKey = 'user_id' | 'api_key_id' | 'account_id' | 'group_id'
const idFilterFields = computed<Array<{ key: IdFilterKey; label: string }>>(() => [
  { key: 'user_id', label: t('admin.tokenAnalysis.filterUserId') },
  { key: 'api_key_id', label: t('admin.tokenAnalysis.filterApiKeyId') },
  { key: 'account_id', label: t('admin.tokenAnalysis.filterAccountId') },
  { key: 'group_id', label: t('admin.tokenAnalysis.filterGroupId') }
])

const advancedActiveCount = computed(() => {
  const f = appliedFilters.value
  return idFilterFields.value.filter((field) => f[field.key]).length + (f.include_unmatched ? 1 : 0)
})

type FilterChipKey = 'user_email' | 'project' | 'model' | 'risk_reason' | 'risk_min' | 'include_unmatched' | IdFilterKey

const activeFilterChips = computed(() => {
  const f = appliedFilters.value
  const chips: Array<{ key: FilterChipKey; label: string }> = []
  if (f.user_email) chips.push({ key: 'user_email', label: `${t('admin.tokenAnalysis.userEmail')}: ${f.user_email}` })
  if (f.project) {
    const project = f.project === 'unattributed' ? t('admin.tokenAnalysis.unattributed') : f.project
    chips.push({ key: 'project', label: `${t('admin.tokenAnalysis.project')}: ${project}` })
  }
  if (f.model) chips.push({ key: 'model', label: `${t('admin.tokenAnalysis.model')}: ${f.model}` })
  if (f.risk_reason) chips.push({ key: 'risk_reason', label: `${t('admin.tokenAnalysis.riskReason')}: ${riskLabel(f.risk_reason)}` })
  if (f.risk_min) chips.push({ key: 'risk_min', label: t('admin.tokenAnalysis.riskMinAtLeast', { n: f.risk_min }) })
  for (const field of idFilterFields.value) {
    if (f[field.key]) chips.push({ key: field.key, label: `${field.label}: ${f[field.key]}` })
  }
  if (f.include_unmatched) chips.push({ key: 'include_unmatched', label: t('admin.tokenAnalysis.includeUnmatched') })
  return chips
})

const archiveNeedsAttention = computed(() => {
  return Boolean(indexStatus.value?.last_error) || archiveFiles.value.some((file) => file.status === 'attention')
})

const tabs = computed(() => [
  { key: 'users' as const, icon: 'users' as const, label: t('admin.tokenAnalysis.tabs.users'), count: formatRequestMetric(usersPagination.total), alert: false },
  { key: 'projects' as const, icon: 'cube' as const, label: t('admin.tokenAnalysis.tabs.projects'), count: formatRequestMetric(projectsPagination.total), alert: false },
  { key: 'requests' as const, icon: 'document' as const, label: t('admin.tokenAnalysis.tabs.requests'), count: formatRequestMetric(requestsPagination.total), alert: false },
  { key: 'index' as const, icon: 'database' as const, label: t('admin.tokenAnalysis.tabs.index'), count: formatRequestMetric(archiveFiles.value.length), alert: archiveNeedsAttention.value }
])

const tabRefs = new Map<TabKey, HTMLElement>()

function setTabRef(key: TabKey, el: Element | ComponentPublicInstance | null) {
  if (el instanceof HTMLElement) tabRefs.set(key, el)
  else tabRefs.delete(key)
}

// 方向键在标签间移动并激活, Home/End 跳到首尾(WAI-ARIA tabs 模式)。
function onTabKeydown(event: KeyboardEvent, key: TabKey) {
  const keys = tabs.value.map((tab) => tab.key)
  const index = keys.indexOf(key)
  let next = -1
  if (event.key === 'ArrowRight') next = (index + 1) % keys.length
  else if (event.key === 'ArrowLeft') next = (index - 1 + keys.length) % keys.length
  else if (event.key === 'Home') next = 0
  else if (event.key === 'End') next = keys.length - 1
  if (next < 0) return
  event.preventDefault()
  activeTab.value = keys[next]
  tabRefs.get(keys[next])?.focus()
}

const indexPill = computed(() => {
  const status = indexStatus.value
  const state = tokenAnalysisIndexState(status)
  const text = {
    running: t('admin.tokenAnalysis.indexRunning'),
    error: t('admin.tokenAnalysis.indexHasError'),
    idle: t('admin.tokenAnalysis.indexUpdatedAt', { time: formatTimeCN(status?.updated_at) })
  }[state]
  return { dot: TOKEN_ANALYSIS_INDEX_DOT[state], text, title: state === 'error' ? status?.last_error : text }
})

const canUseHourlyTrend = computed(() => {
  const { start_date, end_date } = appliedFilters.value
  return Boolean(start_date && end_date && start_date === end_date)
})

// 颜色槽位在选中时分配并固定: 取消某个用户不会让其余用户换色。
function toggleTrendUser(user: TokenAnalysisUserUsage) {
  const userID = Number(user.user_id || 0)
  if (userID <= 0) return
  const selectedIndex = selectedTrendUsers.value.findIndex((item) => item.user_id === userID)
  if (selectedIndex >= 0) {
    selectedTrendUsers.value.splice(selectedIndex, 1)
  } else if (selectedTrendUsers.value.length < MAX_SELECTED_TREND_USERS) {
    const used = new Set(selectedTrendUsers.value.map((item) => item.slot))
    const slot = Array.from({ length: MAX_SELECTED_TREND_USERS }, (_, index) => index).find((index) => !used.has(index)) ?? 0
    selectedTrendUsers.value.push({
      user_id: userID,
      email: user.user_email || `User ${userID}`,
      slot
    })
  }
  void loadSelectedUserTrend()
}

function removeTrendUser(userID: number) {
  selectedTrendUsers.value = selectedTrendUsers.value.filter((user) => user.user_id !== userID)
  void loadSelectedUserTrend()
}

function clearTrendUsers() {
  selectedTrendUsers.value = []
  void loadSelectedUserTrend()
}

function setUserTrendGranularity(granularity: TokenAnalysisTrendGranularity) {
  if (granularity === 'hour' && !canUseHourlyTrend.value) return
  if (userTrendGranularity.value === granularity) return
  userTrendGranularity.value = granularity
  void loadSelectedUserTrend()
}

async function loadSelectedUserTrend() {
  const seq = ++userTrendLoadSeq
  userTrendPoints.value = []
  userTrendError.value = false
  if (selectedTrendUsers.value.length === 0) {
    userTrendLoading.value = false
    return
  }
  const { start_date: startDate, end_date: endDate } = appliedFilters.value
  if (!startDate || !endDate) {
    userTrendLoading.value = false
    userTrendError.value = true
    return
  }
  if (userTrendGranularity.value === 'hour' && !canUseHourlyTrend.value) {
    userTrendGranularity.value = 'day'
  }

  userTrendLoading.value = true
  try {
    const response = await adminAPI.dashboard.getUserUsageTrend({
      user_ids: selectedTrendUsers.value.map((user) => user.user_id),
      start_date: startDate,
      end_date: endDate,
      granularity: userTrendGranularity.value
    })
    if (seq !== userTrendLoadSeq) return
    userTrendPoints.value = response.trend || []
  } catch {
    if (seq !== userTrendLoadSeq) return
    userTrendError.value = true
  } finally {
    if (seq === userTrendLoadSeq) userTrendLoading.value = false
  }
}

async function loadSummary() {
  summary.value = await adminAPI.tokenAnalysis.getSummary(appliedFilters.value)
}

async function loadUsers() {
  usersLoading.value = true
  try {
    const result = await adminAPI.tokenAnalysis.listUsers({
      ...appliedFilters.value,
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
      ...appliedFilters.value,
      page: projectsPagination.page,
      page_size: projectsPagination.page_size,
      sort_by: projectSort.value,
      sort_order: projectSortOrder.value
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
      ...appliedFilters.value,
      page: requestsPagination.page,
      page_size: requestsPagination.page_size,
      sort_by: requestSort.value,
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

function changeRequestSort(value: TokenAnalysisRequestSort) {
  if (requestSort.value === value) return
  requestSort.value = value
  requestsPagination.page = 1
  void loadRequests()
}

// 点已选中列翻转方向, 点新列重置为降序。
function changeProjectSort(field: TokenAnalysisProjectSortField) {
  if (projectSort.value === field) {
    projectSortOrder.value = projectSortOrder.value === 'desc' ? 'asc' : 'desc'
  } else {
    projectSort.value = field
    projectSortOrder.value = 'desc'
  }
  projectsPagination.page = 1
  void loadProjects()
}

// 点行打开抽屉并懒加载净输入全文(列表只带 300 字预览)。
function selectRequest(item: TokenAnalysisRequestItem) {
  selectedRequest.value = item
  requestInput.value = null
  if (!item.has_input) return
  requestInputLoading.value = true
  adminAPI.tokenAnalysis
    .getRequestInput(item.archive_id)
    .then((input) => {
      if (selectedRequest.value?.archive_id === item.archive_id) {
        requestInput.value = input
      }
    })
    .catch(() => {
      // 全文加载失败时抽屉回退展示预览, 不打断查看。
    })
    .finally(() => {
      // 仅当抽屉仍停留在本条记录时才收起 loading,
      // 避免旧请求的 finally 提前关掉新记录的 loading(竞态)。
      if (selectedRequest.value?.archive_id === item.archive_id) {
        requestInputLoading.value = false
      }
    })
}

async function loadIndexStatus() {
  indexStatus.value = await adminAPI.tokenAnalysis.getIndexStatus()
}

async function loadArchiveFiles() {
  archiveFiles.value = await adminAPI.tokenAnalysis.listArchiveFiles()
}

async function reloadAll() {
  appliedFilters.value = compactFilters(filters)
  usersPagination.page = 1
  requestsPagination.page = 1
  projectsPagination.page = 1
  if (userTrendGranularity.value === 'hour' && !canUseHourlyTrend.value) {
    userTrendGranularity.value = 'day'
  }
  try {
    const loaders = [loadSummary(), loadUsers(), loadProjects(), loadRequests(), loadIndexStatus(), loadArchiveFiles()]
    if (selectedTrendUsers.value.length > 0) {
      loaders.push(loadSelectedUserTrend())
    }
    await Promise.all(loaders)
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  }
}

function onDateRangeChange(range: { startDate: string; endDate: string }) {
  filters.start_date = range.startDate
  filters.end_date = range.endDate
  void reloadAll()
}

function onRiskMinChange(value: string | number | boolean | null) {
  filters.risk_min = Number(value) || 0
  void reloadAll()
}

function clearFilter(key: FilterChipKey) {
  if (key === 'risk_min') filters.risk_min = 0
  else if (key === 'include_unmatched') filters.include_unmatched = false
  else filters[key] = undefined
  void reloadAll()
}

function resetDimensionFilters(dates: Pick<TokenAnalysisQueryParams, 'start_date' | 'end_date'>) {
  Object.assign(filters, defaultFilters(), {
    user_email: undefined,
    project: undefined,
    model: undefined,
    risk_reason: undefined,
    user_id: undefined,
    api_key_id: undefined,
    account_id: undefined,
    group_id: undefined
  }, dates)
}

// 清除维度筛选, 保留当前日期范围。
function clearAllFilters() {
  resetDimensionFilters({ start_date: filters.start_date, end_date: filters.end_date })
  void reloadAll()
}

// 重置为进入页面时的状态(含日期回到今天)。
function resetFilters() {
  resetDimensionFilters({})
  void reloadAll()
}

// 索引触发已改异步(后端立即返回 202): 大范围回扫可达分钟级, 同步等待会撞
// axios 30s 超时报"网络错误"。触发后轮询索引状态, running 翻 false 即完成。
const INDEX_POLL_INTERVAL_MS = 3000
let indexPollTimer: ReturnType<typeof setTimeout> | null = null

function stopIndexPolling() {
  if (indexPollTimer !== null) {
    clearTimeout(indexPollTimer)
    indexPollTimer = null
  }
}

function scheduleIndexPoll() {
  stopIndexPolling()
  indexing.value = true
  indexPollTimer = setTimeout(() => void pollIndexStatus(), INDEX_POLL_INTERVAL_MS)
}

async function pollIndexStatus() {
  indexPollTimer = null
  try {
    await Promise.all([loadIndexStatus(), loadArchiveFiles()])
  } catch {
    // 单次轮询失败不终止(可能是瞬时网络抖动), 下一轮重试。
  }
  if (indexStatus.value?.running) {
    scheduleIndexPoll()
    return
  }
  indexing.value = false
  appStore.showSuccess(t('admin.tokenAnalysis.indexDone'))
  void reloadAll()
}

async function triggerIndex() {
  indexing.value = true
  try {
    await adminAPI.tokenAnalysis.triggerIndex({
      start_date: appliedFilters.value.start_date,
      end_date: appliedFilters.value.end_date,
      timezone: appliedFilters.value.timezone
    })
    appStore.showSuccess(t('admin.tokenAnalysis.indexStarted'))
    scheduleIndexPoll()
  } catch (error) {
    const err = error as { reason?: string; message?: string }
    if (err?.reason === 'TOKEN_ANALYSIS_INDEX_RUNNING') {
      // 自动索引或他人触发的轮次在跑: 跟进其进度而非报错收场。
      appStore.showError(t('admin.tokenAnalysis.indexAlreadyRunning'))
      scheduleIndexPoll()
      return
    }
    appStore.showError(err?.message || t('errors.networkError'))
    indexing.value = false
  }
}

function changeRequestPage(page: number) {
  requestsPagination.page = page
  void loadRequests()
}

function changeUserPage(page: number) {
  usersPagination.page = page
  void loadUsers()
}

function changeUserPageSize(pageSize: number) {
  usersPagination.page_size = pageSize
  usersPagination.page = 1
  void loadUsers()
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
  void reloadAll().then(() => {
    // 进页面时已有索引在跑(自动索引或他人触发): 直接跟进进度。
    if (indexStatus.value?.running) {
      scheduleIndexPoll()
    }
  })
})

onUnmounted(() => {
  userTrendLoadSeq++
  stopIndexPolling()
})
</script>
