<template>
  <div class="grid gap-5 border-b border-gray-200 py-5 dark:border-dark-700 xl:grid-cols-2">
    <section v-if="departments.length" class="min-w-0">
      <h2 class="mb-3 font-semibold text-gray-900 dark:text-white">{{ t('admin.departments.departmentSummary') }}</h2>
      <div class="overflow-x-auto"><table class="w-full text-sm">
        <thead><tr class="border-b text-left text-xs text-gray-500"><th class="py-2">{{ t('admin.departments.department') }}</th><th class="px-2 text-right">{{ t('admin.departments.memberCount') }}</th><th class="px-2 text-right">{{ t('admin.organizationUsage.metrics.usedUsers') }}</th><th class="px-2 text-right">{{ t('admin.organizationUsage.metrics.totalTokens') }}</th><th class="px-2 text-right">{{ t('admin.organizationUsage.metrics.actualCost') }}</th></tr></thead>
        <tbody><tr v-for="department in departments" :key="`${department.organization}:${department.department_id}`" class="border-b border-gray-100 dark:border-dark-800"><td class="py-3"><button class="text-primary-600 hover:underline" @click="emit('department', department.organization, department.department_id === null ? 'unassigned' : String(department.department_id))">{{ department.department_name || t('admin.departments.unassigned') }}</button></td><td class="px-2 text-right tabular-nums">{{ formatNumber(department.active_users) }}</td><td class="px-2 text-right tabular-nums">{{ formatNumber(department.used_users) }}</td><td class="px-2 text-right tabular-nums">{{ formatNumber(department.total_tokens) }}</td><td class="px-2 text-right tabular-nums">${{ formatCostFixed(department.actual_cost) }}</td></tr></tbody>
      </table></div>
    </section>
    <section class="min-w-0">
      <h2 class="mb-3 font-semibold text-gray-900 dark:text-white">{{ t('admin.departments.platformSummary') }}</h2>
      <div class="overflow-x-auto"><table class="w-full text-sm">
        <thead><tr class="border-b text-left text-xs text-gray-500"><th class="py-2">{{ t('admin.departments.platform') }}</th><th class="px-2 text-right">{{ t('admin.organizationUsage.metrics.usedUsers') }}</th><th class="px-2 text-right">{{ t('admin.organizationUsage.metrics.requests') }}</th><th class="px-2 text-right">{{ t('admin.organizationUsage.metrics.totalTokens') }}</th><th class="px-2 text-right">{{ t('admin.organizationUsage.metrics.actualCost') }}</th></tr></thead>
        <tbody><tr v-for="platform in platforms" :key="platform.platform" class="border-b border-gray-100 dark:border-dark-800"><td class="py-3"><button class="text-primary-600 hover:underline" @click="emit('platform', platform.platform)">{{ platformLabel(platform.platform) }}</button></td><td class="px-2 text-right tabular-nums">{{ formatNumber(platform.used_users) }}</td><td class="px-2 text-right tabular-nums">{{ formatNumber(platform.requests) }}</td><td class="px-2 text-right tabular-nums">{{ formatNumber(platform.total_tokens) }}</td><td class="px-2 text-right tabular-nums">${{ formatCostFixed(platform.actual_cost) }}</td></tr><tr v-if="!platforms.length"><td colspan="5" class="py-6 text-center text-gray-500">{{ t('admin.organizationUsage.common.noData') }}</td></tr></tbody>
      </table></div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { CONCRETE_PLATFORM_OPTIONS } from '@/constants/platforms'
import { formatCostFixed, formatNumber } from '@/utils/format'
import type { OrganizationUsageDepartment, OrganizationUsagePlatform } from '@/api/admin/organizationUsage'
defineProps<{ departments: OrganizationUsageDepartment[]; platforms: OrganizationUsagePlatform[] }>()
const emit = defineEmits<{ department: [organization: string, departmentID: string]; platform: [platform: string] }>()
const { t } = useI18n()
const platformLabel = (value: string) => value === 'unknown' ? t('admin.departments.unknownPlatform') : CONCRETE_PLATFORM_OPTIONS.find(option => option.value === value)?.label ?? value
</script>
