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
})
