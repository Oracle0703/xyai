/**
 * GPT 账号额度共享展示 API。
 * 用户接口只读已保存快照；管理员接口在 /admin/gpt-quota 下，仅完整管理员可用。
 */
import { apiClient } from './client'

export type GPTQuotaGroupKey = 'xunyou' | 'wsdashi'

export interface GPTQuotaWindow {
  remaining_percent: number
  reset_at?: string
  reset_time_source?: string
}

export interface GPTQuotaSchedule {
  start: string
  end: string
  interval_minutes: number
  timezone: string
}

export interface GPTQuotaUserCard {
  id: number
  display_name: string
  five_hour: GPTQuotaWindow | null
  seven_day: GPTQuotaWindow | null
  sampled_at: string | null
  stale: boolean
}

export interface GPTQuotaUserView {
  enabled: boolean
  server_time: string
  poll_interval_seconds: number
  schedule: GPTQuotaSchedule
  next_scheduled_at: string | null
  in_schedule_window: boolean
  groups: Record<GPTQuotaGroupKey, GPTQuotaUserCard[]>
}

export interface GPTQuotaAdminEntry extends Omit<GPTQuotaUserCard, 'display_name'> {
  account_id: number
  account_name: string
  group: GPTQuotaGroupKey | ''
  display_name: string
  effective_display_name: string
  eligible: boolean
  reason?: string
  warnings?: string[]
  last_attempt_at: string | null
  last_attempt_status: string
  retry_after: string | null
}

export interface GPTQuotaConfig {
  enabled: boolean
  interval_minutes: number
  start_time: string
  end_time: string
  version: number
  updated_at: string
}

export interface GPTQuotaBatchStatus {
  running: boolean
  trigger?: string
  started_at?: string
  finished_at?: string
  total: number
  succeeded: number
  failed: number
  skipped: number
  failure_categories?: Record<string, number>
}

export interface GPTQuotaAdminView {
  config: GPTQuotaConfig
  server_time: string
  schedule: GPTQuotaSchedule
  next_scheduled_at: string | null
  in_schedule_window: boolean
  entries: GPTQuotaAdminEntry[]
  batch: GPTQuotaBatchStatus
}

export interface GPTQuotaCandidate {
  account_id: number
  account_name: string
  group: GPTQuotaGroupKey | ''
  eligible: boolean
  reason?: string
  selected: boolean
}

export interface GPTQuotaCandidatePage {
  items: GPTQuotaCandidate[]
  total: number
  page: number
  page_size: number
}

export interface GPTQuotaSelection {
  account_id: number
  display_name: string
}

export interface GPTQuotaSaveRequest {
  enabled: boolean
  interval_minutes: number
  expected_version: number
  entries: GPTQuotaSelection[]
}

export interface GPTQuotaSaveResult {
  config: GPTQuotaConfig
  warnings?: string[]
}

export interface GPTQuotaRefreshResult {
  entry_id: number
  status: string
  entry?: GPTQuotaAdminEntry
}

export async function getGPTQuotaDisplay(signal?: AbortSignal): Promise<GPTQuotaUserView> {
  const { data } = await apiClient.get<GPTQuotaUserView>('/gpt-quota', { signal })
  return data
}

export async function getGPTQuotaStatus(): Promise<{ enabled: boolean }> {
  const { data } = await apiClient.get<{ enabled: boolean }>('/gpt-quota/status')
  return data
}

export async function getAdminGPTQuota(): Promise<GPTQuotaAdminView> {
  const { data } = await apiClient.get<GPTQuotaAdminView>('/admin/gpt-quota')
  return data
}

export async function listGPTQuotaCandidates(params: { search?: string; page: number; page_size: number }): Promise<GPTQuotaCandidatePage> {
  const { data } = await apiClient.get<GPTQuotaCandidatePage>('/admin/gpt-quota/candidates', { params })
  return data
}

export async function saveGPTQuotaConfig(payload: GPTQuotaSaveRequest): Promise<GPTQuotaSaveResult> {
  const { data } = await apiClient.put<GPTQuotaSaveResult>('/admin/gpt-quota/config', payload)
  return data
}

// 单条刷新同步等待上游（令牌刷新 + 20 秒上游超时），后端上限 45 秒，客户端超时需大于它。
const GPT_QUOTA_REFRESH_TIMEOUT_MS = 60_000

export async function refreshGPTQuotaEntry(entryId: number): Promise<GPTQuotaRefreshResult> {
  const { data } = await apiClient.post<GPTQuotaRefreshResult>('/admin/gpt-quota/refresh', { entry_id: entryId }, { timeout: GPT_QUOTA_REFRESH_TIMEOUT_MS })
  return data
}

export async function refreshAllGPTQuota(): Promise<GPTQuotaBatchStatus> {
  const { data } = await apiClient.post<GPTQuotaBatchStatus>('/admin/gpt-quota/refresh', { all: true })
  return data
}
