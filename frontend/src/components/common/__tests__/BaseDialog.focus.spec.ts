import { afterEach, describe, expect, it } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import BaseDialog from '../BaseDialog.vue'

enableAutoUnmount(afterEach)
afterEach(() => { document.body.innerHTML = '' })

function tab(shiftKey = false) {
  const event = new KeyboardEvent('keydown', { key: 'Tab', shiftKey, cancelable: true })
  document.dispatchEvent(event)
  return event
}

describe('BaseDialog focus', () => {
  it('cycles focus at both edges and restores the trigger when closed', async () => {
    const trigger = document.createElement('button')
    document.body.appendChild(trigger)
    trigger.focus()
    const wrapper = mount(BaseDialog, {
      props: { show: true, title: 'Edit' },
      slots: { default: '<button disabled>Disabled</button><div hidden><input /></div><input id="last-field" />' }
    })
    await nextTick()
    const first = document.body.querySelector<HTMLButtonElement>('.modal-header button')!
    const last = document.getElementById('last-field')!
    expect(document.activeElement).toBe(first)
    expect(tab(true).defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(last)
    tab()
    expect(document.activeElement).toBe(first)
    await wrapper.setProps({ show: false })
    expect(document.activeElement).toBe(trigger)
  })

  it('traps only the topmost dialog and returns to the parent on close', async () => {
    mount(BaseDialog, { props: { show: true, title: 'Parent' } })
    await nextTick()
    const parentButton = document.body.querySelector<HTMLButtonElement>('.modal-header button')!
    const child = mount(BaseDialog, {
      props: { show: true, title: 'Child' },
      slots: { default: '<button id="child-last">Save</button>' }
    })
    await nextTick()
    tab(true)
    expect(document.activeElement?.id).toBe('child-last')
    await child.setProps({ show: false })
    expect(document.activeElement).toBe(parentButton)
  })

  it('focuses the panel when it has no enabled controls', async () => {
    mount(BaseDialog, {
      props: { show: true, title: 'Status', showCloseButton: false },
      slots: { default: '<button disabled>Wait</button>' }
    })
    await nextTick()
    expect(document.activeElement?.classList.contains('modal-content')).toBe(true)
    expect(tab().defaultPrevented).toBe(true)
    expect(document.activeElement?.classList.contains('modal-content')).toBe(true)
  })
})
