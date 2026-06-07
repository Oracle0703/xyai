import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export interface TokenAnalysisQueryParams {
  start_date?: string
  end_date?: string
  timezone?: string
  user_id?: number
  api_key_id?: number
  account_id?: number
  group_id?: number
  model?: string
  endpoint?: string
  risk_min?: number
  risk_reason?: string
  project?: string
  include_unmatched?: boolean
  page?: number
  page_size?: number
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export interface TokenAnalysisRiskReason {
  code: string
  message: string
  score: number
  metrics?: Record<string, unknown>
}

export interface TokenAnalysisSummary {
  total_requests: number
  matched_requests?: number
  unmatched_requests?: number
  total_input_tokens?: number
  total_output_tokens?: number
  cache_read_tokens: number
  cache_creation_tokens: number
  total_tokens: number
  total_cost?: number
  total_actual_cost: number
  cache_hit_rate: number
  risky_requests: number
  risky_cost: number
  unmatched_rate?: number
  risk_request_rate?: number
  risk_reasons?: TokenAnalysisRiskReasonSummary[]
}

export interface TokenAnalysisRiskReasonSummary {
  code: string
  count: number
}

export interface TokenAnalysisUserUsage {
  user_id?: number
  user_email: string
  api_key_id?: number
  api_key_name: string
  request_count: number
  risky_request_count: number
  total_tokens: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_creation_tokens: number
  actual_cost: number
  risky_cost: number
  cache_hit_rate: number
  risk_ratio: number
  last_event_time?: string
}

export interface TokenAnalysisProjectUsage {
  project: string
  user_id?: number
  user_email: string
  request_count: number
  matched_request_count: number
  total_tokens: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_creation_tokens: number
  actual_cost: number
  last_event_time?: string
}

export interface TokenAnalysisRequestItem {
  id: number
  archive_id: string
  usage_log_id?: number
  match_confidence: number
  event_time: string
  user_id?: number
  user_email: string
  api_key_id?: number
  api_key_name: string
  account_id?: number
  group_id?: number
  model: string
  endpoint: string
  method: string
  request_body_size: number
  request_body_truncated: boolean
  message_count: number
  system_chars: number
  user_chars: number
  last_user_preview: string
  tools_count: number
  image_count: number
  summary_json?: Record<string, unknown>
  client_workdir?: string
  client_project?: string
  client_branch?: string
  attribution_source?: string
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_creation_tokens: number
  total_tokens: number
  actual_cost: number
  risk_score: number
  risk_reasons: TokenAnalysisRiskReason[]
}

export interface TokenAnalysisIndexRequest {
  start_date?: string
  end_date?: string
  timezone?: string
}

export interface TokenAnalysisIndexResult {
  indexed_rows: number
  skipped_rows?: number
  failed_rows: number
  files?: number
}

export interface TokenAnalysisIndexState {
  source_file: string
  last_offset: number
  last_archive_id: string
  processed_rows: number
  failed_rows: number
  last_error: string
  started_at?: string
  finished_at?: string
  updated_at: string
}

export interface TokenAnalysisIndexStatus {
  running: boolean
  processed_rows: number
  failed_rows: number
  files: TokenAnalysisIndexState[]
  last_error?: string
  updated_at?: string
}

async function getSummary(params: TokenAnalysisQueryParams): Promise<TokenAnalysisSummary> {
  const { data } = await apiClient.get<TokenAnalysisSummary>('/admin/token-analysis/summary', { params })
  return data
}

async function listUsers(
  params: TokenAnalysisQueryParams,
  options?: { signal?: AbortSignal }
): Promise<PaginatedResponse<TokenAnalysisUserUsage>> {
  const { data } = await apiClient.get<PaginatedResponse<TokenAnalysisUserUsage>>('/admin/token-analysis/users', {
    params,
    signal: options?.signal
  })
  return data
}

async function listProjects(
  params: TokenAnalysisQueryParams,
  options?: { signal?: AbortSignal }
): Promise<PaginatedResponse<TokenAnalysisProjectUsage>> {
  const { data } = await apiClient.get<PaginatedResponse<TokenAnalysisProjectUsage>>(
    '/admin/token-analysis/projects',
    {
      params,
      signal: options?.signal
    }
  )
  return data
}

async function listRequests(
  params: TokenAnalysisQueryParams,
  options?: { signal?: AbortSignal }
): Promise<PaginatedResponse<TokenAnalysisRequestItem>> {
  const { data } = await apiClient.get<PaginatedResponse<TokenAnalysisRequestItem>>('/admin/token-analysis/requests', {
    params,
    signal: options?.signal
  })
  return data
}

async function triggerIndex(payload: TokenAnalysisIndexRequest): Promise<TokenAnalysisIndexResult> {
  const { data } = await apiClient.post<TokenAnalysisIndexResult>('/admin/token-analysis/index', payload)
  return data
}

async function getIndexStatus(): Promise<TokenAnalysisIndexStatus> {
  const { data } = await apiClient.get<TokenAnalysisIndexStatus>('/admin/token-analysis/index/status')
  return data
}

export const tokenAnalysisAPI = {
  getSummary,
  listUsers,
  listProjects,
  listRequests,
  triggerIndex,
  getIndexStatus
}

export default tokenAnalysisAPI
