import { describe, expect, it } from 'vitest'

import { formatCacheTokens } from '@/utils/formatters'

describe('formatCacheTokens', () => {
  it('keeps small cache token counts exact for usage rows', () => {
    expect(formatCacheTokens(1848)).toBe('1,848')
  })
})
