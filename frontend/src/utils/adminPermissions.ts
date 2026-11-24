import type { AdminPermission } from '@/types'

export const ADMIN_PERMISSION_SUBSCRIPTIONS: AdminPermission = 'admin.subscriptions'
export const ADMIN_PERMISSION_USAGE: AdminPermission = 'admin.usage'
export const ADMIN_PERMISSION_TOKEN_ANALYSIS: AdminPermission = 'admin.token_analysis'
export const ADMIN_PERMISSION_ORGANIZATION_USAGE: AdminPermission = 'admin.organization_usage'
export const ADMIN_PERMISSION_DEPARTMENT_SUBSCRIPTIONS: AdminPermission = 'admin.department_subscriptions'
export const ADMIN_PERMISSION_DEPARTMENT_MEMBERS: AdminPermission = 'admin.department_members'
export const ADMIN_PERMISSION_ORGANIZATION_MANAGE: AdminPermission = 'admin.organization_manage'
export const ADMIN_PERMISSION_ACCOUNTS_OPERATOR: AdminPermission = 'admin.accounts_operator'
export const ADMIN_PERMISSION_GPT_IQ_TEST: AdminPermission = 'admin.gpt_iq_test'

const ADMIN_PERMISSION_LANDING_ROUTES: ReadonlyArray<{
  code: AdminPermission
  path: string
}> = [
  // Organization leaders land on department management, their distinguishing duty.
  { code: ADMIN_PERMISSION_ORGANIZATION_MANAGE, path: '/admin/departments' },
  { code: ADMIN_PERMISSION_SUBSCRIPTIONS, path: '/admin/subscriptions' },
  { code: ADMIN_PERMISSION_USAGE, path: '/admin/usage' },
  { code: ADMIN_PERMISSION_TOKEN_ANALYSIS, path: '/admin/token-analysis' },
  { code: ADMIN_PERMISSION_ORGANIZATION_USAGE, path: '/admin/organization-usage' },
  { code: ADMIN_PERMISSION_DEPARTMENT_SUBSCRIPTIONS, path: '/admin/subscriptions' },
  { code: ADMIN_PERMISSION_DEPARTMENT_MEMBERS, path: '/admin/my-department' },
  { code: ADMIN_PERMISSION_ACCOUNTS_OPERATOR, path: '/admin/accounts' },
  // Last so organization leaders (who can also open this page) keep landing on departments.
  { code: ADMIN_PERMISSION_GPT_IQ_TEST, path: '/admin/gpt-iq-test' },
]

export function getAdminLandingPath(
  permissions: readonly string[] | null | undefined,
  backendMode: boolean,
): string {
	const allowed = new Set(permissions ?? [])
	const isDepartmentCode = (code: string) =>
		code === ADMIN_PERMISSION_ORGANIZATION_USAGE || code === ADMIN_PERMISSION_DEPARTMENT_SUBSCRIPTIONS || code === ADMIN_PERMISSION_DEPARTMENT_MEMBERS || code === ADMIN_PERMISSION_ORGANIZATION_MANAGE
	// 降智测试配置不改变部门负责人的落地页：持有部门能力时它不算"其他权限"；只持有它时仍按普通子管理员处理。
	const onlyDepartmentManagement = [...allowed].some(isDepartmentCode) && [...allowed].every(
		code => isDepartmentCode(code) || code === ADMIN_PERMISSION_GPT_IQ_TEST,
	)
  if (!backendMode && !onlyDepartmentManagement) return '/dashboard'
  return ADMIN_PERMISSION_LANDING_ROUTES.find((item) => allowed.has(item.code))?.path ?? '/login'
}

export function departmentLoginDestination(
  destination: string,
  source: string,
  explicitRedirect: unknown,
  permissions: readonly string[] | null | undefined,
  backendMode: boolean,
): string {
  const fromAuthentication = source === '/login' || source === '/email-verify' || source.startsWith('/auth/')
  if (destination !== '/dashboard' || !fromAuthentication || (typeof explicitRedirect === 'string' && explicitRedirect !== '')) return destination
  return getAdminLandingPath(permissions, backendMode)
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
