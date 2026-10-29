import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import MyDepartmentView from '../MyDepartmentView.vue'
import type { Department, DepartmentMember, DepartmentPage, DepartmentScope, ManagedDepartmentMember } from '@/api/admin/departments'

const { api, auth, showSuccess } = vi.hoisted(() => ({
  api: { managedScope: vi.fn(), managedMembers: vi.fn(), managedCandidates: vi.fn(), managedAssign: vi.fn(), updateManagedMember: vi.fn() },
  auth: { isAdmin: false },
  showSuccess: vi.fn()
}))
vi.mock('@/api/admin/departments', () => ({ departmentsAPI: api, default: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => auth }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))

const SelectStub = defineComponent({
  props: ['modelValue', 'options', 'disabled'], emits: ['update:modelValue', 'change'],
  setup(props, { emit }) {
    return () => h('select', {
      value: props.modelValue, disabled: props.disabled,
      onChange: (event: Event) => {
        const value = (event.target as HTMLSelectElement).value
        emit('update:modelValue', value)
        emit('change', value)
      }
    }, props.options.map((option: { value: string; label: string }) => h('option', { value: option.value }, option.label)))
  }
})
const DataTableStub = defineComponent({
  props: ['data', 'columns', 'loading'],
  setup(props, { slots }) {
    return () => h('div', props.data.length
      ? props.data.map((row: Record<string, unknown>) => h('div', { 'data-row': row.id }, props.columns.map((column: { key: string }) =>
        slots[`cell-${column.key}`]?.({ row, value: row[column.key] }) ?? h('span', String(row[column.key] ?? '')))))
      : slots.empty?.())
  }
})
const stubs = {
  AppLayout: { template: '<div><slot/></div>' },
  TablePageLayout: { template: '<div><slot name="filters"/><slot name="table"/><slot name="pagination"/></div>' },
  BaseDialog: { props: ['show', 'title'], template: '<section v-if="show" :data-dialog="title"><slot/><slot name="footer"/></section>' },
  Select: SelectStub, DataTable: DataTableStub, Pagination: true
}
const department: Department = { id: 7, organization_key: 'xunyou', name: '研发部', status: 'active', version: 1, sort_order: 0, member_count: 1, active_member_count: 1, managers: [] }
const member: ManagedDepartmentMember = { id: 10, email: 'member@xunyou.com', username: 'original', department_note: 'original note', status: 'active', disabled_source: null, can_enable: false, manageable: true, department_id: 7, department_name: '研发部', department_version: 3 }
const candidate: DepartmentMember = { id: 20, email: 'candidate@xunyou.com', username: 'candidate', status: 'active', organization: 'xunyou', department_id: null, department_name: '', department_version: 2 }
const asPage = <T,>(items: T[]): DepartmentPage<T> => ({ items, total: items.length, page: 1, page_size: 20 })
const scope = (departments = [department]): DepartmentScope => ({ unrestricted: false, organizations: ['xunyou'], departments, catalog_version: 'catalog', default_organization: 'xunyou', default_department_id: '7' })
const denied = (reason: string, status = 403) => ({ status, code: status, reason })
const wrappers: VueWrapper[] = []

async function openPage() {
  const wrapper = mount(MyDepartmentView, { global: { stubs } })
  wrappers.push(wrapper)
  await flushPromises()
  return wrapper
}
async function openCandidates(wrapper: VueWrapper, query = 'cand') {
  await wrapper.get('[data-testid="open-add-members"]').trigger('click')
  await wrapper.get('[data-testid="candidate-query"]').setValue(query)
  await wrapper.get('[data-testid="candidate-search"]').trigger('submit')
  await flushPromises()
}
async function openEditor(wrapper: VueWrapper) {
  await wrapper.get('[data-testid="edit-member-10"]').trigger('click')
}

beforeEach(() => {
  vi.resetAllMocks()
  auth.isAdmin = false
  api.managedScope.mockResolvedValue(scope())
  api.managedMembers.mockResolvedValue(asPage([member]))
  api.managedCandidates.mockResolvedValue([candidate])
  api.managedAssign.mockResolvedValue({ updated_count: 1 })
  api.updateManagedMember.mockResolvedValue(member)
})
afterEach(() => { for (const wrapper of wrappers.splice(0)) wrapper.unmount() })

describe('MyDepartmentView', () => {
  it('selects the only authorized department and queries only its dedicated member endpoint', async () => {
    const wrapper = await openPage()
    expect((wrapper.get('[data-testid="managed-department"]').element as HTMLSelectElement).value).toBe('7')
    expect(api.managedScope).toHaveBeenCalledTimes(1)
    expect(api.managedMembers).toHaveBeenCalledWith({ department_id: '7', status: '', q: '', page: 1, page_size: 20 }, expect.any(AbortSignal))
    expect(wrapper.text()).toContain(member.email)
  })

  it('distinguishes departments with the same name across organizations in selection, rows and the candidate dialog', async () => {
    api.managedScope.mockResolvedValue(scope([department, { ...department, id: 8, organization_key: 'wsdashi' }]))
    api.managedMembers.mockResolvedValue(asPage([member, { ...member, id: 11, department_id: 8 }]))
    const wrapper = await openPage()
    const options = wrapper.get('[data-testid="managed-department"]').findAll('option').map(option => option.text())
    expect(options).toContain('迅游 · 研发部')
    expect(options).toContain('速宝 · 研发部')
    expect(wrapper.get('[data-row="10"]').text()).toContain('迅游 · 研发部')
    expect(wrapper.get('[data-row="11"]').text()).toContain('速宝 · 研发部')
    await wrapper.get('[data-testid="managed-department"]').setValue('8')
    await flushPromises()
    await wrapper.get('[data-testid="open-add-members"]').trigger('click')
    expect(wrapper.get('section[data-dialog]').attributes('data-dialog')).toBe('admin.departments.addMembers · 速宝 · 研发部')
  })

  it('shows enable only for can_enable and hides notes and actions on readonly rows', async () => {
    api.managedMembers.mockResolvedValue(asPage([
      member,
      { ...member, id: 11, status: 'disabled', disabled_source: 'department_manager', can_enable: true },
      { ...member, id: 12, status: 'disabled', disabled_source: 'other' },
      { ...member, id: 13, manageable: false, department_note: 'READONLY SECRET' }
    ]))
    const wrapper = await openPage()
    expect(wrapper.find('[data-testid="disable-member-10"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="enable-member-11"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="enable-member-12"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="edit-member-13"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="remove-member-13"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('READONLY SECRET')
    expect(wrapper.text()).toContain('admin.departments.managerDisabled')
    expect(wrapper.text()).toContain('admin.departments.otherDisabled')
  })

  it('renders an empty authorization state without repeatedly refreshing scope', async () => {
    api.managedScope.mockRejectedValue(denied('DEPARTMENT_SCOPE_DENIED'))
    const wrapper = await openPage()
    expect(wrapper.get('[data-testid="managed-empty"]').text()).toBe('admin.departments.memberManagement.noScope')
    expect(api.managedScope).toHaveBeenCalledTimes(1)
    expect(api.managedMembers).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="open-add-members"]').attributes('disabled')).toBeDefined()
  })

  it('stops a denied list after a scope refresh without a recursive request loop', async () => {
    api.managedMembers.mockRejectedValue(denied('DEPARTMENT_SCOPE_DENIED'))
    const wrapper = await openPage()
    expect(api.managedScope).toHaveBeenCalledTimes(1)
    expect(api.managedMembers).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-testid="managed-empty"]').text()).toContain('noScope')
  })

  it('submits only changed profile fields with the original membership version', async () => {
    const wrapper = await openPage()
    await openEditor(wrapper)
    await wrapper.get('[data-testid="managed-note"]').setValue('  updated note  ')
    await wrapper.get('#managed-member-editor').trigger('submit')
    await flushPromises()
    expect(api.updateManagedMember).toHaveBeenCalledTimes(1)
    expect(api.updateManagedMember).toHaveBeenCalledWith(10, { expected_department_id: 7, expected_department_version: 3, department_note: 'updated note' })
    expect(api.managedMembers).toHaveBeenCalledTimes(2)
  })

  it.each(['a'.repeat(100), '名'.repeat(33), ''])('accepts valid byte-length usernames and Unicode notes: %s', async username => {
    const wrapper = await openPage()
    await openEditor(wrapper)
    await wrapper.get('[data-testid="managed-username"]').setValue(`  ${username}  `)
    await wrapper.get('[data-testid="managed-note"]').setValue('🙂'.repeat(1000))
    await wrapper.get('#managed-member-editor').trigger('submit')
    await flushPromises()
    expect(api.updateManagedMember).toHaveBeenCalledTimes(1)
    expect(api.updateManagedMember).toHaveBeenCalledWith(10, {
      expected_department_id: 7, expected_department_version: 3, username, department_note: '🙂'.repeat(1000)
    })
  })

  it.each([
    ['managed-username', 'a'.repeat(101), 'usernameLimit'],
    ['managed-username', '名'.repeat(34), 'usernameLimit'],
    ['managed-note', '🙂'.repeat(1001), 'noteLimit']
  ])('blocks an invalid %s before submitting', async (field, value, key) => {
    const wrapper = await openPage()
    await openEditor(wrapper)
    await wrapper.get(`[data-testid="${field}"]`).setValue(value)
    expect(wrapper.get('[data-testid="profile-error"]').text()).toBe(`admin.departments.memberManagement.${key}`)
    expect(wrapper.get('[data-testid="save-managed-profile"]').attributes('disabled')).toBeDefined()
    await wrapper.get('#managed-member-editor').trigger('submit')
    expect(api.updateManagedMember).not.toHaveBeenCalled()
  })

  it('keeps department notes readonly for a full administrator', async () => {
    auth.isAdmin = true
    const wrapper = await openPage()
    await openEditor(wrapper)
    expect(wrapper.get('[data-testid="managed-note"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="managed-username"]').setValue('renamed')
    await wrapper.get('#managed-member-editor').trigger('submit')
    await flushPromises()
    expect(api.updateManagedMember).toHaveBeenCalledTimes(1)
    expect(api.updateManagedMember).toHaveBeenCalledWith(10, { expected_department_id: 7, expected_department_version: 3, username: 'renamed' })
  })

  it('uses a status-only payload for disable and never replays profile fields', async () => {
    const wrapper = await openPage()
    await wrapper.get('[data-testid="disable-member-10"]').trigger('click')
    expect(wrapper.text()).toContain('admin.departments.memberManagement.disableHint')
    await wrapper.get('[data-testid="confirm-managed-action"]').trigger('click')
    await flushPromises()
    expect(api.updateManagedMember).toHaveBeenCalledTimes(1)
    expect(api.updateManagedMember).toHaveBeenCalledWith(10, { expected_department_id: 7, expected_department_version: 3, status: 'disabled' })
  })

  it('handles enable-denied by refreshing the member without discarding its department scope', async () => {
    api.managedMembers.mockResolvedValueOnce(asPage([{ ...member, status: 'disabled', disabled_source: 'department_manager', can_enable: true }]))
      .mockResolvedValue(asPage([{ ...member, status: 'disabled', disabled_source: 'other', can_enable: false }]))
    api.updateManagedMember.mockRejectedValueOnce(denied('DEPARTMENT_MEMBER_ENABLE_DENIED'))
    const wrapper = await openPage()
    await wrapper.get('[data-testid="enable-member-10"]').trigger('click')
    await wrapper.get('[data-testid="confirm-managed-action"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="managed-error"]').text()).toBe('admin.departments.enableDenied')
    expect(api.managedScope).toHaveBeenCalledTimes(1)
    expect(api.managedMembers).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="enable-member-10"]').exists()).toBe(false)
    expect(wrapper.text()).toContain(member.email)
  })

  it('clears protected data and refreshes scope once after a permission denial', async () => {
    const wrapper = await openPage()
    api.updateManagedMember.mockRejectedValueOnce(denied('DEPARTMENT_SCOPE_DENIED'))
    api.managedScope.mockRejectedValueOnce(denied('DEPARTMENT_SCOPE_DENIED'))
    await openEditor(wrapper)
    await wrapper.get('[data-testid="managed-note"]').setValue('unsaved')
    await wrapper.get('#managed-member-editor').trigger('submit')
    await flushPromises()
    expect(api.managedScope).toHaveBeenCalledTimes(2)
    expect(api.updateManagedMember).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).not.toContain(member.email)
    expect(wrapper.find('#managed-member-editor').exists()).toBe(false)
    expect(wrapper.get('[data-testid="managed-empty"]').text()).toContain('noScope')
  })

  it('discards the editor on a version conflict and requires a new explicit write', async () => {
    api.updateManagedMember.mockRejectedValueOnce(denied('DEPARTMENT_CONFLICT', 409))
    const wrapper = await openPage()
    api.managedMembers.mockResolvedValue(asPage([{ ...member, department_version: 5 }]))
    await openEditor(wrapper)
    await wrapper.get('[data-testid="managed-username"]').setValue('renamed')
    await wrapper.get('#managed-member-editor').trigger('submit')
    await flushPromises()
    expect(api.updateManagedMember).toHaveBeenCalledTimes(1)
    expect(wrapper.find('#managed-member-editor').exists()).toBe(false)
    expect(wrapper.get('[data-testid="managed-error"]').text()).toContain('conflict')
    await openEditor(wrapper)
    await wrapper.get('[data-testid="managed-username"]').setValue('confirmed')
    await wrapper.get('#managed-member-editor').trigger('submit')
    await flushPromises()
    expect(api.updateManagedMember).toHaveBeenLastCalledWith(10, { expected_department_id: 7, expected_department_version: 5, username: 'confirmed' })
  })

  it('keeps a failed profile draft and displays the failure inside the open dialog without replaying the request', async () => {
    api.updateManagedMember.mockRejectedValueOnce(new Error('network unavailable'))
    const wrapper = await openPage()
    await openEditor(wrapper)
    await wrapper.get('[data-testid="managed-note"]').setValue('keep this draft')
    await wrapper.get('#managed-member-editor').trigger('submit')
    await flushPromises()
    expect(wrapper.get('#managed-member-editor').text()).toContain('admin.departments.failed')
    expect((wrapper.get('[data-testid="managed-note"]').element as HTMLTextAreaElement).value).toBe('keep this draft')
    expect(api.updateManagedMember).toHaveBeenCalledTimes(1)
    expect(api.managedMembers).toHaveBeenCalledTimes(1)
  })

  it('removes a member with the frozen assignment snapshot and shows the note clearing warning', async () => {
    const wrapper = await openPage()
    await wrapper.get('[data-testid="remove-member-10"]').trigger('click')
    expect(wrapper.text()).toContain('admin.departments.memberManagement.removeHint')
    await wrapper.get('[data-testid="confirm-managed-action"]').trigger('click')
    await flushPromises()
    expect(api.managedAssign).toHaveBeenCalledTimes(1)
    expect(api.managedAssign).toHaveBeenCalledWith(null, [{ user_id: 10, expected_department_id: 7, expected_department_version: 3 }])
  })

  it('blocks additions to inactive departments while preserving existing member actions', async () => {
    api.managedScope.mockResolvedValue(scope([{ ...department, status: 'inactive' }]))
    const wrapper = await openPage()
    expect(wrapper.get('[data-testid="open-add-members"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-testid="edit-member-10"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="remove-member-10"]').exists()).toBe(true)
    expect(api.managedCandidates).not.toHaveBeenCalled()
  })

  it.each(['', ' a ', '🙂'])('does not search candidates with fewer than two characters: %s', async query => {
    const wrapper = await openPage()
    await openCandidates(wrapper, query)
    expect(api.managedCandidates).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="candidate-error"]').text()).toContain('candidateMinimum')
  })

  it('caps candidate display at twenty and submits only the selected unassigned snapshot', async () => {
    api.managedCandidates.mockResolvedValue(Array.from({ length: 21 }, (_, index) => ({ ...candidate, id: 20 + index, email: `candidate-${index}@xunyou.com` })))
    const wrapper = await openPage()
    await openCandidates(wrapper, ' cand ')
    expect(api.managedCandidates).toHaveBeenCalledWith(7, 'cand', expect.any(AbortSignal))
    expect(wrapper.findAll('input[type="checkbox"]')).toHaveLength(20)
    await wrapper.get('[data-testid="candidate-20"]').setValue(true)
    await wrapper.get('[data-testid="add-selected-members"]').trigger('click')
    await flushPromises()
    expect(api.managedAssign).toHaveBeenCalledTimes(1)
    expect(api.managedAssign).toHaveBeenCalledWith(7, [{ user_id: 20, expected_department_id: null, expected_department_version: 2 }])
  })

  it('clears candidate selection as soon as the search text changes', async () => {
    const wrapper = await openPage()
    await openCandidates(wrapper)
    await wrapper.get('[data-testid="candidate-20"]').setValue(true)
    await wrapper.get('[data-testid="candidate-query"]').setValue('changed')
    expect(wrapper.find('[data-testid="candidate-20"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="add-selected-members"]').attributes('disabled')).toBeDefined()
    expect(api.managedCandidates).toHaveBeenCalledTimes(1)
  })

  it.each(['success', 'failure'])('ignores a late member %s after switching departments even when transport ignores abort', async result => {
    let resolveOld!: (value: DepartmentPage<ManagedDepartmentMember>) => void
    let rejectOld!: (error: unknown) => void
    api.managedScope.mockResolvedValue(scope([department, { ...department, id: 8, name: '运营部' }]))
    api.managedMembers.mockImplementationOnce(() => new Promise((resolve, reject) => { resolveOld = resolve; rejectOld = reject }))
      .mockResolvedValue(asPage([{ ...member, id: 80, email: 'new@xunyou.com', department_id: 8 }]))
    const wrapper = await openPage()
    const oldSignal = api.managedMembers.mock.calls[0][1] as AbortSignal
    await wrapper.get('[data-testid="managed-department"]').setValue('8')
    await flushPromises()
    if (result === 'success') resolveOld(asPage([member]))
    else rejectOld(denied('DEPARTMENT_SCOPE_DENIED'))
    await flushPromises()
    expect(oldSignal.aborted).toBe(true)
    expect(wrapper.text()).toContain('new@xunyou.com')
    expect(wrapper.text()).not.toContain(member.email)
    expect(wrapper.find('[data-testid="managed-error"]').exists()).toBe(false)
    expect(api.managedScope).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-testid="open-add-members"]').attributes('disabled')).toBeUndefined()
  })

  it('discards candidates and selections when switching departments and ignores the old response', async () => {
    api.managedScope.mockResolvedValue(scope([department, { ...department, id: 8 }]))
    const wrapper = await openPage()
    await openCandidates(wrapper)
    await wrapper.get('[data-testid="candidate-20"]').setValue(true)
    let resolveOld!: (value: DepartmentMember[]) => void
    api.managedCandidates.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
    await wrapper.get('[data-testid="candidate-search"]').trigger('submit')
    const oldSignal = api.managedCandidates.mock.calls[1][2] as AbortSignal
    await wrapper.get('[data-testid="managed-department"]').setValue('8')
    await flushPromises()
    resolveOld([candidate])
    await flushPromises()
    expect(oldSignal.aborted).toBe(true)
    expect(wrapper.find('[data-testid="candidate-query"]').exists()).toBe(false)
    await wrapper.get('[data-testid="open-add-members"]').trigger('click')
    expect(wrapper.find('[data-testid="candidate-20"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="add-selected-members"]').attributes('disabled')).toBeDefined()
  })

  it('does not restore old candidate data after a scope denial', async () => {
    const wrapper = await openPage()
    let resolveOld!: (value: DepartmentMember[]) => void
    api.managedCandidates.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
    await openCandidates(wrapper)
    const oldSignal = api.managedCandidates.mock.calls[0][2] as AbortSignal
    api.managedScope.mockRejectedValueOnce(denied('DEPARTMENT_SCOPE_DENIED'))
    await wrapper.get('[data-testid="refresh-managed-scope"]').trigger('click')
    await flushPromises()
    resolveOld([candidate])
    await flushPromises()
    expect(oldSignal.aborted).toBe(true)
    expect(wrapper.text()).not.toContain(candidate.email)
    expect(wrapper.text()).not.toContain(member.email)
    expect(wrapper.find('[data-testid="candidate-query"]').exists()).toBe(false)
    expect(api.managedScope).toHaveBeenCalledTimes(2)
  })

  it.each(['success', 'failure'])('preserves the denied scope when an in-flight member request returns a late %s', async result => {
    let resolveOld!: (value: DepartmentPage<ManagedDepartmentMember>) => void
    let rejectOld!: (error: unknown) => void
    api.managedMembers.mockImplementationOnce(() => new Promise((resolve, reject) => { resolveOld = resolve; rejectOld = reject }))
    const wrapper = await openPage()
    const oldSignal = api.managedMembers.mock.calls[0][1] as AbortSignal
    api.managedScope.mockRejectedValueOnce(denied('DEPARTMENT_SCOPE_DENIED'))
    await wrapper.get('[data-testid="refresh-managed-scope"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="managed-error"]').text()).toBe('admin.departments.scopeDenied')
    if (result === 'success') resolveOld(asPage([member]))
    else rejectOld(new Error('late response must not replace the scope denial'))
    await flushPromises()
    expect(oldSignal.aborted).toBe(true)
    expect(wrapper.text()).not.toContain(member.email)
    expect(wrapper.get('[data-testid="managed-empty"]').text()).toContain('noScope')
    expect(wrapper.get('[data-testid="managed-error"]').text()).toBe('admin.departments.scopeDenied')
    expect(wrapper.get('[data-testid="open-add-members"]').attributes('disabled')).toBeDefined()
    expect(api.managedScope).toHaveBeenCalledTimes(2)
    expect(api.managedMembers).toHaveBeenCalledTimes(1)
  })

  it.each(['success', 'failure'])('ignores a late candidate %s after closing and reopening the dialog', async result => {
    let resolveOld!: (value: DepartmentMember[]) => void
    let rejectOld!: (error: unknown) => void
    api.managedCandidates.mockImplementationOnce(() => new Promise((resolve, reject) => { resolveOld = resolve; rejectOld = reject }))
    const wrapper = await openPage()
    await openCandidates(wrapper)
    const oldSignal = api.managedCandidates.mock.calls[0][2] as AbortSignal
    const cancel = wrapper.get('section[data-dialog]').findAll('button').find(button => button.text() === 'admin.departments.cancel')!
    await cancel.trigger('click')
    expect(wrapper.find('[data-testid="candidate-query"]').exists()).toBe(false)
    await wrapper.get('[data-testid="open-add-members"]').trigger('click')
    if (result === 'success') resolveOld([candidate])
    else rejectOld(denied('DEPARTMENT_SCOPE_DENIED'))
    await flushPromises()
    expect(oldSignal.aborted).toBe(true)
    expect(wrapper.find('[data-testid="candidate-20"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="add-selected-members"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-testid="candidate-error"]').exists()).toBe(false)
    expect(wrapper.text()).toContain(member.email)
    expect(api.managedScope).toHaveBeenCalledTimes(1)
    expect(api.managedCandidates).toHaveBeenCalledTimes(1)
  })
})
