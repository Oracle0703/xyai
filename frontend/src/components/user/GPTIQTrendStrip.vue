<template>
  <div>
    <p v-if="showLabel" class="mb-1 text-[10px] text-gray-400 dark:text-dark-500">{{ t('gptIQTest.trendTitle', { days }) }}</p>
    <div class="flex w-full gap-0.5" role="list" :aria-label="t('gptIQTest.trendTitle', { days })" data-testid="gpt-iq-trend">
      <span
        v-for="cell in cells"
        :key="cell.date"
        role="listitem"
        class="h-2 min-w-0 flex-1 rounded-sm"
        :class="cell.point ? gptIQVerdictStyle(cell.point.verdict).cell : 'bg-gray-100 dark:bg-dark-700'"
        :title="cellTitle(cell)"
        :aria-label="cellTitle(cell)"
        :data-verdict="cell.point?.verdict ?? 'none'"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { GPTIQTrendPoint } from '@/api/gptIQTest'
import { gptIQTrendCells, gptIQVerdictLabel, gptIQVerdictStyle } from '@/utils/gptIQTest'

const props = withDefaults(defineProps<{
  trend: GPTIQTrendPoint[]
  today: string
  days?: number
  showLabel?: boolean
}>(), { days: 14, showLabel: false })

const { t, te } = useI18n()

const cells = computed(() => gptIQTrendCells(props.trend, props.today, props.days))

function cellTitle(cell: { date: string; point: GPTIQTrendPoint | null }) {
  if (!cell.point) return t('gptIQTest.trendNoData', { date: cell.date })
  const verdict = gptIQVerdictLabel(cell.point.verdict, t, te)
  return cell.point.graded > 0
    ? t('gptIQTest.trendCellGraded', { date: cell.date, verdict, correct: cell.point.correct, graded: cell.point.graded })
    : t('gptIQTest.trendCell', { date: cell.date, verdict })
}
</script>
