import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe.each([
  ['en', en],
  ['zh', zh]
] as const)('organization usage locale in %s', (_locale, messages) => {
  it('provides the route, navigation and report labels', () => {
    expect(messages.nav.organizationUsage).toBeTypeOf('string')
    expect(messages.admin.organizationUsage.title).toBeTypeOf('string')
    expect(messages.admin.organizationUsage.filters.custom).toBeTypeOf('string')
    expect(messages.admin.organizationUsage.feedback.exportSuccess).toBeTypeOf('string')
    expect(messages.admin.organizationUsage.feedback.generatingWorkbook).toBeTypeOf('string')
  })
})

it.each([
  ['zh', zh, '注册人数', '活跃人数'],
  ['en', en, 'Registered users', 'Active users']
] as const)('uses the requested headcount labels in %s', (_locale, messages, registered, active) => {
  expect(messages.admin.organizationUsage.metrics.activeUsers).toBe(registered)
  expect(messages.admin.organizationUsage.metrics.usedUsers).toBe(active)
})
