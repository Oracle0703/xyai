<template>
  <section class="pt-5">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.organizationUsage.people.title') }}</h2>
        <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.organizationUsage.people.total', { count: pagination.total }) }}</p>
      </div>
      <label class="flex items-center gap-2 text-xs text-gray-500 dark:text-dark-400">
        {{ t('admin.organizationUsage.people.pageSize') }}
        <select
          data-testid="people-page-size"
          class="input h-9 w-20 py-1"
          :value="pagination.page_size"
          @change="emit('pageSize', Number(($event.target as HTMLSelectElement).value))"
        >
          <option v-for="size in [20, 50, 100]" :key="size" :value="size">{{ size }}</option>
        </select>
      </label>
    </div>

    <div class="mt-3 overflow-x-auto border-y border-gray-200 dark:border-dark-700">
      <table class="min-w-[1620px] w-full border-collapse text-sm">
        <thead class="bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-800 dark:text-dark-400">
          <tr>
            <th v-for="column in columns" :key="column.key" class="whitespace-nowrap px-3 py-2.5 font-medium" :aria-sort="ariaSort(column)">
              <button
                v-if="column.sortable"
                type="button"
                :data-sort-key="column.sortKey"
                class="inline-flex items-center gap-1 hover:text-gray-900 dark:hover:text-white"
                @click="emitSort(column.sortKey!)"
              >
                {{ column.label }}
                <span class="inline-flex w-3 flex-col text-[8px] leading-[7px]" aria-hidden="true">
                  <span :class="sortBy === column.sortKey && sortOrder === 'asc' ? 'text-primary-500' : 'text-gray-300 dark:text-dark-600'">▲</span>
                  <span :class="sortBy === column.sortKey && sortOrder === 'desc' ? 'text-primary-500' : 'text-gray-300 dark:text-dark-600'">▼</span>
                </span>
              </button>
              <span v-else>{{ column.label }}</span>
            </th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
          <template v-if="loading">
            <tr v-for="index in 5" :key="index" data-testid="people-loading">
              <td v-for="column in columns" :key="column.key" class="px-3 py-3">
                <div class="h-4 w-20 animate-pulse rounded bg-gray-200 dark:bg-dark-700" />
              </td>
            </tr>
          </template>
          <tr v-else-if="items.length === 0" data-testid="people-empty">
            <td :colspan="columns.length" class="px-3 py-12 text-center text-sm text-gray-500 dark:text-dark-400">
              <Icon name="inbox" size="lg" class="mx-auto mb-2" />
              {{ t('admin.organizationUsage.common.noData') }}
            </td>
          </tr>
          <template v-else>
            <tr v-for="item in items" :key="item.user_id" class="hover:bg-gray-50 dark:hover:bg-dark-800/70">
              <td class="whitespace-nowrap px-3 py-3">{{ organizationLabel(item.organization) }}</td>
              <td class="max-w-[240px] truncate px-3 py-3 font-medium text-gray-900 dark:text-white" :title="item.email">{{ item.email }}</td>
              <td class="px-3 py-3 text-right tabular-nums">{{ formatNumber(item.requests) }}</td>
              <td class="px-3 py-3 text-right tabular-nums">{{ formatNumber(item.input_tokens) }}</td>
              <td class="px-3 py-3 text-right tabular-nums">{{ formatNumber(item.output_tokens) }}</td>
              <td class="px-3 py-3 text-right tabular-nums">{{ formatNumber(item.cache_read_tokens) }}</td>
              <td class="px-3 py-3 text-right font-medium tabular-nums">{{ formatNumber(item.total_tokens) }}</td>
              <td class="px-3 py-3 text-right tabular-nums">${{ formatCostFixed(item.actual_cost) }}</td>
              <td class="px-3 py-3"><PeakCell :period="item.peak_day" /></td>
              <td class="px-3 py-3"><PeakCell :period="item.peak_week" /></td>
              <td class="px-3 py-3"><PeakCell :period="item.peak_month" /></td>
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
      :show-page-size-selector="false"
      @update:page="emit('page', $event)"
      @update:page-size="emit('pageSize', $event)"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, defineComponent, h } from 'vue'
import { useI18n } from 'vue-i18n'

import type {
  OrganizationUsagePagination,
  OrganizationUsagePeriod,
  OrganizationUsageSortBy,
  OrganizationUsageSortOrder,
  OrganizationUsageSummaryItem
} from '@/api/admin/organizationUsage'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import { formatCostFixed, formatNumber } from '@/utils/format'

const props = defineProps<{
  items: OrganizationUsageSummaryItem[]
  pagination: OrganizationUsagePagination
  loading: boolean
  sortBy: OrganizationUsageSortBy
  sortOrder: OrganizationUsageSortOrder
}>()

const emit = defineEmits<{
  sort: [sortBy: OrganizationUsageSortBy, sortOrder: OrganizationUsageSortOrder]
  page: [page: number]
  pageSize: [pageSize: number]
}>()

const { t } = useI18n()

const PeakCell = defineComponent({
  props: { period: { type: Object as () => OrganizationUsagePeriod | null, default: null } },
  setup(componentProps) {
    return () => {
      const period = componentProps.period
      if (!period) return h('span', { class: 'text-gray-400 dark:text-dark-500' }, t('admin.organizationUsage.common.noData'))
      return h('div', { class: 'min-w-[180px]' }, [
        h('div', { class: 'whitespace-nowrap text-xs text-gray-500 dark:text-dark-400' }, [
          `${period.period_start} - ${period.period_end}`,
          period.partial ? h('span', { class: 'ml-1 rounded bg-amber-100 px-1 py-0.5 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300' }, t('admin.organizationUsage.common.partial')) : null
        ]),
        h('div', { class: 'mt-1 tabular-nums text-gray-900 dark:text-white' }, `${formatNumber(period.total_tokens)} ${t('admin.organizationUsage.common.tokens')}`)
      ])
    }
  }
})

const columns = computed(() => [
  { key: 'organization', label: t('admin.organizationUsage.columns.organization') },
  { key: 'email', label: t('admin.organizationUsage.columns.email'), sortable: true, sortKey: 'email' as const },
  { key: 'requests', label: t('admin.organizationUsage.metrics.requests'), sortable: true, sortKey: 'requests' as const },
  { key: 'input_tokens', label: t('admin.organizationUsage.metrics.inputTokens'), sortable: true, sortKey: 'input_tokens' as const },
  { key: 'output_tokens', label: t('admin.organizationUsage.metrics.outputTokens'), sortable: true, sortKey: 'output_tokens' as const },
  { key: 'cache_read_tokens', label: t('admin.organizationUsage.metrics.cacheTokens'), sortable: true, sortKey: 'cache_read_tokens' as const },
  { key: 'total_tokens', label: t('admin.organizationUsage.metrics.totalTokens'), sortable: true, sortKey: 'total_tokens' as const },
  { key: 'actual_cost', label: t('admin.organizationUsage.metrics.actualCost'), sortable: true, sortKey: 'actual_cost' as const },
  { key: 'peak_day', label: t('admin.organizationUsage.columns.peakDay'), sortable: true, sortKey: 'peak_day_tokens' as const },
  { key: 'peak_week', label: t('admin.organizationUsage.columns.peakWeek'), sortable: true, sortKey: 'peak_week_tokens' as const },
  { key: 'peak_month', label: t('admin.organizationUsage.columns.peakMonth'), sortable: true, sortKey: 'peak_month_tokens' as const }
])

function ariaSort(column: (typeof columns.value)[number]) {
  if (!column.sortable) return undefined
  if (column.sortKey !== props.sortBy) return 'none'
  return props.sortOrder === 'asc' ? 'ascending' : 'descending'
}

function emitSort(sortBy: OrganizationUsageSortBy) {
  emit('sort', sortBy, props.sortBy === sortBy && props.sortOrder === 'asc' ? 'desc' : 'asc')
}

function organizationLabel(value: string) {
  if (value === 'xunyou' || value === 'xunyou.com') return 'xunyou.com'
  if (value === 'wsdashi' || value === 'wsdashi.com') return 'wsdashi.com'
  return t('admin.organizationUsage.organizations.other')
}
</script>
