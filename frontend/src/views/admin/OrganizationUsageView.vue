<template>
  <AppLayout>
    <div class="mx-auto w-full max-w-[1800px] space-y-0" data-testid="organization-usage-view">
      <header class="pb-5">
        <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('admin.organizationUsage.title') }}</h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.organizationUsage.description') }}</p>
      </header>

      <OrganizationUsageFilters
        v-model="draft"
        :loading="loading"
        :exporting="exporting"
        @apply="applyFilters"
        @reset="resetFilters"
        @export="exportReport"
      />

      <div v-if="errorMessage" data-testid="load-error" class="my-5 flex items-center justify-between gap-4 border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-900/10 dark:text-red-300" role="alert">
        <span>{{ errorMessage }}</span>
        <button type="button" data-testid="retry-load" class="btn btn-secondary shrink-0" @click="loadFullReport">
          <Icon name="refresh" size="sm" class="mr-1.5" />
          {{ t('admin.organizationUsage.actions.retry') }}
        </button>
      </div>

      <template v-if="!errorMessage">
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
        <OrganizationUsagePeopleTable
          :items="report?.items ?? []"
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
import { onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
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
    organization: 'all',
    q: ''
  }
}

const draft = ref<OrganizationUsageFilterDraft>(initialDraft())
const initialRange = getDefaultOrganizationUsageRange('month')
const applied = reactive({
  ...initialRange,
  organization: 'all' as OrganizationUsageOrganizationFilter,
  q: ''
})
const sortBy = ref<OrganizationUsageSortBy>('total_tokens')
const sortOrder = ref<OrganizationUsageSortOrder>('desc')
const pagination = reactive<OrganizationUsagePagination>({ total: 0, page: 1, page_size: 20, pages: 0 })
const report = ref<OrganizationUsageSummaryResponse | null>(null)
const loading = ref(false)
const errorMessage = ref('')

// Shared snapshot + dual-controller state (see organization-usage-trend-chart-design-cn.md K6/K7)
const reportCycleId = ref(0)
const snapshotAsOf = ref('')
const reconciledCycleId = ref<number | null>(null)
const trendRequestedAsOf = ref('')
const trendLoading = ref(false)
const trendError = ref('')
const trendPoints = ref<OrganizationUsageTrendPoint[]>([])
const trendMeta = ref<Pick<OrganizationUsageTrendResponse, 'range' | 'granularity' | 'data_through'> | null>(null)
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
  try {
    const response = await adminAPI.organizationUsage.getSummary(currentQuery(asOf), { signal: controller.signal })
    if (reportController !== controller || controller.signal.aborted || reportCycleId.value !== cycleId) return
    if (!response.range.as_of) {
      errorMessage.value = t('admin.organizationUsage.feedback.loadFailed')
      report.value = null
      return
    }
    snapshotAsOf.value = response.range.as_of
    report.value = response
    Object.assign(pagination, response.pagination)
    maybeReconcileTrend(cycleId)
  } catch {
    if (controller.signal.aborted || reportController !== controller || reportCycleId.value !== cycleId) return
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

function failTrendLocally(clearPoints: boolean) {
  if (clearPoints) clearTrendSuccessState()
  trendError.value = t('admin.organizationUsage.trend.loadFailed')
}

/**
 * Load trend series.
 *
 * Acceptance rule once Summary canonical exists: displayability is decided only by
 * response.range.as_of strictly equaling that canonical — never by request params alone.
 *
 * Parallel fast path: if Summary has not returned yet, a non-empty response as_of may be
 * stored as provisional until Summary arrives.
 */
async function loadTrend(asOf: string, cycleId: number, options?: { clearOnError?: boolean }) {
  const clearOnError = options?.clearOnError !== false
  trendController?.abort()
  const controller = new AbortController()
  trendController = controller
  // Requested as_of is only for in-flight abort decisions, not final equality checks.
  trendRequestedAsOf.value = asOf
  trendLoading.value = true
  trendError.value = ''
  try {
    const q = applied.q.trim()
    const response = await adminAPI.organizationUsage.getTrend(
      {
        start_date: applied.start_date,
        end_date: applied.end_date,
        organization: applied.organization,
        granularity: effectiveGranularity.value,
        as_of: asOf,
        ...(q ? { q } : {})
      },
      { signal: controller.signal }
    )
    if (trendController !== controller || controller.signal.aborted || reportCycleId.value !== cycleId) return

    const responseAsOf = response.range?.as_of?.trim() ?? ''
    if (!responseAsOf) {
      // Missing response canonical is never displayable.
      failTrendLocally(true)
      scheduleTrendReconcileIfNeeded(cycleId)
      return
    }

    const summaryCanonical = report.value?.range.as_of?.trim() ?? ''
    if (summaryCanonical && responseAsOf !== summaryCanonical) {
      // Do not write or keep a mismatched response once Summary canonical exists.
      if (reconciledCycleId.value === cycleId) {
        failTrendLocally(true)
        return
      }
      reconciledCycleId.value = cycleId
      void loadTrend(summaryCanonical, cycleId, { clearOnError: true })
      return
    }

    // Provisional (no Summary yet) or strictly aligned with Summary canonical.
    trendPoints.value = response.points
    trendMeta.value = {
      range: response.range,
      granularity: response.granularity,
      data_through: response.data_through
    }
    trendError.value = ''
  } catch {
    if (controller.signal.aborted || trendController !== controller || reportCycleId.value !== cycleId) return
    failTrendLocally(clearOnError)
    scheduleTrendReconcileIfNeeded(cycleId)
  } finally {
    if (trendController === controller) {
      trendLoading.value = false
      trendController = null
    }
  }
}

/**
 * When Summary arrives after/with Trend: align stored or in-flight work to canonical.
 * trendRequestedAsOf only decides whether to abort an in-flight request early.
 */
function maybeReconcileTrend(cycleId: number) {
  if (reportCycleId.value !== cycleId || reconciledCycleId.value === cycleId) return
  const summaryCanonical = report.value?.range.as_of?.trim() ?? ''
  if (!summaryCanonical) return

  if (trendLoading.value) {
    // In-flight request param differs from canonical → abort and reload once.
    // If param already equals canonical, wait for the response; loadTrend compares response as_of.
    if (trendRequestedAsOf.value !== summaryCanonical) {
      reconciledCycleId.value = cycleId
      void loadTrend(summaryCanonical, cycleId, { clearOnError: true })
    }
    return
  }

  const storedAsOf = trendMeta.value?.range.as_of?.trim() ?? ''
  if (storedAsOf === summaryCanonical) return

  // No aligned success yet (empty, error, or provisional mismatch).
  reconciledCycleId.value = cycleId
  void loadTrend(summaryCanonical, cycleId, { clearOnError: true })
}

/** After a failed/missing response, one reconcile is still allowed if Summary is already canonical. */
function scheduleTrendReconcileIfNeeded(cycleId: number) {
  if (reportCycleId.value !== cycleId || reconciledCycleId.value === cycleId) return
  const summaryCanonical = report.value?.range.as_of?.trim() ?? ''
  if (!summaryCanonical) return
  reconciledCycleId.value = cycleId
  void loadTrend(summaryCanonical, cycleId, { clearOnError: true })
}

function loadFullReport() {
  reportController?.abort()
  trendController?.abort()
  reportCycleId.value += 1
  const cycleId = reportCycleId.value
  const candidateAsOf = new Date().toISOString()
  snapshotAsOf.value = candidateAsOf
  reconciledCycleId.value = null
  trendRequestedAsOf.value = candidateAsOf
  trendPoints.value = []
  trendMeta.value = null
  trendError.value = ''
  resolveAutoGranularity()
  void loadSummaryOnly(candidateAsOf, cycleId)
  void loadTrend(candidateAsOf, cycleId)
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
    q: draft.value.q
  })
  if (dateChanged) granularityMode.value = 'auto'
  pagination.page = 1
  loadFullReport()
}

function resetFilters() {
  draft.value = initialDraft()
  const range = getDefaultOrganizationUsageRange('month')
  Object.assign(applied, range, { organization: 'all', q: '' })
  sortBy.value = 'total_tokens'
  sortOrder.value = 'desc'
  pagination.page = 1
  pagination.page_size = 20
  granularityMode.value = 'auto'
  loadFullReport()
}

function selectOrganization(organization: Exclude<OrganizationUsageOrganizationFilter, 'all'>) {
  draft.value = { ...draft.value, organization }
  applied.organization = organization
  pagination.page = 1
  // Keep granularity mode when only org changes
  loadFullReport()
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
  if (exporting.value) return
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

onMounted(loadFullReport)
onUnmounted(() => {
  isUnmounted = true
  reportController?.abort()
  trendController?.abort()
  exportController?.abort()
})
</script>
