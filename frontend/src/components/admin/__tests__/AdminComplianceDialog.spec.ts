import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AdminComplianceDialog from '../AdminComplianceDialog.vue'

const authState = vi.hoisted(() => ({
  isAuthenticated: true,
  isAdmin: false,
  isSubAdmin: true,
  canAccessAdmin: true,
  logout: vi.fn()
}))

const complianceState = vi.hoisted(() => ({
  shouldShow: true,
  expectedPhrase: 'I accept',
  submitting: false,
  status: null,
  accept: vi.fn()
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => authState,
  useAdminComplianceStore: () => complianceState,
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() })
}))

vi.mock('@/i18n', () => ({ getLocale: () => 'zh' }))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

describe('AdminComplianceDialog', () => {
  it('is visible for an authenticated sub admin with management access', () => {
    const wrapper = mount(AdminComplianceDialog, {
      global: {
        stubs: {
          BaseDialog: {
            props: ['show'],
            template: '<div data-test="compliance-dialog" :data-show="String(show)"><slot /><slot name="footer" /></div>'
          },
          Input: true,
          Icon: true
        }
      }
    })

    expect(wrapper.get('[data-test="compliance-dialog"]').attributes('data-show')).toBe('true')
  })
})
