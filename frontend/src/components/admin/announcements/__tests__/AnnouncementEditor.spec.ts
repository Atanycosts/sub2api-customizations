import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AnnouncementEditor from '../AnnouncementEditor.vue'

const { uploadImage, showError } = vi.hoisted(() => ({ uploadImage: vi.fn(), showError: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { announcements: { uploadImage } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError }) }))
vi.mock('vue-i18n', async importOriginal => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) =>
    params?.name ? `${key}:${String(params.name)}` : params?.count ? `${key}:${String(params.count)}` : key
  }),
}))

type ExecCommand = (
  command: string,
  showUI?: boolean,
  value?: string,
) => boolean

function installExecCommandHarness() {
  const queryCommandState = vi.fn(() => false)
  const execCommand = vi.fn<ExecCommand>((command, _showUI, value = '') => {
    const selection = window.getSelection()
    if (!selection?.rangeCount) return false
    const range = selection.getRangeAt(0)

    switch (command) {
      case 'insertHTML': {
        const fragment = range.createContextualFragment(value)
        const last = fragment.lastChild
        range.deleteContents()
        range.insertNode(fragment)
        if (last) {
          range.setStartAfter(last)
          range.collapse(true)
          selection.removeAllRanges()
          selection.addRange(range)
        }
        return true
      }
      case 'insertText': {
        const node = document.createTextNode(value)
        range.deleteContents()
        range.insertNode(node)
        range.setStartAfter(node)
        range.collapse(true)
        selection.removeAllRanges()
        selection.addRange(range)
        return true
      }
      case 'delete':
        range.deleteContents()
        range.collapse(true)
        selection.removeAllRanges()
        selection.addRange(range)
        return true
      case 'formatBlock': {
        const node = range.commonAncestorContainer instanceof Element
          ? range.commonAncestorContainer
          : range.commonAncestorContainer.parentElement
        const paragraph = node?.closest('p, div')
        if (!paragraph) return false
        const heading = document.createElement(value.toLowerCase())
        heading.append(...Array.from(paragraph.childNodes))
        paragraph.replaceWith(heading)
        range.selectNodeContents(heading)
        selection.removeAllRanges()
        selection.addRange(range)
        return true
      }
      case 'bold': {
        const strong = document.createElement('strong')
        if (range.collapsed) return false
        strong.append(range.extractContents())
        range.insertNode(strong)
        range.selectNodeContents(strong)
        selection.removeAllRanges()
        selection.addRange(range)
        return true
      }
      case 'insertUnorderedList': {
        const node = range.commonAncestorContainer instanceof Element
          ? range.commonAncestorContainer
          : range.commonAncestorContainer.parentElement
        const paragraph = node?.closest('p, div')
        if (!paragraph) return false
        const list = document.createElement('ul')
        const item = document.createElement('li')
        item.append(...Array.from(paragraph.childNodes))
        list.append(item)
        paragraph.replaceWith(list)
        range.selectNodeContents(item)
        selection.removeAllRanges()
        selection.addRange(range)
        return true
      }
      case 'createLink': {
        const anchor = document.createElement('a')
        anchor.href = value
        anchor.append(range.extractContents())
        range.insertNode(anchor)
        range.selectNodeContents(anchor)
        selection.removeAllRanges()
        selection.addRange(range)
        return true
      }
      default:
        return true
    }
  })

  Object.defineProperty(document, 'execCommand', { configurable: true, value: execCommand })
  Object.defineProperty(document, 'queryCommandState', { configurable: true, value: queryCommandState })
  return { execCommand, queryCommandState }
}

enableAutoUnmount(afterEach)
beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  installExecCommandHarness()
})

function mountEditor(value = '', disabled = false) {
  return mount(AnnouncementEditor, {
    attachTo: document.body,
    props: { modelValue: value, session: 1, disabled },
    global: { stubs: { Icon: true } },
  })
}

describe('visual announcement editor', () => {
  it('renders legacy Markdown once as visual content without exposing source syntax', () => {
    const wrapper = mountEditor('## 标题\n\n**正文**\n\n- 项目')
    const editor = wrapper.get('[data-testid="announcement-visual-editor"]')
    expect(editor.attributes('contenteditable')).toBe('true')
    expect(editor.attributes('role')).toBe('textbox')
    expect(editor.attributes('aria-multiline')).toBe('true')
    expect(editor.find('h2').text()).toBe('标题')
    expect(editor.find('strong').text()).toBe('正文')
    expect(editor.find('li').text()).toBe('项目')
    expect(editor.text()).not.toContain('##')
  })

  it('serializes edits but does not rewrite the DOM when the parent returns the same value', async () => {
    const wrapper = mountEditor('<p>正文</p>')
    const editor = wrapper.get<HTMLElement>('[data-testid="announcement-visual-editor"]')
    const paragraph = editor.element.querySelector('p')!
    paragraph.append(document.createTextNode('继续'))
    await editor.trigger('input')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toContain('正文继续')
    const sameNode = editor.element.querySelector('p')
    await wrapper.setProps({ modelValue: wrapper.emitted('update:modelValue')?.at(-1)?.[0] as string })
    expect(editor.element.querySelector('p')).toBe(sameNode)
    expect(editor.text()).toContain('正文继续')
  })

  it('selects an image from a direct click and persists width, alignment, and removal', async () => {
    const wrapper = mountEditor('<img src="https://images.example.test/a.png" data-announcement-width="100" data-announcement-align="center">')
    const image = wrapper.get('img')
    await image.trigger('click')
    expect(wrapper.find('[data-testid="announcement-image-controls"]').exists()).toBe(true)
    await wrapper.get('[data-testid="announcement-image-controls"] [data-width="50"]').trigger('click')
    await wrapper.get('[data-testid="announcement-image-controls"] [data-align="right"]').trigger('click')
    expect(wrapper.get('[data-testid="announcement-visual-editor"]').html()).toContain('data-announcement-width="50"')
    expect(wrapper.get('[data-testid="announcement-visual-editor"]').html()).toContain('data-announcement-align="right"')
    await wrapper.get('[data-testid="announcement-image-remove"]').trigger('click')
    expect(wrapper.find('img').exists()).toBe(false)
  })

  it('shows an uneditable upload card and replaces it in place when upload completes', async () => {
    let resolveUpload!: (value: { url: string; width: number; height: number }) => void
    uploadImage.mockImplementation(() => new Promise(resolve => { resolveUpload = resolve }))
    const wrapper = mountEditor('<p>正文</p>')
    const editor = wrapper.get<HTMLElement>('[data-testid="announcement-visual-editor"]')
    editor.element.focus()
    const range = document.createRange()
    range.selectNodeContents(editor.element)
    range.collapse(false)
    window.getSelection()?.removeAllRanges()
    window.getSelection()?.addRange(range)
    const input = wrapper.get<HTMLInputElement>('[data-testid="announcement-image-file"]')
    Object.defineProperty(input.element, 'files', {
      configurable: true,
      value: [new File(['png'], 'photo.png', { type: 'image/png' })],
    })
    await input.trigger('change')
    expect(wrapper.get('.announcement-upload-card').attributes('contenteditable')).toBe('false')
    resolveUpload({ url: '/api/v1/announcements/assets/photo.png', width: 120, height: 80 })
    await flushPromises()
    expect(wrapper.find('.announcement-upload-card').exists()).toBe(false)
    expect(wrapper.get('img').attributes('src')).toBe('/api/v1/announcements/assets/photo.png')
    expect(wrapper.text()).not.toContain('admin.announcements.form.images.uploadingCount')
  })

  it('does not resurrect a successfully uploaded image after its upload card was deleted', async () => {
    let resolveUpload!: (value: { url: string; width: number; height: number }) => void
    uploadImage.mockImplementation(() => new Promise(resolve => { resolveUpload = resolve }))
    const wrapper = mountEditor('')
    const editor = wrapper.get<HTMLElement>('[data-testid="announcement-visual-editor"]')
    editor.element.focus()
    const range = document.createRange()
    range.selectNodeContents(editor.element)
    range.collapse(false)
    window.getSelection()?.removeAllRanges()
    window.getSelection()?.addRange(range)
    const input = wrapper.get<HTMLInputElement>('[data-testid="announcement-image-file"]')
    Object.defineProperty(input.element, 'files', {
      configurable: true,
      value: [new File(['png'], 'deleted.png', { type: 'image/png' })],
    })
    await input.trigger('change')
    wrapper.get('.announcement-upload-card').element.remove()
    resolveUpload({ url: '/api/v1/announcements/assets/deleted.png', width: 120, height: 80 })
    await flushPromises()
    expect(wrapper.find('img').exists()).toBe(false)
  })

  it('locks the editor and all tools while disabled', () => {
    const wrapper = mountEditor('<p>正文</p>', true)
    expect(wrapper.get('[data-testid="announcement-visual-editor"]').attributes('contenteditable')).toBe('false')
    expect(wrapper.get('[data-testid="announcement-visual-editor"]').attributes('aria-disabled')).toBe('true')
    expect(wrapper.get('[data-testid="announcement-format-bold"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="announcement-image-file"]').attributes('disabled')).toBeDefined()
  })

  it('inserts an image link at the saved range after focus moves to the URL field', async () => {
    const wrapper = mountEditor('<p>正文</p>')
    const editor = wrapper.get<HTMLElement>('[data-testid="announcement-visual-editor"]')
    editor.element.focus()
    const range = document.createRange()
    range.selectNodeContents(editor.element)
    range.collapse(false)
    window.getSelection()?.removeAllRanges()
    window.getSelection()?.addRange(range)
    editor.element.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }))

    await wrapper.get('[data-testid="announcement-image-link-open"]').trigger('click')
    await wrapper.get('[data-testid="announcement-image-link"]').setValue('https://images.example.test/link.png')
    await wrapper.get('[data-testid="announcement-image-link-insert"]').trigger('click')

    expect(editor.find('img').attributes('src')).toBe('https://images.example.test/link.png')
    expect(editor.text()).toContain('正文')
  })

  it('applies the text link attributes to the selected text before an existing link', async () => {
    const wrapper = mountEditor('<p>选中文字</p><p><a href="https://example.test/old">已有链接</a></p>')
    const editor = wrapper.get<HTMLElement>('[data-testid="announcement-visual-editor"]')
    editor.element.focus()
    const range = document.createRange()
    range.selectNodeContents(editor.element.querySelector('p')!)
    window.getSelection()?.removeAllRanges()
    window.getSelection()?.addRange(range)
    await editor.trigger('mouseup')
    await wrapper.get('[data-testid="announcement-format-createLink"]').trigger('click')
    await wrapper.get('[data-testid="announcement-text-link"]').setValue('https://example.test/new')
    await wrapper.get('[data-testid="announcement-text-link-apply"]').trigger('click')

    const link = editor.get('a[href="https://example.test/new"]')
    expect(link.text()).toBe('选中文字')
    expect(link.attributes('target')).toBe('_blank')
    expect(link.attributes('rel')).toBe('noopener noreferrer')
    expect(editor.get('a[href="https://example.test/old"]').attributes('target')).toBeUndefined()
  })
})
