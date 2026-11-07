<template>
  <div>
    <label class="input-label">{{ t('admin.groupOrganizationScope.field') }}</label>
    <div class="flex flex-wrap items-center gap-4">
      <label
        v-for="key in GROUP_ORGANIZATION_KEYS"
        :key="key"
        class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300"
      >
        <input
          type="checkbox"
          :checked="modelValue.includes(key)"
          :disabled="disabled"
          :data-testid="`group-organization-${key}`"
          @change="toggle(key, ($event.target as HTMLInputElement).checked)"
        />
        {{ formatOrganizationUsageOrganization(key) }}
      </label>
    </div>
    <p v-if="status === 'loading'" class="input-hint" data-testid="group-organization-loading">
      {{ t('admin.groupOrganizationScope.loading') }}
    </p>
    <p v-else-if="status === 'error'" class="input-hint text-amber-600 dark:text-amber-400" data-testid="group-organization-load-failed">
      {{ t('admin.groupOrganizationScope.loadScopesFailed') }}
      <button type="button" class="ml-2 underline" data-testid="group-organization-retry" @click="emit('retry')">
        {{ t('admin.groupOrganizationScope.retry') }}
      </button>
    </p>
    <p v-else class="input-hint">{{ t('admin.groupOrganizationScope.fieldHint') }}</p>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { GROUP_ORGANIZATION_KEYS, type GroupOrganizationKey } from '@/api/admin/groupOrganizationScopes'
import { formatOrganizationUsageOrganization } from '@/utils/organizations'

// status describes the current scope this field edits: loading and error mean
// it is not known yet, and the caller keeps the field disabled (K24).
const props = withDefaults(
  defineProps<{ modelValue: GroupOrganizationKey[]; disabled?: boolean; status?: 'ready' | 'loading' | 'error' }>(),
  { disabled: false, status: 'ready' }
)
const emit = defineEmits<{ 'update:modelValue': [value: GroupOrganizationKey[]]; retry: [] }>()
const { t } = useI18n()

function toggle(key: GroupOrganizationKey, checked: boolean) {
  const next = props.modelValue.filter(value => value !== key)
  if (checked) next.push(key)
  emit('update:modelValue', GROUP_ORGANIZATION_KEYS.filter(value => next.includes(value)))
}
</script>
