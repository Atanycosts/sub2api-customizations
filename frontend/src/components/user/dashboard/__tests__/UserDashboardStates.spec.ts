import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import UserDashboardCharts from '../UserDashboardCharts.vue'
import UserDashboardRecentUsage from '../UserDashboardRecentUsage.vue'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('vue-chartjs', () => ({ Doughnut: { template: '<div />' }, Line: { template: '<div />' } }))
enableAutoUnmount(afterEach)

describe('dashboard section states', () => {
  it('renders independent chart errors with retry, then normal empty states after recovery', async () => {
    const wrapper = mount(UserDashboardCharts, {
      props: {
        loading: false, startDate: '2026-11-01', endDate: '2026-11-07', granularity: 'day',
        trend: [], models: [], modelError: true, trendError: true,
      },
      global: { stubs: { DateRangePicker: true, Select: true, LoadingSpinner: true } },
    })
    expect(wrapper.findAll('[role="alert"]')).toHaveLength(2)
    expect(wrapper.text()).not.toContain('dashboard.noDataAvailable')
    expect(wrapper.findComponent(TokenUsageTrend).exists()).toBe(false)
    await wrapper.get('[role="alert"] button').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)

    await wrapper.setProps({ modelError: false, trendError: false })
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('dashboard.noDataAvailable')
    expect(wrapper.findComponent(TokenUsageTrend).exists()).toBe(true)
  })

  it('distinguishes recent usage failure from a successful empty list', async () => {
    const wrapper = mount(UserDashboardRecentUsage, {
      props: { data: [], loading: false, error: true },
      global: { stubs: { Icon: true, RouterLink: true } },
    })
    expect(wrapper.get('[role="alert"]').text()).toContain('dashboard.recentLoadFailed')
    expect(wrapper.text()).not.toContain('dashboard.noUsageRecords')
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
    await wrapper.setProps({ error: false })
    expect(wrapper.text()).toContain('dashboard.noUsageRecords')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })
})
