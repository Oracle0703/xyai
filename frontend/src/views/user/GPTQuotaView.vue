<template>
  <AppLayout>
    <div class="space-y-6">
      <header class="space-y-1">
        <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('gptQuota.title') }}</h1>
        <p class="text-sm text-gray-600 dark:text-dark-300">{{ t('gptQuota.description') }}</p>
        <p v-if="view?.enabled" class="text-xs text-gray-500 dark:text-dark-400">
          {{ t('gptQuota.schedule', { start: view.schedule.start, end: view.schedule.end, timezone: view.schedule.timezone, interval: view.schedule.interval_minutes }) }}
          <template v-if="view.next_scheduled_at">
            · {{ t('gptQuota.nextScheduled', { time: formatDateTimeToMinute(view.next_scheduled_at) }) }}
          </template>
        </p>
      </header>

      <p v-if="loadError" class="rounded-lg bg-red-50 px-4 py-2 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
        {{ t('gptQuota.loadFailed') }}
      </p>

      <template v-if="view">
        <div v-if="!view.enabled" class="card p-8 text-center text-sm text-gray-500 dark:text-dark-400">
          {{ t('gptQuota.disabled') }}
        </div>
        <template v-else>
          <p v-if="!view.in_schedule_window" class="rounded-lg bg-gray-50 px-4 py-2 text-sm text-gray-600 dark:bg-dark-800 dark:text-dark-300">
            {{ t('gptQuota.outsideWindow') }}
          </p>
          <!-- 桌面左迅游、右速宝；窄屏单列，迅游在前。 -->
          <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
            <GPTQuotaColumn group-key="xunyou" :title="t('gptQuota.groups.xunyou')" :items="view.groups.xunyou" :now="now" />
            <GPTQuotaColumn group-key="wsdashi" :title="t('gptQuota.groups.wsdashi')" :items="view.groups.wsdashi" :now="now" />
          </div>
        </template>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import GPTQuotaColumn from '@/components/user/GPTQuotaColumn.vue'
import { getGPTQuotaDisplay, type GPTQuotaUserView } from '@/api/gptQuotaDisplay'
import { useGPTQuotaVisibility } from '@/composables/useGPTQuotaVisibility'
import { formatDateTimeToMinute } from '@/utils/format'

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
