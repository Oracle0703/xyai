import { apiClient } from '../client'

export type RequestInterceptMatchMode = 'exact' | 'contains' | 'regex'
export type RequestInterceptMatchScope = 'latest_user' | 'full_context'
export type RequestInterceptScope = 'all' | 'messages' | 'responses' | 'chat_completions' | 'gemini' | 'images'

export interface RequestInterceptNormalization {
  trim_space: boolean
  case_insensitive: boolean
  full_width_to_half: boolean
  collapse_space: boolean
  remove_punctuation: boolean
}

export interface RequestInterceptRule {
  id: string
  name: string
  enabled: boolean
  priority: number
  match_mode: RequestInterceptMatchMode
  match_scope: RequestInterceptMatchScope
  keywords: string[]
  reply: string
  scopes: RequestInterceptScope[]
  normalize: RequestInterceptNormalization
  case_insensitive: boolean
  description?: string
  created_at?: string
  updated_at?: string
}

export interface RequestInterceptRulesResponse {
  rules: RequestInterceptRule[]
}

export interface RequestInterceptConfig {
  enabled: boolean
}

export interface RequestInterceptTestPayload {
  text: string
  endpoint: string
}

export interface RequestInterceptDecision {
  rule_id: string
  rule_name: string
  keyword: string
  match_mode: RequestInterceptMatchMode
  match_scope?: RequestInterceptMatchScope
  reply: string
  endpoint: string
}

export interface RequestInterceptTestResponse {
  matched: boolean
  decision: RequestInterceptDecision | null
}

export async function listRules(): Promise<RequestInterceptRulesResponse> {
  const { data } = await apiClient.get<RequestInterceptRulesResponse>('/admin/request-intercept/rules')
  return data
}

export async function getConfig(): Promise<RequestInterceptConfig> {
  const { data } = await apiClient.get<RequestInterceptConfig>('/admin/request-intercept/config')
  return data
}

export async function updateConfig(config: RequestInterceptConfig): Promise<RequestInterceptConfig> {
  const { data } = await apiClient.put<RequestInterceptConfig>('/admin/request-intercept/config', config)
  return data
}

export async function saveRules(rules: RequestInterceptRule[]): Promise<RequestInterceptRulesResponse> {
  const { data } = await apiClient.put<RequestInterceptRulesResponse>('/admin/request-intercept/rules', { rules })
  return data
}

export async function upsertRule(id: string, rule: Omit<RequestInterceptRule, 'id' | 'created_at' | 'updated_at'>): Promise<RequestInterceptRule> {
  const { data } = await apiClient.put<RequestInterceptRule>(`/admin/request-intercept/rules/${encodeURIComponent(id)}`, rule)
  return data
}

export async function deleteRule(id: string): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/request-intercept/rules/${encodeURIComponent(id)}`)
  return data
}

export async function testRules(payload: RequestInterceptTestPayload): Promise<RequestInterceptTestResponse> {
  const { data } = await apiClient.post<RequestInterceptTestResponse>('/admin/request-intercept/test', payload)
  return data
}

export const requestInterceptAPI = {
  getConfig,
  updateConfig,
  listRules,
  saveRules,
  upsertRule,
  deleteRule,
  testRules,
}

export default requestInterceptAPI
