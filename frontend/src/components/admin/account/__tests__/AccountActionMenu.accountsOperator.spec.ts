import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountActionMenu from '../AccountActionMenu.vue'
import type { Account } from '@/types'

vi.mock('@/composables/useAccountOperatorAccess', async () => {
  const { ref } = await import('vue')
  return { useAccountOperatorAccess: () => ({ canManageAccounts: ref(false) }) }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const account = {
  id: 1,
  name: 'limited-oauth',
  platform: 'openai',
  type: 'oauth',
  parent_account_id: null,
  status: 'error',
  schedulable: true,
  rate_limit_reset_at: new Date(Date.now() + 60_000).toISOString(),
  extra: { privacy_mode: 'off', quota_limit: 10 },
} as unknown as Account

describe('AccountActionMenu — restricted account operator', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('only offers test connection, stats and re-authorization', () => {
    const wrapper = mount(AccountActionMenu, {
      props: { show: true, account, anchorRect: new DOMRect(100, 100, 24, 24) },
      global: { stubs: { Icon: true } },
    })

    const labels = Array.from(document.body.querySelectorAll('.action-menu-content button')).map(button => button.textContent?.trim())
    expect(labels).toEqual([
      'admin.accounts.testConnection',
      'admin.accounts.viewStats',
      'admin.accounts.reAuthorize',
    ])
    wrapper.unmount()
  })
})
