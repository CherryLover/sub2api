import { describe, expect, it, vi } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import EndpointDistributionChart from '../EndpointDistributionChart.vue'
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/dashboard', () => ({ getUserBreakdown: vi.fn() }))
const stats = [{ endpoint: '/v1/messages', requests: 3, total_tokens: 10, cost: 6, actual_cost: 2.4, account_cost: 0.9 }]
describe('endpoint cost dimensions', () => {
  it('keeps actual charge and account cost separate for administrators', () => {
    const wrapper = shallowMount(EndpointDistributionChart, { props: { endpointStats: stats, enableBreakdown: false, showAccountCost: true } })
    expect(wrapper.text()).toContain('$2.40')
    expect(wrapper.text()).toContain('$0.900')
    expect(wrapper.text()).toContain('admin.dashboard.accountCost')
    wrapper.unmount()
  })
  it('does not render account cost on the user surface', () => {
    const wrapper = shallowMount(EndpointDistributionChart, { props: { endpointStats: stats, enableBreakdown: false, showAccountCost: false } })
    expect(wrapper.text()).toContain('$2.40')
    expect(wrapper.text()).not.toContain('$0.900')
    expect(wrapper.text()).not.toContain('admin.dashboard.accountCost')
    wrapper.unmount()
  })
})
