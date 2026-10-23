<template>
  <section class="card overflow-hidden">
    <div class="org-usage-card-header">
      <h3 class="org-usage-card-title">{{ t('admin.organizationUsage.organizationSummary.title') }}</h3>
    </div>
    <div class="overflow-x-auto">
      <table class="org-usage-table min-w-[760px]">
        <thead>
          <tr>
            <th>{{ t('admin.organizationUsage.columns.organization') }}</th>
            <th class="org-usage-num">{{ t('admin.organizationUsage.metrics.activeUsers') }}</th>
            <th class="org-usage-num">{{ t('admin.organizationUsage.metrics.usedUsers') }}</th>
            <th class="org-usage-num">{{ t('admin.organizationUsage.metrics.requests') }}</th>
            <th class="org-usage-num">{{ t('admin.organizationUsage.metrics.totalTokens') }}</th>
            <th class="org-usage-num">{{ t('admin.organizationUsage.metrics.actualCost') }}</th>
            <th class="org-usage-num">{{ t('admin.organizationUsage.metrics.tokenShare') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="row in rows"
            :key="row.filter"
            :class="selectedOrganization === row.filter ? 'bg-primary-50/70 dark:bg-primary-900/10' : ''"
          >
            <td>
              <button
                type="button"
                :data-organization="row.filter"
                :aria-pressed="selectedOrganization === row.filter"
                class="org-usage-link"
                @click="emit('select', row.filter)"
              >
                {{ row.label }}
              </button>
            </td>
            <td class="org-usage-num">{{ formatNumber(row.active_users) }}</td>
            <td class="org-usage-num">{{ formatNumber(row.used_users) }}</td>
            <td class="org-usage-num">{{ formatNumber(row.requests) }}</td>
            <td class="org-usage-num">{{ formatNumber(row.total_tokens) }}</td>
            <td class="org-usage-num">${{ formatCostFixed(row.actual_cost) }}</td>
            <td class="org-usage-num">
              <div class="flex items-center justify-end gap-2">
                <div class="h-1.5 w-16 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700" aria-hidden="true">
                  <div class="h-full rounded-full bg-primary-500" :style="{ width: `${Math.min(100, row.share)}%` }" />
                </div>
                <span class="w-12">{{ row.share.toFixed(1) }}%</span>
              </div>
            </td>
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
import './organizationUsageTable.css'

const props = defineProps<{
  organizations: OrganizationUsageOrganization[]
  selectedOrganization: OrganizationUsageOrganizationFilter
}>()

const emit = defineEmits<{ select: [organization: Exclude<OrganizationUsageOrganizationFilter, 'all'>] }>()
const { t } = useI18n()

const rows = computed(() => {
  const values = (['xunyou', 'wsdashi', 'other'] as const).flatMap((filter) => {
    const item = props.organizations.find(item => item.organization === filter || item.organization === `${filter}.com`)
    if (!item) return []
    return [{
    ...item,
    filter,
    label: formatOrganizationUsageOrganization(
      filter,
      t('admin.organizationUsage.organizations.other')
    )
  }]
  })
  const totalTokens = values.reduce((sum, row) => sum + row.total_tokens, 0)
  return values.map((row) => ({ ...row, share: totalTokens ? row.total_tokens / totalTokens * 100 : 0 }))
})
</script>
