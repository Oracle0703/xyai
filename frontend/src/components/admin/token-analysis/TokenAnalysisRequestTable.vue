<template>
  <section class="card overflow-hidden" data-testid="token-analysis-request-details">
    <div class="ta-card-header">
      <div class="flex items-center gap-2">
        <h3 class="ta-card-title">{{ t('admin.tokenAnalysis.requestDetails') }}</h3>
        <span class="ta-count">{{ formatInteger(pagination.total) }}</span>
        <span class="hidden text-xs text-gray-500 dark:text-dark-400 md:inline">{{ t('admin.tokenAnalysis.requestDetailsHint') }}</span>
      </div>
      <OrganizationUsageSegmented
        :model-value="sort"
        :options="sortOptions"
        testid-prefix="token-analysis-sort-"
        :aria-label="t('admin.tokenAnalysis.requestDetails')"
        @update:model-value="emit('sort', $event)"
      />
    </div>
    <p class="border-b border-gray-100 px-4 pb-3 text-xs text-gray-500 dark:border-dark-700 dark:text-dark-400">{{ t('admin.tokenAnalysis.requestOrigin.hint') }}</p>
    <div class="overflow-x-auto transition-opacity" :class="{ 'opacity-60': loading }">
      <table class="ta-table">
        <thead>
          <tr>
            <th>{{ t('admin.tokenAnalysis.time') }}</th>
            <th>{{ t('admin.tokenAnalysis.user') }}</th>
            <th>{{ t('admin.tokenAnalysis.requestOrigin.label') }}</th>
            <th>{{ t('admin.tokenAnalysis.model') }}</th>
            <th>{{ t('admin.tokenAnalysis.project') }}</th>
            <th class="ta-num">{{ t('admin.tokenAnalysis.usage') }}</th>
            <th>{{ t('admin.tokenAnalysis.risk') }}</th>
            <th>{{ t('admin.tokenAnalysis.preview') }}</th>
            <th class="text-center">{{ t('admin.tokenAnalysis.quality') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="{ item, origin, link } in requestRows"
            :key="item.id"
            class="cursor-pointer"
            :class="selectedId === item.id ? 'bg-primary-50/60 dark:bg-primary-900/10' : ''"
            tabindex="0"
            @click="emit('select', item)"
            @keydown.enter="emit('select', item)"
          >
            <td class="whitespace-nowrap text-xs text-gray-600 dark:text-gray-300">{{ formatTimeCN(item.event_time) }}</td>
            <td>
              <div class="max-w-[180px] truncate font-medium" :title="item.user_email">{{ item.user_email || '-' }}</div>
              <div class="ta-sub max-w-[180px] truncate">{{ item.api_key_name || '-' }}</div>
            </td>
            <td>
              <span class="ta-tag whitespace-nowrap" :class="origin.className" :title="origin.tooltip" data-testid="request-origin">{{ origin.label }}</span>
            </td>
            <td>
              <div class="max-w-[180px] truncate" :title="item.model">{{ item.model }}</div>
              <div class="ta-sub max-w-[180px] truncate">{{ item.endpoint }}</div>
            </td>
            <td>
              <div class="max-w-[160px] truncate" :class="item.client_project ? '' : 'italic text-gray-400 dark:text-dark-400'">
                {{ item.client_project || t('admin.tokenAnalysis.unattributed') }}
              </div>
              <div class="ta-sub max-w-[160px] truncate">{{ item.client_branch || '-' }}</div>
            </td>
            <td class="ta-num">
              <template v-if="link.linked">
                <div class="font-medium" :title="formatInteger(totalTokens(item))">{{ formatTokenMetric(totalTokens(item)) }}</div>
                <div class="ta-sub">{{ formatCost(item.actual_cost) }}</div>
              </template>
              <template v-else>
                <div class="font-medium text-gray-400 dark:text-dark-400" :title="link.tooltip">—</div>
                <span class="ta-tag mt-0.5 whitespace-nowrap" :class="link.className" :title="link.tooltip" data-testid="request-usage-link">{{ link.label }}</span>
              </template>
            </td>
            <td>
              <div v-if="item.risk_score > 0 || (item.risk_reasons || []).length" class="flex max-w-[220px] flex-wrap items-center gap-1">
                <span v-if="item.risk_score > 0" class="inline-flex min-w-8 justify-center rounded-full px-1.5 py-0.5 text-xs font-semibold" :class="riskScoreClass(item.risk_score)">
                  {{ item.risk_score }}
                </span>
                <span
                  v-for="reason in item.risk_reasons || []"
                  :key="reason.code"
                  class="ta-tag cursor-help bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300"
                  :title="riskDesc(reason.code) || reason.message || reason.code"
                >
                  {{ riskLabel(reason.code) }}
                </span>
              </div>
              <span v-else class="text-xs text-gray-300 dark:text-dark-600">-</span>
            </td>
            <td>
              <div class="max-w-[360px] truncate text-gray-700 dark:text-gray-200" :title="item.last_user_preview">{{ item.last_user_preview || '-' }}</div>
              <div v-if="(item.duplicate_count ?? 0) > 1 || item.request_body_truncated" class="mt-1 flex flex-wrap items-center gap-1">
                <span
                  v-if="(item.duplicate_count ?? 0) > 1"
                  class="ta-tag rounded-full bg-purple-100 font-semibold text-purple-700 dark:bg-purple-900/30 dark:text-purple-300"
                  :title="t('admin.tokenAnalysis.duplicateHint')"
                >
                  ×{{ item.duplicate_count }}
                </span>
                <span
                  v-if="item.request_body_truncated"
                  class="ta-tag rounded-full bg-amber-100 font-semibold text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
                  :title="t('admin.tokenAnalysis.bodyTruncatedHint')"
                >
                  {{ t('admin.tokenAnalysis.bodyTruncated') }}
                </span>
              </div>
            </td>
            <td class="text-center">
              <span v-if="item.quality_score !== undefined && item.quality_score !== null" class="inline-flex min-w-10 justify-center rounded-full bg-blue-100 px-2 py-0.5 text-xs font-semibold text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">
                {{ item.quality_score }}
              </span>
              <span v-else-if="item.has_input" class="whitespace-nowrap text-xs text-gray-400">{{ t('admin.tokenAnalysis.qualityNotEvaluated') }}</span>
              <span v-else class="text-xs text-gray-300 dark:text-dark-600">-</span>
            </td>
          </tr>
          <tr v-if="!loading && requests.length === 0">
            <td colspan="9" class="ta-empty">{{ t('common.noData') }}</td>
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

import type { TokenAnalysisRequestItem } from '@/api/admin/tokenAnalysis'
import OrganizationUsageSegmented from '@/components/admin/organization-usage/OrganizationUsageSegmented.vue'
import Pagination from '@/components/common/Pagination.vue'
import { useRequestOrigin } from './requestOrigin'
import { useUsageLink } from './usageLink'
import {
  formatCost,
  formatInteger,
  formatTimeCN,
  formatTokenMetric,
  riskScoreClass,
  useTokenAnalysisRisk,
  type TokenAnalysisPagination
} from './tokenAnalysisFormat'
import './tokenAnalysis.css'

export type TokenAnalysisRequestSort = 'event_time' | 'risk_score'

const props = defineProps<{
  requests: TokenAnalysisRequestItem[]
  pagination: TokenAnalysisPagination
  loading: boolean
  sort: TokenAnalysisRequestSort
  selectedId?: number
}>()

const emit = defineEmits<{
  sort: [value: TokenAnalysisRequestSort]
  select: [item: TokenAnalysisRequestItem]
  page: [page: number]
  'page-size': [pageSize: number]
}>()

const { t } = useI18n()
const { riskLabel, riskDesc } = useTokenAnalysisRisk()
const { requestOrigin } = useRequestOrigin()
const { usageLink } = useUsageLink()
const requestRows = computed(() => props.requests.map((item) => ({ item, origin: requestOrigin(item.summary_json), link: usageLink(item) })))

const sortOptions = computed(() => [
  { value: 'event_time' as const, label: t('admin.tokenAnalysis.sortByTime') },
  { value: 'risk_score' as const, label: t('admin.tokenAnalysis.sortByRisk') }
])

function totalTokens(item: TokenAnalysisRequestItem): number {
  return (item.input_tokens || 0) + (item.output_tokens || 0) + (item.cache_read_tokens || 0) + (item.cache_creation_tokens || 0)
}
</script>
