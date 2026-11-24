import { computed, ref } from 'vue'
import { getGPTQuotaStatus } from '@/api/gptQuotaDisplay'
import { useAuthStore } from '@/stores/auth'

// /gpt-quota 页同时承载额度展示与降智测试，两者的公开开关各自存放在独立配置表，不走 public settings。
// 侧栏按 opt-in 处理：状态未加载或读取失败时隐藏菜单；任一开关明确为 true 时显示。
// 两个开关分开记录，写入一方（如关闭额度展示）不会覆盖另一方。
const quotaEnabled = ref<boolean | undefined>(undefined)
const iqTestEnabled = ref<boolean | undefined>(undefined)
// 每个开关的本地写入代次：读取期间若页面已写入较新的值，迟到的状态响应不覆盖它。
let quotaWrites = 0
let iqTestWrites = 0
let loadedAt = 0
let pendingLoad: Promise<boolean> | null = null
// 侧栏随路由重新挂载时按此间隔重读，管理员开启展示后无需整页刷新即可出现菜单。
const VISIBILITY_TTL_MS = 5 * 60 * 1000

const menuVisible = computed(() => quotaEnabled.value === true || iqTestEnabled.value === true)

function isLoaded() {
  return quotaEnabled.value !== undefined && iqTestEnabled.value !== undefined
}

async function loadGPTQuotaVisibility(force = false): Promise<boolean> {
  const authStore = useAuthStore()
  if (!authStore.isAuthenticated) {
    // 未登录不缓存结果，登录后侧栏重新挂载时再读取。
    quotaEnabled.value = undefined
    iqTestEnabled.value = undefined
    return false
  }
  if (isLoaded() && !force && Date.now() - loadedAt < VISIBILITY_TTL_MS) {
    return menuVisible.value
  }
  if (pendingLoad && !force) {
    return pendingLoad
  }
  const quotaWritesAtStart = quotaWrites
  const iqTestWritesAtStart = iqTestWrites
  pendingLoad = getGPTQuotaStatus()
    .then((status) => {
      if (quotaWrites === quotaWritesAtStart) quotaEnabled.value = status.enabled === true
      if (iqTestWrites === iqTestWritesAtStart) iqTestEnabled.value = status.iq_test_enabled === true
      loadedAt = Date.now()
      return menuVisible.value
    })
    .catch(() => {
      // 失败按未开启处理（opt-in）；页面已写入的值保留。
      if (quotaWrites === quotaWritesAtStart) quotaEnabled.value = false
      if (iqTestWrites === iqTestWritesAtStart) iqTestEnabled.value = false
      loadedAt = Date.now()
      return menuVisible.value
    })
    .finally(() => {
      pendingLoad = null
    })
  return pendingLoad
}

function markWritten() {
  if (isLoaded()) loadedAt = Date.now()
}

export function useGPTQuotaVisibility() {
  return {
    gptQuotaMenuVisible: menuVisible,
    refreshGPTQuotaVisibility: loadGPTQuotaVisibility,
    /** 额度展示开关：用户页读取额度后、管理员保存额度展示配置后调用。 */
    setGPTQuotaDisplayVisibility: (value: boolean) => {
      quotaWrites++
      quotaEnabled.value = value
      markWritten()
    },
    /** 降智测试开关：用户页读取降智结果后、保存降智测试配置后调用。 */
    setGPTIQTestVisibility: (value: boolean) => {
      iqTestWrites++
      iqTestEnabled.value = value
      markWritten()
    },
  }
}
