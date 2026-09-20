import { isAxiosError } from 'axios'

// api/client normalizes HTTP failures into { status, code, message } objects.
export function departmentErrorStatus(error: unknown): number | undefined {
  if (error && typeof error === 'object' && 'status' in error && typeof error.status === 'number') return error.status
  return isAxiosError(error) ? error.response?.status : undefined
}

export function isDepartmentScopeChanged(error: unknown): boolean {
  if (departmentErrorStatus(error) === 409) return true
  if (!error || typeof error !== 'object') return false
  return ('code' in error && error.code === 'REPORT_SCOPE_CHANGED') || ('message' in error && error.message === 'REPORT_SCOPE_CHANGED')
}
