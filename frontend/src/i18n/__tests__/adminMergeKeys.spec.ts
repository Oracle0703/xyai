import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

function getPath(root: Record<string, unknown>, path: string): unknown {
  return path.split('.').reduce<unknown>((current, key) => {
    if (!current || typeof current !== 'object') return undefined
    return (current as Record<string, unknown>)[key]
  }, root)
}

const requiredPaths = [
  'nav.requestIntercept',
  'nav.tokenAnalysis',
  'nav.promptMetrics',
  'admin.promptMetrics.title',
  'admin.requestIntercept.title',
  'admin.tokenAnalysis.title',
  'admin.settings.requestArchive.title',
  'admin.settings.requestArchive.captureResponseHint',
  'admin.settings.requestArchive.saveFailed',
  'admin.riskControl.tabs.promptRisk',
  'admin.riskControl.action.promptRiskBlock',
  'admin.riskControl.action.promptRiskObserve',
  'admin.riskControl.promptRisk.intro',
  'admin.riskControl.promptRisk.judge.title',
  'admin.subscriptions.resetDailyQuota',
  'admin.subscriptions.resetDailyQuotaTitle',
  'admin.subscriptions.resetDailyQuotaConfirm',
  'admin.subscriptions.guide.actions.resetDailyQuota',
  'admin.subscriptions.guide.actions.resetDailyQuotaDesc',
  'admin.accounts.openai.compatibleProvider',
  'admin.accounts.openai.compatibleProviderHint'
]

describe.each([
  ['en', en],
  ['zh', zh]
] as const)('admin merge locale keys %s', (_locale, messages) => {
  it('keeps local admin feature translations after upstream locale split', () => {
    for (const path of requiredPaths) {
      expect(getPath(messages, path), path).toBeTruthy()
    }
  })
})
