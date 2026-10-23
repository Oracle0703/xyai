<template>
  <section class="card flex min-w-0 flex-col overflow-hidden" data-testid="token-analysis-user-trend">
    <div class="ta-card-header">
      <div class="flex min-w-0 items-center gap-1">
        <h3 class="ta-card-title">{{ t('admin.tokenAnalysis.usageTrend') }}</h3>
        <HelpTooltip :content="t('admin.tokenAnalysis.usageTrendHelp')" />
        <span class="ml-2 text-xs text-gray-500 dark:text-dark-400">
          {{ t('admin.tokenAnalysis.selectedUsersCount', { count: selectedUsers.length }) }}
        </span>
      </div>
      <OrganizationUsageSegmented
        :model-value="granularity"
        :options="granularityOptions"
        testid-prefix="token-analysis-trend-"
        :aria-label="t('admin.tokenAnalysis.usageTrend')"
        @update:model-value="emit('granularity', $event)"
      />
    </div>

    <div v-if="selectedUsers.length > 0" class="flex flex-wrap items-center gap-2 border-b border-gray-100 px-4 py-2.5 dark:border-dark-700">
      <span
        v-for="user in selectedUsers"
        :key="user.user_id"
        class="inline-flex max-w-[260px] items-center gap-1.5 rounded-full border border-gray-200 bg-white py-0.5 pl-2 pr-1 text-xs text-gray-700 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200"
      >
        <span class="h-2 w-2 shrink-0 rounded-full" :class="USER_TREND_DOT_CLASSES[user.slot]" />
        <span class="truncate" :title="user.email">{{ user.email }}</span>
        <button
          type="button"
          class="rounded-full p-0.5 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-gray-200"
          :aria-label="t('admin.tokenAnalysis.removeTrendUser', { email: user.email })"
          @click="emit('remove', user.user_id)"
        >
          <Icon name="x" size="xs" />
        </button>
      </span>
      <button type="button" class="text-xs font-medium text-gray-500 hover:text-gray-700 dark:text-dark-400 dark:hover:text-gray-200" @click="emit('clear')">
        {{ t('admin.tokenAnalysis.clearTrendUsers') }}
      </button>
    </div>

    <div class="flex-1 p-4">
      <div
        v-if="selectedUsers.length === 0"
        class="flex h-72 flex-col items-center justify-center gap-3 rounded-xl border border-dashed border-gray-200 text-center dark:border-dark-700 md:h-80"
        data-testid="token-analysis-trend-empty"
      >
        <span class="rounded-full bg-gray-100 p-3 text-gray-400 dark:bg-dark-700 dark:text-dark-400">
          <Icon name="trendingUp" size="lg" />
        </span>
        <p class="max-w-xs text-sm text-gray-500 dark:text-dark-400">{{ t('admin.tokenAnalysis.trendEmpty', { max: MAX_SELECTED_TREND_USERS }) }}</p>
      </div>
      <div v-else-if="loading" class="flex h-72 items-center justify-center gap-2 text-sm text-gray-500 dark:text-dark-400 md:h-80">
        <LoadingSpinner />
        {{ t('common.loading') }}
      </div>
      <div v-else-if="error" class="flex h-72 flex-col items-center justify-center gap-3 text-center md:h-80">
        <p class="text-sm text-red-600 dark:text-red-400">{{ t('admin.tokenAnalysis.trendLoadFailed') }}</p>
        <button type="button" class="btn btn-secondary btn-sm" @click="emit('retry')">
          {{ t('admin.tokenAnalysis.trendRetry') }}
        </button>
      </div>
      <template v-else>
        <p v-if="points.length === 0" class="mb-2 text-xs text-gray-500 dark:text-dark-400">
          {{ t('admin.tokenAnalysis.trendNoUsage') }}
        </p>
        <div class="h-72 md:h-80">
          <Line :data="chartData" :options="chartOptions" />
        </div>
      </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  type ChartData,
  type ChartOptions
} from 'chart.js'
import { Line } from 'vue-chartjs'

import OrganizationUsageSegmented from '@/components/admin/organization-usage/OrganizationUsageSegmented.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import type { UserUsageTrendPoint } from '@/types'
import { formatCompactNumber } from '@/utils/format'
import {
  MAX_SELECTED_TREND_USERS,
  USER_TREND_COLORS_DARK,
  USER_TREND_COLORS_LIGHT,
  USER_TREND_DOT_CLASSES,
  formatInteger,
  type TokenAnalysisSelectedUser,
  type TokenAnalysisTrendGranularity
} from './tokenAnalysisFormat'
import './tokenAnalysis.css'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend)

const props = defineProps<{
  selectedUsers: TokenAnalysisSelectedUser[]
  points: UserUsageTrendPoint[]
  granularity: TokenAnalysisTrendGranularity
  loading: boolean
  error: boolean
  canUseHourly: boolean
  startDate: string
  endDate: string
}>()

const emit = defineEmits<{
  granularity: [value: TokenAnalysisTrendGranularity]
  retry: []
  remove: [userID: number]
  clear: []
}>()

const { t } = useI18n()

const isDarkMode = ref(document.documentElement.classList.contains('dark'))
let themeObserver: MutationObserver | null = null

onMounted(() => {
  themeObserver = new MutationObserver(() => {
    isDarkMode.value = document.documentElement.classList.contains('dark')
  })
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})

onUnmounted(() => {
  themeObserver?.disconnect()
  themeObserver = null
})

const granularityOptions = computed(() => [
  { value: 'day' as const, label: t('admin.tokenAnalysis.trendDay') },
  {
    value: 'hour' as const,
    label: t('admin.tokenAnalysis.trendHour'),
    disabled: !props.canUseHourly,
    title: props.canUseHourly ? undefined : t('admin.tokenAnalysis.trendHourSingleDayHint')
  }
])

// 日粒度按筛选范围生成完整日期轴; 小时粒度只在同一日期生成 24 个整点。
function buildTrendLabels(startDate: string, endDate: string, granularity: TokenAnalysisTrendGranularity): string[] {
  if (!startDate || !endDate) return []
  if (granularity === 'hour') {
    if (startDate !== endDate) return []
    return Array.from({ length: 24 }, (_, hour) => `${startDate} ${String(hour).padStart(2, '0')}:00`)
  }

  const labels: string[] = []
  const cursor = new Date(`${startDate}T00:00:00Z`)
  const end = new Date(`${endDate}T00:00:00Z`)
  if (Number.isNaN(cursor.getTime()) || Number.isNaN(end.getTime()) || cursor > end) return labels
  while (cursor <= end) {
    labels.push(cursor.toISOString().slice(0, 10))
    cursor.setUTCDate(cursor.getUTCDate() + 1)
  }
  return labels
}

const chartData = computed<ChartData<'line'>>(() => {
  const labels = buildTrendLabels(props.startDate, props.endDate, props.granularity)
  const values = new Map<string, number>()
  for (const point of props.points) {
    values.set(`${point.user_id}/${point.date}`, point.tokens)
  }
  const palette = isDarkMode.value ? USER_TREND_COLORS_DARK : USER_TREND_COLORS_LIGHT
  // 点少时保留标记, 否则单点/稀疏数据画不出线; 点多时只在悬停显示。
  const pointRadius = labels.length <= 1 ? 4 : labels.length <= 31 ? 2.5 : 0
  return {
    labels,
    datasets: props.selectedUsers.map((user) => {
      const color = palette[user.slot % palette.length]
      return {
        label: user.email,
        // 缺失周期补 0, 完全无用量也渲染零值折线。
        data: labels.map((label) => values.get(`${user.user_id}/${label}`) ?? 0),
        borderColor: color,
        backgroundColor: color,
        borderWidth: 2,
        pointRadius,
        pointHoverRadius: 5,
        fill: false,
        tension: 0.25
      }
    })
  }
})

const chartOptions = computed<ChartOptions<'line'>>(() => {
  const text = isDarkMode.value ? '#cbd5e1' : '#4b5563'
  const grid = isDarkMode.value ? 'rgba(51, 65, 85, 0.6)' : '#f1f5f9'
  return {
    responsive: true,
    maintainAspectRatio: false,
    interaction: {
      intersect: false,
      mode: 'index'
    },
    plugins: {
      // 图上方的已选用户标签(色点 + 邮箱)即图例, 不再重复画 Chart.js 图例。
      legend: { display: false },
      tooltip: {
        callbacks: {
          label: (item) => `${item.dataset.label}: ${formatInteger(Number(item.raw))}`
        }
      }
    },
    scales: {
      x: {
        grid: { display: false },
        ticks: {
          color: text,
          maxRotation: 0,
          autoSkip: true,
          // 小时粒度只展示 HH:00, 日期已由筛选范围给出; tooltip 标题仍是完整标签。
          callback(value) {
            const label = String(this.getLabelForValue(Number(value)))
            return props.granularity === 'hour' ? label.slice(11) : label
          }
        }
      },
      y: {
        beginAtZero: true,
        grid: { color: grid },
        border: { display: false },
        ticks: {
          color: text,
          callback: (value) => formatCompactNumber(Number(value))
        }
      }
    }
  }
})
</script>
