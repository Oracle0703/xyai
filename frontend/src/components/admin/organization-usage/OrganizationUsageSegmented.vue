<template>
  <div
    class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-dark-700 dark:bg-dark-800"
    role="group"
    :aria-label="ariaLabel"
  >
    <button
      v-for="option in options"
      :key="option.value"
      type="button"
      class="rounded-md font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-60"
      :class="[
        size === 'md' ? 'px-3 py-1.5 text-sm' : 'px-2.5 py-1 text-xs',
        modelValue === option.value
          ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white'
          : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
      ]"
      :data-testid="`${testidPrefix}${option.value}`"
      :aria-pressed="modelValue === option.value"
      :disabled="disabled || option.disabled"
      :title="option.title"
      @click="emit('update:modelValue', option.value)"
    >
      {{ option.label }}
    </button>
  </div>
</template>

<script setup lang="ts" generic="T extends string">
// Same segmented style as the dashboard/usage chart toggles (ModelDistributionChart).
withDefaults(defineProps<{
  modelValue: T
  options: Array<{ value: T; label: string; disabled?: boolean; title?: string }>
  testidPrefix: string
  ariaLabel?: string
  size?: 'sm' | 'md'
  disabled?: boolean
}>(), { size: 'sm', ariaLabel: undefined, disabled: false })

const emit = defineEmits<{ 'update:modelValue': [value: T] }>()
</script>
