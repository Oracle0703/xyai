import { isAxiosError } from 'axios'

// Domain errors use {code: HTTP status, reason: business code}; middleware
// errors can use a string code. api/client preserves both envelope formats.
export function departmentErrorStatus(error: unknown): number | undefined {
  if (error && typeof error === 'object' && 'status' in error && typeof error.status === 'number') return error.status
  return isAxiosError(error) ? error.response?.status : undefined
}

export function isDepartmentScopeChanged(error: unknown): boolean {
  return departmentErrorCode(error) === 'REPORT_SCOPE_CHANGED' || (error instanceof Error && error.message === 'REPORT_SCOPE_CHANGED')
}

export function departmentErrorCode(error: unknown): string | undefined {
  if (isAxiosError(error)) {
    const body = error.response?.data
    if (typeof body?.reason === 'string') return body.reason
    if (typeof body?.code === 'string') return body.code
  }
  if (error && typeof error === 'object' && 'reason' in error && typeof error.reason === 'string') return error.reason
  if (error && typeof error === 'object' && 'code' in error && typeof error.code === 'string') return error.code
  return undefined
}

export function departmentErrorKey(error: unknown): string {
  const keys: Record<string, string> = {
    ADMIN_ACCESS_CHANGED: 'accessChanged', ADMIN_ACCESS_VERSION_REQUIRED: 'accessVersionRequired',
    DEPARTMENT_DUPLICATE: 'duplicateName', DEPARTMENT_CONFLICT: 'conflict',
    REPORT_SCOPE_CHANGED: 'scopeChanged', DEPARTMENT_INACTIVE: 'inactiveTarget',
    DEPARTMENT_MEMBER_ORGANIZATION_MISMATCH: 'organizationMismatch',
    DEPARTMENT_GLOBAL_SUBSCRIPTION_CONFIRMATION_REQUIRED: 'globalConfirmationRequired',
    DEPARTMENT_SCOPE_DENIED: 'scopeDenied'
  }
  return `admin.departments.${keys[departmentErrorCode(error) ?? ''] ?? 'failed'}`
}
