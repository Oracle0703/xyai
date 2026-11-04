import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import DepartmentsView from '@/views/admin/DepartmentsView.vue'
import OrganizationLeaderDialog from '../OrganizationLeaderDialog.vue'
import OrganizationScopeDialog from '../OrganizationScopeDialog.vue'
import type { Department, DepartmentScope, OrganizationLeader } from '@/api/admin/departments'

const { api, auth, showSuccess } = vi.hoisted(() => ({
  api: {
    list: vi.fn(), save: vi.fn(), organizationScope: vi.fn(), organizationLeaders: vi.fn(), organizationLeader: vi.fn(),
    organizationLeaderCandidates: vi.fn(), updateOrganizationLeader: vi.fn(), getOrganizationAccess: vi.fn(), setOrganizationAccess: vi.fn()
  },
  auth: { isAdmin: false, hasAdminPermission: (code: string) => code === 'admin.organization_manage' },
  showSuccess: vi.fn()
}))
vi.mock('@/api/admin/departments', async () => ({ ...await vi.importActual<typeof import('@/api/admin/departments')>('@/api/admin/departments'), departmentsAPI: api, default: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess, showError: vi.fn() }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))

const SelectStub = defineComponent({
  props: ['modelValue', 'options', 'disabled'], emits: ['update:modelValue', 'change'],
  setup(props) { return () => h('select', { value: props.modelValue, 'data-options': props.options.map((o: { value: string }) => o.value).join(',') }) }
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
  Select: SelectStub, DataTable: DataTableStub, Pagination: true, RouterLink: { template: '<a><slot/></a>' },
  DepartmentMembersDialog: true
}
const xunyou: Department = { id: 7, organization_key: 'xunyou', name: '研发部', status: 'active', version: 2, sort_order: 0, member_count: 1, active_member_count: 1, managers: [] }
const speed: Department = { ...xunyou, id: 9, organization_key: 'wsdashi', name: '市场部' }
const scope: DepartmentScope = { unrestricted: false, organizations: ['xunyou', 'wsdashi'], full_organizations: ['xunyou', 'wsdashi'], departments: [xunyou, speed], catalog_version: 'c', default_organization: 'all', default_department_id: 'all' }
const leader: OrganizationLeader = { id: 50, email: 'leader@xunyou.com', username: '', status: 'active', organization: 'xunyou', departments: [{ id: 7, organization_key: 'xunyou', name: '研发部', status: 'active' }], permissions: ['admin.organization_usage'], managed: true }

beforeEach(() => {
  vi.resetAllMocks()
  api.list.mockResolvedValue({ items: [xunyou], total: 1, page: 1, page_size: 20 })
  api.organizationScope.mockResolvedValue(scope)
  api.organizationLeaders.mockResolvedValue({ items: [leader], total: 1, page: 1, page_size: 100 })
  api.organizationLeader.mockResolvedValue({ ...leader, departments: [leader.departments[0]!, { id: 9, organization_key: 'wsdashi', name: '市场部', status: 'active' }], version: 'leader-v1' })
  api.updateOrganizationLeader.mockResolvedValue({})
})

describe('organization leader department page', () => {
  it('limits organizations to the grant and replaces the access dialog with leader appointment', async () => {
    const wrapper = mount(DepartmentsView, { global: { stubs: { ...stubs, OrganizationLeaderDialog: true } } })
    await flushPromises()
    expect(api.organizationScope).toHaveBeenCalled()
    expect(wrapper.get('select').attributes('data-options')).toBe('xunyou,wsdashi')
    expect(wrapper.text()).not.toContain('admin.departments.manageAccess')
    expect(wrapper.find('[data-testid="open-organization-leaders"]').exists()).toBe(true)
  })
})

describe('OrganizationLeaderDialog', () => {
  it('edits from the full detail snapshot so grants of another organization are kept', async () => {
    const wrapper = mount(OrganizationLeaderDialog, { props: { show: true, organizations: ['xunyou', 'wsdashi'], departments: [xunyou, speed] }, global: { stubs } })
    await flushPromises()
    await wrapper.get('[data-testid="edit-leader-50"]').trigger('click')
    await flushPromises()
    expect(api.organizationLeader).toHaveBeenCalledWith(50, expect.anything())
    expect((wrapper.get('[data-testid="leader-department-9"]').element as HTMLInputElement).checked).toBe(true)
    await wrapper.get('[data-testid="leader-members"]').setValue(true)
    await wrapper.get('[data-testid="save-leader"]').trigger('click')
    await flushPromises()
    expect(api.updateOrganizationLeader).toHaveBeenCalledWith(50, { department_ids: [7, 9], report: true, reset_quota: false, manage_members: true, expected_version: 'leader-v1' })
  })

  it('revokes only after confirmation with an empty grant set', async () => {
    const wrapper = mount(OrganizationLeaderDialog, { props: { show: true, organizations: ['xunyou'], departments: [xunyou] }, global: { stubs } })
    await flushPromises()
    await wrapper.get('[data-testid="edit-leader-50"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="revoke-leader"]').trigger('click')
    expect(api.updateOrganizationLeader).not.toHaveBeenCalled()
    await wrapper.get('[data-testid="revoke-leader"]').trigger('click')
    await flushPromises()
    expect(api.updateOrganizationLeader).toHaveBeenCalledWith(50, { department_ids: [], report: false, reset_quota: false, manage_members: false, expected_version: 'leader-v1' })
  })

  it('cannot save an appointment without a department', async () => {
    api.organizationLeaderCandidates.mockResolvedValue([{ id: 60, email: 'new@xunyou.com', username: '', organization: 'xunyou', department_id: null, department_name: null, version: 'cand-v1' }])
    const wrapper = mount(OrganizationLeaderDialog, { props: { show: true, organizations: ['xunyou'], departments: [xunyou] }, global: { stubs } })
    await flushPromises()
    await wrapper.get('[data-testid="appoint-leader"]').trigger('click')
    await wrapper.get('[data-testid="leader-candidate-query"]').setValue('new')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    await wrapper.get('[data-testid="choose-candidate-60"]').trigger('click')
    expect(wrapper.get('[data-testid="save-leader"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-testid="revoke-leader"]').exists()).toBe(false)
    await wrapper.get('[data-testid="leader-department-7"]').setValue(true)
    await wrapper.get('[data-testid="save-leader"]').trigger('click')
    await flushPromises()
    expect(api.updateOrganizationLeader).toHaveBeenCalledWith(60, { department_ids: [7], report: true, reset_quota: true, manage_members: false, expected_version: 'cand-v1' })
  })
})

describe('OrganizationScopeDialog', () => {
  it('requires explicit confirmation before replacing department grants', async () => {
    api.getOrganizationAccess.mockResolvedValue({ user_id: 5, department_ids: [7], organizations: [], permissions: ['admin.organization_usage'], version: 'v1' })
    api.setOrganizationAccess.mockResolvedValue({})
    const wrapper = mount(OrganizationScopeDialog, { props: { show: true, userId: 5 }, global: { stubs } })
    await flushPromises()
    await wrapper.get('[data-testid="organization-scope-xunyou"]').setValue(true)
    expect(wrapper.get('[data-testid="save-organization-scope"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="organization-scope-replace-departments"]').setValue(true)
    await wrapper.get('[data-testid="save-organization-scope"]').trigger('click')
    await flushPromises()
    expect(api.setOrganizationAccess).toHaveBeenCalledWith(5, { organizations: ['xunyou'], expected_version: 'v1', replace_department_grants: true, replace_global_subscriptions: false })
  })

  it('revokes with an explicit empty array', async () => {
    api.getOrganizationAccess.mockResolvedValue({ user_id: 5, department_ids: [], organizations: ['wsdashi'], permissions: [], version: 'v2' })
    api.setOrganizationAccess.mockResolvedValue({})
    const wrapper = mount(OrganizationScopeDialog, { props: { show: true, userId: 5 }, global: { stubs } })
    await flushPromises()
    await wrapper.get('[data-testid="organization-scope-wsdashi"]').setValue(false)
    await wrapper.get('[data-testid="save-organization-scope"]').trigger('click')
    await flushPromises()
    expect(api.setOrganizationAccess).toHaveBeenCalledWith(5, { organizations: [], expected_version: 'v2', replace_department_grants: false, replace_global_subscriptions: false })
  })
})
