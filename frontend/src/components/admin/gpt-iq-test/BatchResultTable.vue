<template>
  <div class="overflow-x-auto" data-testid="gpt-iq-batch-table">
    <p v-if="!entryIds.length || !caseIds.length" class="px-6 py-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('gptIQTest.admin.results.empty') }}</p>
    <table v-else class="w-full text-left text-sm">
      <thead class="bg-gray-50/80 text-xs text-gray-500 dark:bg-dark-900/40 dark:text-gray-400">
        <tr>
          <th class="sticky left-0 z-[1] bg-gray-50 px-4 py-2.5 font-medium dark:bg-dark-900">{{ t('gptIQTest.admin.results.account') }}</th>
          <th v-for="caseId in caseIds" :key="caseId" class="min-w-[15rem] px-3 py-2.5 font-medium">
            <span class="text-gray-700 dark:text-gray-200">{{ caseName(caseId) }}</span>
            <span v-if="caseKind(caseId)" class="ml-1.5 text-gray-400">{{ t(`gptIQTest.kinds.${caseKind(caseId)}`) }}</span>
          </th>
        </tr>
      </thead>
      <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
        <tr v-for="row in grid" :key="row.entryId" class="align-top" data-testid="gpt-iq-batch-row">
          <td class="sticky left-0 z-[1] bg-white px-4 py-3 dark:bg-dark-800">
            <div class="flex items-center gap-2">
              <span v-if="row.group" class="h-2 w-2 shrink-0 rounded-full" :class="row.group === 'xunyou' ? 'bg-sky-500 dark:bg-sky-600' : 'bg-violet-500'" />
              <span class="whitespace-nowrap font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
            </div>
            <span v-if="row.group" class="badge badge-gray mt-1">{{ t(`gptQuota.groups.${row.group}`) }}</span>
          </td>
          <td v-for="cell in row.cells" :key="cell.caseId" class="px-3 py-3">
            <p v-if="cell.summary" class="mb-1.5 text-xs font-semibold tabular-nums text-gray-700 dark:text-gray-200" data-testid="gpt-iq-batch-case-summary">
              {{ t('gptIQTest.admin.results.summary', cell.summary) }}
            </p>
            <div class="space-y-1.5">
              <div
                v-for="run in cell.runs"
                :key="run.index"
                class="rounded-md border px-2 py-1.5 text-[11px]"
                :class="cellClass(run.result)"
                data-testid="gpt-iq-result-cell"
                :data-status="run.result?.status ?? 'pending'"
              >
                <div class="flex items-center justify-between gap-2">
                  <span class="font-medium tabular-nums text-gray-500 dark:text-gray-400">#{{ run.index }}</span>
                  <span class="inline-flex items-center gap-1">
                    <span
                      v-if="run.result && run.result.correct !== null"
                      class="inline-flex h-4 w-4 items-center justify-center rounded text-[10px] font-bold"
                      :class="GPT_IQ_RUN_MARK_STYLES[gptIQRunMark(run.result)].className"
                    >{{ GPT_IQ_RUN_MARK_STYLES[gptIQRunMark(run.result)].symbol }}</span>
                    <Icon v-if="run.result?.status === 'running'" name="refresh" size="xs" class="animate-spin text-primary-500" />
                    <span class="font-medium">{{ run.result ? gptIQRunStatusLabel(run.result.status, t, te) : t('gptIQTest.runStatus.pending') }}</span>
                  </span>
                </div>
                <template v-if="run.result">
                  <!-- 回答按纯文本展示（pre-wrap），不得使用 v-html。 -->
                  <p
                    v-if="run.result.answer_text"
                    class="mt-0.5 whitespace-pre-wrap break-words text-gray-700 dark:text-gray-200"
                    :class="isExpanded(run.result.id) ? '' : 'line-clamp-3'"
                    data-testid="gpt-iq-result-answer"
                  >{{ run.result.answer_text }}</p>
                  <button
                    v-if="run.result.answer_text.length > 120"
                    type="button"
                    class="text-[11px] text-primary-600 hover:underline dark:text-primary-400"
                    @click="toggleExpanded(run.result.id)"
                  >
                    {{ isExpanded(run.result.id) ? t('gptIQTest.drawer.collapse') : t('gptIQTest.drawer.expand') }}
                  </button>
                  <span v-if="run.result.answer_truncated" class="ml-1 text-[11px] text-amber-600 dark:text-amber-400">{{ t('gptIQTest.admin.results.truncated') }}</span>
                  <p v-if="hasMetrics(run.result)" class="mt-0.5 tabular-nums text-gray-500 dark:text-gray-400">
                    {{ t('gptIQTest.admin.results.input') }} {{ formatGPTIQTokens(run.result.input_tokens) }}
                    · {{ t('gptIQTest.admin.results.output') }} {{ formatGPTIQTokens(run.result.output_tokens) }}
                    · {{ t('gptIQTest.admin.results.reasoning') }} {{ formatGPTIQTokens(run.result.reasoning_tokens) }}
                  </p>
                  <p v-if="hasMetrics(run.result)" class="tabular-nums text-gray-500 dark:text-gray-400">
                    {{ formatGPTIQSeconds(run.result.duration_ms) }}s · {{ t('gptIQTest.avgTps', { value: formatGPTIQTps(run.result.tps) }) }}
                    <template v-if="run.result.ttft_ms !== null"> · {{ t('gptIQTest.admin.results.ttft', { value: formatGPTIQSeconds(run.result.ttft_ms) }) }}</template>
                  </p>
                  <p v-if="run.result.attempts > 1" class="text-gray-500 dark:text-gray-400">{{ t('gptIQTest.admin.results.attempts', { count: run.result.attempts }) }}</p>
                  <p v-if="run.result.model_responded && run.result.model_responded !== run.result.model_requested" class="break-all text-gray-500 dark:text-gray-400">
                    {{ t('gptIQTest.admin.results.responded', { model: run.result.model_responded }) }}
                  </p>
                  <p v-if="run.result.reason" class="mt-0.5" :class="gptIQRunNoteClass(run.result.status)">
                    {{ gptIQRunReasonLabel(run.result.reason, t, te) }}<template v-if="run.result.http_status"> · {{ t('gptIQTest.admin.results.http', { status: run.result.http_status }) }}</template>
                  </p>
                  <p v-if="run.result.error_summary" class="mt-0.5 whitespace-pre-wrap break-all text-red-600 dark:text-red-400" data-testid="gpt-iq-result-error">{{ run.result.error_summary }}</p>
                </template>
              </div>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { GPTIQAdminCase, GPTIQAdminEntry, GPTIQBatchDetail, GPTIQBatchResult } from '@/api/admin/gptIQTest'
import {
  GPT_IQ_RUN_MARK_STYLES,
  formatGPTIQSeconds,
  formatGPTIQTokens,
  formatGPTIQTps,
  gptIQRunMark,
  gptIQRunNoteClass,
  gptIQRunReasonLabel,
  gptIQRunStatusLabel,
} from '@/utils/gptIQTest'

const props = defineProps<{
  detail: GPTIQBatchDetail
  entries: GPTIQAdminEntry[]
  cases: GPTIQAdminCase[]
}>()

const { t, te } = useI18n()

const expanded = ref(new Set<number>())

function uniqueInOrder(values: number[]): number[] {
  return [...new Set(values)]
}

// 批次创建时已把"全部"解析成具体条目与测试项；旧数据缺失时退回结果中出现过的 ID。
const entryIds = computed(() => {
  const listed = props.detail.batch.entry_ids ?? []
  return uniqueInOrder([...listed, ...props.detail.results.map((result) => result.entry_id)])
})
const caseIds = computed(() => {
  const listed = props.detail.batch.case_ids ?? []
  return uniqueInOrder([...listed, ...props.detail.results.map((result) => result.case_id)])
})

const entryById = computed(() => new Map(props.entries.map((entry) => [entry.id, entry])))
const caseById = computed(() => new Map(props.cases.map((item) => [item.id, item])))
const resultByKey = computed(() => {
  const map = new Map<string, GPTIQBatchResult>()
  for (const result of props.detail.results) map.set(`${result.entry_id}-${result.case_id}-${result.run_index}`, result)
  return map
})
const firstResultByEntry = computed(() => {
  const map = new Map<number, GPTIQBatchResult>()
  for (const result of props.detail.results) if (!map.has(result.entry_id)) map.set(result.entry_id, result)
  return map
})

// 表格：账号 × 测试项，每格按第 i 次调用逐条展示；尚未派发的调用显示"等待中"。
const grid = computed(() =>
  entryIds.value.map((entryId) => ({
    entryId,
    name: entryName(entryId),
    group: entryGroup(entryId),
    cells: caseIds.value.map((caseId) => ({
      caseId,
      summary: caseSummary(entryId, caseId),
      runs: Array.from({ length: runCount(caseId) }, (_, offset) => ({ index: offset + 1, result: resultAt(entryId, caseId, offset + 1) })),
    })),
  })),
)

function resultAt(entryId: number, caseId: number, index: number) {
  return resultByKey.value.get(`${entryId}-${caseId}-${index}`)
}

function runCount(caseId: number) {
  const configured = caseById.value.get(caseId)?.runs ?? 1
  const seen = props.detail.results.filter((result) => result.case_id === caseId).reduce((max, result) => Math.max(max, result.run_index), 0)
  return Math.max(configured, seen, 1)
}

function entryName(entryId: number) {
  return firstResultByEntry.value.get(entryId)?.entry_name || entryById.value.get(entryId)?.display_name || `#${entryId}`
}

function entryGroup(entryId: number) {
  return firstResultByEntry.value.get(entryId)?.group || entryById.value.get(entryId)?.group || ''
}

function caseName(caseId: number) {
  return caseById.value.get(caseId)?.name ?? t('gptIQTest.unknownCase', { id: caseId })
}

function caseKind(caseId: number) {
  return caseById.value.get(caseId)?.kind ?? ''
}

function caseSummary(entryId: number, caseId: number) {
  const results = props.detail.results.filter((result) => result.entry_id === entryId && result.case_id === caseId)
  const graded = results.filter((result) => result.correct !== null)
  if (!graded.length) return null
  return { correct: graded.filter((result) => result.correct === true).length, graded: graded.length }
}

function hasMetrics(result: GPTIQBatchResult) {
  return result.duration_ms !== null || result.input_tokens !== null || result.output_tokens !== null
}

function cellClass(result: GPTIQBatchResult | undefined) {
  if (!result) return 'border-dashed border-gray-200 text-gray-400 dark:border-dark-600'
  switch (result.status) {
    case 'running':
      return 'border-primary-200 bg-primary-50/50 dark:border-primary-800 dark:bg-primary-900/10'
    case 'ok':
      if (result.correct === false) return 'border-red-200 bg-red-50/50 dark:border-red-900/60 dark:bg-red-950/10'
      return 'border-emerald-200 bg-emerald-50/40 dark:border-emerald-900/60 dark:bg-emerald-950/10'
    case 'error':
      return 'border-orange-200 bg-orange-50/50 dark:border-orange-900/60 dark:bg-orange-950/10'
    default:
      return 'border-gray-200 bg-gray-50/60 text-gray-500 dark:border-dark-600 dark:bg-dark-800/40'
  }
}

function isExpanded(id: number) {
  return expanded.value.has(id)
}

function toggleExpanded(id: number) {
  const next = new Set(expanded.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expanded.value = next
}
</script>
