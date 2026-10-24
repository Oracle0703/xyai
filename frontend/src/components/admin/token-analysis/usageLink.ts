import { useI18n } from 'vue-i18n'
import type {
  TokenAnalysisRequestItem,
  TokenAnalysisUsageLinkStatus,
  TokenAnalysisUsageMatchMethod
} from '@/api/admin/tokenAnalysis'

const statuses: TokenAnalysisUsageLinkStatus[] = ['linked', 'unlinked', 'failed', 'not_billable']
const methods: TokenAnalysisUsageMatchMethod[] = ['request_id', 'timing', 'legacy_window']

type UsageLinkFields = Pick<TokenAnalysisRequestItem, 'usage_link_status' | 'usage_log_id' | 'usage_match_method'>

// 旧接口没有 usage_link_status 时只能按 usage_log_id 判断; 未关联行的 Token/费用
// 由服务端补 0, 只能显示为未知。
export function usageLinkStatus(item: UsageLinkFields): TokenAnalysisUsageLinkStatus {
  const status = item.usage_link_status
  if (status && statuses.includes(status)) return status
  return item.usage_log_id ? 'linked' : 'unlinked'
}

export function useUsageLink() {
  const { t } = useI18n()

  function usageLink(item: UsageLinkFields) {
    const status = usageLinkStatus(item)
    const method = item.usage_match_method && methods.includes(item.usage_match_method) ? item.usage_match_method : null
    return {
      status,
      linked: status === 'linked',
      label: t(`admin.tokenAnalysis.usageLink.statuses.${status}`),
      tooltip: t(`admin.tokenAnalysis.usageLink.statusHints.${status}`),
      method: status === 'linked' && method ? t(`admin.tokenAnalysis.usageLink.methods.${method}`) : '—',
      className: status === 'linked'
        ? 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
        : status === 'unlinked'
          ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
          : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'
    }
  }

  return { usageLink }
}
