<template>
  <section v-if="view?.enabled || loadError" class="space-y-4" data-testid="gpt-iq-section">
    <header class="flex flex-wrap items-end justify-between gap-3">
      <div class="min-w-0">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('gptIQTest.title') }}</h2>
        <p class="mt-0.5 text-sm text-gray-500 dark:text-gray-400">{{ t('gptIQTest.sectionHint') }}</p>
      </div>
      <div v-if="view?.enabled" class="shrink-0 text-right text-xs text-gray-500 dark:text-gray-400" data-testid="gpt-iq-schedule">
        <p class="flex items-center justify-end gap-1.5">
          <Icon name="clock" size="xs" />
          {{ view.schedule.enabled ? t('gptIQTest.schedule', { time: view.schedule.run_time }) : t('gptIQTest.scheduleOff') }}
        </p>
        <p v-if="view.next_run_at" class="mt-0.5 tabular-nums">{{ t('gptIQTest.nextRun', { time: formatGPTIQTime(view.next_run_at, new Date(now)) }) }}</p>
      </div>
    </header>

    <p v-if="loadError" class="rounded-lg bg-red-50 px-4 py-2 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300" data-testid="gpt-iq-load-error">
      {{ t('gptIQTest.loadFailed') }}
    </p>

    <template v-if="view?.enabled">
      <p class="flex items-start gap-2 text-xs text-gray-500 dark:text-gray-400">
        <Icon name="infoCircle" size="xs" class="mt-0.5 shrink-0" />
        {{ t('gptIQTest.thresholdsHint', { accuracy: view.thresholds.min_accuracy_percent, ratio: view.thresholds.reasoning_ratio_percent, min: view.thresholds.min_graded_runs }) }}
      </p>
      <!-- 与额度区相同：桌面左迅游、右速宝；窄屏单列，迅游在前。 -->
      <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
        <GPTIQTestColumn group-key="xunyou" :title="t('gptQuota.groups.xunyou')" :items="view.groups.xunyou" :cases="view.cases" :today="today" :now="now" :current-revisions="currentRevisions" @open="openCard" />
        <GPTIQTestColumn group-key="wsdashi" :title="t('gptQuota.groups.wsdashi')" :items="view.groups.wsdashi" :cases="view.cases" :today="today" :now="now" :current-revisions="currentRevisions" @open="openCard" />
      </div>
    </template>

    <GPTIQTestRunsDrawer :card="selectedCard" :cases="view?.cases ?? []" :today="today" :current-revisions="currentRevisions" @close="selectedId = null" />
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import GPTIQTestColumn from '@/components/user/GPTIQTestColumn.vue'
import GPTIQTestRunsDrawer from '@/components/user/GPTIQTestRunsDrawer.vue'
import type { GPTIQUserCard, GPTIQUserView } from '@/api/gptIQTest'
import { formatGPTIQTime, gptIQCurrentRevisionIds, gptIQTodayInZone } from '@/utils/gptIQTest'

// 只负责展示；读取与 15 分钟轮询由 GPTQuotaView 统一调度，与额度区各发各的请求。
const props = defineProps<{
  view: GPTIQUserView | null
  loadError: boolean
  now: number
}>()

const { t } = useI18n()

const selectedId = ref<number | null>(null)

const allCards = computed(() => (props.view?.enabled ? [...props.view.groups.xunyou, ...props.view.groups.wsdashi] : []))
// 轮询刷新后用新数据渲染抽屉头部；账号已不在列表中时关闭抽屉。
const selectedCard = computed<GPTIQUserCard | null>(() => (selectedId.value === null ? null : allCards.value.find((card) => card.id === selectedId.value) ?? null))
const today = computed(() => gptIQTodayInZone(props.view?.server_time, props.view?.schedule.timezone))
// "第 N 版"标记的基准：接口给出的当前版本，否则取全部卡片中最新执行的版本。
const currentRevisions = computed(() => gptIQCurrentRevisionIds(props.view?.cases ?? [], allCards.value.flatMap((card) => card.cases)))

watch(selectedCard, (card) => {
  if (!card) selectedId.value = null
})

function openCard(card: GPTIQUserCard) {
  selectedId.value = card.id
}
</script>
