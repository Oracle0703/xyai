import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

import AccountsView from '../AccountsView.vue'
import AccountActionMenu from '@/components/admin/account/AccountActionMenu.vue'

vi.mock('@/composables/useAccountOperatorAccess', async () => {
  const { ref } = await import('vue')
  return { useAccountOperatorAccess: () => ({ canManageAccounts: ref(false) }) }
})

const {
  listAccounts,
  getById,
  getBatchTodayStats,
  getUpstreamBillingProbeSettings,
  getFormOptions,
  getAllProxies,
  getAllGroups,
  setSchedulable
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  getById: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getUpstreamBillingProbeSettings: vi.fn(),
  getFormOptions: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  setSchedulable: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag: vi.fn(),
      getById,
      getBatchTodayStats,
      getUpstreamBillingProbeSettings,
      getFormOptions,
      setSchedulable
    },
    proxies: { getAll: getAllProxies },
    groups: { getAll: getAllGroups }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showWarning: vi.fn(), showSuccess: vi.fn(), showInfo: vi.fn() })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ token: 'test-token', isSimpleMode: false })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const DataTableStub = defineComponent({
  props: {
    data: { type: Array, default: () => [] },
    columns: { type: Array, default: () => [] }
  },
  template: `
    <div>
      <span data-test="column-keys">{{ columns.map(column => column.key).join(",") }}</span>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-groups" :row="row" />
        <slot name="cell-schedulable" :row="row" />
        <slot name="cell-priority" :row="row" />
        <slot name="cell-actions" :row="row" />
      </div>
    </div>
  `
})

const AccountGroupsCellStub = defineComponent({
  props: { groups: { type: Array, default: () => [] } },
  template: '<span data-test="account-groups">{{ groups.map(group => group.name).join(",") }}</span>'
})

const AccountTestModalStub = defineComponent({
  props: { show: Boolean, account: { type: Object, default: null } },
  template: '<div data-test="test-account">{{ show ? account?.name : "" }}</div>'
})

function mountView() {
  return mount(AccountsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
        DataTable: DataTableStub,
        AccountTableActions: { template: '<div><slot name="after" /></div>' },
        AccountTableFilters: true,
        AccountBulkActionsBar: true,
        Pagination: true,
        ConfirmDialog: true,
        AccountActionMenu: true,
        ImportDataModal: true,
        ReAuthAccountModal: true,
        AccountTestModal: AccountTestModalStub,
        AccountStatsModal: true,
        ScheduledTestsPanel: true,
        SyncFromCrsModal: true,
        TempUnschedStatusModal: true,
        ErrorPassthroughRulesModal: true,
        TLSFingerprintProfilesModal: true,
        CreateAccountModal: true,
        EditAccountModal: true,
        BulkEditAccountModal: true,
        TotpStepUpDialog: true,
        PlatformTypeBadge: true,
        AccountCapacityCell: true,
        AccountStatusIndicator: true,
        AccountTodayStatsCell: true,
        AccountGroupsCell: AccountGroupsCellStub,
        AccountPriorityCell: true,
        AccountUsageCell: true,
        UpstreamBillingRateCell: true,
        HelpTooltip: true,
        Icon: true,
        Teleport: true
      }
    }
  })
}

const listRow = {
  id: 42,
  name: 'operator row',
  platform: 'openai',
  type: 'oauth',
  status: 'active',
  schedulable: true,
  concurrency: 2,
  priority: 7,
  group_ids: [7],
  extra: {},
  credentials: {}
}

describe('admin AccountsView — restricted account operator', () => {
  beforeEach(() => {
    localStorage.clear()
    listAccounts.mockReset().mockResolvedValue({ items: [listRow], total: 1, page: 1, page_size: 20, pages: 1 })
    getById.mockReset().mockResolvedValue({ ...listRow, credentials: {} })
    getBatchTodayStats.mockReset().mockResolvedValue({ stats: {} })
    getUpstreamBillingProbeSettings.mockReset().mockResolvedValue({ enabled: true })
    getFormOptions.mockReset().mockResolvedValue({ groups: [{ id: 7, name: 'codex', platform: 'openai' }], proxies: [] })
    getAllProxies.mockReset()
    getAllGroups.mockReset()
    setSchedulable.mockReset()
  })

  it('loads compact form options instead of full admin catalogs and settings', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(getFormOptions).toHaveBeenCalledTimes(1)
    expect(getAllProxies).not.toHaveBeenCalled()
    expect(getAllGroups).not.toHaveBeenCalled()
    expect(getUpstreamBillingProbeSettings).not.toHaveBeenCalled()
    expect(wrapper.get('[data-test="account-groups"]').text()).toBe('codex')
    wrapper.unmount()
  })

  it('hides write actions and admin-only modals', async () => {
    const wrapper = mountView()
    await flushPromises()

    const buttonTexts = wrapper.findAll('button').map(button => button.text())
    expect(buttonTexts.some(text => text.includes('common.edit'))).toBe(false)
    expect(buttonTexts.some(text => text.includes('common.delete'))).toBe(false)
    expect(buttonTexts.some(text => text.includes('common.more'))).toBe(true)
    expect(buttonTexts.some(text => text.includes('admin.accounts.syncFromCrs'))).toBe(false)
    expect(wrapper.get('[data-test="column-keys"]').text().split(',')).not.toContain('select')
    expect(wrapper.findComponent({ name: 'CreateAccountModal' }).exists()).toBe(true)
    expect(wrapper.findComponent({ name: 'ReAuthAccountModal' }).exists()).toBe(true)
    for (const name of ['AccountBulkActionsBar', 'EditAccountModal', 'BulkEditAccountModal', 'SyncFromCrsModal', 'ImportDataModal', 'ScheduledTestsPanel']) {
      expect(wrapper.findComponent({ name }).exists()).toBe(false)
    }
    expect(wrapper.find('[data-testid="account-priority-cell"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('7')
    wrapper.unmount()
  })

  it('keeps the schedulable toggle read-only', async () => {
    const wrapper = mountView()
    await flushPromises()

    const toggle = wrapper.find('button[title="admin.accounts.schedulableEnabled"]')
    expect(toggle.attributes('disabled')).toBeDefined()
    await toggle.trigger('click')
    expect(setSchedulable).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('still opens the test modal from the action menu', async () => {
    const wrapper = mountView()
    await flushPromises()

    wrapper.findComponent(AccountActionMenu).vm.$emit('test', listRow)
    await flushPromises()
    expect(getById).toHaveBeenCalledWith(42)
    expect(wrapper.get('[data-test="test-account"]').text()).toBe('operator row')
    wrapper.unmount()
  })
})
