import { describe, expect, it } from 'vitest'
import * as XLSX from 'xlsx'

import type {
  OrganizationUsageMetrics,
  OrganizationUsagePeriod,
  OrganizationUsageSummaryResponse
} from '@/api/admin/organizationUsage'
import { MAX_CLIENT_EXPORT_ROWS, MAX_XLSX_DATA_ROWS } from '@/api/admin/organizationUsage'
import { buildOrganizationUsageWorkbook } from '@/utils/organizationUsageReport'

const zeroMetrics: OrganizationUsageMetrics = {
  requests: 0,
  input_tokens: 0,
  output_tokens: 0,
  cache_creation_tokens: 0,
  cache_read_tokens: 0,
  total_tokens: 0,
  actual_cost: 0
}

const EXPECTED_MAX_XLSX_DATA_ROWS = 1_048_575
const EXPECTED_MAX_CLIENT_EXPORT_ROWS = 100_000

const emptySummary: OrganizationUsageSummaryResponse = {
  range: { start_date: '2026-07-01', end_date: '2026-07-10' },
  overview: { active_users: 0, used_users: 0, ...zeroMetrics },
  organizations: [],
  champions: { day: null, week: null, month: null },
  items: [],
  pagination: { total: 0, page: 1, page_size: 500, pages: 1 }
}

function rows(workbook: XLSX.WorkBook, sheetName: string): unknown[][] {
  return XLSX.utils.sheet_to_json(workbook.Sheets[sheetName], { header: 1, raw: true })
}

function roundTrip(workbook: XLSX.WorkBook): XLSX.WorkBook {
  const contents = XLSX.write(workbook, { bookType: 'xlsx', type: 'array' })
  return XLSX.read(contents, { type: 'array' })
}

function period(overrides: Partial<OrganizationUsagePeriod> = {}): OrganizationUsagePeriod {
  return {
    period_start: '2026-07-06',
    period_end: '2026-07-10',
    partial: true,
    user_id: 7,
    email: 'zero@example.com',
    organization: 'other',
    ...zeroMetrics,
    total_tokens: 123,
    actual_cost: 1.25,
    ...overrides
  }
}

describe('organization usage workbook', () => {
  it('creates six legal sheets in the required order and keeps table headers when data is empty', () => {
    const workbook = roundTrip(
      buildOrganizationUsageWorkbook({
        summary: emptySummary,
        periods: { day: [], week: [], month: [] }
      })
    )

    expect(workbook.SheetNames).toEqual([
      '报表概览',
      '组织汇总',
      '人员汇总',
      '月度明细',
      '周度明细',
      '日度明细'
    ])
    for (const name of workbook.SheetNames) {
      expect(name.length).toBeLessThanOrEqual(31)
      expect(name).not.toMatch(/[\\/?*[\]:]/)
      expect(rows(workbook, name)[0]?.length).toBeGreaterThan(0)
    }
    expect(rows(workbook, '组织汇总')).toHaveLength(1)
    expect(rows(workbook, '人员汇总')).toHaveLength(1)
    expect(rows(workbook, '月度明细')).toHaveLength(1)
    expect(rows(workbook, '周度明细')).toHaveLength(1)
    expect(rows(workbook, '日度明细')).toHaveLength(1)
  })

  it('keeps zero-usage people and exports numeric metrics plus peak range and partial values', () => {
    const peakDay = period()
    const summary: OrganizationUsageSummaryResponse = {
      ...emptySummary,
      overview: {
        ...emptySummary.overview,
        active_users: 1,
        used_users: 0,
        requests: 9,
        actual_cost: 3.5
      },
      champions: { day: peakDay, week: null, month: null },
      items: [
        {
          user_id: 7,
          email: 'zero@example.com',
          organization: 'other',
          ...zeroMetrics,
          peak_day: peakDay,
          peak_week: null,
          peak_month: period({
            period_start: '2026-07-01',
            period_end: '2026-07-10',
            partial: false,
            total_tokens: 456
          })
        }
      ],
      pagination: { total: 1, page: 1, page_size: 500, pages: 1 }
    }

    const workbook = roundTrip(
      buildOrganizationUsageWorkbook({
        summary,
        periods: { day: [peakDay], week: [], month: [] }
      })
    )
    const personRows = rows(workbook, '人员汇总')
    const headers = personRows[0] as string[]
    const person = personRows[1]

    expect(personRows).toHaveLength(2)
    expect(person[headers.indexOf('邮箱')]).toBe('zero@example.com')
    expect(person[headers.indexOf('Requests')]).toBe(0)
    expect(typeof person[headers.indexOf('Requests')]).toBe('number')
    expect(person[headers.indexOf('日峰值日期范围')]).toBe('2026-07-06 至 2026-07-10')
    expect(person[headers.indexOf('日峰值Partial')]).toBe(true)
    expect(person[headers.indexOf('日峰值Total Tokens')]).toBe(123)
    expect(typeof person[headers.indexOf('日峰值Total Tokens')]).toBe('number')
    expect(person[headers.indexOf('月峰值Partial')]).toBe(false)

    const overviewRows = rows(workbook, '报表概览')
    const requestsRow = overviewRows.find((row) => row[0] === 'Requests')
    const actualCostRow = overviewRows.find((row) => row[0] === 'Actual Cost')
    const championRow = overviewRows.find((row) => row[0] === '日度 Champion')
    expect(requestsRow?.[1]).toBe(9)
    expect(typeof requestsRow?.[1]).toBe('number')
    expect(actualCostRow?.[1]).toBe(3.5)
    expect(typeof actualCostRow?.[1]).toBe('number')
    expect(championRow).toEqual([
      '日度 Champion',
      '',
      '2026-07-06',
      '2026-07-10',
      true,
      'zero@example.com',
      '其他',
      123
    ])
  })

  it('uses display names and headcount labels across every workbook organization column', () => {
    const xunyouPeriod = period({ organization: 'xunyou' })
    const wsdashiPeriod = period({ organization: 'wsdashi.com' })
    const summary: OrganizationUsageSummaryResponse = {
      ...emptySummary,
      overview: { ...zeroMetrics, active_users: 2, used_users: 1 },
      organizations: [
        { ...zeroMetrics, organization: 'xunyou', active_users: 1, used_users: 1 },
        { ...zeroMetrics, organization: 'wsdashi', active_users: 1, used_users: 0 }
      ],
      champions: { day: wsdashiPeriod, week: null, month: null },
      items: [{
        ...zeroMetrics,
        user_id: 7,
        email: 'alice@xunyou.com',
        organization: 'xunyou.com',
        peak_day: null,
        peak_week: null,
        peak_month: null
      }]
    }
    const workbook = roundTrip(buildOrganizationUsageWorkbook({
      summary,
      periods: { month: [xunyouPeriod], week: [wsdashiPeriod], day: [xunyouPeriod] }
    }))

    const overviewRows = rows(workbook, '报表概览')
    expect(overviewRows.find((row) => row[0] === '注册人数')?.slice(0, 2)).toEqual(['注册人数', 2])
    expect(overviewRows.find((row) => row[0] === '活跃人数')?.slice(0, 2)).toEqual(['活跃人数', 1])
    expect(overviewRows.some((row) => row[0] === '总活跃人数' || row[0] === '有用量人数')).toBe(false)
    expect(overviewRows.find((row) => row[0] === '日度 Champion')?.[6]).toBe('速宝')

    const organizationRows = rows(workbook, '组织汇总')
    expect(organizationRows[0]?.slice(0, 3)).toEqual(['组织', '注册人数', '活跃人数'])
    expect(organizationRows.slice(1).map((row) => row[0])).toEqual(['迅游', '速宝'])
    expect(rows(workbook, '人员汇总')[1]?.[2]).toBe('迅游')

    for (const [sheet, expected] of [
      ['月度明细', '迅游'],
      ['周度明细', '速宝'],
      ['日度明细', '迅游']
    ] as const) {
      const detailRows = rows(workbook, sheet)
      const headers = detailRows[0] as string[]
      expect(detailRows[1]?.[headers.indexOf('组织')]).toBe(expected)
    }
  })

  it('maps period arrays to month, week and day detail sheets without changing numeric cells', () => {
    const month = period({ period_start: '2026-07-01', total_tokens: 300 })
    const week = period({ total_tokens: 200 })
    const day = period({ period_start: '2026-07-10', period_end: '2026-07-10', total_tokens: 100 })
    const workbook = buildOrganizationUsageWorkbook({
      summary: emptySummary,
      periods: { month: [month], week: [week], day: [day] }
    })

    for (const [name, expectedTokens] of [
      ['月度明细', 300],
      ['周度明细', 200],
      ['日度明细', 100]
    ] as const) {
      const detailRows = rows(workbook, name)
      const headers = detailRows[0] as string[]
      expect(detailRows).toHaveLength(2)
      expect(detailRows[1][headers.indexOf('Total Tokens')]).toBe(expectedTokens)
      expect(typeof detailRows[1][headers.indexOf('Actual Cost')]).toBe('number')
    }
  })

  it('rejects sparse summary or period arrays above the Excel data-row limit before building AoA rows', () => {
    expect(MAX_XLSX_DATA_ROWS).toBe(EXPECTED_MAX_XLSX_DATA_ROWS)
    const tooManyPeople = new Array(EXPECTED_MAX_XLSX_DATA_ROWS + 1) as OrganizationUsageSummaryResponse['items']
    expect(() =>
      buildOrganizationUsageWorkbook({
        summary: { ...emptySummary, items: tooManyPeople },
        periods: { day: [], week: [], month: [] }
      })
    ).toThrow(/人员汇总.*Excel row limit/)

    const tooManyDays = new Array(EXPECTED_MAX_XLSX_DATA_ROWS + 1) as OrganizationUsagePeriod[]
    expect(() =>
      buildOrganizationUsageWorkbook({
        summary: emptySummary,
        periods: { day: tooManyDays, week: [], month: [] }
      })
    ).toThrow(/日度明细.*Excel row limit/)
  })

  it('rejects multiple individually valid datasets whose combined rows exceed the client export limit', () => {
    expect(MAX_CLIENT_EXPORT_ROWS).toBe(EXPECTED_MAX_CLIENT_EXPORT_ROWS)
    const people = new Array(60_000) as OrganizationUsageSummaryResponse['items']
    const days = new Array(60_000) as OrganizationUsagePeriod[]

    expect(() =>
      buildOrganizationUsageWorkbook({
        summary: { ...emptySummary, items: people },
        periods: { day: days, week: [], month: [] }
      })
    ).toThrow(/client export row limit/)
  })
})
