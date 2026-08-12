/**
 * Admin Subscriptions API endpoints
 * Handles user subscription management for administrators
 */

import { apiClient } from '../client'
import type {
  UserSubscription,
  SubscriptionProgress,
  AssignSubscriptionRequest,
  BulkAssignSubscriptionRequest,
  ExtendSubscriptionRequest,
  PaginatedResponse
} from '@/types'

export interface SubscriptionGroupFilterOption {
  id: number
  name: string
}

export type SubscriptionOrganization = 'xunyou' | 'wsdashi'

export interface SubscriptionAdminFilters {
  status?: 'active' | 'expired' | 'revoked' | 'suspended'
  user_id?: number
  group_id?: number
  platform?: string
  organization?: SubscriptionOrganization
  sort_by?: 'created_at' | 'expires_at' | 'status'
  sort_order?: 'asc' | 'desc'
}

type SubscriptionDailyResetFilters = Pick<
  SubscriptionAdminFilters,
  'status' | 'user_id' | 'group_id' | 'platform' | 'organization'
>

/**
 * List all subscriptions with pagination
 * @param page - Page number (default: 1)
 * @param pageSize - Items per page (default: 20)
 * @param filters - Optional filters and secondary sort fields
 * @returns Paginated list of subscriptions
 */
export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: SubscriptionAdminFilters,
  options?: {
    signal?: AbortSignal
  }
): Promise<PaginatedResponse<UserSubscription>> {
  const { data } = await apiClient.get<PaginatedResponse<UserSubscription>>(
    '/admin/subscriptions',
    {
      params: {
        page,
        page_size: pageSize,
        ...filters
      },
      signal: options?.signal
    }
  )
  return data
}

/** Load compact group options for subscription filters without group-management fields. */
export async function searchGroups(keyword = ''): Promise<SubscriptionGroupFilterOption[]> {
  const { data } = await apiClient.get<SubscriptionGroupFilterOption[]>(
    '/admin/subscriptions/search-groups',
    { params: { q: keyword } }
  )
  return data
}

/**
 * Get subscription by ID
 * @param id - Subscription ID
 * @returns Subscription details
 */
export async function getById(id: number): Promise<UserSubscription> {
  const { data } = await apiClient.get<UserSubscription>(`/admin/subscriptions/${id}`)
  return data
}

/**
 * Get subscription progress
 * @param id - Subscription ID
 * @returns Subscription progress with usage stats
 */
export async function getProgress(id: number): Promise<SubscriptionProgress> {
  const { data } = await apiClient.get<SubscriptionProgress>(`/admin/subscriptions/${id}/progress`)
  return data
}

/**
 * Assign subscription to user
 * @param request - Assignment request
 * @returns Created subscription
 */
export async function assign(request: AssignSubscriptionRequest): Promise<UserSubscription> {
  const { data } = await apiClient.post<UserSubscription>('/admin/subscriptions/assign', request)
  return data
}

/**
 * Bulk assign subscriptions to multiple users
 * @param request - Bulk assignment request
 * @returns Created subscriptions
 */
export async function bulkAssign(
  request: BulkAssignSubscriptionRequest
): Promise<UserSubscription[]> {
  const { data } = await apiClient.post<UserSubscription[]>(
    '/admin/subscriptions/bulk-assign',
    request
  )
  return data
}

/**
 * Extend subscription validity
 * @param id - Subscription ID
 * @param request - Extension request with days
 * @returns Updated subscription
 */
export async function extend(
  id: number,
  request: ExtendSubscriptionRequest
): Promise<UserSubscription> {
  const { data } = await apiClient.post<UserSubscription>(
    `/admin/subscriptions/${id}/extend`,
    request
  )
  return data
}

/**
 * Revoke subscription
 * @param id - Subscription ID
 * @returns Success confirmation
 */
export async function revoke(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(`/admin/subscriptions/${id}/revoke`)
  return data
}

/**
 * Restore revoked subscription
 * @param id - Subscription ID
 * @returns Restored subscription
 */
export async function restore(id: number): Promise<UserSubscription> {
  const { data } = await apiClient.post<UserSubscription>(`/admin/subscriptions/${id}/restore`)
  return data
}

/**
 * Reset daily, weekly, and/or monthly usage quota for a subscription
 * @param id - Subscription ID
 * @param options - Which windows to reset
 * @returns Updated subscription
 */
export async function resetQuota(
  id: number,
  options: { daily: boolean; weekly: boolean; monthly: boolean }
): Promise<UserSubscription> {
  const { data } = await apiClient.post<UserSubscription>(
    `/admin/subscriptions/${id}/reset-quota`,
    options
  )
  return data
}

/** Reset the daily quota for every active subscription matching the applied filters. */
export async function resetDailyFiltered(
  filters: SubscriptionAdminFilters,
  idempotencyKey: string
): Promise<{ reset_count: number }> {
  const resetFilters: SubscriptionDailyResetFilters = {
    status: filters.status,
    user_id: filters.user_id,
    group_id: filters.group_id,
    platform: filters.platform,
    organization: filters.organization
  }
  const { data } = await apiClient.post<{ reset_count: number }>(
    '/admin/subscriptions/reset-daily-filtered',
    resetFilters,
    {
      headers: { 'Idempotency-Key': idempotencyKey }
    }
  )
  return data
}

/**
 * List subscriptions by group
 * @param groupId - Group ID
 * @param page - Page number
 * @param pageSize - Items per page
 * @returns Paginated list of subscriptions in the group
 */
export async function listByGroup(
  groupId: number,
  page: number = 1,
  pageSize: number = 20
): Promise<PaginatedResponse<UserSubscription>> {
  const { data } = await apiClient.get<PaginatedResponse<UserSubscription>>(
    `/admin/groups/${groupId}/subscriptions`,
    {
      params: { page, page_size: pageSize }
    }
  )
  return data
}

/**
 * List subscriptions by user
 * @param userId - User ID
 * @param page - Page number
 * @param pageSize - Items per page
 * @returns Paginated list of user's subscriptions
 */
export async function listByUser(
  userId: number,
  page: number = 1,
  pageSize: number = 20
): Promise<PaginatedResponse<UserSubscription>> {
  const { data } = await apiClient.get<PaginatedResponse<UserSubscription>>(
    `/admin/users/${userId}/subscriptions`,
    {
      params: { page, page_size: pageSize }
    }
  )
  return data
}

export const subscriptionsAPI = {
  list,
  searchGroups,
  getById,
  getProgress,
  assign,
  bulkAssign,
  extend,
  revoke,
  restore,
  resetQuota,
  resetDailyFiltered,
  listByGroup,
  listByUser
}

export default subscriptionsAPI
