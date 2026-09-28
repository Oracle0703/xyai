<template>
  <section class="border-b border-gray-200 pb-5 dark:border-dark-700" :aria-label="t('admin.organizationUsage.filters.title')">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-1 dark:border-dark-700 dark:bg-dark-900">
        <button
          v-for="item in modes"
          :key="item.value"
          type="button"
          :data-testid="`mode-${item.value}`"
          class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
          :class="modelValue.mode === item.value
            ? 'bg-white text-primary-600 shadow-sm dark:bg-dark-700 dark:text-primary-400'
            : 'text-gray-500 hover:text-gray-800 dark:text-dark-300 dark:hover:text-white'"
          :aria-pressed="modelValue.mode === item.value"
          @click="patch({ mode: item.value })"
        >
          {{ item.label }}
        </button>
      </div>

      <button
        type="button"
        data-testid="export-report"
        class="btn btn-secondary inline-flex items-center gap-2"
        :disabled="exporting || exportDisabled"
        :title="exportDisabled ? t('admin.organizationUsage.actions.applyFiltersBeforeExport') : undefined"
        @click="emit('export')"
      >
        <Icon name="download" size="sm" />
        {{ exporting ? t('admin.organizationUsage.actions.exporting') : t('admin.organizationUsage.actions.export') }}
      </button>
    </div>

    <div class="mt-4 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
      <label v-if="modelValue.mode === 'month'" class="space-y-1.5">
        <span class="text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.organizationUsage.filters.month') }}</span>
        <input
          data-testid="month-input"
          type="month"
          required
          class="input w-full"
          :aria-invalid="validationMessage ? 'true' : undefined"
          :aria-describedby="validationMessage ? dateErrorId : undefined"
          :value="modelValue.month"
          @input="patch({ month: ($event.target as HTMLInputElement).value })"
        />
      </label>

      <label v-else-if="modelValue.mode === 'week'" class="space-y-1.5">
        <span class="text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.organizationUsage.filters.weekAnchor') }}</span>
        <input
          type="date"
          required
          class="input w-full"
          :aria-invalid="validationMessage ? 'true' : undefined"
          :aria-describedby="validationMessage ? dateErrorId : undefined"
          :value="modelValue.weekAnchor"
          @input="patch({ weekAnchor: ($event.target as HTMLInputElement).value })"
        />
        <span v-if="weekRange" data-testid="week-range" class="block text-xs text-gray-500 dark:text-dark-400">
          {{ weekRange.start_date }} - {{ weekRange.end_date }}
        </span>
      </label>

      <div v-else class="grid grid-cols-2 gap-2 md:col-span-2 xl:col-span-2">
        <label class="space-y-1.5">
          <span class="text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.organizationUsage.filters.startDate') }}</span>
          <input
            data-testid="custom-start"
            type="date"
            class="input w-full"
            :aria-invalid="validationMessage ? 'true' : undefined"
            :aria-describedby="validationMessage ? dateErrorId : undefined"
            :value="modelValue.customStart"
            @input="patch({ customStart: ($event.target as HTMLInputElement).value })"
          />
        </label>
        <label class="space-y-1.5">
          <span class="text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.organizationUsage.filters.endDate') }}</span>
          <input
            data-testid="custom-end"
            type="date"
            class="input w-full"
            :aria-invalid="validationMessage ? 'true' : undefined"
            :aria-describedby="validationMessage ? dateErrorId : undefined"
            :value="modelValue.customEnd"
            @input="patch({ customEnd: ($event.target as HTMLInputElement).value })"
          />
        </label>
      </div>

      <label class="space-y-1.5">
        <span class="text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.organizationUsage.filters.organization') }}</span>
        <Select
          :model-value="modelValue.organization"
          :options="organizationOptions"
          :searchable="false"
          @update:model-value="patch({ organization: $event as OrganizationUsageOrganizationFilter, department_id: 'all' })"
          :disabled="scope != null && !scope.unrestricted && scope.organizations.length === 1"
        />
      </label>

      <label class="space-y-1.5">
        <span class="text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.departments.department') }}</span>
        <Select :model-value="modelValue.department_id ?? 'all'" :options="departmentOptions" :disabled="modelValue.organization === 'all' || (scope != null && !scope.unrestricted && scope.departments.length === 1)" @update:model-value="patch({ department_id: String($event) })" />
      </label>
      <label class="space-y-1.5">
        <span class="text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.departments.platform') }}</span>
        <Select :model-value="modelValue.platform ?? 'all'" :options="platformOptions" @update:model-value="patch({ platform: String($event) })" />
      </label>

      <label class="space-y-1.5 xl:col-span-2">
        <span class="text-xs font-medium text-gray-600 dark:text-dark-300">{{ t('admin.organizationUsage.filters.email') }}</span>
        <div class="relative">
          <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
          <input
            type="search"
            class="input w-full pl-9"
            :placeholder="t('admin.organizationUsage.filters.emailPlaceholder')"
            :value="modelValue.q"
            @input="patch({ q: ($event.target as HTMLInputElement).value })"
            @keydown.enter.prevent="apply"
          />
        </div>
      </label>

      <div class="flex items-end gap-2">
        <button type="button" data-testid="apply-filters" class="btn btn-primary" :disabled="loading" @click="apply">
          <Icon name="search" size="sm" class="mr-1.5" />
          {{ t('admin.organizationUsage.actions.query') }}
        </button>
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="reset">
          <Icon name="refresh" size="sm" class="mr-1.5" />
          {{ t('admin.organizationUsage.actions.reset') }}
        </button>
      </div>
    </div>

    <p
      v-if="validationMessage"
      :id="dateErrorId"
      data-testid="date-error"
      role="alert"
      aria-live="polite"
      class="mt-2 text-sm text-red-600 dark:text-red-400"
    >
      {{ validationMessage }}
    </p>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import type { DepartmentScope } from '@/api/admin/departments'
import type { OrganizationUsageOrganizationFilter, OrganizationUsageRange } from '@/api/admin/organizationUsage'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import {
  formatOrganizationUsageOrganization,
  getMonthDateRange,
  getWeekDateRange,
  validateCustomDateRange,
  type OrganizationUsageReportMode
} from '@/utils/organizationUsageReport'

export interface OrganizationUsageFilterDraft {
  department_id?: string
  platform?: string
  mode: OrganizationUsageReportMode
  month: string
  weekAnchor: string
  customStart: string
  customEnd: string
  organization: OrganizationUsageOrganizationFilter
  q: string
}

const props = defineProps<{
  modelValue: OrganizationUsageFilterDraft
  loading: boolean
  exporting: boolean
  exportDisabled?: boolean
  scope?: DepartmentScope | null
}>()

const emit = defineEmits<{
  'update:modelValue': [value: OrganizationUsageFilterDraft]
  apply: [range: OrganizationUsageRange]
  reset: []
  export: []
}>()

const { t } = useI18n()
const dateErrorId = 'organization-usage-date-error'
const validationMessage = ref('')

const modes = computed(() => ([
  { value: 'month' as const, label: t('admin.organizationUsage.filters.monthly') },
  { value: 'week' as const, label: t('admin.organizationUsage.filters.weekly') },
  { value: 'custom' as const, label: t('admin.organizationUsage.filters.custom') }
]))

const organizationOptions = computed(() => {
  const allowed = props.scope?.organizations ?? ['xunyou', 'wsdashi', 'other']
  return [{ value: 'all', label: t('admin.organizationUsage.organizations.all') }, ...allowed.map(value => ({ value, label: formatOrganizationUsageOrganization(value, t('admin.organizationUsage.organizations.other')) }))]
})
const departmentOptions = computed(() => {
  const options = [{ value: 'all', label: t('admin.departments.allDepartments') }]
  if (props.modelValue.organization === 'all') return options
  for (const department of props.scope?.departments ?? []) {
    if (department.organization_key === props.modelValue.organization) options.push({ value: String(department.id), label: department.name })
  }
  if (props.scope?.unrestricted) options.push({ value: 'unassigned', label: t('admin.departments.unassigned') })
  return options
})
const platformOptions = computed(() => [
  { value: 'all', label: t('admin.departments.allPlatforms') },
  ...[['openai', 'OpenAI'], ['anthropic', 'Anthropic'], ['gemini', 'Gemini'], ['antigravity', 'Antigravity'], ['grok', 'Grok'], ['kimi', 'Kimi'], ['zhipu', 'Zhipu'], ['deepseek', 'DeepSeek'], ['minimax', 'MiniMax'], ['opencode_go', 'OpenCode Go'], ['kiro', 'Kiro']].map(([value, label]) => ({ value, label })),
  { value: 'unknown', label: t('admin.departments.unknownPlatform') }
])

const weekRange = computed(() => {
  const anchor = props.modelValue.weekAnchor
  if (!anchor || !validateCustomDateRange(anchor, anchor).valid) return null
  return getWeekDateRange(anchor)
})

function patch(value: Partial<OrganizationUsageFilterDraft>) {
  validationMessage.value = ''
  emit('update:modelValue', { ...props.modelValue, ...value })
}

function reset() {
  validationMessage.value = ''
  emit('reset')
}

function setValidationError(error: 'required' | 'invalid_date' | 'start_after_end' | 'range_too_long') {
  const key = {
    required: 'required',
    invalid_date: 'invalidDate',
    start_after_end: 'startAfterEnd',
    range_too_long: 'rangeTooLong'
  }[error]
  validationMessage.value = t(`admin.organizationUsage.validation.${key}`)
}

function apply() {
  if (props.modelValue.mode === 'month') {
    if (!props.modelValue.month) {
      setValidationError('required')
      return
    }
    const date = `${props.modelValue.month}-01`
    const validation = validateCustomDateRange(date, date)
    if (!validation.valid) {
      setValidationError(validation.error)
      return
    }
    emit('apply', getMonthDateRange(date))
    return
  }
  if (props.modelValue.mode === 'week') {
    if (!props.modelValue.weekAnchor) {
      setValidationError('required')
      return
    }
    if (!weekRange.value) {
      setValidationError('invalid_date')
      return
    }
    emit('apply', weekRange.value)
    return
  }

  const validation = validateCustomDateRange(props.modelValue.customStart, props.modelValue.customEnd)
  if (!validation.valid) {
    setValidationError(validation.error)
    return
  }
  emit('apply', { start_date: props.modelValue.customStart, end_date: props.modelValue.customEnd })
}
</script>
