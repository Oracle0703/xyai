<template>
  <div data-testid="gpt-quota-meter">
    <div class="flex items-baseline justify-between gap-2 text-xs">
      <span class="text-gray-500 dark:text-gray-400">{{ label }}</span>
      <span class="font-semibold tabular-nums text-gray-900 dark:text-white" :class="compact ? 'text-xs' : 'text-sm'">
        {{ window ? `${window.remaining_percent.toFixed(1)}%` : t('gptQuota.notProvided') }}
      </span>
    </div>
    <!-- 填充色表示分级，底轨用同色系浅色，额度见底时整条仍能读出状态。 -->
    <div
      class="overflow-hidden rounded-full"
      :class="[compact ? 'mt-1 h-1.5' : 'mt-1.5 h-2', window ? styles.track : 'bg-gray-100 dark:bg-dark-700']"
      role="progressbar"
      aria-valuemin="0"
      aria-valuemax="100"
      :aria-valuenow="window?.remaining_percent"
      :aria-label="label"
      :title="reset.title || undefined"
    >
      <div v-if="window" class="h-full rounded-full transition-all" :class="styles.fill" :style="{ width: `${window.remaining_percent}%` }" />
    </div>
    <p v-if="!compact && window" class="mt-1 truncate text-[11px] text-gray-500 dark:text-gray-400" :title="reset.title">{{ reset.text }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { GPTQuotaWindow } from '@/api/gptQuotaDisplay'
import { GPT_QUOTA_LEVEL_STYLES, gptQuotaLevel } from '@/utils/gptQuota'
import { formatCountdown, formatDateTimeToMinute } from '@/utils/format'

const props = withDefaults(defineProps<{
  label: string
  window: GPTQuotaWindow | null
  /** 当前时间戳（毫秒），由页面本地定时刷新，驱动重置倒计时；不产生网络请求。 */
  now: number
  compact?: boolean
}>(), { compact: false })

const { t } = useI18n()

const styles = computed(() => GPT_QUOTA_LEVEL_STYLES[gptQuotaLevel(props.window?.remaining_percent ?? 0)])

const reset = computed(() => {
  const window = props.window
  if (!window) return { text: '', title: '' }
  if (!window.reset_at) return { text: t('gptQuota.resetUnknown'), title: '' }
  const resetMs = new Date(window.reset_at).getTime()
  const title = formatDateTimeToMinute(window.reset_at)
  // 倒计时结束只提示待更新，不把剩余比例改成 100%。
  if (Number.isNaN(resetMs) || resetMs <= props.now) return { text: t('gptQuota.resetPending'), title }
  const countdown = formatCountdown(window.reset_at)
  return { text: countdown ? t('gptQuota.resetIn', { time: countdown }) : t('gptQuota.resetAt', { time: title }), title }
})
</script>
