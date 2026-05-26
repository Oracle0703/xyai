import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export interface PromptMetricsFilters {
  from?: string
  to?: string
  user_id?: number
  api_key_id?: number
  group_id?: number
  project?: string
  branch?: string
  client?: string
  model?: string
  endpoint?: string
  hash?: string
  min_quality?: number
  max_quality?: number
  only_low_quality?: boolean
  page?: number
  page_size?: number
}

export interface PromptMetricsOverview {
  total_events: number
  active_users: number
  low_quality: number
  truncated: number
  pending_analysis: number
  total_tokens: number
  total_cost: number
  average_quality: number
}

export interface PromptMetricsTrendPoint {
  bucket: string
  events: number
  users: number
  tokens: number
  cost: number
  avg_quality: number
  low_quality: number
  pending_count: number
}

export interface PromptMetricsRankItem {
  key: string
  label: string
  events: number
  users: number
  tokens: number
  cost: number
  avg_quality: number
}

export interface PromptMetricsAnalysis {
  prompt_event_id: number
  summary: string
  quality_score: number
  clarity_score: number
  context_score: number
  actionability_score: number
  constraint_score: number
  risk_score: number
  categories: string[]
  improvement_suggestions: string[]
  analyzer_model: string
  analyzed_at: string
}

export interface PromptMetricsEvent {
  id: number
  request_id: string
  user_id?: number
  api_key_id?: number
  group_id?: number
  model: string
  requested_model: string
  endpoint: string
  source_protocol: string
  prompt_text?: string
  prompt_excerpt: string
  prompt_hash: string
  prompt_chars: number
  prompt_segments: number
  prompt_tokens_estimated: number
  project_name: string
  git_branch: string
  client_name: string
  client_version: string
  user_agent: string
  ip_address: string
  truncated: boolean
  analysis_status: string
  created_at: string
  input_tokens?: number
  output_tokens?: number
  cache_creation_tokens?: number
  cache_read_tokens?: number
  total_tokens?: number
  actual_cost?: number
  user_email?: string
  api_key_name?: string
  group_name?: string
  analysis?: PromptMetricsAnalysis
}

async function getOverview(params: PromptMetricsFilters): Promise<PromptMetricsOverview> {
  const { data } = await apiClient.get<PromptMetricsOverview>('/admin/prompt-metrics/overview', { params })
  return data
}

async function getTrend(params: PromptMetricsFilters & { bucket?: 'day' | 'hour' }): Promise<PromptMetricsTrendPoint[]> {
  const { data } = await apiClient.get<PromptMetricsTrendPoint[]>('/admin/prompt-metrics/trend', { params })
  return data
}

async function getRank(params: PromptMetricsFilters & { dimension?: string; limit?: number }): Promise<PromptMetricsRankItem[]> {
  const { data } = await apiClient.get<PromptMetricsRankItem[]>('/admin/prompt-metrics/rank', { params })
  return data
}

async function listEvents(params: PromptMetricsFilters): Promise<PaginatedResponse<PromptMetricsEvent>> {
  const { data } = await apiClient.get<PaginatedResponse<PromptMetricsEvent>>('/admin/prompt-metrics/events', { params })
  return data
}

async function getEvent(id: number): Promise<PromptMetricsEvent> {
  const { data } = await apiClient.get<PromptMetricsEvent>(`/admin/prompt-metrics/events/${id}`)
  return data
}

async function reanalyze(id: number): Promise<PromptMetricsAnalysis> {
  const { data } = await apiClient.post<PromptMetricsAnalysis>(`/admin/prompt-metrics/events/${id}/reanalyze`)
  return data
}

export const promptMetricsAPI = {
  getOverview,
  getTrend,
  getRank,
  listEvents,
  getEvent,
  reanalyze
}

export default promptMetricsAPI
