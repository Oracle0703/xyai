/**
 * GPT 降智测试用户侧 API（只读）。
 * 结果挂在 /gpt-quota 页额度区下方；读取只查数据库，不会触发测试。
 * 设计见 docs/features/gpt-iq-test-design-cn.md §10。
 */
import { apiClient } from './client'
import type { GPTQuotaGroupKey } from './gptQuotaDisplay'

export type GPTIQCaseKind = 'chat' | 'graded'
export type GPTIQVerdict = 'untested' | 'skipped' | 'failed' | 'insufficient' | 'suspect' | 'normal' | 'connected'
export type GPTIQReasonCode = 'low_accuracy' | 'low_reasoning' | 'insufficient_samples' | 'all_failed' | 'chat_failed' | 'interrupted'
export type GPTIQRunStatus = 'running' | 'ok' | 'error' | 'skipped' | 'cancelled' | 'abandoned'
export type GPTIQTrigger = 'schedule' | 'manual'

export interface GPTIQSchedule {
  enabled: boolean
  run_time: string
  timezone: string
}

export interface GPTIQThresholds {
  min_accuracy_percent: number
  min_graded_runs: number
  reasoning_ratio_percent: number
}

export interface GPTIQPublicCase {
  id: number
  name: string
  kind: GPTIQCaseKind
  model: string
  reasoning_effort?: string
  runs: number
  /** 仅在测试项设置为公开题面时返回。 */
  prompt?: string
  public_answer: boolean
  /**
   * 当前版本 ID（接口若提供则用于"第 N 版"标记）；未提供时前端以已加载结果中最新执行的版本为准。
   */
  revision_id?: number
}

/** 结果实际执行的题目版本；prompt 仅在测试项当前公开题面时返回。 */
export interface GPTIQCaseRevision {
  id: number
  revision: number
  kind: GPTIQCaseKind
  model: string
  reasoning_effort?: string
  runs: number
  prompt?: string
}

export interface GPTIQReason {
  code: GPTIQReasonCode
  case_id?: number
  correct: number
  graded: number
  percent: number
  /** 失败类原因的错误类别（如 timeout），不含原始错误文本。 */
  detail?: string
}

export interface GPTIQRun {
  index: number
  status: GPTIQRunStatus
  reason?: string
  correct: boolean | null
  answer_preview?: string
  /** 仅详情接口在测试项公开回答时返回全文。 */
  answer?: string
  answer_truncated?: boolean
  input_tokens: number | null
  output_tokens: number | null
  reasoning_tokens: number | null
  duration_ms: number | null
  ttft_ms: number | null
  tps: number | null
  started_at: string
  finished_at?: string
}

export interface GPTIQCaseSet {
  case_id: number
  batch_id: number
  trigger: GPTIQTrigger
  run_date: string
  tested_at: string
  /** 本组结果实际执行的版本；模型、思考强度、次数与题面以它为准，而不是当前测试项。 */
  revision: GPTIQCaseRevision
  runs: GPTIQRun[]
  total: number
  ok: number
  errors: number
  skipped: number
  /** cancelled + abandoned 的调用数（中断，不计为失败）。 */
  interrupted: number
  graded: number
  correct: number
  median_reasoning_tokens: number | null
  avg_duration_ms: number | null
  avg_tps: number | null
}

/** 趋势只来自定时批次；手动复测不产生趋势点。 */
export interface GPTIQTrendPoint {
  date: string
  verdict: GPTIQVerdict
  correct: number
  graded: number
}

export interface GPTIQUserCard {
  /** 与额度卡片相同的条目 ID。 */
  id: number
  display_name: string
  verdict: GPTIQVerdict
  reasons: GPTIQReason[]
  tested_at: string | null
  cases: GPTIQCaseSet[]
  trend: GPTIQTrendPoint[]
}

export interface GPTIQUserView {
  enabled: boolean
  server_time: string
  poll_interval_seconds: number
  schedule: GPTIQSchedule
  next_run_at: string | null
  thresholds: GPTIQThresholds
  cases: GPTIQPublicCase[]
  groups: Record<GPTQuotaGroupKey, GPTIQUserCard[]>
}

export interface GPTIQEntryDetail {
  enabled: boolean
  id: number
  display_name: string
  days: number
  cases: GPTIQPublicCase[]
  /** 最新批次在前。 */
  sets: GPTIQCaseSet[]
}

export interface GPTIQEntryDetailParams {
  case_id?: number
  /** ≤30，后端默认 14。 */
  days?: number
}

export async function getGPTIQTests(signal?: AbortSignal): Promise<GPTIQUserView> {
  const { data } = await apiClient.get<GPTIQUserView>('/gpt-quota/iq-tests', { signal })
  return data
}

export async function getGPTIQTestEntry(entryId: number, params: GPTIQEntryDetailParams = {}, signal?: AbortSignal): Promise<GPTIQEntryDetail> {
  const { data } = await apiClient.get<GPTIQEntryDetail>(`/gpt-quota/iq-tests/entries/${entryId}`, { params, signal })
  return data
}
