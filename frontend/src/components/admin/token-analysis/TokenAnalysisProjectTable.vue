<template>
  <section class="card overflow-hidden" data-testid="token-analysis-project-ranking">
    <div class="ta-card-header">
      <div class="flex items-center gap-2">
        <h3 class="ta-card-title">{{ t('admin.tokenAnalysis.projectRanking') }}</h3>
        <span class="ta-count">{{ formatInteger(pagination.total) }}</span>
      </div>
      <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.tokenAnalysis.projectRankingHint') }}</span>
    </div>
    <div class="overflow-x-auto transition-opacity" :class="{ 'opacity-60': loading }">
      <table class="ta-table">
        <thead>
          <tr>
            <th>{{ t('admin.tokenAnalysis.project') }}</th>
            <th>{{ t('admin.tokenAnalysis.user') }}</th>
            <th class="ta-num">
              <button type="button" class="ta-sort" :class="{ 'text-primary-600 dark:text-primary-400': sortBy === 'request_count' }" @click="emit('sort', 'request_count')">
                {{ t('admin.tokenAnalysis.requests') }}
                <Icon :name="sortIcon('request_count')" size="xs" />
              </button>
            </th>
            <th class="ta-num">
              <button type="button" class="ta-sort" :class="{ 'text-primary-600 dark:text-primary-400': sortBy === 'total_tokens' }" @click="emit('sort', 'total_tokens')">
                {{ t('admin.tokenAnalysis.tokens') }}
                <Icon :name="sortIcon('total_tokens')" size="xs" />
              </button>
            </th>
            <th class="ta-num">{{ t('admin.tokenAnalysis.inputOutput') }}</th>
            <th class="ta-num">{{ t('admin.tokenAnalysis.cacheTokens') }}</th>
            <th class="ta-num">
              <span class="inline-flex items-center">
                {{ t('admin.tokenAnalysis.outputInputRatio') }}
                <HelpTooltip :content="t('admin.tokenAnalysis.outputInputRatioHelp', { threshold: percent(PROJECT_OUTPUT_INPUT_HEALTHY_RATIO) })" />
              </span>
            </th>
            <th>
              <button type="button" class="ta-sort" :class="{ 'text-primary-600 dark:text-primary-400': sortBy === 'last_event_time' }" @click="emit('sort', 'last_event_time')">
                {{ t('admin.tokenAnalysis.lastActive') }}
                <Icon :name="sortIcon('last_event_time')" size="xs" />
              </button>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="row in projects"
            :key="`${row.project}-${row.user_id || 0}`"
            :class="activeProject === row.project ? 'bg-primary-50/60 dark:bg-primary-900/10' : ''"
          >
            <td>
              <button
                type="button"
                class="inline-flex max-w-[240px] items-center gap-1.5 rounded text-left font-medium hover:text-primary-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:hover:text-primary-400"
                :class="row.project === 'unattributed' ? 'italic text-gray-400 dark:text-dark-400' : ''"
                :title="activeProject === row.project ? t('admin.tokenAnalysis.clearProjectFilter') : t('admin.tokenAnalysis.filterByProject')"
                @click="emit('filter-project', row.project)"
              >
                <Icon name="filter" size="xs" class="shrink-0" :class="activeProject === row.project ? 'text-primary-600 dark:text-primary-400' : 'text-gray-300 dark:text-dark-600'" />
                <span class="truncate">{{ row.project === 'unattributed' ? t('admin.tokenAnalysis.unattributed') : row.project }}</span>
              </button>
            </td>
            <td>
              <div class="max-w-[220px] truncate" :title="row.user_email">{{ row.user_email || '-' }}</div>
            </td>
            <td class="ta-num">
              <div :title="formatInteger(row.request_count)">{{ formatInteger(row.request_count) }}</div>
              <div class="ta-sub">{{ t('admin.tokenAnalysis.matched') }} {{ formatInteger(row.matched_request_count) }}</div>
            </td>
            <td class="ta-num">
              <span class="font-medium" :title="formatInteger(row.total_tokens)">{{ formatCompactNumber(row.total_tokens) }}</span>
              <div class="ml-auto mt-1 h-1 w-20 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                <div class="h-full rounded-full bg-primary-500" :style="{ width: `${share(row.total_tokens) * 100}%` }" />
              </div>
            </td>
            <td class="ta-num text-xs text-gray-600 dark:text-gray-300" :title="`${formatInteger(row.input_tokens)} / ${formatInteger(row.output_tokens)}`">
              {{ formatCompactNumber(row.input_tokens) }} / {{ formatCompactNumber(row.output_tokens) }}
            </td>
            <td class="ta-num text-xs text-gray-600 dark:text-gray-300" :title="formatInteger(row.cache_read_tokens + row.cache_creation_tokens)">
              {{ formatCompactNumber(row.cache_read_tokens + row.cache_creation_tokens) }}
            </td>
            <td class="ta-num">
              <span
                class="text-xs font-semibold"
                :class="ratioClass(row)"
                :data-test="ratioDataTest(row)"
                :title="ratioTitle(row)"
              >
                {{ formatRatio(row) }}
              </span>
            </td>
            <td class="whitespace-nowrap text-xs text-gray-500 dark:text-dark-400">{{ formatTimeCN(row.last_event_time) }}</td>
          </tr>
          <tr v-if="!loading && projects.length === 0">
            <td colspan="8" class="ta-empty">{{ t('common.noData') }}</td>
          </tr>
        </tbody>
      </table>
    </div>
    <Pagination
      v-if="pagination.total > 0"
      :page="pagination.page"
      :page-size="pagination.page_size"
      :total="pagination.total"
      @update:page="emit('page', $event)"
      @update:page-size="emit('page-size', $event)"
    />
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type { TokenAnalysisProjectUsage } from '@/api/admin/tokenAnalysis'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatCompactNumber } from '@/utils/format'
import {
  PROJECT_OUTPUT_INPUT_HEALTHY_RATIO,
  formatInteger,
  formatTimeCN,
  percent,
  shareOfMax,
  type TokenAnalysisPagination
} from './tokenAnalysisFormat'
import './tokenAnalysis.css'

export type TokenAnalysisProjectSortField = 'request_count' | 'total_tokens' | 'last_event_time'

const props = defineProps<{
  projects: TokenAnalysisProjectUsage[]
  pagination: TokenAnalysisPagination
  loading: boolean
  sortBy: TokenAnalysisProjectSortField
  sortOrder: 'asc' | 'desc'
  activeProject?: string
}>()

const emit = defineEmits<{
  sort: [field: TokenAnalysisProjectSortField]
  'filter-project': [project: string]
  page: [page: number]
  'page-size': [pageSize: number]
}>()

const { t } = useI18n()

const share = computed(() => shareOfMax(props.projects.map((row) => row.total_tokens)))

function sortIcon(field: TokenAnalysisProjectSortField) {
  if (props.sortBy !== field) return 'arrowsUpDown' as const
  return props.sortOrder === 'desc' ? ('arrowDown' as const) : ('arrowUp' as const)
}

function ratio(row: TokenAnalysisProjectUsage): number | null {
  if (!row.input_tokens || row.input_tokens <= 0) return null
  return (row.output_tokens || 0) / row.input_tokens
}

function formatRatio(row: TokenAnalysisProjectUsage): string {
  const value = ratio(row)
  return value === null ? '-' : percent(value)
}

function ratioClass(row: TokenAnalysisProjectUsage): string {
  const value = ratio(row)
  if (value === null) return 'text-gray-400'
  if (value >= PROJECT_OUTPUT_INPUT_HEALTHY_RATIO) return 'text-emerald-600 dark:text-emerald-400'
  return 'text-red-600 dark:text-red-400'
}

function ratioDataTest(row: TokenAnalysisProjectUsage): string {
  const value = ratio(row)
  if (value === null) return 'project-io-ratio-empty'
  return value >= PROJECT_OUTPUT_INPUT_HEALTHY_RATIO ? 'project-io-ratio-healthy' : 'project-io-ratio-low'
}

function ratioTitle(row: TokenAnalysisProjectUsage): string {
  const value = ratio(row)
  if (value === null) return t('admin.tokenAnalysis.outputInputRatioUnavailable')
  return `${t('admin.tokenAnalysis.outputInputRatio')}: ${percent(value)} (${t('admin.tokenAnalysis.outputInputRatioThreshold')}: ${percent(PROJECT_OUTPUT_INPUT_HEALTHY_RATIO)})`
}
</script>
