<template>
  <div class="space-y-5">
    <div class="rounded-lg border border-sky-200 bg-sky-50 p-4 text-sm text-sky-800 dark:border-sky-900/60 dark:bg-sky-900/20 dark:text-sky-200">
      {{ t('admin.riskControl.promptRisk.intro') }}
    </div>

    <div v-if="loading" class="flex items-center gap-2 text-sm text-gray-500">
      <Icon name="refresh" size="sm" class="animate-spin" /> {{ t('common.loading') }}
    </div>

    <template v-else>
      <!-- 基础开关 -->
      <div class="grid grid-cols-1 gap-5 lg:grid-cols-3">
        <div class="flex items-center justify-between rounded-lg border border-gray-100 p-4 dark:border-dark-700">
          <div>
            <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.promptRisk.enabled') }}</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.promptRisk.enabledHint') }}</p>
          </div>
          <Toggle v-model="form.enabled" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.riskControl.promptRisk.mode') }}</label>
          <Select v-model="form.mode" :options="modeOptions" />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.promptRisk.modeHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.riskControl.promptRisk.inputScope') }}</label>
          <Select v-model="form.input_scope" :options="inputScopeOptions" />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.promptRisk.inputScopeHint') }}</p>
        </div>
      </div>

      <!-- 作用域 -->
      <div class="rounded-lg border border-gray-100 p-4 dark:border-dark-700">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.promptRisk.allGroups') }}</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.promptRisk.allGroupsHint') }}</p>
          </div>
          <Toggle v-model="form.all_groups" />
        </div>
        <div v-if="!form.all_groups" class="mt-3">
          <label class="input-label">{{ t('admin.riskControl.promptRisk.groupIds') }}</label>
          <input v-model="form.group_ids_text" class="input" :placeholder="t('admin.riskControl.promptRisk.idsPlaceholder')" />
        </div>
      </div>

      <!-- 阈值 / 状态码 -->
      <div class="grid grid-cols-1 gap-5 lg:grid-cols-2">
        <div>
          <label class="input-label">{{ t('admin.riskControl.promptRisk.threshold') }}</label>
          <input v-model.number="form.escalate_threshold" type="number" min="0" step="0.1" class="input" />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.promptRisk.thresholdHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.riskControl.promptRisk.blockStatus') }}</label>
          <input v-model.number="form.block_status" type="number" min="400" max="599" class="input" />
        </div>
      </div>

      <!-- 拦截消息 + 改写建议 -->
      <div class="grid grid-cols-1 gap-5">
        <div>
          <label class="input-label">{{ t('admin.riskControl.promptRisk.blockMessage') }}</label>
          <textarea v-model="form.block_message" class="input min-h-20 resize-y" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.riskControl.promptRisk.rewriteSuggestion') }}</label>
          <textarea v-model="form.rewrite_suggestion" class="input min-h-24 resize-y" />
        </div>
      </div>

      <!-- 关键词集 -->
      <div class="space-y-3">
        <div class="flex items-center justify-between">
          <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.promptRisk.keywordSets') }}</p>
          <button type="button" class="btn btn-secondary btn-sm inline-flex items-center gap-1" @click="addKeywordSet">
            <Icon name="plus" size="sm" /> {{ t('admin.riskControl.promptRisk.addSet') }}
          </button>
        </div>
        <div
          v-for="(set, idx) in form.keyword_sets"
          :key="idx"
          class="rounded-lg border border-gray-100 p-4 dark:border-dark-700"
        >
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div>
              <label class="input-label">{{ t('admin.riskControl.promptRisk.level') }}</label>
              <Select v-model="set.level" :options="levelOptions" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.riskControl.promptRisk.matchMode') }}</label>
              <Select v-model="set.match_mode" :options="matchModeOptions" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.riskControl.promptRisk.score') }}</label>
              <input v-model.number="set.score" type="number" min="0" step="0.1" class="input" />
            </div>
          </div>
          <div class="mt-3">
            <label class="input-label">{{ t('admin.riskControl.promptRisk.keywords') }}</label>
            <textarea v-model="set.keywords_text" class="input min-h-20 resize-y font-mono text-sm" :placeholder="t('admin.riskControl.promptRisk.keywordsPlaceholder')" />
          </div>
          <div class="mt-2 flex justify-end">
            <button type="button" class="btn btn-danger btn-sm inline-flex items-center gap-1" @click="form.keyword_sets.splice(idx, 1)">
              <Icon name="trash" size="sm" /> {{ t('common.delete') }}
            </button>
          </div>
        </div>
      </div>

      <!-- 豁免 -->
      <div class="space-y-3">
        <div class="flex items-center justify-between">
          <div>
            <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.promptRisk.exemptions') }}</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.promptRisk.exemptionsHint') }}</p>
          </div>
          <button type="button" class="btn btn-secondary btn-sm inline-flex items-center gap-1" @click="addExemption">
            <Icon name="plus" size="sm" /> {{ t('admin.riskControl.promptRisk.addExemption') }}
          </button>
        </div>
        <div
          v-for="(ex, idx) in form.exemptions"
          :key="idx"
          class="grid grid-cols-1 gap-3 rounded-lg border border-gray-100 p-4 dark:border-dark-700 sm:grid-cols-4"
        >
          <div>
            <label class="input-label">{{ t('admin.riskControl.promptRisk.groupIds') }}</label>
            <input v-model="ex.group_ids_text" class="input" :placeholder="t('admin.riskControl.promptRisk.idsPlaceholder')" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.riskControl.promptRisk.userIds') }}</label>
            <input v-model="ex.user_ids_text" class="input" :placeholder="t('admin.riskControl.promptRisk.idsPlaceholder')" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.riskControl.promptRisk.apiKeyIds') }}</label>
            <input v-model="ex.api_key_ids_text" class="input" :placeholder="t('admin.riskControl.promptRisk.idsPlaceholder')" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.riskControl.promptRisk.maxLevel') }}</label>
            <div class="flex items-center gap-2">
              <Select v-model="ex.max_level" :options="levelOptions" class="flex-1" />
              <button type="button" class="btn btn-danger btn-sm" @click="form.exemptions.splice(idx, 1)">
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- 保存 -->
      <div class="flex justify-end">
        <button type="button" class="btn btn-primary inline-flex items-center gap-2" :disabled="saving" @click="save">
          <Icon v-if="saving" name="refresh" size="sm" class="animate-spin" />
          <Icon v-else name="check" size="sm" />
          {{ saving ? t('common.saving') : t('admin.riskControl.promptRisk.save') }}
        </button>
      </div>

      <!-- 在线测试器 -->
      <div class="rounded-xl border border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/60">
        <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.promptRisk.tester') }}</p>
        <textarea v-model="testPrompt" class="input mt-2 min-h-20 resize-y" :placeholder="t('admin.riskControl.promptRisk.testerPlaceholder')" />
        <div class="mt-2 flex items-center gap-3">
          <button type="button" class="btn btn-secondary btn-sm inline-flex items-center gap-1" :disabled="testing || !testPrompt.trim()" @click="runTest">
            <Icon v-if="testing" name="refresh" size="sm" class="animate-spin" />
            <Icon v-else name="play" size="sm" />
            {{ t('admin.riskControl.promptRisk.runTest') }}
          </button>
          <div v-if="testResult" class="flex flex-wrap items-center gap-2 text-sm">
            <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="testActionClass">
              {{ testActionLabel }}
            </span>
            <span class="text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.promptRisk.level') }}: {{ testResult.decision.level }} · {{ t('admin.riskControl.promptRisk.score') }}: {{ testResult.decision.score.toFixed(2) }}
            </span>
          </div>
        </div>
        <div v-if="testResult && testResult.decision.reasons.length" class="mt-2 flex flex-wrap gap-1.5">
          <span
            v-for="(r, i) in testResult.decision.reasons"
            :key="i"
            class="inline-flex rounded bg-gray-200 px-2 py-0.5 text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-200"
          >
            {{ r.keyword }} ({{ r.level }}/{{ r.source }})
          </span>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import { riskControlAPI } from '@/api/admin/riskControl'
import type {
  PromptRiskConfig,
  PromptRiskInputScope,
  PromptRiskLevel,
  PromptRiskMatchMode,
  PromptRiskMode,
  PromptRiskTestResponse,
} from '@/api/admin/riskControl'
import type { SelectOption } from '@/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()

interface EditableKeywordSet {
  level: PromptRiskLevel
  match_mode: PromptRiskMatchMode
  score: number
  keywords_text: string
}

interface EditableExemption {
  group_ids_text: string
  user_ids_text: string
  api_key_ids_text: string
  max_level: PromptRiskLevel
}

interface EditableForm {
  enabled: boolean
  mode: PromptRiskMode
  all_groups: boolean
  group_ids_text: string
  input_scope: PromptRiskInputScope
  block_status: number
  escalate_threshold: number
  block_message: string
  rewrite_suggestion: string
  keyword_sets: EditableKeywordSet[]
  exemptions: EditableExemption[]
}

const loading = ref(true)
const saving = ref(false)
const testing = ref(false)
const testPrompt = ref('')
const testResult = ref<PromptRiskTestResponse | null>(null)

const form = reactive<EditableForm>({
  enabled: false,
  mode: 'observe',
  all_groups: false,
  group_ids_text: '',
  input_scope: 'newest',
  block_status: 403,
  escalate_threshold: 1,
  block_message: '',
  rewrite_suggestion: '',
  keyword_sets: [],
  exemptions: [],
})

const modeOptions = computed<SelectOption[]>(() => [
  { value: 'off', label: t('admin.riskControl.promptRisk.modeOff') },
  { value: 'observe', label: t('admin.riskControl.promptRisk.modeObserve') },
  { value: 'block', label: t('admin.riskControl.promptRisk.modeBlock') },
])

const inputScopeOptions = computed<SelectOption[]>(() => [
  { value: 'newest', label: t('admin.riskControl.promptRisk.scopeNewest') },
  { value: 'full', label: t('admin.riskControl.promptRisk.scopeFull') },
])

const levelOptions = computed<SelectOption[]>(() => [
  { value: 'low', label: t('admin.riskControl.promptRisk.levelLow') },
  { value: 'medium', label: t('admin.riskControl.promptRisk.levelMedium') },
  { value: 'high', label: t('admin.riskControl.promptRisk.levelHigh') },
])

const matchModeOptions = computed<SelectOption[]>(() => [
  { value: 'contains', label: t('admin.riskControl.promptRisk.matchContains') },
  { value: 'word', label: t('admin.riskControl.promptRisk.matchWord') },
  { value: 'regex', label: t('admin.riskControl.promptRisk.matchRegex') },
])

const testActionLabel = computed(() => {
  const action = testResult.value?.decision.action
  if (action === 'block') return t('admin.riskControl.promptRisk.actionBlock')
  if (action === 'log_notify') return t('admin.riskControl.promptRisk.actionObserve')
  return t('admin.riskControl.promptRisk.actionAllow')
})

const testActionClass = computed(() => {
  const action = testResult.value?.decision.action
  if (action === 'block') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (action === 'log_notify') return 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300'
  return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
})

function parseKeywords(text: string): string[] {
  return text
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter(Boolean)
}

function parseIds(text: string): number[] {
  return text
    .split(/[\s,]+/)
    .map((s) => s.trim())
    .filter(Boolean)
    .map((s) => Number(s))
    .filter((n) => Number.isFinite(n) && n > 0)
}

function applyConfig(cfg: PromptRiskConfig) {
  form.enabled = cfg.enabled
  form.mode = cfg.mode
  form.all_groups = cfg.all_groups
  form.group_ids_text = (cfg.group_ids ?? []).join(', ')
  form.input_scope = cfg.input_scope
  form.block_status = cfg.block_status
  form.escalate_threshold = cfg.escalate_threshold
  form.block_message = cfg.block_message
  form.rewrite_suggestion = cfg.rewrite_suggestion
  form.keyword_sets = (cfg.keyword_sets ?? []).map((s) => ({
    level: s.level,
    match_mode: s.match_mode,
    score: s.score,
    keywords_text: (s.keywords ?? []).join('\n'),
  }))
  form.exemptions = (cfg.exemptions ?? []).map((e) => ({
    group_ids_text: (e.group_ids ?? []).join(', '),
    user_ids_text: (e.user_ids ?? []).join(', '),
    api_key_ids_text: (e.api_key_ids ?? []).join(', '),
    max_level: e.max_level,
  }))
}

function buildPayload(): PromptRiskConfig {
  return {
    enabled: form.enabled,
    mode: form.mode,
    all_groups: form.all_groups,
    group_ids: parseIds(form.group_ids_text),
    input_scope: form.input_scope,
    block_status: form.block_status,
    escalate_threshold: form.escalate_threshold,
    block_message: form.block_message,
    rewrite_suggestion: form.rewrite_suggestion,
    keyword_sets: form.keyword_sets.map((s) => ({
      level: s.level,
      match_mode: s.match_mode,
      score: Number(s.score) || 0,
      keywords: parseKeywords(s.keywords_text),
    })),
    exemptions: form.exemptions.map((e) => ({
      group_ids: parseIds(e.group_ids_text),
      user_ids: parseIds(e.user_ids_text),
      api_key_ids: parseIds(e.api_key_ids_text),
      max_level: e.max_level,
    })),
  }
}

function addKeywordSet() {
  form.keyword_sets.push({ level: 'medium', match_mode: 'word', score: 0.4, keywords_text: '' })
}

function addExemption() {
  form.exemptions.push({ group_ids_text: '', user_ids_text: '', api_key_ids_text: '', max_level: 'medium' })
}

async function load() {
  loading.value = true
  try {
    applyConfig(await riskControlAPI.getPromptRiskConfig())
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.promptRisk.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    applyConfig(await riskControlAPI.updatePromptRiskConfig(buildPayload()))
    appStore.showSuccess(t('admin.riskControl.promptRisk.saved'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.promptRisk.saveFailed')))
  } finally {
    saving.value = false
  }
}

async function runTest() {
  if (!testPrompt.value.trim()) return
  testing.value = true
  try {
    testResult.value = await riskControlAPI.testPromptRisk(testPrompt.value)
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.promptRisk.testFailed')))
  } finally {
    testing.value = false
  }
}

onMounted(load)
</script>
