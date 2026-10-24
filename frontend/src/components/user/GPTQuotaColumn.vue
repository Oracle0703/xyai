<template>
  <section class="card overflow-hidden" :data-testid="`gpt-quota-column-${groupKey}`">
    <header class="flex items-center justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700">
      <div class="flex min-w-0 items-center gap-2.5">
        <!-- 分组标识色只配合分组名使用，不承担额度状态。 -->
        <span class="h-2.5 w-2.5 shrink-0 rounded-full" :class="groupKey === 'xunyou' ? 'bg-sky-500 dark:bg-sky-600' : 'bg-violet-500'" />
        <h2 class="truncate text-base font-semibold text-gray-900 dark:text-white">{{ title }}</h2>
        <span class="badge badge-gray shrink-0">{{ t('gptQuota.count', { count: items.length }) }}</span>
      </div>
      <p v-if="summary.average !== null" class="shrink-0 text-xs text-gray-500 dark:text-gray-400" data-testid="gpt-quota-group-average">
        {{ t('gptQuota.summary.groupAverage') }}
        <span class="ml-1 text-sm font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatGPTQuotaPercent(summary.average) }}</span>
      </p>
    </header>

    <div v-if="!items.length" class="px-5 py-10 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('gptQuota.emptyGroup') }}
    </div>

    <ul v-else class="divide-y divide-gray-100 dark:divide-dark-700">
      <li v-for="item in items" :key="item.id" class="px-5 py-4" data-testid="gpt-quota-card">
        <div class="flex items-center justify-between gap-3">
          <strong class="truncate text-sm font-semibold text-gray-900 dark:text-white" :title="item.display_name">
            {{ item.display_name }}
          </strong>
          <div class="flex shrink-0 items-center gap-1.5">
            <span v-if="item.stale" class="badge badge-warning">{{ t('gptQuota.stale') }}</span>
            <span v-if="levelOf(item)" class="badge inline-flex items-center gap-1" :class="GPT_QUOTA_LEVEL_STYLES[levelOf(item)!].badge" data-testid="gpt-quota-level">
              <Icon :name="GPT_QUOTA_LEVEL_STYLES[levelOf(item)!].icon" size="xs" :stroke-width="2" />
              {{ t(`gptQuota.levels.${levelOf(item)}`) }}
            </span>
          </div>
        </div>

        <p v-if="!item.sampled_at" class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('gptQuota.noData') }}</p>
        <template v-else>
          <div class="mt-3 grid gap-3" :class="visibleGPTQuotaWindows(item).length > 1 ? 'sm:grid-cols-2' : ''">
            <GPTQuotaMeter
              v-for="row in visibleGPTQuotaWindows(item)"
              :key="row.key"
              :label="row.key === 'five_hour' ? t('gptQuota.fiveHour') : t('gptQuota.sevenDay')"
              :window="row.window"
              :now="now"
            />
          </div>
          <p class="mt-2 text-[11px] text-gray-400 dark:text-dark-500">
            {{ t('gptQuota.sampledAt', { time: formatDateTimeToMinute(item.sampled_at) }) }}
          </p>
        </template>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import GPTQuotaMeter from '@/components/user/GPTQuotaMeter.vue'
import type { GPTQuotaGroupKey, GPTQuotaUserCard } from '@/api/gptQuotaDisplay'
import {
  GPT_QUOTA_LEVEL_STYLES,
  averageSevenDayRemaining,
  effectiveGPTQuotaRemaining,
  formatGPTQuotaPercent,
  gptQuotaLevel,
  visibleGPTQuotaWindows,
  type GPTQuotaLevel,
} from '@/utils/gptQuota'
import { formatDateTimeToMinute } from '@/utils/format'

const props = defineProps<{
  groupKey: GPTQuotaGroupKey
  title: string
  items: GPTQuotaUserCard[]
  /** 当前时间戳（毫秒），由页面本地定时刷新，驱动重置倒计时；不产生网络请求。 */
  now: number
}>()

const { t } = useI18n()

const summary = computed(() => averageSevenDayRemaining(props.items.filter((item) => item.sampled_at)))

function levelOf(item: GPTQuotaUserCard): GPTQuotaLevel | null {
  if (!item.sampled_at) return null
  const remaining = effectiveGPTQuotaRemaining(item)
  return remaining === null ? null : gptQuotaLevel(remaining)
}
</script>
