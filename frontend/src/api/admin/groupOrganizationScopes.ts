import { apiClient } from '../client'
import { ORGANIZATION_LEADER_ORGANIZATIONS } from './departments'

// Group organization scopes (docs/features/organization-leader-design-cn.md D13):
// organization leaders may only assign subscription groups scoped to one of
// their organizations. Full administrators only.

export type GroupOrganizationKey = (typeof ORGANIZATION_LEADER_ORGANIZATIONS)[number]
export const GROUP_ORGANIZATION_KEYS: readonly GroupOrganizationKey[] = ORGANIZATION_LEADER_ORGANIZATIONS
// Mirrors backend MaxGroupOrganizationScopeBatch.
export const GROUP_ORGANIZATION_SCOPE_BATCH_LIMIT = 500

export interface GroupOrganizationScope {
  group_id: number
  organization_keys: GroupOrganizationKey[]
}

export async function listGroupOrganizationScopes(): Promise<Map<number, GroupOrganizationKey[]>> {
  const { data } = await apiClient.get<GroupOrganizationScope[]>('/admin/groups/organization-scopes')
  return new Map((data ?? []).map(item => [item.group_id, item.organization_keys]))
}

// Replaces the scopes of every listed group; [] clears them.
export async function setGroupOrganizationScopes(groupIds: number[], organizationKeys: GroupOrganizationKey[]): Promise<number> {
  const { data } = await apiClient.put<{ updated: number }>('/admin/groups/organization-scopes', {
    group_ids: groupIds,
    organization_keys: organizationKeys
  })
  return data.updated
}

export function sameOrganizationKeys(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && [...a].sort().join(',') === [...b].sort().join(',')
}
