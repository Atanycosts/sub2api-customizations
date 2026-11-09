import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AnnouncementsView from '../AnnouncementsView.vue'
import { useAnnouncementStore } from '@/stores/announcements'

const { list, create, update, uploadImage, showError, showSuccess } = vi.hoisted(() => ({
  list: vi.fn(), create: vi.fn(), update: vi.fn(), uploadImage: vi.fn(), showError: vi.fn(), showSuccess: vi.fn(),
}))
vi.mock('@/api/admin', () => ({ adminAPI: { announcements: { list, create, update, uploadImage }, groups: { getAll: vi.fn().mockResolvedValue([]) } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess }) }))
vi.mock('vue-i18n', async importOriginal => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))

enableAutoUnmount(afterEach)
const notice = { id: 3, title: '后台原始标题', content: '后台原始正文', status: 'active', notify_mode: 'popup', targeting: { any_of: [] }, created_at: '2026-11-09T00:00:00Z', updated_at: '2026-11-09T00:00:00Z' }

function installExecCommandHarness() {
  Object.defineProperty(document, 'execCommand', {
    configurable: true,
    value: vi.fn((command: string, _showUI?: boolean, value = '') => {
      const selection = window.getSelection()
      if (!selection) return false
      if (!selection.rangeCount) return false
      const range = selection.getRangeAt(0)
      if (command === 'insertHTML') {
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
      } else if (command === 'insertText') {
        const node = document.createTextNode(value)
        range.deleteContents()
        range.insertNode(node)
        range.setStartAfter(node)
        range.collapse(true)
        selection.removeAllRanges()
        selection.addRange(range)
      } else if (command === 'delete') {
        range.deleteContents()
        range.collapse(true)
        selection.removeAllRanges()
        selection.addRange(range)
      }
      return true
    }),
  })
  Object.defineProperty(document, 'queryCommandState', { configurable: true, value: vi.fn(() => false) })
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  installExecCommandHarness()
  list.mockResolvedValue({ items: [notice], page: 1, page_size: 20, total: 1, pages: 1 })
  create.mockResolvedValue(notice)
  update.mockResolvedValue(notice)
  uploadImage.mockReset()
})
afterEach(() => { document.body.innerHTML = '' })

function mountView() {
  return mount(AnnouncementsView, { attachTo: document.body, global: { stubs: {
    Teleport: true, Transition: true, Icon: true,
    AppLayout: { template: '<div><slot /></div>' },
    TablePageLayout: { template: '<div><slot name="filters"/><slot name="table"/></div>' },
    DataTable: { props: ['data'], template: '<div><slot v-for="row in data" name="cell-actions" :row="row" /></div>' },
    Select: true, AnnouncementTargetingEditor: true, AnnouncementReadStatusDialog: true, ConfirmDialog: true,
  } } })
}

describe('announcement editor and shared preview', () => {
  it('previews the current draft verbatim without saving or marking anything read', async () => {
    const wrapper = mountView()
    await flushPromises()
    const dismiss = vi.spyOn(useAnnouncementStore(), 'dismissPopup')
    const button = (text: string) => wrapper.findAll('button').find(b => b.text() === text)!
    await button('admin.announcements.createAnnouncement').trigger('click')
    await wrapper.get('#announcement-title').setValue('自定义标题 <API>')
    const editor = wrapper.get('[data-testid="announcement-visual-editor"]')
    expect(wrapper.find('#announcement-content').element.tagName).not.toBe('TEXTAREA')
    expect(wrapper.text()).not.toContain('## 原文小标题')
    editor.element.innerHTML = '<h2>原文小标题</h2><p><strong>后台内容</strong></p><script>window.__xss=true</script>'
    await editor.trigger('input')
    expect(wrapper.get('.announcement-draft-preview .announcement-title').text()).toBe('自定义标题 <API>')
    expect(wrapper.get('.announcement-draft-preview .announcement-body h2').text()).toBe('原文小标题')
    expect(wrapper.get('.announcement-draft-preview strong').text()).toBe('后台内容')
    expect(wrapper.find('.announcement-draft-preview script').exists()).toBe(false)
    expect(wrapper.find('.announcement-draft-preview time').exists()).toBe(false)
    await button('admin.announcements.fullPreview').trigger('click')
    expect(wrapper.findAll('.announcement-title').map(title => title.text())).toEqual(['自定义标题 <API>', '自定义标题 <API>'])
    await wrapper.get('[data-testid="announcement-popup-dismiss"]').trigger('click')
    expect(wrapper.findAll('.announcement-title')).toHaveLength(1)
    expect(wrapper.get<HTMLInputElement>('#announcement-title').element.value).toBe('自定义标题 <API>')
    expect(create).not.toHaveBeenCalled()
    expect(update).not.toHaveBeenCalled()
    expect(dismiss).not.toHaveBeenCalled()
    await wrapper.get('#announcement-form').trigger('submit')
    await flushPromises()
    expect(create).toHaveBeenCalledWith(expect.objectContaining({
      title: '自定义标题 <API>', content: '<h2>原文小标题</h2><p><strong>后台内容</strong></p>',
      status: 'draft', notify_mode: 'silent', targeting: { any_of: [] },
    }))
  })

  it('edits the existing backend record without changing its publishing fields', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[title="common.edit"]').trigger('click')
    expect(wrapper.get<HTMLInputElement>('#announcement-title').element.value).toBe(notice.title)
    expect(wrapper.get('.announcement-draft-preview .announcement-title').text()).toBe(notice.title)
    const editor = wrapper.get('[data-testid="announcement-visual-editor"]')
    editor.element.innerHTML = '<p>修改后的正文</p>'
    await editor.trigger('input')
    await wrapper.get('#announcement-form').trigger('submit')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(3, { content: '<p>修改后的正文</p>' })
    expect(create).not.toHaveBeenCalled()
  })

  it('omits untouched legacy content when only metadata changes', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[title="common.edit"]').trigger('click')
    const editor = wrapper.get('[data-testid="announcement-visual-editor"]')
    expect(editor.text()).toContain(notice.content)
    expect(editor.find('textarea').exists()).toBe(false)
    await wrapper.get('#announcement-title').setValue('更新后的标题')
    await wrapper.get('#announcement-form').trigger('submit')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(3, { title: '更新后的标题' })
    expect(update.mock.calls[0]?.[1]).not.toHaveProperty('content')
  })

  it('rejects an emptied visual body but accepts an image-only announcement', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[title="common.edit"]').trigger('click')
    const editor = wrapper.get('[data-testid="announcement-visual-editor"]')
    editor.element.innerHTML = '<p><br></p>'
    await editor.trigger('input')
    await wrapper.get('#announcement-form').trigger('submit')
    await flushPromises()
    expect(update).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.announcements.form.contentRequired')

    editor.element.innerHTML = '<img src="/cover.png" alt="封面">'
    await editor.trigger('input')
    await wrapper.get('#announcement-form').trigger('submit')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(3, { content: '<img src="/cover.png" alt="封面">' })
  })

  it('waits for uploads and submits only once while save is pending', async () => {
    let resolveUpload!: (value: { url: string; width: number; height: number }) => void
    uploadImage.mockImplementation(() => new Promise(resolve => { resolveUpload = resolve }))
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(b => b.text() === 'admin.announcements.createAnnouncement')!.trigger('click')
    await wrapper.get('#announcement-title').setValue('带图片公告')
    const editor = wrapper.get('[data-testid="announcement-visual-editor"]')
    editor.element.innerHTML = '<p>正文</p>'
    await editor.trigger('input')
    await editor.trigger('focus')
    editor.element.focus()
    const range = document.createRange()
    range.selectNodeContents(editor.element)
    range.collapse(false)
    window.getSelection()?.removeAllRanges()
    window.getSelection()?.addRange(range)
    editor.element.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }))
    const input = wrapper.get<HTMLInputElement>('[data-testid="announcement-image-file"]')
    const file = new File(['png'], 'image.png', { type: 'image/png' })
    Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })
    await input.trigger('change')
    expect(wrapper.find('.announcement-upload-card').exists()).toBe(true)

    const form = wrapper.get('#announcement-form')
    void form.trigger('submit')
    void form.trigger('submit')
    await flushPromises()
    expect(create).not.toHaveBeenCalled()

    resolveUpload({ url: '/api/v1/announcements/assets/image.png', width: 120, height: 80 })
    await flushPromises()
    expect(create).toHaveBeenCalledTimes(1)
    expect(create).toHaveBeenCalledWith(expect.objectContaining({
      content: expect.stringContaining('<img src="/api/v1/announcements/assets/image.png"'),
    }))
  })

  it('keeps a failed upload marker until the user discards it', async () => {
    uploadImage.mockRejectedValue(new Error('offline'))
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(b => b.text() === 'admin.announcements.createAnnouncement')!.trigger('click')
    await wrapper.get('#announcement-title').setValue('上传失败公告')
    const editor = wrapper.get('[data-testid="announcement-visual-editor"]')
    editor.element.innerHTML = '<p>保留正文</p>'
    await editor.trigger('input')
    await editor.trigger('focus')
    editor.element.focus()
    const range = document.createRange()
    range.selectNodeContents(editor.element)
    range.collapse(false)
    window.getSelection()?.removeAllRanges()
    window.getSelection()?.addRange(range)
    editor.element.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }))
    const input = wrapper.get<HTMLInputElement>('[data-testid="announcement-image-file"]')
    Object.defineProperty(input.element, 'files', {
      configurable: true,
      value: [new File(['png'], 'broken.png', { type: 'image/png' })],
    })
    await input.trigger('change')
    await flushPromises()

    expect(editor.text()).toContain('保留正文')
    expect(editor.find('.announcement-upload-card').exists()).toBe(true)
    await wrapper.get('[data-testid="announcement-image-discard"]').trigger('click')
    expect(editor.find('.announcement-upload-card').exists()).toBe(false)
    expect(editor.text()).toContain('保留正文')
  })

  it('inserts an upload marker without splitting it when typing continues at the end', async () => {
    let resolveUpload!: (value: { url: string; width: number; height: number }) => void
    uploadImage.mockImplementation(() => new Promise(resolve => { resolveUpload = resolve }))
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(b => b.text() === 'admin.announcements.createAnnouncement')!.trigger('click')
    await wrapper.get('#announcement-title').setValue('继续编辑公告')
    const content = wrapper.get('[data-testid="announcement-visual-editor"]')
    content.element.innerHTML = '<p>已有正文</p>'
    await content.trigger('input')
    await content.trigger('focus')
    content.element.focus()
    const range = document.createRange()
    range.selectNodeContents(content.element)
    range.collapse(false)
    window.getSelection()?.removeAllRanges()
    window.getSelection()?.addRange(range)
    content.element.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }))
    const input = wrapper.get<HTMLInputElement>('[data-testid="announcement-image-file"]')
    Object.defineProperty(input.element, 'files', {
      configurable: true,
      value: [new File(['png'], 'tail.png', { type: 'image/png' })],
    })
    await input.trigger('change')
    await flushPromises()

    expect(content.find('.announcement-upload-card').exists()).toBe(true)
    content.element.append(document.createTextNode('继续输入'))
    await content.trigger('input')
    resolveUpload({ url: '/api/v1/announcements/assets/tail.png', width: 120, height: 80 })
    await flushPromises()
    expect(content.text()).toContain('已有正文')
    expect(content.get('img').attributes('src')).toBe('/api/v1/announcements/assets/tail.png')
    expect(content.text()).toContain('继续输入')
    expect(wrapper.text()).not.toContain('admin.announcements.form.images.uploadingCount')
  })

  it('selects an image from the draft preview for resizing', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[title="common.edit"]').trigger('click')
    const editor = wrapper.get('[data-testid="announcement-visual-editor"]')
    editor.element.innerHTML = '<span class="announcement-image-frame" data-announcement-width="100" data-announcement-align="center"><img src="https://images.example.test/a.png" alt="A"></span>'
    await editor.get('img').trigger('click')
    const range = document.createRange()
    range.selectNode(editor.get('img').element)
    window.getSelection()?.removeAllRanges()
    window.getSelection()?.addRange(range)
    expect(wrapper.find('[data-testid="announcement-image-controls"]').exists()).toBe(true)
    await wrapper.get('[data-testid="announcement-image-controls"] [data-width="50"]').trigger('click')
    expect(editor.html()).toContain('data-announcement-width="50"')
  })
})
