import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AppSidebar from '../AppSidebar.vue'

const { auth, app, adminSettings } = vi.hoisted(() => ({
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
  adminSettings: { customMenuItems: [], fetch: vi.fn() },
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
  useGPTQuotaVisibility: () => ({ gptQuotaMenuVisible: { value: false }, refreshGPTQuotaVisibility: vi.fn() }),
}))

beforeEach(() => {
  auth.isAdmin = false
  auth.isSubAdmin = true
  auth.user.admin_permissions = []
  auth.hasAdminPermission.mockImplementation(permission => auth.user.admin_permissions.includes(permission))
})

describe('department member sidebar access', () => {
  it.each([
    ['member manager', false, ['admin.department_members'], true],
    ['report manager', false, ['admin.organization_usage'], false],
    ['full admin', true, [], false],
  ] as const)('shows the dedicated entry only to an authorized %s', async (_name, isAdmin, permissions, visible) => {
    auth.isAdmin = isAdmin
    auth.isSubAdmin = !isAdmin
    auth.user.admin_permissions = [...permissions]
    const wrapper = mount(AppSidebar, {
      global: {
        stubs: {
          RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
          VersionBadge: true, Icon: true,
        },
      },
    })
    await flushPromises()
    const links = wrapper.findAll('a').filter(link => link.attributes('href') === '/admin/my-department')
    // The brand may also point at the default landing page; the menu uses this label.
    const entry = links.find(link => link.text() === 'admin.departments.myDepartmentTitle')
    expect(!!entry).toBe(visible)
    if (!isAdmin) expect(wrapper.find('a[href="/admin/users"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
