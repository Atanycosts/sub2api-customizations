import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import ProfileTotpCard from '../ProfileTotpCard.vue'

const getStatus = vi.hoisted(() => vi.fn())
vi.mock('@/api', () => ({ totpAPI: { getStatus } }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
enableAutoUnmount(afterEach)
beforeEach(() => getStatus.mockReset())

describe('TOTP status', () => {
  it('treats failed status as unknown and retries before offering security actions', async () => {
    getStatus.mockRejectedValueOnce(new Error('status unavailable'))
    getStatus.mockResolvedValueOnce({ feature_enabled: true, enabled: true, enabled_at: null })
    const wrapper = mount(ProfileTotpCard)
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('profile.totp.statusLoadFailed')
    expect(wrapper.text()).not.toContain('profile.totp.notEnabled')
    expect(wrapper.findAll('button').map(button => button.text())).toEqual(['common.refresh'])

    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('profile.totp.enabled')
    expect(wrapper.findAll('button').map(button => button.text())).toEqual(['profile.totp.disable'])
  })

  it('offers enable only after the server confirms it is disabled', async () => {
    getStatus.mockResolvedValue({ feature_enabled: true, enabled: false })
    const wrapper = mount(ProfileTotpCard)
    await flushPromises()
    expect(wrapper.text()).toContain('profile.totp.notEnabled')
    expect(wrapper.get('button').text()).toBe('profile.totp.enable')
  })
})
