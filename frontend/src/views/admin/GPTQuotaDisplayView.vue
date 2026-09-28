<template>
  <AppLayout>
    <div class="space-y-6">
      <header class="flex flex-wrap items-start justify-between gap-3">
        <div class="space-y-1">
          <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('gptQuota.admin.title') }}</h1>
          <p class="text-sm text-gray-600 dark:text-dark-300">{{ t('gptQuota.admin.description') }}</p>
        </div>
        <div class="flex gap-2">
          <button class="btn btn-secondary btn-sm" :disabled="loading" @click="reloadAll">{{ t('gptQuota.admin.reload') }}</button>
          <button class="btn btn-secondary btn-sm" :disabled="refreshingAll || batch?.running || !serverConfig?.enabled" data-testid="gpt-quota-refresh-all" @click="refreshAll">
            {{ t('gptQuota.admin.refreshAll') }}
          </button>
        </div>
      </header>

      <section class="card space-y-4 p-5">
        <div class="flex flex-wrap items-center justify-between gap-4">
          <label class="flex items-center gap-3">
            <Toggle v-model="enabled" />
            <span class="text-sm font-medium text-gray-900 dark:text-white">{{ t('gptQuota.admin.publicSwitch') }}</span>
          </label>
          <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-dark-200">
            {{ t('gptQuota.admin.interval') }}
            <select v-model.number="interval" class="input w-auto py-1.5 text-sm">
              <option :value="30">{{ t('gptQuota.admin.interval30') }}</option>
              <option :value="60">{{ t('gptQuota.admin.interval60') }}</option>
            </select>
          </label>
        </div>
        <p class="input-hint">{{ t('gptQuota.admin.publicSwitchHint') }}</p>
        <p v-if="schedule" class="text-xs text-gray-500 dark:text-dark-400">
          {{ t('gptQuota.admin.scheduleHint', { start: schedule.start, end: schedule.end, timezone: schedule.timezone }) }}
          <template v-if="nextScheduledAt"> · {{ t('gptQuota.admin.nextScheduled', { time: formatDateTimeToMinute(nextScheduledAt) }) }}</template>
        </p>
        <p v-if="batch" class="text-xs text-gray-500 dark:text-dark-400" data-testid="gpt-quota-batch">
          <template v-if="batch.running">{{ t('gptQuota.admin.batchRunningStatus') }}</template>
          <template v-else-if="batch.finished_at">
            {{ t('gptQuota.admin.batch', { total: batch.total, succeeded: batch.succeeded, failed: batch.failed, skipped: batch.skipped }) }}
            <span v-for="(count, status) in batch.failure_categories" :key="status" class="ml-2">{{ statusLabel(String(status)) }} × {{ count }}</span>
          </template>
        </p>
        <div class="flex items-center gap-3">
          <button class="btn btn-primary" :disabled="saving || loading" data-testid="gpt-quota-save" @click="save">{{ t('gptQuota.admin.save') }}</button>
          <span v-if="dirty" class="text-xs text-amber-600 dark:text-amber-400">{{ t('gptQuota.admin.unsavedChanges') }}</span>
        </div>
      </section>

      <section class="card space-y-3 p-5">
        <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('gptQuota.admin.selectedTitle', { count: selections.length }) }}</h2>
        <p class="input-hint">{{ t('gptQuota.admin.aliasHint') }}</p>
        <p v-if="!selections.length" class="py-4 text-sm text-gray-500 dark:text-dark-400">{{ t('gptQuota.admin.selectedEmpty') }}</p>
        <div v-else class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="text-xs text-gray-500 dark:text-dark-400">
              <tr>
                <th class="py-2 pr-3">{{ t('gptQuota.admin.columns.account') }}</th>
                <th class="py-2 pr-3">{{ t('gptQuota.admin.columns.alias') }}</th>
                <th class="py-2 pr-3">{{ t('gptQuota.admin.columns.quota') }}</th>
                <th class="py-2 pr-3">{{ t('gptQuota.admin.columns.status') }}</th>
                <th class="py-2">{{ t('gptQuota.admin.columns.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="row in selectionRows" :key="row.account_id" data-testid="gpt-quota-selected-row">
                <td class="py-3 pr-3 align-top">
                  <div class="font-medium text-gray-900 dark:text-white">{{ row.account_name || `#${row.account_id}` }}</div>
                  <div class="mt-1 flex flex-wrap gap-1">
                    <span v-if="row.group" class="badge badge-gray">{{ t(`gptQuota.groups.${row.group}`) }}</span>
                    <span v-if="!row.eligible" class="badge badge-danger">{{ reasonLabel(row.reason) }}</span>
                    <span v-if="row.unsaved" class="badge badge-primary">{{ t('gptQuota.admin.unsavedChanges') }}</span>
                    <span v-for="warning in row.warnings" :key="warning" class="badge badge-warning">{{ t(`gptQuota.admin.warnings.${warning}`) }}</span>
                  </div>
                </td>
                <td class="py-3 pr-3 align-top">
                  <input v-model="row.selection.display_name" class="input py-1.5 text-sm" maxlength="40" :placeholder="t('gptQuota.admin.aliasPlaceholder', { name: row.default_name })" />
                </td>
                <td class="py-3 pr-3 align-top text-xs text-gray-600 dark:text-dark-300">
                  <template v-if="row.entry?.sampled_at">
                    <div>{{ t('gptQuota.fiveHour') }}：{{ percent(row.entry.five_hour) }}</div>
                    <div>{{ t('gptQuota.sevenDay') }}：{{ percent(row.entry.seven_day) }}</div>
                    <div class="text-gray-400">
                      {{ t('gptQuota.sampledAt', { time: formatDateTimeToMinute(row.entry.sampled_at) }) }}
                      <span v-if="row.entry.stale" class="badge badge-warning ml-1">{{ t('gptQuota.stale') }}</span>
                    </div>
                  </template>
                  <span v-else>{{ t('gptQuota.noData') }}</span>
                </td>
                <td class="py-3 pr-3 align-top text-xs text-gray-600 dark:text-dark-300">
                  <template v-if="row.entry">
                    <div>{{ statusLabel(row.entry.last_attempt_status) }}</div>
                    <div v-if="row.entry.last_attempt_at" class="text-gray-400">{{ t('gptQuota.admin.lastAttempt', { time: formatDateTimeToMinute(row.entry.last_attempt_at) }) }}</div>
                    <div v-if="row.entry.retry_after" class="text-gray-400">{{ t('gptQuota.admin.retryAfter', { time: formatDateTimeToMinute(row.entry.retry_after) }) }}</div>
                  </template>
                </td>
                <td class="py-3 align-top">
                  <div class="flex gap-2">
                    <button
                      v-if="row.entry"
                      class="btn btn-secondary btn-sm"
                      :disabled="!row.eligible || !serverConfig?.enabled || refreshingEntry === row.entry.id"
                      @click="refreshEntry(row.entry.id)"
                    >
                      {{ t('gptQuota.admin.refresh') }}
                    </button>
                    <button class="btn btn-secondary btn-sm" data-testid="gpt-quota-remove" @click="removeSelection(row.account_id)">{{ t('gptQuota.admin.remove') }}</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="card space-y-3 p-5">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('gptQuota.admin.candidatesTitle') }}</h2>
            <p class="input-hint">{{ t('gptQuota.admin.candidatesHint') }}</p>
          </div>
          <input v-model="search" class="input w-64 py-1.5 text-sm" :placeholder="t('gptQuota.admin.searchPlaceholder')" @keyup.enter="searchCandidates" />
        </div>
        <ul class="divide-y divide-gray-100 dark:divide-dark-700">
          <li v-for="candidate in candidates.items" :key="candidate.account_id" class="flex items-center justify-between gap-3 py-2">
            <div class="min-w-0">
              <div class="truncate text-sm text-gray-900 dark:text-white">{{ candidate.account_name }}</div>
              <div class="mt-0.5 flex gap-1">
                <span v-if="candidate.group" class="badge badge-gray">{{ t(`gptQuota.groups.${candidate.group}`) }}</span>
                <span v-if="!candidate.eligible" class="badge badge-danger">{{ reasonLabel(candidate.reason) }}</span>
              </div>
            </div>
            <button
              class="btn btn-secondary btn-sm shrink-0"
              :disabled="!candidate.eligible || isSelected(candidate.account_id)"
              data-testid="gpt-quota-add"
              @click="addCandidate(candidate)"
            >
              {{ isSelected(candidate.account_id) ? t('gptQuota.admin.added') : t('gptQuota.admin.add') }}
            </button>
          </li>
        </ul>
        <Pagination
          v-if="candidates.total > candidatePageSize"
          :total="candidates.total"
          :page="candidatePage"
          :page-size="candidatePageSize"
          :show-page-size-selector="false"
          @update:page="changeCandidatePage"
        />
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Toggle from '@/components/common/Toggle.vue'
import Pagination from '@/components/common/Pagination.vue'
import {
  getAdminGPTQuota,
  listGPTQuotaCandidates,
  refreshAllGPTQuota,
  refreshGPTQuotaEntry,
  saveGPTQuotaConfig,
  type GPTQuotaAdminEntry,
  type GPTQuotaBatchStatus,
  type GPTQuotaCandidate,
  type GPTQuotaCandidatePage,
  type GPTQuotaConfig,
  type GPTQuotaGroupKey,
  type GPTQuotaSchedule,
  type GPTQuotaSelection,
  type GPTQuotaWindow,
} from '@/api/gptQuotaDisplay'
import { useAppStore } from '@/stores/app'
import { useGPTQuotaVisibility } from '@/composables/useGPTQuotaVisibility'
import { extractApiErrorCode, extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTimeToMinute } from '@/utils/format'

const BATCH_POLL_MS = 5000

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
const batch = ref<GPTQuotaBatchStatus | null>(null)
const serverEntries = ref<GPTQuotaAdminEntry[]>([])

// 本地可编辑状态；轮询批次进度时只刷新服务端条目状态，不覆盖未保存的修改。
const enabled = ref(false)
const interval = ref(30)
const selections = ref<GPTQuotaSelection[]>([])
const addedMeta = ref<Record<number, GPTQuotaCandidate>>({})

const search = ref('')
const candidatePage = ref(1)
const candidatePageSize = 20
const candidates = ref<GPTQuotaCandidatePage>({ items: [], total: 0, page: 1, page_size: candidatePageSize })

let batchTimer: number | undefined
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

async function loadCandidates() {
  candidates.value = await listGPTQuotaCandidates({ search: search.value.trim() || undefined, page: candidatePage.value, page_size: candidatePageSize })
}

async function reloadAll() {
  loading.value = true
  try {
    await Promise.all([applyAdminView(true), loadCandidates()])
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

function searchCandidates() {
  candidatePage.value = 1
  void loadCandidates().catch((err) => appStore.showError(extractApiErrorMessage(err, t('common.error'))))
}

function changeCandidatePage(page: number) {
  candidatePage.value = page
  void loadCandidates().catch((err) => appStore.showError(extractApiErrorMessage(err, t('common.error'))))
}

function isSelected(accountId: number) {
  return selections.value.some((selection) => selection.account_id === accountId)
}

function addCandidate(candidate: GPTQuotaCandidate) {
  if (!candidate.eligible || isSelected(candidate.account_id)) return
  addedMeta.value = { ...addedMeta.value, [candidate.account_id]: candidate }
  selections.value = [...selections.value, { account_id: candidate.account_id, display_name: '' }]
}

// 已失去资格的条目也能直接移除，避免挡住保存。
function removeSelection(accountId: number) {
  selections.value = selections.value.filter((selection) => selection.account_id !== accountId)
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
    await Promise.all([applyAdminView(true), loadCandidates()])
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

function percent(window: GPTQuotaWindow | null) {
  return window ? `${window.remaining_percent.toFixed(1)}%` : t('gptQuota.notProvided')
}

onMounted(() => {
  void reloadAll()
})

onBeforeUnmount(() => {
  disposed = true
  window.clearTimeout(batchTimer)
})
</script>
