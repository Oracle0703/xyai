<template>
  <AppLayout>
    <div class="mx-auto w-full max-w-[1800px] space-y-0" data-testid="organization-usage-view">
      <header class="pb-5">
        <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('admin.organizationUsage.title') }}</h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.organizationUsage.description') }}</p>
      </header>

      <OrganizationUsageFilters
        v-if="scope && canViewScope"
        :scope="scope"
        v-model="draft"
        :loading="loading || scopeLoading"
        :exporting="exporting"
        :export-disabled="hasPendingFilters || loading || scopeLoading || !canViewScope"
        @apply="applyFilters"
        @reset="resetFilters"
        @export="exportReport"
      />

      <div v-if="errorMessage" data-testid="load-error" class="my-5 flex items-center justify-between gap-4 border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-900/10 dark:text-red-300" role="alert">
        <span>{{ errorMessage }}</span>
        <button type="button" data-testid="retry-load" class="btn btn-secondary shrink-0" @click="loadFullReport()">
          <Icon name="refresh" size="sm" class="mr-1.5" />
          {{ t('admin.organizationUsage.actions.retry') }}
        </button>
      </div>

      <p v-if="scopeLoading" class="py-6 text-sm text-gray-500">{{ t('admin.departments.loadingScope') }}</p>
      <div v-else-if="scope && !canViewScope" class="flex items-center justify-between gap-3 py-6 text-sm">
        <span>{{ t('admin.departments.noDepartmentScope') }}</span><button class="btn btn-secondary" @click="loadFullReport()">{{ t('admin.departments.refresh') }}</button>
      </div>
      <p v-if="scope && canViewScope" class="pt-3 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.departments.attribution') }}</p>
      <template v-if="!errorMessage && canViewScope && !scopeLoading">
        <OrganizationUsageOverview
          v-if="report && !loading"
          :overview="report.overview"
          :champions="report.champions"
          :range="report.range"
        />
        <OrganizationUsageTrendChart
          :points="trendPoints"
          :granularity="effectiveGranularity"
          :loading="trendLoading"
          :error="trendError"
          :range="trendMeta?.range ?? null"
          :data-through="trendMeta?.data_through"
          @update:granularity="changeGranularity"
          @retry="retryTrend"
        />
        <OrganizationUsageSummary
          v-if="report && !loading"
          :organizations="report.organizations"
          :selected-organization="applied.organization"
          @select="selectOrganization"
        />
        <OrganizationUsageBreakdowns v-if="report && !loading" :departments="report.departments ?? []" :platforms="report.platforms ?? []" @department="selectDepartment" @platform="selectPlatform" />
        <OrganizationUsagePeopleTable
          :items="report?.items ?? []"
          :range="report?.range"
          :pagination="pagination"
          :loading="loading"
          :sort-by="sortBy"
          :sort-order="sortOrder"
          @sort="changeSort"
          @page="changePage"
          @page-size="changePageSize"
        />
      </template>
    </div>
  </AppLayout>

  <UsageExportProgress
    :show="exportProgress.show"
    :progress="exportProgress.progress"
    :current="exportProgress.current"
    :total="exportProgress.total"
    :estimated-time="exportProgress.estimatedTime"
    @cancel="cancelExport"
  />
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { departmentErrorStatus, isDepartmentScopeChanged } from '@/utils/departmentErrors'
import { departmentsAPI, type DepartmentScope } from '@/api/admin/departments'
import { saveAs } from 'file-saver'

import { adminAPI } from '@/api/admin'
import type {
  OrganizationUsageGranularity,
  OrganizationUsageOrganizationFilter,
  OrganizationUsagePagination,
  OrganizationUsageRange,
  OrganizationUsageSortBy,
  OrganizationUsageSortOrder,
  OrganizationUsageSummaryQuery,
  OrganizationUsageSummaryResponse,
  OrganizationUsageTrendPoint,
  OrganizationUsageTrendResponse
} from '@/api/admin/organizationUsage'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import UsageExportProgress from '@/components/admin/usage/UsageExportProgress.vue'
import OrganizationUsageFilters, {
  type OrganizationUsageFilterDraft
} from '@/components/admin/organization-usage/OrganizationUsageFilters.vue'
import OrganizationUsageOverview from '@/components/admin/organization-usage/OrganizationUsageOverview.vue'
import OrganizationUsageTrendChart from '@/components/admin/organization-usage/OrganizationUsageTrendChart.vue'
import OrganizationUsageSummary from '@/components/admin/organization-usage/OrganizationUsageSummary.vue'
import OrganizationUsageBreakdowns from '@/components/admin/organization-usage/OrganizationUsageBreakdowns.vue'
import OrganizationUsagePeopleTable from '@/components/admin/organization-usage/OrganizationUsagePeopleTable.vue'
import { useAppStore } from '@/stores/app'
import {
  getBusinessDateString,
  getDefaultOrganizationUsageRange,
  getOrganizationUsageExportFileName,
  inferOrganizationUsageTrendGranularity
} from '@/utils/organizationUsageReport'
import { generateOrganizationUsageWorkbook } from '@/utils/organizationUsageExportWorker'

const { t } = useI18n()
const appStore = useAppStore()
const route = useRoute()
const scope = ref<DepartmentScope | null>(null)
const scopeLoading = ref(true)
const scopeVersion = ref('')
const canViewScope = computed(() => !!scope.value && (scope.value.unrestricted || scope.value.departments.length > 0))
let scopeController: AbortController | null = null
let scopeInitialized = false
let scopeRetryAvailable = true

function initialDraft(): OrganizationUsageFilterDraft {
  const today = getBusinessDateString()
  const monthRange = getDefaultOrganizationUsageRange('month', today)
  const customRange = getDefaultOrganizationUsageRange('custom', today)
  return {
    mode: 'month',
    month: monthRange.start_date.slice(0, 7),
    weekAnchor: today,
    customStart: customRange.start_date,
    customEnd: customRange.end_date,
    organization: scope.value?.default_organization ?? 'all',
    department_id: scope.value?.default_department_id ?? 'all',
    platform: 'all',
    q: ''
  }
}

const draft = ref<OrganizationUsageFilterDraft>(initialDraft())

function filterFingerprint(value: OrganizationUsageFilterDraft): string {
  const activeDateValues = value.mode === 'month'
    ? [value.month]
    : value.mode === 'week'
      ? [value.weekAnchor]
      : [value.customStart, value.customEnd]
  return JSON.stringify([value.mode, ...activeDateValues, value.organization, value.department_id ?? 'all', value.platform ?? 'all', value.q.trim()])
}

let appliedFilterDraft = { ...draft.value }
const appliedFilterFingerprint = ref(filterFingerprint(appliedFilterDraft))
const hasPendingFilters = computed(() => filterFingerprint(draft.value) !== appliedFilterFingerprint.value)

function saveAppliedFilterFingerprint(value: OrganizationUsageFilterDraft) {
  appliedFilterDraft = { ...value }
  appliedFilterFingerprint.value = filterFingerprint(appliedFilterDraft)
}

const initialRange = getDefaultOrganizationUsageRange('month')
const applied = reactive({
  ...initialRange,
  organization: 'all' as OrganizationUsageOrganizationFilter,
  department_id: 'all',
  platform: 'all',
  q: ''
})
const sortBy = ref<OrganizationUsageSortBy>('total_tokens')
const sortOrder = ref<OrganizationUsageSortOrder>('desc')
const pagination = reactive<OrganizationUsagePagination>({ total: 0, page: 1, page_size: 20, pages: 0 })
const report = ref<OrganizationUsageSummaryResponse | null>(null)
const loading = ref(false)
const errorMessage = ref('')

// Summary establishes the snapshot; paging and Trend reuse it.
const reportCycleId = ref(0)
const snapshotAsOf = ref('')
const reconciledCycleId = ref<number | null>(null)
const trendLoading = ref(false)
const trendError = ref('')
const trendPoints = ref<OrganizationUsageTrendPoint[]>([])
const trendMeta = ref<Pick<OrganizationUsageTrendResponse, 'range' | 'granularity' | 'data_through' | 'scope_version'> | null>(null)
const granularityMode = ref<'auto' | 'manual'>('auto')
const effectiveGranularity = ref<OrganizationUsageGranularity>(
  inferOrganizationUsageTrendGranularity(initialRange.start_date, initialRange.end_date)
)

let reportController: AbortController | null = null
let trendController: AbortController | null = null
let exportController: AbortController | null = null
let userCanceledExport = false
let isUnmounted = false
const exporting = ref(false)
const exportProgress = reactive({ show: false, progress: 0, current: 0, total: 5, estimatedTime: '' })

function currentQuery(asOf?: string): OrganizationUsageSummaryQuery {
  const query: OrganizationUsageSummaryQuery = {
    start_date: applied.start_date,
    end_date: applied.end_date,
    organization: applied.organization,
    department_id: applied.department_id,
    platform: applied.platform,
    ...(scopeVersion.value ? { scope_version: scopeVersion.value } : {}),
    page: pagination.page,
    page_size: pagination.page_size,
    sort_by: sortBy.value,
    sort_order: sortOrder.value
  }
  const q = applied.q.trim()
  if (q) query.q = q
  if (asOf) query.as_of = asOf
  return query
}

function resolveAutoGranularity() {
  if (granularityMode.value === 'auto') {
    effectiveGranularity.value = inferOrganizationUsageTrendGranularity(applied.start_date, applied.end_date)
  }
}

async function loadSummaryOnly(asOf: string, cycleId: number) {
  reportController?.abort()
  const controller = new AbortController()
  reportController = controller
  loading.value = true
  errorMessage.value = ''
  const requestedVersion = scopeVersion.value
  try {
    const response = await adminAPI.organizationUsage.getSummary(currentQuery(asOf), { signal: controller.signal })
    if (reportController !== controller || controller.signal.aborted || reportCycleId.value !== cycleId) return
    if (!response.scope_version || (requestedVersion && response.scope_version !== requestedVersion)) throw new Error('REPORT_SCOPE_CHANGED')
    if (!response.range.as_of || (requestedVersion && response.range.as_of !== asOf)) {
      errorMessage.value = t('admin.organizationUsage.feedback.loadFailed')
      report.value = null
      return
    }
    snapshotAsOf.value = response.range.as_of
    scopeVersion.value = response.scope_version
    report.value = response
    Object.assign(pagination, response.pagination)
    if (!requestedVersion) void loadTrend(response.range.as_of, cycleId)
  } catch (cause) {
    if (controller.signal.aborted || reportController !== controller || reportCycleId.value !== cycleId) return
    if (handleScopeFailure(cause, cycleId)) return
    errorMessage.value = t('admin.organizationUsage.feedback.loadFailed')
    report.value = null
  } finally {
    if (reportController === controller) {
      loading.value = false
      reportController = null
    }
  }
}

function clearTrendSuccessState() {
  trendPoints.value = []
  trendMeta.value = null
}

// One automatic retry per report cycle; an invalid trend never replaces valid Summary data.
async function loadTrend(asOf: string, cycleId: number) {
  if (!scopeVersion.value) return
  trendController?.abort()
  const controller = new AbortController()
  trendController = controller
  trendLoading.value = true
  trendError.value = ''
  try {
    const q = applied.q.trim()
    const response = await adminAPI.organizationUsage.getTrend(
      {
        start_date: applied.start_date,
        end_date: applied.end_date,
        organization: applied.organization,
        department_id: applied.department_id,
        platform: applied.platform,
        scope_version: scopeVersion.value,
        granularity: effectiveGranularity.value,
        as_of: asOf,
        ...(q ? { q } : {})
      },
      { signal: controller.signal }
    )
    if (trendController !== controller || controller.signal.aborted || reportCycleId.value !== cycleId) return

    if (response.scope_version !== scopeVersion.value) throw new Error('REPORT_SCOPE_CHANGED')
    if (response.range?.as_of?.trim() !== asOf) throw new Error('Invalid trend snapshot')

    trendPoints.value = response.points
    trendMeta.value = {
      range: response.range,
      granularity: response.granularity,
      data_through: response.data_through,
      scope_version: response.scope_version
    }
    trendError.value = ''
  } catch (cause) {
    if (controller.signal.aborted || trendController !== controller || reportCycleId.value !== cycleId) return
    if (handleScopeFailure(cause, cycleId)) return
    clearTrendSuccessState()
    trendError.value = t('admin.organizationUsage.trend.loadFailed')
    if (reconciledCycleId.value !== cycleId) {
      reconciledCycleId.value = cycleId
      void loadTrend(asOf, cycleId)
    }
  } finally {
    if (trendController === controller) {
      trendLoading.value = false
      trendController = null
    }
  }
}

function selectionAllowed(current: DepartmentScope, organization: string, department: string) {
  if (organization !== 'all' && !current.organizations.includes(organization as DepartmentScope['organizations'][number])) return false
  if (department === 'all') return true
  if (organization === 'all') return false
  if (department === 'unassigned') return current.unrestricted
  return current.departments.some(item => String(item.id) === department && item.organization_key === organization)
}

function handleScopeFailure(cause: unknown, cycleId: number) {
  const status = departmentErrorStatus(cause)
  const changed = isDepartmentScopeChanged(cause)
  if (!changed && status !== 403) return false
  if (cycleId !== reportCycleId.value) return true
  reportController?.abort()
  trendController?.abort()
  exportController?.abort()
  report.value = null
  clearTrendSuccessState()
  if (changed && scopeRetryAvailable) {
    scopeRetryAvailable = false
    void loadFullReport(false)
  } else {
    ++reportCycleId.value
    scope.value = null
    scopeVersion.value = ''
    loading.value = false
    trendLoading.value = false
    errorMessage.value = t('admin.departments.scopeChanged')
  }
  return true
}

async function loadFullReport(allowScopeRetry = true) {
  reportController?.abort()
  reportController = null
  trendController?.abort()
  scopeController?.abort()
  const controller = new AbortController()
  scopeController = controller
  const cycleId = ++reportCycleId.value
  scopeRetryAvailable = allowScopeRetry
  scopeLoading.value = true
  loading.value = true
  errorMessage.value = ''
  report.value = null
  scopeVersion.value = ''
  snapshotAsOf.value = ''
  clearTrendSuccessState()
  try {
    const currentScope = await departmentsAPI.reportScope(controller.signal)
    if (controller.signal.aborted || cycleId !== reportCycleId.value) return
    if (!currentScope.catalog_version) throw new Error('missing catalog version')
    scope.value = currentScope
    if (!scopeInitialized) {
      draft.value = initialDraft()
      const query = route?.query ?? {}
      if (typeof query.organization === 'string') draft.value.organization = query.organization as OrganizationUsageOrganizationFilter
      if (typeof query.department_id === 'string') draft.value.department_id = query.department_id
      if (typeof query.q === 'string') draft.value.q = query.q
      Object.assign(applied, { organization: draft.value.organization, department_id: draft.value.department_id ?? 'all', platform: draft.value.platform ?? 'all', q: draft.value.q })
      saveAppliedFilterFingerprint(draft.value)
      scopeInitialized = true
    }
    if (!canViewScope.value) { loading.value = false; return }
    if (!selectionAllowed(currentScope, applied.organization, applied.department_id)) {
      loading.value = false
      errorMessage.value = t('admin.departments.scopeChanged')
      return
    }
    const candidateAsOf = new Date().toISOString()
    snapshotAsOf.value = candidateAsOf
    reconciledCycleId.value = null
    trendError.value = ''
    resolveAutoGranularity()
    void loadSummaryOnly(candidateAsOf, cycleId)
  } catch {
    if (!controller.signal.aborted && cycleId === reportCycleId.value) {
      scope.value = null
      loading.value = false
      errorMessage.value = t('admin.departments.failed')
    }
  } finally {
    if (scopeController === controller) { scopeLoading.value = false; scopeController = null }
  }
}

/** People table sort/page: summary only, keep trend, reuse snapshot. */
function loadReport() {
  if (!snapshotAsOf.value) {
    loadFullReport()
    return
  }
  void loadSummaryOnly(snapshotAsOf.value, reportCycleId.value)
}

function applyFilters(range: OrganizationUsageRange) {
  const dateChanged = range.start_date !== applied.start_date || range.end_date !== applied.end_date
  Object.assign(applied, range, {
    organization: draft.value.organization,
    department_id: draft.value.department_id ?? 'all',
    platform: draft.value.platform ?? 'all',
    q: draft.value.q
  })
  saveAppliedFilterFingerprint(draft.value)
  if (dateChanged) granularityMode.value = 'auto'
  pagination.page = 1
  loadFullReport()
}

function resetFilters() {
  draft.value = initialDraft()
  const range = getDefaultOrganizationUsageRange('month')
  Object.assign(applied, range, { organization: draft.value.organization, department_id: draft.value.department_id ?? 'all', platform: 'all', q: '' })
  saveAppliedFilterFingerprint(draft.value)
  sortBy.value = 'total_tokens'
  sortOrder.value = 'desc'
  pagination.page = 1
  pagination.page_size = 20
  granularityMode.value = 'auto'
  loadFullReport()
}

function selectOrganization(organization: Exclude<OrganizationUsageOrganizationFilter, 'all'>) {
  draft.value = { ...draft.value, organization, department_id: 'all' }
  applied.organization = organization
  applied.department_id = 'all'
  saveAppliedFilterFingerprint({ ...appliedFilterDraft, organization, department_id: 'all' })
  pagination.page = 1
  // Keep granularity mode when only org changes
  loadFullReport()
}

function selectDepartment(organization: string, departmentID: string) {
  draft.value = { ...draft.value, organization: organization as OrganizationUsageOrganizationFilter, department_id: departmentID }
  applied.organization = organization as OrganizationUsageOrganizationFilter
  applied.department_id = departmentID
  saveAppliedFilterFingerprint({ ...appliedFilterDraft, organization: applied.organization, department_id: departmentID })
  pagination.page = 1
  void loadFullReport()
}

function selectPlatform(platform: string) {
  draft.value = { ...draft.value, platform }
  applied.platform = platform
  saveAppliedFilterFingerprint({ ...appliedFilterDraft, platform })
  pagination.page = 1
  void loadFullReport()
}

function changeSort(nextSortBy: OrganizationUsageSortBy, nextSortOrder: OrganizationUsageSortOrder) {
  sortBy.value = nextSortBy
  sortOrder.value = nextSortOrder
  pagination.page = 1
  loadReport()
}

function changePage(page: number) {
  pagination.page = page
  loadReport()
}

function changePageSize(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  loadReport()
}

function changeGranularity(value: OrganizationUsageGranularity) {
  granularityMode.value = 'manual'
  effectiveGranularity.value = value
  if (!snapshotAsOf.value) {
    loadFullReport()
    return
  }
  void loadTrend(snapshotAsOf.value, reportCycleId.value)
}

function retryTrend() {
  if (!snapshotAsOf.value) {
    loadFullReport()
    return
  }
  void loadTrend(snapshotAsOf.value, reportCycleId.value)
}

function cancelExport() {
  userCanceledExport = true
  exportController?.abort()
}

function isCapacityError(error: unknown) {
  return error instanceof Error && /(?:client export|Excel|row limit|exceeds).*(?:row|limit)|row limit/i.test(error.message)
}

async function exportReport() {
  if (exporting.value || hasPendingFilters.value || !canViewScope.value || scopeLoading.value || loading.value) return
  const controller = new AbortController()
  exportController = controller
  userCanceledExport = false
  exporting.value = true
  Object.assign(exportProgress, {
    show: true,
    progress: 0,
    current: 0,
    total: 5,
    estimatedTime: t('admin.organizationUsage.feedback.exportPreparing')
  })

  const { page: _page, page_size: _pageSize, ...query } = currentQuery()
  try {
    const data = await adminAPI.organizationUsage.fetchAll(query, {
      signal: controller.signal,
      onProgress: ({ completed, total }) => {
        exportProgress.current = completed
        exportProgress.total = total + 1
        exportProgress.progress = Math.min(100, Math.round(completed / (total + 1) * 100))
      }
    })
    if (controller.signal.aborted) return
    exportProgress.total = Math.max(exportProgress.total, exportProgress.current + 1)
    exportProgress.estimatedTime = t('admin.organizationUsage.feedback.generatingWorkbook')
    const bytes = await generateOrganizationUsageWorkbook(data, {
      signal: controller.signal,
      onStage: () => {
        exportProgress.estimatedTime = t('admin.organizationUsage.feedback.generatingWorkbook')
      }
    })
    if (controller.signal.aborted) return
    exportProgress.current = exportProgress.total
    exportProgress.progress = 100
    saveAs(
      new Blob([bytes], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }),
      getOrganizationUsageExportFileName(query.start_date, query.end_date)
    )
    appStore.showSuccess(t('admin.organizationUsage.feedback.exportSuccess'))
  } catch (error) {
    if (controller.signal.aborted || (error instanceof Error && error.name === 'AbortError')) {
      if (userCanceledExport && !isUnmounted) {
        appStore.showInfo(t('admin.organizationUsage.feedback.exportCanceled'))
      }
    } else if (departmentErrorStatus(error) === 403 || isDepartmentScopeChanged(error)) {
      appStore.showError(t('admin.departments.scopeChanged'))
      report.value = null
      clearTrendSuccessState()
    } else if (isCapacityError(error)) {
      appStore.showError(t('admin.organizationUsage.feedback.exportTooLarge'))
    } else {
      appStore.showError(t('admin.organizationUsage.feedback.exportFailed'))
    }
  } finally {
    if (exportController === controller) {
      exportController = null
      exporting.value = false
      exportProgress.show = false
    }
  }
}

onMounted(() => { void loadFullReport() })
onUnmounted(() => {
  isUnmounted = true
  scopeController?.abort()
  reportController?.abort()
  trendController?.abort()
  exportController?.abort()
})
</script>
