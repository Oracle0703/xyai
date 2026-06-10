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
  // 同期 usage_logs 计费请求数与归档覆盖率(matched/billed);
  // 概览口径固定含未匹配行, 不随 include_unmatched 勾选变化。
  billed_requests?: number
  archive_coverage?: number
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
  has_input: boolean
  input_truncated: boolean
  quality_score?: number
  // 当前筛选范围内净输入相同的请求数; >1 表示同一输入在 agent 轮次/重试里重发。
  duplicate_count?: number
}

export interface TokenAnalysisRequestInput {
  id: number
  archive_id: string
  event_time: string
  user_id?: number
  content: string
  content_sha256: string
  chars: number
  truncated: boolean
  quality_score?: number
  quality_findings?: Record<string, unknown>
  quality_version: string
  evaluated_at?: string
}

export interface TokenAnalysisIndexRequest {
  start_date?: string
  end_date?: string
  timezone?: string
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

export type TokenAnalysisArchiveFileStatus = 'writing' | 'indexing' | 'deletable' | 'attention' | 'compressed'

export interface TokenAnalysisArchiveFile {
  name: string
  size_bytes: number
  mod_time: string
  indexed_offset: number
  processed_rows: number
  failed_rows: number
  last_error: string
  status: TokenAnalysisArchiveFileStatus
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

async function getRequestInput(archiveId: string): Promise<TokenAnalysisRequestInput> {
  const { data } = await apiClient.get<TokenAnalysisRequestInput>('/admin/token-analysis/requests/input', {
    params: { archive_id: archiveId }
  })
  return data
}

// 异步触发: 后端校验通过即返回 202, 索引在后台执行, 进度轮询 getIndexStatus。
async function triggerIndex(payload: TokenAnalysisIndexRequest): Promise<void> {
  await apiClient.post('/admin/token-analysis/index', payload)
}

async function getIndexStatus(): Promise<TokenAnalysisIndexStatus> {
  const { data } = await apiClient.get<TokenAnalysisIndexStatus>('/admin/token-analysis/index/status')
  return data
}

async function listArchiveFiles(): Promise<TokenAnalysisArchiveFile[]> {
  const { data } = await apiClient.get<TokenAnalysisArchiveFile[]>('/admin/token-analysis/archive-files')
  return data
}

export const tokenAnalysisAPI = {
  getSummary,
  listUsers,
  listProjects,
  listRequests,
  getRequestInput,
  triggerIndex,
  getIndexStatus,
  listArchiveFiles
}

export default tokenAnalysisAPI
