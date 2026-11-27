import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import type { SelfResetPolicy } from '@/api/subscriptionSelfReset'
import SubscriptionSelfResetPolicyDialog from '../SubscriptionSelfResetPolicyDialog.vue'

const mocks = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn(), closeRollout: vi.fn(), success: vi.fn(), error: vi.fn() }))
vi.mock('@/api/subscriptionSelfReset', () => ({ getSelfResetConfig: mocks.get, saveSelfResetConfig: mocks.put, closeSelfResetRollout: mocks.closeRollout }))
let adminID = 100
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: { id: adminID } }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: mocks.success, showError: mocks.error }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params ? `${key} ${JSON.stringify(params)}` : key }) }))

const group = (id: number, enabled: boolean, extra: Record<string, unknown> = {}) => ({ id, name: `group-${id}`, platform: 'openai', status: 'active', subscription_type: 'subscription', daily_limit_usd: 50, enabled, ...extra })
const candidates = (enabled: Record<number, boolean> = { 12: true, 18: false, 25: false }) => ({
  xunyou: [group(12, enabled[12]!), group(18, enabled[18]!)],
  wsdashi: [group(25, enabled[25]!, { platform: 'anthropic', status: 'disabled', daily_limit_usd: null })],
  other: []
})
const create = () => mount(SubscriptionSelfResetPolicyDialog, { global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' } } } })
const limitInputs = (wrapper: VueWrapper) => wrapper.findAll('input[type="number"]')
const checkbox = (wrapper: VueWrapper, org: string, id: number) => wrapper.get(`[data-testid="self-reset-group-${org}-${id}"] input`)
const checked = (wrapper: VueWrapper, org: string, id: number) => (checkbox(wrapper, org, id).element as HTMLInputElement).checked
const submit = async (wrapper: VueWrapper) => {
  await wrapper.get('form').trigger('submit')
  await flushPromises()
}
const alertText = (wrapper: VueWrapper) => wrapper.get('[role="alert"]').text()
const config = (revision = 4, rollout: SelfResetPolicy['rollout'] = 'admin', enabled = false) => ({
  revision, policy: { daily_limit_by_organization: { xunyou: 1, wsdashi: 2, other: 0 }, rollout },
  groups_by_organization: candidates({ 12: true, 18: enabled, 25: false })
})
const conflicts = {
  revision: { status: 409, reason: 'SELF_RESET_REVISION_CONFLICT' },
  groups: { status: 409, reason: 'SELF_RESET_GROUP_CONFLICT' }
}
const open = async () => {
  const wrapper = create()
  await flushPromises()
  return wrapper
}
const retry = async (wrapper: VueWrapper) => {
  await wrapper.get('[data-testid="self-reset-retry"]').trigger('click')
  await flushPromises()
}
const marker = () => `subscription-self-reset-config:${adminID}`
const limitValues = (wrapper: VueWrapper) => limitInputs(wrapper).map(input => (input.element as HTMLInputElement).value)
const rolloutValue = (wrapper: VueWrapper) => (wrapper.get('select').element as HTMLSelectElement).value

beforeEach(() => {
  adminID++
  sessionStorage.clear()
  vi.resetAllMocks()
  mocks.get.mockImplementation(() => Promise.resolve(config()))
  mocks.put.mockResolvedValue({ revision: 5 })
  mocks.closeRollout.mockResolvedValue({ revision: 5 })
})

describe('self-reset settings', () => {
  it('renders the candidate groups of each organization', async () => {
    const wrapper = create()
    await flushPromises()
    const cards = ['xunyou', 'wsdashi', 'other'].map(org => wrapper.get(`[data-testid="self-reset-card-${org}"]`))
    expect(cards[0]!.text()).toContain('userSubscriptions.selfReset.xunyou')
    expect(cards[2]!.text()).toContain('userSubscriptions.selfReset.otherGroupsTitle')
    expect(wrapper.get('[data-testid="self-reset-count-xunyou"]').text()).toContain('{"selected":1,"total":2}')
    expect(checked(wrapper, 'xunyou', 12)).toBe(true)
    expect(checked(wrapper, 'xunyou', 18)).toBe(false)
    const active = wrapper.get('[data-testid="self-reset-group-xunyou-12"]').text()
    expect(active).toContain('group-12')
    expect(active).toContain('#12')
    expect(active).toContain('openai')
    expect(active).toContain('groupDailyLimit {"amount":"$50"}')
    expect(active).not.toContain('groupNoDailyLimit')
    expect(active).not.toContain('groupInactive')
    // A disabled group without a daily limit shows both labels.
    const disabled = wrapper.get('[data-testid="self-reset-group-wsdashi-25"]').text()
    expect(disabled).toContain('anthropic')
    expect(disabled).toContain('groupNoDailyLimit')
    expect(disabled).toContain('groupInactive')
    expect(cards[2]!.text()).toContain('groupsEmptyOther')
    expect(wrapper.find('[data-testid="self-reset-select-all-other"]').exists()).toBe(false)
    // A count of 0 keeps the selection and says it is not open yet.
    expect(wrapper.find('[data-testid="self-reset-zero-other"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="self-reset-zero-xunyou"]').exists()).toBe(false)
    await limitInputs(wrapper)[0]!.setValue(0)
    expect(wrapper.find('[data-testid="self-reset-zero-xunyou"]').exists()).toBe(true)
    expect(checked(wrapper, 'xunyou', 12)).toBe(true)
    expect((checkbox(wrapper, 'xunyou', 12).element as HTMLInputElement).disabled).toBe(false)
    wrapper.unmount()
  })


  it('submits one versioned mutation for a policy change and group deltas', async () => {
    const w = await open()
    await w.get('select').setValue('all')
    await checkbox(w, 'xunyou', 18).setValue(true)
    await checkbox(w, 'xunyou', 12).setValue(false)
    await submit(w)
    expect(mocks.put.mock.calls).toEqual([[{
      expected_revision: 4, policy: config(4, 'all').policy,
      changes_by_organization: { xunyou: { enable: [18], disable: [12] } }
    }]])
    expect(w.emitted('close')).toHaveLength(1)
    expect(sessionStorage.getItem(marker())).toBeNull()
    w.unmount()
  })

  it('closes an unchanged confirmed form without writing or claiming to save', async () => {
    const w = await open()
    await submit(w)
    expect(mocks.put).not.toHaveBeenCalled()
    expect(mocks.success).not.toHaveBeenCalled()
    expect(w.emitted('close')).toHaveLength(1)
    w.unmount()
  })

  it('validates integers and persists the policy alone when candidates are unavailable', async () => {
    mocks.get.mockResolvedValue({ ...config(), groups_by_organization: null })
    const w = await open()
    expect(w.text()).toContain('groupsLoadFailed')
    await limitInputs(w)[0]!.setValue(1.5)
    await submit(w)
    expect(alertText(w)).toContain('policyInvalid')
    expect(mocks.put).not.toHaveBeenCalled()
    await limitInputs(w)[0]!.setValue(3)
    await submit(w)
    expect(mocks.put.mock.calls).toEqual([[{ expected_revision: 4, policy: {
      daily_limit_by_organization: { xunyou: 3, wsdashi: 2, other: 0 }, rollout: 'admin'
    } }]])
    w.unmount()
  })

  it('can save groups alone or explicitly repair an unreadable policy at its known revision', async () => {
    mocks.get.mockImplementation(() => Promise.resolve({ ...config(), policy: null }))
    let w = await open()
    expect(limitValues(w)).toEqual(['', '', ''])
    await w.get('[data-testid="self-reset-select-all-xunyou"]').trigger('click')
    await submit(w)
    expect(mocks.put).toHaveBeenCalledWith({ expected_revision: 4, changes_by_organization: { xunyou: { enable: [18], disable: [] } } })
    w.unmount()
    w = await open()
    await limitInputs(w)[0]!.setValue(1)
    await submit(w)
    expect(alertText(w)).toContain('policyInvalid')
    await limitInputs(w)[1]!.setValue(2)
    await limitInputs(w)[2]!.setValue(0)
    await submit(w)
    expect(mocks.put).toHaveBeenLastCalledWith({ expected_revision: 4, policy: config().policy })
    w.unmount()
  })

  it('preserves valid drafts on a same-revision partial retry and drops them if the version changes', async () => {
    mocks.get.mockResolvedValueOnce({ ...config(), policy: null })
    let w = await open()
    await checkbox(w, 'xunyou', 18).setValue(true)
    await retry(w)
    expect(checked(w, 'xunyou', 18)).toBe(true)
    await submit(w)
    expect(mocks.put).toHaveBeenCalledWith({ expected_revision: 4, changes_by_organization: { xunyou: { enable: [18], disable: [] } } })
    w.unmount()

    mocks.get.mockResolvedValueOnce({ ...config(4, 'all'), groups_by_organization: null })
    mocks.get.mockResolvedValueOnce(config(5, 'off'))
    w = await open()
    await limitInputs(w)[0]!.setValue(3)
    await retry(w)
    expect(rolloutValue(w)).toBe('off')
    expect(limitValues(w)).toEqual(['1', '2', '0'])
    expect(w.text()).toContain('revisionConflict')
    w.unmount()
  })

  it('always closes separately, preserves server counts and drops unrelated edits', async () => {
    const w = await open()
    await checkbox(w, 'xunyou', 18).setValue(true)
    await limitInputs(w)[0]!.setValue(3)
    await w.get('select').setValue('off')
    expect(w.text()).toContain('closeOnlyHint')
    mocks.get.mockResolvedValueOnce(config(5, 'off'))
    await submit(w)
    expect(mocks.closeRollout.mock.calls).toEqual([[]])
    expect(mocks.put).not.toHaveBeenCalled()
    expect(checked(w, 'xunyou', 18)).toBe(false)
    expect(limitValues(w)).toEqual(['1', '2', '0'])
    expect(rolloutValue(w)).toBe('off')
    expect(w.emitted('close')).toBeUndefined()
    w.unmount()
  })

  it('can force a close even if reading the complete configuration failed', async () => {
    mocks.get.mockRejectedValueOnce(new Error('unavailable'))
    const w = await open()
    expect(w.find('form').exists()).toBe(false)
    mocks.get.mockResolvedValueOnce(config(5, 'off'))
    await w.get('[data-testid="self-reset-close-rollout"]').trigger('click')
    await flushPromises()
    expect(mocks.closeRollout.mock.calls).toEqual([[]])
    expect(rolloutValue(w)).toBe('off')
    w.unmount()
  })

  it('replaces the entire stale form after another admin closes the rollout', async () => {
    mocks.get.mockResolvedValueOnce(config(4, 'all'))
    const w = await open()
    await limitInputs(w)[0]!.setValue(3)
    await checkbox(w, 'xunyou', 18).setValue(true)
    mocks.put.mockRejectedValueOnce(conflicts.revision)
    mocks.get.mockResolvedValueOnce(config(5, 'off'))
    await submit(w)
    expect(w.text()).toContain('revisionConflict')
    expect(rolloutValue(w)).toBe('off')
    expect(limitValues(w)).toEqual(['1', '2', '0'])
    expect(checked(w, 'xunyou', 18)).toBe(false)
    expect(w.emitted('close')).toBeUndefined()
    await submit(w)
    expect(mocks.put).toHaveBeenCalledTimes(1)
    w.unmount()
  })

  it('reloads the eligible candidates and discards a wider draft on a group conflict', async () => {
    const w = await open()
    await checkbox(w, 'xunyou', 18).setValue(true)
    await w.get('select').setValue('all')
    const moved = config()
    moved.groups_by_organization.wsdashi.push(moved.groups_by_organization.xunyou.pop()!)
    mocks.put.mockRejectedValueOnce(conflicts.groups)
    mocks.get.mockResolvedValueOnce(moved)
    await submit(w)
    expect(w.find('[data-testid="self-reset-group-xunyou-18"]').exists()).toBe(false)
    expect(checked(w, 'wsdashi', 18)).toBe(false)
    expect(rolloutValue(w)).toBe('admin')
    expect(w.text()).toContain('groupConflict')
    expect(w.emitted('close')).toBeUndefined()
    w.unmount()
  })

  it.each(['groups', 'policy'])('confirms a committed %s save with a lost reply and sends the actual reversal', async kind => {
    const w = await open()
    if (kind === 'groups') await checkbox(w, 'xunyou', 18).setValue(true)
    else await w.get('select').setValue('all')
    mocks.put.mockRejectedValueOnce({ status: 503 }).mockRejectedValueOnce(conflicts.revision)
    mocks.get.mockResolvedValueOnce(config(5, kind === 'groups' ? 'admin' : 'all', kind === 'groups'))
    await submit(w)
    expect(mocks.put.mock.calls[1]).toEqual([{ expected_revision: 4 }])
    expect(w.text()).toContain('reviewSnapshot')
    expect(w.emitted('close')).toBeUndefined()
    if (kind === 'groups') await checkbox(w, 'xunyou', 18).setValue(false)
    else await w.get('select').setValue('admin')
    mocks.put.mockResolvedValueOnce({ revision: 6 })
    await submit(w)
    expect(mocks.put.mock.lastCall).toEqual([kind === 'groups'
      ? { expected_revision: 5, changes_by_organization: { xunyou: { enable: [], disable: [18] } } }
      : { expected_revision: 5, policy: config().policy }])
    expect(w.emitted('close')).toHaveLength(1)
    w.unmount()
  })

  it('fences a delayed write before reading rather than replaying it', async () => {
    const w = await open()
    await checkbox(w, 'xunyou', 18).setValue(true)
    mocks.put.mockRejectedValueOnce(new Error('timed out')).mockResolvedValueOnce({ revision: 5 })
    mocks.get.mockResolvedValueOnce(config(5))
    await submit(w)
    expect(mocks.put.mock.calls).toEqual([
      [{ expected_revision: 4, changes_by_organization: { xunyou: { enable: [18], disable: [] } } }],
      [{ expected_revision: 4 }]
    ])
    expect(checked(w, 'xunyou', 18)).toBe(false)
    expect(w.emitted('close')).toBeUndefined()
    w.unmount()
  })

  it('holds an unknown result across repeated fence failures, unmounting and reopening', async () => {
    const w = await open()
    await checkbox(w, 'xunyou', 18).setValue(true)
    mocks.put.mockRejectedValue({ status: 0 })
    await submit(w)
    expect(w.text()).toContain('saveOutcomeUnknown')
    expect(checkbox(w, 'xunyou', 18).attributes('disabled')).toBeDefined()
    await submit(w)
    expect(w.emitted('close')).toBeUndefined()
    expect(mocks.get).toHaveBeenCalledTimes(1)
    expect(JSON.parse(sessionStorage.getItem(marker())!)).toEqual({ revision: 4 })
    w.unmount()
    const reopened = await open()
    expect(reopened.text()).toContain('saveOutcomeUnknown')
    expect(mocks.put.mock.lastCall).toEqual([{ expected_revision: 4 }])
    mocks.put.mockRejectedValueOnce(conflicts.revision)
    mocks.get.mockResolvedValueOnce(config(5, 'admin', true))
    await retry(reopened)
    expect(checked(reopened, 'xunyou', 18)).toBe(true)
    expect(sessionStorage.getItem(marker())).toBeNull()
    reopened.unmount()
  })

  it('recovers a prior session marker and does not expose stale data when rereading fails', async () => {
    sessionStorage.setItem(marker(), JSON.stringify({ revision: 3 }))
    mocks.put.mockRejectedValue(conflicts.revision)
    mocks.get.mockRejectedValueOnce(new Error('read failed'))
    const w = await open()
    expect(w.find('form').exists()).toBe(false)
    expect(sessionStorage.getItem(marker())).not.toBeNull()
    await retry(w)
    expect(mocks.put.mock.calls).toEqual([[{ expected_revision: 3 }], [{ expected_revision: 3 }]])
    expect(w.text()).toContain('reviewSnapshot')
    expect(sessionStorage.getItem(marker())).toBeNull()
    w.unmount()
  })

  it('does not automatically send a second unconditional close when the first is uncertain', async () => {
    const w = await open()
    await w.get('select').setValue('off')
    mocks.closeRollout.mockRejectedValueOnce({ status: 0 })
    await submit(w)
    expect(alertText(w)).toContain('closeOutcomeUnknown')
    expect(JSON.parse(sessionStorage.getItem(marker())!)).toEqual({ close: true, closeRevision: 4 })
    await retry(w)
    expect(mocks.closeRollout).toHaveBeenCalledTimes(1)
    expect(mocks.put).not.toHaveBeenCalled()
    mocks.get.mockResolvedValueOnce(config(5, 'off'))
    await retry(w)
    expect(alertText(w)).toContain('closeOutcomeUnknown')
    expect(mocks.closeRollout).toHaveBeenCalledTimes(1)
    mocks.closeRollout.mockResolvedValueOnce({ revision: 6 })
    mocks.get.mockResolvedValueOnce(config(6, 'off'))
    await w.get('[data-testid="self-reset-close-rollout"]').trigger('click')
    await flushPromises()
    expect(sessionStorage.getItem(marker())).toBeNull()
    w.unmount()
  })


  it.each(['unknown', 'conflict'])('keeps ordinary saves locked on a partial %s reread and after reopening', async (kind) => {
    const w = await open()
    await checkbox(w, 'xunyou', 18).setValue(true)
    if (kind === 'unknown') mocks.put.mockRejectedValueOnce({ status: 0 })
    mocks.put.mockRejectedValueOnce(conflicts.revision)
    mocks.get.mockResolvedValueOnce({ ...config(5), groups_by_organization: null })
    await submit(w)
    expect(alertText(w)).toContain('snapshotIncomplete')
    expect(w.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    await submit(w)
    expect(w.emitted('close')).toBeUndefined()
    expect(sessionStorage.getItem(marker())).not.toBeNull()
    w.unmount()

    mocks.put.mockRejectedValue(conflicts.revision)
    mocks.get.mockResolvedValueOnce({ ...config(5, 'admin', true), policy: null })
    const next = await open()
    expect(alertText(next)).toContain('snapshotIncomplete')
    mocks.get.mockResolvedValueOnce(config(5, 'admin', true))
    await retry(next)
    expect(checked(next, 'xunyou', 18)).toBe(true)
    expect(next.get('button[type="submit"]').attributes('disabled')).toBeUndefined()
    expect(sessionStorage.getItem(marker())).toBeNull()
    next.unmount()
  })

  it('confirms the affected groups when the original snapshot already had an unreadable policy', async () => {
    mocks.get.mockResolvedValueOnce({ ...config(), policy: null })
    const w = await open()
    await checkbox(w, 'xunyou', 18).setValue(true)
    mocks.put.mockRejectedValueOnce({ status: 503 }).mockRejectedValueOnce(conflicts.revision)
    mocks.get.mockResolvedValueOnce({ ...config(5, 'admin', true), policy: null })
    await submit(w)
    expect(checked(w, 'xunyou', 18)).toBe(true)
    expect(alertText(w)).toContain('policyLoadFailed')
    expect(sessionStorage.getItem(marker())).toBeNull()
    expect(w.get('button[type="submit"]').attributes('disabled')).toBeUndefined()
    w.unmount()
  })

  it('allows an explicit emergency close even when the loaded snapshot is off', async () => {
    mocks.get.mockResolvedValueOnce(config(4, 'off')).mockResolvedValueOnce(config(5, 'off'))
    const w = await open()
    await w.get('[data-testid="self-reset-close-rollout"]').trigger('click')
    await flushPromises()
    expect(mocks.closeRollout.mock.calls).toEqual([[]])
    expect(mocks.put).not.toHaveBeenCalled()
    expect(mocks.get).toHaveBeenCalledTimes(2)
    w.unmount()
  })

  it('keeps the revision marker when the component unmounts during an in-flight save', async () => {
    let finish!: (value: { revision: number }) => void
    mocks.put.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const w = await open()
    await checkbox(w, 'xunyou', 18).setValue(true)
    await w.get('form').trigger('submit')
    w.unmount()
    expect(JSON.parse(sessionStorage.getItem(marker())!)).toEqual({ revision: 4 })
    finish({ revision: 5 })
    await flushPromises()
    expect(mocks.success).not.toHaveBeenCalled()
    expect(sessionStorage.getItem(marker())).not.toBeNull()
  })

  it('rejects a stale reread that predates the confirmed revision', async () => {
    sessionStorage.setItem(marker(), JSON.stringify({ revision: 4 }))
    mocks.get.mockResolvedValueOnce(config(4))
    const w = await open()
    expect(w.find('form').exists()).toBe(false)
    expect(sessionStorage.getItem(marker())).not.toBeNull()
    mocks.put.mockRejectedValueOnce(conflicts.revision)
    mocks.get.mockResolvedValueOnce(config(5, 'admin', true))
    await retry(w)
    expect(checked(w, 'xunyou', 18)).toBe(true)
    expect(sessionStorage.getItem(marker())).toBeNull()
    w.unmount()
  })

  it('still needs the groups after an emergency close resolves an uncertain ordinary save', async () => {
    const w = await open()
    await checkbox(w, 'xunyou', 18).setValue(true)
    mocks.put.mockRejectedValueOnce({ status: 0 }).mockRejectedValueOnce({ status: 503 })
    await submit(w)
    mocks.get.mockResolvedValueOnce({ ...config(5, 'off'), groups_by_organization: null })
    await w.get('[data-testid="self-reset-close-rollout"]').trigger('click')
    await flushPromises()
    expect(mocks.closeRollout).toHaveBeenCalledTimes(1)
    expect(alertText(w)).toContain('snapshotIncomplete')
    expect(sessionStorage.getItem(marker())).not.toBeNull()
    expect(w.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    mocks.put.mockRejectedValueOnce(conflicts.revision)
    mocks.get.mockResolvedValueOnce(config(6, 'off', true))
    await retry(w)
    expect(rolloutValue(w)).toBe('off')
    expect(checked(w, 'xunyou', 18)).toBe(true)
    expect(sessionStorage.getItem(marker())).toBeNull()
    w.unmount()
  })


  it('requires an advanced version even when the rollout was already off before an uncertain close', async () => {
    let server = config(4, 'off')
    mocks.get.mockImplementation(() => Promise.resolve(structuredClone(server)))
    mocks.closeRollout.mockRejectedValueOnce({ status: 503 }).mockImplementationOnce(async () => {
      server = config(server.revision + 1, 'off')
      return { revision: server.revision }
    })
    let w = await open()
    await w.get('[data-testid="self-reset-close-rollout"]').trigger('click')
    await flushPromises()
    expect(alertText(w)).toContain('closeOutcomeUnknown')
    expect(JSON.parse(sessionStorage.getItem(marker())!)).toEqual({ close: true, closeRevision: 4 })
    w.unmount()

    w = await open()
    expect(w.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    // A request based on version 4 can still open the rollout at this point.
    server = config(5, 'all')
    await retry(w)
    expect(rolloutValue(w)).toBe('all')
    expect(alertText(w)).toContain('closeOutcomeUnknown')
    await w.get('[data-testid="self-reset-close-rollout"]').trigger('click')
    await flushPromises()
    expect(server).toMatchObject({ revision: 6, policy: { rollout: 'off' } })
    expect(sessionStorage.getItem(marker())).toBeNull()
    expect(w.get('button[type="submit"]').attributes('disabled')).toBeUndefined()
    w.unmount()
  })

  it.each(['legacy', 'unavailable'])('needs an acknowledged close when its baseline is %s', async kind => {
    if (kind === 'legacy') sessionStorage.setItem(marker(), JSON.stringify({ close: true }))
    else mocks.get.mockRejectedValueOnce(new Error('initial read failed'))
    mocks.get.mockResolvedValue(config(7, 'off'))
    let w = await open()
    if (kind === 'unavailable') {
      mocks.closeRollout.mockRejectedValueOnce({ status: 0 })
      await w.get('[data-testid="self-reset-close-rollout"]').trigger('click')
      await flushPromises()
    }
    expect(alertText(w)).toContain('closeOutcomeUnknown')
    w.unmount()
    w = await open()
    expect(w.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    mocks.closeRollout.mockResolvedValueOnce({ revision: 8 })
    mocks.get.mockResolvedValueOnce(config(8, 'off'))
    await w.get('[data-testid="self-reset-close-rollout"]').trigger('click')
    await flushPromises()
    expect(sessionStorage.getItem(marker())).toBeNull()
    w.unmount()
  })


  it('does not treat an unrelated pre-click revision bump as proof the later close fenced another admin', async () => {
    let server = config(4, 'off')
    const delayedOpen = (expected: number) => {
      if (server.revision !== expected) return false
      server = config(expected + 1, 'all')
      return true
    }
    mocks.get.mockImplementation(() => Promise.resolve(structuredClone(server)))
    mocks.closeRollout.mockRejectedValueOnce({ status: 0 }).mockImplementationOnce(async () => {
      server = config(server.revision + 1, 'off')
      return { revision: server.revision }
    })
    let w = await open()
    // A still displays r4. An unrelated edit leaves r5 off, and C starts an r5 save to reopen.
    server = config(5, 'off')
    const cRevision = server.revision
    await w.get('[data-testid="self-reset-close-rollout"]').trigger('click')
    await flushPromises()
    expect(alertText(w)).toContain('closeOutcomeUnknown')
    expect(w.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    w.unmount()
    w = await open()
    expect(sessionStorage.getItem(marker())).not.toBeNull()
    expect(delayedOpen(cRevision)).toBe(true)
    await retry(w)
    expect(rolloutValue(w)).toBe('all')
    expect(mocks.closeRollout).toHaveBeenCalledTimes(1)
    expect(mocks.put).not.toHaveBeenCalled()
    const reopenedRevision = server.revision
    await w.get('[data-testid="self-reset-close-rollout"]').trigger('click')
    await flushPromises()
    expect(server).toMatchObject({ revision: 7, policy: { rollout: 'off' } })
    expect(delayedOpen(reopenedRevision)).toBe(false)
    expect(sessionStorage.getItem(marker())).toBeNull()
    w.unmount()
  })

  it('requires the separate close baseline for mixed pending operations', async () => {
    sessionStorage.setItem(marker(), JSON.stringify({ revision: 4, close: true, closeRevision: 5 }))
    mocks.put.mockRejectedValue(conflicts.revision)
    mocks.get.mockResolvedValueOnce(config(5, 'off')).mockResolvedValueOnce(config(6, 'off'))
    const w = await open()
    expect(mocks.put).toHaveBeenCalledWith({ expected_revision: 4 })
    expect(alertText(w)).toContain('closeOutcomeUnknown')
    await retry(w)
    expect(alertText(w)).toContain('closeOutcomeUnknown')
    expect(mocks.closeRollout).not.toHaveBeenCalled()
    expect(sessionStorage.getItem(marker())).not.toBeNull()
    w.unmount()
  })

  it('reports only the observed closed state if the ordinary fence itself advances the version', async () => {
    sessionStorage.setItem(marker(), JSON.stringify({ revision: 4, close: true, closeRevision: 4 }))
    let server = config(4, 'off')
    mocks.put.mockImplementationOnce(async ({ expected_revision }) => {
      expect(expected_revision).toBe(server.revision)
      server = config(server.revision + 1, 'off')
      return { revision: server.revision }
    })
    mocks.get.mockImplementation(() => Promise.resolve(structuredClone(server)))
    const w = await open()
    expect(server.revision).toBe(5)
    expect(w.text()).toContain('closedSnapshot')
    expect(w.text()).not.toContain('rolloutClosed')
    expect(mocks.closeRollout).not.toHaveBeenCalled()
    expect(sessionStorage.getItem(marker())).toBeNull()
    w.unmount()
  })

  it('keeps a lost or transient close pending until a definite corruption rejection allows an explicit repair', async () => {
    const invalid = { ...config(4), policy: null }
    mocks.get.mockImplementation(() => Promise.resolve(structuredClone(invalid)))
    mocks.closeRollout.mockRejectedValueOnce({ status: 503 }).mockRejectedValueOnce({ status: 409, reason: 'SELF_RESET_CLOSE_POLICY_INVALID' })
    let w = await open()
    await w.get('[data-testid="self-reset-close-rollout"]').trigger('click')
    await flushPromises()
    expect(alertText(w)).toContain('closeOutcomeUnknown')
    expect(sessionStorage.getItem(marker())).not.toBeNull()
    w.unmount()
    w = await open()
    expect(w.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    await w.get('[data-testid="self-reset-close-rollout"]').trigger('click')
    await flushPromises()
    expect(alertText(w)).toContain('closePolicyInvalid')
    expect(sessionStorage.getItem(marker())).toBeNull()
    expect(limitValues(w)).toEqual(['', '', ''])
    await w.get('select').setValue('off')
    await submit(w)
    expect(alertText(w)).toContain('policyInvalid')
    for (const [index, count] of [1, 2, 0].entries()) await limitInputs(w)[index]!.setValue(count)
    await submit(w)
    expect(mocks.put.mock.calls).toEqual([[{ expected_revision: 4, policy: config(4, 'off').policy }]])
    expect(mocks.closeRollout).toHaveBeenCalledTimes(2)
    expect(w.emitted('close')).toHaveLength(1)
    w.unmount()
  })

  it('retains an ordinary write after a definitely rejected close and only permits repair after fencing and verifying groups', async () => {
    let w = await open()
    await checkbox(w, 'xunyou', 18).setValue(true)
    mocks.put.mockRejectedValue({ status: 503 })
    await submit(w)
    mocks.closeRollout.mockRejectedValueOnce({ status: 409, reason: 'SELF_RESET_CLOSE_POLICY_INVALID' })
    await w.get('[data-testid="self-reset-close-rollout"]').trigger('click')
    await flushPromises()
    expect(mocks.put.mock.lastCall).toEqual([{ expected_revision: 4 }])
    expect(JSON.parse(sessionStorage.getItem(marker())!)).toMatchObject({ revision: 4 })
    expect(w.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    w.unmount()
    mocks.put.mockResolvedValueOnce({ revision: 5 }).mockRejectedValueOnce(conflicts.revision)
    mocks.get.mockResolvedValueOnce({ revision: 5, policy: null, groups_by_organization: null })
    w = await open()
    expect(alertText(w)).toContain('snapshotIncomplete')
    expect(sessionStorage.getItem(marker())).not.toBeNull()
    mocks.get.mockResolvedValueOnce({ ...config(5, 'admin', true), policy: null })
    await retry(w)
    expect(checked(w, 'xunyou', 18)).toBe(true)
    expect(alertText(w)).toContain('closePolicyInvalid')
    expect(sessionStorage.getItem(marker())).toBeNull()
    expect(w.get('button[type="submit"]').attributes('disabled')).toBeUndefined()
    w.unmount()
  })

  it('treats an unchanged revision on a save response as unknown and recovers on reopening', async () => {
    let server = config(4)
    mocks.get.mockImplementation(() => Promise.resolve(structuredClone(server)))
    mocks.put.mockImplementationOnce(async () => {
      server = config(5, 'admin', true)
      return { revision: 4 }
    }).mockRejectedValueOnce({ status: 503 }).mockRejectedValueOnce(conflicts.revision)
    let w = await open()
    await checkbox(w, 'xunyou', 18).setValue(true)
    await submit(w)
    expect(w.emitted('close')).toBeUndefined()
    expect(mocks.success).not.toHaveBeenCalled()
    expect(JSON.parse(sessionStorage.getItem(marker())!)).toEqual({ revision: 4 })
    w.unmount()
    w = await open()
    expect(mocks.put.mock.lastCall).toEqual([{ expected_revision: 4 }])
    expect(checked(w, 'xunyou', 18)).toBe(true)
    expect(sessionStorage.getItem(marker())).toBeNull()
    w.unmount()
  })

  it.each(['same revision', 'unrelated 409', 'unclassified 409'])('keeps an unconfirmed fence pending after %s and recovers the real write on reopening', async kind => {
    let server = config(4)
    mocks.get.mockImplementation(() => Promise.resolve(structuredClone(server)))
    mocks.put.mockRejectedValueOnce({ status: 0 })
    if (kind === 'same revision') mocks.put.mockResolvedValueOnce({ revision: 4 })
    else mocks.put.mockRejectedValueOnce(kind === 'unrelated 409' ? conflicts.groups : { status: 409 })
    let w = await open()
    await checkbox(w, 'xunyou', 18).setValue(true)
    // Even a newer-looking GET must not unlock the form after an invalid confirmation.
    server = config(5, 'admin', true)
    await submit(w)
    expect(w.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    expect(alertText(w)).toContain('saveOutcomeUnknown')
    expect(w.emitted('close')).toBeUndefined()
    expect(sessionStorage.getItem(marker())).not.toBeNull()
    expect(mocks.get).toHaveBeenCalledTimes(1)
    w.unmount()
    mocks.put.mockRejectedValueOnce(conflicts.revision)
    w = await open()
    expect(checked(w, 'xunyou', 18)).toBe(true)
    expect(mocks.put.mock.lastCall).toEqual([{ expected_revision: 4 }])
    expect(sessionStorage.getItem(marker())).toBeNull()
    w.unmount()
  })

  it('shows the latest rollout if someone reopened it after the close committed', async () => {
    const w = await open()
    await w.get('select').setValue('off')
    mocks.get.mockResolvedValueOnce(config(6, 'all'))
    await submit(w)
    expect(w.text()).toContain('closeChangedAgain')
    expect(rolloutValue(w)).toBe('all')
    w.unmount()
  })
})
