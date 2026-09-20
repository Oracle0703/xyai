import { apiClient } from '../client'
import type { AdminPermission } from '@/types'

export type OrganizationKey = 'xunyou' | 'wsdashi' | 'other'
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
}

export interface DepartmentAccess {
  user_id: number
  department_ids: number[]
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

export interface DepartmentAccessInput {
  department_ids: number[]
  report: boolean
  reset_quota: boolean
  replace_global_subscriptions: boolean
  expected_version: string
}

export const departmentsAPI = {
  async list(params: DepartmentListQuery, signal?: AbortSignal) {
    return (await apiClient.get<DepartmentPage<Department>>('/admin/departments', { params, signal })).data
  },
  async save(id: number | null, input: DepartmentSaveInput) {
    return id === null
      ? (await apiClient.post<Department>('/admin/departments', input)).data
      : (await apiClient.put<Department>(`/admin/departments/${id}`, input)).data
  },
  async members(params: DepartmentListQuery, signal?: AbortSignal) {
    return (await apiClient.get<DepartmentPage<DepartmentMember>>('/admin/departments/members', { params, signal })).data
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
