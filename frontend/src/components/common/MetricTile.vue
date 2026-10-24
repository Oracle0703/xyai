<template>
  <div class="card p-4" :data-testid="testId">
    <div class="flex items-center gap-3">
      <div class="shrink-0 rounded-lg p-2" :class="toneClass.box">
        <Icon :name="icon" size="md" :class="toneClass.icon" :stroke-width="2" />
      </div>
      <div class="min-w-0">
        <p class="truncate text-xs font-medium text-gray-500 dark:text-gray-400">{{ label }}</p>
        <p class="truncate text-xl font-bold text-gray-900 dark:text-white" :title="value">{{ value }}</p>
        <p v-if="hint" class="truncate text-xs text-gray-500 dark:text-gray-400" :title="hint">{{ hint }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Icon from '@/components/icons/Icon.vue'

type Tone = 'primary' | 'success' | 'warning' | 'danger' | 'gray'

// 与用户 Dashboard 统计卡同一套样式：图标色块承担语义色，数值和说明保持文字色。
const props = withDefaults(defineProps<{
  label: string
  value: string
  hint?: string
  icon: InstanceType<typeof Icon>['$props']['name']
  tone?: Tone
  testId?: string
}>(), { tone: 'primary', hint: undefined, testId: undefined })

const TONES: Record<Tone, { box: string; icon: string }> = {
  primary: { box: 'bg-blue-100 dark:bg-blue-900/30', icon: 'text-blue-600 dark:text-blue-400' },
  success: { box: 'bg-emerald-100 dark:bg-emerald-900/30', icon: 'text-emerald-600 dark:text-emerald-400' },
  warning: { box: 'bg-amber-100 dark:bg-amber-900/30', icon: 'text-amber-600 dark:text-amber-400' },
  danger: { box: 'bg-red-100 dark:bg-red-900/30', icon: 'text-red-600 dark:text-red-400' },
  gray: { box: 'bg-gray-100 dark:bg-dark-700', icon: 'text-gray-500 dark:text-gray-400' },
}

const toneClass = computed(() => TONES[props.tone])
</script>
