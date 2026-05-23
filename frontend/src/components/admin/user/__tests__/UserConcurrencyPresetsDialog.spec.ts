import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import UserConcurrencyPresetsDialog from '../UserConcurrencyPresetsDialog.vue'

const {
  listPresets,
  createPreset,
  updatePreset,
  deletePreset,
  applyPreset,
  listPresetRuns,
  listUsers,
  showError,
  showSuccess
} = vi.hoisted(() => ({
  listPresets: vi.fn(),
  createPreset: vi.fn(),
  updatePreset: vi.fn(),
  deletePreset: vi.fn(),
  applyPreset: vi.fn(),
  listPresetRuns: vi.fn(),
  listUsers: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin/userConcurrencyPresets', () => ({
  listPresets,
  createPreset,
  updatePreset,
  deletePreset,
  applyPreset,
  listPresetRuns
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      list: listUsers
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

const BaseDialogStub = {
  props: ['show', 'title'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
}

const user = {
  id: 11,
  email: 'user@example.com',
  username: 'user',
  role: 'user',
  balance: 0,
  concurrency: 3,
  status: 'active',
  allowed_groups: [],
  balance_notify_enabled: false,
  balance_notify_threshold: null,
  balance_notify_extra_emails: [],
  created_at: '',
  updated_at: '',
  notes: '',
  current_concurrency: 0
}

function findButtonByText(wrapper: any, text: string) {
  const button = wrapper.findAll('button').find((button: any) => button.text() === text)
  expect(button, `button "${text}" should exist`).toBeTruthy()
  return button!
}

function findButtonContainingText(wrapper: any, text: string) {
  const button = wrapper.findAll('button').find((button: any) => button.text().includes(text))
  expect(button, `button containing "${text}" should exist`).toBeTruthy()
  return button!
}

async function selectUser(wrapper: any) {
  await findButtonContainingText(wrapper, user.email).trigger('click')
}

describe('UserConcurrencyPresetsDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    listPresets.mockResolvedValue([])
    createPreset.mockResolvedValue({ id: 1, name: 'day', target_concurrency: 12, user_ids: [11] })
    updatePreset.mockResolvedValue({ id: 1, name: 'day', target_concurrency: 12, user_ids: [11] })
    deletePreset.mockResolvedValue(undefined)
    applyPreset.mockResolvedValue({ id: 5, status: 'success', affected_count: 1, trigger: 'manual' })
    listPresetRuns.mockResolvedValue([])
    listUsers.mockResolvedValue({ items: [user], total: 1, page: 1, page_size: 20, pages: 1 })
  })

  it('creates preset with selected users and daily schedule', async () => {
    const wrapper = mount(UserConcurrencyPresetsDialog, {
      props: { show: true },
      global: { stubs: { BaseDialog: BaseDialogStub } }
    })

    await flushPromises()
    await selectUser(wrapper)
    await wrapper.find('input[placeholder="例如 白天高并发"]').setValue('day')
    const numberInput = wrapper.find('input[type="number"]')
    await numberInput.setValue(12)
    await wrapper.find('input[type="checkbox"]').setValue(true)
    await wrapper.find('input[placeholder="09:00"]').setValue('09:00')
    await findButtonByText(wrapper, '创建').trigger('click')
    await flushPromises()

    expect(createPreset).toHaveBeenCalledWith(expect.objectContaining({
      name: 'day',
      target_concurrency: 12,
      user_ids: [11],
      schedule_enabled: true,
      schedule_time: '09:00'
    }))
  })

  it('blocks invalid schedule time', async () => {
    const wrapper = mount(UserConcurrencyPresetsDialog, {
      props: { show: true },
      global: { stubs: { BaseDialog: BaseDialogStub } }
    })

    await flushPromises()
    await selectUser(wrapper)
    await wrapper.find('input[placeholder="例如 白天高并发"]').setValue('day')
    await wrapper.find('input[type="checkbox"]').setValue(true)
    await wrapper.find('input[placeholder="09:00"]').setValue('25:00')
    await findButtonByText(wrapper, '创建').trigger('click')

    expect(createPreset).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('执行时间必须是 HH:mm 格式')
  })

  it('applies selected preset after confirmation', async () => {
    listPresets.mockResolvedValue([{
      id: 1,
      name: 'day',
      description: '',
      target_concurrency: 12,
      user_ids: [11],
      schedule_enabled: false,
      schedule_time: '',
      created_at: '',
      updated_at: ''
    }])

    const wrapper = mount(UserConcurrencyPresetsDialog, {
      props: { show: true },
      global: { stubs: { BaseDialog: BaseDialogStub } }
    })

    await flushPromises()
    const applyButton = wrapper.findAll('button').find((button) => button.text() === '应用方案')
    expect(applyButton).toBeTruthy()
    await applyButton!.trigger('click')
    await flushPromises()

    expect(applyPreset).toHaveBeenCalledWith(1)
    expect(wrapper.emitted('applied')).toBeTruthy()
  })
})
