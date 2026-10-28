<template>
  <section class="card flex min-w-0 flex-col overflow-hidden" data-testid="token-analysis-user-ranking">
    <div class="ta-card-header">
      <div class="flex items-center gap-2">
        <h3 class="ta-card-title">{{ t('admin.tokenAnalysis.userRanking') }}</h3>
        <span class="ta-count">{{ formatInteger(pagination.total) }}</span>
      </div>
      <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.tokenAnalysis.userRankingHint') }}</span>
    </div>
    <div class="flex-1 overflow-x-auto transition-opacity" :class="{ 'opacity-60': loading }">
      <table class="ta-table">
        <thead>
          <tr>
            <th class="w-10 !pr-0"><span class="sr-only">{{ t('admin.tokenAnalysis.usageTrend') }}</span></th>
            <th class="w-12">#</th>
            <th>{{ t('admin.tokenAnalysis.user') }}</th>
            <th class="ta-num">{{ t('admin.tokenAnalysis.requests') }}</th>
            <th class="ta-num">{{ t('admin.tokenAnalysis.tokens') }}</th>
            <th class="ta-num">{{ t('admin.tokenAnalysis.cost') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="(user, index) in users"
            :key="user.user_id || `unknown-${index}`"
            :class="slotOf(user.user_id) !== null ? 'bg-primary-50/60 dark:bg-primary-900/10' : ''"
          >
            <td class="!pr-0">
              <input
                v-if="user.user_id"
                data-user-trend-select
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 disabled:cursor-not-allowed disabled:opacity-40"
                :checked="slotOf(user.user_id) !== null"
                :disabled="selectionDisabled(user.user_id)"
                :title="selectionDisabled(user.user_id) ? t('admin.tokenAnalysis.selectLimitReached', { max: MAX_SELECTED_TREND_USERS }) : undefined"
                :aria-label="t('admin.tokenAnalysis.selectUserTrend', { email: user.user_email || user.user_id })"
                @change="emit('toggle', user)"
              />
              <span v-else class="text-gray-300 dark:text-dark-600">-</span>
            </td>
            <td class="tabular-nums text-gray-500 dark:text-dark-400">{{ rank(index) }}</td>
            <td>
              <div class="flex min-w-0 items-center gap-2">
                <span
                  v-if="slotOf(user.user_id) !== null"
                  class="h-2 w-2 shrink-0 rounded-full"
                  :class="USER_TREND_DOT_CLASSES[slotOf(user.user_id)!]"
                />
                <span class="max-w-[240px] truncate font-medium" :title="user.user_email">{{ user.user_email || '-' }}</span>
              </div>
            </td>
            <td class="ta-num text-gray-600 dark:text-gray-300">
              <div :title="formatInteger(user.request_count)">{{ formatRequestMetric(user.request_count) }}</div>
              <div class="ta-sub" :title="t('admin.tokenAnalysis.help.humanInputs')">{{ t('admin.tokenAnalysis.humanInputs') }} {{ formatInteger(user.human_input_count) }}</div>
            </td>
            <td class="ta-num">
              <span class="font-medium" :title="formatInteger(user.total_tokens)">{{ formatTokenMetric(user.total_tokens) }}</span>
              <div class="ml-auto mt-1 h-1 w-16 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                <div class="h-full rounded-full bg-primary-500" :style="{ width: `${share(user.total_tokens) * 100}%` }" />
              </div>
            </td>
            <td class="ta-num text-gray-600 dark:text-gray-300" :title="formatCost(user.actual_cost)">{{ formatCompactCost(user.actual_cost) }}</td>
          </tr>
          <tr v-if="!loading && users.length === 0">
            <td colspan="6" class="ta-empty">{{ t('common.noData') }}</td>
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

import type { TokenAnalysisUserUsage } from '@/api/admin/tokenAnalysis'
import Pagination from '@/components/common/Pagination.vue'
import {
  MAX_SELECTED_TREND_USERS,
  USER_TREND_DOT_CLASSES,
  formatCompactCost,
  formatCost,
  formatInteger,
  formatRequestMetric,
  formatTokenMetric,
  shareOfMax,
  type TokenAnalysisPagination,
  type TokenAnalysisSelectedUser
} from './tokenAnalysisFormat'
import './tokenAnalysis.css'

const props = defineProps<{
  users: TokenAnalysisUserUsage[]
  pagination: TokenAnalysisPagination
  loading: boolean
  selectedUsers: TokenAnalysisSelectedUser[]
}>()

const emit = defineEmits<{
  toggle: [user: TokenAnalysisUserUsage]
  page: [page: number]
  'page-size': [pageSize: number]
}>()

const { t } = useI18n()

const share = computed(() => shareOfMax(props.users.map((user) => user.total_tokens)))

function rank(index: number): number {
  return (props.pagination.page - 1) * props.pagination.page_size + index + 1
}

function slotOf(userID?: number): number | null {
  if (!userID) return null
  return props.selectedUsers.find((user) => user.user_id === userID)?.slot ?? null
}

// 达到上限后只禁用未选项, 已选项仍可取消。
function selectionDisabled(userID?: number): boolean {
  if (!userID) return true
  return props.selectedUsers.length >= MAX_SELECTED_TREND_USERS && slotOf(userID) === null
}
</script>
