import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

import type { Group, User, UserSubscription } from '@/types'
import SubscriptionsView from '../SubscriptionsView.vue'

const { listSubscriptions, resetQuota, searchSubscriptionGroups, getAllGroups, searchUsers, showError, showSuccess } = vi.hoisted(() => ({
  listSubscriptions: vi.fn(),
  resetQuota: vi.fn(),
  searchSubscriptionGroups: vi.fn(),
  getAllGroups: vi.fn(),
  searchUsers: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

const authState = vi.hoisted(() => ({
  isAdmin: true,
  isSubAdmin: false
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    subscriptions: {
      list: listSubscriptions,
      assign: vi.fn(),
      extend: vi.fn(),
      revoke: vi.fn(),
      resetQuota,
      searchGroups: searchSubscriptionGroups
    },
    groups: {
      getAll: getAllGroups
    },
    usage: {
      searchUsers
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | undefined>) =>
        params?.user ? `${key}:${params.user}` : key
    })
  }
})

const testUser: User = {
  id: 11,
  username: 'daily-user',
  email: 'daily@example.com',
  role: 'user',
  balance: 0,
  concurrency: 1,
  status: 'active',
  allowed_groups: [],
  balance_notify_enabled: false,
  balance_notify_threshold: null,
  balance_notify_extra_emails: [],
  created_at: '2026-05-01T00:00:00Z',
  updated_at: '2026-05-01T00:00:00Z'
}

const testGroup: Group = {
  id: 21,
  name: 'Subscription Group',
  description: null,
  platform: 'openai',
  rate_multiplier: 1,
  is_exclusive: false,
  status: 'active',
  subscription_type: 'subscription',
  daily_limit_usd: 10,
  weekly_limit_usd: 50,
  monthly_limit_usd: 100,
  allow_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '2026-05-01T00:00:00Z',
  updated_at: '2026-05-01T00:00:00Z'
}

const testSubscription: UserSubscription = {
  id: 31,
  user_id: testUser.id,
  group_id: testGroup.id,
  status: 'active',
  starts_at: '2026-05-01T00:00:00Z',
  daily_usage_usd: 7,
  weekly_usage_usd: 20,
  monthly_usage_usd: 80,
  daily_window_start: '2026-05-27T00:00:00Z',
  weekly_window_start: '2026-05-23T00:00:00Z',
  monthly_window_start: '2026-05-01T00:00:00Z',
  created_at: '2026-05-01T00:00:00Z',
  updated_at: '2026-05-27T00:00:00Z',
  expires_at: '2026-06-01T00:00:00Z',
  user: testUser,
  group: testGroup
}

const DataTableStub = defineComponent({
  props: ['data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-actions" :row="row" />
      </div>
    </div>
  `
})

const ConfirmDialogStub = defineComponent({
  props: ['show', 'title', 'message', 'confirmText', 'cancelText'],
  emits: ['confirm', 'cancel'],
  template: `
    <div v-if="show" data-test="confirm-dialog">
      <div data-test="confirm-title">{{ title }}</div>
      <div data-test="confirm-message">{{ message }}</div>
      <button data-test="confirm" @click="$emit('confirm')">{{ confirmText }}</button>
      <button data-test="cancel" @click="$emit('cancel')">{{ cancelText }}</button>
    </div>
  `
})

const mountView = async () => {
  const wrapper = mount(SubscriptionsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: {
          template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
        },
        DataTable: DataTableStub,
        Pagination: true,
        BaseDialog: true,
        ConfirmDialog: ConfirmDialogStub,
        EmptyState: true,
        Select: true,
        GroupBadge: true,
        GroupOptionItem: true,
        Icon: true,
        RouterLink: true,
        Teleport: true
      }
    }
  })
  await flushPromises()
  return wrapper
}

const findButtonByText = (wrapper: Awaited<ReturnType<typeof mountView>>, text: string) => {
  const button = wrapper.findAll('button').find((item) => item.text() === text)
  expect(button, `button ${text} should exist`).toBeTruthy()
  return button!
}

describe('admin SubscriptionsView quota reset actions', () => {
  beforeEach(() => {
    localStorage.clear()
    listSubscriptions.mockReset()
    resetQuota.mockReset()
    searchSubscriptionGroups.mockReset()
    getAllGroups.mockReset()
    searchUsers.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    authState.isAdmin = true
    authState.isSubAdmin = false

    listSubscriptions.mockResolvedValue({
      items: [testSubscription],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    resetQuota.mockResolvedValue({ ...testSubscription, daily_usage_usd: 0 })
    searchSubscriptionGroups.mockResolvedValue([{
      id: testGroup.id,
      name: testGroup.name,
      platform: testGroup.platform,
      subscription_type: testGroup.subscription_type
    }])
    getAllGroups.mockResolvedValue([testGroup])
    searchUsers.mockResolvedValue([])
  })

  it('resets only the daily quota when Reset Daily is confirmed', async () => {
    const wrapper = await mountView()

    await findButtonByText(wrapper, 'admin.subscriptions.resetDailyQuota').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="confirm-title"]').text()).toBe('admin.subscriptions.resetDailyQuotaTitle')
    expect(wrapper.get('[data-test="confirm-message"]').text()).toBe(
      'admin.subscriptions.resetDailyQuotaConfirm:daily@example.com'
    )

    await wrapper.get('[data-test="confirm"]').trigger('click')
    await flushPromises()

    expect(resetQuota).toHaveBeenCalledWith(31, { daily: true, weekly: false, monthly: false })
    expect(showSuccess).toHaveBeenCalledWith('admin.subscriptions.quotaResetSuccess')
  })

  it('keeps the existing Reset Quota action resetting all windows', async () => {
    const wrapper = await mountView()

    await findButtonByText(wrapper, 'admin.subscriptions.resetQuota').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="confirm-title"]').text()).toBe('admin.subscriptions.resetQuotaTitle')

    await wrapper.get('[data-test="confirm"]').trigger('click')
    await flushPromises()

    expect(resetQuota).toHaveBeenCalledWith(31, { daily: true, weekly: true, monthly: true })
  })

  it('shows sub admins only the two permitted quota reset actions', async () => {
    authState.isAdmin = false
    authState.isSubAdmin = true

    const wrapper = await mountView()
    const buttonTexts = wrapper.findAll('button').map((button) => button.text())

    expect(buttonTexts).toContain('admin.subscriptions.resetQuota')
    expect(buttonTexts).toContain('admin.subscriptions.resetDailyQuota')
    expect(buttonTexts).not.toContain('admin.subscriptions.assignSubscription')
    expect(buttonTexts).not.toContain('admin.subscriptions.adjust')
    expect(buttonTexts).not.toContain('admin.subscriptions.revoke')
    expect(buttonTexts).not.toContain('admin.subscriptions.restore')
    expect(searchSubscriptionGroups).toHaveBeenCalledTimes(1)
    expect(getAllGroups).not.toHaveBeenCalled()
  })
})
