import { describe, expect, it } from 'vitest'

import * as organizationUsageReport from '@/utils/organizationUsageReport'
import {
  getBusinessDateString,
  getDefaultOrganizationUsageRange,
  getMonthDateRange,
  getOrganizationUsageExportFileName,
  getWeekDateRange,
  inferOrganizationUsageTrendGranularity,
  validateCustomDateRange
} from '@/utils/organizationUsageReport'

describe('organization usage report date helpers', () => {
  it('calculates month boundaries including leap years from a date string', () => {
    expect(getMonthDateRange('2024-02-10')).toEqual({
      start_date: '2024-02-01',
      end_date: '2024-02-29'
    })
    expect(getMonthDateRange('2025-02-10')).toEqual({
      start_date: '2025-02-01',
      end_date: '2025-02-28'
    })
  })

  it('calculates a natural Monday-to-Sunday week across month boundaries', () => {
    expect(getWeekDateRange('2026-07-01')).toEqual({
      start_date: '2026-06-29',
      end_date: '2026-07-05'
    })
    expect(getWeekDateRange('2026-07-05')).toEqual({
      start_date: '2026-06-29',
      end_date: '2026-07-05'
    })
  })

  it('returns mode defaults using a fixed business date string', () => {
    expect(getDefaultOrganizationUsageRange('month', '2026-07-10')).toEqual({
      start_date: '2026-07-01',
      end_date: '2026-07-31'
    })
    expect(getDefaultOrganizationUsageRange('week', '2026-07-10')).toEqual({
      start_date: '2026-07-06',
      end_date: '2026-07-12'
    })
    expect(getDefaultOrganizationUsageRange('custom', '2026-07-10')).toEqual({
      start_date: '2026-06-11',
      end_date: '2026-07-10'
    })
  })

  it('derives today in the fixed Asia/Shanghai business timezone', () => {
    expect(getBusinessDateString(new Date('2026-07-10T15:59:59Z'))).toBe('2026-07-10')
    expect(getBusinessDateString(new Date('2026-07-10T16:00:00Z'))).toBe('2026-07-11')
  })

  it('validates required dates and ascending order', () => {
    expect(validateCustomDateRange('', '2026-07-10')).toEqual({ valid: false, error: 'required' })
    expect(validateCustomDateRange('2026-07-11', '2026-07-10')).toEqual({ valid: false, error: 'start_after_end' })
    expect(validateCustomDateRange('2026-02-30', '2026-03-01')).toEqual({ valid: false, error: 'invalid_date' })
  })

  it('allows at most 366 inclusive calendar days', () => {
    expect(validateCustomDateRange('2024-01-01', '2024-12-31')).toEqual({ valid: true, days: 366 })
    expect(validateCustomDateRange('2024-01-01', '2025-01-01')).toEqual({
      valid: false,
      error: 'range_too_long'
    })
  })

  it('builds the fixed xlsx export filename', () => {
    expect(getOrganizationUsageExportFileName('2026-07-01', '2026-07-10')).toBe(
      'organization_usage_2026-07-01_to_2026-07-10.xlsx'
    )
  })

  it('infers trend granularity from inclusive calendar days', () => {
    expect(inferOrganizationUsageTrendGranularity('2026-07-01', '2026-07-31')).toBe('day')
    expect(inferOrganizationUsageTrendGranularity('2026-07-01', '2026-08-01')).toBe('week')
    expect(inferOrganizationUsageTrendGranularity('2026-01-01', '2026-04-30')).toBe('week')
    expect(inferOrganizationUsageTrendGranularity('2026-01-01', '2026-05-02')).toBe('month')
    // Default month report range (28-31 days) stays on day
    expect(inferOrganizationUsageTrendGranularity('2026-02-01', '2026-02-28')).toBe('day')
  })

  it.each([
    ['xunyou', undefined, '迅游'],
    ['xunyou.com', undefined, '迅游'],
    ['wsdashi', undefined, '速宝'],
    ['wsdashi.com', undefined, '速宝'],
    ['other', undefined, '其他'],
    ['unknown', 'Other', 'Other']
  ] as const)('formats organization %s for display', (value, otherLabel, expected) => {
    const formatter = (organizationUsageReport as unknown as {
      formatOrganizationUsageOrganization?: (organization: string, fallback?: string) => string
    }).formatOrganizationUsageOrganization
    expect(formatter).toBeTypeOf('function')
    if (!formatter) return
    expect(formatter(value, otherLabel)).toBe(expected)
  })
})
