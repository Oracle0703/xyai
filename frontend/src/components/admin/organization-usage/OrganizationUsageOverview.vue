<template>
  <section class="space-y-6" :aria-label="t('admin.organizationUsage.overview.title')">
    <dl class="grid grid-cols-2 gap-4 md:grid-cols-3 2xl:grid-cols-6">
      <div v-for="metric in overviewMetrics" :key="metric.label" class="card flex min-w-0 items-center gap-3 p-4">
        <div class="shrink-0 rounded-lg p-2" :class="metric.tone">
          <Icon :name="metric.icon" size="md" />
        </div>
        <div class="min-w-0">
          <dt class="flex items-center text-xs font-medium text-gray-500 dark:text-dark-400">
            <span class="truncate">{{ metric.label }}</span>
            <HelpTooltip v-if="metric.help" :content="metric.help" />
          </dt>
          <dd class="mt-0.5 truncate text-xl font-bold tabular-nums text-gray-900 dark:text-white" :title="metric.value">{{ metric.value }}</dd>
        </div>
      </div>
    </dl>

    <div class="card overflow-hidden">
      <div class="org-usage-card-header">
        <h3 class="org-usage-card-title">{{ t('admin.organizationUsage.champions.title') }}</h3>
        <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500 dark:text-dark-400">
          <span>{{ range.start_date }} - {{ range.end_date }}</span>
          <span v-if="asOfLabel">{{ asOfLabel }}</span>
          <span v-if="attribution">{{ attribution }}</span>
        </div>
      </div>
      <div class="grid divide-y divide-gray-100 dark:divide-dark-700 lg:grid-cols-3 lg:divide-x lg:divide-y-0">
        <div v-for="champion in champions" :key="champion.label" class="flex min-w-0 items-start gap-3 p-4">
          <div class="shrink-0 rounded-lg bg-amber-100 p-2 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400">
            <Icon name="trophy" size="md" />
          </div>
          <div class="min-w-0 flex-1">
            <p class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ champion.label }}</p>
            <template v-if="champion.value">
              <p class="mt-0.5 truncate text-sm font-semibold text-gray-900 dark:text-white" :title="champion.value.email">
                {{ champion.value.email }}
              </p>
              <p class="mt-1 text-lg font-bold tabular-nums text-gray-900 dark:text-white">
                {{ formatNumber(champion.value.total_tokens) }}
                <span class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('admin.organizationUsage.common.tokens') }}</span>
              </p>
              <p class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-500 dark:text-dark-400">
                <span>{{ formatOrganizationUsageOrganization(champion.value.organization, t('admin.organizationUsage.organizations.other')) }}</span>
                <span>{{ champion.value.period_start }} - {{ champion.value.period_end }}</span>
                <span v-if="champion.value.partial" class="badge badge-warning">
                  {{ t('admin.organizationUsage.common.partial') }}
                </span>
              </p>
            </template>
            <p v-else class="mt-1 text-sm text-gray-400 dark:text-dark-500">{{ t('admin.organizationUsage.common.noData') }}</p>
          </div>
        </div>
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
import Icon from '@/components/icons/Icon.vue'
import { formatCostFixed, formatDateTime, formatNumber } from '@/utils/format'
import { formatOrganizationUsageOrganization } from '@/utils/organizationUsageReport'
import './organizationUsageTable.css'

const props = defineProps<{
  overview: OrganizationUsageOverview
  champions: OrganizationUsageChampions
  range: OrganizationUsageRange
  attribution?: string
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
    help: t('admin.organizationUsage.metrics.registeredUsersHelp'),
    icon: 'users' as const,
    tone: 'bg-blue-100 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400'
  },
  {
    label: t('admin.organizationUsage.metrics.usedUsers'),
    value: formatNumber(props.overview.used_users),
    help: t('admin.organizationUsage.metrics.activeUsersHelp'),
    icon: 'user' as const,
    tone: 'bg-emerald-100 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400'
  },
  {
    label: t('admin.organizationUsage.metrics.activeRate'),
    value: `${(props.overview.active_users > 0 ? props.overview.used_users / props.overview.active_users * 100 : 0).toFixed(1)}%`,
    icon: 'chart' as const,
    tone: 'bg-teal-100 text-teal-600 dark:bg-teal-900/30 dark:text-teal-400'
  },
  {
    label: t('admin.organizationUsage.metrics.requests'),
    value: formatNumber(props.overview.requests),
    icon: 'document' as const,
    tone: 'bg-violet-100 text-violet-600 dark:bg-violet-900/30 dark:text-violet-400'
  },
  {
    label: t('admin.organizationUsage.metrics.totalTokens'),
    value: formatNumber(props.overview.total_tokens),
    help: t('admin.organizationUsage.metrics.totalTokensHelp'),
    icon: 'cube' as const,
    tone: 'bg-amber-100 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400'
  },
  {
    label: t('admin.organizationUsage.metrics.actualCost'),
    value: `$${formatCostFixed(props.overview.actual_cost)}`,
    icon: 'dollar' as const,
    tone: 'bg-rose-100 text-rose-600 dark:bg-rose-900/30 dark:text-rose-400'
  }
])

const champions = computed(() => [
  { label: t('admin.organizationUsage.champions.day'), value: props.champions.day },
  { label: t('admin.organizationUsage.champions.week'), value: props.champions.week },
  { label: t('admin.organizationUsage.champions.month'), value: props.champions.month }
])
</script>
