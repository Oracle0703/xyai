import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import DepartmentsView from '@/views/admin/DepartmentsView.vue'
import DepartmentMembersDialog from '../DepartmentMembersDialog.vue'
import DepartmentAccessDialog from '../DepartmentAccessDialog.vue'
import DepartmentAssignmentDialog from '../DepartmentAssignmentDialog.vue'
import type { Department, DepartmentMember } from '@/api/admin/departments'

const { api, listUsers, showSuccess, showError } = vi.hoisted(() => ({
  api: { list: vi.fn(), save: vi.fn(), members: vi.fn(), assign: vi.fn(), getAccess: vi.fn(), setAccess: vi.fn() },
  listUsers: vi.fn(), showSuccess: vi.fn(), showError: vi.fn()
}))
vi.mock('@/api/admin/departments', () => ({ departmentsAPI: api, default: api }))
vi.mock('@/api/admin/users', () => ({ default: { list: listUsers } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess, showError }) }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))

const SelectStub = defineComponent({
  props: ['modelValue', 'options', 'disabled'], emits: ['update:modelValue', 'change'],
  setup(props, { emit }) { return () => h('select', { value: props.modelValue, disabled: props.disabled, onChange: (event: Event) => {
    const value = (event.target as HTMLSelectElement).value
    emit('update:modelValue', value); emit('change', value)
  } }, props.options.map((option: { value: string; label: string }) => h('option', { value: option.value }, option.label))) }
})
const DataTableStub = defineComponent({
  props: ['data', 'columns'],
  setup(props, { slots }) { return () => h('div', props.data.map((row: Record<string, unknown>) => h('div', { 'data-row': row.id }, [
    h('span', String(row.name ?? row.email)), ...Object.keys(slots).flatMap(key => slots[key]?.({ row }) ?? [])
  ]))) }
})
const stubs = {
  AppLayout: { template: '<div><slot /></div>' },
  TablePageLayout: { template: '<div><slot name="filters"/><slot name="table"/><slot name="pagination"/></div>' },
  BaseDialog: { props: ['show'], template: '<section v-if="show"><slot/><slot name="footer"/></section>' },
  Select: SelectStub, DataTable: DataTableStub, Pagination: true, RouterLink: { template: '<a><slot/></a>' }
}
const department: Department = { id: 7, organization_key: 'xunyou', name: '研发部', status: 'active', version: 2, sort_order: 0, member_count: 1, active_member_count: 1, managers: [] }
const current: DepartmentMember = { id: 10, email: 'current@xunyou.com', username: '', organization: 'xunyou', status: 'active', department_id: 7, department_name: '研发部', department_version: 2 }
const candidate: DepartmentMember = { ...current, id: 11, email: 'candidate@xunyou.com', department_id: 8, department_name: '运营部', department_version: 5 }
const page = <T,>(items: T[]) => ({ items, total: items.length, page: 1, page_size: 20 })

beforeEach(() => {
  vi.resetAllMocks()
  api.list.mockResolvedValue(page([department]))
  api.save.mockResolvedValue(department)
  api.members.mockImplementation(async (query: { department_id?: string }) => page(query.department_id === '7' ? [current] : [candidate]))
  api.assign.mockResolvedValue({ updated_count: 1 })
  listUsers.mockResolvedValue(page([{ id: 99, email: 'leader@xunyou.com' }]))
  api.getAccess.mockResolvedValue({ user_id: 99, department_ids: [8], permissions: ['admin.subscriptions'], version: 'grant-v1' })
  api.setAccess.mockResolvedValue({ user_id: 99, department_ids: [7, 8], permissions: ['admin.organization_usage', 'admin.department_subscriptions'], version: 'grant-v2' })
})

describe('department administration', () => {
  it('ignores an old department list failure after changing organization', async () => {
    let rejectOld!: (error: Error) => void
    api.list.mockImplementationOnce(() => new Promise((_resolve, reject) => { rejectOld = reject }))
    const wrapper = mount(DepartmentsView, { global: { stubs } })
    await flushPromises()
    const oldSignal = api.list.mock.calls[0][1] as AbortSignal
    api.list.mockResolvedValue(page([{ ...department, name: 'new department', organization_key: 'wsdashi' }]))
    await wrapper.findAll('select')[0]!.setValue('wsdashi')
    await flushPromises()
    rejectOld(new Error('request failed after cancellation'))
    await flushPromises()
    expect(oldSignal.aborted).toBe(true)
    expect(wrapper.text()).toContain('new department')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('ignores a late member list after closing and reopening the dialog', async () => {
    let resolveOld!: (value: ReturnType<typeof page<DepartmentMember>>) => void
    api.members.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
    const wrapper = mount(DepartmentMembersDialog, { props: { show: true, department }, global: { stubs } })
    await flushPromises()
    const oldSignal = api.members.mock.calls[0][1] as AbortSignal
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, department: { ...department, id: 8 } })
    await flushPromises()
    resolveOld(page([current]))
    await flushPromises()
    expect(oldSignal.aborted).toBe(true)
    expect(wrapper.text()).toContain(candidate.email)
    expect(wrapper.text()).not.toContain(current.email)
    wrapper.unmount()
  })

  it('keeps the new assignment snapshot when an aborted request resolves late', async () => {
    let resolveOld!: (value: ReturnType<typeof page<DepartmentMember>>) => void
    api.members.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
    const wrapper = mount(DepartmentAssignmentDialog, { props: { show: true, userIds: [10] }, global: { stubs } })
    await flushPromises()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, userIds: [11] })
    await flushPromises()
    resolveOld(page([current]))
    await flushPromises()
    await wrapper.get('select').setValue('7')
    await wrapper.findAll('button').find(b => b.text() === 'admin.departments.confirmMembers')!.trigger('click')
    await flushPromises()
    expect(api.assign).toHaveBeenCalledWith(7, [{ user_id: 11, expected_department_id: 8, expected_department_version: 5 }])
    wrapper.unmount()
  })

  it('does not restore an old manager permission snapshot after switching users', async () => {
    listUsers.mockResolvedValue(page([{ id: 99, email: 'old@example.com' }, { id: 100, email: 'new@example.com' }]))
    let resolveOld!: (value: unknown) => void
    api.getAccess.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
      .mockResolvedValue({ user_id: 100, department_ids: [7], permissions: ['admin.organization_usage'], version: 'new-version' })
    const wrapper = mount(DepartmentAccessDialog, { props: { show: true, department }, global: { stubs } })
    await flushPromises()
    await wrapper.get('select').setValue('99')
    await wrapper.get('select').setValue('100')
    await flushPromises()
    resolveOld({ user_id: 99, department_ids: [8], permissions: ['admin.subscriptions'], version: 'old-version' })
    await flushPromises()
    await wrapper.get('[data-testid="save-department-access"]').trigger('click')
    await flushPromises()
    expect(api.setAccess).toHaveBeenCalledWith(100, { department_ids: [7], report: true, reset_quota: false, replace_global_subscriptions: false, expected_version: 'new-version' })
    wrapper.unmount()
  })

  it('creates a department under the selected organization and submits its actual name', async () => {
    const wrapper = mount(DepartmentsView, { global: { stubs } })
    await flushPromises()
    await wrapper.findAll('select')[0]!.setValue('wsdashi')
    await flushPromises()
    expect(api.list).toHaveBeenLastCalledWith(expect.objectContaining({ organization: 'wsdashi', page: 1 }), expect.any(AbortSignal))
    await wrapper.get('[data-testid="create-department"]').trigger('click')
    await wrapper.get('[data-testid="department-name"]').setValue('  产品技术部  ')
    await wrapper.get('#department-editor').trigger('submit')
    await flushPromises()
    expect(api.save).toHaveBeenCalledWith(null, expect.objectContaining({ organization_key: 'wsdashi', name: '产品技术部', status: 'active' }))
    wrapper.unmount()
  })

  it('moves members with their previous department and version, without touching subscriptions', async () => {
    const wrapper = mount(DepartmentMembersDialog, { props: { show: true, department }, global: { stubs } })
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'admin.departments.addMembers')!.trigger('click')
    await flushPromises()
    await wrapper.get('input[aria-label="candidate@xunyou.com"]').setValue(true)
    await wrapper.get('[data-testid="apply-department-members"]').trigger('click')
    await flushPromises()
    expect(api.assign).toHaveBeenCalledWith(7, [{ user_id: 11, expected_department_id: 8, expected_department_version: 5 }])
    expect(wrapper.emitted('saved')).toHaveLength(1)
    wrapper.unmount()
  })

  it('clears stale selections after a normalized conflict and requires a fresh confirmation', async () => {
    const wrapper = mount(DepartmentMembersDialog, { props: { show: true, department }, global: { stubs } })
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'admin.departments.addMembers')!.trigger('click')
    await flushPromises()
    await wrapper.get('input[aria-label="candidate@xunyou.com"]').setValue(true)
    api.assign.mockRejectedValueOnce({ status: 409, code: 'DEPARTMENT_CONFLICT' })
    api.members.mockResolvedValue(page([{ ...candidate, department_version: 6 }]))
    await wrapper.get('[data-testid="apply-department-members"]').trigger('click')
    await flushPromises()
    expect(api.assign).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('admin.departments.conflict')
    expect(wrapper.get('[data-testid="apply-department-members"]').attributes('disabled')).toBeDefined()
    await wrapper.get('input[aria-label="candidate@xunyou.com"]').setValue(true)
    await wrapper.get('[data-testid="apply-department-members"]').trigger('click')
    await flushPromises()
    expect(api.assign).toHaveBeenLastCalledWith(7, [{ user_id: 11, expected_department_id: 8, expected_department_version: 6 }])
    wrapper.unmount()
  })

  it('does not revive dormant grants and requires explicit replacement of global subscription access', async () => {
    const wrapper = mount(DepartmentAccessDialog, { props: { show: true, department }, global: { stubs } })
    await flushPromises()
    await wrapper.get('select').setValue('99')
    await flushPromises()
    expect(wrapper.get('[data-testid="save-department-access"]').attributes('disabled')).toBeUndefined()
    const currentGrant = wrapper.findAll('label').find(label => label.text().includes('admin.departments.allowDepartment'))!
    await currentGrant.get('input').setValue(true)
    const resetPermission = wrapper.findAll('label').find(label => label.text().includes('admin.departments.resetPermission'))!
    await resetPermission.get('input').setValue(true)
    const replacement = wrapper.findAll('label').find(label => label.text().includes('admin.departments.replaceGlobal'))!
    expect(wrapper.get('[data-testid="save-department-access"]').attributes('disabled')).toBeDefined()
    await replacement.get('input').setValue(true)
    await wrapper.get('[data-testid="save-department-access"]').trigger('click')
    await flushPromises()
    expect(api.setAccess).toHaveBeenCalledWith(99, { department_ids: [7], report: false, reset_quota: true, replace_global_subscriptions: true, expected_version: 'grant-v1' })
    wrapper.unmount()
  })

  it('preselects the current department so a new manager is not left without scope', async () => {
    const wrapper = mount(DepartmentAccessDialog, { props: { show: true, department }, global: { stubs } })
    await flushPromises()
    await wrapper.get('select').setValue('99')
    await flushPromises()
    const reportPermission = wrapper.findAll('label').find(label => label.text().includes('admin.departments.reportPermission'))!
    await reportPermission.get('input').setValue(true)
    await wrapper.get('[data-testid="save-department-access"]').trigger('click')
    await flushPromises()
    expect(api.setAccess).toHaveBeenCalledWith(99, { department_ids: [7], report: true, reset_quota: false, replace_global_subscriptions: false, expected_version: 'grant-v1' })
    wrapper.unmount()
  })

  it('preserves the live department scope of an existing manager', async () => {
    api.getAccess.mockResolvedValueOnce({ user_id: 99, department_ids: [8], permissions: ['admin.organization_usage'], version: 'live-v1' })
    const wrapper = mount(DepartmentAccessDialog, { props: { show: true, department }, global: { stubs } })
    await flushPromises()
    await wrapper.get('select').setValue('99')
    await flushPromises()
    await wrapper.get('[data-testid="save-department-access"]').trigger('click')
    await flushPromises()
    expect(api.setAccess).toHaveBeenCalledWith(99, { department_ids: [7, 8], report: true, reset_quota: false, replace_global_subscriptions: false, expected_version: 'live-v1' })
    wrapper.unmount()
  })

  it('blocks adding members to an inactive department while allowing member removal', async () => {
    const wrapper = mount(DepartmentMembersDialog, { props: { show: true, department: { ...department, status: 'inactive' as const } }, global: { stubs } })
    await flushPromises()
    expect(wrapper.findAll('button').find(button => button.text() === 'admin.departments.addMembers')!.attributes('disabled')).toBeDefined()
    await wrapper.get('input[aria-label="current@xunyou.com"]').setValue(true)
    await wrapper.get('[data-testid="apply-department-members"]').trigger('click')
    await flushPromises()
    expect(api.assign).toHaveBeenCalledWith(null, [{ user_id: 10, expected_department_id: 7, expected_department_version: 2 }])
    wrapper.unmount()
  })
})
