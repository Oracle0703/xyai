<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('admin.requestIntercept.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.requestIntercept.description') }}</p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="loading" @click="loadRules">
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
            {{ t('common.refresh') }}
          </button>
          <button type="button" class="btn btn-primary inline-flex items-center gap-2" @click="createRule">
            <Icon name="plus" size="sm" />
            {{ t('admin.requestIntercept.addRule') }}
          </button>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-3 md:grid-cols-4">
        <div class="rounded-lg border border-gray-100 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex items-start justify-between gap-3">
            <div>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.requestIntercept.globalSwitch') }}</p>
              <p class="mt-2 text-sm font-medium text-gray-900 dark:text-white">{{ globalEnabled ? t('common.enabled') : t('common.disabled') }}</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.requestIntercept.globalSwitchHint') }}</p>
            </div>
            <Toggle :model-value="globalEnabled" :disabled="configSaving" @update:model-value="updateGlobalEnabled" />
          </div>
        </div>
        <div class="rounded-lg border border-gray-100 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.requestIntercept.totalRules') }}</p>
          <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">{{ rules.length }}</p>
        </div>
        <div class="rounded-lg border border-gray-100 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.requestIntercept.enabledRules') }}</p>
          <p class="mt-2 text-2xl font-semibold text-emerald-600 dark:text-emerald-300">{{ enabledCount }}</p>
        </div>
        <div class="rounded-lg border border-gray-100 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.requestIntercept.firstMatch') }}</p>
          <p class="mt-2 text-sm font-medium text-gray-900 dark:text-white">{{ firstEnabledRuleName }}</p>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-6 xl:grid-cols-[minmax(0,1fr)_420px]">
        <div class="card">
          <div class="flex flex-col gap-3 border-b border-gray-100 px-6 py-4 dark:border-dark-700 md:flex-row md:items-center md:justify-between">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.requestIntercept.rules') }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.requestIntercept.rulesHint') }}</p>
            </div>
            <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="saving || rules.length === 0" @click="saveAllRules">
              <Icon name="check" size="sm" />
              {{ saving ? t('common.saving') : t('common.save') }}
            </button>
          </div>

          <div v-if="loading" class="flex items-center justify-center py-16">
            <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
          </div>
          <div v-else-if="rules.length === 0" class="px-6 py-14 text-center">
            <Icon name="shield" size="xl" class="mx-auto text-gray-300 dark:text-dark-500" />
            <p class="mt-3 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.requestIntercept.empty') }}</p>
          </div>
          <div v-else class="divide-y divide-gray-100 dark:divide-dark-700">
            <div
              v-for="rule in sortedRules"
              :key="rule.id"
              class="flex flex-col gap-4 px-6 py-4 transition-colors hover:bg-gray-50 dark:hover:bg-dark-700/50 lg:flex-row lg:items-start lg:justify-between"
              :class="selectedRule?.id === rule.id ? 'bg-primary-50/60 dark:bg-primary-900/10' : ''"
            >
              <button type="button" class="min-w-0 flex-1 text-left" @click="selectRule(rule)">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="font-medium text-gray-900 dark:text-white">{{ rule.name || rule.id }}</span>
                  <span class="rounded-md bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">#{{ rule.priority }}</span>
                  <span class="rounded-md px-2 py-0.5 text-xs font-medium" :class="rule.enabled ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300' : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'">
                    {{ rule.enabled ? t('common.enabled') : t('common.disabled') }}
                  </span>
                  <span class="rounded-md bg-blue-50 px-2 py-0.5 text-xs text-blue-700 dark:bg-blue-900/20 dark:text-blue-300">{{ matchModeLabel(rule.match_mode) }}</span>
                </div>
                <p class="mt-2 line-clamp-2 text-sm text-gray-500 dark:text-gray-400">{{ rule.keywords.join(', ') }}</p>
                <p class="mt-2 line-clamp-1 text-sm text-gray-700 dark:text-gray-300">{{ rule.reply }}</p>
              </button>
              <div class="flex flex-shrink-0 items-center gap-2">
                <Toggle :model-value="rule.enabled" @update:model-value="toggleRule(rule, $event)" />
                <button type="button" class="btn-icon" :title="t('common.edit')" @click="selectRule(rule)">
                  <Icon name="edit" size="sm" />
                </button>
                <button type="button" class="btn-icon text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20" :title="t('common.delete')" @click="removeRule(rule)">
                  <Icon name="trash" size="sm" />
                </button>
              </div>
            </div>
          </div>
        </div>

        <div class="space-y-6">
          <div class="card">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ selectedRule ? t('admin.requestIntercept.editRule') : t('admin.requestIntercept.newRule') }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.requestIntercept.editorHint') }}</p>
            </div>
            <form class="space-y-4 p-6" @submit.prevent="saveCurrentRule">
              <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div>
                  <label class="input-label">{{ t('admin.requestIntercept.ruleId') }}</label>
                  <input v-model.trim="draft.id" class="input" :disabled="!!selectedRule" />
                </div>
                <div>
                  <label class="input-label">{{ t('admin.requestIntercept.priority') }}</label>
                  <input v-model.number="draft.priority" type="number" class="input" />
                </div>
              </div>
              <div>
                <label class="input-label">{{ t('admin.requestIntercept.name') }}</label>
                <input v-model.trim="draft.name" class="input" />
              </div>
              <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div>
                  <label class="input-label">{{ t('admin.requestIntercept.matchMode') }}</label>
                  <Select v-model="draft.match_mode" :options="matchModeOptions" />
                </div>
                <div>
                  <label class="input-label">{{ t('admin.requestIntercept.matchScope') }}</label>
                  <Select v-model="draft.match_scope" :options="matchScopeOptions" />
                </div>
              </div>
              <div>
                <label class="input-label">{{ t('admin.requestIntercept.scopes') }}</label>
                <Select v-model="scopeSelector" :options="scopeOptions" @change="setSingleScope" />
              </div>
              <p class="-mt-2 text-xs text-gray-500 dark:text-gray-400">{{ matchScopeHint }}</p>
              <div>
                <label class="input-label">{{ t('admin.requestIntercept.keywords') }}</label>
                <textarea v-model="keywordsText" class="input min-h-[88px]" :placeholder="t('admin.requestIntercept.keywordsPlaceholder')"></textarea>
              </div>
              <div>
                <label class="input-label">{{ t('admin.requestIntercept.reply') }}</label>
                <textarea v-model="draft.reply" class="input min-h-[96px]"></textarea>
              </div>
              <div>
                <label class="input-label">{{ t('admin.requestIntercept.descriptionLabel') }}</label>
                <input v-model.trim="draft.description" class="input" />
              </div>
              <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700/50">
                <div class="mb-3 flex items-center justify-between">
                  <span class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.requestIntercept.normalization') }}</span>
                  <Toggle v-model="draft.enabled" />
                </div>
                <div class="grid grid-cols-1 gap-3 text-sm sm:grid-cols-2">
                  <label v-for="item in normalizationOptions" :key="item.key" class="flex items-center justify-between gap-3 rounded-md bg-white px-3 py-2 dark:bg-dark-800">
                    <span class="text-gray-700 dark:text-gray-300">{{ item.label }}</span>
                    <Toggle v-model="draft.normalize[item.key]" />
                  </label>
                </div>
              </div>
              <div class="flex justify-end gap-2">
                <button type="button" class="btn btn-secondary" @click="resetDraft">{{ t('common.reset') }}</button>
                <button type="submit" class="btn btn-primary" :disabled="saving">{{ t('common.save') }}</button>
              </div>
            </form>
          </div>

          <div class="card">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.requestIntercept.testTitle') }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.requestIntercept.testHint') }}</p>
            </div>
            <div class="space-y-4 p-6">
              <Select v-model="testEndpoint" :options="endpointOptions" />
              <textarea v-model="testText" class="input min-h-[90px]" :placeholder="t('admin.requestIntercept.testPlaceholder')"></textarea>
              <button type="button" class="btn btn-secondary inline-flex items-center gap-2" :disabled="testing" @click="runTest">
                <Icon name="play" size="sm" />
                {{ testing ? t('common.testing') : t('admin.requestIntercept.runTest') }}
              </button>
              <div v-if="testResult" class="rounded-lg border p-4" :class="testResult.matched ? 'border-emerald-200 bg-emerald-50 dark:border-emerald-900/40 dark:bg-emerald-900/10' : 'border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-700/40'">
                <p class="text-sm font-medium" :class="testResult.matched ? 'text-emerald-700 dark:text-emerald-300' : 'text-gray-700 dark:text-gray-300'">
                  {{ testResult.matched ? t('admin.requestIntercept.testMatched') : t('admin.requestIntercept.testMissed') }}
                </p>
                <p v-if="testResult.decision" class="mt-2 text-sm text-gray-700 dark:text-gray-300">
                  {{ testResult.decision.rule_name || testResult.decision.rule_id }} / {{ testResult.decision.keyword }}
                </p>
                <p v-if="testResult.decision" class="mt-2 text-sm text-gray-600 dark:text-gray-400">{{ testResult.decision.reply }}</p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import { adminAPI, type RequestInterceptMatchScope, type RequestInterceptRule, type RequestInterceptScope, type RequestInterceptTestResponse } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const saving = ref(false)
const configSaving = ref(false)
const testing = ref(false)
const globalEnabled = ref(true)
const rules = ref<RequestInterceptRule[]>([])
const selectedRule = ref<RequestInterceptRule | null>(null)
const keywordsText = ref('')
const scopeSelector = ref<RequestInterceptScope>('all')
const testText = ref(' HI ')
const testEndpoint = ref('/v1/responses')
const testResult = ref<RequestInterceptTestResponse | null>(null)

const defaultNormalize = () => ({
  trim_space: true,
  case_insensitive: true,
  full_width_to_half: true,
  collapse_space: true,
  remove_punctuation: true,
})

const emptyDraft = (): RequestInterceptRule => ({
  id: '',
  name: '',
  enabled: true,
  priority: 100,
  match_mode: 'contains',
  match_scope: 'latest_user',
  keywords: [],
  reply: '你好，我是迅游AI，有什么可以帮助你？',
  scopes: ['all'],
  normalize: defaultNormalize(),
  case_insensitive: true,
  description: '',
})

const draft = reactive<RequestInterceptRule>(emptyDraft())

const matchModeOptions = computed(() => [
  { value: 'contains', label: t('admin.requestIntercept.matchContains') },
  { value: 'exact', label: t('admin.requestIntercept.matchExact') },
  { value: 'regex', label: t('admin.requestIntercept.matchRegex') },
])

const matchScopeOptions = computed(() => [
  { value: 'latest_user', label: t('admin.requestIntercept.matchScopeLatestUser') },
  { value: 'full_context', label: t('admin.requestIntercept.matchScopeFullContext') },
])

const matchScopeHint = computed(() => draft.match_scope === 'full_context'
  ? t('admin.requestIntercept.matchScopeFullContextHint')
  : t('admin.requestIntercept.matchScopeLatestUserHint'))

const scopeOptions = computed(() => [
  { value: 'all', label: t('admin.requestIntercept.scopeAll') },
  { value: 'messages', label: '/v1/messages' },
  { value: 'responses', label: '/v1/responses' },
  { value: 'chat_completions', label: '/v1/chat/completions' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'images', label: 'Images' },
])

const endpointOptions = computed(() => [
  { value: '/v1/responses', label: '/v1/responses' },
  { value: '/v1/messages', label: '/v1/messages' },
  { value: '/v1/chat/completions', label: '/v1/chat/completions' },
  { value: '/v1beta/models/gemini-2.5-pro:generateContent', label: 'Gemini generateContent' },
  { value: '/v1/images/generations', label: 'Images' },
])

const normalizationOptions = computed(() => [
  { key: 'trim_space' as const, label: t('admin.requestIntercept.trimSpace') },
  { key: 'case_insensitive' as const, label: t('admin.requestIntercept.caseInsensitive') },
  { key: 'full_width_to_half' as const, label: t('admin.requestIntercept.fullWidthToHalf') },
  { key: 'collapse_space' as const, label: t('admin.requestIntercept.collapseSpace') },
  { key: 'remove_punctuation' as const, label: t('admin.requestIntercept.removePunctuation') },
])

const sortedRules = computed(() => [...rules.value].sort((a, b) => a.priority === b.priority ? a.id.localeCompare(b.id) : a.priority - b.priority))
const enabledCount = computed(() => rules.value.filter(rule => rule.enabled).length)
const firstEnabledRuleName = computed(() => sortedRules.value.find(rule => rule.enabled)?.name || '-')

function applyDraft(rule: RequestInterceptRule) {
  Object.assign(draft, JSON.parse(JSON.stringify(rule)))
  draft.match_scope = (draft.match_scope || 'latest_user') as RequestInterceptMatchScope
  keywordsText.value = draft.keywords.join('\n')
  scopeSelector.value = (draft.scopes[0] || 'all') as RequestInterceptScope
}

function resetDraft() {
  applyDraft(selectedRule.value || emptyDraft())
}

function createRule() {
  selectedRule.value = null
  applyDraft(emptyDraft())
}

function selectRule(rule: RequestInterceptRule) {
  selectedRule.value = rule
  applyDraft(rule)
}

function setSingleScope(value: string | number | boolean | null) {
  draft.scopes = [String(value || 'all') as RequestInterceptScope]
}

function toggleRule(rule: RequestInterceptRule, enabled: boolean) {
  rule.enabled = enabled
  if (selectedRule.value?.id === rule.id) {
    draft.enabled = enabled
  }
}

function validateDraft(): boolean {
  draft.keywords = keywordsText.value.split(/\r?\n|,/).map(item => item.trim()).filter(Boolean)
  draft.case_insensitive = draft.normalize.case_insensitive
  if (!draft.id.trim()) {
    appStore.showError(t('admin.requestIntercept.idRequired'))
    return false
  }
  if (!draft.name.trim()) {
    appStore.showError(t('admin.requestIntercept.nameRequired'))
    return false
  }
  if (draft.keywords.length === 0) {
    appStore.showError(t('admin.requestIntercept.keywordsRequired'))
    return false
  }
  if (!draft.reply.trim()) {
    appStore.showError(t('admin.requestIntercept.replyRequired'))
    return false
  }
  return true
}

async function loadRules() {
  loading.value = true
  try {
    const [config, result] = await Promise.all([
      adminAPI.requestIntercept.getConfig(),
      adminAPI.requestIntercept.listRules()
    ])
    globalEnabled.value = config.enabled
    rules.value = result.rules || []
    if (rules.value.length > 0) {
      selectRule(sortedRules.value[0])
    } else {
      createRule()
    }
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.requestIntercept.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function updateGlobalEnabled(enabled: boolean) {
  const previous = globalEnabled.value
  globalEnabled.value = enabled
  configSaving.value = true
  try {
    const config = await adminAPI.requestIntercept.updateConfig({ enabled })
    globalEnabled.value = config.enabled
    appStore.showSuccess(t('common.saved'))
  } catch (err) {
    globalEnabled.value = previous
    appStore.showError(extractApiErrorMessage(err, t('admin.requestIntercept.saveConfigFailed')))
  } finally {
    configSaving.value = false
  }
}

async function saveCurrentRule() {
  if (!validateDraft()) return
  saving.value = true
  try {
    const payload = { ...draft, id: undefined, created_at: undefined, updated_at: undefined } as unknown as Omit<RequestInterceptRule, 'id' | 'created_at' | 'updated_at'>
    const saved = await adminAPI.requestIntercept.upsertRule(draft.id, payload)
    const index = rules.value.findIndex(rule => rule.id === saved.id)
    if (index >= 0) {
      rules.value[index] = saved
    } else {
      rules.value.push(saved)
    }
    selectedRule.value = saved
    applyDraft(saved)
    appStore.showSuccess(t('common.saved'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.requestIntercept.saveFailed')))
  } finally {
    saving.value = false
  }
}

async function saveAllRules() {
  saving.value = true
  try {
    const result = await adminAPI.requestIntercept.saveRules(rules.value)
    rules.value = result.rules || []
    appStore.showSuccess(t('common.saved'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.requestIntercept.saveFailed')))
  } finally {
    saving.value = false
  }
}

async function removeRule(rule: RequestInterceptRule) {
  if (!window.confirm(t('admin.requestIntercept.deleteConfirm', { name: rule.name || rule.id }))) return
  try {
    await adminAPI.requestIntercept.deleteRule(rule.id)
    rules.value = rules.value.filter(item => item.id !== rule.id)
    if (selectedRule.value?.id === rule.id) {
      createRule()
    }
    appStore.showSuccess(t('common.deleted'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.requestIntercept.deleteFailed')))
  }
}

async function runTest() {
  testing.value = true
  try {
    testResult.value = await adminAPI.requestIntercept.testRules({
      text: testText.value,
      endpoint: testEndpoint.value,
    })
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.requestIntercept.testFailed')))
  } finally {
    testing.value = false
  }
}

function matchModeLabel(mode: string): string {
  if (mode === 'exact') return t('admin.requestIntercept.matchExact')
  if (mode === 'regex') return t('admin.requestIntercept.matchRegex')
  return t('admin.requestIntercept.matchContains')
}

onMounted(loadRules)
</script>
