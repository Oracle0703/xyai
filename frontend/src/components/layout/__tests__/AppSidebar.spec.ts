import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const stylePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css')
const styleSource = readFileSync(stylePath, 'utf8')

describe('AppSidebar custom SVG styles', () => {
  it('does not override uploaded SVG fill or stroke colors', () => {
    expect(componentSource).toContain('.sidebar-svg-icon {')
    expect(componentSource).toContain('color: currentColor;')
    expect(componentSource).toContain('display: block;')
    expect(componentSource).not.toContain('stroke: currentColor;')
    expect(componentSource).not.toContain('fill: none;')
  })
})

describe('AppSidebar scroll position persistence', () => {
  it('binds a template ref to the sidebar nav element', () => {
    expect(componentSource).toContain('ref="sidebarNavRef"')
    expect(componentSource).toContain('sidebar-nav')
  })

  it('declares sidebarNavRef in script setup', () => {
    expect(componentSource).toContain("const sidebarNavRef = ref<HTMLElement | null>(null)")
  })

  it('saves scroll position on beforeUnmount', () => {
    expect(componentSource).toContain('onBeforeUnmount')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('sidebarNavRef.value.scrollTop')
  })

  it('restores scroll position on mount', () => {
    expect(componentSource).toContain('onMounted')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('nextTick')
  })
})

describe('AppSidebar header styles', () => {
  it('does not clip the version badge dropdown', () => {
    const sidebarHeaderBlockMatch = styleSource.match(/\.sidebar-header\s*\{[\s\S]*?\n {2}\}/)
    const sidebarBrandBlockMatch = componentSource.match(/\.sidebar-brand\s*\{[\s\S]*?\n\}/)

    expect(sidebarHeaderBlockMatch).not.toBeNull()
    expect(sidebarBrandBlockMatch).not.toBeNull()
    expect(sidebarHeaderBlockMatch?.[0]).not.toContain('@apply overflow-hidden;')
    expect(sidebarBrandBlockMatch?.[0]).not.toContain('overflow: hidden;')
  })
})

describe('AppSidebar navigation entries', () => {
  it('links to the image generation page from the personal navigation', () => {
    expect(componentSource).toContain("path: '/image-gen'")
    expect(componentSource).toContain("label: t('nav.imageGeneration')")
  })

  it('places the organization report immediately after admin usage with a distinct icon', () => {
    expect(componentSource).toContain(
      "{ path: '/admin/usage', label: t('nav.usage'), icon: ChartIcon },\n    { path: '/admin/organization-usage', label: t('nav.organizationUsage'), icon: BuildingOfficeIcon },"
    )
  })

  it('uses distinct icons for analysis, security, affiliate and payment nav entries', () => {
    expect(componentSource).toContain("{ path: '/admin/ops', label: t('nav.ops'), icon: CpuChipIcon")
    expect(componentSource).toContain("{ path: '/admin/token-analysis', label: t('nav.tokenAnalysis'), icon: CubeTransparentIcon")
    expect(componentSource).toContain("{ path: '/admin/risk-control', label: t('nav.contentModeration'), icon: NoSymbolIcon }")
    expect(componentSource).toContain("{ path: '/admin/prompt-audit', label: t('nav.promptAudit'), icon: DocumentSearchIcon }")
    expect(componentSource).toContain("{ path: '/admin/request-intercept', label: t('nav.requestIntercept'), icon: FunnelIcon")
    expect(componentSource).toContain("{ path: '/admin/audit-logs', label: t('nav.auditLogs'), icon: ClipboardCheckIcon")
    expect(componentSource).toContain("{ path: '/admin/affiliates/invites', label: t('nav.affiliateInviteRecords'), icon: UserPlusIcon }")
    expect(componentSource).toContain("{ path: '/admin/affiliates/transfers', label: t('nav.affiliateTransferRecords'), icon: ArrowUpTrayIcon }")
    expect(componentSource).toContain("{ path: '/admin/orders/dashboard', label: t('nav.paymentDashboard'), icon: BanknotesIcon }")
    expect(componentSource).toContain("{ path: '/admin/orders/plans', label: t('nav.paymentPlans'), icon: RectangleStackIcon }")
    expect(componentSource).toContain("{ path: '/affiliate', label: t('nav.affiliate'), icon: ShareIcon")
  })

  it('renders a permission-filtered management section for sub-admins', () => {
    expect(componentSource).toContain('v-else-if="isSubAdmin"')
    expect(componentSource).toContain("t('nav.adminFeatures')")
    expect(componentSource).toContain('subAdminNavItems')
    expect(componentSource).toContain('authStore.hasAdminPermission')
  })
})
