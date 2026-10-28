<template>
  <section class="space-y-4" :aria-label="t('admin.tokenAnalysis.overview')">
    <div class="grid grid-cols-2 gap-4 md:grid-cols-3 xl:grid-cols-5">
      <div
        v-for="metric in metrics"
        :key="metric.key"
        class="card flex min-w-0 flex-col p-4"
        :data-testid="`token-analysis-metric-${metric.key}`"
      >
        <div class="flex min-w-0 items-center gap-2 text-xs font-medium text-gray-500 dark:text-dark-400">
          <span class="shrink-0 rounded-lg p-1.5" :class="metric.tone">
            <Icon :name="metric.icon" size="sm" />
          </span>
          <span class="truncate">{{ metric.label }}</span>
          <HelpTooltip v-if="metric.help" :content="metric.help" />
        </div>
        <div class="mt-2 truncate text-2xl font-bold text-gray-900 dark:text-white" :title="metric.title">{{ metric.value }}</div>
        <div v-if="metric.progress" class="mt-3">
          <div class="flex items-center justify-between gap-2 text-xs">
            <span class="truncate text-gray-500 dark:text-dark-400">{{ metric.progress.label }}</span>
            <span class="font-semibold tabular-nums text-gray-700 dark:text-gray-200">{{ metric.progress.text }}</span>
          </div>
          <div class="mt-1.5 h-1.5 overflow-hidden rounded-full bg-blue-100 dark:bg-blue-900/40">
            <div class="h-full rounded-full bg-blue-500" :style="{ width: `${metric.progress.ratio * 100}%` }" />
          </div>
        </div>
        <dl class="mt-auto space-y-1 pt-3 text-xs">
          <div v-for="row in metric.rows" :key="row.label" class="flex items-center justify-between gap-2">
            <dt class="truncate text-gray-500 dark:text-dark-400">{{ row.label }}</dt>
            <dd class="shrink-0 font-medium tabular-nums" :class="row.tone || 'text-gray-700 dark:text-gray-200'" :title="row.title">{{ row.value }}</dd>
          </div>
        </dl>
      </div>
    </div>

    <div class="grid grid-cols-1 items-start gap-4 lg:grid-cols-5">
      <div class="card p-4 lg:col-span-2" data-testid="token-analysis-composition">
        <div class="flex items-center justify-between gap-3">
          <h3 class="ta-card-title">{{ t('admin.tokenAnalysis.tokenComposition') }}</h3>
          <span class="text-xs text-gray-500 dark:text-dark-400" :title="formatInteger(compositionTotal)">
            {{ t('admin.tokenAnalysis.tokenCompositionTotal', { value: formatTokenMetric(compositionTotal) }) }}
          </span>
        </div>
        <!-- 段与段之间留 2px 表面间隙, 不画描边; 宽度按 flex-grow 分配以扣除间隙。 -->
        <div v-if="compositionTotal > 0" class="mt-4 flex h-2.5 gap-0.5 overflow-hidden rounded-full">
          <div
            v-for="segment in visibleSegments"
            :key="segment.key"
            class="h-full min-w-[2px]"
            :class="segment.color"
            :style="{ flex: `${segment.value} 1 0%` }"
            :title="`${segment.label}: ${formatInteger(segment.value)} (${percent(segment.share)})`"
          />
        </div>
        <div v-else class="mt-4 h-2.5 rounded-full bg-gray-100 dark:bg-dark-700" />
        <dl class="mt-4 divide-y divide-gray-100 dark:divide-dark-700">
          <div v-for="segment in segments" :key="segment.key" class="flex items-center gap-3 py-2.5 text-sm">
            <span class="h-2.5 w-2.5 shrink-0 rounded-sm" :class="segment.color" />
            <dt class="min-w-0 flex-1 truncate text-gray-600 dark:text-gray-300">{{ segment.label }}</dt>
            <dd class="font-semibold tabular-nums text-gray-900 dark:text-white" :title="formatInteger(segment.value)">{{ formatTokenMetric(segment.value) }}</dd>
            <dd class="w-14 text-right text-xs tabular-nums text-gray-500 dark:text-dark-400">{{ percent(segment.share) }}</dd>
          </div>
        </dl>
      </div>

      <div class="card p-4 lg:col-span-3" data-testid="token-analysis-risk-reasons">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
            <h3 class="ta-card-title">{{ t('admin.tokenAnalysis.riskReasons') }}</h3>
            <span class="text-xs text-gray-500 dark:text-dark-400">
              {{ t('admin.tokenAnalysis.riskRate') }} {{ percent(summary?.risk_request_rate) }}
            </span>
          </div>
          <button
            type="button"
            class="inline-flex items-center gap-1 rounded text-xs font-medium text-primary-600 hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-primary-400"
            :aria-expanded="showLegend"
            @click="showLegend = !showLegend"
          >
            <Icon name="questionCircle" size="xs" />
            {{ showLegend ? t('admin.tokenAnalysis.hideRiskLegend') : t('admin.tokenAnalysis.riskLegend') }}
          </button>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.tokenAnalysis.riskReasonsHint') }}</p>

        <ul v-if="riskReasons.length" class="mt-3 space-y-0.5">
          <li v-for="reason in riskReasons" :key="reason.code">
            <button
              type="button"
              class="grid w-full grid-cols-[minmax(0,11rem)_minmax(0,1fr)_3.5rem] items-center gap-3 rounded-lg px-2 py-1.5 text-left text-xs transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
              :class="activeRiskReason === reason.code
                ? 'bg-primary-50 ring-1 ring-primary-200 dark:bg-primary-900/20 dark:ring-primary-800'
                : 'hover:bg-gray-50 dark:hover:bg-dark-700/60'"
              :aria-pressed="activeRiskReason === reason.code"
              :title="riskDesc(reason.code) || reason.code"
              @click="emit('toggle-risk-reason', reason.code)"
            >
              <span class="truncate font-medium text-gray-700 dark:text-gray-200">{{ riskLabel(reason.code) }}</span>
              <span class="block h-2">
                <span
                  class="block h-full rounded-r"
                  :class="riskBarClass(reason.code)"
                  :style="{ width: `max(2px, ${(reason.count / maxRiskCount) * 100}%)` }"
                />
              </span>
              <span class="text-right font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatInteger(reason.count) }}</span>
            </button>
          </li>
        </ul>
        <p v-else class="mt-6 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('admin.tokenAnalysis.noRiskReasons') }}</p>

        <div v-if="showLegend" class="mt-4 border-t border-gray-100 pt-4 dark:border-dark-700">
          <p class="mb-3 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.tokenAnalysis.riskLegendHint') }}</p>
          <div class="grid grid-cols-1 gap-x-6 gap-y-2.5 md:grid-cols-2">
            <div v-for="code in TOKEN_ANALYSIS_RISK_CODES" :key="code" class="flex items-start gap-2">
              <span class="ta-tag mt-0.5 shrink-0 bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300">{{ riskLabel(code) }}</span>
              <p class="text-xs leading-relaxed text-gray-500 dark:text-dark-400">{{ riskDesc(code) }}</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import type { TokenAnalysisSummary } from '@/api/admin/tokenAnalysis'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  TOKEN_ANALYSIS_RISK_CODES,
  finiteNumber,
  formatCost,
  formatInteger,
  formatRequestMetric,
  formatTokenMetric,
  percent,
  useTokenAnalysisRisk
} from './tokenAnalysisFormat'
import './tokenAnalysis.css'

const props = defineProps<{
  summary: TokenAnalysisSummary | null
  activeRiskReason?: string
}>()

const emit = defineEmits<{
  'toggle-risk-reason': [code: string]
}>()

const { t } = useI18n()
const { riskLabel, riskDesc } = useTokenAnalysisRisk()
const showLegend = ref(false)

type MetricRow = { label: string; value: string; title?: string; tone?: string }

const metrics = computed(() => {
  const s = props.summary
  const coverage = Math.min(Math.max(finiteNumber(s?.archive_coverage), 0), 1)
  const totalCost = finiteNumber(s?.total_actual_cost)
  const riskyCost = finiteNumber(s?.risky_cost)
  return [
    {
      key: 'requests',
      label: t('admin.tokenAnalysis.summary.totalRequests'),
      help: t('admin.tokenAnalysis.help.archivedRequests'),
      icon: 'document' as const,
      tone: 'bg-blue-100 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400',
      value: formatRequestMetric(s?.total_requests),
      title: formatInteger(s?.total_requests),
      progress: { label: t('admin.tokenAnalysis.summary.archiveCoverage'), text: percent(s?.archive_coverage), ratio: coverage },
      rows: [
        { label: t('admin.tokenAnalysis.summary.billedRequests'), value: formatRequestMetric(s?.billed_requests), title: formatInteger(s?.billed_requests) },
        { label: t('admin.tokenAnalysis.matchedUnmatched'), value: `${formatInteger(s?.matched_requests)} / ${formatInteger(s?.unmatched_requests)}` },
        { label: t('admin.tokenAnalysis.unmatchedRate'), value: percent(s?.unmatched_rate) },
        { label: t('admin.tokenAnalysis.summary.humanInputs'), value: formatRequestMetric(s?.human_input_count), title: t('admin.tokenAnalysis.help.humanInputs') }
      ] as MetricRow[]
    },
    {
      key: 'tokens',
      label: t('admin.tokenAnalysis.summary.totalTokens'),
      help: t('admin.tokenAnalysis.help.matchedOnly'),
      icon: 'cube' as const,
      tone: 'bg-amber-100 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400',
      value: formatTokenMetric(s?.total_tokens),
      title: formatInteger(s?.total_tokens),
      rows: [
        { label: t('admin.tokenAnalysis.summary.inputTokens'), value: formatTokenMetric(s?.total_input_tokens), title: formatInteger(s?.total_input_tokens) },
        { label: t('admin.tokenAnalysis.summary.outputTokens'), value: formatTokenMetric(s?.total_output_tokens), title: formatInteger(s?.total_output_tokens) }
      ] as MetricRow[]
    },
    {
      key: 'cost',
      label: t('admin.tokenAnalysis.summary.totalCost'),
      help: t('admin.tokenAnalysis.help.matchedOnly'),
      icon: 'dollar' as const,
      tone: 'bg-emerald-100 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400',
      value: formatHeroCost(totalCost),
      title: formatCost(totalCost),
      rows: [
        {
          label: t('admin.tokenAnalysis.summary.riskyCost'),
          value: formatCost(riskyCost),
          tone: riskyCost > 0 ? 'text-red-600 dark:text-red-400' : undefined
        },
        { label: t('admin.tokenAnalysis.summary.riskyCostShare'), value: totalCost > 0 ? percent(riskyCost / totalCost) : '-' }
      ] as MetricRow[]
    },
    {
      key: 'cache',
      label: t('admin.tokenAnalysis.summary.cacheHitRate'),
      icon: 'bolt' as const,
      tone: 'bg-teal-100 text-teal-600 dark:bg-teal-900/30 dark:text-teal-400',
      value: percent(s?.cache_hit_rate),
      title: percent(s?.cache_hit_rate),
      rows: [
        { label: t('admin.tokenAnalysis.summary.cacheRead'), value: formatTokenMetric(s?.cache_read_tokens), title: formatInteger(s?.cache_read_tokens) },
        { label: t('admin.tokenAnalysis.summary.cacheCreation'), value: formatTokenMetric(s?.cache_creation_tokens), title: formatInteger(s?.cache_creation_tokens) }
      ] as MetricRow[]
    },
    {
      key: 'risk',
      label: t('admin.tokenAnalysis.summary.riskyRequests'),
      icon: 'exclamationTriangle' as const,
      tone: 'bg-red-100 text-red-600 dark:bg-red-900/30 dark:text-red-400',
      value: formatRequestMetric(s?.risky_requests),
      title: formatInteger(s?.risky_requests),
      rows: [
        { label: t('admin.tokenAnalysis.riskRate'), value: percent(s?.risk_request_rate) },
        { label: t('admin.tokenAnalysis.riskReasonKinds'), value: formatInteger(s?.risk_reasons?.length) }
      ] as MetricRow[]
    }
  ]
})

// 构成按四类 Token 自身求和计算占比, 与 total_tokens 口径一致(四类之和)。
const compositionParts = computed(() => {
  const s = props.summary
  return [
    { key: 'input', label: t('admin.tokenAnalysis.summary.inputTokens'), value: finiteNumber(s?.total_input_tokens), color: 'bg-blue-500' },
    { key: 'output', label: t('admin.tokenAnalysis.summary.outputTokens'), value: finiteNumber(s?.total_output_tokens), color: 'bg-emerald-500 dark:bg-emerald-600' },
    { key: 'cacheRead', label: t('admin.tokenAnalysis.summary.cacheRead'), value: finiteNumber(s?.cache_read_tokens), color: 'bg-amber-500 dark:bg-amber-600' },
    { key: 'cacheCreation', label: t('admin.tokenAnalysis.summary.cacheCreation'), value: finiteNumber(s?.cache_creation_tokens), color: 'bg-violet-500' }
  ]
})
const compositionTotal = computed(() => compositionParts.value.reduce((sum, part) => sum + Math.max(part.value, 0), 0))
const segments = computed(() => {
  const total = compositionTotal.value
  return compositionParts.value.map((part) => ({ ...part, share: total > 0 ? Math.max(part.value, 0) / total : 0 }))
})
const visibleSegments = computed(() => segments.value.filter((segment) => segment.value > 0))

// 卡片主值只看量级: >=1 美元保留两位小数并加千分位, 精确值见 title。
function formatHeroCost(value: number): string {
  if (Math.abs(value) < 1) return formatCost(value)
  return `$${new Intl.NumberFormat(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value)}`
}

const riskReasons = computed(() => [...(props.summary?.risk_reasons || [])].sort((a, b) => b.count - a.count))
const maxRiskCount = computed(() => Math.max(1, ...riskReasons.value.map((reason) => reason.count)))

// 选中某原因时强调该条, 其余转灰; 未选中时所有条同一色(按长度比较, 不按色深)。
function riskBarClass(code: string): string {
  if (!props.activeRiskReason || props.activeRiskReason === code) return 'bg-primary-500'
  return 'bg-gray-300 dark:bg-dark-600'
}
</script>
