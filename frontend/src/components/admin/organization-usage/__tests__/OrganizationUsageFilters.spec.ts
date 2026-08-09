import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import Select from '@/components/common/Select.vue'
import OrganizationUsageFilters, { type OrganizationUsageFilterDraft } from '../OrganizationUsageFilters.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const baseDraft: OrganizationUsageFilterDraft = {
  mode: 'month',
  month: '2026-07',
  weekAnchor: '2026-07-10',
  customStart: '2026-07-01',
  customEnd: '2026-07-31',
  organization: 'all',
  q: ''
}

function mountFilters(modelValue: OrganizationUsageFilterDraft) {
  const errorHandler = vi.fn()
  const wrapper = mount(OrganizationUsageFilters, {
    props: { modelValue, loading: false, exporting: false },
    global: { config: { errorHandler } }
  })
  return { wrapper, errorHandler }
}

describe('OrganizationUsageFilters date validation', () => {
  it('shows brand names while preserving organization filter values', () => {
    const { wrapper } = mountFilters(baseDraft)

    expect(wrapper.getComponent(Select).props('options')).toEqual([
      { value: 'all', label: 'admin.organizationUsage.organizations.all' },
      { value: 'xunyou', label: '迅游' },
      { value: 'wsdashi', label: '速宝' },
      { value: 'other', label: 'admin.organizationUsage.organizations.other' }
    ])
  })

  it.each([
    ['month', { month: '' }],
    ['week', { weekAnchor: '' }]
  ] as const)('treats a cleared %s value as required without applying', async (mode, patch) => {
    const { wrapper, errorHandler } = mountFilters({ ...baseDraft, mode, ...patch })

    expect(wrapper.find('[data-testid="week-range"]').exists()).toBe(false)
    await wrapper.get('[data-testid="apply-filters"]').trigger('click')

    expect(errorHandler).not.toHaveBeenCalled()
    expect(wrapper.emitted('apply')).toBeUndefined()
    expect(wrapper.get('[data-testid="date-error"]').text()).toBe('admin.organizationUsage.validation.required')
  })

  it.each([
    ['month', { month: '2026-99' }],
    ['week', { weekAnchor: 'not-a-date' }]
  ] as const)('treats an invalid %s value as invalid without throwing', async (mode, patch) => {
    const { wrapper, errorHandler } = mountFilters({ ...baseDraft, mode, ...patch })

    const applyButton = wrapper.find('[data-testid="apply-filters"]')
    if (applyButton.exists()) await applyButton.trigger('click')
    expect(errorHandler).not.toHaveBeenCalled()
    expect(wrapper.emitted('apply')).toBeUndefined()
    expect(wrapper.get('[data-testid="date-error"]').text()).toBe('admin.organizationUsage.validation.invalidDate')
  })

  it.each([
    ['month', { month: '2026-06' }, { start_date: '2026-06-01', end_date: '2026-06-30' }],
    ['week', { weekAnchor: '2026-07-08' }, { start_date: '2026-07-06', end_date: '2026-07-12' }]
  ] as const)('emits the exact %s query range for a valid value', async (mode, patch, expectedRange) => {
    const { wrapper, errorHandler } = mountFilters({ ...baseDraft, mode, ...patch })

    await wrapper.get('[data-testid="apply-filters"]').trigger('click')

    expect(errorHandler).not.toHaveBeenCalled()
    expect(wrapper.emitted('apply')).toEqual([[expectedRange]])
  })

  it.each([
    ['month', { month: '' }],
    ['week', { weekAnchor: '' }]
  ] as const)('clears a %s validation error before emitting reset', async (mode, patch) => {
    const { wrapper } = mountFilters({ ...baseDraft, mode, ...patch })
    await wrapper.get('[data-testid="apply-filters"]').trigger('click')
    expect(wrapper.find('[data-testid="date-error"]').exists()).toBe(true)

    const resetButton = wrapper.findAll('button').find((button) =>
      button.text().includes('admin.organizationUsage.actions.reset')
    )
    expect(resetButton).toBeDefined()
    await resetButton!.trigger('click')

    expect(wrapper.find('[data-testid="date-error"]').exists()).toBe(false)
    expect(wrapper.emitted('reset')).toEqual([[]])
  })

  it.each([
    ['month', { mode: 'month', month: '' }, ['[data-testid="month-input"]']],
    ['week', { mode: 'week', weekAnchor: '' }, ['input[type="date"]']],
    ['custom', { mode: 'custom', customStart: '', customEnd: '' }, [
      '[data-testid="custom-start"]',
      '[data-testid="custom-end"]'
    ]]
  ] as const)('associates the %s validation alert with the relevant inputs', async (_mode, patch, selectors) => {
    const { wrapper } = mountFilters({ ...baseDraft, ...patch } as OrganizationUsageFilterDraft)
    await wrapper.get('[data-testid="apply-filters"]').trigger('click')

    const error = wrapper.get('[data-testid="date-error"]')
    expect(error.attributes()).toMatchObject({
      id: 'organization-usage-date-error',
      role: 'alert',
      'aria-live': 'polite'
    })
    for (const selector of selectors) {
      expect(wrapper.get(selector).attributes()).toMatchObject({
        'aria-invalid': 'true',
        'aria-describedby': 'organization-usage-date-error'
      })
    }
  })
})
