<template>
  <section class="card overflow-hidden">
    <div class="org-usage-card-header">
      <div class="flex items-baseline gap-2">
        <h3 class="org-usage-card-title">{{ t('admin.organizationUsage.people.title') }}</h3>
        <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.organizationUsage.people.total', { count: pagination.total }) }}</span>
      </div>
      <button
        type="button"
        data-testid="people-screenshot"
        class="btn btn-secondary btn-sm"
        :disabled="loading || capturing || items.length === 0"
        :aria-busy="capturing"
        :title="t('admin.organizationUsage.people.screenshotHint')"
        @click="downloadScreenshot"
      >
        <Icon name="download" size="sm" class="mr-1.5" />
        {{ t(capturing ? 'admin.organizationUsage.people.capturing' : 'admin.organizationUsage.people.screenshot') }}
      </button>
    </div>

    <div class="overflow-x-auto">
      <table ref="tableRef" class="org-usage-table org-usage-table--compact min-w-[1360px]">
        <thead>
          <tr>
            <th :class="[rankCellClass, 'bg-gray-50 text-center dark:bg-dark-800']">{{ t('admin.organizationUsage.columns.rank') }}</th>
            <th
              v-for="column in columns"
              :key="column.key"
              :class="[column.numeric ? 'org-usage-num' : '', column.key === 'email' ? [emailCellClass, 'bg-gray-50 dark:bg-dark-800'] : '']"
              :aria-sort="ariaSort(column)"
            >
              <button
                v-if="column.sortable"
                type="button"
                :data-sort-key="column.sortKey"
                class="inline-flex items-center gap-1 hover:text-gray-700 dark:hover:text-gray-200"
                @click="emitSort(column.sortKey!)"
              >
                {{ column.label }}
                <span class="inline-flex h-5 w-4 flex-col items-center justify-center" aria-hidden="true">
                  <svg class="h-2.5 w-2.5" :class="sortIndicatorClass(column.sortKey!, 'asc')" fill="currentColor" viewBox="0 0 10 10">
                    <path d="M5 2L1.5 6.5h7L5 2z" />
                  </svg>
                  <svg class="-mt-0.5 h-2.5 w-2.5" :class="sortIndicatorClass(column.sortKey!, 'desc')" fill="currentColor" viewBox="0 0 10 10">
                    <path d="M5 8L1.5 3.5h7L5 8z" />
                  </svg>
                </span>
              </button>
              <span v-else>{{ column.label }}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          <template v-if="loading">
            <tr v-for="index in 5" :key="index" data-testid="people-loading">
              <td :class="rankCellClass"><div class="mx-auto h-4 w-6 animate-pulse rounded bg-gray-200 dark:bg-dark-700" /></td>
              <td v-for="column in columns" :key="column.key">
                <div class="h-4 w-20 animate-pulse rounded bg-gray-200 dark:bg-dark-700" :class="column.numeric ? 'ml-auto' : ''" />
              </td>
            </tr>
          </template>
          <tr v-else-if="items.length === 0" data-testid="people-empty">
            <td :colspan="columns.length + 1" class="py-12 text-center text-sm text-gray-500 dark:text-dark-400">
              <Icon name="inbox" size="lg" class="mx-auto mb-2 text-gray-400 dark:text-dark-500" />
              {{ t('admin.organizationUsage.common.noData') }}
            </td>
          </tr>
          <template v-else>
            <tr v-for="(item, index) in items" :key="item.user_id" class="group">
              <td :class="[rankCellClass, stickyBodyClass, 'text-center tabular-nums text-gray-500 dark:text-dark-400']">
                {{ (pagination.page - 1) * pagination.page_size + index + 1 }}
              </td>
              <td :class="[emailCellClass, stickyBodyClass, 'max-w-[240px] truncate font-medium']" :title="item.email">{{ item.email }}</td>
              <td>{{ organizationLabel(item.organization) }}</td>
              <td>{{ item.department_name || t('admin.departments.unassigned') }}</td>
              <td class="org-usage-num">{{ formatNumber(item.requests) }}</td>
              <td class="org-usage-num">{{ formatNumber(item.input_tokens) }}</td>
              <td class="org-usage-num">{{ formatNumber(item.output_tokens) }}</td>
              <td class="org-usage-num">{{ formatNumber(item.cache_read_tokens) }}</td>
              <td class="org-usage-num font-semibold">{{ formatNumber(item.total_tokens) }}</td>
              <td class="org-usage-num">${{ formatCostFixed(item.actual_cost) }}</td>
              <td class="org-usage-num"><PeakCell :period="item.peak_day" /></td>
              <td class="org-usage-num"><PeakCell :period="item.peak_week" /></td>
              <td class="org-usage-num"><PeakCell :period="item.peak_month" /></td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>

    <Pagination
      v-if="pagination.total > 0"
      :page="pagination.page"
      :page-size="pagination.page_size"
      :total="pagination.total"
      :page-size-options="[20, 50, 100]"
      @update:page="emit('page', $event)"
      @update:page-size="emit('pageSize', $event)"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'

import type {
  OrganizationUsagePagination,
  OrganizationUsagePeriod,
  OrganizationUsageRange,
  OrganizationUsageSortBy,
  OrganizationUsageSortOrder,
  OrganizationUsageSummaryItem
} from '@/api/admin/organizationUsage'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import { formatCostFixed, formatNumber } from '@/utils/format'
import { formatOrganizationUsageOrganization } from '@/utils/organizationUsageReport'
import { captureTablePng } from '@/utils/tableScreenshot'
import { useAppStore } from '@/stores/app'
import './organizationUsageTable.css'

const props = defineProps<{
  items: OrganizationUsageSummaryItem[]
  pagination: OrganizationUsagePagination
  loading: boolean
  sortBy: OrganizationUsageSortBy
  sortOrder: OrganizationUsageSortOrder
  range?: OrganizationUsageRange
}>()

const emit = defineEmits<{
  sort: [sortBy: OrganizationUsageSortBy, sortOrder: OrganizationUsageSortOrder]
  page: [page: number]
  pageSize: [pageSize: number]
}>()

const { t } = useI18n()
const appStore = useAppStore()
const tableRef = ref<HTMLTableElement | null>(null)
const capturing = ref(false)
let captureController: AbortController | null = null

// A new page/query or revoked scope must not download a stale table after rendering finishes.
watch(
  () => [props.items, props.loading, props.pagination.page, props.pagination.page_size, props.sortBy, props.sortOrder, props.range],
  () => captureController?.abort(),
  { flush: 'sync' }
)
onUnmounted(() => captureController?.abort())

async function downloadScreenshot() {
  if (capturing.value || props.loading || !props.items.length || !tableRef.value) return
  const controller = new AbortController()
  captureController = controller
  capturing.value = true
  const range = props.range ? `${props.range.start_date}_to_${props.range.end_date}_` : ''
  const filename = `organization_usage_people_${range}page_${props.pagination.page}.png`
  try {
    const blob = await captureTablePng(tableRef.value, controller.signal)
    if (controller.signal.aborted) return
    saveAs(blob, filename)
    appStore.showSuccess(t('admin.organizationUsage.people.screenshotSuccess'))
  } catch {
    if (!controller.signal.aborted) appStore.showError(t('admin.organizationUsage.people.screenshotFailed'))
  } finally {
    captureController = null
    capturing.value = false
  }
}

const PeakCell = defineComponent({
  props: { period: { type: Object as () => OrganizationUsagePeriod | null, default: null } },
  setup(componentProps) {
    return () => {
      const period = componentProps.period
      if (!period) return h('span', { class: 'text-gray-400 dark:text-dark-500' }, t('admin.organizationUsage.common.noData'))
      return h('div', { class: 'min-w-[130px]' }, [
        h('div', { class: 'font-medium tabular-nums text-gray-900 dark:text-white' }, formatNumber(period.total_tokens)),
        h('div', { class: 'mt-0.5 flex items-center justify-end gap-1 whitespace-nowrap text-xs text-gray-500 dark:text-dark-400' }, [
          period.partial ? h('span', { class: 'badge badge-warning' }, t('admin.organizationUsage.common.partial')) : null,
          period.period_start === period.period_end ? period.period_start : `${period.period_start} - ${period.period_end}`
        ])
      ])
    }
  }
})

const columns = computed(() => [
  { key: 'email', label: t('admin.organizationUsage.columns.email'), sortable: true, sortKey: 'email' as const },
  { key: 'organization', label: t('admin.organizationUsage.columns.organization') },
  { key: 'department', label: t('admin.departments.department') },
  { key: 'requests', label: t('admin.organizationUsage.metrics.requests'), sortable: true, sortKey: 'requests' as const, numeric: true },
  { key: 'input_tokens', label: t('admin.organizationUsage.metrics.inputTokens'), sortable: true, sortKey: 'input_tokens' as const, numeric: true },
  { key: 'output_tokens', label: t('admin.organizationUsage.metrics.outputTokens'), sortable: true, sortKey: 'output_tokens' as const, numeric: true },
  { key: 'cache_read_tokens', label: t('admin.organizationUsage.metrics.cacheTokens'), sortable: true, sortKey: 'cache_read_tokens' as const, numeric: true },
  { key: 'total_tokens', label: t('admin.organizationUsage.metrics.totalTokens'), sortable: true, sortKey: 'total_tokens' as const, numeric: true },
  { key: 'actual_cost', label: t('admin.organizationUsage.metrics.actualCost'), sortable: true, sortKey: 'actual_cost' as const, numeric: true },
  { key: 'peak_day', label: t('admin.organizationUsage.columns.peakDay'), sortable: true, sortKey: 'peak_day_tokens' as const, numeric: true },
  { key: 'peak_week', label: t('admin.organizationUsage.columns.peakWeek'), sortable: true, sortKey: 'peak_week_tokens' as const, numeric: true },
  { key: 'peak_month', label: t('admin.organizationUsage.columns.peakMonth'), sortable: true, sortKey: 'peak_month_tokens' as const, numeric: true }
])

// Rank and email stay visible while the wide metric columns scroll horizontally.
const rankCellClass = 'sticky left-0 z-10 w-14 min-w-[3.5rem] max-w-[3.5rem]'
const emailCellClass = 'sticky left-14 z-10 border-r border-gray-100 dark:border-dark-700'
const stickyBodyClass = 'bg-white transition-colors group-hover:bg-gray-50 dark:bg-dark-900 dark:group-hover:bg-dark-800'

function sortIndicatorClass(sortKey: OrganizationUsageSortBy, order: OrganizationUsageSortOrder) {
  return props.sortBy === sortKey && props.sortOrder === order
    ? 'text-primary-500 dark:text-primary-400'
    : 'text-gray-400 dark:text-dark-500'
}

function ariaSort(column: (typeof columns.value)[number]) {
  if (!column.sortable) return undefined
  if (column.sortKey !== props.sortBy) return 'none'
  return props.sortOrder === 'asc' ? 'ascending' : 'descending'
}

function emitSort(sortBy: OrganizationUsageSortBy) {
  emit('sort', sortBy, props.sortBy === sortBy && props.sortOrder === 'asc' ? 'desc' : 'asc')
}

function organizationLabel(value: string) {
  return formatOrganizationUsageOrganization(
    value,
    t('admin.organizationUsage.organizations.other')
  )
}
</script>
