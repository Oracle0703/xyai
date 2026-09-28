<template>
  <section class="space-y-3" :data-testid="`gpt-quota-column-${groupKey}`">
    <header class="flex items-center justify-between rounded-2xl border border-gray-200/80 bg-white/80 px-4 py-3 shadow-sm dark:border-dark-700 dark:bg-dark-800/80">
      <div class="flex items-center gap-2.5">
        <span class="h-2.5 w-2.5 rounded-full" :class="groupKey === 'xunyou' ? 'bg-sky-500' : 'bg-violet-500'" />
        <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ title }}</h2>
      </div>
      <span class="rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-dark-300">{{ t('gptQuota.count', { count: items.length }) }}</span>
    </header>

    <div v-if="!items.length" class="card p-6 text-center text-sm text-gray-500 dark:text-dark-400">
      {{ t('gptQuota.emptyGroup') }}
    </div>

    <article v-for="item in items" :key="item.id" class="group rounded-2xl border border-gray-200/80 bg-white p-5 shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:border-primary-200 hover:shadow-md dark:border-dark-700 dark:bg-dark-800/80 dark:hover:border-primary-800" data-testid="gpt-quota-card">
      <div class="flex items-center justify-between gap-2">
        <strong class="truncate text-sm font-semibold text-gray-900 dark:text-white" :title="item.display_name">
          {{ item.display_name }}
        </strong>
        <span v-if="item.stale" class="badge badge-warning shrink-0">{{ t('gptQuota.stale') }}</span>
      </div>

      <p v-if="!item.sampled_at" class="mt-3 text-sm text-gray-500 dark:text-dark-400">{{ t('gptQuota.noData') }}</p>
      <template v-else>
        <div v-for="row in windowRows(item)" :key="row.key" class="mt-3">
          <div class="flex items-center justify-between text-sm">
            <span class="text-gray-600 dark:text-dark-300">{{ row.label }}</span>
            <span class="font-medium tabular-nums text-gray-900 dark:text-white">{{ row.value }}</span>
          </div>
          <div v-if="row.window" class="mt-2 h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
            <div class="h-full rounded-full transition-all" :class="barClass(row.window.remaining_percent)" :style="{ width: `${row.window.remaining_percent}%` }" />
          </div>
          <p v-if="row.window" class="mt-1 text-xs text-gray-500 dark:text-dark-400" :title="row.resetTitle">{{ row.reset }}</p>
        </div>
        <p class="mt-3 text-xs text-gray-400 dark:text-dark-500">
          {{ t('gptQuota.sampledAt', { time: formatDateTimeToMinute(item.sampled_at) }) }}
        </p>
      </template>
    </article>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { GPTQuotaGroupKey, GPTQuotaUserCard, GPTQuotaWindow } from '@/api/gptQuotaDisplay'
import { formatCountdown, formatDateTimeToMinute } from '@/utils/format'

const props = defineProps<{
  groupKey: GPTQuotaGroupKey
  title: string
  items: GPTQuotaUserCard[]
  /** 当前时间戳（毫秒），由页面本地定时刷新，驱动重置倒计时；不产生网络请求。 */
  now: number
}>()

const { t } = useI18n()

interface WindowRow {
  key: string
  label: string
  value: string
  window: GPTQuotaWindow | null
  reset: string
  resetTitle: string
}

function resetText(window: GPTQuotaWindow): { reset: string; resetTitle: string } {
  if (!window.reset_at) return { reset: t('gptQuota.resetUnknown'), resetTitle: '' }
  const resetMs = new Date(window.reset_at).getTime()
  const title = formatDateTimeToMinute(window.reset_at)
  // 倒计时结束只提示待更新，不把剩余比例改成 100%。
  if (Number.isNaN(resetMs) || resetMs <= props.now) return { reset: t('gptQuota.resetPending'), resetTitle: title }
  const countdown = formatCountdown(window.reset_at)
  return { reset: countdown ? t('gptQuota.resetIn', { time: countdown }) : t('gptQuota.resetAt', { time: title }), resetTitle: title }
}

function windowRows(item: GPTQuotaUserCard): WindowRow[] {
  return [
    { key: 'five_hour', label: t('gptQuota.fiveHour'), window: item.five_hour },
    { key: 'seven_day', label: t('gptQuota.sevenDay'), window: item.seven_day },
  ].map(({ key, label, window }) => ({
    key,
    label,
    window,
    value: window ? `${window.remaining_percent.toFixed(1)}%` : t('gptQuota.notProvided'),
    ...(window ? resetText(window) : { reset: '', resetTitle: '' }),
  }))
}

function barClass(remaining: number): string {
  if (remaining <= 10) return 'bg-red-500'
  if (remaining <= 30) return 'bg-amber-500'
  return 'bg-emerald-500'
}
</script>
