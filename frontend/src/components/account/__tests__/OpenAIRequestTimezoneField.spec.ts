import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getTimezonesMock } = vi.hoisted(() => ({ getTimezonesMock: vi.fn() }))

vi.mock('@/api/admin', () => ({
  adminAPI: { accounts: { getOpenAIRequestTimezones: getTimezonesMock } }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import OpenAIRequestTimezoneField from '../OpenAIRequestTimezoneField.vue'

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: {
    modelValue: { type: [String, Number, Boolean, null], default: '' },
    options: { type: Array, default: () => [] }
  },
  emits: ['update:modelValue'],
  template: `
    <select v-bind="$attrs" :value="modelValue" @change="$emit('update:modelValue', $event.target.value)">
      <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
    </select>
  `
})

function mountField(props: { enabled: boolean; timezone: string; managedByParent?: boolean }) {
  return mount(OpenAIRequestTimezoneField, { props, global: { stubs: { Select: SelectStub } } })
}

describe('OpenAIRequestTimezoneField', () => {
  beforeEach(() => {
    getTimezonesMock.mockReset().mockResolvedValue({
      default: 'America/Los_Angeles',
      timezones: ['America/Los_Angeles', 'Asia/Tokyo']
    })
  })

  it('shows a retained timezone but keeps the selector disabled while the switch is off', async () => {
    const wrapper = mountField({ enabled: false, timezone: 'Asia/Tokyo' })
    await flushPromises()

    const toggle = wrapper.get('[data-testid="openai-request-timezone-rewrite-toggle"]')
    const select = wrapper.get<HTMLSelectElement>('[data-testid="openai-request-timezone-select"]')
    expect(toggle.attributes('aria-checked')).toBe('false')
    expect(select.attributes('disabled')).toBeDefined()
    expect(select.element.value).toBe('Asia/Tokyo')
    expect(wrapper.emitted()).not.toHaveProperty('update:timezone')

    await toggle.trigger('click')
    expect(wrapper.emitted('update:enabled')).toEqual([[true]])
  })

  it('enables the selector with the switch and emits only user choices', async () => {
    const wrapper = mountField({ enabled: true, timezone: 'America/Los_Angeles' })
    await flushPromises()

    const select = wrapper.get('[data-testid="openai-request-timezone-select"]')
    expect(select.attributes('disabled')).toBeUndefined()
    await select.setValue('Asia/Tokyo')
    expect(wrapper.emitted('update:timezone')).toEqual([['Asia/Tokyo']])
    expect(wrapper.emitted()).not.toHaveProperty('update:enabled')
  })

  it('keeps the current value selectable when the list cannot be loaded', async () => {
    getTimezonesMock.mockReset().mockRejectedValue(new Error('offline'))
    const wrapper = mountField({ enabled: true, timezone: 'Asia/Tokyo' })
    await flushPromises()

    const values = wrapper.findAll('option').map(option => option.attributes('value'))
    expect(values).toEqual(['Asia/Tokyo', 'America/Los_Angeles'])
    expect(wrapper.text()).toContain('admin.accounts.openai.requestTimezoneLoadFailed')
    expect(wrapper.emitted()).not.toHaveProperty('update:timezone')
  })

  it('shows a parent-managed notice for Spark shadows without editable controls', async () => {
    const wrapper = mountField({ enabled: false, timezone: 'America/Los_Angeles', managedByParent: true })
    await flushPromises()

    expect(wrapper.find('[data-testid="openai-request-timezone-managed-by-parent"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="openai-request-timezone-rewrite-toggle"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="openai-request-timezone-select"]').exists()).toBe(false)
    expect(getTimezonesMock).not.toHaveBeenCalled()

    await wrapper.setProps({ managedByParent: false })
    await flushPromises()
    expect(getTimezonesMock).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="openai-request-timezone-rewrite-toggle"]').exists()).toBe(true)
  })
})
