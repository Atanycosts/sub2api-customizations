import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import ProfileView from '@/views/user/ProfileView.vue'

const { fetchPublicSettingsMock, refreshUserMock, authState } = vi.hoisted(
  () => ({
    fetchPublicSettingsMock: vi.fn(),
    refreshUserMock: vi.fn(),
    authState: {
      user: null as Record<string, unknown> | null,
      refreshUser: vi.fn()
    }
  })
)

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    fetchPublicSettings: fetchPublicSettingsMock
  })
}))

vi.mock('@/utils/format', () => ({
  formatDate: () => 'April 2026'
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('ProfileView', () => {
  beforeEach(() => {
    refreshUserMock.mockReset()
    fetchPublicSettingsMock.mockReset()
    refreshUserMock.mockResolvedValue(undefined)
    authState.refreshUser = refreshUserMock
    authState.user = {
      id: 1,
      username: 'alice',
      email: 'alice@example.com',
      role: 'user',
      balance: 10,
      concurrency: 2,
      status: 'active',
      allowed_groups: null,
      balance_notify_enabled: true,
      balance_notify_threshold: null,
      balance_notify_extra_emails: [],
      created_at: '2026-04-20T00:00:00Z',
      updated_at: '2026-04-20T00:00:00Z'
    }
    fetchPublicSettingsMock.mockResolvedValue({
      contact_info: '',
      balance_low_notify_enabled: false,
      balance_low_notify_threshold: 0,
      linuxdo_oauth_enabled: true,
      wechat_oauth_enabled: true,
      wechat_oauth_open_enabled: true,
      wechat_oauth_mp_enabled: false,
      oidc_oauth_enabled: true,
      oidc_oauth_provider_name: 'OIDC'
    })
  })

  it('uses one tab layer and respects globally disabled notifications', async () => {
    const wrapper = mount(ProfileView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          StatCard: { template: '<div class="stat-card" />' },
          ProfileInfoCard: defineComponent({
            setup(_, { expose }) {
              expose({ sourceHints: [] })
              return {}
            },
            template: '<div data-testid="profile-info-card" />'
          }),
          ProfileAvatarCard: true,
          ProfileEditForm: true,
          ProfileIdentityBindingsSection: true,
          ProfilePasskeyCard: true,
          BaseDialog: {
            props: ['show'],
            template: '<div v-if="show"><slot /></div>'
          },
          ProfileBalanceNotifyCard: {
            template: '<div data-testid="profile-balance-notify-card" />'
          },
          ProfilePasswordForm: {
            template: '<div data-testid="profile-password-form" />'
          },
          ProfileTotpCard: {
            template: '<div data-testid="profile-totp-card" />'
          },
          Icon: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.findAll('.stat-card')).toHaveLength(0)
    expect(wrapper.get('[data-testid="profile-shell"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="profile-shell"]').html()).toContain(
      'profile-info-card'
    )
    expect(wrapper.findAll('[role="tablist"]').length).toBe(1)
    expect(wrapper.findAll('[role="tab"]').length).toBe(3)
    await wrapper.get('#profile-tab-security').trigger('click')
    expect(wrapper.get('#profile-panel-security').isVisible()).toBe(true)
    expect(wrapper.get('[data-testid="profile-shell"]').html()).toContain(
      'profile-totp-card'
    )
  })
})
