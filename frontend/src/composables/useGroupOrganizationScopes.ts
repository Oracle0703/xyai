import { ref } from 'vue'
import { listGroupOrganizationScopes, type GroupOrganizationKey } from '@/api/admin/groupOrganizationScopes'

export type GroupOrganizationScopeStatus = 'idle' | 'loading' | 'ready' | 'error'

/**
 * Loads group organization scopes (docs/features/organization-leader-design-cn.md
 * D13, K24). Only the latest request is accepted: an older response or error
 * that arrives late is ignored. `generation` is the request number whose result
 * is in `scopes`; a consumer that captured `mark()` treats the scopes as read
 * after that moment only once `generation > mark`. A failed load keeps the old
 * map for display context but sets `status` to `error`, so it is no longer a
 * verified snapshot.
 */
export function useGroupOrganizationScopes(enabled: () => boolean) {
  const scopes = ref(new Map<number, GroupOrganizationKey[]>())
  const status = ref<GroupOrganizationScopeStatus>('idle')
  const generation = ref(0)
  let sequence = 0

  async function load(): Promise<boolean> {
    if (!enabled()) return false
    const request = ++sequence
    status.value = 'loading'
    try {
      const result = await listGroupOrganizationScopes()
      if (request !== sequence) return false
      scopes.value = result
      generation.value = request
      status.value = 'ready'
      return true
    } catch (error) {
      if (request !== sequence) return false
      status.value = 'error'
      console.error('Error loading group organization scopes:', error)
      return false
    }
  }

  const mark = () => sequence

  return { scopes, status, generation, load, mark }
}
