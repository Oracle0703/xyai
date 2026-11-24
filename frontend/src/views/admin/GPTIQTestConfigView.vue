<template>
  <AppLayout>
    <div class="space-y-6 pb-8">
      <div v-if="loadError && !view" class="card flex flex-col items-center gap-3 p-8 text-center text-sm text-red-700 dark:text-red-300" data-testid="gpt-iq-admin-load-error">
        {{ t('gptIQTest.admin.loadFailed') }}
        <button class="btn btn-secondary btn-sm" :disabled="loading" @click="reload">{{ t('gptIQTest.admin.retry') }}</button>
      </div>
      <div v-else-if="!view" class="grid grid-cols-2 gap-4 lg:grid-cols-4" aria-hidden="true">
        <div v-for="index in 4" :key="index" class="card h-[76px] animate-pulse bg-gray-50 dark:bg-dark-800" />
      </div>

      <template v-if="view">
        <div class="grid grid-cols-2 gap-4 lg:grid-cols-4" data-testid="gpt-iq-admin-summary">
          <MetricTile
            icon="eye"
            :tone="view.config.enabled ? 'success' : 'gray'"
            :label="t('gptIQTest.admin.summary.public')"
            :value="t(view.config.enabled ? 'gptIQTest.admin.summary.on' : 'gptIQTest.admin.summary.off')"
          />
          <MetricTile
            icon="clock"
            :tone="view.config.schedule_enabled ? 'primary' : 'gray'"
            :label="t('gptIQTest.admin.summary.schedule')"
            :value="view.config.schedule_enabled ? t('gptIQTest.admin.summary.scheduleValue', { time: view.config.run_time }) : t('gptIQTest.admin.summary.scheduleOff')"
            :hint="view.next_run_at ? t('gptIQTest.admin.summary.nextRun', { time: formatGPTIQTime(view.next_run_at) }) : t('gptIQTest.admin.summary.noNextRun')"
          />
          <MetricTile
            icon="bolt"
            :tone="budgetTone"
            :label="t('gptIQTest.admin.summary.budget')"
            :value="t('gptIQTest.admin.summary.budgetValue', { used: view.dispatched_today, total: view.config.daily_call_budget })"
            :hint="t('gptIQTest.admin.summary.budgetHint', { total: view.config.daily_call_budget })"
            test-id="gpt-iq-admin-budget"
          />
          <MetricTile
            icon="beaker"
            :label="t('gptIQTest.admin.summary.cases')"
            :value="t('gptIQTest.admin.summary.casesValue', { enabled: enabledCaseCount, total: view.cases.length })"
            :hint="t('gptIQTest.admin.summary.casesHint')"
          />
        </div>

        <!-- 测试设置：开关、定时、阈值、额度保护与预算；保存带 expected_version。 -->
        <section class="card" data-testid="gpt-iq-config">
          <div class="card-header flex flex-wrap items-center justify-between gap-3">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('gptIQTest.admin.config.title') }}</h2>
            <button class="btn btn-ghost btn-sm" :disabled="loading" data-testid="gpt-iq-reload" @click="reload">
              <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
              <span class="ml-1">{{ t('gptIQTest.admin.reload') }}</span>
            </button>
          </div>
          <p v-if="!canConfigure" class="mx-6 mt-4 flex items-start gap-2 rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:bg-amber-950/20 dark:text-amber-200" data-testid="gpt-iq-readonly">
            <Icon name="lock" size="xs" class="mt-0.5 shrink-0" />
            {{ t('gptIQTest.admin.readOnly') }}
          </p>
          <!-- 只读（组织负责人未单独授权）时整组控件禁用；写接口服务端同样返回 403。 -->
          <fieldset class="min-w-0 space-y-6 p-6" :disabled="!canConfigure" data-testid="gpt-iq-config-fields">
            <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
              <div class="flex items-start justify-between gap-4">
                <div>
                  <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('gptIQTest.admin.config.public') }}</p>
                  <p class="input-hint mt-1">{{ t('gptIQTest.admin.config.publicHint') }}</p>
                </div>
                <Toggle v-model="form.enabled" class="disabled:cursor-not-allowed disabled:opacity-60" data-testid="gpt-iq-config-enabled" />
              </div>
              <div>
                <div class="flex items-start justify-between gap-4">
                  <div>
                    <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('gptIQTest.admin.config.schedule') }}</p>
                    <p class="input-hint mt-1">{{ t('gptIQTest.admin.config.scheduleHint', { timezone: view.timezone }) }}</p>
                  </div>
                  <Toggle v-model="form.schedule_enabled" class="disabled:cursor-not-allowed disabled:opacity-60" data-testid="gpt-iq-config-schedule" />
                </div>
                <div class="mt-3 flex flex-wrap items-center gap-3">
                  <label class="text-sm text-gray-600 dark:text-gray-300" for="gpt-iq-run-time">{{ t('gptIQTest.admin.config.runTime') }}</label>
                  <input id="gpt-iq-run-time" v-model="form.run_time" type="time" min="05:00" max="11:00" step="60" class="input w-32 py-1.5" data-testid="gpt-iq-config-run-time" />
                  <span v-if="view.config.last_run_date" class="text-xs text-gray-500 dark:text-gray-400">{{ t('gptIQTest.admin.config.lastRunDate', { date: view.config.last_run_date }) }}</span>
                </div>
                <p v-if="!runTimeValid" class="mt-1 text-xs text-red-600 dark:text-red-400">{{ t('gptIQTest.admin.config.runTimeInvalid') }}</p>
              </div>
            </div>

            <div>
              <p class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('gptIQTest.admin.config.thresholdsTitle') }}</p>
              <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
                <div v-for="field in thresholdFields" :key="field.key">
                  <label class="input-label" :for="`gpt-iq-${field.key}`">{{ t(field.label) }}</label>
                  <input :id="`gpt-iq-${field.key}`" v-model.number="form[field.key]" type="number" :min="field.min" :max="field.max" step="1" class="input" :data-testid="`gpt-iq-config-${field.key}`" />
                  <p class="input-hint mt-1">{{ t(field.hint) }}</p>
                </div>
              </div>
            </div>

            <div>
              <p class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('gptIQTest.admin.config.protectionTitle') }}</p>
              <div class="grid grid-cols-1 gap-4 md:grid-cols-4">
                <div v-for="field in protectionFields" :key="field.key">
                  <label class="input-label" :for="`gpt-iq-${field.key}`">{{ t(field.label) }}</label>
                  <input :id="`gpt-iq-${field.key}`" v-model.number="form[field.key]" type="number" :min="field.min" :max="field.max" step="1" class="input" :data-testid="`gpt-iq-config-${field.key}`" />
                  <p v-if="field.hint" class="input-hint mt-1">{{ field.hintParams ? t(field.hint, field.hintParams) : t(field.hint) }}</p>
                </div>
              </div>
              <p class="input-hint mt-2">{{ t('gptIQTest.admin.config.protectionHint') }}</p>
            </div>
          </fieldset>
          <div v-if="canConfigure" class="flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 px-6 py-3 dark:border-dark-700" data-testid="gpt-iq-config-bar">
            <p class="flex items-center gap-2 text-sm" :class="dirty ? 'text-amber-700 dark:text-amber-300' : 'text-gray-500 dark:text-gray-400'">
              <Icon :name="dirty ? 'exclamationCircle' : 'checkCircle'" size="sm" />
              {{ dirty ? t('gptIQTest.admin.unsaved') : t('gptIQTest.admin.allSaved') }}
              <span v-if="!dirty && view.config.updated_at" class="text-xs text-gray-400">{{ t('gptIQTest.admin.config.updatedAt', { time: formatDateTimeToMinute(view.config.updated_at) }) }}</span>
            </p>
            <div class="flex gap-2">
              <button class="btn btn-secondary btn-sm" :disabled="!dirty || saving" data-testid="gpt-iq-config-discard" @click="resetForm">{{ t('gptIQTest.admin.discard') }}</button>
              <button class="btn btn-primary btn-sm" :disabled="!dirty || saving || !formValid" data-testid="gpt-iq-config-save" @click="saveConfig">{{ t('gptIQTest.admin.save') }}</button>
            </div>
          </div>
        </section>

        <!-- 测试项：全站共享；编辑/删除带 expected_version。 -->
        <section class="card" data-testid="gpt-iq-cases">
          <div class="card-header flex flex-wrap items-center justify-between gap-3">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('gptIQTest.admin.cases.title', { count: view.cases.length }) }}</h2>
            <div v-if="canConfigure" class="flex flex-wrap items-center gap-2">
              <div class="relative">
                <button class="btn btn-secondary btn-sm" :disabled="!view.templates.length" :aria-expanded="templateMenuOpen" data-testid="gpt-iq-template-menu" @click="templateMenuOpen = !templateMenuOpen">
                  <Icon name="document" size="sm" />
                  <span class="ml-1">{{ t('gptIQTest.admin.cases.fromTemplate') }}</span>
                  <Icon name="chevronDown" size="xs" class="ml-1" />
                </button>
                <ul v-if="templateMenuOpen" class="absolute right-0 z-20 mt-1 min-w-[10rem] rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-dark-600 dark:bg-dark-800">
                  <li v-for="template in view.templates" :key="template.key">
                    <button type="button" class="block w-full px-3 py-1.5 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-700" :data-testid="`gpt-iq-template-${template.key}`" @click="openTemplate(template)">
                      {{ te(`gptIQTest.admin.templates.${template.key}`) ? t(`gptIQTest.admin.templates.${template.key}`) : template.input.name }}
                    </button>
                  </li>
                </ul>
              </div>
              <button class="btn btn-primary btn-sm" data-testid="gpt-iq-case-add" @click="openCreate">
                <Icon name="plus" size="sm" />
                <span class="ml-1">{{ t('gptIQTest.admin.cases.add') }}</span>
              </button>
            </div>
          </div>
          <p v-if="!view.cases.length" class="px-6 py-10 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('gptIQTest.admin.cases.empty') }}</p>
          <div v-else class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <thead class="bg-gray-50/80 text-xs text-gray-500 dark:bg-dark-900/40 dark:text-gray-400">
                <tr>
                  <th class="px-6 py-2.5 font-medium">{{ t('gptIQTest.admin.cases.columns.name') }}</th>
                  <th class="px-3 py-2.5 font-medium">{{ t('gptIQTest.admin.cases.columns.kind') }}</th>
                  <th class="px-3 py-2.5 font-medium">{{ t('gptIQTest.admin.cases.columns.model') }}</th>
                  <th class="px-3 py-2.5 font-medium">{{ t('gptIQTest.admin.cases.columns.effort') }}</th>
                  <th class="px-3 py-2.5 text-right font-medium">{{ t('gptIQTest.admin.cases.columns.runs') }}</th>
                  <th class="px-3 py-2.5 font-medium">{{ t('gptIQTest.admin.cases.columns.scoring') }}</th>
                  <th class="px-3 py-2.5 font-medium">{{ t('gptIQTest.admin.cases.columns.status') }}</th>
                  <th class="px-3 py-2.5 font-medium">{{ t('gptIQTest.admin.cases.columns.revision') }}</th>
                  <th v-if="canConfigure" class="px-6 py-2.5 text-right font-medium">{{ t('gptIQTest.admin.cases.columns.actions') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr v-for="item in view.cases" :key="item.id" class="align-top" data-testid="gpt-iq-case-row">
                  <td class="px-6 py-3">
                    <p class="font-medium text-gray-900 dark:text-white">{{ item.name }}</p>
                    <div class="mt-1 flex flex-wrap gap-1">
                      <span v-if="item.public_prompt" class="badge badge-gray">{{ t('gptIQTest.admin.cases.publicPrompt') }}</span>
                      <span v-if="item.public_answer" class="badge badge-gray">{{ t('gptIQTest.admin.cases.publicAnswer') }}</span>
                    </div>
                    <p v-if="rejudgeResults[item.id]" class="mt-1 text-xs text-emerald-700 dark:text-emerald-400" data-testid="gpt-iq-rejudge-result">{{ rejudgeResults[item.id] }}</p>
                  </td>
                  <td class="px-3 py-3">{{ t(`gptIQTest.kinds.${item.kind}`) }}</td>
                  <td class="px-3 py-3 font-mono text-xs">{{ item.model }}</td>
                  <td class="px-3 py-3 text-xs">{{ item.reasoning_effort || t('gptIQTest.admin.cases.effortDefault') }}</td>
                  <td class="px-3 py-3 text-right tabular-nums">{{ item.runs }}</td>
                  <td class="px-3 py-3 text-xs">
                    {{ scoringLabel(item.scoring_mode) }}
                    <span v-if="item.kind === 'graded' && item.scoring_expected" class="ml-1 break-all font-mono text-gray-500">= {{ item.scoring_expected }}</span>
                  </td>
                  <td class="px-3 py-3">
                    <span class="badge" :class="item.enabled ? 'badge-success' : 'badge-gray'">{{ t(item.enabled ? 'gptIQTest.admin.cases.enabled' : 'gptIQTest.admin.cases.disabled') }}</span>
                  </td>
                  <td class="px-3 py-3 text-xs tabular-nums text-gray-500">{{ t('gptIQTest.admin.cases.revision', { revision: item.revision }) }}</td>
                  <td v-if="canConfigure" class="px-6 py-3">
                    <div class="flex justify-end gap-1">
                      <button
                        v-if="item.kind === 'graded'"
                        class="btn btn-ghost btn-sm"
                        :disabled="rejudging === item.id"
                        :title="t('gptIQTest.admin.cases.rejudgeHint')"
                        data-testid="gpt-iq-case-rejudge"
                        @click="rejudge(item)"
                      >
                        <Icon name="calculator" size="sm" />
                        <span class="ml-1">{{ rejudging === item.id ? t('gptIQTest.admin.cases.rejudging') : t('gptIQTest.admin.cases.rejudge') }}</span>
                      </button>
                      <button class="btn btn-ghost btn-sm" data-testid="gpt-iq-case-edit" @click="openEdit(item)">
                        <Icon name="edit" size="sm" />
                        <span class="ml-1">{{ t('gptIQTest.admin.cases.edit') }}</span>
                      </button>
                      <button class="btn btn-ghost btn-sm text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20" data-testid="gpt-iq-case-delete" @click="deleting = item">
                        <Icon name="trash" size="sm" />
                        <span class="ml-1">{{ t('gptIQTest.admin.cases.delete') }}</span>
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <RunPanel
          :entries="view.entries"
          :cases="view.cases"
          :batches="view.batches"
          :running-batch-id="view.running_batch_id"
          :scope="view.scope"
          @refresh="refreshView"
        />
      </template>
    </div>

    <CaseEditorDialog
      v-if="canConfigure"
      :show="editorOpen"
      :editing="editingCase"
      :template="editorTemplate"
      :default-sort-order="nextSortOrder"
      @close="closeEditor"
      @saved="onCaseSaved"
    />
    <ConfirmDialog
      v-if="canConfigure"
      :show="deleting !== null"
      :title="t('gptIQTest.admin.cases.deleteTitle')"
      :message="t('gptIQTest.admin.cases.deleteConfirm', { name: deleting?.name ?? '' })"
      :disabled="deletingBusy"
      danger
      @confirm="confirmDelete"
      @cancel="deleting = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { onBeforeRouteLeave } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import MetricTile from '@/components/common/MetricTile.vue'
import Toggle from '@/components/common/Toggle.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import CaseEditorDialog from '@/components/admin/gpt-iq-test/CaseEditorDialog.vue'
import RunPanel from '@/components/admin/gpt-iq-test/RunPanel.vue'
import {
  deleteGPTIQTestCase,
  getAdminGPTIQTest,
  rejudgeGPTIQTestCase,
  updateGPTIQTestConfig,
  type GPTIQAdminCase,
  type GPTIQAdminConfig,
  type GPTIQAdminView,
  type GPTIQCaseTemplate,
  type GPTIQConfigUpdate,
} from '@/api/admin/gptIQTest'
import { useAppStore } from '@/stores/app'
import { useGPTQuotaVisibility } from '@/composables/useGPTQuotaVisibility'
import { extractApiErrorCode, extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTimeToMinute } from '@/utils/format'
import { formatGPTIQTime } from '@/utils/gptIQTest'

// 本页只调用 /admin/gpt-iq-test*：组织负责人无权访问 /admin/gpt-quota。
const { t, te } = useI18n()
const appStore = useAppStore()
const { setGPTIQTestVisibility } = useGPTQuotaVisibility()

type ConfigForm = Omit<GPTIQConfigUpdate, 'expected_version'>
type NumberField = 'min_accuracy_percent' | 'min_graded_runs' | 'reasoning_ratio_percent' | 'min_five_hour_percent' | 'min_seven_day_percent' | 'daily_call_budget' | 'retention_days'

const NUMBER_RANGES: Record<NumberField, [number, number]> = {
  min_accuracy_percent: [0, 100],
  min_graded_runs: [1, 5],
  reasoning_ratio_percent: [0, 100],
  min_five_hour_percent: [0, 100],
  min_seven_day_percent: [0, 100],
  daily_call_budget: [0, 5000],
  retention_days: [7, 365],
}

const view = ref<GPTIQAdminView | null>(null)
const loading = ref(false)
const loadError = ref(false)
const saving = ref(false)
const form = reactive<ConfigForm>(emptyForm())

const editorOpen = ref(false)
const editingCase = ref<GPTIQAdminCase | null>(null)
const editorTemplate = ref<GPTIQCaseTemplate | null>(null)
const templateMenuOpen = ref(false)
const deleting = ref<GPTIQAdminCase | null>(null)
const deletingBusy = ref(false)
const rejudging = ref<number | null>(null)
const rejudgeResults = ref<Record<number, string>>({})
let loadSeq = 0

function emptyForm(): ConfigForm {
  return {
    enabled: false,
    schedule_enabled: false,
    run_time: '08:30',
    min_accuracy_percent: 60,
    min_graded_runs: 3,
    reasoning_ratio_percent: 30,
    min_five_hour_percent: 10,
    min_seven_day_percent: 5,
    daily_call_budget: 300,
    retention_days: 90,
  }
}

function formFromConfig(config: GPTIQAdminConfig): ConfigForm {
  return {
    enabled: config.enabled,
    schedule_enabled: config.schedule_enabled,
    run_time: config.run_time,
    min_accuracy_percent: config.min_accuracy_percent,
    min_graded_runs: config.min_graded_runs,
    reasoning_ratio_percent: config.reasoning_ratio_percent,
    min_five_hour_percent: config.min_five_hour_percent,
    min_seven_day_percent: config.min_seven_day_percent,
    daily_call_budget: config.daily_call_budget,
    retention_days: config.retention_days,
  }
}

const thresholdFields = computed(() => [
  { key: 'min_accuracy_percent' as const, label: 'gptIQTest.admin.config.minAccuracy', hint: 'gptIQTest.admin.config.minAccuracyHint', min: 0, max: 100 },
  { key: 'min_graded_runs' as const, label: 'gptIQTest.admin.config.minGradedRuns', hint: 'gptIQTest.admin.config.minGradedRunsHint', min: 1, max: 5 },
  { key: 'reasoning_ratio_percent' as const, label: 'gptIQTest.admin.config.reasoningRatio', hint: 'gptIQTest.admin.config.reasoningRatioHint', min: 0, max: 100 },
])

const protectionFields = computed(() => [
  { key: 'min_five_hour_percent' as const, label: 'gptIQTest.admin.config.minFiveHour', hint: '', min: 0, max: 100, hintParams: null },
  { key: 'min_seven_day_percent' as const, label: 'gptIQTest.admin.config.minSevenDay', hint: '', min: 0, max: 100, hintParams: null },
  {
    key: 'daily_call_budget' as const,
    label: 'gptIQTest.admin.config.budget',
    hint: 'gptIQTest.admin.config.budgetHint',
    min: 0,
    max: 5000,
    hintParams: { used: view.value?.dispatched_today ?? 0, total: view.value?.config.daily_call_budget ?? 0 },
  },
  { key: 'retention_days' as const, label: 'gptIQTest.admin.config.retention', hint: 'gptIQTest.admin.config.retentionHint', min: 7, max: 365, hintParams: null },
])

// 与后端一致：05:00–11:00（含两端）。
const runTimeValid = computed(() => /^\d{2}:\d{2}$/.test(form.run_time) && form.run_time >= '05:00' && form.run_time <= '11:00')
const numbersValid = computed(() =>
  (Object.keys(NUMBER_RANGES) as NumberField[]).every((key) => {
    const value = form[key]
    const [min, max] = NUMBER_RANGES[key]
    return Number.isInteger(value) && value >= min && value <= max
  }),
)
const formValid = computed(() => runTimeValid.value && numbersValid.value)

const dirty = computed(() => {
  const config = view.value?.config
  if (!config) return false
  const base = formFromConfig(config)
  return (Object.keys(base) as Array<keyof ConfigForm>).some((key) => base[key] !== form[key])
})

// 只有完整管理员与持有 admin.gpt_iq_test 的子管理员可写；组织负责人只读设置与测试项（仍可手动测试）。
const canConfigure = computed(() => view.value?.scope.can_configure === true)
const enabledCaseCount = computed(() => view.value?.cases.filter((item) => item.enabled).length ?? 0)
const nextSortOrder = computed(() => (view.value?.cases.reduce((max, item) => Math.max(max, item.sort_order), 0) ?? 0) + 10)
const budgetTone = computed(() => {
  const current = view.value
  if (!current) return 'gray' as const
  if (current.dispatched_today >= current.config.daily_call_budget) return 'danger' as const
  if (current.dispatched_today >= current.config.daily_call_budget * 0.8) return 'warning' as const
  return 'primary' as const
})

function resetForm() {
  if (view.value) Object.assign(form, formFromConfig(view.value.config))
}

// resetEdits=false 时只刷新服务端状态（预算、批次、测试项），不覆盖未保存的设置修改与编辑基线。
async function applyView(resetEdits: boolean) {
  const seq = ++loadSeq
  const result = await getAdminGPTIQTest()
  if (seq !== loadSeq) return
  const previousConfig = view.value?.config
  // 未保存修改时保留旧的 config/version，避免绕过乐观锁覆盖他人的保存。
  const keepConfig = !resetEdits && previousConfig && dirty.value
  view.value = keepConfig ? { ...result, config: previousConfig } : result
  if (!keepConfig) resetForm()
  loadError.value = false
}

async function reload() {
  loading.value = true
  try {
    await applyView(true)
  } catch (err) {
    loadError.value = true
    appStore.showError(extractApiErrorMessage(err, t('gptIQTest.admin.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function refreshView() {
  try {
    await applyView(false)
  } catch {
    // 后台刷新失败保留当前画面；用户可手动重新读取。
  }
}

async function saveConfig() {
  const current = view.value
  if (!current || !canConfigure.value || !formValid.value) return
  saving.value = true
  try {
    const config = await updateGPTIQTestConfig({ ...form, expected_version: current.config.version })
    if (view.value) view.value = { ...view.value, config }
    resetForm()
    setGPTIQTestVisibility(config.enabled)
    appStore.showSuccess(t('gptIQTest.admin.saved'))
    // 定时开关/时间变化会影响 next_run_at，静默刷新一次。
    void refreshView()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error'), { GPT_IQ_TEST_CONFIG_CONFLICT: t('gptIQTest.admin.conflict') }))
  } finally {
    saving.value = false
  }
}

function openCreate() {
  templateMenuOpen.value = false
  editingCase.value = null
  editorTemplate.value = null
  editorOpen.value = true
}

function openTemplate(template: GPTIQCaseTemplate) {
  templateMenuOpen.value = false
  editingCase.value = null
  editorTemplate.value = { ...template, input: { ...template.input, sort_order: nextSortOrder.value } }
  editorOpen.value = true
}

function openEdit(item: GPTIQAdminCase) {
  templateMenuOpen.value = false
  editorTemplate.value = null
  editingCase.value = item
  editorOpen.value = true
}

function closeEditor() {
  editorOpen.value = false
  editingCase.value = null
  editorTemplate.value = null
}

function onCaseSaved() {
  closeEditor()
  void refreshView()
}

async function confirmDelete() {
  const item = deleting.value
  if (!item || deletingBusy.value) return
  deletingBusy.value = true
  try {
    await deleteGPTIQTestCase(item.id, item.version)
    appStore.showSuccess(t('gptIQTest.admin.cases.deleted'))
    deleting.value = null
    await refreshView()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error'), {
      GPT_IQ_TEST_CASE_CONFLICT: t('gptIQTest.admin.cases.conflict'),
      GPT_IQ_TEST_CASE_NOT_FOUND: t('gptIQTest.admin.cases.notFound'),
    }))
    if (extractApiErrorCode(err) === 'GPT_IQ_TEST_CASE_CONFLICT') deleting.value = null
  } finally {
    deletingBusy.value = false
  }
}

async function rejudge(item: GPTIQAdminCase) {
  if (rejudging.value !== null || !canConfigure.value) return
  rejudging.value = item.id
  try {
    // 只重算当前版本；测试项在此期间被修改时服务端返回 409，提示重新读取。
    const result = await rejudgeGPTIQTestCase(item.id, item.revision_id)
    const message = t('gptIQTest.admin.cases.rejudgeDone', { checked: result.checked, updated: result.updated, skipped: result.skipped_truncated })
    rejudgeResults.value = { ...rejudgeResults.value, [item.id]: message }
    appStore.showSuccess(message)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('common.error'), {
      GPT_IQ_TEST_REJUDGE_NOT_GRADED: t('gptIQTest.admin.cases.rejudgeNotGraded'),
      GPT_IQ_TEST_CASE_CONFLICT: t('gptIQTest.admin.cases.rejudgeConflict'),
      GPT_IQ_TEST_CASE_NOT_FOUND: t('gptIQTest.admin.cases.notFound'),
    }))
  } finally {
    rejudging.value = null
  }
}

function scoringLabel(mode: string) {
  const key = `gptIQTest.admin.scoringModes.${mode}`
  return te(key) ? t(key) : mode
}

function onDocumentClick(event: MouseEvent) {
  if (!templateMenuOpen.value) return
  const target = event.target as HTMLElement | null
  if (!target?.closest('[data-testid="gpt-iq-template-menu"]') && !target?.closest('[data-testid^="gpt-iq-template-"]')) templateMenuOpen.value = false
}

// 有未保存的设置修改时，切换页面或关闭标签页前提醒。
function onBeforeUnload(event: BeforeUnloadEvent) {
  if (!dirty.value) return
  event.preventDefault()
  event.returnValue = ''
}

onBeforeRouteLeave(() => (dirty.value ? window.confirm(t('gptIQTest.admin.leaveConfirm')) : true))

onMounted(() => {
  void reload()
  window.addEventListener('beforeunload', onBeforeUnload)
  document.addEventListener('click', onDocumentClick)
})

onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', onBeforeUnload)
  document.removeEventListener('click', onDocumentClick)
})
</script>
