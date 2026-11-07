import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { AdminGroup } from '@/types'
import GroupOrganizationScopeBatchModal from '../GroupOrganizationScopeBatchModal.vue'
import GroupOrganizationScopeField from '../GroupOrganizationScopeField.vue'
import GroupOrganizationScopeBadges from '../GroupOrganizationScopeBadges.vue'

const mocks = vi.hoisted(() => ({
  getAllIncludingInactive: vi.fn(),
  setGroupOrganizationScopes: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: { getAllIncludingInactive: mocks.getAllIncludingInactive } } }))
vi.mock('@/api/admin/groupOrganizationScopes', () => ({
  GROUP_ORGANIZATION_KEYS: ['xunyou', 'wsdashi'],
  GROUP_ORGANIZATION_SCOPE_BATCH_LIMIT: 2,
  setGroupOrganizationScopes: mocks.setGroupOrganizationScopes
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.showError, showSuccess: mocks.showSuccess }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
beforeEach(() => vi.resetAllMocks())

const group = (id: number, subscriptionType = 'subscription', status = 'active') =>
  ({ id, name: `Group ${id}`, subscription_type: subscriptionType, status } as AdminGroup)

const stubs = { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' } }

function mountModal(scopes = new Map<number, ('xunyou' | 'wsdashi')[]>(), scopesStatus: 'ready' | 'loading' | 'error' = 'ready') {
  return mount(GroupOrganizationScopeBatchModal, { props: { show: true, scopes, scopesStatus }, global: { stubs } })
}

function saveButton(wrapper: ReturnType<typeof mountModal>) {
  return wrapper.get('[data-testid="group-organization-batch-save"]')
}

describe('group organization scope field', () => {
  it('emits the keys in a stable order', async () => {
    const wrapper = mount(GroupOrganizationScopeField, { props: { modelValue: ['wsdashi'] } })
    await wrapper.get('[data-testid="group-organization-xunyou"]').setValue(true)
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['xunyou', 'wsdashi']])
    await wrapper.setProps({ modelValue: ['xunyou', 'wsdashi'] })
    await wrapper.get('[data-testid="group-organization-wsdashi"]').setValue(false)
    expect(wrapper.emitted('update:modelValue')?.[1]).toEqual([['xunyou']])
  })

  it('shows load state and retry instead of the hint while the scope is unknown', async () => {
    const wrapper = mount(GroupOrganizationScopeField, { props: { modelValue: [], disabled: true, status: 'error' } })
    expect(wrapper.find('[data-testid="group-organization-load-failed"]').exists()).toBe(true)
    await wrapper.get('[data-testid="group-organization-retry"]').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
    await wrapper.setProps({ status: 'loading' })
    expect(wrapper.find('[data-testid="group-organization-loading"]').exists()).toBe(true)
  })

  it('shows an unscoped label without organizations', () => {
    expect(mount(GroupOrganizationScopeBadges, { props: { keys: [] } }).text()).toBe('admin.groupOrganizationScope.unscoped')
    expect(mount(GroupOrganizationScopeBadges, { props: { keys: ['xunyou', 'wsdashi'] } }).text()).toContain('迅游')
    expect(mount(GroupOrganizationScopeBadges, { props: { keys: [], status: 'error' } }).text()).toBe('admin.groupOrganizationScope.unknown')
    expect(mount(GroupOrganizationScopeBadges, { props: { keys: ['xunyou'], status: 'loading' } }).text()).toBe('admin.groupOrganizationScope.loading')
  })
})

describe('group organization scope batch modal', () => {
  it('lists subscription groups only and replaces the scopes of the selection', async () => {
    mocks.getAllIncludingInactive.mockResolvedValue([group(1), group(2, 'standard'), group(3, 'subscription', 'inactive')])
    mocks.setGroupOrganizationScopes.mockResolvedValue(2)
    const wrapper = mountModal(new Map([[1, ['wsdashi']]]))
    await flushPromises()
    expect(wrapper.find('[data-testid="group-organization-batch-row-2"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('admin.groupOrganizationScope.inactive')
    expect(saveButton(wrapper).attributes('disabled')).toBeDefined()

    await wrapper.get('[data-testid="group-organization-batch-row-1"]').setValue(true)
    await wrapper.get('[data-testid="group-organization-batch-row-3"]').setValue(true)
    await wrapper.get('[data-testid="group-organization-xunyou"]').setValue(true)
    await saveButton(wrapper).trigger('click')
    await flushPromises()
    expect(mocks.setGroupOrganizationScopes).toHaveBeenCalledWith([1, 3], ['xunyou'])
    expect(wrapper.emitted('saved')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('warns that an empty selection leaves the groups to administrators and sends []', async () => {
    mocks.getAllIncludingInactive.mockResolvedValue([group(1), group(4)])
    mocks.setGroupOrganizationScopes.mockResolvedValue(2)
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.text()).toContain('admin.groupOrganizationScope.clearWarning')
    await wrapper.get('[data-testid="group-organization-batch-search"]').setValue('4')
    expect(wrapper.find('[data-testid="group-organization-batch-row-1"]').exists()).toBe(false)
    await wrapper.findAll('button').find(button => button.text() === 'admin.groupOrganizationScope.selectVisible')!.trigger('click')
    await saveButton(wrapper).trigger('click')
    await flushPromises()
    expect(mocks.setGroupOrganizationScopes).toHaveBeenCalledWith([4], [])
  })

  it('keeps the dialog open when saving fails', async () => {
    mocks.getAllIncludingInactive.mockResolvedValue([group(1)])
    mocks.setGroupOrganizationScopes.mockRejectedValue({ message: 'denied' })
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('[data-testid="group-organization-batch-row-1"]').setValue(true)
    await saveButton(wrapper).trigger('click')
    await flushPromises()
    expect(mocks.showError).toHaveBeenCalledWith('denied')
    expect(wrapper.emitted('saved')).toBeUndefined()
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  // K26: the API limit is mocked as 2 here; the real value is 500.
  it('blocks a selection above the API limit instead of splitting it', async () => {
    mocks.getAllIncludingInactive.mockResolvedValue([group(1), group(4), group(5)])
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'admin.groupOrganizationScope.selectVisible')!.trigger('click')
    expect(wrapper.find('[data-testid="group-organization-batch-over-limit"]').exists()).toBe(true)
    expect(saveButton(wrapper).attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="group-organization-batch-row-5"]').setValue(false)
    expect(wrapper.find('[data-testid="group-organization-batch-over-limit"]').exists()).toBe(false)
    await saveButton(wrapper).trigger('click')
    await flushPromises()
    expect(mocks.setGroupOrganizationScopes).toHaveBeenCalledTimes(1)
    expect(mocks.setGroupOrganizationScopes).toHaveBeenCalledWith([1, 4], [])
  })

  it('labels unknown scopes instead of showing them as unscoped (K24)', async () => {
    mocks.getAllIncludingInactive.mockResolvedValue([group(1)])
    const failed = mountModal(new Map(), 'error')
    await flushPromises()
    expect(failed.text()).toContain('admin.groupOrganizationScope.unknown')
    expect(failed.text()).not.toContain('admin.groupOrganizationScope.unscoped')
    const loading = mountModal(new Map(), 'loading')
    await flushPromises()
    expect(loading.text()).toContain('admin.groupOrganizationScope.loading')
  })

  it('ignores an older load after the dialog is reopened', async () => {
    let resolveOld!: (groups: AdminGroup[]) => void
    mocks.getAllIncludingInactive
      .mockReturnValueOnce(new Promise(resolve => { resolveOld = resolve }))
      .mockResolvedValueOnce([group(2)])
    const wrapper = mountModal()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await flushPromises()
    resolveOld([group(1)])
    await flushPromises()
    expect(wrapper.find('[data-testid="group-organization-batch-row-2"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="group-organization-batch-row-1"]').exists()).toBe(false)
  })
})
