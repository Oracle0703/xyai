import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import SubscriptionsView from '../SubscriptionsView.vue'

const mocks = vi.hoisted(() => ({ list: vi.fn(), status: vi.fn(), reset: vi.fn(), refresh: vi.fn(), push: vi.fn(), showError: vi.fn() }))
vi.mock('@/api/subscriptions', () => ({ default: { getMySubscriptions: mocks.list } }))
vi.mock('@/api/subscriptionSelfReset', () => ({ getSelfResetStatus: mocks.status, resetSubscriptionDaily: mocks.reset }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: { id: 7 } }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: mocks.showError, cachedPublicSettings: null }) }))
vi.mock('@/stores/subscriptions', () => ({ useSubscriptionStore: () => ({ fetchActiveSubscriptions: mocks.refresh }) }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: mocks.push }) }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string, args?: Record<string, unknown>) => args ? `${key}:${JSON.stringify(args)}` : key, te: () => true }) }))
let wrapper: ReturnType<typeof mount>
const row = { id: 31, group_id: 21, status: 'active', starts_at: '2026-01-01T00:00:00Z', expires_at: '2036-01-01T00:00:00Z', daily_usage_usd: 80, weekly_usage_usd: 90, monthly_usage_usd: 95, group: { name: 'Long subscription', platform: 'openai', daily_limit_usd: 100 } }
const snapshot = (count = 1, reason: string | null = null) => ({ organization: 'xunyou', daily_limit: 1, quota_date: '2026-09-23', server_now: '2026-09-23T12:00:00+08:00', next_reset_at: '2026-09-24T00:00:00+08:00', subscriptions: [{ subscription_id: 31, used_count: 1-count, remaining_count: count, can_reset: count > 0 && !reason, disabled_reason: reason }] })
beforeEach(() => {
  vi.clearAllMocks()
  sessionStorage.clear()
  vi.spyOn(console, 'error').mockImplementation(() => {})
  mocks.list.mockResolvedValue([row])
  mocks.status.mockResolvedValue(snapshot())
  mocks.reset.mockResolvedValue({})
  mocks.refresh.mockResolvedValue([])
})
afterEach(() => wrapper?.unmount())
function create() {
  wrapper = mount(SubscriptionsView, { global: { stubs: {
    AppLayout: { template: '<main><slot /></main>' }, Icon: true,
    ConfirmDialog: { props: ['show', 'disabled', 'message'], emits: ['confirm', 'cancel'], template: '<div v-if="show"><p>{{ message }}</p><button data-confirm :disabled="disabled" @click="$emit(\'confirm\')">confirm</button><slot /></div>' }
  } } })
}
describe('user subscription self-reset button', () => {
  it('appears after renewal and becomes gray/disabled with zero after a successful reset', async () => {
    create()
    await flushPromises()
    const reset = wrapper.get('[data-self-reset="31"]')
    expect(reset.element.previousElementSibling?.textContent).toContain('payment.renewNow')
    expect(reset.text()).toContain('"count":1')
    expect(reset.attributes('disabled')).toBeUndefined()
    await reset.trigger('click')
    expect(wrapper.text()).toContain('"id":31')
    mocks.status.mockResolvedValue(snapshot(0, 'DAILY_LIMIT_REACHED'))
    mocks.list.mockResolvedValue([{ ...row, daily_usage_usd: 0 }])
    await wrapper.get('[data-confirm]').trigger('click')
    await flushPromises()
    expect(reset.text()).toContain('"count":0')
    expect(reset.attributes('disabled')).toBeDefined()
    expect(reset.classes()).toContain('disabled:opacity-40')
    expect(wrapper.text()).toContain('$0.00 / $100.00')
    expect(mocks.reset).toHaveBeenCalledWith(31, '2026-09-23', expect.any(String))
  })
  it('keeps one-time passes disabled even when their organization has opportunities', async () => {
    mocks.status.mockResolvedValue(snapshot(1, 'ONE_TIME_QUOTA'))
    create()
    await flushPromises()
    expect(wrapper.get('[data-self-reset="31"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('userSubscriptions.selfReset.ONE_TIME_QUOTA')
    expect(mocks.reset).not.toHaveBeenCalled()
  })

  it('reports a failed list reload without discarding the reset status', async () => {
    create()
    await flushPromises()
    await wrapper.get('[data-self-reset="31"]').trigger('click')
    mocks.status.mockResolvedValue(snapshot(0, 'DAILY_LIMIT_REACHED'))
    mocks.list.mockRejectedValueOnce(new Error('list failed'))
    await wrapper.get('[data-confirm]').trigger('click')
    await flushPromises()
    expect(mocks.showError).toHaveBeenCalledWith('userSubscriptions.failedToLoad')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.get('[data-self-reset="31"]').text()).toContain('"count":0')
    expect(wrapper.get('[data-self-reset="31"]').attributes('disabled')).toBeDefined()
    expect(mocks.refresh).toHaveBeenCalledOnce()
    expect(mocks.reset).toHaveBeenCalledOnce()
  })

  it('hides the button until status loads, shows a retryable error when it fails, and retries it', async () => {
    mocks.status.mockRejectedValueOnce(new Error('offline'))
    create()
    await flushPromises()
    expect(wrapper.find('[data-self-reset="31"]').exists()).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toContain('userSubscriptions.selfReset.statusFailed')
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.get('[data-self-reset="31"]').attributes('disabled')).toBeUndefined()
  })

  it('shows nothing about self reset to accounts outside the rollout', async () => {
    mocks.status.mockResolvedValue(snapshot(1, 'ROLLOUT_DISABLED'))
    create()
    await flushPromises()
    expect(wrapper.text()).toContain('payment.renewNow')
    expect(wrapper.find('[data-self-reset="31"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('userSubscriptions.selfReset')
  })
})
