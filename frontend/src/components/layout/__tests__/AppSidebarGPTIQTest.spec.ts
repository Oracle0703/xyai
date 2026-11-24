import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AppSidebar from '../AppSidebar.vue'

const { auth, app, adminSettings, gptQuotaMenu } = vi.hoisted(() => ({
  auth: {
    isAdmin: false, isSubAdmin: true, isSimpleMode: false,
    user: { admin_permissions: [] as string[] },
    hasAdminPermission: vi.fn((_permission: string): boolean => false),
  },
  app: {
    sidebarCollapsed: false, mobileOpen: false, backendModeEnabled: false,
    publicSettingsLoaded: true, siteName: 'Test', siteLogo: '', siteVersion: '',
    sidebarScrollTop: 0, cachedPublicSettings: {}, setMobileOpen: vi.fn(), toggleSidebar: vi.fn(),
  },
  adminSettings: { customMenuItems: [], fetch: vi.fn(), opsMonitoringEnabled: false, paymentEnabled: false },
  gptQuotaMenu: { value: false },
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => auth,
  useAppStore: () => app,
  useAdminSettingsStore: () => adminSettings,
  useOnboardingStore: () => ({ isCurrentStep: () => false, nextStep: vi.fn() }),
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => app }))
vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/dashboard', query: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/common/VersionBadge.vue', () => ({ default: { template: '<span />' } }))
vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({ canUseBatchImage: { value: false }, refreshBatchImageAccess: vi.fn() }),
}))
vi.mock('@/composables/useGPTQuotaVisibility', () => ({
  useGPTQuotaVisibility: () => ({ gptQuotaMenuVisible: gptQuotaMenu, refreshGPTQuotaVisibility: vi.fn() }),
}))

beforeEach(() => {
  auth.isAdmin = false
  auth.isSubAdmin = true
  auth.user.admin_permissions = []
  auth.hasAdminPermission.mockImplementation(permission => auth.user.admin_permissions.includes(permission))
  gptQuotaMenu.value = false
})

function mountSidebar() {
  return mount(AppSidebar, {
    global: {
      stubs: {
        RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
        VersionBadge: true, Icon: true,
      },
    },
  })
}

describe('IQ test config sidebar access', () => {
  it.each([
    ['IQ test sub-admin', false, ['admin.gpt_iq_test'], true],
    ['organization leader', false, ['admin.organization_manage'], true],
    ['unrelated sub-admin', false, ['admin.usage'], false],
    ['full admin', true, [], true],
  ] as const)('shows the config entry for an authorized %s', async (_name, isAdmin, permissions, visible) => {
    auth.isAdmin = isAdmin
    auth.isSubAdmin = !isAdmin
    auth.user.admin_permissions = [...permissions]
    const wrapper = mountSidebar()
    await flushPromises()
    const entry = wrapper.findAll('a').find(link => link.attributes('href') === '/admin/gpt-iq-test' && link.text() === 'nav.gptIQTestConfig')
    expect(!!entry).toBe(visible)
    // 组织负责人与单独授权者都不能看到额度展示管理页。
    if (!isAdmin) expect(wrapper.find('a[href="/admin/gpt-quota"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows the /gpt-quota user entry whenever the shared menu flag is on', async () => {
    auth.isSubAdmin = false
    gptQuotaMenu.value = true
    const wrapper = mountSidebar()
    await flushPromises()
    expect(wrapper.find('a[href="/gpt-quota"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
