import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { useSubscriptionSelfReset } from '../useSubscriptionSelfReset'

const mocks = vi.hoisted(() => ({ status: vi.fn(), reset: vi.fn(), refreshStore: vi.fn(), success: vi.fn(), error: vi.fn() }))
vi.mock('@/api/subscriptionSelfReset', () => ({ getSelfResetStatus: mocks.status, resetSubscriptionDaily: mocks.reset }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: { id: 7 } }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: mocks.success, showError: mocks.error }) }))
vi.mock('@/stores/subscriptions', () => ({ useSubscriptionStore: () => ({ fetchActiveSubscriptions: mocks.refreshStore }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, te: () => true }) }))

const snapshot = (count = 1, limit = Math.max(count, 1)) => ({
  organization: 'xunyou', daily_limit: limit, quota_date: '2026-09-23',
  server_now: '2026-09-23T23:59:59+08:00', next_reset_at: '2026-09-24T00:00:00+08:00',
  subscriptions: [{ subscription_id: 31, used_count: limit - count, remaining_count: count, can_reset: count > 0, disabled_reason: count ? null : 'DAILY_LIMIT_REACHED' }]
})
const storedKeys = () => JSON.parse(sessionStorage.getItem('subscription-self-reset:7') || '[]')
let controller: ReturnType<typeof useSubscriptionSelfReset>
const refreshList = vi.fn()
const invalidateList = vi.fn()
const Harness = defineComponent({ setup() { controller = useSubscriptionSelfReset(refreshList, invalidateList); return {} }, template: '<div />' })
let wrapper: ReturnType<typeof mount> | undefined
beforeEach(() => {
  vi.clearAllMocks()
  vi.useFakeTimers()
  sessionStorage.clear()
  mocks.status.mockResolvedValue(snapshot())
  mocks.reset.mockResolvedValue({})
  mocks.refreshStore.mockResolvedValue([])
  refreshList.mockResolvedValue(undefined)
})
afterEach(() => { wrapper?.unmount(); vi.restoreAllMocks(); vi.useRealTimers() })

describe('subscription self reset operation', () => {
  it('disables until authoritative status, blocks duplicate confirmation, and refreshes all consumers', async () => {
    wrapper = mount(Harness)
    expect(controller.canReset(31)).toBe(false)
    await controller.refresh()
    expect(mocks.refreshStore).not.toHaveBeenCalled()
    expect(controller.canReset(31)).toBe(true)
    controller.open(31)
    let finish!: () => void
    mocks.reset.mockReturnValueOnce(new Promise<void>(resolve => { finish = resolve }))
    const first = controller.confirm()
    await controller.confirm()
    expect(mocks.reset).toHaveBeenCalledTimes(1)
    expect(invalidateList).toHaveBeenCalledTimes(1)
    expect(controller.items.value.get(31)?.remaining_count).toBe(1)
    mocks.status.mockResolvedValue(snapshot(0))
    finish()
    await first
    expect(controller.items.value.get(31)?.remaining_count).toBe(0)
    expect(controller.canReset(31)).toBe(false)
    expect(refreshList).toHaveBeenCalledTimes(2)
    expect(mocks.refreshStore).toHaveBeenCalledWith(true)
  })

  it('keeps the last status visible while refreshing', async () => {
    wrapper = mount(Harness)
    await controller.refresh()
    let finish!: (data: ReturnType<typeof snapshot>) => void
    mocks.status.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    const pending = controller.refresh()
    expect(controller.canReset(31)).toBe(true)
    finish(snapshot(0))
    await pending
    expect(controller.canReset(31)).toBe(false)
  })

  it('keeps an authoritative status when only the list reload fails', async () => {
    refreshList.mockRejectedValueOnce(new Error('list failed'))
    wrapper = mount(Harness)
    expect(await controller.refresh()).toBe(true)
    expect(controller.statusError.value).toBe(false)
    expect(controller.canReset(31)).toBe(true)
  })

  it('retries an unknown outcome with the original key and date while the dialog stays open', async () => {
    wrapper = mount(Harness)
    await controller.refresh()
    controller.open(31)
    mocks.reset.mockRejectedValueOnce({ status: 0 })
    await controller.confirm()
    expect(controller.dialogOpen.value).toBe(true)
    expect(controller.requestError.value).toBe('userSubscriptions.selfReset.retrySame')
    expect(controller.items.value.get(31)?.remaining_count).toBe(1)
    await controller.confirm()
    expect(mocks.reset.mock.calls[1]).toEqual(mocks.reset.mock.calls[0])
  })

  it('keeps an unknown outcome key after closing, so new usage cannot trigger a second reset', async () => {
    mocks.status.mockResolvedValue(snapshot(3))
    wrapper = mount(Harness)
    await controller.refresh()
    controller.open(31)
    mocks.reset.mockRejectedValueOnce({ status: 0 })
    await controller.confirm()
    const original = mocks.reset.mock.calls[0]
    // The lost request committed and new usage arrived, so the subscription is resettable again.
    mocks.status.mockResolvedValue(snapshot(2, 3))
    controller.close()
    await flushPromises()
    expect(mocks.status).toHaveBeenCalledTimes(2)
    expect(controller.canReset(31)).toBe(true)
    controller.open(31)
    expect(controller.requestError.value).toBe('userSubscriptions.selfReset.resumePending')
    await controller.confirm()
    expect(mocks.reset.mock.calls[1]).toEqual(original)
    expect(sessionStorage.getItem('subscription-self-reset:7')).toBeNull()
    controller.open(31)
    await controller.confirm()
    expect(mocks.reset.mock.calls[2]?.[2]).not.toBe(original?.[2])
  })

  it('reuses an unknown outcome key after the page is reloaded', async () => {
    wrapper = mount(Harness)
    await controller.refresh()
    controller.open(31)
    mocks.reset.mockRejectedValueOnce({ status: 0 })
    await controller.confirm()
    const original = mocks.reset.mock.calls[0]
    expect(storedKeys()).toEqual([[31, { date: '2026-09-23', key: original?.[2] }]])
    wrapper.unmount()
    wrapper = mount(Harness)
    await controller.refresh()
    controller.open(31)
    await controller.confirm()
    expect(mocks.reset.mock.calls[1]).toEqual(original)
  })

  it('does not block other subscriptions while one outcome is unconfirmed', async () => {
    mocks.status.mockResolvedValue({ ...snapshot(), subscriptions: [...snapshot().subscriptions, { ...snapshot().subscriptions[0]!, subscription_id: 32 }] })
    wrapper = mount(Harness)
    await controller.refresh()
    controller.open(31)
    mocks.reset.mockRejectedValueOnce({ status: 0 })
    await controller.confirm()
    controller.close()
    await flushPromises()
    controller.open(32)
    expect(controller.requestError.value).toBe('')
    await controller.confirm()
    expect(mocks.reset.mock.calls[1]?.[0]).toBe(32)
    expect(mocks.reset.mock.calls[1]?.[2]).not.toBe(mocks.reset.mock.calls[0]?.[2])
    expect(storedKeys().map(([id]: [number]) => id)).toEqual([31])
  })

  it('drops a previous day key instead of replaying it', async () => {
    sessionStorage.setItem('subscription-self-reset:7', JSON.stringify([[31, { date: '2026-09-22', key: 'yesterday' }]]))
    wrapper = mount(Harness)
    await controller.refresh()
    controller.open(31)
    expect(controller.operation.value?.key).toBeUndefined()
    await controller.confirm()
    expect(mocks.reset.mock.calls[0]?.[2]).not.toBe('yesterday')
  })

  it('still submits and keeps the key in memory when session storage is unavailable', async () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('denied') })
    wrapper = mount(Harness)
    await controller.refresh()
    controller.open(31)
    mocks.reset.mockRejectedValueOnce({ status: 0 })
    await controller.confirm()
    controller.close()
    await flushPromises()
    controller.open(31)
    await controller.confirm()
    expect(mocks.reset.mock.calls[1]).toEqual(mocks.reset.mock.calls[0])
  })

  it('does not retain an unsubmitted operation when confirmation is cancelled', async () => {
    wrapper = mount(Harness)
    await controller.refresh()
    controller.open(31)
    expect(controller.operation.value?.key).toBeUndefined()
    controller.close()
    expect(controller.operation.value).toBeNull()
    expect(mocks.reset).not.toHaveBeenCalled()
    expect(mocks.status).toHaveBeenCalledOnce()
    expect(controller.canReset(31)).toBe(true)
  })

  it('creates an operation key when randomUUID is unavailable on HTTP', async () => {
    vi.spyOn(crypto, 'randomUUID').mockImplementation(() => { throw new Error('unavailable on HTTP') })
    wrapper = mount(Harness)
    await controller.refresh()
    controller.open(31)
    await controller.confirm()
    expect(crypto.randomUUID).not.toHaveBeenCalled()
    expect(mocks.reset).toHaveBeenCalledWith(31, '2026-09-23', expect.stringMatching(/^[0-9a-f]+(?:-[0-9a-f]+){3}$/))
  })

  it.each([401, 408, 429])('retains an uncertain key when its retry is rejected with %s', async (status) => {
    wrapper = mount(Harness)
    await controller.refresh()
    controller.open(31)
    mocks.reset.mockRejectedValueOnce({ status: 0 })
    await controller.confirm()
    const original = mocks.reset.mock.calls[0]
    mocks.reset.mockRejectedValueOnce({ status, retryAfter: '5' })
    await controller.confirm()
    expect(controller.operation.value?.key).toBe(original?.[2])
    await controller.confirm()
    expect(mocks.reset).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(5000)
    await controller.confirm()
    expect(mocks.reset.mock.calls[2]).toEqual(original)
  })

  it('honors retry-after for in-progress 409 without replacing the key', async () => {
    wrapper = mount(Harness)
    await controller.refresh()
    controller.open(31)
    mocks.reset.mockRejectedValueOnce({ status: 409, reason: 'IDEMPOTENCY_IN_PROGRESS', retryAfter: '5' })
    await controller.confirm()
    await controller.confirm()
    expect(mocks.reset).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(5000)
    await controller.confirm()
    expect(mocks.reset.mock.calls[1]).toEqual(mocks.reset.mock.calls[0])
  })

  it('ends a day-changed operation and requires a new confirmation with server date', async () => {
    wrapper = mount(Harness)
    await controller.refresh()
    controller.open(31)
    mocks.reset.mockRejectedValueOnce({ status: 409, reason: 'SELF_RESET_DAY_CHANGED' })
    mocks.status.mockResolvedValue({ ...snapshot(), quota_date: '2026-09-24' })
    await controller.confirm()
    expect(controller.operation.value).toBeNull()
    expect(controller.dialogOpen.value).toBe(false)
    controller.open(31)
    await controller.confirm()
    expect(mocks.reset.mock.calls[1]?.[1]).toBe('2026-09-24')
    expect(mocks.reset.mock.calls[1]?.[2]).not.toBe(mocks.reset.mock.calls[0]?.[2])
    expect(sessionStorage.getItem('subscription-self-reset:7')).toBeNull()
  })

  it('ends the operation with the rollout message when rollout is withdrawn while the page is open', async () => {
    wrapper = mount(Harness)
    await controller.refresh()
    controller.open(31)
    mocks.reset.mockRejectedValueOnce({ status: 403, reason: 'SELF_RESET_ROLLOUT_DISABLED' })
    await controller.confirm()
    expect(mocks.error).toHaveBeenCalledWith('userSubscriptions.selfReset.ROLLOUT_DISABLED')
    expect(controller.operation.value).toBeNull()
    expect(sessionStorage.getItem('subscription-self-reset:7')).toBeNull()
  })

  it('reports a failed global refresh after a reset and retries it on the next refresh only', async () => {
    wrapper = mount(Harness)
    await controller.refresh()
    controller.open(31)
    mocks.refreshStore.mockRejectedValueOnce(new Error('global failed'))
    await controller.confirm()
    expect(mocks.error).toHaveBeenCalledWith('userSubscriptions.selfReset.refreshFailed')
    expect(controller.statusError.value).toBe(false)
    expect(await controller.refresh()).toBe(true)
    expect(mocks.refreshStore).toHaveBeenCalledTimes(2)
    await controller.refresh()
    expect(mocks.refreshStore).toHaveBeenCalledTimes(2)
  })

  it('does not turn a committed reset into a retry when refreshing fails', async () => {
    wrapper = mount(Harness)
    await controller.refresh()
    controller.open(31)
    mocks.status.mockRejectedValueOnce(new Error('read failed'))
    await controller.confirm()
    expect(mocks.success).toHaveBeenCalledOnce()
    expect(mocks.error).toHaveBeenCalledWith('userSubscriptions.selfReset.refreshFailed')
    expect(controller.operation.value).toBeNull()
    expect(controller.statusError.value).toBe(true)
    expect(controller.canReset(31)).toBe(false)
    await controller.confirm()
    expect(mocks.reset).toHaveBeenCalledOnce()
    await controller.refresh()
    expect(controller.statusError.value).toBe(false)
    expect(refreshList).toHaveBeenCalledTimes(3)
    expect(mocks.refreshStore).toHaveBeenCalledOnce()
    expect(mocks.reset).toHaveBeenCalledOnce()
  })

  it('ignores a stale status response started before the write', async () => {
    wrapper = mount(Harness)
    await controller.refresh()
    controller.open(31)
    let finish!: (data: ReturnType<typeof snapshot>) => void
    mocks.status.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    const old = controller.refresh()
    mocks.status.mockResolvedValue(snapshot(0))
    await controller.confirm()
    finish(snapshot())
    await old
    expect(controller.items.value.get(31)?.remaining_count).toBe(0)
  })

  it('refreshes only global state when a submitted reset completes after leaving the page', async () => {
    wrapper = mount(Harness)
    await controller.refresh()
    controller.open(31)
    let finish!: () => void
    mocks.reset.mockReturnValueOnce(new Promise<void>(resolve => { finish = resolve }))
    const submitted = controller.confirm()
    wrapper.unmount()
    finish()
    await submitted
    expect(refreshList).toHaveBeenCalledTimes(1)
    expect(mocks.status).toHaveBeenCalledTimes(1)
    expect(mocks.refreshStore).toHaveBeenCalledOnce()
    expect(mocks.error).not.toHaveBeenCalled()
  })

  it('refetches at midnight instead of locally manufacturing new opportunities', async () => {
    wrapper = mount(Harness)
    mocks.status.mockResolvedValueOnce(snapshot(0))
    await controller.refresh()
    mocks.status.mockRejectedValueOnce(new Error('offline'))
    await vi.advanceTimersByTimeAsync(1100)
    await flushPromises()
    expect(mocks.status).toHaveBeenCalledTimes(2)
    expect(controller.canReset(31)).toBe(false)
  })
})
