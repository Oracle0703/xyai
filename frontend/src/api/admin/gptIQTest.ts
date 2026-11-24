/**
 * GPT 降智测试配置页 API（/admin/gpt-iq-test）。
 * 完整管理员、持有 admin.gpt_iq_test 的子管理员与组织负责人（admin.organization_manage）可用；
 * 组织负责人的手动测试范围由服务端按组织授权重读判定。本页不得调用 /admin/gpt-quota（负责人无权）。
 */
import { apiClient } from '../client'
import type { GPTQuotaGroupKey } from '../gptQuotaDisplay'
import type { GPTIQCaseKind, GPTIQRunStatus, GPTIQTrigger } from '../gptIQTest'

export type GPTIQReasoningEffort = '' | 'none' | 'minimal' | 'low' | 'medium' | 'high' | 'xhigh' | 'max' | 'ultra'
export type GPTIQScoringMode = 'none' | 'final_line' | 'number_any' | 'contains' | 'regex'
export type GPTIQBatchStatus = 'running' | 'finished' | 'cancelled' | 'aborted' | 'abandoned'
export type GPTIQTemplateKey = 'hi' | 'candy'
/** 批次异常结束的原因：超过 90 分钟上限、服务停止、结果写入失败；正常为空。 */
export type GPTIQBatchFailureReason = '' | 'timeout' | 'stopping' | 'persist_failed'

export interface GPTIQAdminConfig {
  enabled: boolean
  schedule_enabled: boolean
  run_time: string
  last_run_date?: string
  min_accuracy_percent: number
  min_graded_runs: number
  reasoning_ratio_percent: number
  min_five_hour_percent: number
  min_seven_day_percent: number
  daily_call_budget: number
  retention_days: number
  version: number
  updated_by?: number
  updated_at: string
}

export interface GPTIQConfigUpdate {
  enabled: boolean
  schedule_enabled: boolean
  run_time: string
  min_accuracy_percent: number
  min_graded_runs: number
  reasoning_ratio_percent: number
  min_five_hour_percent: number
  min_seven_day_percent: number
  daily_call_budget: number
  retention_days: number
  expected_version: number
}

export interface GPTIQCaseInput {
  name: string
  enabled: boolean
  sort_order: number
  public_prompt: boolean
  public_answer: boolean
  kind: GPTIQCaseKind
  prompt: string
  model: string
  reasoning_effort: GPTIQReasoningEffort
  runs: number
  /** 30–600；0 表示按类型默认值。 */
  timeout_seconds: number
  scoring_mode: GPTIQScoringMode
  scoring_expected: string
}

export interface GPTIQAdminCase extends GPTIQCaseInput {
  id: number
  version: number
  revision_id: number
  revision: number
  prompt_sha256: string
  updated_at: string
}

export interface GPTIQCaseTemplate {
  key: GPTIQTemplateKey | string
  input: GPTIQCaseInput
}

export interface GPTIQAdminEntry {
  id: number
  display_name: string
  group: GPTQuotaGroupKey
  runnable: boolean
  backoff_until?: string
}

export interface GPTIQActorScope {
  full_scope: boolean
  groups: string[]
  /**
   * 是否可修改设置与测试项：完整管理员与持有 admin.gpt_iq_test 的子管理员为 true；
   * 仅有组织负责人身份时为 false（只读设置，可手动测试与查看批次）。
   */
  can_configure: boolean
}

export interface GPTIQBatch {
  id: number
  trigger: GPTIQTrigger
  run_date: string
  /** 组织负责人只在本人发起时看到发起人，否则为 null。 */
  triggered_by?: number | null
  scope_groups: string[] | null
  entry_ids: number[] | null
  case_ids: number[] | null
  status: GPTIQBatchStatus
  failure_reason: GPTIQBatchFailureReason
  cancel_requested: boolean
  heartbeat_at: string
  total_calls: number
  started_at: string
  finished_at?: string
}

/**
 * 组织负责人看到的批次元数据已按其范围投影（entry_ids、total_calls 与计数只含本组织条目，
 * triggered_by 仅在本人发起时返回）。
 */
export interface GPTIQBatchSummary extends GPTIQBatch {
  completed: number
  ok: number
  /** 不含 abandoned（中断计入 interrupted）。 */
  errors: number
  skipped: number
  /** cancelled + abandoned。 */
  interrupted: number
  graded: number
  correct: number
}

export interface GPTIQAdminView {
  config: GPTIQAdminConfig
  server_time: string
  timezone: string
  next_run_at: string | null
  scope: GPTIQActorScope
  cases: GPTIQAdminCase[]
  templates: GPTIQCaseTemplate[]
  entries: GPTIQAdminEntry[]
  batches: GPTIQBatchSummary[]
  running_batch_id: number | null
  dispatched_today: number
}

export interface GPTIQBatchResult {
  id: number
  batch_id: number
  run_date: string
  trigger: GPTIQTrigger
  entry_id: number
  case_id: number
  revision_id: number
  run_index: number
  attempts: number
  status: GPTIQRunStatus
  reason?: string
  /** 脱敏错误摘要（≤500 字符），仅管理页可见。 */
  error_summary?: string
  http_status?: number
  correct: boolean | null
  model_requested: string
  model_responded?: string
  reasoning_effort?: string
  /** ≤2000 字符。 */
  answer_text: string
  answer_truncated: boolean
  input_tokens: number | null
  cached_tokens: number | null
  output_tokens: number | null
  reasoning_tokens: number | null
  ttft_ms: number | null
  duration_ms: number | null
  started_at: string
  finished_at?: string
  entry_name: string
  group: GPTQuotaGroupKey | ''
  tps: number | null
}

export interface GPTIQBatchDetail {
  batch: GPTIQBatch
  results: GPTIQBatchResult[]
  /** 操作者范围内的计划调用数（与按范围投影后的 batch.total_calls 一致）；缺失时用 batch.total_calls。 */
  visible_total_calls?: number
}

export interface GPTIQRunRequest {
  entry_ids?: number[]
  case_ids?: number[]
}

export interface GPTIQRejudgeResult {
  checked: number
  updated: number
  skipped_truncated: number
}

const BASE = '/admin/gpt-iq-test'

export async function getAdminGPTIQTest(signal?: AbortSignal): Promise<GPTIQAdminView> {
  const { data } = await apiClient.get<GPTIQAdminView>(BASE, { signal })
  return data
}

export async function updateGPTIQTestConfig(payload: GPTIQConfigUpdate): Promise<GPTIQAdminConfig> {
  const { data } = await apiClient.put<GPTIQAdminConfig>(`${BASE}/config`, payload)
  return data
}

export async function createGPTIQTestCase(payload: GPTIQCaseInput): Promise<GPTIQAdminCase> {
  const { data } = await apiClient.post<GPTIQAdminCase>(`${BASE}/cases`, payload)
  return data
}

export async function updateGPTIQTestCase(id: number, payload: GPTIQCaseInput, expectedVersion: number): Promise<GPTIQAdminCase> {
  const { data } = await apiClient.put<GPTIQAdminCase>(`${BASE}/cases/${id}`, { ...payload, expected_version: expectedVersion })
  return data
}

export async function deleteGPTIQTestCase(id: number, expectedVersion: number): Promise<{ deleted: boolean }> {
  const { data } = await apiClient.delete<{ deleted: boolean }>(`${BASE}/cases/${id}`, { params: { expected_version: expectedVersion } })
  return data
}

/** 只重算 revisionId（应为测试项当前版本）的结果；测试项已被修改时返回 409 GPT_IQ_TEST_CASE_CONFLICT。 */
export async function rejudgeGPTIQTestCase(id: number, revisionId: number): Promise<GPTIQRejudgeResult> {
  const { data } = await apiClient.post<GPTIQRejudgeResult>(`${BASE}/cases/${id}/rejudge`, { revision_id: revisionId })
  return data
}

export async function startGPTIQTestRun(payload: GPTIQRunRequest): Promise<GPTIQBatch> {
  const { data } = await apiClient.post<GPTIQBatch>(`${BASE}/runs`, payload)
  return data
}

export async function getGPTIQTestRun(id: number, signal?: AbortSignal): Promise<GPTIQBatchDetail> {
  const { data } = await apiClient.get<GPTIQBatchDetail>(`${BASE}/runs/${id}`, { signal })
  return data
}

export async function cancelGPTIQTestRun(id: number): Promise<{ cancel_requested: boolean }> {
  const { data } = await apiClient.post<{ cancel_requested: boolean }>(`${BASE}/runs/${id}/cancel`)
  return data
}
