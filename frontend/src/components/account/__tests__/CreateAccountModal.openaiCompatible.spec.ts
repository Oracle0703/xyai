import { describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { mount } from '@vue/test-utils'

const { createAccountMock, checkMixedChannelRiskMock } = vi.hoisted(() => ({
  createAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isSimpleMode: true
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      create: createAccountMock,
      checkMixedChannelRisk: checkMixedChannelRiskMock
    },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({})
    },
    tlsFingerprintProfiles: {
      list: vi.fn().mockResolvedValue([])
    }
  }
}))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

import CreateAccountModal from '../CreateAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: {
      type: Boolean,
      default: false
    }
  },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: {
    modelValue: {
      type: [String, Number, Boolean, null],
      default: ''
    },
    options: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue'],
  template: `
    <select
      v-bind="$attrs"
      :value="modelValue"
      @change="$emit('update:modelValue', $event.target.value)"
    >
      <option v-for="option in options" :key="option.value" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  `
})

function mountModal() {
  return mount(CreateAccountModal, {
    props: {
      show: true,
      proxies: [],
      groups: []
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: SelectStub,
        Icon: true,
        ProxySelector: true,
        GroupSelector: true,
        ModelWhitelistSelector: true,
        OAuthAuthorizationFlow: true,
        QuotaLimitCard: true,
        ConfirmDialog: true
      }
    }
  })
}

describe('CreateAccountModal OpenAI-compatible providers', () => {
  it('creates a Qwen preset account as an OpenAI API key account', async () => {
    createAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    createAccountMock.mockResolvedValue({ id: 1 })
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })

    const wrapper = mountModal()

    const platformButtons = wrapper.findAll('[data-tour="account-form-platform"] button')
    await platformButtons[1].trigger('click')

    const typeButtons = wrapper.findAll('[data-tour="account-form-type"] button')
    await typeButtons[1].trigger('click')

    const selects = wrapper.findAll('select')
    const providerSelect = selects.find((select) => select.text().includes('Qwen'))
    expect(providerSelect).toBeTruthy()
    await providerSelect!.setValue('qwen')
    await nextTick()

    const inputs = wrapper.findAll('input')
    const nameInput = inputs.find((input) => input.attributes('data-tour') === 'account-form-name')
    expect(nameInput).toBeTruthy()
    await nameInput!.setValue('Qwen Account')

    const apiKeyInput = inputs.find((input) => input.attributes('type') === 'password')
    expect(apiKeyInput).toBeTruthy()
    await apiKeyInput!.setValue('sk-qwen-test')

    await wrapper.get('form#create-account-form').trigger('submit.prevent')

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]).toMatchObject({
      name: 'Qwen Account',
      platform: 'openai',
      type: 'apikey',
      credentials: {
        api_key: 'sk-qwen-test',
        base_url: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
        openai_compatible_provider: 'qwen'
      }
    })
  })
})
