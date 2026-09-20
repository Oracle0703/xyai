import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UserEditModal from '../UserEditModal.vue'

const { update, getById, updateUserAttributeValues, showSuccess, showError } = vi.hoisted(() => ({
  update: vi.fn(),
  getById: vi.fn(),
  updateUserAttributeValues: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { update, getById, getPermissionCatalog: vi.fn().mockResolvedValue([]) },
    userAttributes: { updateUserAttributeValues }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() })
}))

// useStepUp pulls in the API client, which needs the real i18n instance.
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${JSON.stringify(params)}` : key
  })
}))

const mountModal = async (concurrency: number) => {
  getById.mockResolvedValue({ id: 7, email: 'user@example.test', username: 'user', notes: '', role: 'user', concurrency, rpm_limit: 0, admin_permissions: [], admin_access_version: 'access-v1' })
  const wrapper = mount(UserEditModal, {
  props: {
    show: true,
    user: { id: 7, email: 'user@example.test', username: 'user', notes: '', role: 'user', concurrency, rpm_limit: 0 } as never
  },
  global: {
    stubs: {
      BaseDialog: {
        props: ['show', 'title'],
        template: '<div v-if="show"><slot /><slot name="footer" /></div>'
      },
      Select: true,
      Icon: true,
      UserAttributeForm: true,
      TotpStepUpDialog: true
    }
  }
  })
  await flushPromises()
  return wrapper
}

describe('UserEditModal concurrency', () => {
  beforeEach(() => {
    update.mockReset()
    getById.mockReset()
    updateUserAttributeValues.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    update.mockResolvedValue({})
  })

  // Regression coverage for issue #5977: the gateway treats concurrency <= 0 as
  // unlimited (AcquireUserSlot) and both the batch limits endpoint and the bulk
  // edit modal accept 0, so this dialog must not be the only place that rejects
  // it — doing so blocked every other edit on such a user.
  it('saves an unlimited (0) concurrency instead of blocking the whole form', async () => {
    const wrapper = await mountModal(0)
    await wrapper.get('textarea').setValue('updated notes')

    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(showError).not.toHaveBeenCalled()
    expect(update).toHaveBeenCalledWith(7, { notes: 'updated notes' })
    expect(wrapper.emitted('success')).toBeTruthy()
  })

  it('still rejects a negative concurrency', async () => {
    const wrapper = await mountModal(3)

    await wrapper.get('[data-test="concurrency-input"]').setValue('-1')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.users.concurrencyNonNegative')
    expect(update).not.toHaveBeenCalled()
  })

  it('loads a fresh detail and sends no role, permission or access version when only notes change', async () => {
    const wrapper = await mountModal(3)
    expect(getById).toHaveBeenCalledWith(7)
    await wrapper.get('textarea').setValue('notes only')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(7, { notes: 'notes only' })
    wrapper.unmount()
  })

  it('binds a role change to the loaded access version', async () => {
    const wrapper = await mountModal(3)
    wrapper.getComponent('[data-testid="role-select"]').vm.$emit('update:modelValue', 'sub_admin')
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(7, { role: 'sub_admin', admin_permissions: [], expected_admin_access_version: 'access-v1' })
    wrapper.unmount()
  })

  it('blocks saving instead of falling back to the list snapshot when detail loading fails', async () => {
    getById.mockRejectedValueOnce(new Error('detail failed'))
    const wrapper = await mountModal(3)
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[role="alert"]').text()).toBe('admin.departments.userDetailsFailed')
    expect(update).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('refreshes after an access conflict and never automatically repeats the rejected write', async () => {
    const wrapper = await mountModal(3)
    wrapper.getComponent('[data-testid="role-select"]').vm.$emit('update:modelValue', 'sub_admin')
    update.mockRejectedValueOnce({ status: 409, code: 409, reason: 'ADMIN_ACCESS_CHANGED' })
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(update).toHaveBeenCalledTimes(1)
    expect(getById).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[role="alert"]').text()).toBe('admin.departments.accessChanged')
    expect(wrapper.emitted('success')).toBeUndefined()
    wrapper.unmount()
  })
})
