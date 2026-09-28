import { computed, ref } from 'vue'
import { getGPTQuotaStatus } from '@/api/gptQuotaDisplay'
import { useAuthStore } from '@/stores/auth'

// GPT 额度展示的公开开关存放在独立配置表，不走 public settings。
// 侧栏按 opt-in 处理：状态未加载或读取失败时隐藏菜单，只有后端明确返回 enabled 才显示。
const enabled = ref<boolean | undefined>(undefined)
let loadedAt = 0
let pendingLoad: Promise<boolean> | null = null
// 侧栏随路由重新挂载时按此间隔重读，管理员开启展示后无需整页刷新即可出现菜单。
const VISIBILITY_TTL_MS = 5 * 60 * 1000

async function loadGPTQuotaVisibility(force = false): Promise<boolean> {
  const authStore = useAuthStore()
  if (!authStore.isAuthenticated) {
    // 未登录不缓存结果，登录后侧栏重新挂载时再读取。
    enabled.value = undefined
    return false
  }
  if (enabled.value !== undefined && !force && Date.now() - loadedAt < VISIBILITY_TTL_MS) {
    return enabled.value
  }
  if (pendingLoad && !force) {
    return pendingLoad
  }
  pendingLoad = getGPTQuotaStatus()
    .then((status) => {
      enabled.value = status.enabled === true
      loadedAt = Date.now()
      return enabled.value
    })
    .catch(() => {
      enabled.value = false
      loadedAt = Date.now()
      return false
    })
    .finally(() => {
      pendingLoad = null
    })
  return pendingLoad
}

export function useGPTQuotaVisibility() {
  return {
    gptQuotaMenuVisible: computed(() => enabled.value === true),
    refreshGPTQuotaVisibility: loadGPTQuotaVisibility,
    setGPTQuotaVisibility: (value: boolean) => {
      enabled.value = value
      loadedAt = Date.now()
    },
  }
}
