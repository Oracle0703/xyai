<template>
  <section class="card" data-testid="gpt-iq-run-panel">
    <div class="card-header flex flex-wrap items-center justify-between gap-3">
      <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('gptIQTest.admin.run.title') }}</h2>
      <button
        class="btn btn-primary btn-sm"
        :disabled="starting || batchRunning || !entries.length || !cases.length"
        data-testid="gpt-iq-run-start"
        @click="start"
      >
        <Icon name="play" size="sm" />
        <span class="ml-1">{{ t('gptIQTest.admin.run.start') }}</span>
      </button>
    </div>

    <div class="space-y-4 p-6">
      <p class="input-hint">{{ t('gptIQTest.admin.run.hint') }}</p>
      <p v-if="!scope.full_scope" class="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:bg-amber-950/20 dark:text-amber-200">
        {{ t('gptIQTest.admin.scopeLimited', { groups: scopeGroupsText }) }}
      </p>

      <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <!-- 账号：来自额度展示条目（只读），只能勾选 runnable 的条目。 -->
        <div class="lg:col-span-2">
          <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
            <p class="text-sm font-medium text-gray-900 dark:text-white">
              {{ t('gptIQTest.admin.run.entries') }}
              <span class="ml-1 text-xs font-normal text-gray-500">{{ selectedEntries.length ? t('gptIQTest.admin.run.selectedCount', { count: selectedEntries.length }) : t('gptIQTest.admin.run.allInScope') }}</span>
            </p>
            <div class="flex gap-1">
              <button type="button" class="btn btn-ghost btn-sm" :disabled="!runnableEntryIds.length" @click="selectedEntries = [...runnableEntryIds]">{{ t('gptIQTest.admin.run.selectAll') }}</button>
              <button type="button" class="btn btn-ghost btn-sm" :disabled="!selectedEntries.length" @click="selectedEntries = []">{{ t('gptIQTest.admin.run.clear') }}</button>
            </div>
          </div>
          <p v-if="!entries.length" class="rounded-lg border border-dashed border-gray-200 px-4 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">{{ t('gptIQTest.admin.run.noEntries') }}</p>
          <div v-else class="grid grid-cols-1 gap-3 md:grid-cols-2">
            <div v-for="group in entryGroups" :key="group.key" class="rounded-lg border border-gray-100 dark:border-dark-700">
              <p class="flex items-center gap-2 border-b border-gray-100 px-3 py-2 text-xs font-medium text-gray-600 dark:border-dark-700 dark:text-gray-300">
                <span class="h-2 w-2 rounded-full" :class="group.key === 'xunyou' ? 'bg-sky-500 dark:bg-sky-600' : 'bg-violet-500'" />
                {{ t(`gptQuota.groups.${group.key}`) }}
                <span class="text-gray-400">{{ group.items.length }}</span>
              </p>
              <ul class="max-h-64 space-y-0.5 overflow-y-auto p-2">
                <li v-for="entry in group.items" :key="entry.id">
                  <label
                    class="flex items-center justify-between gap-2 rounded px-2 py-1 text-sm"
                    :class="entry.runnable ? 'cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-700/50' : 'cursor-not-allowed opacity-60'"
                  >
                    <span class="flex min-w-0 items-center gap-2">
                      <input v-model="selectedEntries" type="checkbox" :value="entry.id" :disabled="!entry.runnable" data-testid="gpt-iq-run-entry" />
                      <span class="truncate">{{ entry.display_name }}</span>
                    </span>
                    <span class="flex shrink-0 gap-1">
                      <span v-if="entry.backoff_until" class="badge badge-warning">{{ t('gptIQTest.admin.run.backoffUntil', { time: formatDateTimeToMinute(entry.backoff_until) }) }}</span>
                      <span v-if="!entry.runnable" class="badge badge-gray">{{ t('gptIQTest.admin.run.notRunnable') }}</span>
                    </span>
                  </label>
                </li>
              </ul>
            </div>
          </div>
        </div>

        <!-- 测试项：含停用项；显式勾选停用项即试运行。 -->
        <div>
          <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
            <p class="text-sm font-medium text-gray-900 dark:text-white">
              {{ t('gptIQTest.admin.run.cases') }}
              <span class="ml-1 text-xs font-normal text-gray-500">{{ selectedCases.length ? t('gptIQTest.admin.run.selectedCount', { count: selectedCases.length }) : t('gptIQTest.admin.run.allInScope') }}</span>
            </p>
            <button type="button" class="btn btn-ghost btn-sm" :disabled="!selectedCases.length" @click="selectedCases = []">{{ t('gptIQTest.admin.run.clear') }}</button>
          </div>
          <p v-if="!cases.length" class="rounded-lg border border-dashed border-gray-200 px-4 py-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">{{ t('gptIQTest.admin.run.noCases') }}</p>
          <ul v-else class="space-y-0.5 rounded-lg border border-gray-100 p-2 dark:border-dark-700">
            <li v-for="item in cases" :key="item.id">
              <label class="flex cursor-pointer items-center justify-between gap-2 rounded px-2 py-1 text-sm hover:bg-gray-50 dark:hover:bg-dark-700/50">
                <span class="flex min-w-0 items-center gap-2">
                  <input v-model="selectedCases" type="checkbox" :value="item.id" data-testid="gpt-iq-run-case" />
                  <span class="truncate">{{ item.name }}</span>
                </span>
                <span class="flex shrink-0 gap-1">
                  <span class="badge badge-gray">{{ t(`gptIQTest.kinds.${item.kind}`) }} × {{ item.runs }}</span>
                  <span v-if="!item.enabled" class="badge badge-warning">{{ t('gptIQTest.admin.run.disabledCase') }}</span>
                </span>
              </label>
            </li>
          </ul>
        </div>
      </div>
    </div>

    <!-- 当前批次：执行中每 3 秒读取一次，逐格出现结果。 -->
    <div class="border-t border-gray-100 dark:border-dark-700" data-testid="gpt-iq-run-current">
      <div v-if="detail" class="flex flex-wrap items-center justify-between gap-3 px-6 py-3">
        <div class="flex flex-wrap items-center gap-2 text-sm">
          <span class="font-medium text-gray-900 dark:text-white">
            {{ t('gptIQTest.admin.run.batchTitle', { id: detail.batch.id, trigger: t(`gptIQTest.triggers.${detail.batch.trigger}`), date: detail.batch.run_date }) }}
          </span>
          <span class="badge" :class="batchStatusBadge(detail.batch.status)" data-testid="gpt-iq-run-status">{{ batchStatusLabel(detail.batch.status) }}</span>
          <span v-if="detail.batch.failure_reason" class="text-xs text-red-600 dark:text-red-400" data-testid="gpt-iq-run-failure">{{ batchFailureLabel(detail.batch.failure_reason) }}</span>
          <span class="tabular-nums text-xs text-gray-500">{{ t('gptIQTest.admin.run.progress', { done: completedCalls, total: visibleTotalCalls }) }}</span>
          <span v-if="detail.batch.cancel_requested && detail.batch.status === 'running'" class="text-xs text-amber-600 dark:text-amber-400">{{ t('gptIQTest.admin.run.cancelRequested') }}</span>
          <span v-if="pollError" class="text-xs text-red-600 dark:text-red-400">{{ t('gptIQTest.admin.run.pollFailed') }}</span>
        </div>
        <button v-if="canCancel" class="btn btn-secondary btn-sm" :disabled="cancelling" data-testid="gpt-iq-run-cancel" @click="cancel">
          <Icon name="x" size="sm" />
          <span class="ml-1">{{ t('gptIQTest.admin.run.cancel') }}</span>
        </button>
      </div>
      <div v-if="detail && detail.batch.status === 'running'" class="mx-6 mb-3 h-1.5 overflow-hidden rounded-full bg-primary-100 dark:bg-primary-900/40">
        <div class="h-full rounded-full bg-primary-500 transition-all" :style="{ width: `${progressPercent}%` }" />
      </div>
      <BatchResultTable v-if="detail" :detail="detail" :entries="entries" :cases="cases" />
      <p v-else-if="activeBatchId !== null" class="px-6 py-8 text-center text-sm text-gray-500 dark:text-gray-400">
        <Icon name="refresh" size="sm" class="mr-1 inline animate-spin" />{{ pollError ? t('gptIQTest.admin.run.pollFailed') : t('common.loading') }}
      </p>
      <p v-else class="px-6 py-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('gptIQTest.admin.run.noBatch') }}</p>
    </div>

    <!-- 最近批次：点击把该批次的逐条结果读入上方表格。 -->
    <div class="border-t border-gray-100 dark:border-dark-700">
      <h3 class="px-6 pb-2 pt-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('gptIQTest.admin.batches.title') }}</h3>
      <p v-if="!batches.length" class="px-6 pb-6 text-sm text-gray-500 dark:text-gray-400">{{ t('gptIQTest.admin.batches.empty') }}</p>
      <div v-else class="overflow-x-auto pb-2">
        <table class="w-full text-left text-xs">
          <thead class="text-gray-500 dark:text-gray-400">
            <tr>
              <th class="px-6 py-2 font-medium">{{ t('gptIQTest.admin.batches.columns.id') }}</th>
              <th class="px-3 py-2 font-medium">{{ t('gptIQTest.admin.batches.columns.trigger') }}</th>
              <th class="px-3 py-2 font-medium">{{ t('gptIQTest.admin.batches.columns.date') }}</th>
              <th class="px-3 py-2 font-medium">{{ t('gptIQTest.admin.batches.columns.status') }}</th>
              <th class="px-3 py-2 text-right font-medium">{{ t('gptIQTest.admin.batches.columns.progress') }}</th>
              <th class="px-3 py-2 text-right font-medium">{{ t('gptIQTest.admin.batches.columns.correct') }}</th>
              <th class="px-3 py-2 text-right font-medium">{{ t('gptIQTest.admin.batches.columns.errors') }}</th>
              <th class="px-3 py-2 text-right font-medium">{{ t('gptIQTest.admin.batches.columns.skipped') }}</th>
              <th class="px-3 py-2 text-right font-medium">{{ t('gptIQTest.admin.batches.columns.interrupted') }}</th>
              <th class="px-3 py-2 font-medium">{{ t('gptIQTest.admin.batches.columns.started') }}</th>
              <th class="px-3 py-2 font-medium">{{ t('gptIQTest.admin.batches.columns.finished') }}</th>
              <th class="px-6 py-2" />
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
            <tr
              v-for="item in batches"
              :key="item.id"
              class="cursor-pointer hover:bg-gray-50 dark:hover:bg-dark-700/40"
              :class="item.id === activeBatchId ? 'bg-primary-50/50 dark:bg-primary-900/10' : ''"
              data-testid="gpt-iq-batch-item"
              @click="viewBatch(item.id)"
            >
              <td class="px-6 py-2 font-medium tabular-nums">#{{ item.id }}</td>
              <td class="px-3 py-2">{{ t(`gptIQTest.triggers.${item.trigger}`) }}</td>
              <td class="px-3 py-2 tabular-nums">{{ item.run_date }}</td>
              <td class="px-3 py-2">
                <span class="badge" :class="batchStatusBadge(item.status)">{{ batchStatusLabel(item.status) }}</span>
                <span v-if="item.failure_reason" class="ml-1 whitespace-nowrap text-red-600 dark:text-red-400" data-testid="gpt-iq-batch-failure">{{ batchFailureLabel(item.failure_reason) }}</span>
              </td>
              <td class="px-3 py-2 text-right tabular-nums">{{ item.completed }}/{{ item.total_calls }}</td>
              <td class="px-3 py-2 text-right tabular-nums">{{ item.graded ? `${item.correct}/${item.graded}` : '—' }}</td>
              <td class="px-3 py-2 text-right tabular-nums">{{ item.errors }}</td>
              <td class="px-3 py-2 text-right tabular-nums">{{ item.skipped }}</td>
              <td class="px-3 py-2 text-right tabular-nums">{{ item.interrupted ?? 0 }}</td>
              <td class="px-3 py-2 tabular-nums">{{ formatDateTimeToMinute(item.started_at) }}</td>
              <td class="px-3 py-2 tabular-nums">{{ item.finished_at ? formatDateTimeToMinute(item.finished_at) : '—' }}</td>
              <td class="px-6 py-2 text-right">
                <span class="text-primary-600 dark:text-primary-400">{{ item.id === activeBatchId ? t('gptIQTest.admin.batches.viewing') : t('gptIQTest.admin.batches.view') }}</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import BatchResultTable from '@/components/admin/gpt-iq-test/BatchResultTable.vue'
import {
  cancelGPTIQTestRun,
  getGPTIQTestRun,
  startGPTIQTestRun,
  type GPTIQActorScope,
  type GPTIQAdminCase,
  type GPTIQAdminEntry,
  type GPTIQBatchDetail,
  type GPTIQBatchSummary,
} from '@/api/admin/gptIQTest'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorCode, extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTimeToMinute } from '@/utils/format'

const props = defineProps<{
  entries: GPTIQAdminEntry[]
  cases: GPTIQAdminCase[]
  batches: GPTIQBatchSummary[]
  runningBatchId: number | null
  scope: GPTIQActorScope
}>()

const emit = defineEmits<{
  /** 批次开始或结束后请父页面重读（预算、最近批次与 running_batch_id）。 */
  (e: 'refresh'): void
}>()

const BATCH_POLL_MS = 3000

const { t, te } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const selectedEntries = ref<number[]>([])
const selectedCases = ref<number[]>([])
const starting = ref(false)
const cancelling = ref(false)
const activeBatchId = ref<number | null>(null)
const detail = ref<GPTIQBatchDetail | null>(null)
const pollError = ref(false)

let pollTimer: number | undefined
let controller: AbortController | null = null
// 最近一次观察到"执行中"的批次；它结束时通知父页面刷新一次。
let runningSeenFor: number | null = null
let disposed = false

const entryGroups = computed(() =>
  (['xunyou', 'wsdashi'] as const)
    .map((key) => ({ key, items: props.entries.filter((entry) => entry.group === key) }))
    .filter((group) => group.items.length),
)
const runnableEntryIds = computed(() => props.entries.filter((entry) => entry.runnable).map((entry) => entry.id))
const scopeGroupsText = computed(() => props.scope.groups.map((group) => (te(`gptQuota.groups.${group}`) ? t(`gptQuota.groups.${group}`) : group)).join('、'))

const batchRunning = computed(() => props.runningBatchId !== null || detail.value?.batch.status === 'running')
const completedCalls = computed(() => detail.value?.results.filter((result) => result.status !== 'running').length ?? 0)
// 组织负责人只拿到本组织的结果，进度分母用后端按范围折算的计划数。
const visibleTotalCalls = computed(() => detail.value?.visible_total_calls ?? detail.value?.batch.total_calls ?? 0)
const progressPercent = computed(() => {
  const total = visibleTotalCalls.value
  return total ? Math.min(100, (completedCalls.value / total) * 100) : 0
})
// 组织负责人只能取消自己发起的手动批次；其余情况由服务端 403 兜底。
const canCancel = computed(() => {
  const batch = detail.value?.batch
  if (!batch || batch.status !== 'running' || batch.cancel_requested) return false
  if (props.scope.full_scope) return true
  return batch.trigger === 'manual' && batch.triggered_by === authStore.user?.id
})

// 运行中的批次不是选择框里的条目时（例如已失去资格），不强制保留无效勾选。
watch(
  () => props.entries,
  (entries) => {
    const runnable = new Set(entries.filter((entry) => entry.runnable).map((entry) => entry.id))
    selectedEntries.value = selectedEntries.value.filter((id) => runnable.has(id))
  },
)
watch(
  () => props.cases,
  (cases) => {
    const known = new Set(cases.map((item) => item.id))
    selectedCases.value = selectedCases.value.filter((id) => known.has(id))
  },
)

// 页面读取到运行中批次（含他人或定时发起）时自动跟踪；正在跟踪另一个运行中批次时不切换。
watch(
  () => props.runningBatchId,
  (id) => {
    if (id === null || id === activeBatchId.value) return
    if (detail.value?.batch.status === 'running') return
    viewBatch(id)
  },
  { immediate: true },
)

function schedulePoll(id: number) {
  window.clearTimeout(pollTimer)
  if (disposed) return
  pollTimer = window.setTimeout(() => void fetchBatch(id), BATCH_POLL_MS)
}

async function fetchBatch(id: number) {
  controller?.abort()
  const current = new AbortController()
  controller = current
  try {
    const result = await getGPTIQTestRun(id, current.signal)
    if (current.signal.aborted || activeBatchId.value !== id) return
    detail.value = result
    pollError.value = false
    if (result.batch.status === 'running') {
      runningSeenFor = id
      schedulePoll(id)
    } else if (runningSeenFor === id) {
      runningSeenFor = null
      emit('refresh')
    }
  } catch (err) {
    if (current.signal.aborted || activeBatchId.value !== id) return
    pollError.value = true
    if (extractApiErrorCode(err) === 'GPT_IQ_TEST_BATCH_NOT_FOUND') return
    // 读取失败时只要批次仍可能在执行就继续重试，避免界面停在过期进度。
    if (!detail.value || detail.value.batch.status === 'running') schedulePoll(id)
  } finally {
    if (controller === current) controller = null
  }
}

function viewBatch(id: number) {
  window.clearTimeout(pollTimer)
  if (activeBatchId.value !== id) {
    activeBatchId.value = id
    detail.value = null
    pollError.value = false
  }
  void fetchBatch(id)
}

async function start() {
  if (starting.value) return
  starting.value = true
  try {
    const payload: { entry_ids?: number[]; case_ids?: number[] } = {}
    if (selectedEntries.value.length) payload.entry_ids = [...selectedEntries.value]
    if (selectedCases.value.length) payload.case_ids = [...selectedCases.value]
    const batch = await startGPTIQTestRun(payload)
    appStore.showSuccess(t('gptIQTest.admin.run.started'))
    window.clearTimeout(pollTimer)
    activeBatchId.value = batch.id
    detail.value = { batch, results: [] }
    pollError.value = false
    runningSeenFor = batch.id
    emit('refresh')
    schedulePoll(batch.id)
  } catch (err) {
    const code = extractApiErrorCode(err)
    appStore.showError(extractApiErrorMessage(err, t('common.error'), {
      GPT_IQ_TEST_BATCH_RUNNING: t('gptIQTest.admin.run.batchRunning'),
      GPT_IQ_TEST_DAILY_BUDGET_EXCEEDED: t('gptIQTest.admin.run.budgetExceeded'),
      GPT_IQ_TEST_SCOPE_DENIED: t('gptIQTest.admin.run.scopeDenied'),
      GPT_IQ_TEST_NOTHING_TO_RUN: t('gptIQTest.admin.run.nothingToRun'),
      GPT_IQ_TEST_ENTRY_NOT_FOUND: t('gptIQTest.admin.run.entryNotFound'),
      GPT_IQ_TEST_CASE_NOT_FOUND: t('gptIQTest.admin.run.caseNotFound'),
    }))
    // 已有批次在执行：重读页面以拿到 running_batch_id 并开始跟踪。
    if (code === 'GPT_IQ_TEST_BATCH_RUNNING') emit('refresh')
  } finally {
    starting.value = false
  }
}

async function cancel() {
  const batch = detail.value?.batch
  if (!batch || cancelling.value) return
  cancelling.value = true
  try {
    await cancelGPTIQTestRun(batch.id)
    if (detail.value?.batch.id === batch.id) detail.value = { ...detail.value, batch: { ...detail.value.batch, cancel_requested: true } }
    appStore.showSuccess(t('gptIQTest.admin.run.cancelRequested'))
  } catch (err) {
    // 批次刚好结束：直接重读，不提示错误。
    if (extractApiErrorCode(err) === 'GPT_IQ_TEST_BATCH_NOT_RUNNING') {
      if (activeBatchId.value === batch.id) void fetchBatch(batch.id)
      return
    }
    appStore.showError(extractApiErrorMessage(err, t('common.error'), {
      GPT_IQ_TEST_SCOPE_DENIED: t('gptIQTest.admin.run.cancelDenied'),
    }))
  } finally {
    cancelling.value = false
  }
}

function batchStatusLabel(status: string) {
  const key = `gptIQTest.admin.batchStatus.${status}`
  return te(key) ? t(key) : status
}

// 批次异常结束原因（超时 / 服务停止 / 写入失败）；空字符串表示正常。
function batchFailureLabel(reason: string) {
  const key = `gptIQTest.admin.batchFailure.${reason}`
  return te(key) ? t(key) : reason
}

function batchStatusBadge(status: string) {
  if (status === 'running') return 'badge-primary'
  if (status === 'finished') return 'badge-success'
  if (status === 'cancelled') return 'badge-gray'
  return 'badge-warning'
}

onBeforeUnmount(() => {
  disposed = true
  window.clearTimeout(pollTimer)
  controller?.abort()
})
</script>
