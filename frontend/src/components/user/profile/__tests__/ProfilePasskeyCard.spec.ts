import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ProfilePasskeyCard from '../ProfilePasskeyCard.vue'

const api = vi.hoisted(() => ({
  isSupported: vi.fn(() => true),
  list: vi.fn(),
  register: vi.fn(),
  rename: vi.fn(),
  remove: vi.fn()
}))
const app = vi.hoisted(() => ({ showSuccess: vi.fn(), showError: vi.fn() }))
vi.mock('@/api', () => ({ passkeyAPI: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => app }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const credentials = Array.from({ length: 7 }, (_, index) => ({
  id: index + 1,
  name: `Device ${index + 1}`,
  created_at: '2026-11-01T00:00:00Z',
  backup: index === 0,
  last_used_at: index === 0 ? '2026-11-15T00:00:00Z' : undefined
}))
function mountCard(enabled = true) {
  return mount(ProfilePasskeyCard, {
    props: { enabled },
    global: {
      stubs: {
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show" class="passkey-dialog"><slot /></div>'
        }
      }
    }
  })
}
const button = (wrapper: ReturnType<typeof mount>, label: string) =>
  wrapper.findAll('button').find((item) => item.text() === label)!
beforeEach(() => {
  vi.resetAllMocks()
  api.isSupported.mockReturnValue(true)
  api.list.mockResolvedValue(structuredClone(credentials))
})

describe('Passkey settings', () => {
  it('does not query or offer operations when the feature is disabled', async () => {
    const wrapper = mountCard(false)
    await flushPromises()
    expect(api.list).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('profile.passkey.featureDisabled')
    expect(wrapper.find('button').exists()).toBe(false)
  })

  it('keeps all credentials reachable by pagination and exposes sync and usage metadata', async () => {
    const wrapper = mountCard()
    await flushPromises()
    await button(wrapper, 'profile.manageShort').trigger('click')
    expect(wrapper.text()).toContain('profile.passkey.synced')
    expect(wrapper.text()).toContain('profile.passkey.lastUsed')
    const seen = new Set<string>()
    while (true) {
      wrapper
        .findAll('[data-testid="passkey-row"] h3')
        .forEach((row) => seen.add(row.text()))
      const next = wrapper.get('[aria-label="pagination.next"]')
      if (next.attributes('disabled') !== undefined) break
      await next.trigger('click')
    }
    expect([...seen]).toEqual(credentials.map((item) => item.name))
  })

  it('allows managing existing credentials on a browser that cannot register new ones', async () => {
    api.isSupported.mockReturnValue(false)
    const wrapper = mountCard()
    await flushPromises()
    await button(wrapper, 'profile.manageShort').trigger('click')
    expect(
      wrapper.findAll('[data-testid="passkey-row"]').length
    ).toBeGreaterThan(0)
    expect(
      wrapper
        .findAll('button')
        .some((item) => item.text() === 'profile.passkey.add')
    ).toBe(false)
  })

  it('renames through the existing endpoint and requires the current password to delete, retaining the dialog on failure', async () => {
    const wrapper = mountCard()
    await flushPromises()
    await button(wrapper, 'profile.manageShort').trigger('click')
    await wrapper.get('[data-testid="passkey-row"] button').trigger('click')
    await wrapper.get('input').setValue('Personal laptop')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.rename).toHaveBeenCalledWith(1, 'Personal laptop')
    await wrapper.get('[data-testid="passkey-row"] button').trigger('click')
    await button(wrapper, 'common.delete').trigger('click')
    expect(
      wrapper.get('button[type="submit"]').attributes('disabled')
    ).toBeDefined()
    api.remove.mockRejectedValueOnce({ message: 'Incorrect password' })
    await wrapper.get('#passkey-delete-password').setValue('wrong')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.remove).toHaveBeenCalledWith(1, 'wrong')
    expect(app.showError).toHaveBeenCalledWith('Incorrect password')
    expect(wrapper.get('#passkey-delete-password').exists()).toBe(true)
    api.remove.mockResolvedValueOnce(undefined)
    await wrapper.get('#passkey-delete-password').setValue('correct')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('#passkey-delete-password').exists()).toBe(false)
    expect(
      wrapper
        .findAll('[data-testid="passkey-row"]')
        .map((row) => row.text())
        .join(' ')
    ).not.toContain('Personal laptop')
  })

  it('registers with the current password and returns to the saved credential list', async () => {
    const wrapper = mountCard()
    await flushPromises()
    await button(wrapper, 'profile.manageShort').trigger('click')
    await button(wrapper, 'profile.passkey.add').trigger('click')
    await wrapper.get('#passkey-name').setValue('New device')
    await wrapper.get('#passkey-add-password').setValue('current-password')
    api.list.mockResolvedValueOnce([
      ...credentials,
      { ...credentials[0], id: 8, name: 'New device' }
    ])
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.register).toHaveBeenCalledWith('New device', 'current-password')
    expect(wrapper.text()).toContain('New device')
    expect(wrapper.find('#passkey-add-password').exists()).toBe(false)
  })

  it('keeps a failed credential load distinct from an empty list and retries', async () => {
    api.list.mockRejectedValueOnce({ message: 'Unavailable' })
    const wrapper = mountCard()
    await flushPromises()
    await button(wrapper, 'profile.manageShort').trigger('click')
    expect(wrapper.get('[role="alert"]').text()).toContain(
      'profile.passkey.loadFailed'
    )
    expect(wrapper.text()).not.toContain('profile.passkey.empty')
    await button(wrapper, 'common.refresh').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(
      wrapper.findAll('[data-testid="passkey-row"]').length
    ).toBeGreaterThan(0)
  })
})
