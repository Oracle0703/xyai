import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import OrganizationUsageSummary from '../OrganizationUsageSummary.vue'

vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))

describe('organization summary scope', () => {
  it('renders only explicitly returned organizations, including authorized zero-usage rows', () => {
    const wrapper = mount(OrganizationUsageSummary, { props: { selectedOrganization: 'xunyou', organizations: [{ organization: 'xunyou', active_users: 2, used_users: 0, requests: 0, input_tokens: 0, output_tokens: 0, cache_creation_tokens: 0, cache_read_tokens: 0, total_tokens: 0, actual_cost: 0 }] } })
    expect(wrapper.findAll('tbody tr')).toHaveLength(1)
    expect(wrapper.get('[data-organization="xunyou"]').text()).toBe('迅游')
    expect(wrapper.find('[data-organization="wsdashi"]').exists()).toBe(false)
    expect(wrapper.find('[data-organization="other"]').exists()).toBe(false)
  })
  it('does not invent an organization when the server returns no rows', () => {
    const wrapper = mount(OrganizationUsageSummary, { props: { selectedOrganization: 'all', organizations: [] } })
    expect(wrapper.findAll('tbody tr')).toHaveLength(0)
  })
})
