import { formatOrganizationUsageOrganization } from './organizations'
export { formatOrganizationUsageOrganization } from './organizations'
import * as XLSX from 'xlsx'

import { MAX_CLIENT_EXPORT_ROWS, MAX_XLSX_DATA_ROWS } from '@/constants/organizationUsage'
import type {
  OrganizationUsageGranularity,
  OrganizationUsageMetrics,
  OrganizationUsagePeriod,
  OrganizationUsageRange,
  OrganizationUsageSummaryResponse
} from '@/api/admin/organizationUsage'

export type OrganizationUsageReportMode = 'month' | 'week' | 'custom'
export type CustomDateRangeValidation =
  | { valid: true; days: number }
  | { valid: false; error: 'required' | 'invalid_date' | 'start_after_end' | 'range_too_long' }

export interface OrganizationUsageWorkbookInput {
  summary: OrganizationUsageSummaryResponse
  periods: Record<OrganizationUsageGranularity, OrganizationUsagePeriod[]>
}

const DATE_PATTERN = /^(\d{4})-(\d{2})-(\d{2})$/
const DAY_MS = 24 * 60 * 60 * 1000

function formatUTCDate(date: Date): string {
  const year = String(date.getUTCFullYear()).padStart(4, '0')
  const month = String(date.getUTCMonth() + 1).padStart(2, '0')
  const day = String(date.getUTCDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function parseDateString(value: string): Date | null {
  const match = DATE_PATTERN.exec(value)
  if (!match) return null
  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  if (year < 1000 || month < 1 || month > 12 || day < 1 || day > 31) return null

  const date = new Date(0)
  date.setUTCHours(0, 0, 0, 0)
  date.setUTCFullYear(year, month - 1, day)
  return formatUTCDate(date) === value ? date : null
}

function requireDateString(value: string): Date {
  const date = parseDateString(value)
  if (!date) throw new RangeError(`Invalid business date: ${value}`)
  return date
}

function addCalendarDays(value: string, days: number): string {
  const date = requireDateString(value)
  return formatUTCDate(new Date(date.getTime() + days * DAY_MS))
}

export function getBusinessDateString(now: Date = new Date()): string {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit'
  }).formatToParts(now)
  const values = Object.fromEntries(parts.map((part) => [part.type, part.value]))
  return `${values.year}-${values.month}-${values.day}`
}

export function getMonthDateRange(dateString: string): OrganizationUsageRange {
  const date = requireDateString(dateString)
  const year = date.getUTCFullYear()
  const month = date.getUTCMonth()
  const start = new Date(0)
  start.setUTCHours(0, 0, 0, 0)
  start.setUTCFullYear(year, month, 1)
  const end = new Date(0)
  end.setUTCHours(0, 0, 0, 0)
  end.setUTCFullYear(year, month + 1, 0)
  return { start_date: formatUTCDate(start), end_date: formatUTCDate(end) }
}

export function getWeekDateRange(dateString: string): OrganizationUsageRange {
  const date = requireDateString(dateString)
  const mondayOffset = (date.getUTCDay() + 6) % 7
  const startDate = addCalendarDays(dateString, -mondayOffset)
  return { start_date: startDate, end_date: addCalendarDays(startDate, 6) }
}

export function getDefaultOrganizationUsageRange(
  mode: OrganizationUsageReportMode,
  today: string = getBusinessDateString()
): OrganizationUsageRange {
  if (mode === 'month') return getMonthDateRange(today)
  if (mode === 'week') return getWeekDateRange(today)
  requireDateString(today)
  return { start_date: addCalendarDays(today, -29), end_date: today }
}

export function validateCustomDateRange(startDate: string, endDate: string): CustomDateRangeValidation {
  if (!startDate || !endDate) return { valid: false, error: 'required' }
  const start = parseDateString(startDate)
  const end = parseDateString(endDate)
  if (!start || !end) return { valid: false, error: 'invalid_date' }
  if (start.getTime() > end.getTime()) return { valid: false, error: 'start_after_end' }

  const days = Math.floor((end.getTime() - start.getTime()) / DAY_MS) + 1
  if (days > 366) return { valid: false, error: 'range_too_long' }
  return { valid: true, days }
}

/** Inclusive calendar-day count using the same UTC date parsing as validateCustomDateRange. */
export function organizationUsageInclusiveDays(startDate: string, endDate: string): number | null {
  const result = validateCustomDateRange(startDate, endDate)
  return result.valid ? result.days : null
}

/**
 * Auto trend granularity: ≤31 day, ≤120 week, else month.
 * Must not use browser-local Date subtraction.
 */
export function inferOrganizationUsageTrendGranularity(
  startDate: string,
  endDate: string
): OrganizationUsageGranularity {
  const days = organizationUsageInclusiveDays(startDate, endDate)
  if (days == null) return 'day'
  if (days <= 31) return 'day'
  if (days <= 120) return 'week'
  return 'month'
}

export function getOrganizationUsageExportFileName(startDate: string, endDate: string): string {
  return `organization_usage_${startDate}_to_${endDate}.xlsx`
}


const METRIC_HEADERS = [
  'Requests',
  'Input Tokens',
  'Output Tokens',
  'Cache Creation Tokens',
  'Cache Read Tokens',
  'Total Tokens',
  'Actual Cost'
] as const

function metricCells(metrics: OrganizationUsageMetrics): number[] {
  return [
    metrics.requests,
    metrics.input_tokens,
    metrics.output_tokens,
    metrics.cache_creation_tokens,
    metrics.cache_read_tokens,
    metrics.total_tokens,
    metrics.actual_cost
  ]
}

function periodRange(period: OrganizationUsagePeriod | null): string {
  return period ? `${period.period_start} 至 ${period.period_end}` : ''
}

function peakCells(period: OrganizationUsagePeriod | null): Array<string | boolean | number> {
  return period ? [periodRange(period), period.partial, period.total_tokens] : ['', '', '']
}

function championRow(label: string, champion: OrganizationUsagePeriod | null): unknown[] {
  if (!champion) return [label, '', '', '', '', '', '', '']
  return [
    label,
    '',
    champion.period_start,
    champion.period_end,
    champion.partial,
    champion.email,
    formatOrganizationUsageOrganization(champion.organization),
    champion.total_tokens
  ]
}

function buildOverviewRows(summary: OrganizationUsageSummaryResponse): unknown[][] {
  const { overview, champions, range } = summary
  return [
    ['指标', '值', '周期开始', '周期结束', 'Partial', '用户', '组织', 'Total Tokens'],
    ['日期范围', `${range.start_date} 至 ${range.end_date}`, '', '', '', '', '', ''],
    ['生成时间', new Date().toISOString()],
    ['统计时区', 'Asia/Shanghai'],
    ['数据截至', range.as_of ?? ''],
    ['归属口径', '当前启用成员及当前部门归属'],
    ['筛选组织', !summary.applied_filters || summary.applied_filters.organization === 'all' ? '全部可见组织' : formatOrganizationUsageOrganization(summary.applied_filters.organization)],
    ['筛选部门', summary.applied_filters?.department_id === 'unassigned' ? '未分配' : summary.departments?.find(item => String(item.department_id) === summary.applied_filters?.department_id)?.department_name ?? '全部可见部门'],
    ['筛选平台', !summary.applied_filters || summary.applied_filters.platform === 'all' ? '全部平台' : summary.applied_filters.platform],
    ['邮箱筛选', summary.applied_filters?.q ?? ''],
    ['范围版本', summary.scope_version ?? ''],
    ['成员人数', overview.active_users, '', '', '', '', '', ''],
    ['活跃人数', overview.used_users, '', '', '', '', '', ''],
    ['Requests', overview.requests, '', '', '', '', '', ''],
    ['Input Tokens', overview.input_tokens, '', '', '', '', '', ''],
    ['Output Tokens', overview.output_tokens, '', '', '', '', '', ''],
    ['Cache Creation Tokens', overview.cache_creation_tokens, '', '', '', '', '', ''],
    ['Cache Read Tokens', overview.cache_read_tokens, '', '', '', '', '', ''],
    ['Total Tokens', overview.total_tokens, '', '', '', '', '', ''],
    ['Actual Cost', overview.actual_cost, '', '', '', '', '', ''],
    championRow('日度 Champion', champions.day),
    championRow('周度 Champion', champions.week),
    championRow('月度 Champion', champions.month)
  ]
}

function buildOrganizationRows(summary: OrganizationUsageSummaryResponse): unknown[][] {
  return [
    ['组织', '成员人数', '活跃人数', ...METRIC_HEADERS],
    ...summary.organizations.map((organization) => [
      formatOrganizationUsageOrganization(organization.organization),
      organization.active_users,
      organization.used_users,
      ...metricCells(organization)
    ])
  ]
}

function buildPeopleRows(summary: OrganizationUsageSummaryResponse): unknown[][] {
  return [
    [
      '用户ID',
      '邮箱',
      '组织',
      '部门',
      ...METRIC_HEADERS,
      '日峰值日期范围',
      '日峰值Partial',
      '日峰值Total Tokens',
      '周峰值日期范围',
      '周峰值Partial',
      '周峰值Total Tokens',
      '月峰值日期范围',
      '月峰值Partial',
      '月峰值Total Tokens'
    ],
    ...summary.items.map((item) => [
      item.user_id,
      item.email,
      formatOrganizationUsageOrganization(item.organization),
      item.department_name || '未分配',
      ...metricCells(item),
      ...peakCells(item.peak_day),
      ...peakCells(item.peak_week),
      ...peakCells(item.peak_month)
    ])
  ]
}

function buildPeriodRows(periods: OrganizationUsagePeriod[]): unknown[][] {
  return [
    ['周期开始', '周期结束', 'Partial', '用户ID', '邮箱', '组织', '部门', ...METRIC_HEADERS],
    ...periods.map((period) => [
      period.period_start,
      period.period_end,
      period.partial,
      period.user_id,
      period.email,
      formatOrganizationUsageOrganization(period.organization),
      period.department_name || '未分配',
      ...metricCells(period)
    ])
  ]
}

function buildDepartmentRows(summary: OrganizationUsageSummaryResponse): unknown[][] {
  return [['组织', '部门ID', '部门', '成员人数', '活跃人数', ...METRIC_HEADERS], ...(summary.departments ?? []).map(department => [
    formatOrganizationUsageOrganization(department.organization), department.department_id ?? '', department.department_name || '未分配', department.active_users, department.used_users, ...metricCells(department)
  ])]
}

function buildPlatformRows(summary: OrganizationUsageSummaryResponse): unknown[][] {
  return [['平台', '活跃人数', ...METRIC_HEADERS], ...(summary.platforms ?? []).map(platform => [
    platform.platform, platform.used_users, ...metricCells(platform)
  ])]
}

export function buildOrganizationUsageWorkbook(input: OrganizationUsageWorkbookInput): XLSX.WorkBook {
  const rowCounts: Array<[string, number]> = [
    ['组织汇总', input.summary.organizations.length],
    ['部门汇总', input.summary.departments?.length ?? 0],
    ['平台汇总', input.summary.platforms?.length ?? 0],
    ['人员汇总', input.summary.items.length],
    ['月度明细', input.periods.month.length],
    ['周度明细', input.periods.week.length],
    ['日度明细', input.periods.day.length]
  ]
  for (const [sheetName, rowCount] of rowCounts) {
    if (rowCount > MAX_XLSX_DATA_ROWS) {
      throw new Error(`${sheetName} exceeds the Excel row limit of ${MAX_XLSX_DATA_ROWS}`)
    }
  }
  const totalRows = rowCounts.reduce((total, [, rowCount]) => total + rowCount, 0)
  if (totalRows > MAX_CLIENT_EXPORT_ROWS) {
    throw new Error(`Workbook exceeds the client export row limit of ${MAX_CLIENT_EXPORT_ROWS}`)
  }

  const workbook = XLSX.utils.book_new()
  const sheets: Array<[string, unknown[][]]> = [
    ['报表概览', buildOverviewRows(input.summary)],
    ['人员汇总', buildPeopleRows(input.summary)],
    ['组织汇总', buildOrganizationRows(input.summary)],
    ['部门汇总', buildDepartmentRows(input.summary)],
    ['平台汇总', buildPlatformRows(input.summary)],
    ['月度明细', buildPeriodRows(input.periods.month)],
    ['周度明细', buildPeriodRows(input.periods.week)],
    ['日度明细', buildPeriodRows(input.periods.day)]
  ]

  for (const [name, rows] of sheets) {
    const sheet = XLSX.utils.aoa_to_sheet(rows)
    if (name === '人员汇总') {
      sheet['!cols'] = rows[0].map((_, index) => ({
        wch: index === 1 ? 36 : index >= 11 && (index - 11) % 3 === 0 ? 26 : 20
      }))
      sheet['!autofilter'] = { ref: sheet['!ref']! }
    }
    XLSX.utils.book_append_sheet(workbook, sheet, name)
  }
  return workbook
}
