import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import SubscriptionSelfResetEventsView from '../SubscriptionSelfResetEventsView.vue'

const mocks = vi.hoisted(() => ({ list: vi.fn(), eventGroups: vi.fn(), showError: vi.fn() }))
vi.mock('@/api/subscriptionSelfReset', () => ({ listSelfResetEvents: mocks.list, getSelfResetEventGroups: mocks.eventGroups }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.showError }) }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))

const SelectStub = { name: 'Select', props: ['modelValue', 'options'], emits: ['update:modelValue', 'change'], template: '<div />' }
const DataTableStub = { name: 'DataTable', props: ['columns', 'data', 'loading'], template: '<div data-testid="rows">{{ data.length }}</div>' }
const create = () => mount(SubscriptionSelfResetEventsView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
      DataTable: DataTableStub,
      Pagination: true,
      Select: SelectStub
    }
  }
})
// The filter bar has two selects: organization, then group.
const groupSelect = (wrapper: VueWrapper) => wrapper.findAllComponents(SelectStub)[1]!
const chooseGroup = async (wrapper: VueWrapper, value: number | '') => {
  groupSelect(wrapper).vm.$emit('update:modelValue', value)
  groupSelect(wrapper).vm.$emit('change')
  await flushPromises()
}
const event = { id: 1, subscription_id: 31, user_id: 5, user_email: 'a@xunyou.com', group_id: 12, group_name: 'xunyou-ai', organization: 'xunyou', quota_date: '2026-11-27', used_count: 1, daily_limit: 1, daily_usage_usd_before: 80, created_at: '2026-11-27T02:00:00Z' }

beforeEach(() => {
  vi.clearAllMocks()
  mocks.list.mockResolvedValue({ items: [event], total: 1 })
  mocks.eventGroups.mockResolvedValue({ event_groups: [{ id: 12, name: 'xunyou-ai' }, { id: 99, name: '#99' }] })
})

describe('self reset history group filter', () => {
  it('loads the list without waiting for the group options', async () => {
    let resolveGroups: (value: unknown) => void = () => {}
    mocks.eventGroups.mockReturnValueOnce(new Promise((resolve) => { resolveGroups = resolve }))
    const wrapper = create()
    await flushPromises()
    expect(mocks.list).toHaveBeenCalledWith(1, 20, {})
    expect(wrapper.get('[data-testid="rows"]').text()).toBe('1')
    expect(groupSelect(wrapper).props('options')).toEqual([{ value: '', label: 'userSubscriptions.selfReset.allGroups' }])
    resolveGroups({ event_groups: [{ id: 12, name: 'xunyou-ai' }, { id: 99, name: '#99' }] })
    await flushPromises()
    expect(groupSelect(wrapper).props('options')).toEqual([
      { value: '', label: 'userSubscriptions.selfReset.allGroups' },
      { value: 12, label: 'xunyou-ai' },
      { value: 99, label: '#99' }
    ])
    expect(groupSelect(wrapper).props('modelValue')).toBe('')
    wrapper.unmount()
  })

  it('filters by group from the first page', async () => {
    mocks.list.mockResolvedValue({ items: [event], total: 45 })
    const wrapper = create()
    await flushPromises()
    wrapper.findComponent({ name: 'Pagination' }).vm.$emit('update:page', 2)
    await flushPromises()
    expect(mocks.list).toHaveBeenLastCalledWith(2, 20, {})
    await chooseGroup(wrapper, 12)
    expect(mocks.list).toHaveBeenLastCalledWith(1, 20, { group_id: 12 })
    await chooseGroup(wrapper, '')
    expect(mocks.list).toHaveBeenLastCalledWith(1, 20, {})
    wrapper.unmount()
  })

  it('clears the group with the other filters on reset', async () => {
    const wrapper = create()
    await flushPromises()
    await chooseGroup(wrapper, 99)
    await wrapper.get('input[type="text"]').setValue('a@xunyou.com')
    await wrapper.findAll('button').find(button => button.text() === 'common.search')!.trigger('click')
    await flushPromises()
    expect(mocks.list).toHaveBeenLastCalledWith(1, 20, { email: 'a@xunyou.com', group_id: 99 })
    await wrapper.findAll('button').find(button => button.text() === 'common.reset')!.trigger('click')
    await flushPromises()
    expect(mocks.list).toHaveBeenLastCalledWith(1, 20, {})
    expect(groupSelect(wrapper).props('modelValue')).toBe('')
    wrapper.unmount()
  })

  it('still lists the history when the group options fail to load', async () => {
    mocks.eventGroups.mockRejectedValue({ message: 'options unavailable' })
    const wrapper = create()
    await flushPromises()
    expect(mocks.showError).toHaveBeenCalledWith('options unavailable')
    expect(mocks.list).toHaveBeenCalledWith(1, 20, {})
    expect(wrapper.get('[data-testid="rows"]').text()).toBe('1')
    expect(groupSelect(wrapper).props('options')).toEqual([{ value: '', label: 'userSubscriptions.selfReset.allGroups' }])
    wrapper.unmount()
  })
})
