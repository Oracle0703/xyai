<template>
  <template v-if="request">
    <div class="fixed inset-0 z-40 bg-gray-900/30 backdrop-blur-[1px]" @click="emit('close')"></div>
    <aside
      class="fixed right-0 top-0 z-50 flex h-full w-full max-w-2xl flex-col border-l border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-900"
      role="dialog"
      aria-modal="true"
      :aria-label="t('admin.tokenAnalysis.requestDetail')"
      data-testid="token-analysis-request-drawer"
    >
      <header class="flex items-start justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700">
        <div class="min-w-0">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.requestDetail') }}</h2>
          <div class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500 dark:text-dark-400">
            <span class="tabular-nums">{{ formatTimeCN(request.event_time) }}</span>
            <span class="break-all font-mono">{{ request.archive_id }}</span>
          </div>
        </div>
        <button type="button" class="btn btn-ghost btn-sm shrink-0" :aria-label="t('common.close')" @click="emit('close')">
          <Icon name="x" size="sm" />
        </button>
      </header>

      <div class="flex-1 space-y-5 overflow-y-auto px-5 py-4">
        <div class="flex flex-wrap items-center gap-2 text-xs">
          <span class="ta-tag bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200">
            <Icon name="user" size="xs" class="mr-1" />{{ request.user_email || '-' }}
          </span>
          <span v-if="request.api_key_name" class="ta-tag bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200">
            <Icon name="key" size="xs" class="mr-1" />{{ request.api_key_name }}
          </span>
          <span class="ta-tag bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200">
            <Icon name="cpu" size="xs" class="mr-1" />{{ request.model || '-' }}
          </span>
          <span class="ta-tag bg-gray-100 font-mono text-gray-500 dark:bg-dark-700 dark:text-gray-400">{{ request.endpoint }}</span>
        </div>

        <section data-testid="request-origin-detail">
          <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.tokenAnalysis.requestOrigin.label') }}
            <span class="ta-tag" :class="origin.className" :title="origin.tooltip">{{ origin.label }}</span>
            <span class="text-xs font-normal text-gray-500">{{ t('admin.tokenAnalysis.requestOrigin.inferred') }}</span>
          </h3>
          <dl class="space-y-2 rounded-lg border border-gray-100 p-3 text-sm dark:border-dark-700">
            <div>
              <dt class="text-gray-500 dark:text-dark-400">{{ t('admin.tokenAnalysis.requestOrigin.reason') }}</dt>
              <dd class="mt-1 text-gray-700 dark:text-gray-200">{{ origin.reason }}</dd>
            </div>
            <div class="flex justify-between gap-3">
              <dt class="text-gray-500 dark:text-dark-400">{{ t('admin.tokenAnalysis.requestOrigin.confidence') }}</dt>
              <dd>{{ origin.confidence }}</dd>
            </div>
            <div class="flex justify-between gap-3">
              <dt class="text-gray-500 dark:text-dark-400">{{ t('admin.tokenAnalysis.requestOrigin.version') }}</dt>
              <dd>{{ origin.version }}</dd>
            </div>
          </dl>
          <p class="mt-1.5 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.tokenAnalysis.requestOrigin.humanHint') }}</p>
        </section>

        <section data-testid="request-usage-link-detail">
          <div class="grid grid-cols-2 gap-2 sm:grid-cols-5">
            <div v-for="stat in usageStats" :key="stat.label" class="rounded-lg border border-gray-100 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-800">
              <div class="truncate text-xs text-gray-500 dark:text-dark-400">{{ stat.label }}</div>
              <div class="mt-0.5 truncate text-sm font-semibold tabular-nums" :class="link.linked ? 'text-gray-900 dark:text-white' : 'text-gray-400 dark:text-dark-400'" :title="stat.title">{{ stat.value }}</div>
            </div>
          </div>
          <p v-if="!link.linked" class="mt-1.5 flex items-center gap-2 text-xs text-gray-500 dark:text-dark-400">
            <span class="ta-tag" :class="link.className">{{ link.label }}</span>
            <span>{{ link.tooltip }}</span>
          </p>
        </section>

        <section>
          <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.tokenAnalysis.risk') }}
            <span v-if="request.risk_score > 0" class="inline-flex min-w-8 justify-center rounded-full px-1.5 py-0.5 text-xs font-semibold" :class="riskScoreClass(request.risk_score)">
              {{ request.risk_score }}
            </span>
          </h3>
          <ul v-if="(request.risk_reasons || []).length" class="space-y-2">
            <li
              v-for="reason in request.risk_reasons"
              :key="reason.code"
              class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 dark:border-amber-900/40 dark:bg-amber-900/10"
            >
              <div class="flex items-center justify-between gap-2 text-sm font-medium text-amber-800 dark:text-amber-200">
                <span>{{ riskLabel(reason.code) }}</span>
                <span v-if="reason.score" class="text-xs tabular-nums">+{{ reason.score }}</span>
              </div>
              <p class="mt-0.5 text-xs leading-relaxed text-amber-700/90 dark:text-amber-300/80">{{ riskDesc(reason.code) || reason.message }}</p>
            </li>
          </ul>
          <p v-else class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.tokenAnalysis.noRiskFlags') }}</p>
        </section>

        <section v-if="extractedUserRequest">
          <h3 class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.userRequest') }}</h3>
          <div class="max-h-72 overflow-y-auto whitespace-pre-wrap break-words rounded-lg border border-blue-200 bg-blue-50 p-3 text-sm text-gray-800 dark:border-blue-900/50 dark:bg-blue-950/30 dark:text-gray-100">{{ extractedUserRequest }}</div>
          <div class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.tokenAnalysis.userRequestHint') }}</div>
        </section>

        <section>
          <h3 class="mb-2 flex flex-wrap items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
            <span>{{ t('admin.tokenAnalysis.inputFull') }}</span>
            <span v-if="input?.truncated" class="rounded bg-amber-100 px-1.5 py-0.5 text-xs font-normal text-amber-700 dark:bg-amber-900/30 dark:text-amber-300">
              {{ t('admin.tokenAnalysis.inputTruncatedNote') }}
            </span>
            <span v-if="input" class="text-xs font-normal text-gray-500">{{ formatInteger(input.chars) }} {{ t('admin.tokenAnalysis.chars') }}</span>
          </h3>
          <div v-if="inputLoading" class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800">
            {{ t('common.loading') }}
          </div>
          <div v-else-if="input" class="max-h-96 overflow-y-auto whitespace-pre-wrap break-words rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200">{{ input.content }}</div>
          <div v-else class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200">
            {{ request.last_user_preview || t('admin.tokenAnalysis.noInputStored') }}
          </div>
          <div
            v-if="input?.content_sha256"
            class="mt-1.5 break-all font-mono text-xs text-gray-400 dark:text-gray-500"
            :title="t('admin.tokenAnalysis.contentHashHint')"
          >
            {{ t('admin.tokenAnalysis.contentHash') }}: {{ input.content_sha256 }}
          </div>
        </section>

        <section>
          <h3 class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.tokenAnalysis.quality') }}</h3>
          <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm dark:border-dark-700 dark:bg-dark-800">
            <template v-if="input && input.quality_score !== undefined && input.quality_score !== null">
              <div class="flex items-center gap-2">
                <span class="inline-flex min-w-10 justify-center rounded-full bg-blue-100 px-2 py-0.5 text-xs font-semibold text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">
                  {{ input.quality_score }}
                </span>
                <span v-if="input.quality_version" class="text-xs text-gray-500">{{ input.quality_version }}</span>
                <span v-if="input.evaluated_at" class="text-xs text-gray-500">{{ formatTimeCN(input.evaluated_at) }}</span>
              </div>
              <pre v-if="input.quality_findings && Object.keys(input.quality_findings).length > 0" class="mt-2 max-h-48 overflow-y-auto whitespace-pre-wrap break-words text-xs text-gray-600 dark:text-gray-300">{{ JSON.stringify(input.quality_findings, null, 2) }}</pre>
            </template>
            <span v-else class="text-gray-500">{{ t('admin.tokenAnalysis.qualityNotEvaluated') }}</span>
          </div>
        </section>

        <section v-for="group in detailGroups" :key="group.title">
          <h3 class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">{{ group.title }}</h3>
          <dl class="grid grid-cols-1 gap-x-6 gap-y-2 rounded-lg border border-gray-100 p-3 text-sm dark:border-dark-700 sm:grid-cols-2">
            <div v-for="field in group.fields" :key="field.label" class="flex min-w-0 items-baseline justify-between gap-3" :class="field.wide ? 'sm:col-span-2' : ''">
              <dt class="shrink-0 text-gray-500 dark:text-dark-400">{{ field.label }}</dt>
              <dd class="min-w-0 break-all text-right font-medium text-gray-900 dark:text-gray-100">{{ field.value }}</dd>
            </div>
          </dl>
        </section>
      </div>
    </aside>
  </template>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'

import type { TokenAnalysisRequestInput, TokenAnalysisRequestItem } from '@/api/admin/tokenAnalysis'
import Icon from '@/components/icons/Icon.vue'
import { formatBytes } from '@/utils/format'
import { useRequestOrigin } from './requestOrigin'
import { useUsageLink } from './usageLink'
import {
  formatCost,
  formatInteger,
  formatTimeCN,
  formatTokenMetric,
  riskScoreClass,
  useTokenAnalysisRisk
} from './tokenAnalysisFormat'
import './tokenAnalysis.css'

const props = defineProps<{
  request: TokenAnalysisRequestItem | null
  input: TokenAnalysisRequestInput | null
  inputLoading: boolean
}>()

const emit = defineEmits<{ close: [] }>()

const { t } = useI18n()
const { riskLabel, riskDesc } = useTokenAnalysisRisk()
const { requestOrigin } = useRequestOrigin()
const origin = computed(() => requestOrigin(props.request?.summary_json))
const { usageLink } = useUsageLink()
const link = computed(() => usageLink(props.request ?? {}))

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && props.request) emit('close')
}

onMounted(() => document.addEventListener('keydown', onKeydown))
onUnmounted(() => document.removeEventListener('keydown', onKeydown))

const extractedUserRequest = computed(() => {
  const match = (props.input?.content || '').match(/<userRequest>([\s\S]*?)<\/userRequest>/i)
  return match?.[1]?.trim() || ''
})

const usageStats = computed(() => {
  const item = props.request
  if (!item) return []
  const stats = [
    { label: t('admin.tokenAnalysis.summary.inputTokens'), value: formatTokenMetric(item.input_tokens), title: formatInteger(item.input_tokens) },
    { label: t('admin.tokenAnalysis.summary.outputTokens'), value: formatTokenMetric(item.output_tokens), title: formatInteger(item.output_tokens) },
    { label: t('admin.tokenAnalysis.summary.cacheRead'), value: formatTokenMetric(item.cache_read_tokens), title: formatInteger(item.cache_read_tokens) },
    { label: t('admin.tokenAnalysis.summary.cacheCreation'), value: formatTokenMetric(item.cache_creation_tokens), title: formatInteger(item.cache_creation_tokens) },
    { label: t('admin.tokenAnalysis.cost'), value: formatCost(item.actual_cost), title: formatCost(item.actual_cost) }
  ]
  if (link.value.linked) return stats
  return stats.map((stat) => ({ ...stat, value: '—', title: t('admin.tokenAnalysis.usageLink.unknownHint') }))
})

const detailGroups = computed(() => {
  const item = props.request
  if (!item) return []
  return [
    {
      title: t('admin.tokenAnalysis.detail.structure'),
      fields: [
        { label: t('admin.tokenAnalysis.detail.messages'), value: formatInteger(item.message_count) },
        { label: t('admin.tokenAnalysis.detail.tools'), value: formatInteger(item.tools_count) },
        { label: t('admin.tokenAnalysis.detail.systemChars'), value: formatInteger(item.system_chars) },
        { label: t('admin.tokenAnalysis.detail.userChars'), value: formatInteger(item.user_chars) },
        { label: t('admin.tokenAnalysis.detail.images'), value: formatInteger(item.image_count) },
        { label: t('admin.tokenAnalysis.detail.bodySize'), value: formatBytes(item.request_body_size ?? 0) },
        { label: t('admin.tokenAnalysis.bodyTruncated'), value: item.request_body_truncated ? t('common.yes') : t('common.no') },
        { label: t('admin.tokenAnalysis.duplicateCount'), value: formatInteger(item.duplicate_count ?? 1) }
      ]
    },
    {
      title: t('admin.tokenAnalysis.usageLink.title'),
      fields: [
        { label: t('admin.tokenAnalysis.usageLink.status'), value: link.value.label },
        { label: t('admin.tokenAnalysis.usageLink.method'), value: link.value.method },
        { label: t('admin.tokenAnalysis.detail.matchConfidence'), value: link.value.linked ? String(item.match_confidence ?? '-') : '—' },
        { label: t('admin.tokenAnalysis.usageLink.usageLogId'), value: item.usage_log_id ? String(item.usage_log_id) : '—' },
        { label: t('admin.tokenAnalysis.usageLink.completedAt'), value: item.completed_at ? formatTimeCN(item.completed_at) : '—' },
        { label: t('admin.tokenAnalysis.usageLink.responseStatus'), value: item.response_status ? String(item.response_status) : '—' }
      ]
    },
    {
      title: t('admin.tokenAnalysis.detail.attribution'),
      fields: [
        { label: t('admin.tokenAnalysis.project'), value: item.client_project || '-' },
        { label: t('admin.tokenAnalysis.branch'), value: item.client_branch || '-' },
        { label: t('admin.tokenAnalysis.attributionSource'), value: item.attribution_source || '-' },
        { label: t('admin.tokenAnalysis.workdir'), value: item.client_workdir || '-', wide: true }
      ]
    }
  ]
})
</script>
