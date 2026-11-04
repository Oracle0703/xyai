import { apiClient } from '../client'
import type { AdminPermission } from '@/types'

export type OrganizationKey = 'xunyou' | 'wsdashi' | 'other'
// Organizations that may have an organization leader (never "other").
export const ORGANIZATION_LEADER_ORGANIZATIONS = ['xunyou', 'wsdashi'] as const
export type DepartmentOrganizationFilter = OrganizationKey | 'all'

export interface Department {
  id: number
  organization_key: OrganizationKey
  name: string
  status: 'active' | 'inactive'
  sort_order: number
  version: number
  member_count: number
  active_member_count: number
  managers: { id: number; email: string }[]
}

export interface DepartmentMember {
  id: number
  email: string
  username: string
  status: string
  organization: OrganizationKey
  department_id: number | null
  department_name: string
  department_version: number
}

// A /admin/departments/members row; manageable mirrors what assign-members accepts
// for the caller (always true for administrators).
export interface DepartmentMemberRow extends DepartmentMember {
  manageable: boolean
}

export interface DepartmentPage<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export interface DepartmentScope {
  unrestricted: boolean
  organizations: OrganizationKey[]
  departments: Department[]
  catalog_version: string
  default_organization: DepartmentOrganizationFilter
  default_department_id: string
  // Organizations reached through an organization grant: every department plus unassigned members.
  full_organizations?: OrganizationKey[]
  delegable_permissions?: AdminPermission[]
}

export interface DepartmentAccess {
  user_id: number
  department_ids: number[]
  organizations: OrganizationKey[]
  permissions: AdminPermission[]
  version: string
}

export interface DepartmentListQuery {
	user_ids?: string
  organization?: DepartmentOrganizationFilter
  department_id?: string
  status?: string
  q?: string
  page?: number
  page_size?: number
}

export interface DepartmentSaveInput {
  organization_key: OrganizationKey
  name: string
  status: Department['status']
  sort_order: number
  expected_version?: number
}

export interface DepartmentMemberChange {
  user_id: number
  expected_department_id: number | null
  expected_department_version: number
}

export interface ManagedDepartmentMember {
  id: number
  email: string
  username: string
  department_note: string
  status: 'active' | 'disabled'
  disabled_source: 'department_manager' | 'organization_manager' | 'other' | null
  can_enable: boolean
  manageable: boolean
  // null for unassigned members, which only organization leaders see
  department_id: number | null
  department_name: string | null
  department_version: number
}

export interface ManagedMemberUpdateInput {
  // Always sent: null confirms the member is unassigned, omitting it is rejected.
  expected_department_id: number | null
  expected_department_version: number
  username?: string
  department_note?: string
  status?: ManagedDepartmentMember['status']
}

export interface DepartmentAccessInput {
  department_ids: number[]
  report: boolean
  reset_quota: boolean
  manage_members?: boolean
  replace_global_subscriptions: boolean
  expected_version: string
}

export interface OrganizationAccessInput {
  // [] revokes; the field must always be an explicit array.
  organizations: OrganizationKey[]
  expected_version: string
  replace_global_subscriptions: boolean
  replace_department_grants: boolean
}

export interface OrganizationLeaderDepartment {
  id: number
  organization_key: OrganizationKey
  name: string
  status: Department['status']
}

export interface OrganizationLeader {
  id: number
  email: string
  username: string
  status: string
  organization: OrganizationKey
  departments: OrganizationLeaderDepartment[]
  permissions: AdminPermission[]
  managed: boolean
  version?: string
}

export interface OrganizationLeaderCandidate {
  id: number
  email: string
  username: string
  organization: OrganizationKey
  department_id: number | null
  department_name: string | null
  version: string
}

// The final grant set; empty departments with every capability off revokes the leader.
export interface OrganizationLeaderInput {
  department_ids: number[]
  report: boolean
  reset_quota: boolean
  manage_members: boolean
  expected_version: string
}

export const departmentsAPI = {
  async managedScope(signal?: AbortSignal) {
    return (await apiClient.get<DepartmentScope>('/admin/department-members/scope', { signal })).data
  },
  async managedMembers(params: DepartmentListQuery, signal?: AbortSignal) {
    return (await apiClient.get<DepartmentPage<ManagedDepartmentMember>>('/admin/department-members', { params, signal })).data
  },
  async managedCandidates(department_id: number, q: string, signal?: AbortSignal) {
    return (await apiClient.get<DepartmentMember[]>('/admin/department-members/candidates', { params: { department_id, q }, signal })).data
  },
  async managedAssign(department_id: number | null, members: DepartmentMemberChange[]) {
    return (await apiClient.post<{ updated_count: number }>('/admin/department-members/assign', { department_id, members })).data
  },
  async updateManagedMember(userID: number, input: ManagedMemberUpdateInput) {
    return (await apiClient.put<ManagedDepartmentMember>(`/admin/department-members/${userID}`, input)).data
  },
  async list(params: DepartmentListQuery, signal?: AbortSignal) {
    return (await apiClient.get<DepartmentPage<Department>>('/admin/departments', { params, signal })).data
  },
  async save(id: number | null, input: DepartmentSaveInput) {
    return id === null
      ? (await apiClient.post<Department>('/admin/departments', input)).data
      : (await apiClient.put<Department>(`/admin/departments/${id}`, input)).data
  },
  async members(params: DepartmentListQuery, signal?: AbortSignal) {
    return (await apiClient.get<DepartmentPage<DepartmentMemberRow>>('/admin/departments/members', { params, signal })).data
  },
  async assign(department_id: number | null, members: DepartmentMemberChange[]) {
    return (await apiClient.post<{ updated_count: number }>('/admin/departments/assign-members', { department_id, members })).data
  },
  async getAccess(userID: number, signal?: AbortSignal) {
    return (await apiClient.get<DepartmentAccess>(`/admin/users/${userID}/department-scope`, { signal })).data
  },
  async setAccess(userID: number, input: DepartmentAccessInput) {
    return (await apiClient.put<DepartmentAccess>(`/admin/users/${userID}/department-scope`, input)).data
  },
  async getOrganizationAccess(userID: number, signal?: AbortSignal) {
    return (await apiClient.get<DepartmentAccess>(`/admin/users/${userID}/organization-scope`, { signal })).data
  },
  async setOrganizationAccess(userID: number, input: OrganizationAccessInput) {
    return (await apiClient.put<DepartmentAccess>(`/admin/users/${userID}/organization-scope`, input)).data
  },
  async organizationScope(signal?: AbortSignal) {
    return (await apiClient.get<DepartmentScope>('/admin/organization/scope', { signal })).data
  },
  async organizationLeaders(params: DepartmentListQuery, signal?: AbortSignal) {
    return (await apiClient.get<DepartmentPage<OrganizationLeader>>('/admin/organization/leaders', { params, signal })).data
  },
  async organizationLeader(userID: number, signal?: AbortSignal) {
    return (await apiClient.get<OrganizationLeader>(`/admin/organization/leaders/${userID}`, { signal })).data
  },
  async organizationLeaderCandidates(organization: DepartmentOrganizationFilter, q: string, signal?: AbortSignal) {
    return (await apiClient.get<OrganizationLeaderCandidate[]>('/admin/organization/leader-candidates', { params: { organization, q }, signal })).data
  },
  async updateOrganizationLeader(userID: number, input: OrganizationLeaderInput) {
    return (await apiClient.put<DepartmentAccess>(`/admin/organization/leaders/${userID}`, input)).data
  },
  async reportScope(signal?: AbortSignal) {
    return (await apiClient.get<DepartmentScope>('/admin/usage/organization-report/scope', { signal })).data
  },
  async subscriptionScope(signal?: AbortSignal) {
    return (await apiClient.get<DepartmentScope>('/admin/subscriptions/scope', { signal })).data
  },
  async subscriptionUsers(q: string, organization?: string, department_id?: string) {
    return (await apiClient.get<{ id: number; email: string; deleted: boolean }[]>('/admin/subscriptions/search-users', { params: { q, organization, department_id } })).data
  }
}

export default departmentsAPI
