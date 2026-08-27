/**
 * Admin API barrel export
 * Centralized exports for all admin API modules
 */

import dashboardAPI from './dashboard'
import usersAPI from './users'
import groupsAPI from './groups'
import accountsAPI from './accounts'
import proxiesAPI from './proxies'
import redeemAPI from './redeem'
import promoAPI from './promo'
import announcementsAPI from './announcements'
import settingsAPI from './settings'
import systemAPI from './system'
import subscriptionsAPI from './subscriptions'
import usageAPI from './usage'
import tokenAnalysisAPI from './tokenAnalysis'
import geminiAPI from './gemini'
import antigravityAPI from './antigravity'
import grokAPI from './grok'
import cnProvidersAPI from './cnProviders'
import userAttributesAPI from './userAttributes'
import opsAPI from './ops'
import errorPassthroughAPI from './errorPassthrough'
import dataManagementAPI from './dataManagement'
import apiKeysAPI from './apiKeys'
import scheduledTestsAPI from './scheduledTests'
import backupAPI from './backup'
import tlsFingerprintProfileAPI from './tlsFingerprintProfile'
import channelsAPI from './channels'
import channelMonitorAPI from './channelMonitor'
import channelMonitorTemplateAPI from './channelMonitorTemplate'
import adminPaymentAPI from './payment'
import affiliatesAPI from './affiliates'
import riskControlAPI from './riskControl'
import requestInterceptAPI from './requestIntercept'
import userConcurrencyPresetsAPI from './userConcurrencyPresets'
import promptMetricsAPI from './promptMetrics'
import adminComplianceAPI from './compliance'
import organizationUsageAPI from './organizationUsage'
import auditAPI from './audit'
import pluginsAPI from './plugins'

/**
 * Unified admin API object for convenient access
 */
export const adminAPI = {
  dashboard: dashboardAPI,
  users: usersAPI,
  groups: groupsAPI,
  accounts: accountsAPI,
  proxies: proxiesAPI,
  redeem: redeemAPI,
  promo: promoAPI,
  announcements: announcementsAPI,
  settings: settingsAPI,
  system: systemAPI,
  subscriptions: subscriptionsAPI,
  usage: usageAPI,
  tokenAnalysis: tokenAnalysisAPI,
  gemini: geminiAPI,
  antigravity: antigravityAPI,
  grok: grokAPI,
  cnProviders: cnProvidersAPI,
  userAttributes: userAttributesAPI,
  ops: opsAPI,
  errorPassthrough: errorPassthroughAPI,
  dataManagement: dataManagementAPI,
  apiKeys: apiKeysAPI,
  scheduledTests: scheduledTestsAPI,
  backup: backupAPI,
  tlsFingerprintProfiles: tlsFingerprintProfileAPI,
  channels: channelsAPI,
  channelMonitor: channelMonitorAPI,
  channelMonitorTemplate: channelMonitorTemplateAPI,
  payment: adminPaymentAPI,
  affiliates: affiliatesAPI,
  riskControl: riskControlAPI,
  requestIntercept: requestInterceptAPI,
  userConcurrencyPresets: userConcurrencyPresetsAPI,
  promptMetrics: promptMetricsAPI,
  compliance: adminComplianceAPI,
  organizationUsage: organizationUsageAPI,
  audit: auditAPI,
  plugins: pluginsAPI
}

export {
  dashboardAPI,
  usersAPI,
  groupsAPI,
  accountsAPI,
  proxiesAPI,
  redeemAPI,
  promoAPI,
  announcementsAPI,
  settingsAPI,
  systemAPI,
  subscriptionsAPI,
  usageAPI,
  tokenAnalysisAPI,
  geminiAPI,
  antigravityAPI,
  grokAPI,
  cnProvidersAPI,
  userAttributesAPI,
  opsAPI,
  errorPassthroughAPI,
  dataManagementAPI,
  apiKeysAPI,
  scheduledTestsAPI,
  backupAPI,
  tlsFingerprintProfileAPI,
  channelsAPI,
  channelMonitorAPI,
  channelMonitorTemplateAPI,
  adminPaymentAPI,
  affiliatesAPI,
  riskControlAPI,
  requestInterceptAPI,
  userConcurrencyPresetsAPI,
  promptMetricsAPI,
  adminComplianceAPI,
  organizationUsageAPI,
  auditAPI,
  pluginsAPI
}

export default adminAPI

// Re-export types used by components
export type { AuditLog, AuditLogQuery, AuditLogListResponse } from './audit'
export type { BalanceHistoryItem } from './users'
export type { ErrorPassthroughRule, CreateRuleRequest, UpdateRuleRequest } from './errorPassthrough'
export type { BackupAgentHealth, DataManagementConfig } from './dataManagement'
export type { TLSFingerprintProfile, CreateProfileRequest, UpdateProfileRequest } from './tlsFingerprintProfile'
export type { ContentModerationConfig, ContentModerationLog, ModerationMode } from './riskControl'
export type { RequestInterceptRule, RequestInterceptNormalization, RequestInterceptMatchMode, RequestInterceptMatchScope, RequestInterceptScope, RequestInterceptTestResponse } from './requestIntercept'
export type {
  OrganizationUsageMetrics,
  OrganizationUsageRange,
  OrganizationUsagePagination,
  OrganizationUsagePeriod,
  OrganizationUsageSummaryItem,
  OrganizationUsageOverview,
  OrganizationUsageOrganization,
  OrganizationUsageChampions,
  OrganizationUsageSummaryResponse,
  OrganizationUsagePeriodsResponse,
  OrganizationUsageTrendPoint,
  OrganizationUsageTrendResponse,
  OrganizationUsageTrendQuery,
  OrganizationUsageOrganizationFilter,
  OrganizationUsageGranularity,
  OrganizationUsageSortOrder,
  OrganizationUsageSortBy,
  OrganizationUsageQuery,
  OrganizationUsageSummaryQuery,
  OrganizationUsagePeriodsQuery,
  OrganizationUsageRequestOptions,
  OrganizationUsageFetchProgress,
  OrganizationUsageFetchAllOptions,
  OrganizationUsageReportData
} from './organizationUsage'
export {
  MAX_CLIENT_EXPORT_ROWS,
  MAX_XLSX_DATA_ROWS,
  ORGANIZATION_USAGE_SORT_FIELDS
} from './organizationUsage'
export type {
  PluginInstallation,
  PluginCompatibility,
  PluginUISession,
  PluginTestResult
} from './plugins'
