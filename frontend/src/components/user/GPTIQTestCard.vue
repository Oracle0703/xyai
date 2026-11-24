<template>
  <li class="min-w-0">
    <button
      type="button"
      class="flex h-full w-full min-w-0 flex-col rounded-xl border border-gray-100 bg-gray-50/60 p-3 text-left transition-colors hover:border-primary-300 hover:bg-white focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/40 dark:border-dark-700 dark:bg-dark-800/50 dark:hover:border-primary-700 dark:hover:bg-dark-800"
      :aria-label="t('gptIQTest.openDetail', { name: card.display_name })"
      data-testid="gpt-iq-card"
      @click="emit('open', card)"
    >
      <div class="flex w-full items-center justify-between gap-2">
        <strong class="truncate text-sm font-semibold text-gray-900 dark:text-white" :title="card.display_name">{{ card.display_name }}</strong>
        <span class="badge shrink-0" :class="verdictStyle.badge" data-testid="gpt-iq-verdict">
          <Icon :name="verdictStyle.icon" size="xs" :stroke-width="2" />
          {{ gptIQVerdictLabel(card.verdict, t, te) }}
        </span>
      </div>

      <ul v-if="reasonTexts.length" class="mt-1.5 w-full space-y-0.5 text-xs" :class="reasonClass" data-testid="gpt-iq-reasons">
        <li v-for="(text, index) in reasonTexts" :key="index" class="break-words">{{ text }}</li>
      </ul>

      <div v-if="rows.length" class="mt-2 w-full space-y-1.5">
        <div v-for="row in rows" :key="row.set.case_id" class="rounded-lg bg-white/80 px-2 py-1.5 dark:bg-dark-900/40" data-testid="gpt-iq-case-row">
          <div class="flex items-center justify-between gap-2 text-xs">
            <!-- 模型、次数、思考强度取实际执行的版本（set.revision），不是当前测试项。 -->
            <span class="truncate font-medium text-gray-700 dark:text-gray-200" :title="row.meta ? `${row.name} · ${row.meta}` : row.name">{{ row.name }}</span>
            <span class="flex shrink-0 items-center gap-1">
              <span v-if="row.oldRevision" class="badge badge-gray px-1.5 py-0 text-[10px]" :title="t('gptIQTest.versionTitle', { meta: row.meta })" data-testid="gpt-iq-version">
                {{ t('gptIQTest.versionLabel', { revision: row.set.revision.revision }) }}
              </span>
              <span v-if="row.set.trigger === 'manual'" class="badge badge-primary px-1.5 py-0 text-[10px]" data-testid="gpt-iq-retest">
                {{ t('gptIQTest.retest', { time: formatGPTIQClock(row.set.tested_at) }) }}
              </span>
            </span>
          </div>

          <template v-if="row.kind === 'graded'">
            <div class="mt-1 flex flex-wrap items-center gap-1" data-testid="gpt-iq-run-marks">
              <span
                v-for="run in row.set.runs"
                :key="run.index"
                class="inline-flex h-4 w-4 items-center justify-center rounded text-[10px] font-bold leading-none"
                :class="GPT_IQ_RUN_MARK_STYLES[gptIQRunMark(run)].className"
                :title="runTitle(run)"
              >{{ GPT_IQ_RUN_MARK_STYLES[gptIQRunMark(run)].symbol }}</span>
              <span class="ml-1 text-xs font-semibold tabular-nums text-gray-900 dark:text-white">
                {{ t('gptIQTest.graded', { correct: row.set.correct, graded: row.set.graded }) }}
              </span>
            </div>
            <p class="mt-1 flex flex-wrap gap-x-2 text-[11px] tabular-nums text-gray-500 dark:text-gray-400">
              <span>{{ t('gptIQTest.avgTime', { value: formatGPTIQSeconds(row.set.avg_duration_ms) }) }}</span>
              <span>{{ t('gptIQTest.medianReasoning', { value: formatGPTIQTokens(row.set.median_reasoning_tokens) }) }}</span>
              <span>{{ t('gptIQTest.avgTps', { value: formatGPTIQTps(row.set.avg_tps) }) }}</span>
            </p>
          </template>
          <template v-else>
            <!-- 模型回答一律按纯文本渲染，不得使用 v-html。 -->
            <p v-if="row.preview" class="mt-1 line-clamp-2 whitespace-pre-wrap break-words text-xs text-gray-600 dark:text-gray-300" :title="row.preview" data-testid="gpt-iq-preview">{{ row.preview }}</p>
            <p v-else-if="row.failure" class="mt-1 text-xs" :class="gptIQRunNoteClass(row.failure.status)" data-testid="gpt-iq-failure">
              {{ failureText(row.failure) }}
            </p>
            <p class="mt-0.5 text-[11px] tabular-nums text-gray-500 dark:text-gray-400">
              {{ t('gptIQTest.latency', { value: formatGPTIQSeconds(row.set.avg_duration_ms) }) }}
            </p>
          </template>
        </div>
      </div>
      <p v-else class="flex min-h-16 w-full flex-1 items-center justify-center text-xs text-gray-500 dark:text-gray-400">{{ t('gptIQTest.noResult') }}</p>

      <div class="mt-auto w-full pt-2">
        <GPTIQTrendStrip :trend="card.trend" :today="today" />
        <p v-if="card.tested_at" class="mt-1 truncate text-center text-[10px] text-gray-400 dark:text-dark-500">
          {{ t('gptIQTest.testedAt', { time: formatGPTIQTime(card.tested_at, new Date(now)) }) }}
        </p>
      </div>
    </button>
  </li>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import GPTIQTrendStrip from '@/components/user/GPTIQTrendStrip.vue'
import type { GPTIQPublicCase, GPTIQRun, GPTIQUserCard } from '@/api/gptIQTest'
import {
  GPT_IQ_RUN_MARK_STYLES,
  formatGPTIQClock,
  formatGPTIQSeconds,
  formatGPTIQTime,
  formatGPTIQTokens,
  formatGPTIQTps,
  gptIQCaseNameMap,
  gptIQChatPreview,
  gptIQCurrentRevisionIds,
  gptIQFirstFailure,
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
} from '@/utils/gptIQTest'

const props = withDefaults(defineProps<{
  card: GPTIQUserCard
  cases: GPTIQPublicCase[]
  /** 服务端时区下的当天日期 YYYY-MM-DD，用于趋势条。 */
  today: string
  now: number
  /** 各测试项当前版本 ID（由列表页按全部卡片算出）；缺省时只按本卡片数据判断。 */
  currentRevisions?: ReadonlyMap<number, number> | null
}>(), { currentRevisions: null })

const emit = defineEmits<{ (e: 'open', card: GPTIQUserCard): void }>()

const { t, te } = useI18n()

const caseNames = computed(() => gptIQCaseNameMap(props.cases))
const verdictStyle = computed(() => gptIQVerdictStyle(props.card.verdict))
const reasonTexts = computed(() => props.card.reasons.map((reason) => gptIQReasonText(reason, caseNames.value, t, te)))
const reasonClass = computed(() => {
  if (props.card.verdict === 'suspect') return 'text-red-600 dark:text-red-400'
  if (props.card.verdict === 'failed') return 'text-orange-600 dark:text-orange-400'
  return 'text-gray-500 dark:text-gray-400'
})

const currentRevisions = computed(() => props.currentRevisions ?? gptIQCurrentRevisionIds(props.cases, props.card.cases))

const rows = computed(() =>
  props.card.cases.map((set) => ({
    set,
    kind: gptIQSetKind(set),
    name: caseNames.value.get(set.case_id) ?? t('gptIQTest.unknownCase', { id: set.case_id }),
    meta: gptIQRevisionMeta(set.revision, t),
    oldRevision: gptIQIsOldRevision(set, currentRevisions.value),
    preview: gptIQChatPreview(set),
    failure: gptIQFirstFailure(set),
  })),
)

function failureText(run: GPTIQRun) {
  const status = gptIQRunStatusLabel(run.status, t, te)
  const reason = gptIQRunReasonLabel(run.reason, t, te)
  return reason ? `${status} · ${reason}` : status
}

function runTitle(run: GPTIQRun) {
  const mark = GPT_IQ_RUN_MARK_STYLES[gptIQRunMark(run)].symbol
  if (run.status !== 'ok') return `#${run.index} ${failureText(run)}`
  return `#${run.index} ${mark} · ${formatGPTIQSeconds(run.duration_ms)}s`
}
</script>
