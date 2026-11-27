import { apiClient } from './client'
import type { PaginatedResponse } from '@/types'

export type SelfResetReason = 'SUBSCRIPTION_INACTIVE' | 'GROUP_NOT_ALLOWED' | 'GROUP_DISABLED' | 'ONE_TIME_QUOTA' | 'NO_DAILY_LIMIT' | 'POLICY_DISABLED' | 'DAILY_LIMIT_REACHED' | 'NO_USAGE' | 'ROLLOUT_DISABLED'
export interface SelfResetItem {
  subscription_id: number
  used_count: number
  remaining_count: number
  can_reset: boolean
  disabled_reason: SelfResetReason | null
}
export interface SelfResetStatus {
  organization: string
  daily_limit: number
  quota_date: string
  server_now: string
  next_reset_at: string
  subscriptions: SelfResetItem[]
}
export interface SelfResetResult {
  subscription_id: number
  quota_date: string
  daily_limit: number
  used_count: number
  remaining_count: number
  next_reset_at: string
}
export interface SelfResetPolicy {
  daily_limit_by_organization: Record<'xunyou' | 'wsdashi' | 'other', number>
  rollout: 'off' | 'admin' | 'all'
}
export async function getSelfResetStatus(): Promise<SelfResetStatus> {
  return (await apiClient.get<SelfResetStatus>('/subscriptions/self-reset-status')).data
}
export async function resetSubscriptionDaily(id: number, date: string, key: string): Promise<SelfResetResult> {
  return (await apiClient.post<SelfResetResult>(`/subscriptions/${id}/reset-daily`, { quota_date: date }, { headers: { 'Idempotency-Key': key } })).data
}
export interface SelfResetGroup {
  id: number
  name: string
  platform: string
  status: string
  subscription_type: string
  daily_limit_usd: number | null
  enabled: boolean
}
export type SelfResetOrganization = 'xunyou' | 'wsdashi' | 'other'
export type SelfResetGroupsByOrganization = Record<SelfResetOrganization, SelfResetGroup[]>
export interface SelfResetGroupChanges {
  enable?: number[]
  disable?: number[]
}
export interface SelfResetConfig {
  revision: number
  policy: SelfResetPolicy | null
  groups_by_organization: SelfResetGroupsByOrganization | null
}
export interface SelfResetConfigUpdate {
  expected_revision: number
  policy?: SelfResetPolicy
  changes_by_organization?: Partial<Record<SelfResetOrganization, SelfResetGroupChanges>>
}
const configPath = '/admin/subscriptions/self-reset-config'
export async function getSelfResetConfig(): Promise<SelfResetConfig> {
  return (await apiClient.get<SelfResetConfig>(configPath, { headers: { 'Cache-Control': 'no-cache' } })).data
}
export async function saveSelfResetConfig(update: SelfResetConfigUpdate): Promise<{ revision: number }> {
  return (await apiClient.put<{ revision: number }>(configPath, update)).data
}
export async function closeSelfResetRollout(): Promise<{ revision: number }> {
  return (await apiClient.post<{ revision: number }>(configPath + '/close')).data
}
export interface SelfResetEventGroup { id: number; name: string }
export async function getSelfResetEventGroups(): Promise<{ event_groups: SelfResetEventGroup[] }> {
  return (await apiClient.get<{ event_groups: SelfResetEventGroup[] }>('/admin/subscriptions/self-reset-event-groups')).data
}
export interface SelfResetEvent {
  id: number
  subscription_id: number
  user_id: number
  user_email: string
  group_id: number
  group_name: string
  organization: 'xunyou' | 'wsdashi' | 'other'
  quota_date: string
  used_count: number
  daily_limit: number
  daily_usage_usd_before: number
  created_at: string
}
export interface SelfResetEventFilters {
  email?: string
  organization?: string
  group_id?: number
  start_date?: string
  end_date?: string
}
export async function listSelfResetEvents(page: number, pageSize: number, filters: SelfResetEventFilters): Promise<PaginatedResponse<SelfResetEvent>> {
  return (await apiClient.get<PaginatedResponse<SelfResetEvent>>('/admin/subscriptions/self-reset-events', { params: { page, page_size: pageSize, ...filters } })).data
}
