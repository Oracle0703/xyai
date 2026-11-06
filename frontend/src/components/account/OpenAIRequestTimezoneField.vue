<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div v-if="managedByParent" data-testid="openai-request-timezone-managed-by-parent">
      <label class="input-label mb-0">{{ t('admin.accounts.openai.requestTimezoneRewrite') }}</label>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.openai.requestTimezoneManagedByParent') }}
      </p>
    </div>
    <template v-else>
      <div class="flex items-center justify-between gap-4">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.openai.requestTimezoneRewrite') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.openai.requestTimezoneRewriteDesc') }}
          </p>
        </div>
        <button
          type="button"
          data-testid="openai-request-timezone-rewrite-toggle"
          role="switch"
          :aria-checked="enabled"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            enabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
          ]"
          @click="emit('update:enabled', !enabled)"
        >
          <span
            :class="[
              'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
              enabled ? 'translate-x-5' : 'translate-x-0'
            ]"
          />
        </button>
      </div>
      <div class="mt-3">
        <label class="input-label">{{ t('admin.accounts.openai.requestTimezone') }}</label>
        <Select
          data-testid="openai-request-timezone-select"
          :model-value="timezone"
          :options="options"
          :loading="loading"
          :disabled="!enabled"
          searchable
          :aria-label="t('admin.accounts.openai.requestTimezone')"
          @update:model-value="selectTimezone"
        />
        <p class="input-hint">{{ t('admin.accounts.openai.requestTimezoneDesc') }}</p>
        <p v-if="loadFailed" class="mt-1 text-xs text-red-500">{{ t('admin.accounts.openai.requestTimezoneLoadFailed') }}</p>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import Select from '@/components/common/Select.vue'

// Emits only on user action so parents can tell an explicit change from the
// initial state; untouched values are never written back.
const props = withDefaults(defineProps<{ enabled: boolean; timezone: string; managedByParent?: boolean }>(), {
  managedByParent: false
})
const emit = defineEmits<{
  (event: 'update:enabled', value: boolean): void
  (event: 'update:timezone', value: string): void
}>()
const { t } = useI18n()
const timezones = ref<string[]>(['America/Los_Angeles'])
const loading = ref(false)
const loadFailed = ref(false)

// A retained value stays selectable even if the list could not be loaded.
const options = computed(() => {
  const names = !props.timezone || timezones.value.includes(props.timezone) ? timezones.value : [props.timezone, ...timezones.value]
  return names.map(value => ({ value, label: value }))
})

const selectTimezone = (value: string | number | boolean | null) => {
  if (typeof value === 'string' && value) emit('update:timezone', value)
}

let requested = false
// The edit modal reuses this component across accounts, so the list is loaded the
// first time a non-shadow account needs it rather than only on mount.
watch(() => props.managedByParent, async managed => {
  if (managed || requested) return
  requested = true
  loading.value = true
  try {
    const result = await adminAPI.accounts.getOpenAIRequestTimezones()
    timezones.value = result.timezones
  } catch {
    loadFailed.value = true
  } finally {
    loading.value = false
  }
}, { immediate: true })
</script>
