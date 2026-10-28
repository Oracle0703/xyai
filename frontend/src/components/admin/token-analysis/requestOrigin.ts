import { useI18n } from 'vue-i18n'
import type {
  TokenAnalysisRequestOrigin,
  TokenAnalysisRequestOriginKind,
  TokenAnalysisRequestOriginReason
} from '@/api/admin/tokenAnalysis'

// 与后端 tokenAnalysisOriginReasons 对应: reason 决定 kind 与置信度。v2 保留全部 v1 reason,
// 新增的 compaction_resume 是唯一 medium 置信度的 AI reason, v1 行出现它即视为坏数据。
const reasonKinds: Record<TokenAnalysisRequestOriginReason, TokenAnalysisRequestOriginKind> = {
  subagent: 'ai',
  automation: 'ai',
  tool_result: 'ai',
  agent_message: 'ai',
  assistant_continuation: 'ai',
  compaction: 'ai',
  compaction_resume: 'ai',
  user_input: 'human',
  no_signal: 'unknown',
  unsupported_shape: 'unknown',
  body_truncated: 'unknown',
  count_tokens: 'unknown'
}

const v2OnlyReasons = new Set<TokenAnalysisRequestOriginReason>(['compaction_resume'])

function expectedConfidence(reason: TokenAnalysisRequestOriginReason, kind: TokenAnalysisRequestOriginKind): TokenAnalysisRequestOrigin['confidence'] {
  if (reason === 'compaction_resume') return 'medium'
  return kind === 'ai' ? 'high' : kind === 'human' ? 'medium' : 'none'
}

// API 响应在运行时仍可能是旧数据或未知版本; 不从预览、重复次数推测来源。
function readRequestOrigin(summary: unknown): TokenAnalysisRequestOrigin | null {
  if (!summary || typeof summary !== 'object' || Array.isArray(summary)) return null
  const origin = (summary as Record<string, unknown>).request_origin
  if (!origin || typeof origin !== 'object' || Array.isArray(origin)) return null
  const { kind, reason, confidence, version } = origin as Record<string, unknown>
  if ((version !== 1 && version !== 2) || typeof reason !== 'string' || !Object.prototype.hasOwnProperty.call(reasonKinds, reason)) return null
  const knownReason = reason as TokenAnalysisRequestOriginReason
  if (version === 1 && v2OnlyReasons.has(knownReason)) return null
  const expectedKind = reasonKinds[knownReason]
  const wantConfidence = expectedConfidence(knownReason, expectedKind)
  if (kind !== expectedKind || confidence !== wantConfidence) return null
  return { kind: expectedKind, reason: knownReason, confidence: wantConfidence, version }
}

export function useRequestOrigin() {
  const { t } = useI18n()

  function requestOrigin(summary: unknown) {
    const origin = readRequestOrigin(summary)
    const kind = origin?.kind ?? 'unknown'
    const reason = origin ? t(`admin.tokenAnalysis.requestOrigin.reasons.${origin.reason}`) : t('admin.tokenAnalysis.requestOrigin.notIdentified')
    const label = kind === 'unknown' ? '—' : t(`admin.tokenAnalysis.requestOrigin.${kind}`)
    return {
      label,
      reason,
      confidence: origin ? t(`admin.tokenAnalysis.requestOrigin.confidences.${origin.confidence}`) : '—',
      version: origin?.version ?? '—',
      tooltip: `${t('admin.tokenAnalysis.requestOrigin.inferred')} · ${reason}`,
      className: kind === 'human'
        ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
        : kind === 'ai'
          ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
          : 'text-gray-400 dark:text-dark-400'
    }
  }

  return { requestOrigin }
}
