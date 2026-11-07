import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import DashboardView from '../DashboardView.vue'
import UserDashboardCharts from '@/components/user/dashboard/UserDashboardCharts.vue'
import UserDashboardRecentUsage from '@/components/user/dashboard/UserDashboardRecentUsage.vue'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'

const mocks = vi.hoisted(() => ({
  refreshUser: vi.fn(),
  getDashboardStats: vi.fn(),
  getDashboardTrend: vi.fn(),
  getDashboardModels: vi.fn(),
  getByDateRange: vi.fn(),
  getMyPlatformQuotas: vi.fn(),
}))
vi.mock('@/api/usage', () => ({ usageAPI: mocks }))
vi.mock('@/api/user', () => ({ getMyPlatformQuotas: mocks.getMyPlatformQuotas }))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ user: { balance: 10 }, isSimpleMode: false, refreshUser: mocks.refreshUser }),
}))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

enableAutoUnmount(afterEach)
const mountDashboard = () => mount(DashboardView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      LoadingSpinner: true,
      UserDashboardStats: true,
      UserDashboardCharts: true,
      UserDashboardRecentUsage: true,
      UserDashboardQuickActions: { template: '<div data-testid="quick-actions" />' },
    },
  },
})

beforeEach(() => {
  vi.resetAllMocks()
  mocks.refreshUser.mockResolvedValue(undefined)
  mocks.getDashboardStats.mockResolvedValue({ total_requests: 1 })
  mocks.getDashboardTrend.mockResolvedValue({ trend: [] })
  mocks.getDashboardModels.mockResolvedValue({ models: [] })
  mocks.getByDateRange.mockResolvedValue({ items: [] })
  mocks.getMyPlatformQuotas.mockResolvedValue({ platform_quotas: [] })
})

describe('user dashboard failure states', () => {
  it('keeps independent sections and offers a working retry when the overview fails', async () => {
    mocks.getDashboardStats.mockRejectedValueOnce(new Error('overview unavailable'))
    const wrapper = mountDashboard()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('dashboard.statsLoadFailed')
    expect(wrapper.findComponent(UserDashboardStats).exists()).toBe(false)
    expect(wrapper.findComponent(UserDashboardCharts).exists()).toBe(true)
    expect(wrapper.findComponent(UserDashboardRecentUsage).exists()).toBe(true)
    expect(wrapper.find('[data-testid="quick-actions"]').exists()).toBe(true)

    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.findComponent(UserDashboardStats).props('stats')).toEqual({ total_requests: 1 })
  })

  it('keeps a successful chart when its sibling request fails and distinguishes recent failure from empty data', async () => {
    const trend = [{ date: '2026-11-07', total_tokens: 20 }]
    mocks.getDashboardTrend.mockResolvedValueOnce({ trend })
    mocks.getDashboardModels.mockRejectedValueOnce(new Error('models unavailable'))
    mocks.getByDateRange.mockRejectedValueOnce(new Error('recent unavailable'))
    const wrapper = mountDashboard()
    await flushPromises()
    const charts = wrapper.findComponent(UserDashboardCharts)
    expect(charts.props('trend')).toEqual(trend)
    expect(charts.props('trendError')).toBe(false)
    expect(charts.props('modelError')).toBe(true)
    expect(wrapper.findComponent(UserDashboardRecentUsage).props('error')).toBe(true)

    charts.vm.$emit('retry')
    wrapper.findComponent(UserDashboardRecentUsage).vm.$emit('retry')
    await flushPromises()
    expect(charts.props('modelError')).toBe(false)
    expect(wrapper.findComponent(UserDashboardRecentUsage).props('error')).toBe(false)
  })

  it('ignores an earlier chart response after the selected range has changed', async () => {
    const wrapper = mountDashboard()
    await flushPromises()
    const charts = wrapper.findComponent(UserDashboardCharts)
    let resolveOlder!: (value: unknown) => void
    let resolveNewer!: (value: unknown) => void
    mocks.getDashboardTrend
      .mockImplementationOnce(() => new Promise(resolve => { resolveOlder = resolve }))
      .mockImplementationOnce(() => new Promise(resolve => { resolveNewer = resolve }))
    charts.vm.$emit('update:startDate', '2026-11-01')
    charts.vm.$emit('dateRangeChange')
    charts.vm.$emit('update:startDate', '2026-11-02')
    charts.vm.$emit('dateRangeChange')
    const newer = [{ date: '2026-11-02', total_tokens: 20 }]
    resolveNewer({ trend: newer })
    await flushPromises()
    resolveOlder({ trend: [{ date: '2026-11-01', total_tokens: 10 }] })
    await flushPromises()
    expect(charts.props('trend')).toEqual(newer)
    expect(charts.props('loading')).toBe(false)
    expect(mocks.getDashboardTrend).toHaveBeenLastCalledWith(expect.objectContaining({ start_date: '2026-11-02' }))
  })
})
