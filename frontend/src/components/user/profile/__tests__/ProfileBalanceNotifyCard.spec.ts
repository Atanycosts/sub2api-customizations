import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import ProfileBalanceNotifyCard from '../ProfileBalanceNotifyCard.vue'
import type { NotifyEmailEntry } from '@/types'

const api = vi.hoisted(() => ({
  sendNotifyEmailCode: vi.fn(),
  verifyNotifyEmail: vi.fn(),
  getProfile: vi.fn(),
  updateProfile: vi.fn(),
  toggleNotifyEmail: vi.fn(),
  removeNotifyEmail: vi.fn()
}))
const showError = vi.hoisted(() => vi.fn())
vi.mock('@/api', () => ({ userAPI: api }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: null }) }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError })
}))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
const button = (wrapper: ReturnType<typeof mount>, text: string) =>
  wrapper.findAll('button').find((item) => item.text() === text)!
const mountCard = (emails: NotifyEmailEntry[] = []) =>
  mount(ProfileBalanceNotifyCard, {
    props: {
      enabled: true,
      threshold: null,
      extraEmails: emails,
      systemDefaultThreshold: 5,
      userEmail: 'alice@example.com'
    },
    global: {
      stubs: {
        BaseDialog: {
          props: ['show'],
          template: '<section v-if="show"><slot /></section>'
        }
      }
    }
  })
beforeEach(() => {
  vi.useFakeTimers()
  vi.resetAllMocks()
  api.sendNotifyEmailCode.mockResolvedValue(undefined)
  api.verifyNotifyEmail.mockResolvedValue(undefined)
  api.getProfile.mockResolvedValue({ balance_notify_extra_emails: [] })
})
afterEach(() => {
  vi.clearAllTimers()
  vi.useRealTimers()
})

describe('notification settings', () => {
  it('shows all three saved emails with independent switches and preserves verified and disabled states', async () => {
    const emails = [
      { email: 'first@example.com', verified: true, disabled: false },
      { email: 'second@example.com', verified: true, disabled: true },
      { email: 'third@example.com', verified: false, disabled: false }
    ]
    const wrapper = mountCard(emails)
    expect(wrapper.findAll('[data-testid="notify-email-row"]')).toHaveLength(3)
    expect(
      wrapper.get('[data-testid="notify-email-add"]').attributes('disabled')
    ).toBeDefined()
    expect(
      wrapper
        .findAll('input[type=checkbox]')
        .map((input) => (input.element as HTMLInputElement).checked)
    ).toEqual([true, true, false, true])
    api.toggleNotifyEmail.mockResolvedValue({
      balance_notify_extra_emails: emails.map((entry, index) => ({
        ...entry,
        disabled: index === 1 ? false : entry.disabled
      }))
    })
    await wrapper.findAll('input[type=checkbox]')[2]!.setValue(true)
    await flushPromises()
    expect(api.toggleNotifyEmail).toHaveBeenCalledWith(
      'second@example.com',
      false
    )
  })

  it('retains the threshold and saved recipients on screen when notifications are off and rolls back failed toggle', async () => {
    api.updateProfile.mockRejectedValueOnce({ message: 'Unable to save' })
    const wrapper = mountCard([
      { email: 'first@example.com', verified: true, disabled: false }
    ])
    await wrapper.get('input[type=checkbox]').setValue(false)
    await flushPromises()
    expect(
      (wrapper.get('input[type=checkbox]').element as HTMLInputElement).checked
    ).toBe(true)
    expect(showError).toHaveBeenCalledWith('Unable to save')
    await wrapper.setProps({ enabled: false })
    expect(
      wrapper.get('#profile-notify-threshold').attributes('disabled')
    ).toBeDefined()
    expect(wrapper.get('[data-testid="notify-email-row"]').text()).toContain(
      'first@example.com'
    )
  })

  it('sends zero to clear the custom threshold back to the system default', async () => {
    api.updateProfile.mockResolvedValue({ balance_notify_threshold: null })
    const wrapper = mountCard()
    await wrapper.get('#profile-notify-threshold').setValue('')
    await wrapper.get('.profile-threshold-form').trigger('submit')
    expect(api.updateProfile).toHaveBeenCalledWith({
      balance_notify_threshold: 0
    })
  })

  it('verifies a new notification email in a dialog before refreshing the saved list', async () => {
    const wrapper = mountCard()
    await wrapper.get('[data-testid="notify-email-add"]').trigger('click')
    expect(
      (wrapper.get('input[type=email]').element as HTMLInputElement).value
    ).toBe('alice@example.com')
    await wrapper.get('input[type=email]').setValue('new@example.com')
    await button(wrapper, 'profile.balanceNotify.sendCode').trigger('click')
    await flushPromises()
    expect(api.sendNotifyEmailCode).toHaveBeenCalledWith('new@example.com')
    expect(vi.getTimerCount()).toBe(1)
    await wrapper.get('input[maxlength="6"]').setValue('123456')
    api.getProfile.mockResolvedValue({
      balance_notify_extra_emails: [
        { email: 'new@example.com', verified: true, disabled: false }
      ]
    })
    await wrapper.get('.profile-dialog-form').trigger('submit')
    await flushPromises()
    expect(api.verifyNotifyEmail).toHaveBeenCalledWith(
      'new@example.com',
      '123456'
    )
    expect(wrapper.get('[data-testid="notify-email-row"]').text()).toContain(
      'new@example.com'
    )
    expect(vi.getTimerCount()).toBe(0)
  })

  it('re-verifies existing unverified emails without consuming another slot', async () => {
    const wrapper = mountCard([
      { email: 'saved@example.com', verified: false, disabled: true }
    ])
    await button(wrapper, 'profile.balanceNotify.verify').trigger('click')
    expect(
      wrapper.get('input[type=email]').attributes('disabled')
    ).toBeDefined()
    await button(wrapper, 'profile.balanceNotify.sendCode').trigger('click')
    await flushPromises()
    await wrapper.get('input[maxlength="6"]').setValue('654321')
    await wrapper.get('.profile-dialog-form').trigger('submit')
    expect(api.verifyNotifyEmail).toHaveBeenCalledWith(
      'saved@example.com',
      '654321'
    )
  })

  it('does not start a cooldown after an email send completes after unmount', async () => {
    let finish!: () => void
    api.sendNotifyEmailCode.mockReturnValue(
      new Promise<void>((resolve) => {
        finish = resolve
      })
    )
    const wrapper = mountCard()
    await wrapper.get('[data-testid="notify-email-add"]').trigger('click')
    await button(wrapper, 'profile.balanceNotify.sendCode').trigger('click')
    wrapper.unmount()
    finish()
    await flushPromises()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('restores an email switch after its update fails', async () => {
    api.toggleNotifyEmail.mockRejectedValueOnce({ message: 'Save failed' })
    const wrapper = mountCard([
      { email: 'first@example.com', verified: true, disabled: false }
    ])
    const input = wrapper.findAll('input[type=checkbox]')[1]!
    await input.setValue(false)
    await flushPromises()
    expect((input.element as HTMLInputElement).checked).toBe(true)
    expect(showError).toHaveBeenCalledWith('Save failed')
  })
})
