<template>
  <AppLayout>
    <div class="space-y-6 pb-8">
      <p v-if="loadError" class="rounded-lg bg-red-50 px-4 py-2 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
        {{ t('gptQuota.loadFailed') }}
      </p>

      <template v-if="view">
        <div v-if="!view.enabled" class="card p-8 text-center text-sm text-gray-500 dark:text-gray-400">
          {{ t('gptQuota.disabled') }}
        </div>
        <template v-else>
          <!-- 与 Dashboard 一致的统计卡：总量、7 天平均、偏低数量、下次更新。 -->
          <div class="grid grid-cols-2 gap-4 lg:grid-cols-4" data-testid="gpt-quota-summary">
            <MetricTile
              icon="users"
              :label="t('gptQuota.summary.accounts')"
              :value="String(allCards.length)"
              :hint="t('gptQuota.summary.accountsHint', { xunyou: view.groups.xunyou.length, wsdashi: view.groups.wsdashi.length })"
            />
            <MetricTile
              icon="chartBar"
              tone="success"
              :label="t('gptQuota.summary.sevenDayAverage')"
              :value="formatGPTQuotaPercent(sevenDay.average)"
              :hint="sevenDay.count ? t('gptQuota.summary.sevenDayAverageHint', { count: sevenDay.count }) : t('gptQuota.noData')"
            />
            <MetricTile
              icon="exclamationTriangle"
              :tone="lowCount > 0 ? 'warning' : 'gray'"
              :label="t('gptQuota.summary.low')"
              :value="String(lowCount)"
              :hint="t('gptQuota.summary.lowHint', { count: criticalCount })"
              test-id="gpt-quota-summary-low"
            />
            <MetricTile
              icon="clock"
              tone="gray"
              :label="t('gptQuota.summary.nextUpdate')"
              :value="formatGPTQuotaTileTime(view.next_scheduled_at, new Date(now))"
              :hint="t('gptQuota.summary.scheduleHint', { start: view.schedule.start, end: view.schedule.end, interval: view.schedule.interval_minutes })"
            />
          </div>

          <p v-if="!view.in_schedule_window" class="flex items-center gap-2 rounded-lg border border-amber-200 bg-amber-50 px-4 py-2.5 text-sm text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/20 dark:text-amber-200">
            <Icon name="infoCircle" size="sm" class="shrink-0" />
            {{ t('gptQuota.outsideWindow') }}
          </p>

          <!-- 桌面左迅游、右速宝；窄屏单列，迅游在前。 -->
          <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
            <GPTQuotaColumn group-key="xunyou" :title="t('gptQuota.groups.xunyou')" :items="view.groups.xunyou" :now="now" />
            <GPTQuotaColumn group-key="wsdashi" :title="t('gptQuota.groups.wsdashi')" :items="view.groups.wsdashi" :now="now" />
          </div>
        </template>
      </template>
      <div v-else-if="!loadError" class="grid grid-cols-2 gap-4 lg:grid-cols-4" aria-hidden="true">
        <div v-for="index in 4" :key="index" class="card h-[76px] animate-pulse bg-gray-50 dark:bg-dark-800" />
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import MetricTile from '@/components/common/MetricTile.vue'
import Icon from '@/components/icons/Icon.vue'
import GPTQuotaColumn from '@/components/user/GPTQuotaColumn.vue'
import { getGPTQuotaDisplay, type GPTQuotaUserView } from '@/api/gptQuotaDisplay'
import { useGPTQuotaVisibility } from '@/composables/useGPTQuotaVisibility'
import { averageSevenDayRemaining, effectiveGPTQuotaRemaining, formatGPTQuotaPercent, formatGPTQuotaTileTime } from '@/utils/gptQuota'

// 页面只读已保存快照：进入时读取一次，之后按固定间隔轮询；隐藏标签页暂停，恢复可见时间隔已到才补读。
const GPT_QUOTA_POLL_INTERVAL_MS = 15 * 60 * 1000
const COUNTDOWN_TICK_MS = 30 * 1000

const { t } = useI18n()
const { setGPTQuotaVisibility } = useGPTQuotaVisibility()

const view = ref<GPTQuotaUserView | null>(null)
const loadError = ref(false)
const now = ref(Date.now())
let lastLoadedAt = 0
let pollTimer: number | undefined
let tickTimer: number | undefined
let controller: AbortController | null = null
let disposed = false

const allCards = computed(() => (view.value ? [...view.value.groups.xunyou, ...view.value.groups.wsdashi] : []))
const sampledCards = computed(() => allCards.value.filter((card) => card.sampled_at))
const sevenDay = computed(() => averageSevenDayRemaining(sampledCards.value))
const remainingValues = computed(() => sampledCards.value.map(effectiveGPTQuotaRemaining).filter((value): value is number => value !== null))
const lowCount = computed(() => remainingValues.value.filter((value) => value <= 30).length)
const criticalCount = computed(() => remainingValues.value.filter((value) => value <= 10).length)

async function load() {
  controller?.abort()
  const current = new AbortController()
  controller = current
  try {
    const result = await getGPTQuotaDisplay(current.signal)
    view.value = result
    loadError.value = false
    setGPTQuotaVisibility(result.enabled)
  } catch {
    if (current.signal.aborted) return
    // 读取失败保留当前画面，按正常周期重试，不做高频重试。
    loadError.value = true
  } finally {
    if (controller === current) controller = null
    lastLoadedAt = Date.now()
    scheduleNextPoll()
  }
}

// 每次读取后重新计时，恢复可见补读后不会紧接着再被定时器读一次。
function scheduleNextPoll() {
  window.clearTimeout(pollTimer)
  if (disposed) return
  pollTimer = window.setTimeout(onPollTick, GPT_QUOTA_POLL_INTERVAL_MS)
}

// 隐藏时不读也不重新计时，由 visibilitychange 在间隔已到时补读一次。
function onPollTick() {
  if (!document.hidden) void load()
}

function onVisibilityChange() {
  if (!document.hidden && Date.now() - lastLoadedAt >= GPT_QUOTA_POLL_INTERVAL_MS) void load()
}

onMounted(() => {
  void load()
  tickTimer = window.setInterval(() => {
    now.value = Date.now()
  }, COUNTDOWN_TICK_MS)
  document.addEventListener('visibilitychange', onVisibilityChange)
})

onBeforeUnmount(() => {
  disposed = true
  window.clearTimeout(pollTimer)
  window.clearInterval(tickTimer)
  document.removeEventListener('visibilitychange', onVisibilityChange)
  controller?.abort()
})
</script>
