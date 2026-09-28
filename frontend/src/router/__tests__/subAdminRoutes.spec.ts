import { describe, expect, it } from 'vitest'

import router from '../index'

describe('sub-admin route permission metadata', () => {
  it.each([
    ['AdminSubscriptions', 'admin.subscriptions'],
    ['AdminUsage', 'admin.usage'],
    ['AdminTokenAnalysis', 'admin.token_analysis'],
  ])('registers %s with %s', (name, permission) => {
    const route = router.getRoutes().find((item) => item.name === name)

    expect(route?.meta).toMatchObject({
      requiresAuth: true,
      requiresAdmin: true,
      adminPermission: permission,
    })
  })

  it('keeps the GPT quota display admin page full-admin only', () => {
    const route = router.getRoutes().find((item) => item.name === 'AdminGPTQuotaDisplay')

    // 无 adminPermission 时，路由守卫会把子管理员重定向到其授权落地页。
    expect(route?.meta).toMatchObject({ requiresAuth: true, requiresAdmin: true })
    expect(route?.meta.adminPermission).toBeUndefined()
  })
})
