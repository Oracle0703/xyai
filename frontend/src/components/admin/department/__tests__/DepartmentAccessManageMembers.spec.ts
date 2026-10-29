import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import DepartmentAccessDialog from '../DepartmentAccessDialog.vue'
import type { Department } from '@/api/admin/departments'

// Locked acceptance tests for docs/features/department-member-management-design-cn.md §2 and review item R3:
// member management is an explicit, default-off permission and saving never grants report or quota reset implicitly.

const { api, listUsers } = vi.hoisted(() => ({
  api: { getAccess: vi.fn(), setAccess: vi.fn() },
  listUsers: vi.fn()
}))
vi.mock('@/api/admin/departments', () => ({ departmentsAPI: api, default: api }))
vi.mock('@/api/admin/users', () => ({ default: { list: listUsers } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))

const SelectStub = defineComponent({
  props: ['modelValue', 'options', 'disabled'], emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () => h('select', { value: props.modelValue, disabled: props.disabled, onChange: (event: Event) => emit('update:modelValue', (event.target as HTMLSelectElement).value) },
      props.options.map((option: { value: string; label: string }) => h('option', { value: option.value }, option.label)))
  }
})
const stubs = { BaseDialog: { props: ['show'], template: '<section v-if="show"><slot/><slot name="footer"/></section>' }, Select: SelectStub }
const department: Department = { id: 7, organization_key: 'xunyou', name: '研发部', status: 'active', version: 2, sort_order: 0, member_count: 1, active_member_count: 1, managers: [] }

async function openFor(permissions: string[], departmentIDs: number[]) {
  api.getAccess.mockResolvedValue({ user_id: 99, department_ids: departmentIDs, permissions, version: 'grant-v1' })
  const wrapper = mount(DepartmentAccessDialog, { props: { show: true, department }, global: { stubs } })
  await flushPromises()
  await wrapper.get('select').setValue('99')
  await flushPromises()
  return wrapper
}

async function save(wrapper: Awaited<ReturnType<typeof openFor>>) {
  await wrapper.get('[data-testid="save-department-access"]').trigger('click')
  await flushPromises()
  return api.setAccess.mock.calls.at(-1)?.[1]
}

beforeEach(() => {
  vi.resetAllMocks()
  listUsers.mockResolvedValue({ items: [{ id: 99, email: 'leader@xunyou.com' }], total: 1, page: 1, page_size: 30 })
  api.setAccess.mockImplementation(async (userID: number, input: { department_ids: number[] }) => ({ user_id: userID, department_ids: input.department_ids, permissions: [], version: 'grant-v2' }))
})

describe('department access member-management permission', () => {
  it('restores a member-only manager without adding report or quota reset', async () => {
    const wrapper = await openFor(['admin.department_members'], [7])
    expect((wrapper.get('[data-testid="access-manage-members"]').element as HTMLInputElement).checked).toBe(true)
    expect(await save(wrapper)).toEqual(expect.objectContaining({ department_ids: [7], report: false, reset_quota: false, manage_members: true }))
    wrapper.unmount()
  })

  it('keeps member management off by default for a new manager', async () => {
    const wrapper = await openFor([], [])
    expect((wrapper.get('[data-testid="access-manage-members"]').element as HTMLInputElement).checked).toBe(false)
    expect(await save(wrapper)).toEqual(expect.objectContaining({ department_ids: [7], report: true, reset_quota: true, manage_members: false }))
    wrapper.unmount()
  })

  it('does not enable member management because other department permissions exist', async () => {
    const wrapper = await openFor(['admin.organization_usage'], [7])
    expect(await save(wrapper)).toEqual(expect.objectContaining({ report: true, reset_quota: false, manage_members: false }))
    wrapper.unmount()
  })

  it('submits member management only when explicitly checked', async () => {
    const wrapper = await openFor(['admin.organization_usage'], [7])
    await wrapper.get('[data-testid="access-manage-members"]').setValue(true)
    expect(await save(wrapper)).toEqual(expect.objectContaining({ report: true, reset_quota: false, manage_members: true }))
    wrapper.unmount()
  })
})
