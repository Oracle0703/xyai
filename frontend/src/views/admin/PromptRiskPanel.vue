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

      <!-- LLM 语义复核(judge) -->
      <div class="space-y-3 rounded-xl border border-indigo-100 bg-indigo-50/40 p-4 dark:border-indigo-900/40 dark:bg-indigo-900/10">
        <div class="flex items-start justify-between gap-3">
          <div>
            <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.promptRisk.judge.title') }}</p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.promptRisk.judge.hint') }}</p>
          </div>
          <label class="inline-flex shrink-0 cursor-pointer items-center gap-2">
            <input v-model="form.judge.enabled" type="checkbox" class="checkbox" />
            <span class="text-sm text-gray-700 dark:text-gray-200">{{ t('admin.riskControl.promptRisk.judge.enabled') }}</span>
          </label>
        </div>

        <div v-if="form.judge.enabled" class="space-y-3">
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div>
              <label class="input-label">{{ t('admin.riskControl.promptRisk.judge.baseUrl') }}</label>
              <input v-model="form.judge.base_url" class="input" placeholder="https://your-gateway.example.com" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.riskControl.promptRisk.judge.model') }}</label>
              <input v-model="form.judge.model" class="input" placeholder="gpt-4o-mini" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.riskControl.promptRisk.judge.apiKey') }}</label>
              <input v-model="form.judge.api_key" type="password" autocomplete="off" class="input" :placeholder="form.judge.api_key_configured ? form.judge.api_key_masked || '••••••••' : ''" />
              <p class="mt-1 text-xs text-gray-400">{{ t('admin.riskControl.promptRisk.judge.apiKeyHint') }}</p>
            </div>
            <div>
              <label class="input-label">{{ t('admin.riskControl.promptRisk.judge.timeoutMs') }}</label>
              <input v-model.number="form.judge.timeout_ms" type="number" min="500" max="15000" step="100" class="input" />
            </div>
          </div>

          <div>
            <label class="input-label">{{ t('admin.riskControl.promptRisk.judge.triggerLevels') }}</label>
            <div class="flex flex-wrap gap-3">
              <label v-for="opt in levelOptions" :key="String(opt.value)" class="inline-flex cursor-pointer items-center gap-1.5 text-sm text-gray-700 dark:text-gray-200">
                <input
                  type="checkbox"
                  class="checkbox"
                  :checked="form.judge.trigger_levels.includes(opt.value as PromptRiskLevel)"
                  @change="toggleJudgeTriggerLevel(opt.value as PromptRiskLevel)"
                />
                {{ opt.label }}
              </label>
            </div>
            <p class="mt-1 text-xs text-gray-400">{{ t('admin.riskControl.promptRisk.judge.triggerLevelsHint') }}</p>
          </div>

          <div>
            <label class="input-label">{{ t('admin.riskControl.promptRisk.judge.promptTemplate') }}</label>
            <textarea v-model="form.judge.prompt_template" class="input min-h-24 resize-y" :placeholder="t('admin.riskControl.promptRisk.judge.promptTemplatePlaceholder')" />
          </div>

          <!-- 防递归提示 -->
          <div class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
            <p class="font-semibold">{{ t('admin.riskControl.promptRisk.judge.recursionTitle') }}</p>
            <p class="mt-1 whitespace-pre-wrap">{{ t('admin.riskControl.promptRisk.judge.recursionHint') }}</p>
            <button type="button" class="btn btn-secondary btn-sm mt-2 inline-flex items-center gap-1" @click="addExemptionForJudge">
              <Icon name="plus" size="sm" /> {{ t('admin.riskControl.promptRisk.judge.addExemption') }}
            </button>
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
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.promptRisk.testerRuleOnlyHint') }}</p>
        <textarea v-model="testPrompt" class="input mt-2 min-h-20 resize-y" :placeholder="t('admin.riskControl.promptRisk.testerPlaceholder')" />
        <div class="mt-2">
          <button type="button" class="btn btn-secondary btn-sm inline-flex items-center gap-1" :disabled="testing || !testPrompt.trim()" @click="runTest">
            <Icon v-if="testing" name="refresh" size="sm" class="animate-spin" />
            <Icon v-else name="play" size="sm" />
            {{ t('admin.riskControl.promptRisk.runTest') }}
          </button>
        </div>

        <!-- 结果:二元提示 —— 拦截 / 请求生效 -->
        <div
          v-if="testResult"
          class="mt-3 rounded-lg border p-3"
          :class="isBlocked
            ? 'border-red-200 bg-red-50 dark:border-red-900/50 dark:bg-red-900/20'
            : 'border-green-200 bg-green-50 dark:border-green-900/50 dark:bg-green-900/20'"
        >
          <div class="flex flex-wrap items-center gap-2">
            <Icon
              :name="isBlocked ? 'x' : 'check'"
              size="sm"
              :class="isBlocked ? 'text-red-600 dark:text-red-400' : 'text-green-600 dark:text-green-400'"
            />
            <span
              class="text-sm font-semibold"
              :class="isBlocked ? 'text-red-700 dark:text-red-300' : 'text-green-700 dark:text-green-300'"
            >
              {{ isBlocked ? t('admin.riskControl.promptRisk.testerBlocked') : t('admin.riskControl.promptRisk.testerPassed') }}
            </span>
            <span class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.promptRisk.level') }}: {{ testResult.decision.level }} · {{ t('admin.riskControl.promptRisk.score') }}: {{ testResult.decision.score.toFixed(2) }}
            </span>
          </div>

          <!-- 观察模式:命中但仍放行 -->
          <p
            v-if="!isBlocked && testResult.decision.action === 'log_notify'"
            class="mt-1.5 text-xs text-amber-600 dark:text-amber-400"
          >
            {{ t('admin.riskControl.promptRisk.testerObserveNote') }}
          </p>

          <!-- 命中词 -->
          <div v-if="testResult.decision.reasons.length" class="mt-2 flex flex-wrap gap-1.5">
            <span
              v-for="(r, i) in testResult.decision.reasons"
              :key="i"
              class="inline-flex rounded bg-white/70 px-2 py-0.5 text-xs text-gray-700 dark:bg-dark-700/70 dark:text-gray-200"
            >
              {{ r.keyword }} ({{ r.level }}/{{ r.source }})
            </span>
          </div>

          <!-- 被拦截时:展示调用方会收到的内容 -->
          <div v-if="isBlocked" class="mt-3 space-y-2 border-t border-red-200 pt-2 dark:border-red-900/50">
            <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.promptRisk.testerWouldReturn') }}</p>
            <p v-if="form.block_message" class="text-sm text-gray-800 dark:text-gray-100">{{ form.block_message }}</p>
            <p v-if="form.rewrite_suggestion" class="whitespace-pre-wrap rounded bg-white/70 p-2 text-xs text-gray-700 dark:bg-dark-700/70 dark:text-gray-200">{{ form.rewrite_suggestion }}</p>
          </div>
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

interface EditableJudge {
  enabled: boolean
  base_url: string
  model: string
  api_key: string // 写入用;留空=沿用已存旧 key
  api_key_configured: boolean
  api_key_masked: string
  timeout_ms: number
  prompt_template: string
  trigger_levels: PromptRiskLevel[]
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
  judge: EditableJudge
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
  judge: {
    enabled: false,
    base_url: '',
    model: '',
    api_key: '',
    api_key_configured: false,
    api_key_masked: '',
    timeout_ms: 4000,
    prompt_template: '',
    trigger_levels: ['high'],
  },
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

const isBlocked = computed(() => testResult.value?.decision.action === 'block')

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
  const j = cfg.judge
  if (j) {
    form.judge = {
      enabled: j.enabled,
      base_url: j.base_url,
      model: j.model,
      api_key: '', // 写入框始终留空,展示用掩码;留空提交=沿用旧 key
      api_key_configured: j.api_key_configured ?? false,
      api_key_masked: j.api_key_masked ?? '',
      timeout_ms: j.timeout_ms || 4000,
      prompt_template: j.prompt_template ?? '',
      trigger_levels: (j.trigger_levels ?? ['high']).slice(),
    }
  }
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
    judge: {
      enabled: form.judge.enabled,
      base_url: form.judge.base_url.trim(),
      model: form.judge.model.trim(),
      api_key: form.judge.api_key, // 留空=后端沿用旧 key
      timeout_ms: Number(form.judge.timeout_ms) || 4000,
      prompt_template: form.judge.prompt_template,
      trigger_levels: form.judge.trigger_levels.slice(),
    },
  }
}

// toggleJudgeTriggerLevel 切换 judge 触发等级(多选)。
function toggleJudgeTriggerLevel(level: PromptRiskLevel) {
  const idx = form.judge.trigger_levels.indexOf(level)
  if (idx >= 0) {
    form.judge.trigger_levels.splice(idx, 1)
  } else {
    form.judge.trigger_levels.push(level)
  }
}

function addKeywordSet() {
  form.keyword_sets.push({ level: 'medium', match_mode: 'word', score: 0.4, keywords_text: '' })
}

function addExemption() {
  form.exemptions.push({ group_ids_text: '', user_ids_text: '', api_key_ids_text: '', max_level: 'medium' })
}

// addExemptionForJudge 追加一条空 API Key 豁免(MaxLevel=low),用户填入 judge 专属 api_key_id
// 即可阻断 judge 回环请求自触发审查(见防递归提示)。
function addExemptionForJudge() {
  form.exemptions.push({ group_ids_text: '', user_ids_text: '', api_key_ids_text: '', max_level: 'low' })
  appStore.showSuccess(t('admin.riskControl.promptRisk.judge.exemptionAdded'))
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
