<template>
  <div data-testid="gpt-quota-meter" :class="variant === 'ring' ? 'flex min-w-0 flex-col items-center text-center' : ''">
    <template v-if="variant === 'ring'">
      <div
        class="relative shrink-0"
        :class="compact ? 'h-16 w-16' : 'h-24 w-24'"
        role="progressbar"
        aria-valuemin="0"
        aria-valuemax="100"
        :aria-valuenow="remaining ?? undefined"
        :aria-valuetext="window ? `${window.remaining_percent.toFixed(1)}%` : t('gptQuota.notProvided')"
        :aria-label="label"
        :title="reset.title || undefined"
      >
        <svg class="h-full w-full" viewBox="0 0 100 100" fill="none" aria-hidden="true">
          <circle cx="50" cy="50" r="42" stroke-width="8" stroke="currentColor" :class="window ? styles.ringTrack : 'text-gray-100 dark:text-dark-700'" />
          <!-- 起点固定在 12 点；剩余额度下降时，弧线末端逆时针回退。平直端帽确保 0% 无残点、100% 无缺口。 -->
          <circle
            v-if="remaining !== null && remaining > 0"
            cx="50"
            cy="50"
            r="42"
            pathLength="100"
            stroke="currentColor"
            stroke-width="8"
            stroke-linecap="butt"
            :stroke-dasharray="remaining < 100 ? '100 100' : undefined"
            :stroke-dashoffset="remaining < 100 ? 100 - remaining : undefined"
            transform="rotate(-90 50 50)"
            class="transition-[stroke-dashoffset,color] duration-500 ease-out motion-reduce:transition-none"
            :class="styles.ring"
          />
        </svg>
        <span
          class="absolute inset-0 flex items-center justify-center font-semibold tabular-nums"
          :class="[compact ? 'text-xs' : 'text-base', window ? styles.value : 'text-gray-400 dark:text-dark-500']"
        >{{ window ? `${window.remaining_percent.toFixed(1)}%` : '—' }}</span>
      </div>
      <span class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ label }}</span>
      <span v-if="!window" class="mt-1 text-[11px] text-gray-400 dark:text-dark-500">{{ t('gptQuota.notProvided') }}</span>
    </template>
    <template v-else>
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
    </template>
    <p v-if="!compact && window" class="mt-1 max-w-full text-[11px] text-gray-500 dark:text-gray-400" :class="variant === 'ring' ? '' : 'truncate'" :title="reset.title">{{ reset.text }}</p>
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
  /** 管理列表默认保留紧凑额度条，用户看板显式启用圆环。 */
  variant?: 'bar' | 'ring'
}>(), { compact: false, variant: 'bar' })

const { t } = useI18n()

const styles = computed(() => GPT_QUOTA_LEVEL_STYLES[gptQuotaLevel(props.window?.remaining_percent ?? 0)])
const remaining = computed(() => props.window ? Math.min(100, Math.max(0, props.window.remaining_percent)) : null)

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
