import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth'

/**
 * 账号管理页的操作边界。完整管理员可执行全部操作；持有 `admin.accounts_operator`
 * 的子管理员只能查看、新增、测试连接和重新授权。后端白名单是安全边界，这里隐藏
 * 入口是为了不触发越权请求（`ADMIN_PERMISSION_DENIED` 会把用户重定向出当前页）。
 */
export function useAccountOperatorAccess() {
  const authStore = useAuthStore()
  const canManageAccounts = computed(() => authStore.isAdmin)
  return { canManageAccounts }
}
