import { describe, expect, it } from 'vitest'

import {
  ADMIN_PERMISSION_GPT_IQ_TEST,
  ADMIN_PERMISSION_SUBSCRIPTIONS,
  ADMIN_PERMISSION_TOKEN_ANALYSIS,
  ADMIN_PERMISSION_USAGE,
  getAdminLandingPath,
  resolveAdminPermissionDeniedRecovery,
  departmentLoginDestination,
} from '@/utils/adminPermissions'

describe('sub-admin landing path', () => {
  it.each([false, true])('lands member-only managers on their department page (backend mode %s)', (backendMode) => {
    expect(getAdminLandingPath(['admin.department_members'], backendMode)).toBe('/admin/my-department')
    expect(getAdminLandingPath(['admin.department_members', 'admin.organization_usage'], backendMode)).toBe('/admin/organization-usage')
    expect(getAdminLandingPath(['admin.department_members', 'admin.department_subscriptions'], backendMode)).toBe('/admin/subscriptions')
    expect(getAdminLandingPath(['admin.department_members', 'admin.department_subscriptions', 'admin.organization_usage'], backendMode)).toBe('/admin/organization-usage')
    expect(departmentLoginDestination('/dashboard', '/login', undefined, ['admin.department_members'], backendMode)).toBe('/admin/my-department')
    expect(departmentLoginDestination('/dashboard', '/login', '/dashboard', ['admin.department_members'], backendMode)).toBe('/dashboard')
  })
  it.each([false, true])('lands organization leaders on department management (backend mode %s)', (backendMode) => {
    const permissions = ['admin.organization_manage', 'admin.organization_usage', 'admin.department_subscriptions', 'admin.department_members']
    expect(getAdminLandingPath(permissions, backendMode)).toBe('/admin/departments')
    expect(departmentLoginDestination('/dashboard', '/login', undefined, permissions, backendMode)).toBe('/admin/departments')
  })
  it('lands department managers on reports in standard mode and preserves explicit destinations', () => {
    const permissions = ['admin.organization_usage', 'admin.department_subscriptions']
    expect(departmentLoginDestination('/dashboard', '/login', undefined, permissions, false)).toBe('/admin/organization-usage')
    expect(departmentLoginDestination('/dashboard', '/auth/oidc/callback', undefined, permissions, false)).toBe('/admin/organization-usage')
    expect(departmentLoginDestination('/dashboard', '/login', '/dashboard', permissions, false)).toBe('/dashboard')
    expect(departmentLoginDestination('/dashboard', '/admin/subscriptions', undefined, permissions, false)).toBe('/dashboard')
    expect(getAdminLandingPath(['admin.department_subscriptions'], false)).toBe('/admin/subscriptions')
  })
  it('uses the user dashboard in standard mode', () => {
    expect(getAdminLandingPath([ADMIN_PERMISSION_USAGE], false)).toBe('/dashboard')
  })

  it('uses the first catalog permission in backend mode', () => {
    expect(getAdminLandingPath([ADMIN_PERMISSION_TOKEN_ANALYSIS, ADMIN_PERMISSION_USAGE], true)).toBe('/admin/usage')
    expect(getAdminLandingPath([ADMIN_PERMISSION_SUBSCRIPTIONS], true)).toBe('/admin/subscriptions')
    expect(getAdminLandingPath(['admin.accounts_operator'], true)).toBe('/admin/accounts')
  })

  it('lands a sub-admin holding only the IQ test permission on its config page', () => {
    expect(getAdminLandingPath([ADMIN_PERMISSION_GPT_IQ_TEST], true)).toBe('/admin/gpt-iq-test')
    // 标准模式下与其他非部门权限一致，进入个人 dashboard。
    expect(getAdminLandingPath([ADMIN_PERMISSION_GPT_IQ_TEST], false)).toBe('/dashboard')
    // 其他权限优先；组织负责人仍落到部门管理。
    expect(getAdminLandingPath([ADMIN_PERMISSION_GPT_IQ_TEST, ADMIN_PERMISSION_USAGE], true)).toBe('/admin/usage')
    expect(getAdminLandingPath(['admin.organization_manage', 'admin.organization_usage', 'admin.department_subscriptions', 'admin.department_members'], true)).toBe('/admin/departments')
    // 负责人另被授予降智测试配置时，普通模式与后台模式都仍落地部门管理。
    for (const backendMode of [false, true]) {
      expect(getAdminLandingPath(['admin.organization_manage', 'admin.organization_usage', 'admin.department_subscriptions', 'admin.department_members', ADMIN_PERMISSION_GPT_IQ_TEST], backendMode)).toBe('/admin/departments')
      expect(getAdminLandingPath(['admin.department_members', ADMIN_PERMISSION_GPT_IQ_TEST], backendMode)).toBe('/admin/my-department')
    }
    expect(resolveAdminPermissionDeniedRecovery({ backendMode: true, isAdmin: false, isSubAdmin: true, permissions: [ADMIN_PERMISSION_GPT_IQ_TEST] }))
      .toEqual({ target: '/admin/gpt-iq-test', logout: false })
  })

  it('falls back to login in backend mode when no permission remains', () => {
    expect(getAdminLandingPath([], true)).toBe('/login')
  })
})

describe('sub-admin permission denial recovery', () => {
  it('logs out a backend-mode sub admin after all permissions are revoked', () => {
    expect(resolveAdminPermissionDeniedRecovery({
      backendMode: true,
      isAdmin: false,
      isSubAdmin: true,
      permissions: [],
    })).toEqual({ target: '/login', logout: true })
  })

  it('keeps the session when an authorized backend landing page remains', () => {
    expect(resolveAdminPermissionDeniedRecovery({
      backendMode: true,
      isAdmin: false,
      isSubAdmin: true,
      permissions: [ADMIN_PERMISSION_USAGE],
    })).toEqual({ target: '/admin/usage', logout: false })
  })

  it('keeps standard-mode sub admins signed in on the user dashboard', () => {
    expect(resolveAdminPermissionDeniedRecovery({
      backendMode: false,
      isAdmin: false,
      isSubAdmin: true,
      permissions: [],
    })).toEqual({ target: '/dashboard', logout: false })
  })
})
