<template>
  <div class="space-y-4" data-testid="token-analysis-index-panel">
    <section class="card p-4">
      <div class="grid grid-cols-2 gap-4 md:grid-cols-4">
        <div class="min-w-0">
          <div class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('admin.tokenAnalysis.indexStatus') }}</div>
          <div class="mt-1 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
            <span class="h-2 w-2 shrink-0 rounded-full" :class="TOKEN_ANALYSIS_INDEX_DOT[state]" />
            <span class="truncate">{{ statusLabel }}</span>
          </div>
        </div>
        <div class="min-w-0">
          <div class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('admin.tokenAnalysis.updated') }}</div>
          <div class="mt-1 truncate text-sm font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatTimeCN(indexStatus?.updated_at) }}</div>
        </div>
        <div class="min-w-0">
          <div class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('admin.tokenAnalysis.processed') }}</div>
          <div class="mt-1 text-sm font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatInteger(indexStatus?.processed_rows) }}</div>
        </div>
        <div class="min-w-0">
          <div class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('admin.tokenAnalysis.failed') }}</div>
          <div
            class="mt-1 text-sm font-semibold tabular-nums"
            :class="(indexStatus?.failed_rows ?? 0) > 0 ? 'text-red-600 dark:text-red-400' : 'text-gray-900 dark:text-white'"
          >
            {{ formatInteger(indexStatus?.failed_rows) }}
          </div>
        </div>
      </div>
      <div
        v-if="indexStatus?.last_error"
        class="mt-4 flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-900/50 dark:bg-red-900/10 dark:text-red-300"
        role="alert"
      >
        <Icon name="exclamationCircle" size="sm" class="mt-px shrink-0" />
        <span class="break-all">{{ indexStatus.last_error }}</span>
      </div>
    </section>

    <section class="card overflow-hidden">
      <div class="ta-card-header">
        <div class="flex items-center gap-2">
          <h3 class="ta-card-title">{{ t('admin.tokenAnalysis.archiveFiles') }}</h3>
          <span class="ta-count">{{ archiveFiles.length }}</span>
        </div>
      </div>
      <p class="border-b border-gray-100 px-4 py-2.5 text-xs leading-relaxed text-gray-500 dark:border-dark-700 dark:text-dark-400">
        {{ t('admin.tokenAnalysis.archiveFilesHint') }}
      </p>
      <div class="overflow-x-auto">
        <table class="ta-table">
          <thead>
            <tr>
              <th>{{ t('admin.tokenAnalysis.file') }}</th>
              <th class="ta-num">{{ t('admin.tokenAnalysis.fileSize') }}</th>
              <th>{{ t('admin.tokenAnalysis.indexProgress') }}</th>
              <th class="ta-num">{{ t('admin.tokenAnalysis.processed') }} / {{ t('admin.tokenAnalysis.failed') }}</th>
              <th>{{ t('admin.tokenAnalysis.fileStatus') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="file in archiveFiles" :key="file.name">
              <td class="whitespace-nowrap font-mono text-xs font-medium">{{ file.name }}</td>
              <td class="ta-num text-xs">{{ formatBytes(file.size_bytes) }}</td>
              <td class="min-w-[180px]" :title="progressDetail(file)">
                <div class="text-xs tabular-nums text-gray-700 dark:text-gray-200">{{ progressText(file) }}</div>
                <div v-if="file.status !== 'compressed' && file.size_bytes > 0" class="mt-1 h-1.5 w-full max-w-[160px] overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                  <div class="h-full rounded-full" :class="fullyRead(file) ? 'bg-emerald-500' : 'bg-blue-500'" :style="{ width: `${progressRatio(file) * 100}%` }" />
                </div>
              </td>
              <td class="ta-num text-xs">
                {{ formatInteger(file.processed_rows) }} /
                <span :class="file.failed_rows > 0 ? 'font-semibold text-red-600 dark:text-red-400' : ''">{{ formatInteger(file.failed_rows) }}</span>
                <div v-if="fullyRead(file)" class="ta-sub">
                  {{ t('admin.tokenAnalysis.requestRowsTotal', { n: formatInteger(file.processed_rows + file.failed_rows) }) }}
                </div>
              </td>
              <td>
                <span
                  class="inline-flex rounded-full px-2 py-0.5 text-xs font-semibold"
                  :class="statusClass(file.status)"
                  :title="t(`admin.tokenAnalysis.archiveFileStatusHint.${file.status}`)"
                >
                  {{ t(`admin.tokenAnalysis.archiveFileStatus.${file.status}`) }}
                </span>
                <div v-if="file.last_error" class="mt-1 max-w-[280px] truncate text-xs text-red-500" :title="file.last_error">{{ file.last_error }}</div>
              </td>
            </tr>
            <tr v-if="archiveFiles.length === 0">
              <td colspan="5" class="ta-empty">{{ t('common.noData') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <section v-if="indexStatus?.files?.length" class="card overflow-hidden">
      <div class="ta-card-header">
        <h3 class="ta-card-title">{{ t('admin.tokenAnalysis.recentIndexFiles') }}</h3>
      </div>
      <div class="overflow-x-auto">
        <table class="ta-table">
          <thead>
            <tr>
              <th>{{ t('admin.tokenAnalysis.file') }}</th>
              <th class="ta-num">{{ t('admin.tokenAnalysis.offset') }}</th>
              <th class="ta-num">{{ t('admin.tokenAnalysis.processed') }}</th>
              <th class="ta-num">{{ t('admin.tokenAnalysis.failed') }}</th>
              <th>{{ t('admin.tokenAnalysis.updated') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="file in indexStatus.files.slice(0, 5)" :key="file.source_file">
              <td class="max-w-[360px] truncate font-mono text-xs" :title="file.source_file">{{ file.source_file }}</td>
              <td class="ta-num text-xs">{{ formatInteger(file.last_offset) }}</td>
              <td class="ta-num text-xs">{{ formatInteger(file.processed_rows) }}</td>
              <td class="ta-num text-xs" :class="file.failed_rows > 0 ? 'font-semibold text-red-600 dark:text-red-400' : ''">{{ formatInteger(file.failed_rows) }}</td>
              <td class="whitespace-nowrap text-xs text-gray-500 dark:text-dark-400">{{ formatTimeCN(file.updated_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type {
  TokenAnalysisArchiveFile,
  TokenAnalysisArchiveFileStatus,
  TokenAnalysisIndexStatus
} from '@/api/admin/tokenAnalysis'
import Icon from '@/components/icons/Icon.vue'
import { formatBytes } from '@/utils/format'
import { TOKEN_ANALYSIS_INDEX_DOT, formatInteger, formatTimeCN, tokenAnalysisIndexState } from './tokenAnalysisFormat'
import './tokenAnalysis.css'

const props = defineProps<{
  indexStatus: TokenAnalysisIndexStatus | null
  archiveFiles: TokenAnalysisArchiveFile[]
}>()

const { t } = useI18n()

const state = computed(() => tokenAnalysisIndexState(props.indexStatus))
const statusLabel = computed(() => ({
  running: t('admin.tokenAnalysis.indexRunning'),
  error: t('admin.tokenAnalysis.indexHasError'),
  idle: t('admin.tokenAnalysis.indexIdle')
})[state.value])

// 文件是否已被索引器读完(字节水位追平文件大小)。"有失败行"状态也意味着
// 已读完 -- 只是其中部分行未入库, 这层语义靠进度列和状态提示显式说出来。
function fullyRead(file: TokenAnalysisArchiveFile): boolean {
  return file.status !== 'compressed' && file.size_bytes > 0 && file.indexed_offset >= file.size_bytes
}

function progressRatio(file: TokenAnalysisArchiveFile): number {
  if (file.size_bytes <= 0) return 0
  return Math.min(Math.max(file.indexed_offset / file.size_bytes, 0), 1)
}

// 进度展示为百分比, 直观回答"处理完没有"; 字节明细放 title 悬浮提示。
function progressText(file: TokenAnalysisArchiveFile): string {
  if (file.status === 'compressed' || file.size_bytes <= 0) return '-'
  if (fullyRead(file)) return `100% · ${t('admin.tokenAnalysis.fullyRead')}`
  return `${(progressRatio(file) * 100).toFixed(1)}%`
}

function progressDetail(file: TokenAnalysisArchiveFile): string {
  return `${formatBytes(file.indexed_offset)} / ${formatBytes(file.size_bytes)}`
}

function statusClass(status: TokenAnalysisArchiveFileStatus): string {
  switch (status) {
    case 'deletable':
      return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
    case 'writing':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
    case 'indexing':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    case 'attention':
      return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  }
}
</script>
