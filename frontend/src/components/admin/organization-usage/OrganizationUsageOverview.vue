<template>
  <section class="border-b border-gray-200 py-5 dark:border-dark-700">
    <div class="flex flex-wrap items-baseline justify-between gap-2">
      <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.organizationUsage.overview.title') }}</h2>
      <div class="text-right text-xs text-gray-500 dark:text-dark-400">
        <p>{{ range.start_date }} - {{ range.end_date }}</p>
        <p v-if="asOfLabel" class="mt-1">{{ asOfLabel }}</p>
      </div>
    </div>

    <dl class="mt-4 grid grid-cols-2 gap-y-4 border-y border-gray-200 py-3 dark:border-dark-700 sm:grid-cols-3 lg:grid-cols-6">
      <div v-for="metric in overviewMetrics" :key="metric.label" class="min-w-0 px-3 first:pl-0">
        <dt class="flex items-center text-xs text-gray-500 dark:text-dark-400">
          <span class="truncate">{{ metric.label }}</span>
          <HelpTooltip v-if="metric.help" :content="metric.help" />
        </dt>
        <dd class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">{{ metric.value }}</dd>
      </div>
    </dl>

    <div class="mt-5 grid gap-4 lg:grid-cols-3">
      <div v-for="champion in champions" :key="champion.label" class="min-w-0 border-l-2 border-primary-400 pl-3">
        <p class="text-xs font-medium uppercase text-gray-500 dark:text-dark-400">{{ champion.label }}</p>
        <template v-if="champion.value">
          <p class="mt-1 truncate text-sm font-medium text-gray-900 dark:text-white" :title="champion.value.email">
            {{ champion.value.email }}
          </p>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
            {{ formatOrganizationUsageOrganization(champion.value.organization, t('admin.organizationUsage.organizations.other')) }}
          </p>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
            {{ champion.value.period_start }} - {{ champion.value.period_end }}
            <span v-if="champion.value.partial" class="ml-1 rounded bg-amber-100 px-1.5 py-0.5 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">
              {{ t('admin.organizationUsage.common.partial') }}
            </span>
          </p>
          <p class="mt-1 text-sm tabular-nums text-gray-700 dark:text-dark-200">
            {{ formatNumber(champion.value.total_tokens) }} {{ t('admin.organizationUsage.common.tokens') }}
          </p>
        </template>
        <p v-else class="mt-2 text-sm text-gray-400 dark:text-dark-500">{{ t('admin.organizationUsage.common.noData') }}</p>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type {
  OrganizationUsageChampions,
  OrganizationUsageOverview,
  OrganizationUsageRange
} from '@/api/admin/organizationUsage'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import { formatCostFixed, formatDateTime, formatNumber } from '@/utils/format'
import { formatOrganizationUsageOrganization } from '@/utils/organizationUsageReport'

const props = defineProps<{
  overview: OrganizationUsageOverview
  champions: OrganizationUsageChampions
  range: OrganizationUsageRange
}>()

const { t } = useI18n()

const asOfLabel = computed(() => {
  if (!props.range.as_of) return ''
  return t('admin.organizationUsage.trend.asOf', { value: formatDateTime(props.range.as_of) || '-' })
})

const overviewMetrics = computed(() => [
  {
    label: t('admin.organizationUsage.metrics.activeUsers'),
    value: formatNumber(props.overview.active_users),
    help: t('admin.organizationUsage.metrics.registeredUsersHelp')
  },
  {
    label: t('admin.organizationUsage.metrics.usedUsers'),
    value: formatNumber(props.overview.used_users),
    help: t('admin.organizationUsage.metrics.activeUsersHelp')
  },
  {
    label: t('admin.organizationUsage.metrics.activeRate'),
    value: `${(props.overview.active_users > 0 ? props.overview.used_users / props.overview.active_users * 100 : 0).toFixed(1)}%`
  },
  { label: t('admin.organizationUsage.metrics.requests'), value: formatNumber(props.overview.requests) },
  {
    label: t('admin.organizationUsage.metrics.totalTokens'),
    value: formatNumber(props.overview.total_tokens),
    help: t('admin.organizationUsage.metrics.totalTokensHelp')
  },
  { label: t('admin.organizationUsage.metrics.actualCost'), value: `$${formatCostFixed(props.overview.actual_cost)}` }
])

const champions = computed(() => [
  { label: t('admin.organizationUsage.champions.day'), value: props.champions.day },
  { label: t('admin.organizationUsage.champions.week'), value: props.champions.week },
  { label: t('admin.organizationUsage.champions.month'), value: props.champions.month }
])
</script>
