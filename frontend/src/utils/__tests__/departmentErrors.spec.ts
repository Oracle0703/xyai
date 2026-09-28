import { describe, expect, it } from 'vitest'
import { departmentErrorKey, isDepartmentScopeChanged } from '../departmentErrors'

describe('department error semantics', () => {
  it.each([
    ['DEPARTMENT_DUPLICATE', 'duplicateName'],
    ['DEPARTMENT_CONFLICT', 'conflict'],
    ['ADMIN_ACCESS_CHANGED', 'accessChanged'],
    ['DEPARTMENT_INACTIVE', 'inactiveTarget'],
    ['DEPARTMENT_MEMBER_ORGANIZATION_MISMATCH', 'organizationMismatch'],
    ['DEPARTMENT_GLOBAL_SUBSCRIPTION_CONFIRMATION_REQUIRED', 'globalConfirmationRequired']
  ])('does not mistake %s for a changed report scope', (code, key) => {
    expect(isDepartmentScopeChanged({ status: 409, code: 409, reason: code })).toBe(false)
    expect(departmentErrorKey({ status: 409, code: 409, reason: code })).toBe(`admin.departments.${key}`)
  })
  it('accepts the report code and local snapshot mismatch, but not an untyped 409', () => {
    expect(isDepartmentScopeChanged({ status: 409 })).toBe(false)
    expect(isDepartmentScopeChanged({ status: 409, code: 'REPORT_SCOPE_CHANGED' })).toBe(true)
    expect(isDepartmentScopeChanged({ status: 409, code: 409, reason: 'REPORT_SCOPE_CHANGED' })).toBe(true)
    expect(isDepartmentScopeChanged({ isAxiosError: true, code: 'ERR_BAD_REQUEST', response: { status:409, data:{code:409,reason:'REPORT_SCOPE_CHANGED'} } })).toBe(true)
    expect(isDepartmentScopeChanged(new Error('REPORT_SCOPE_CHANGED'))).toBe(true)
  })
})
