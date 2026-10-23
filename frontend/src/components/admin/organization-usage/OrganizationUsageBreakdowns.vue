<template>
  <div class="grid items-start gap-6" :class="departments.length ? 'xl:grid-cols-2' : ''">
    <section v-if="departments.length" class="card min-w-0 overflow-hidden">
      <div class="org-usage-card-header">
        <h3 class="org-usage-card-title">{{ t('admin.departments.departmentSummary') }}</h3>
      </div>
      <div class="overflow-x-auto">
        <table class="org-usage-table">
          <thead>
            <tr>
              <th>{{ t('admin.departments.department') }}</th>
              <th class="org-usage-num">{{ t('admin.departments.memberCount') }}</th>
              <th class="org-usage-num">{{ t('admin.organizationUsage.metrics.usedUsers') }}</th>
              <th class="org-usage-num">{{ t('admin.organizationUsage.metrics.totalTokens') }}</th>
              <th class="org-usage-num">{{ t('admin.organizationUsage.metrics.actualCost') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="department in departments" :key="`${department.organization}:${department.department_id}`">
              <td>
                <button
                  type="button"
                  class="org-usage-link"
                  @click="emit('department', department.organization, department.department_id === null ? 'unassigned' : String(department.department_id))"
                >
                  {{ department.department_name || t('admin.departments.unassigned') }}
                </button>
              </td>
              <td class="org-usage-num">{{ formatNumber(department.active_users) }}</td>
              <td class="org-usage-num">{{ formatNumber(department.used_users) }}</td>
              <td class="org-usage-num">{{ formatNumber(department.total_tokens) }}</td>
              <td class="org-usage-num">${{ formatCostFixed(department.actual_cost) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section class="card min-w-0 overflow-hidden">
      <div class="org-usage-card-header">
        <h3 class="org-usage-card-title">{{ t('admin.departments.platformSummary') }}</h3>
      </div>
      <div class="overflow-x-auto">
        <table class="org-usage-table">
          <thead>
            <tr>
              <th>{{ t('admin.departments.platform') }}</th>
              <th class="org-usage-num">{{ t('admin.organizationUsage.metrics.usedUsers') }}</th>
              <th class="org-usage-num">{{ t('admin.organizationUsage.metrics.requests') }}</th>
              <th class="org-usage-num">{{ t('admin.organizationUsage.metrics.totalTokens') }}</th>
              <th class="org-usage-num">{{ t('admin.organizationUsage.metrics.actualCost') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="platform in platforms" :key="platform.platform">
              <td>
                <button type="button" class="org-usage-link" @click="emit('platform', platform.platform)">
                  {{ platformLabel(platform.platform) }}
                </button>
              </td>
              <td class="org-usage-num">{{ formatNumber(platform.used_users) }}</td>
              <td class="org-usage-num">{{ formatNumber(platform.requests) }}</td>
              <td class="org-usage-num">{{ formatNumber(platform.total_tokens) }}</td>
              <td class="org-usage-num">${{ formatCostFixed(platform.actual_cost) }}</td>
            </tr>
            <tr v-if="!platforms.length">
              <td colspan="5" class="py-10 text-center text-sm text-gray-500 dark:text-dark-400">
                {{ t('admin.organizationUsage.common.noData') }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { CONCRETE_PLATFORM_OPTIONS } from '@/constants/platforms'
import { formatCostFixed, formatNumber } from '@/utils/format'
import type { OrganizationUsageDepartment, OrganizationUsagePlatform } from '@/api/admin/organizationUsage'
import './organizationUsageTable.css'

defineProps<{ departments: OrganizationUsageDepartment[]; platforms: OrganizationUsagePlatform[] }>()
const emit = defineEmits<{ department: [organization: string, departmentID: string]; platform: [platform: string] }>()
const { t } = useI18n()
const platformLabel = (value: string) => value === 'unknown'
  ? t('admin.departments.unknownPlatform')
  : CONCRETE_PLATFORM_OPTIONS.find(option => option.value === value)?.label ?? value
</script>
