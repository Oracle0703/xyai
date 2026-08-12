import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

import ConfirmDialog from '../ConfirmDialog.vue'

const BaseDialogStub = defineComponent({
  template: '<div><slot /><slot name="footer" /></div>'
})

describe('ConfirmDialog', () => {
  it('disables both actions while a confirmation is running', () => {
    const wrapper = mount(ConfirmDialog, {
      props: {
        show: true,
        title: 'Reset daily limits',
        message: 'Confirm reset',
        disabled: true
      },
      global: {
        stubs: { BaseDialog: BaseDialogStub }
      }
    })

    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(2)
    expect(buttons.every((button) => button.attributes('disabled') !== undefined)).toBe(true)
  })
})
