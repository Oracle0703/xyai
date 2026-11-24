<template>
  <Teleport to="body">
    <template v-if="card">
      <div class="fixed inset-0 z-40 bg-gray-900/30 backdrop-blur-[1px]" @click="emit('close')"></div>
      <aside
        class="fixed right-0 top-0 z-50 flex h-full w-full max-w-3xl flex-col border-l border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-900"
        role="dialog"
        aria-modal="true"
        :aria-label="t('gptIQTest.drawer.title', { name: card.display_name })"
        data-testid="gpt-iq-drawer"
      >
        <header class="flex items-start justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <div class="min-w-0">
            <h2 class="truncate text-base font-semibold text-gray-900 dark:text-white">{{ t('gptIQTest.drawer.title', { name: card.display_name }) }}</h2>
            <div class="mt-1.5 flex flex-wrap items-center gap-2">
              <span class="badge" :class="gptIQVerdictStyle(card.verdict).badge">{{ gptIQVerdictLabel(card.verdict, t, te) }}</span>
              <span v-for="(text, index) in reasonTexts" :key="index" class="text-xs text-gray-500 dark:text-gray-400">{{ text }}</span>
            </div>
          </div>
          <button type="button" class="btn btn-ghost btn-sm shrink-0" :aria-label="t('gptIQTest.drawer.close')" data-testid="gpt-iq-drawer-close" @click="emit('close')">
            <Icon name="x" size="sm" />
          </button>
        </header>

        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-5 py-3 dark:border-dark-700">
          <div class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-1 dark:border-dark-600 dark:bg-dark-900/50" role="group">
            <button
              v-for="option in DAY_OPTIONS"
              :key="option"
              type="button"
              class="rounded-md px-3 py-1 text-xs transition-colors"
              :class="days === option ? segmentActiveClass : segmentIdleClass"
              :aria-pressed="days === option"
              @click="days = option"
            >
              {{ t('gptIQTest.drawer.days', { days: option }) }}
            </button>
          </div>
          <select v-model.number="caseFilter" class="input w-auto py-1.5 text-sm" :aria-label="t('gptIQTest.drawer.allCases')" data-testid="gpt-iq-drawer-case">
            <option :value="0">{{ t('gptIQTest.drawer.allCases') }}</option>
            <option v-for="item in detailCases" :key="item.id" :value="item.id">{{ item.name }}</option>
          </select>
        </div>

        <div class="flex-1 space-y-5 overflow-y-auto px-5 py-4">
          <GPTIQTrendStrip :trend="card.trend" :today="today" show-label />

          <!-- 题面取实际执行的版本（同一版本只列一次），按纯文本展示、保留换行。 -->
          <details v-for="item in promptRevisions" :key="item.id" class="rounded-lg border border-gray-100 dark:border-dark-700" data-testid="gpt-iq-prompt">
            <summary class="cursor-pointer select-none px-3 py-2 text-sm font-medium text-gray-700 dark:text-gray-200">
              {{ t('gptIQTest.drawer.prompt') }} · {{ item.name }}
              <span v-if="item.old" class="badge badge-gray ml-1 px-1.5 py-0 text-[10px]">{{ t('gptIQTest.versionLabel', { revision: item.revision }) }}</span>
            </summary>
            <pre class="max-h-72 overflow-auto whitespace-pre-wrap break-words border-t border-gray-100 px-3 py-2 font-sans text-xs text-gray-700 dark:border-dark-700 dark:text-gray-300">{{ item.prompt }}</pre>
          </details>

          <div v-if="loading && !detail" class="space-y-3" aria-hidden="true">
            <div v-for="index in 3" :key="index" class="h-24 animate-pulse rounded-lg bg-gray-50 dark:bg-dark-800" />
          </div>
          <div v-else-if="loadError && !detail" class="rounded-lg bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
            {{ t('gptIQTest.drawer.loadFailed') }}
            <button type="button" class="btn btn-secondary btn-sm ml-2" @click="load">{{ t('gptIQTest.drawer.retry') }}</button>
          </div>
          <p v-else-if="!batchGroups.length" class="py-10 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('gptIQTest.drawer.empty') }}</p>

          <section v-for="group in batchGroups" :key="group.batch_id" class="space-y-3" data-testid="gpt-iq-drawer-batch">
            <h3 class="flex flex-wrap items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
              <span class="tabular-nums">{{ group.run_date }}</span>
              <span class="badge" :class="group.trigger === 'manual' ? 'badge-primary' : 'badge-gray'">{{ t(`gptIQTest.triggers.${group.trigger}`) }}</span>
              <span class="text-xs font-normal tabular-nums text-gray-500 dark:text-gray-400">{{ formatDateTimeToMinute(group.tested_at) }}</span>
            </h3>

            <div v-for="set in group.sets" :key="`${group.batch_id}-${set.case_id}`" class="overflow-hidden rounded-lg border border-gray-100 dark:border-dark-700">
              <div class="flex flex-wrap items-center justify-between gap-2 bg-gray-50/80 px-3 py-2 text-xs dark:bg-dark-800/60">
                <span class="flex items-center gap-1.5">
                  <span class="font-medium text-gray-800 dark:text-gray-100">{{ caseName(set.case_id) }}</span>
                  <span v-if="gptIQIsOldRevision(set, currentRevisions)" class="badge badge-gray px-1.5 py-0 text-[10px]" data-testid="gpt-iq-drawer-version">
                    {{ t('gptIQTest.versionLabel', { revision: set.revision.revision }) }}
                  </span>
                </span>
                <!-- 模型、次数、思考强度取本组结果实际执行的版本。 -->
                <span v-if="set.revision" class="text-gray-500 dark:text-gray-400" data-testid="gpt-iq-drawer-meta">{{ gptIQRevisionMeta(set.revision, t) }}</span>
              </div>
              <div class="overflow-x-auto">
                <table class="w-full text-left text-xs">
                  <thead class="text-gray-500 dark:text-gray-400">
                    <tr class="border-b border-gray-100 dark:border-dark-700">
                      <th class="px-3 py-1.5 font-medium">{{ t('gptIQTest.drawer.columns.run') }}</th>
                      <th class="min-w-[12rem] px-3 py-1.5 font-medium">{{ t('gptIQTest.drawer.columns.answer') }}</th>
                      <th class="px-2 py-1.5 text-right font-medium">{{ t('gptIQTest.drawer.columns.input') }}</th>
                      <th class="px-2 py-1.5 text-right font-medium">{{ t('gptIQTest.drawer.columns.output') }}</th>
                      <th class="px-2 py-1.5 text-right font-medium" :title="t('gptIQTest.drawer.reasoningTitle')">{{ t('gptIQTest.drawer.columns.reasoning') }}</th>
                      <th class="px-2 py-1.5 text-right font-medium">{{ t('gptIQTest.drawer.columns.time') }}</th>
                      <th class="px-2 py-1.5 text-right font-medium">{{ t('gptIQTest.drawer.columns.tps') }}</th>
                      <th class="px-3 py-1.5 text-center font-medium">{{ t('gptIQTest.drawer.columns.ok') }}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <template v-for="run in set.runs" :key="run.index">
                      <tr class="border-b border-gray-50 align-top dark:border-dark-800" data-testid="gpt-iq-drawer-run">
                        <td class="px-3 py-1.5 tabular-nums text-gray-500">{{ run.index }}</td>
                        <td class="px-3 py-1.5">
                          <template v-if="run.status === 'ok'">
                            <p class="whitespace-pre-wrap break-words text-gray-700 dark:text-gray-200">{{ run.answer_preview || '—' }}</p>
                            <button
                              v-if="run.answer"
                              type="button"
                              class="mt-0.5 text-[11px] text-primary-600 hover:underline dark:text-primary-400"
                              :aria-expanded="isExpanded(set, run)"
                              data-testid="gpt-iq-expand"
                              @click="toggleExpanded(set, run)"
                            >
                              {{ isExpanded(set, run) ? t('gptIQTest.drawer.collapse') : t('gptIQTest.drawer.expand') }}
                            </button>
                          </template>
                          <p v-else :class="gptIQRunNoteClass(run.status)">{{ failureText(run) }}</p>
                        </td>
                        <td class="px-2 py-1.5 text-right tabular-nums">{{ formatGPTIQTokens(run.input_tokens) }}</td>
                        <td class="px-2 py-1.5 text-right tabular-nums">{{ formatGPTIQTokens(run.output_tokens) }}</td>
                        <td class="px-2 py-1.5 text-right tabular-nums">{{ formatGPTIQTokens(run.reasoning_tokens) }}</td>
                        <td class="px-2 py-1.5 text-right tabular-nums">{{ formatGPTIQSeconds(run.duration_ms) }}</td>
                        <td class="px-2 py-1.5 text-right tabular-nums">{{ formatGPTIQTps(run.tps) }}</td>
                        <td class="px-3 py-1.5 text-center">
                          <span class="inline-flex h-4 w-4 items-center justify-center rounded text-[10px] font-bold" :class="okMark(set, run).className">{{ okMark(set, run).symbol }}</span>
                        </td>
                      </tr>
                      <tr v-if="run.answer && isExpanded(set, run)" class="border-b border-gray-50 dark:border-dark-800">
                        <td colspan="8" class="bg-gray-50/60 px-3 py-2 dark:bg-dark-800/40">
                          <pre class="max-h-96 overflow-auto whitespace-pre-wrap break-words font-sans text-xs text-gray-700 dark:text-gray-200" data-testid="gpt-iq-full-answer">{{ run.answer }}</pre>
                          <p v-if="run.answer_truncated" class="mt-1 text-[11px] text-amber-600 dark:text-amber-400">{{ t('gptIQTest.drawer.truncated') }}</p>
                        </td>
                      </tr>
                    </template>
                  </tbody>
                  <tfoot>
                    <tr class="bg-gray-50/60 text-gray-600 dark:bg-dark-800/40 dark:text-gray-300" data-testid="gpt-iq-drawer-summary">
                      <td colspan="8" class="px-3 py-1.5">
                        <template v-if="isGraded(set)">
                          {{ t('gptIQTest.drawer.summary', { graded: set.graded, correct: set.correct, percent: formatGPTIQPercent(gptIQAccuracyPercent(set.correct, set.graded)) }) }}
                        </template>
                        <template v-else>{{ t('gptIQTest.drawer.chatSummary', { ok: set.ok, total: set.total }) }}</template>
                        <template v-if="set.interrupted > 0"> · {{ t('gptIQTest.drawer.interrupted', { count: set.interrupted }) }}</template>
                      </td>
                    </tr>
                  </tfoot>
                </table>
              </div>
            </div>
          </section>
        </div>
      </aside>
    </template>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import GPTIQTrendStrip from '@/components/user/GPTIQTrendStrip.vue'
import { getGPTIQTestEntry, type GPTIQCaseSet, type GPTIQEntryDetail, type GPTIQPublicCase, type GPTIQRun, type GPTIQUserCard } from '@/api/gptIQTest'
import { formatDateTimeToMinute } from '@/utils/format'
import {
  GPT_IQ_RUN_MARK_STYLES,
  formatGPTIQPercent,
  formatGPTIQSeconds,
  formatGPTIQTokens,
  formatGPTIQTps,
  gptIQAccuracyPercent,
  gptIQCaseNameMap,
  gptIQCurrentRevisionIds,
  gptIQIsOldRevision,
  gptIQReasonText,
  gptIQRevisionMeta,
  gptIQRunMark,
  gptIQRunNoteClass,
  gptIQRunReasonLabel,
  gptIQRunStatusLabel,
  gptIQSetKind,
  gptIQVerdictLabel,
  gptIQVerdictStyle,
  groupGPTIQSetsByBatch,
} from '@/utils/gptIQTest'

const props = withDefaults(defineProps<{
  card: GPTIQUserCard | null
  /** 列表接口返回的公开测试项，详情未加载时用于名称。 */
  cases: GPTIQPublicCase[]
  today: string
  /** 列表页按全部卡片算出的当前版本，与详情数据合并。 */
  currentRevisions?: ReadonlyMap<number, number> | null
}>(), { currentRevisions: null })

const emit = defineEmits<{ (e: 'close'): void }>()

const { t, te } = useI18n()

const DAY_OPTIONS = [7, 14, 30] as const
const segmentActiveClass = 'bg-white font-medium text-primary-700 shadow-sm dark:bg-dark-700 dark:text-primary-300'
const segmentIdleClass = 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'

const days = ref<number>(14)
const caseFilter = ref(0)
const detail = ref<GPTIQEntryDetail | null>(null)
const loading = ref(false)
const loadError = ref(false)
const expanded = ref(new Set<string>())
let controller: AbortController | null = null

const detailCases = computed(() => detail.value?.cases ?? props.cases)
const caseNames = computed(() => gptIQCaseNameMap(detailCases.value))
const reasonTexts = computed(() => (props.card ? props.card.reasons.map((reason) => gptIQReasonText(reason, caseNames.value, t, te)) : []))
const currentRevisions = computed(() => gptIQCurrentRevisionIds(detailCases.value, detail.value?.sets ?? [], props.currentRevisions ?? undefined))
const filteredSets = computed(() => (detail.value?.sets ?? []).filter((set) => !caseFilter.value || set.case_id === caseFilter.value))
const batchGroups = computed(() => groupGPTIQSetsByBatch(filteredSets.value))
// 题面来自实际执行的版本：同一版本只列一次，按结果顺序（新版本在前）；只有测试项当前公开题面时接口才返回 prompt。
const promptRevisions = computed(() => {
  const seen = new Set<number>()
  const out: Array<{ id: number; name: string; revision: number; prompt: string; old: boolean }> = []
  for (const set of filteredSets.value) {
    const revision = set.revision
    if (!revision?.prompt || seen.has(revision.id)) continue
    seen.add(revision.id)
    out.push({ id: revision.id, name: caseName(set.case_id), revision: revision.revision, prompt: revision.prompt, old: gptIQIsOldRevision(set, currentRevisions.value) })
  }
  return out
})

async function load() {
  const card = props.card
  controller?.abort()
  if (!card) return
  const current = new AbortController()
  controller = current
  loading.value = true
  loadError.value = false
  try {
    const result = await getGPTIQTestEntry(card.id, { days: days.value }, current.signal)
    if (current.signal.aborted) return
    detail.value = result
  } catch {
    if (current.signal.aborted) return
    loadError.value = true
  } finally {
    if (controller === current) {
      controller = null
      loading.value = false
    }
  }
}

// 换账号时清空上一账号的详情、筛选与展开状态（天数沿用）；改天数只重读，保留筛选。
watch(
  () => props.card?.id,
  (id, previous) => {
    if (id !== previous) {
      detail.value = null
      caseFilter.value = 0
      expanded.value = new Set()
    }
    if (id) void load()
    else controller?.abort()
  },
  { immediate: true },
)
watch(days, () => {
  if (props.card) void load()
})

function caseName(caseId: number) {
  return caseNames.value.get(caseId) ?? t('gptIQTest.unknownCase', { id: caseId })
}

function isGraded(set: GPTIQCaseSet) {
  return gptIQSetKind(set) === 'graded'
}

// OK 列：判分题看是否答对；对话题看是否拿到回答。
function okMark(set: GPTIQCaseSet, run: GPTIQRun) {
  if (isGraded(set)) return GPT_IQ_RUN_MARK_STYLES[gptIQRunMark(run)]
  if (run.status === 'ok') return GPT_IQ_RUN_MARK_STYLES.correct
  if (run.status === 'error') return GPT_IQ_RUN_MARK_STYLES.wrong
  return GPT_IQ_RUN_MARK_STYLES.none
}

function failureText(run: GPTIQRun) {
  const status = gptIQRunStatusLabel(run.status, t, te)
  const reason = gptIQRunReasonLabel(run.reason, t, te)
  return reason ? `${status} · ${reason}` : status
}

function expandKey(set: GPTIQCaseSet, run: GPTIQRun) {
  return `${set.batch_id}-${set.case_id}-${run.index}`
}

function isExpanded(set: GPTIQCaseSet, run: GPTIQRun) {
  return expanded.value.has(expandKey(set, run))
}

function toggleExpanded(set: GPTIQCaseSet, run: GPTIQRun) {
  const next = new Set(expanded.value)
  const key = expandKey(set, run)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  expanded.value = next
}

function onKeydown(event: KeyboardEvent) {
  if (props.card && event.key === 'Escape') emit('close')
}

onMounted(() => document.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKeydown)
  controller?.abort()
})
</script>
