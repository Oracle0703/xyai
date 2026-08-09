<template>
  <section class="border-b border-gray-200 py-5 dark:border-dark-700">
    <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.organizationUsage.organizationSummary.title') }}</h2>
    <div class="mt-3 overflow-x-auto">
      <table class="w-full min-w-[760px] border-collapse text-sm">
        <thead class="border-y border-gray-200 bg-gray-50 text-left text-xs text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-400">
          <tr>
            <th class="px-3 py-2.5 font-medium">{{ t('admin.organizationUsage.columns.organization') }}</th>
            <th class="px-3 py-2.5 text-right font-medium">{{ t('admin.organizationUsage.metrics.activeUsers') }}</th>
            <th class="px-3 py-2.5 text-right font-medium">{{ t('admin.organizationUsage.metrics.usedUsers') }}</th>
            <th class="px-3 py-2.5 text-right font-medium">{{ t('admin.organizationUsage.metrics.requests') }}</th>
            <th class="px-3 py-2.5 text-right font-medium">{{ t('admin.organizationUsage.metrics.totalTokens') }}</th>
            <th class="px-3 py-2.5 text-right font-medium">{{ t('admin.organizationUsage.metrics.actualCost') }}</th>
            <th class="px-3 py-2.5 text-right font-medium">{{ t('admin.organizationUsage.metrics.tokenShare') }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
          <tr
            v-for="row in rows"
            :key="row.filter"
            class="transition-colors hover:bg-gray-50 dark:hover:bg-dark-800"
            :class="selectedOrganization === row.filter ? 'bg-primary-50/70 dark:bg-primary-900/10' : ''"
          >
            <td class="px-3 py-3 font-medium text-gray-900 dark:text-white">
              <button
                type="button"
                :data-organization="row.filter"
                :aria-pressed="selectedOrganization === row.filter"
                class="rounded px-1 py-0.5 text-left font-medium text-primary-700 hover:underline focus:outline-none focus:ring-2 focus:ring-primary-500 dark:text-primary-300"
                @click="emit('select', row.filter)"
              >
                {{ row.label }}
              </button>
            </td>
            <td class="px-3 py-3 text-right tabular-nums">{{ formatNumber(row.active_users) }}</td>
            <td class="px-3 py-3 text-right tabular-nums">{{ formatNumber(row.used_users) }}</td>
            <td class="px-3 py-3 text-right tabular-nums">{{ formatNumber(row.requests) }}</td>
            <td class="px-3 py-3 text-right tabular-nums">{{ formatNumber(row.total_tokens) }}</td>
            <td class="px-3 py-3 text-right tabular-nums">${{ formatCostFixed(row.actual_cost) }}</td>
            <td class="px-3 py-3 text-right tabular-nums">{{ row.share.toFixed(1) }}%</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type {
  OrganizationUsageOrganization,
  OrganizationUsageOrganizationFilter
} from '@/api/admin/organizationUsage'
import { formatCostFixed, formatNumber } from '@/utils/format'
import { formatOrganizationUsageOrganization } from '@/utils/organizationUsageReport'

const props = defineProps<{
  organizations: OrganizationUsageOrganization[]
  selectedOrganization: OrganizationUsageOrganizationFilter
}>()

const emit = defineEmits<{ select: [organization: Exclude<OrganizationUsageOrganizationFilter, 'all'>] }>()
const { t } = useI18n()

const zero = (organization: string): OrganizationUsageOrganization => ({
  organization,
  active_users: 0,
  used_users: 0,
  requests: 0,
  input_tokens: 0,
  output_tokens: 0,
  cache_creation_tokens: 0,
  cache_read_tokens: 0,
  total_tokens: 0,
  actual_cost: 0
})

function findOrganization(key: 'xunyou' | 'wsdashi' | 'other') {
  return props.organizations.find((item) => item.organization === key || item.organization === `${key}.com`) ?? zero(key)
}

const rows = computed(() => {
  const values = (['xunyou', 'wsdashi', 'other'] as const).map((filter) => ({
    ...findOrganization(filter),
    filter,
    label: formatOrganizationUsageOrganization(
      filter,
      t('admin.organizationUsage.organizations.other')
    )
  }))
  const totalTokens = values.reduce((sum, row) => sum + row.total_tokens, 0)
  return values.map((row) => ({ ...row, share: totalTokens ? row.total_tokens / totalTokens * 100 : 0 }))
})
</script>
