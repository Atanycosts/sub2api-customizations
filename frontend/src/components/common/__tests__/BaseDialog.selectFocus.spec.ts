import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { h, nextTick } from 'vue'
import BaseDialog from '../BaseDialog.vue'
import Select from '../Select.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
afterEach(() => { document.body.innerHTML = '' })

describe('dialog focus with a portaled Select', () => {
  it('wraps Tab from the final select and lets Escape dismiss only the dropdown', async () => {
    const wrapper = mount(BaseDialog, {
      props: { show: true, title: 'Create key' },
      slots: {
        default: () => h(Select, { modelValue: 'one', options: [{ value: 'one', label: 'One' }] })
      }
    })
    await nextTick()
    const select = wrapper.findComponent(Select)
    const trigger = document.body.querySelector<HTMLButtonElement>('.select-trigger')!
    await select.get('.select-trigger').trigger('keydown', { key: 'ArrowDown' })
    await nextTick()
    const dropdown = document.body.querySelector<HTMLElement>('.select-dropdown-portal')!
    expect(document.activeElement).toBe(dropdown)

    dropdown.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
    expect(document.activeElement).toBe(document.body.querySelector('.modal-header button'))
    await nextTick()
    await select.get('.select-trigger').trigger('keydown', { key: 'ArrowDown' })
    await nextTick()
    document.body.querySelector<HTMLElement>('.select-dropdown-portal')!
      .dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    expect(document.activeElement).toBe(trigger)
    expect(wrapper.emitted('close')).toBeUndefined()
  })
})
