import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UserCreateModal from '../UserCreateModal.vue'
import UserEditModal from '../UserEditModal.vue'

const api = vi.hoisted(() => ({
  create: vi.fn(),
  update: vi.fn(),
  getPermissionCatalog: vi.fn(),
  updateUserAttributeValues: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      create: api.create,
      update: api.update,
      getPermissionCatalog: api.getPermissionCatalog,
    },
    userAttributes: { updateUserAttributeValues: api.updateUserAttributeValues },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const BaseDialogStub = {
  template: '<div><slot /><slot name="footer" /></div>',
}

describe('admin user role permission forms', () => {
  beforeEach(() => {
    api.create.mockReset()
    api.update.mockReset()
    api.getPermissionCatalog.mockReset()
    api.updateUserAttributeValues.mockReset()
    api.create.mockResolvedValue({})
    api.update.mockResolvedValue({})
    api.getPermissionCatalog.mockResolvedValue([
      { code: 'admin.subscriptions', menu_key: 'subscriptions', route: '/admin/subscriptions' },
      { code: 'admin.usage', menu_key: 'usage', route: '/admin/usage' },
      { code: 'admin.token_analysis', menu_key: 'token_analysis', route: '/admin/token-analysis' },
    ])
  })

  it('creates a sub-admin with selected permissions', async () => {
    const wrapper = mount(UserCreateModal, {
      props: { show: true },
      global: { stubs: { BaseDialog: BaseDialogStub, Icon: true } },
    })

    await flushPromises()
    expect(api.getPermissionCatalog).toHaveBeenCalledTimes(1)

    await wrapper.get('[data-testid="role-select"]').setValue('sub_admin')
    expect(wrapper.findAll('[data-admin-permission]').length).toBe(3)
    await wrapper.get('[data-admin-permission="admin.usage"]').setValue(true)
    await wrapper.get('input[type="email"]').setValue('sub@example.com')
    await wrapper.get('input[placeholder="admin.users.enterPassword"]').setValue('pass123')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(api.create).toHaveBeenCalledWith(
      expect.objectContaining({
        role: 'sub_admin',
        admin_permissions: ['admin.usage'],
      }),
    )
  })

  it('edits and clears stale permissions when leaving sub-admin role', async () => {
    const wrapper = mount(UserEditModal, {
      props: {
        show: true,
        user: {
          id: 9,
          email: 'sub@example.com',
          username: 'sub',
          notes: '',
          role: 'sub_admin',
          admin_permissions: ['admin.usage'],
          concurrency: 1,
          rpm_limit: 0,
          status: 'active',
        } as any,
      },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          UserAttributeForm: true,
          Icon: true,
        },
      },
    })

    await flushPromises()
    expect(api.getPermissionCatalog).toHaveBeenCalledTimes(1)

    expect((wrapper.get('[data-admin-permission="admin.usage"]').element as HTMLInputElement).checked).toBe(true)
    await wrapper.get('[data-testid="role-select"]').setValue('user')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(api.update).toHaveBeenCalledWith(
      9,
      expect.objectContaining({ role: 'user', admin_permissions: [] }),
    )
  })
})
