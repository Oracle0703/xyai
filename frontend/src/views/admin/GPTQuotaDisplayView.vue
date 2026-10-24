<template>
  <AppLayout>
    <div class="space-y-6 pb-24">
      <!-- 概览：与 Dashboard 一致的统计卡。 -->
      <div class="grid grid-cols-2 gap-4 lg:grid-cols-4" data-testid="gpt-quota-admin-summary">
        <MetricTile
          icon="eye"
          :tone="serverConfig?.enabled ? 'success' : 'gray'"
          :label="t('gptQuota.admin.summary.public')"
          :value="serverConfig ? t(serverConfig.enabled ? 'gptQuota.admin.summary.publicOn' : 'gptQuota.admin.summary.publicOff') : '—'"
          :hint="schedule ? t('gptQuota.admin.summary.publicHint', { interval: serverConfig?.interval_minutes ?? schedule.interval_minutes, start: schedule.start, end: schedule.end }) : undefined"
        />
        <MetricTile
          icon="users"
          :label="t('gptQuota.admin.summary.accounts')"
          :value="String(serverEntries.length)"
          :hint="t('gptQuota.admin.summary.accountsHint', groupCounts)"
        />
        <MetricTile
          icon="checkCircle"
          :tone="attemptCounts.failed > 0 ? 'warning' : 'success'"
          :label="t('gptQuota.admin.summary.lastResult')"
          :value="t('gptQuota.admin.summary.lastResultValue', { ok: attemptCounts.ok, total: serverEntries.length })"
          :hint="t('gptQuota.admin.summary.lastResultHint', attemptCounts)"
        />
        <MetricTile
          icon="clock"
          tone="gray"
          :label="t('gptQuota.admin.summary.nextCollect')"
          :value="formatGPTQuotaTileTime(nextScheduledAt, new Date(now))"
          :hint="t(inScheduleWindow ? 'gptQuota.admin.summary.inWindow' : 'gptQuota.admin.summary.outsideWindow')"
        />
      </div>

      <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <!-- 展示设置：修改后统一在底部操作栏保存。 -->
        <section class="card lg:col-span-2">
          <div class="card-header">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('gptQuota.admin.settingsTitle') }}</h2>
          </div>
          <div class="space-y-5 p-6">
            <div class="flex items-start justify-between gap-4">
              <div>
                <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('gptQuota.admin.publicSwitch') }}</p>
                <p class="input-hint mt-1">{{ t('gptQuota.admin.publicSwitchHint') }}</p>
              </div>
              <Toggle v-model="enabled" />
            </div>
            <div>
              <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('gptQuota.admin.interval') }}</p>
              <div class="mt-2 inline-flex flex-wrap rounded-lg border border-gray-200 bg-gray-50 p-1 dark:border-dark-600 dark:bg-dark-900/50" role="group" :aria-label="t('gptQuota.admin.interval')">
                <button
                  v-for="option in intervalOptions"
                  :key="option.value"
                  type="button"
                  class="rounded-md px-3 py-1.5 text-sm transition-colors"
                  :class="interval === option.value ? segmentActiveClass : segmentIdleClass"
                  :aria-pressed="interval === option.value"
                  :data-testid="`gpt-quota-interval-${option.value}`"
                  @click="interval = option.value"
                >
                  {{ option.label }}
                </button>
              </div>
              <p v-if="schedule" class="input-hint mt-2">
                {{ t('gptQuota.admin.scheduleHint', { start: schedule.start, end: schedule.end, timezone: schedule.timezone }) }}
              </p>
            </div>
          </div>
        </section>

        <!-- 手动刷新与批次进度。 -->
        <section class="card">
          <div class="card-header flex items-center justify-between gap-3">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('gptQuota.admin.refreshTitle') }}</h2>
            <button class="btn btn-ghost btn-sm" :disabled="loading" data-testid="gpt-quota-reload" @click="reloadAll">
              <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
              <span class="ml-1">{{ t('gptQuota.admin.reload') }}</span>
            </button>
          </div>
          <div class="space-y-4 p-6">
            <p class="input-hint">{{ t('gptQuota.admin.refreshHint') }}</p>
            <button class="btn btn-primary w-full" :disabled="refreshingAll || batch?.running || !serverConfig?.enabled" data-testid="gpt-quota-refresh-all" @click="refreshAll">
              <Icon name="sync" size="sm" :class="batch?.running ? 'animate-spin' : ''" />
              <span class="ml-1.5">{{ t('gptQuota.admin.refreshAll') }}</span>
            </button>
            <div class="text-xs text-gray-500 dark:text-gray-400" data-testid="gpt-quota-batch">
              <template v-if="batch?.running">
                <div class="flex items-center justify-between">
                  <span>{{ t('gptQuota.admin.batchRunningStatus') }}</span>
                  <span class="tabular-nums">{{ t('gptQuota.admin.batchProgress', { done: batchDone, total: batch.total }) }}</span>
                </div>
                <div class="mt-1.5 h-1.5 overflow-hidden rounded-full bg-primary-100 dark:bg-primary-900/40">
                  <div class="h-full rounded-full bg-primary-500 transition-all" :style="{ width: `${batchPercent}%` }" />
                </div>
              </template>
              <template v-else-if="batch?.finished_at">
                <p>{{ t('gptQuota.admin.batchFinishedAt', { time: formatDateTimeToMinute(batch.finished_at) }) }}</p>
                <p class="mt-1">{{ t('gptQuota.admin.batch', { total: batch.total, succeeded: batch.succeeded, failed: batch.failed, skipped: batch.skipped }) }}</p>
                <div v-if="batch.failure_categories" class="mt-2 flex flex-wrap gap-1">
                  <span v-for="(count, status) in batch.failure_categories" :key="status" class="badge badge-danger">{{ statusLabel(String(status)) }} × {{ count }}</span>
                </div>
              </template>
              <p v-else>{{ t('gptQuota.admin.batchNever') }}</p>
            </div>
          </div>
        </section>
      </div>

      <!-- 展示账号：筛选、搜索、别名编辑与单条操作。 -->
      <section class="card">
        <div class="card-header flex flex-wrap items-center justify-between gap-3">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('gptQuota.admin.selectedTitle', { count: selections.length }) }}</h2>
          <div class="flex flex-wrap items-center gap-2">
            <button v-if="invalidCount" class="btn btn-secondary btn-sm" data-testid="gpt-quota-remove-invalid" @click="removeInvalid">
              {{ t('gptQuota.admin.removeInvalid', { count: invalidCount }) }}
            </button>
            <button class="btn btn-primary btn-sm" :disabled="loading || !serverConfig" data-testid="gpt-quota-pick" @click="pickerOpen = true">
              <Icon name="plus" size="sm" />
              <span class="ml-1">{{ t('gptQuota.admin.pickAccounts') }}</span>
            </button>
          </div>
        </div>
        <div v-if="selections.length" class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-6 py-3 dark:border-dark-700">
          <div class="inline-flex flex-wrap rounded-lg border border-gray-200 bg-gray-50 p-1 dark:border-dark-600 dark:bg-dark-900/50" role="group">
            <button
              v-for="option in filterOptions"
              :key="option.value"
              type="button"
              class="rounded-md px-3 py-1 text-xs transition-colors"
              :class="filter === option.value ? segmentActiveClass : segmentIdleClass"
              :aria-pressed="filter === option.value"
              :data-testid="`gpt-quota-filter-${option.value}`"
              @click="filter = option.value"
            >
              {{ option.label }}<span class="ml-1 tabular-nums text-gray-400">{{ option.count }}</span>
            </button>
          </div>
          <input v-model.trim="search" class="input w-full py-1.5 text-sm sm:w-64" :placeholder="t('gptQuota.admin.searchSelected')" data-testid="gpt-quota-search" />
        </div>

        <div v-if="!selections.length" class="px-6 py-12 text-center">
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('gptQuota.admin.selectedEmpty') }}</p>
          <button class="btn btn-primary btn-sm mt-3" :disabled="loading || !serverConfig" @click="pickerOpen = true">{{ t('gptQuota.admin.pickAccounts') }}</button>
        </div>
        <p v-else-if="!filteredRows.length" class="px-6 py-10 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('gptQuota.admin.noMatch') }}</p>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="bg-gray-50/80 text-xs text-gray-500 dark:bg-dark-900/40 dark:text-gray-400">
              <tr>
                <th class="px-6 py-2.5 font-medium">{{ t('gptQuota.admin.columns.account') }}</th>
                <th class="px-3 py-2.5 font-medium">{{ t('gptQuota.admin.columns.alias') }}</th>
                <th class="px-3 py-2.5 font-medium">{{ t('gptQuota.admin.columns.quota') }}</th>
                <th class="px-3 py-2.5 font-medium">{{ t('gptQuota.admin.columns.status') }}</th>
                <th class="px-6 py-2.5 text-right font-medium">{{ t('gptQuota.admin.columns.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr
                v-for="row in filteredRows"
                :key="row.account_id"
                :class="row.eligible ? '' : 'bg-red-50/50 dark:bg-red-950/10'"
                data-testid="gpt-quota-selected-row"
              >
                <td class="px-6 py-3 align-top">
                  <div class="flex items-center gap-2">
                    <span v-if="row.group" class="h-2 w-2 shrink-0 rounded-full" :class="row.group === 'xunyou' ? 'bg-sky-500 dark:bg-sky-600' : 'bg-violet-500'" />
                    <span class="font-medium text-gray-900 dark:text-white">{{ row.account_name || `#${row.account_id}` }}</span>
                  </div>
                  <div class="mt-1 flex flex-wrap gap-1">
                    <span v-if="row.group" class="badge badge-gray">{{ t(`gptQuota.groups.${row.group}`) }}</span>
                    <span v-if="!row.eligible" class="badge badge-danger">{{ reasonLabel(row.reason) }}</span>
                    <span v-if="row.unsaved" class="badge badge-primary">{{ t('gptQuota.admin.unsavedChanges') }}</span>
                    <span v-for="warning in row.warnings" :key="warning" class="badge badge-warning">{{ t(`gptQuota.admin.warnings.${warning}`) }}</span>
                  </div>
                </td>
                <td class="px-3 py-3 align-top">
                  <input v-model="row.selection.display_name" class="input w-44 py-1.5 text-sm" maxlength="40" :placeholder="t('gptQuota.admin.aliasPlaceholder', { name: row.default_name })" />
                </td>
                <td class="w-56 px-3 py-3 align-top">
                  <div v-if="row.entry?.sampled_at" class="space-y-2">
                    <GPTQuotaMeter
                      v-for="quotaWindow in visibleGPTQuotaWindows(row.entry)"
                      :key="quotaWindow.key"
                      compact
                      :label="quotaWindow.key === 'five_hour' ? t('gptQuota.fiveHour') : t('gptQuota.sevenDay')"
                      :window="quotaWindow.window"
                      :now="now"
                    />
                    <p class="text-[11px] text-gray-400">
                      {{ t('gptQuota.sampledAt', { time: formatDateTimeToMinute(row.entry.sampled_at) }) }}
                      <span v-if="row.entry.stale" class="badge badge-warning ml-1">{{ t('gptQuota.stale') }}</span>
                    </p>
                  </div>
                  <span v-else class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptQuota.noData') }}</span>
                </td>
                <td class="px-3 py-3 align-top text-xs">
                  <template v-if="row.entry">
                    <span class="badge" :class="statusBadge(row.entry.last_attempt_status)">{{ statusLabel(row.entry.last_attempt_status) }}</span>
                    <div v-if="row.entry.last_attempt_at" class="mt-1 text-gray-400">{{ t('gptQuota.admin.lastAttempt', { time: formatDateTimeToMinute(row.entry.last_attempt_at) }) }}</div>
                    <div v-if="row.entry.retry_after" class="text-gray-400">{{ t('gptQuota.admin.retryAfter', { time: formatDateTimeToMinute(row.entry.retry_after) }) }}</div>
                  </template>
                  <span v-else class="text-gray-400">{{ t('gptQuota.admin.statusAfterSave') }}</span>
                </td>
                <td class="px-6 py-3 align-top">
                  <div class="flex justify-end gap-1">
                    <button
                      v-if="row.entry"
                      class="btn btn-ghost btn-sm"
                      :disabled="!row.eligible || !serverConfig?.enabled || refreshingEntry === row.entry.id"
                      @click="refreshEntry(row.entry.id)"
                    >
                      <Icon name="refresh" size="sm" :class="refreshingEntry === row.entry.id ? 'animate-spin' : ''" />
                      <span class="ml-1">{{ t('gptQuota.admin.refresh') }}</span>
                    </button>
                    <button class="btn btn-ghost btn-sm text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20" data-testid="gpt-quota-remove" @click="removeSelection(row.account_id)">
                      <Icon name="trash" size="sm" />
                      <span class="ml-1">{{ t('gptQuota.admin.remove') }}</span>
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-if="selections.length" class="input-hint border-t border-gray-100 px-6 py-3 dark:border-dark-700">{{ t('gptQuota.admin.aliasHint') }}</p>
      </section>

      <!-- 底部操作栏：开关、间隔、账号与别名的修改在这里统一保存或放弃。 -->
      <div class="sticky bottom-4 z-10">
        <div
          class="card flex flex-wrap items-center justify-between gap-3 px-5 py-3 shadow-lg"
          :class="dirty ? 'border-amber-200 dark:border-amber-900/60' : ''"
          data-testid="gpt-quota-save-bar"
        >
          <p class="flex items-center gap-2 text-sm" :class="dirty ? 'text-amber-700 dark:text-amber-300' : 'text-gray-500 dark:text-gray-400'">
            <Icon :name="dirty ? 'exclamationCircle' : 'checkCircle'" size="sm" />
            {{ dirty ? t('gptQuota.admin.unsavedChanges') : t('gptQuota.admin.allSaved') }}
          </p>
          <div class="flex gap-2">
            <button class="btn btn-secondary btn-sm" :disabled="!dirty || saving || loading" data-testid="gpt-quota-discard" @click="reloadAll">{{ t('gptQuota.admin.discard') }}</button>
            <button class="btn btn-primary btn-sm" :disabled="!dirty || saving || loading" data-testid="gpt-quota-save" @click="save">{{ t('gptQuota.admin.save') }}</button>
          </div>
        </div>
      </div>
    </div>
    <GPTQuotaAccountPickerDialog
      :show="pickerOpen"
      :selected="pickerSelected"
      @close="pickerOpen = false"
      @confirm="applyPicked"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { onBeforeRouteLeave } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Toggle from '@/components/common/Toggle.vue'
import MetricTile from '@/components/common/MetricTile.vue'
import Icon from '@/components/icons/Icon.vue'
import GPTQuotaMeter from '@/components/user/GPTQuotaMeter.vue'
import GPTQuotaAccountPickerDialog from '@/components/admin/gpt-quota/GPTQuotaAccountPickerDialog.vue'
import {
  getAdminGPTQuota,
  refreshAllGPTQuota,
  refreshGPTQuotaEntry,
  saveGPTQuotaConfig,
  type GPTQuotaAdminEntry,
  type GPTQuotaBatchStatus,
  type GPTQuotaCandidate,
  type GPTQuotaConfig,
  type GPTQuotaGroupKey,
  type GPTQuotaSchedule,
  type GPTQuotaSelection,
} from '@/api/gptQuotaDisplay'
import { useAppStore } from '@/stores/app'
import { useGPTQuotaVisibility } from '@/composables/useGPTQuotaVisibility'
import { extractApiErrorCode, extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTimeToMinute } from '@/utils/format'
import { formatGPTQuotaTileTime, visibleGPTQuotaWindows } from '@/utils/gptQuota'

const BATCH_POLL_MS = 5000
const COUNTDOWN_TICK_MS = 30 * 1000

const { t, te } = useI18n()
const appStore = useAppStore()
const { setGPTQuotaVisibility } = useGPTQuotaVisibility()

const loading = ref(false)
const saving = ref(false)
const refreshingAll = ref(false)
const refreshingEntry = ref<number | null>(null)

const serverConfig = ref<GPTQuotaConfig | null>(null)
const schedule = ref<GPTQuotaSchedule | null>(null)
const nextScheduledAt = ref<string | null>(null)
const inScheduleWindow = ref(false)
const batch = ref<GPTQuotaBatchStatus | null>(null)
const serverEntries = ref<GPTQuotaAdminEntry[]>([])

// 本地可编辑状态；轮询批次进度时只刷新服务端条目状态，不覆盖未保存的修改。
const enabled = ref(false)
const interval = ref(30)
const selections = ref<GPTQuotaSelection[]>([])
const addedMeta = ref<Record<number, GPTQuotaCandidate>>({})
const pickerOpen = ref(false)
const now = ref(Date.now())

type RowFilter = 'all' | GPTQuotaGroupKey | 'issues'
const filter = ref<RowFilter>('all')
const search = ref('')

const segmentActiveClass = 'bg-white font-medium text-primary-700 shadow-sm dark:bg-dark-700 dark:text-primary-300'
const segmentIdleClass = 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'

let batchTimer: number | undefined
let tickTimer: number | undefined
let disposed = false

const entryByAccount = computed(() => {
  const map = new Map<number, GPTQuotaAdminEntry>()
  for (const entry of serverEntries.value) map.set(entry.account_id, entry)
  return map
})

interface SelectionRow {
  account_id: number
  selection: GPTQuotaSelection
  entry?: GPTQuotaAdminEntry
  account_name: string
  group: GPTQuotaGroupKey | ''
  eligible: boolean
  reason?: string
  warnings: string[]
  default_name: string
  unsaved: boolean
}

const selectionRows = computed<SelectionRow[]>(() =>
  selections.value.map((selection) => {
    const entry = entryByAccount.value.get(selection.account_id)
    const meta = addedMeta.value[selection.account_id]
    return {
      account_id: selection.account_id,
      selection,
      entry,
      account_name: entry?.account_name ?? meta?.account_name ?? '',
      group: entry?.group ?? meta?.group ?? '',
      eligible: entry?.eligible ?? meta?.eligible ?? false,
      reason: entry?.reason ?? meta?.reason,
      warnings: entry?.warnings ?? [],
      default_name: entry && !entry.display_name ? entry.effective_display_name : defaultDisplayName(entry?.account_name ?? meta?.account_name ?? ''),
      unsaved: !entry,
    }
  }),
)

const intervalOptions = computed(() => [
  { value: 30, label: t('gptQuota.admin.interval30') },
  { value: 60, label: t('gptQuota.admin.interval60') },
])

// 需处理：失去资格，或最近一次采集不是成功/从未采集/采集中。
function rowNeedsAttention(row: SelectionRow) {
  if (!row.eligible) return true
  const status = row.entry?.last_attempt_status
  return !!status && !['ok', 'never', 'running'].includes(status)
}

const filterOptions = computed(() => {
  const rows = selectionRows.value
  return [
    { value: 'all' as RowFilter, label: t('gptQuota.admin.filters.all'), count: rows.length },
    { value: 'xunyou' as RowFilter, label: t('gptQuota.groups.xunyou'), count: rows.filter((row) => row.group === 'xunyou').length },
    { value: 'wsdashi' as RowFilter, label: t('gptQuota.groups.wsdashi'), count: rows.filter((row) => row.group === 'wsdashi').length },
    { value: 'issues' as RowFilter, label: t('gptQuota.admin.filters.issues'), count: rows.filter(rowNeedsAttention).length },
  ]
})

const filteredRows = computed(() => {
  const keyword = search.value.toLowerCase()
  return selectionRows.value.filter((row) => {
    if (filter.value === 'issues' && !rowNeedsAttention(row)) return false
    if ((filter.value === 'xunyou' || filter.value === 'wsdashi') && row.group !== filter.value) return false
    if (!keyword) return true
    return [row.account_name, row.selection.display_name, row.default_name].some((text) => text.toLowerCase().includes(keyword))
  })
})

const invalidCount = computed(() => selectionRows.value.filter((row) => !row.eligible).length)

const groupCounts = computed(() => ({
  xunyou: serverEntries.value.filter((entry) => entry.group === 'xunyou').length,
  wsdashi: serverEntries.value.filter((entry) => entry.group === 'wsdashi').length,
  invalid: serverEntries.value.filter((entry) => !entry.eligible).length,
}))

const attemptCounts = computed(() => {
  const counts = { ok: 0, failed: 0, never: 0 }
  for (const entry of serverEntries.value) {
    if (entry.last_attempt_status === 'ok') counts.ok++
    else if (entry.last_attempt_status === 'never' || entry.last_attempt_status === 'running') counts.never++
    else counts.failed++
  }
  return counts
})

const batchDone = computed(() => (batch.value ? batch.value.succeeded + batch.value.failed + batch.value.skipped : 0))
const batchPercent = computed(() => (batch.value?.total ? Math.min(100, (batchDone.value / batch.value.total) * 100) : 0))

// 弹窗已选区需要展示所有已选账号（含已删除等不在候选列表里的），沿用表格行的名称与资格。
const pickerSelected = computed<GPTQuotaCandidate[]>(() =>
  selectionRows.value.map((row) => ({ account_id: row.account_id, account_name: row.account_name, group: row.group, eligible: row.eligible, reason: row.reason, selected: true })),
)

const dirty = computed(() => {
  if (!serverConfig.value) return false
  if (enabled.value !== serverConfig.value.enabled || interval.value !== serverConfig.value.interval_minutes) return true
  if (selections.value.length !== serverEntries.value.length) return true
  return selections.value.some((selection) => {
    const entry = entryByAccount.value.get(selection.account_id)
    return !entry || entry.display_name !== selection.display_name.trim()
  })
})

function applyAdminView(resetEdits: boolean) {
  return getAdminGPTQuota().then((view) => {
    // 只有重新读取编辑基线时才更新 config/version；批次轮询若也更新 version，
    // 会让本地未保存修改绕过乐观锁覆盖他人刚保存的配置。
    if (resetEdits || !serverConfig.value) serverConfig.value = view.config
    schedule.value = view.schedule
    nextScheduledAt.value = view.next_scheduled_at
    inScheduleWindow.value = view.in_schedule_window
    batch.value = view.batch
    serverEntries.value = view.entries
    if (resetEdits) {
      enabled.value = view.config.enabled
      interval.value = view.config.interval_minutes
      selections.value = view.entries.map((entry) => ({ account_id: entry.account_id, display_name: entry.display_name }))
      addedMeta.value = {}
    }
    scheduleBatchPoll()
  })
}

async function reloadAll() {
  loading.value = true
  try {
    await applyAdminView(true)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

// 弹窗勾选结果只更新本地选择，点击“保存配置”才提交。保留账号沿用已编辑的展示名；
// 本次移除后又勾回的已保存账号恢复服务端展示名。
function applyPicked(ids: number[], meta: Record<number, GPTQuotaCandidate>) {
  const current = new Map(selections.value.map((selection) => [selection.account_id, selection]))
  selections.value = ids.map((id) => current.get(id) ?? { account_id: id, display_name: entryByAccount.value.get(id)?.display_name ?? '' })
  addedMeta.value = { ...addedMeta.value, ...meta }
  pickerOpen.value = false
}

// 已失去资格的条目也能直接移除，避免挡住保存。
function removeSelection(accountId: number) {
  selections.value = selections.value.filter((selection) => selection.account_id !== accountId)
}

function removeInvalid() {
  const invalid = new Set(selectionRows.value.filter((row) => !row.eligible).map((row) => row.account_id))
  selections.value = selections.value.filter((selection) => !invalid.has(selection.account_id))
}

async function save() {
  if (!serverConfig.value) return
  saving.value = true
  try {
    const result = await saveGPTQuotaConfig({
      enabled: enabled.value,
      interval_minutes: interval.value,
      expected_version: serverConfig.value.version,
      entries: selections.value.map((selection) => ({ account_id: selection.account_id, display_name: selection.display_name.trim() })),
    })
    setGPTQuotaVisibility(result.config.enabled)
    appStore.showSuccess(t('gptQuota.admin.saved'))
    for (const warning of result.warnings ?? []) appStore.showError(t(`gptQuota.admin.warnings.${warning}`))
    await applyAdminView(true)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error'), { GPT_QUOTA_CONFIG_CONFLICT: t('gptQuota.admin.conflict') }))
  } finally {
    saving.value = false
  }
}

async function refreshEntry(entryId: number) {
  refreshingEntry.value = entryId
  try {
    const result = await refreshGPTQuotaEntry(entryId)
    const refreshed = result.entry
    if (refreshed) {
      // 单条刷新结果不含跨条目的重复账号告警，沿用原有 warnings。
      serverEntries.value = serverEntries.value.map((entry) => (entry.id === entryId ? { ...refreshed, warnings: refreshed.warnings ?? entry.warnings } : entry))
    }
    const message = statusLabel(result.status)
    if (result.status === 'ok') appStore.showSuccess(message)
    else appStore.showError(message)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    refreshingEntry.value = null
  }
}

async function refreshAll() {
  refreshingAll.value = true
  try {
    batch.value = await refreshAllGPTQuota()
    appStore.showSuccess(t('gptQuota.admin.refreshAllAccepted'))
    scheduleBatchPoll()
  } catch (err) {
    if (extractApiErrorCode(err) === 'GPT_QUOTA_BATCH_RUNNING') appStore.showError(t('gptQuota.admin.batchRunning'))
    else appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    refreshingAll.value = false
  }
}

// 批次执行中定期重读条目状态与计数，完成后停止。
function scheduleBatchPoll() {
  window.clearTimeout(batchTimer)
  if (disposed || !batch.value?.running) return
  batchTimer = window.setTimeout(() => {
    // 读取失败时保留 running 状态并继续重试，避免界面停在过期进度。
    void applyAdminView(false).catch(() => scheduleBatchPoll())
  }, BATCH_POLL_MS)
}

// 与后端默认展示名一致：前缀 + 原样编号；无编号时保存后按条目 ID 生成，这里不展示原名。
function defaultDisplayName(accountName: string) {
  const match = /^([cd])-([0-9]+)/i.exec(accountName.trim())
  return match ? `${match[1].toLowerCase()}-${match[2]}` : t('gptQuota.admin.aliasAutoDefault')
}

function statusLabel(status: string) {
  const key = `gptQuota.admin.statuses.${status}`
  return te(key) ? t(key) : status
}

function reasonLabel(reason?: string) {
  if (!reason) return ''
  const key = `gptQuota.admin.reasons.${reason}`
  return te(key) ? t(key) : reason
}

function statusBadge(status: string) {
  if (status === 'ok') return 'badge-success'
  if (status === 'never') return 'badge-gray'
  if (status === 'running') return 'badge-primary'
  if (status.startsWith('skipped_')) return 'badge-warning'
  return 'badge-danger'
}

// 有未保存修改时，切换页面或关闭标签页前提醒，避免误丢编辑。
function onBeforeUnload(event: BeforeUnloadEvent) {
  if (!dirty.value) return
  event.preventDefault()
  event.returnValue = ''
}

onBeforeRouteLeave(() => (dirty.value ? window.confirm(t('gptQuota.admin.leaveConfirm')) : true))

onMounted(() => {
  void reloadAll()
  tickTimer = window.setInterval(() => {
    now.value = Date.now()
  }, COUNTDOWN_TICK_MS)
  window.addEventListener('beforeunload', onBeforeUnload)
})

onBeforeUnmount(() => {
  disposed = true
  window.clearTimeout(batchTimer)
  window.clearInterval(tickTimer)
  window.removeEventListener('beforeunload', onBeforeUnload)
})
</script>
