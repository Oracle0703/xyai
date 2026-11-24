<template>
  <BaseDialog :show="show" :title="title" width="wide" @close="close">
    <form id="gpt-iq-case-form" class="space-y-5" data-testid="gpt-iq-case-form" @submit.prevent="submit">
      <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
        <div>
          <label class="input-label" for="gpt-iq-case-name">{{ t('gptIQTest.admin.editor.name') }}</label>
          <input id="gpt-iq-case-name" v-model="form.name" class="input" maxlength="40" :placeholder="t('gptIQTest.admin.editor.namePlaceholder')" data-testid="gpt-iq-case-name" />
        </div>
        <div>
          <p class="input-label">{{ t('gptIQTest.admin.editor.kind') }}</p>
          <div class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-1 dark:border-dark-600 dark:bg-dark-900/50" role="group">
            <button
              v-for="kind in KINDS"
              :key="kind"
              type="button"
              class="rounded-md px-3 py-1.5 text-sm transition-colors"
              :class="form.kind === kind ? segmentActiveClass : segmentIdleClass"
              :aria-pressed="form.kind === kind"
              :data-testid="`gpt-iq-case-kind-${kind}`"
              @click="setKind(kind)"
            >
              {{ t(`gptIQTest.kinds.${kind}`) }}
            </button>
          </div>
          <p class="input-hint mt-1">{{ t(`gptIQTest.admin.editor.kindHints.${form.kind}`) }}</p>
        </div>
      </div>

      <div>
        <div class="flex items-center justify-between">
          <label class="input-label" for="gpt-iq-case-prompt">{{ t('gptIQTest.admin.editor.prompt') }}</label>
          <span class="text-xs tabular-nums text-gray-400">{{ t('gptIQTest.admin.editor.promptCount', { count: promptLength }) }}</span>
        </div>
        <!-- 题面原样保存（不 trim、保留换行）。 -->
        <textarea
          id="gpt-iq-case-prompt"
          v-model="form.prompt"
          class="input min-h-[10rem] font-mono text-xs"
          rows="10"
          maxlength="8000"
          spellcheck="false"
          data-testid="gpt-iq-case-prompt"
        />
        <p class="input-hint mt-1">{{ t('gptIQTest.admin.editor.promptHint') }}</p>
      </div>

      <div class="grid grid-cols-1 gap-4 md:grid-cols-4">
        <div class="md:col-span-2">
          <label class="input-label" for="gpt-iq-case-model">{{ t('gptIQTest.admin.editor.model') }}</label>
          <input id="gpt-iq-case-model" v-model.trim="form.model" class="input font-mono text-sm" :placeholder="t('gptIQTest.admin.editor.modelPlaceholder')" data-testid="gpt-iq-case-model" />
        </div>
        <div>
          <label class="input-label" for="gpt-iq-case-effort">{{ t('gptIQTest.admin.editor.effort') }}</label>
          <select id="gpt-iq-case-effort" v-model="form.reasoning_effort" class="input" data-testid="gpt-iq-case-effort">
            <option v-for="effort in EFFORTS" :key="effort" :value="effort">{{ effort || t('gptIQTest.admin.cases.effortDefault') }}</option>
          </select>
        </div>
        <div>
          <label class="input-label" for="gpt-iq-case-runs">{{ t('gptIQTest.admin.editor.runs') }}</label>
          <input id="gpt-iq-case-runs" v-model.number="form.runs" type="number" min="1" max="5" step="1" class="input" data-testid="gpt-iq-case-runs" />
          <p class="input-hint mt-1">{{ t('gptIQTest.admin.editor.runsHint') }}</p>
        </div>
      </div>

      <div v-if="form.kind === 'graded'" class="grid grid-cols-1 gap-4 md:grid-cols-2">
        <div>
          <label class="input-label" for="gpt-iq-case-scoring">{{ t('gptIQTest.admin.editor.scoring') }}</label>
          <select id="gpt-iq-case-scoring" v-model="form.scoring_mode" class="input" data-testid="gpt-iq-case-scoring">
            <option v-for="mode in GRADED_SCORING_MODES" :key="mode" :value="mode">{{ t(`gptIQTest.admin.scoringModes.${mode}`) }}</option>
          </select>
          <p class="input-hint mt-1">{{ t(`gptIQTest.admin.scoringModeHints.${form.scoring_mode}`) }}</p>
        </div>
        <div>
          <label class="input-label" for="gpt-iq-case-expected">{{ t('gptIQTest.admin.editor.expected') }}</label>
          <input id="gpt-iq-case-expected" v-model="form.scoring_expected" class="input font-mono text-sm" :placeholder="t('gptIQTest.admin.editor.expectedPlaceholder')" data-testid="gpt-iq-case-expected" />
        </div>
      </div>

      <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
        <div>
          <label class="input-label" for="gpt-iq-case-timeout">{{ t('gptIQTest.admin.editor.timeout') }}</label>
          <input id="gpt-iq-case-timeout" v-model.number="form.timeout_seconds" type="number" min="0" max="600" step="1" class="input" data-testid="gpt-iq-case-timeout" />
          <p class="input-hint mt-1">{{ t('gptIQTest.admin.editor.timeoutHint') }}</p>
        </div>
        <div>
          <label class="input-label" for="gpt-iq-case-sort">{{ t('gptIQTest.admin.editor.sortOrder') }}</label>
          <input id="gpt-iq-case-sort" v-model.number="form.sort_order" type="number" step="1" class="input" />
          <p class="input-hint mt-1">{{ t('gptIQTest.admin.editor.sortOrderHint') }}</p>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-3 md:grid-cols-3">
        <label class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-dark-600">
          <span>{{ t('gptIQTest.admin.editor.enabled') }}</span>
          <Toggle v-model="form.enabled" />
        </label>
        <label class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-dark-600">
          <span>{{ t('gptIQTest.admin.editor.publicPrompt') }}</span>
          <Toggle v-model="form.public_prompt" />
        </label>
        <label class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-dark-600">
          <span>{{ t('gptIQTest.admin.editor.publicAnswer') }}</span>
          <Toggle v-model="form.public_answer" />
        </label>
      </div>

      <p v-if="editing" class="input-hint">{{ t('gptIQTest.admin.editor.revisionNote', { revision: editing.revision }) }}</p>
      <p v-if="error" class="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300" role="alert" data-testid="gpt-iq-case-error">{{ error }}</p>
    </form>

    <template #footer>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn btn-secondary btn-sm" :disabled="saving" @click="close">{{ t('gptIQTest.admin.editor.cancel') }}</button>
        <button type="submit" form="gpt-iq-case-form" class="btn btn-primary btn-sm" :disabled="saving" data-testid="gpt-iq-case-save">{{ t('gptIQTest.admin.editor.save') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import {
  createGPTIQTestCase,
  updateGPTIQTestCase,
  type GPTIQAdminCase,
  type GPTIQCaseInput,
  type GPTIQCaseTemplate,
  type GPTIQReasoningEffort,
  type GPTIQScoringMode,
} from '@/api/admin/gptIQTest'
import type { GPTIQCaseKind } from '@/api/gptIQTest'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = withDefaults(defineProps<{
  show: boolean
  /** 编辑已有测试项；与 template 互斥。 */
  editing?: GPTIQAdminCase | null
  /** 从模板添加：只是表单预填，保存后与普通测试项无区别。 */
  template?: GPTIQCaseTemplate | null
  defaultSortOrder?: number
}>(), { editing: null, template: null, defaultSortOrder: 10 })

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved', value: GPTIQAdminCase, created: boolean): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const KINDS: GPTIQCaseKind[] = ['chat', 'graded']
const EFFORTS: GPTIQReasoningEffort[] = ['', 'none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max', 'ultra']
const GRADED_SCORING_MODES: GPTIQScoringMode[] = ['final_line', 'number_any', 'contains', 'regex']
const NAME_MAX = 40
const PROMPT_MAX = 8000
const segmentActiveClass = 'bg-white font-medium text-primary-700 shadow-sm dark:bg-dark-700 dark:text-primary-300'
const segmentIdleClass = 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'

function emptyInput(): GPTIQCaseInput {
  return {
    name: '',
    enabled: true,
    sort_order: props.defaultSortOrder,
    public_prompt: true,
    public_answer: true,
    kind: 'graded',
    prompt: '',
    model: '',
    reasoning_effort: '',
    runs: 1,
    timeout_seconds: 0,
    scoring_mode: 'final_line',
    scoring_expected: '',
  }
}

function pickInput(source: GPTIQCaseInput): GPTIQCaseInput {
  return {
    name: source.name,
    enabled: source.enabled,
    sort_order: source.sort_order,
    public_prompt: source.public_prompt,
    public_answer: source.public_answer,
    kind: source.kind,
    prompt: source.prompt,
    model: source.model,
    reasoning_effort: (source.reasoning_effort ?? '') as GPTIQReasoningEffort,
    runs: source.runs,
    timeout_seconds: source.timeout_seconds,
    scoring_mode: source.scoring_mode,
    scoring_expected: source.scoring_expected ?? '',
  }
}

const form = reactive<GPTIQCaseInput>(emptyInput())
const saving = ref(false)
const error = ref('')

const title = computed(() => {
  if (props.editing) return t('gptIQTest.admin.editor.editTitle')
  if (props.template) return t('gptIQTest.admin.editor.templateTitle', { name: props.template.input.name })
  return t('gptIQTest.admin.editor.createTitle')
})

// 按 Unicode 字符计数，与后端按 rune 校验一致。
const promptLength = computed(() => [...form.prompt].length)

watch(
  () => props.show,
  (open) => {
    if (!open) return
    const source = props.editing ?? props.template?.input ?? null
    Object.assign(form, source ? pickInput(source) : emptyInput())
    if (form.kind === 'chat') form.scoring_mode = 'none'
    error.value = ''
  },
  { immediate: true },
)

function setKind(kind: GPTIQCaseKind) {
  form.kind = kind
  if (kind === 'chat') {
    form.scoring_mode = 'none'
    form.scoring_expected = ''
  } else if (form.scoring_mode === 'none') {
    form.scoring_mode = 'final_line'
  }
}

function validate(): string {
  const nameLength = [...form.name.trim()].length
  if (nameLength === 0 || nameLength > NAME_MAX) return t('gptIQTest.admin.editor.nameRequired')
  if (!form.prompt.trim() || promptLength.value > PROMPT_MAX) return t('gptIQTest.admin.editor.promptRequired')
  if (!form.model.trim()) return t('gptIQTest.admin.editor.modelRequired')
  if (!Number.isInteger(form.runs) || form.runs < 1 || form.runs > 5) return t('gptIQTest.admin.editor.runsInvalid')
  const timeout = form.timeout_seconds
  if (!Number.isInteger(timeout) || (timeout !== 0 && (timeout < 30 || timeout > 600))) return t('gptIQTest.admin.editor.timeoutInvalid')
  if (form.kind === 'graded' && !form.scoring_expected.trim()) return t('gptIQTest.admin.editor.expectedRequired')
  return ''
}

function payload(): GPTIQCaseInput {
  const sortOrder = Number.isInteger(form.sort_order) ? form.sort_order : 0
  return {
    ...pickInput(form),
    name: form.name.trim(),
    model: form.model.trim(),
    sort_order: sortOrder,
    // chat 不判分；判分期望值原样提交（正则可能依赖空白）。
    scoring_mode: form.kind === 'chat' ? 'none' : form.scoring_mode,
    scoring_expected: form.kind === 'chat' ? '' : form.scoring_expected,
  }
}

async function submit() {
  if (saving.value) return
  error.value = validate()
  if (error.value) return
  saving.value = true
  try {
    const editing = props.editing
    const saved = editing ? await updateGPTIQTestCase(editing.id, payload(), editing.version) : await createGPTIQTestCase(payload())
    appStore.showSuccess(t(editing ? 'gptIQTest.admin.editor.updated' : 'gptIQTest.admin.editor.created'))
    emit('saved', saved, !editing)
  } catch (err) {
    error.value = extractApiErrorMessage(err, t('common.error'), {
      GPT_IQ_TEST_CASE_CONFLICT: t('gptIQTest.admin.cases.conflict'),
      GPT_IQ_TEST_CASE_NOT_FOUND: t('gptIQTest.admin.cases.notFound'),
    })
  } finally {
    saving.value = false
  }
}

function close() {
  if (!saving.value) emit('close')
}
</script>
