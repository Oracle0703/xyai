import { describe, expect, it } from 'vitest'

import router from '../index'

describe('organization usage admin route', () => {
  it('registers the protected report page with localized metadata', async () => {
    const route = router.getRoutes().find((item) => item.name === 'AdminOrganizationUsage')

    expect(route).toBeDefined()
    expect(route?.path).toBe('/admin/organization-usage')
    expect(route?.meta).toMatchObject({
      requiresAuth: true,
      requiresAdmin: true,
      titleKey: 'admin.organizationUsage.title',
      descriptionKey: 'admin.organizationUsage.description'
    })

    const component = route?.components?.default
    expect(typeof component).toBe('function')
    await expect((component as () => Promise<unknown>)()).resolves.toBeDefined()
  })
})
