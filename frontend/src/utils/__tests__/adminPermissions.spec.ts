import { describe, expect, it } from 'vitest'

import {
  ADMIN_PERMISSION_SUBSCRIPTIONS,
  ADMIN_PERMISSION_TOKEN_ANALYSIS,
  ADMIN_PERMISSION_USAGE,
  getAdminLandingPath,
  resolveAdminPermissionDeniedRecovery,
} from '@/utils/adminPermissions'

describe('sub-admin landing path', () => {
  it('uses the user dashboard in standard mode', () => {
    expect(getAdminLandingPath([ADMIN_PERMISSION_USAGE], false)).toBe('/dashboard')
  })

  it('uses the first catalog permission in backend mode', () => {
    expect(getAdminLandingPath([ADMIN_PERMISSION_TOKEN_ANALYSIS, ADMIN_PERMISSION_USAGE], true)).toBe('/admin/usage')
    expect(getAdminLandingPath([ADMIN_PERMISSION_SUBSCRIPTIONS], true)).toBe('/admin/subscriptions')
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
