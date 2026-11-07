import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { AdminGroup } from '@/types'
import GroupsView from '@/views/admin/GroupsView.vue'

const {
  listGroups,
  duplicateGroup,
  updateGroup,
  createGroup,
  getModelAllowlistCandidates,
  getUsageSummary,
  getCapacitySummary,
  getLiveCapability,
  showSuccess,
  showError
} = vi.hoisted(() => ({
  listGroups: vi.fn(),
  duplicateGroup: vi.fn(),
  updateGroup: vi.fn(),
  createGroup: vi.fn(),
  getModelAllowlistCandidates: vi.fn(),
  getUsageSummary: vi.fn(),
  getCapacitySummary: vi.fn(),
  getLiveCapability: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

const authState = vi.hoisted(() => ({ isSimpleMode: false }))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: {
      list: listGroups,
      duplicate: duplicateGroup,
      getModelAllowlistCandidates,
      getUsageSummary,
      getCapacitySummary,
      getLiveCapability,
      getAll: vi.fn(),
      create: createGroup,
      update: updateGroup,
      delete: vi.fn(),
      updateSortOrder: vi.fn()
    },
    accounts: {
      list: vi.fn(),
      getById: vi.fn()
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    isCurrentStep: vi.fn(() => false),
    nextStep: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const sourceGroup: AdminGroup = {
  id: 42,
  name: 'Primary',
  description: null,
  platform: 'openai',
  rate_multiplier: 1,
  rpm_limit: 0,
  is_exclusive: false,
  status: 'active',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  allow_image_generation: false,
  allow_batch_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  batch_image_discount_multiplier: 0.5,
  batch_image_hold_multiplier: 0.6,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  video_rate_independent: false,
  video_rate_multiplier: 1,
  video_price_480p: null,
  video_price_720p: null,
  video_price_1080p: null,
  web_search_price_per_call: null,
  peak_rate_enabled: false,
  peak_start: '',
  peak_end: '',
  peak_rate_multiplier: 1,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  allow_messages_dispatch: false,
  default_mapped_model: '',
  messages_dispatch_model_config: undefined,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '2026-07-16T00:00:00Z',
  updated_at: '2026-07-16T00:00:00Z',
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: true,
  supported_model_scopes: [],
  account_count: 1,
  active_account_count: 1,
  rate_limited_account_count: 0,
  model_allowlist: undefined,
  sort_order: 10
}

const AppLayoutStub = defineComponent({
  template: '<main><slot /></main>'
})

const TablePageLayoutStub = defineComponent({
  template: '<section><slot name="filters" /><slot name="table" /><slot name="pagination" /></section>'
})

const DataTableStub = defineComponent({
  props: {
    data: { type: Array, default: () => [] },
    columns: { type: Array, default: () => [] },
    loading: { type: Boolean, default: false }
  },
  template: '<div><div v-for="row in data" :key="row.id"><slot name="cell-billing_type" :row="row" /><slot name="cell-actions" :row="row" /></div></div>'
})

const BaseDialogStub = defineComponent({
  props: {
    show: { type: Boolean, default: false }
  },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

function mountView() {
  return mount(GroupsView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        Pagination: true,
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        EmptyState: true,
        Select: true,
        PlatformIcon: true,
        Icon: true,
        GroupCapacityBadge: true,
        GroupRateMultipliersModal: true,
        GroupRPMOverridesModal: true,
        VueDraggable: true
      }
    }
  })
}


// Regression tests for docs/features/organization-leader-design-cn.md K24/K25,
// first reproduced by the 2026-11-06 independent review.

const scopesMock = vi.hoisted(() => ({ list: vi.fn(), set: vi.fn() }))
vi.mock('@/api/admin/groupOrganizationScopes', () => ({
  GROUP_ORGANIZATION_KEYS: ['xunyou', 'wsdashi'],
  GROUP_ORGANIZATION_SCOPE_BATCH_LIMIT: 500,
  listGroupOrganizationScopes: scopesMock.list,
  setGroupOrganizationScopes: scopesMock.set,
  sameOrganizationKeys: (a: string[], b: string[]) => [...a].sort().join(',') === [...b].sort().join(',')
}))

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

const subscriptionGroup = { ...sourceGroup, subscription_type: 'subscription' } as AdminGroup
const state = (w: ReturnType<typeof mountView>) => (w.vm as any).$.setupState
const checkbox = (w: ReturnType<typeof mountView>, key: string) =>
  w.get<HTMLInputElement>(`[data-testid="group-organization-${key}"]`).element

describe('GroupsView group organization scopes', () => {
  beforeEach(() => {
    authState.isSimpleMode = false
    localStorage.clear()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    for (const fn of [listGroups, duplicateGroup, updateGroup, createGroup, getModelAllowlistCandidates, getUsageSummary, getCapacitySummary, getLiveCapability, showSuccess, showError]) {
      fn.mockReset()
    }
    listGroups.mockResolvedValue({ items: [subscriptionGroup], total: 1, page: 1, page_size: 20, pages: 1 })
    updateGroup.mockResolvedValue(subscriptionGroup)
    getModelAllowlistCandidates.mockResolvedValue([])
    getUsageSummary.mockResolvedValue([])
    getCapacitySummary.mockResolvedValue([])
    getLiveCapability.mockResolvedValue({ supported: false })
    scopesMock.list.mockReset().mockResolvedValue(new Map([[42, ['xunyou']]]))
    scopesMock.set.mockReset().mockResolvedValue(1)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('edits from a load started after the dialog opened and ignores a late older response', async () => {
    const w = mountView()
    await flushPromises()
    const older = deferred<Map<number, string[]>>()
    const fresh = deferred<Map<number, string[]>>()
    scopesMock.list.mockReturnValueOnce(older.promise).mockReturnValueOnce(fresh.promise)
    void state(w).loadGroupOrganizationScopes()
    await state(w).handleEdit(subscriptionGroup)
    await flushPromises()
    expect(checkbox(w, 'xunyou').disabled, 'the cached scope is not an edit snapshot').toBe(true)
    expect(w.find('[data-testid="group-organization-loading"]').exists()).toBe(true)

    older.resolve(new Map([[42, ['xunyou']]]))
    await flushPromises()
    expect(checkbox(w, 'xunyou').disabled, 'a late older response is ignored').toBe(true)
    fresh.resolve(new Map([[42, ['xunyou', 'wsdashi']]]))
    await flushPromises()
    expect(checkbox(w, 'xunyou').disabled).toBe(false)
    expect(checkbox(w, 'wsdashi').checked).toBe(true)

    await w.get('[data-testid="group-organization-xunyou"]').setValue(false)
    await w.get('#edit-group-form').trigger('submit')
    await flushPromises()
    expect(scopesMock.set).toHaveBeenCalledWith([42], ['wsdashi'])
    w.unmount()
  })

  it('keeps editing disabled after a failed refresh until a retry succeeds', async () => {
    const w = mountView()
    await flushPromises()
    scopesMock.list.mockRejectedValueOnce(new Error('scope API unavailable'))
    await state(w).handleEdit(subscriptionGroup)
    await flushPromises()
    expect(checkbox(w, 'xunyou').disabled).toBe(true)
    expect(w.find('[data-testid="group-organization-load-failed"]').exists()).toBe(true)

    // Saving the group meanwhile leaves the unseen scope untouched.
    await w.get('#edit-group-form').trigger('submit')
    await flushPromises()
    expect(updateGroup).toHaveBeenCalledTimes(1)
    expect(scopesMock.set).not.toHaveBeenCalled()

    scopesMock.list.mockRejectedValueOnce(new Error('scope API unavailable'))
    await state(w).handleEdit(subscriptionGroup)
    await flushPromises()
    expect(w.find('[data-testid="group-organization-load-failed"]').exists()).toBe(true)
    scopesMock.list.mockResolvedValueOnce(new Map([[42, ['wsdashi']]]))
    await w.get('[data-testid="group-organization-retry"]').trigger('click')
    await flushPromises()
    expect(checkbox(w, 'xunyou').disabled).toBe(false)
    expect(checkbox(w, 'wsdashi').checked).toBe(true)
    w.unmount()
  })

  it('reports a partial edit, keeps the dialog and the selection, and retries both steps', async () => {
    const w = mountView()
    await flushPromises()
    await state(w).handleEdit(subscriptionGroup)
    await flushPromises()
    await w.get('[data-testid="group-organization-xunyou"]').setValue(false)
    scopesMock.set.mockRejectedValueOnce({ status: 500, message: 'Internal server error' })
    await w.get('#edit-group-form').trigger('submit')
    await flushPromises()
    expect(scopesMock.set).toHaveBeenCalledWith([42], [])
    expect(showError).toHaveBeenCalledWith('admin.groupOrganizationScope.editScopeFailed')
    expect(showSuccess).not.toHaveBeenCalledWith('admin.groups.groupUpdated')
    expect(w.find('#edit-group-form').exists(), 'the dialog stays open').toBe(true)
    expect(checkbox(w, 'xunyou').checked, 'the selection is kept').toBe(false)

    await w.get('#edit-group-form').trigger('submit')
    await flushPromises()
    expect(updateGroup).toHaveBeenCalledTimes(2)
    expect(scopesMock.set).toHaveBeenNthCalledWith(2, [42], [])
    expect(showSuccess).toHaveBeenCalledWith('admin.groups.groupUpdated')
    expect(w.find('#edit-group-form').exists()).toBe(false)
    w.unmount()
  })

  it('reports a created group whose scope failed without offering a duplicate create', async () => {
    createGroup.mockResolvedValue({ ...subscriptionGroup, id: 50 })
    scopesMock.set.mockRejectedValueOnce({ status: 500, message: 'Internal server error' })
    const w = mountView()
    await flushPromises()
    state(w).openCreateModal()
    await flushPromises()
    state(w).createForm.name = 'New plan'
    state(w).createForm.subscription_type = 'subscription'
    await flushPromises()
    state(w).createOrganizationKeys = ['xunyou']
    await state(w).handleCreateGroup()
    await flushPromises()
    expect(createGroup).toHaveBeenCalledTimes(1)
    expect(scopesMock.set).toHaveBeenCalledWith([50], ['xunyou'])
    expect(showError).toHaveBeenCalledWith('admin.groupOrganizationScope.createScopeFailed')
    expect(showSuccess).not.toHaveBeenCalledWith('admin.groups.groupCreated')
    w.unmount()
  })

  it('loads scopes on mount and refresh, not on every page, and once per scoped save', async () => {
    const w = mountView()
    await flushPromises()
    expect(scopesMock.list).toHaveBeenCalledTimes(1)

    await state(w).handlePageChange(2)
    await flushPromises()
    expect(listGroups).toHaveBeenCalledTimes(2)
    expect(scopesMock.list, 'scopes are global, not per page').toHaveBeenCalledTimes(1)

    state(w).refreshGroups()
    await flushPromises()
    expect(scopesMock.list).toHaveBeenCalledTimes(2)

    await state(w).handleEdit(subscriptionGroup)
    await flushPromises()
    expect(scopesMock.list, 'the edit dialog reads a fresh snapshot').toHaveBeenCalledTimes(3)
    await w.get('[data-testid="group-organization-wsdashi"]').setValue(true)
    await w.get('#edit-group-form').trigger('submit')
    await flushPromises()
    expect(scopesMock.set).toHaveBeenCalledWith([42], ['xunyou', 'wsdashi'])
    expect(scopesMock.list, 'one reload after the scope save').toHaveBeenCalledTimes(4)
    w.unmount()
  })

  it('never shows an unknown scope as unscoped', async () => {
    const first = deferred<Map<number, string[]>>()
    scopesMock.list.mockReturnValueOnce(first.promise)
    const w = mountView()
    await flushPromises()
    expect(w.get('[data-testid="group-organization-badges"]').text()).toBe('admin.groupOrganizationScope.loading')
    first.reject(new Error('scope API unavailable'))
    await flushPromises()
    expect(w.get('[data-testid="group-organization-badges"]').text()).toBe('admin.groupOrganizationScope.unknown')
    scopesMock.list.mockResolvedValueOnce(new Map())
    await state(w).loadGroupOrganizationScopes()
    await flushPromises()
    expect(w.get('[data-testid="group-organization-badges"]').text()).toBe('admin.groupOrganizationScope.unscoped')
    w.unmount()
  })
})
