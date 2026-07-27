<template>
  <section class="border-b border-gray-200 py-5 dark:border-dark-700" data-testid="organization-usage-trend">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="min-w-0">
        <h2 class="text-base font-semibold text-gray-900 dark:text-white">
          {{ t('admin.organizationUsage.trend.title') }}
        </h2>
        <p v-if="asOfLabel" class="mt-1 text-xs text-gray-500 dark:text-dark-400">
          {{ asOfLabel }}
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-1" role="group" :aria-label="t('admin.organizationUsage.trend.granularity')">
        <button
          v-for="option in granularityOptions"
          :key="option.value"
          type="button"
          class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors"
          :class="granularity === option.value
            ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/40 dark:text-primary-300'
            : 'text-gray-600 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-800'"
          :data-testid="`trend-granularity-${option.value}`"
          :aria-pressed="granularity === option.value"
          :disabled="loading"
          @click="emit('update:granularity', option.value)"
        >
          {{ option.label }}
        </button>
      </div>
    </div>

    <div v-if="loading" class="mt-4 flex h-64 items-center justify-center" data-testid="trend-loading">
      <LoadingSpinner />
    </div>
    <div
      v-else-if="error"
      class="mt-4 flex h-64 flex-col items-center justify-center gap-3 text-sm text-red-600 dark:text-red-400"
      data-testid="trend-error"
      role="alert"
    >
      <span>{{ error }}</span>
      <button type="button" class="btn btn-secondary" data-testid="trend-retry" @click="emit('retry')">
        <Icon name="refresh" size="sm" class="mr-1.5" />
        {{ t('admin.organizationUsage.trend.retry') }}
      </button>
    </div>
    <div v-else-if="!points.length" class="mt-4 flex h-64 items-center justify-center text-sm text-gray-500 dark:text-dark-400" data-testid="trend-empty">
      {{ t('admin.organizationUsage.common.noData') }}
    </div>
    <div v-else-if="chartData" class="mt-4 h-64" data-testid="trend-chart">
      <Line :data="chartData" :options="lineOptions" />
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
  Title,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { Line } from 'vue-chartjs'

import type {
  OrganizationUsageGranularity,
  OrganizationUsageRange,
  OrganizationUsageTrendPoint
} from '@/api/admin/organizationUsage'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Title, Tooltip, Legend, Filler)

const props = defineProps<{
  points: OrganizationUsageTrendPoint[]
  granularity: OrganizationUsageGranularity
  loading?: boolean
  error?: string
  range?: OrganizationUsageRange | null
  dataThrough?: string
}>()

const emit = defineEmits<{
  'update:granularity': [value: OrganizationUsageGranularity]
  retry: []
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
  { value: 'day' as const, label: t('admin.organizationUsage.trend.day') },
  { value: 'week' as const, label: t('admin.organizationUsage.trend.week') },
  { value: 'month' as const, label: t('admin.organizationUsage.trend.month') }
])

const asOfLabel = computed(() => {
  const asOf = props.range?.as_of
  if (!asOf) return ''
  try {
    return t('admin.organizationUsage.trend.asOf', { value: new Date(asOf).toLocaleString() })
  } catch {
    return t('admin.organizationUsage.trend.asOf', { value: asOf })
  }
})

const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb',
  input: '#3b82f6',
  output: '#10b981',
  cacheRead: '#06b6d4',
  requests: '#8b5cf6'
}))

const densePoints = computed(() => props.points.length > 60)
const pointRadius = computed(() => (densePoints.value ? 0 : 2))

const chartData = computed(() => {
  if (!props.points.length) return null
  const colors = chartColors.value
  const radius = pointRadius.value
  return {
    labels: props.points.map((p) => p.period_start),
    datasets: [
      {
        label: t('admin.organizationUsage.metrics.inputTokens'),
        data: props.points.map((p) => p.input_tokens),
        borderColor: colors.input,
        backgroundColor: `${colors.input}20`,
        fill: false,
        yAxisID: 'y',
        cubicInterpolationMode: 'monotone' as const,
        pointRadius: radius,
        pointHoverRadius: 4,
        tension: 0.3
      },
      {
        label: t('admin.organizationUsage.metrics.outputTokens'),
        data: props.points.map((p) => p.output_tokens),
        borderColor: colors.output,
        backgroundColor: `${colors.output}20`,
        fill: false,
        yAxisID: 'y',
        cubicInterpolationMode: 'monotone' as const,
        pointRadius: radius,
        pointHoverRadius: 4,
        tension: 0.3
      },
      {
        label: t('admin.organizationUsage.metrics.cacheTokens'),
        data: props.points.map((p) => p.cache_read_tokens),
        borderColor: colors.cacheRead,
        backgroundColor: `${colors.cacheRead}20`,
        fill: false,
        yAxisID: 'y',
        cubicInterpolationMode: 'monotone' as const,
        pointRadius: radius,
        pointHoverRadius: 4,
        tension: 0.3
      },
      {
        label: t('admin.organizationUsage.metrics.requests'),
        data: props.points.map((p) => p.requests),
        borderColor: colors.requests,
        backgroundColor: `${colors.requests}20`,
        fill: false,
        yAxisID: 'yRequests',
        cubicInterpolationMode: 'monotone' as const,
        pointRadius: radius,
        pointHoverRadius: 4,
        tension: 0.3
      }
    ]
  }
})

function formatTokens(value: number): string {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(2)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(2)}K`
  return value.toLocaleString()
}

const lineOptions = computed(() => {
  const colors = chartColors.value
  const points = props.points
  const asOf = props.range?.as_of
  return {
    responsive: true,
    maintainAspectRatio: false,
    interaction: {
      intersect: false,
      mode: 'index' as const
    },
    plugins: {
      legend: {
        position: 'top' as const,
        labels: {
          color: colors.text,
          usePointStyle: true,
          pointStyle: 'circle',
          padding: 12,
          font: { size: 11 }
        }
      },
      tooltip: {
        callbacks: {
          afterTitle: (items: Array<{ dataIndex: number }>) => {
            const index = items[0]?.dataIndex
            const point = index != null ? points[index] : undefined
            if (!point) return ''
            const lines = [`${point.period_start} – ${point.period_end}`]
            if (point.partial) lines.push(t('admin.organizationUsage.common.partial'))
            if (asOf && index === points.length - 1) {
              try {
                lines.push(t('admin.organizationUsage.trend.asOf', { value: new Date(asOf).toLocaleString() }))
              } catch {
                lines.push(t('admin.organizationUsage.trend.asOf', { value: asOf }))
              }
            }
            return lines
          },
          label: (context: { dataset: { label?: string; yAxisID?: string }; raw: unknown }) => {
            const label = context.dataset.label ?? ''
            const raw = Number(context.raw)
            if (context.dataset.yAxisID === 'yRequests') {
              return `${label}: ${raw.toLocaleString()}`
            }
            return `${label}: ${formatTokens(raw)}`
          }
        }
      }
    },
    scales: {
      x: {
        grid: { color: colors.grid },
        ticks: {
          color: colors.text,
          maxTicksLimit: 14,
          font: { size: 10 }
        }
      },
      y: {
        beginAtZero: true,
        position: 'left' as const,
        grid: { color: colors.grid },
        ticks: {
          color: colors.text,
          font: { size: 10 },
          callback: (value: string | number) => formatTokens(Number(value))
        }
      },
      yRequests: {
        beginAtZero: true,
        position: 'right' as const,
        grid: { drawOnChartArea: false },
        ticks: {
          color: colors.requests,
          precision: 0,
          font: { size: 10 },
          callback: (value: string | number) => Number(value).toLocaleString()
        }
      }
    }
  }
})

// Expose chart config for contract tests (F4 minimal behavior assertions).
defineExpose({ chartData, lineOptions, pointRadius, densePoints })
</script>
