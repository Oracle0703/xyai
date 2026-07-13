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
        <button type="button" data-testid="retry-load" class="btn btn-secondary shrink-0" @click="loadReport">
          <Icon name="refresh" size="sm" class="mr-1.5" />
          {{ t('admin.organizationUsage.actions.retry') }}
        </button>
      </div>

      <template v-if="!errorMessage">
        <OrganizationUsageOverview
          v-if="report"
          :overview="report.overview"
          :champions="report.champions"
          :range="report.range"
        />
        <OrganizationUsageSummary
          v-if="report"
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
  OrganizationUsageOrganizationFilter,
  OrganizationUsagePagination,
  OrganizationUsageRange,
  OrganizationUsageSortBy,
  OrganizationUsageSortOrder,
  OrganizationUsageSummaryQuery,
  OrganizationUsageSummaryResponse
} from '@/api/admin/organizationUsage'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import UsageExportProgress from '@/components/admin/usage/UsageExportProgress.vue'
import OrganizationUsageFilters, {
  type OrganizationUsageFilterDraft
} from '@/components/admin/organization-usage/OrganizationUsageFilters.vue'
import OrganizationUsageOverview from '@/components/admin/organization-usage/OrganizationUsageOverview.vue'
import OrganizationUsageSummary from '@/components/admin/organization-usage/OrganizationUsageSummary.vue'
import OrganizationUsagePeopleTable from '@/components/admin/organization-usage/OrganizationUsagePeopleTable.vue'
import { useAppStore } from '@/stores/app'
import {
  getBusinessDateString,
  getDefaultOrganizationUsageRange,
  getOrganizationUsageExportFileName
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

let reportController: AbortController | null = null
let exportController: AbortController | null = null
let userCanceledExport = false
let isUnmounted = false
const exporting = ref(false)
const exportProgress = reactive({ show: false, progress: 0, current: 0, total: 5, estimatedTime: '' })

function currentQuery(): OrganizationUsageSummaryQuery {
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
  return query
}

async function loadReport() {
  reportController?.abort()
  const controller = new AbortController()
  reportController = controller
  loading.value = true
  errorMessage.value = ''
  try {
    const response = await adminAPI.organizationUsage.getSummary(currentQuery(), { signal: controller.signal })
    if (reportController !== controller || controller.signal.aborted) return
    report.value = response
    Object.assign(pagination, response.pagination)
  } catch (error) {
    if (controller.signal.aborted || reportController !== controller) return
    errorMessage.value = t('admin.organizationUsage.feedback.loadFailed')
  } finally {
    if (reportController === controller) {
      loading.value = false
      reportController = null
    }
  }
}

function applyFilters(range: OrganizationUsageRange) {
  Object.assign(applied, range, {
    organization: draft.value.organization,
    q: draft.value.q
  })
  pagination.page = 1
  void loadReport()
}

function resetFilters() {
  draft.value = initialDraft()
  const range = getDefaultOrganizationUsageRange('month')
  Object.assign(applied, range, { organization: 'all', q: '' })
  sortBy.value = 'total_tokens'
  sortOrder.value = 'desc'
  pagination.page = 1
  pagination.page_size = 20
  void loadReport()
}

function selectOrganization(organization: Exclude<OrganizationUsageOrganizationFilter, 'all'>) {
  draft.value = { ...draft.value, organization }
  applied.organization = organization
  pagination.page = 1
  void loadReport()
}

function changeSort(nextSortBy: OrganizationUsageSortBy, nextSortOrder: OrganizationUsageSortOrder) {
  sortBy.value = nextSortBy
  sortOrder.value = nextSortOrder
  pagination.page = 1
  void loadReport()
}

function changePage(page: number) {
  pagination.page = page
  void loadReport()
}

function changePageSize(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  void loadReport()
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

onMounted(loadReport)
onUnmounted(() => {
  isUnmounted = true
  reportController?.abort()
  exportController?.abort()
})
</script>
