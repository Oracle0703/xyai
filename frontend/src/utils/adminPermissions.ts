import type { AdminPermission } from '@/types'

export const ADMIN_PERMISSION_SUBSCRIPTIONS: AdminPermission = 'admin.subscriptions'
export const ADMIN_PERMISSION_USAGE: AdminPermission = 'admin.usage'
export const ADMIN_PERMISSION_TOKEN_ANALYSIS: AdminPermission = 'admin.token_analysis'

const ADMIN_PERMISSION_LANDING_ROUTES: ReadonlyArray<{
  code: AdminPermission
  path: string
}> = [
  { code: ADMIN_PERMISSION_SUBSCRIPTIONS, path: '/admin/subscriptions' },
  { code: ADMIN_PERMISSION_USAGE, path: '/admin/usage' },
  { code: ADMIN_PERMISSION_TOKEN_ANALYSIS, path: '/admin/token-analysis' },
]

export function getAdminLandingPath(
  permissions: readonly string[] | null | undefined,
  backendMode: boolean,
): string {
  if (!backendMode) return '/dashboard'
  const allowed = new Set(permissions ?? [])
  return ADMIN_PERMISSION_LANDING_ROUTES.find((item) => allowed.has(item.code))?.path ?? '/login'
}

interface PermissionDeniedRecoveryInput {
  backendMode: boolean
  isAdmin: boolean
  isSubAdmin: boolean
  permissions: readonly string[] | null | undefined
}

export function resolveAdminPermissionDeniedRecovery(
  input: PermissionDeniedRecoveryInput,
): { target: string; logout: boolean } {
  if (input.isAdmin) {
    return { target: '/admin/dashboard', logout: false }
  }
  if (input.isSubAdmin) {
    const target = getAdminLandingPath(input.permissions, input.backendMode)
    return { target, logout: input.backendMode && target === '/login' }
  }
  return {
    target: input.backendMode ? '/login' : '/dashboard',
    logout: input.backendMode,
  }
}
