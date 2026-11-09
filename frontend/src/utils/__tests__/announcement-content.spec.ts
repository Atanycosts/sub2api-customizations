import { describe, expect, it } from 'vitest'
import {
  renderAnnouncementContentHtml,
  sanitizeAnnouncementEditorHtml,
} from '../announcement-content'

describe('announcement visual content', () => {
  it('keeps a selected image while removing only upload cards and selection state', () => {
    const html = sanitizeAnnouncementEditorHtml([
      '<p>正文</p>',
      '<span class="announcement-upload-card" contenteditable="false">上传中</span>',
      '<span class="announcement-image-frame is-selected" data-announcement-width="50">',
      '<img src="https://images.example.test/kept.png">',
      '</span>',
    ].join(''))
    expect(html).toContain('正文')
    expect(html).toContain('https://images.example.test/kept.png')
    expect(html).toContain('data-announcement-width="50"')
    expect(html).not.toContain('announcement-upload-card')
    expect(html).not.toContain('is-selected')
  })

  it('renders Markdown as visible HTML and sanitizes pasted scripts and event handlers', () => {
    const html = renderAnnouncementContentHtml('## 标题\n\n**正文**', {
      editable: true,
      imageOpenLabel: '打开原图',
    })
    expect(html).toContain('<h2>标题</h2>')
    expect(html).toContain('<strong>正文</strong>')
    expect(sanitizeAnnouncementEditorHtml('<p onclick="alert(1)">正文</p><script>alert(1)</script>')).toBe('<p>正文</p>')
  })

  it('preserves an existing image link and layout through visual editing and reading', () => {
    const source = '<a href="https://example.test/image-target" data-announcement-width="50"><img src="/cover.png" alt="封面"></a>'
    const editable = renderAnnouncementContentHtml(source, { editable: true, imageOpenLabel: '打开原图' })
    const saved = sanitizeAnnouncementEditorHtml(editable + '<p>新增正文</p>')
    const rendered = renderAnnouncementContentHtml(saved, { editable: false, imageOpenLabel: '打开原图' })
    const doc = new DOMParser().parseFromString(rendered, 'text/html')
    const link = doc.querySelector('a')!
    expect(link.getAttribute('href')).toBe('https://example.test/image-target')
    expect(link.getAttribute('data-announcement-width')).toBe('50')
    expect(link.querySelector('img')?.getAttribute('src')).toBe('/cover.png')
    expect(doc.body.textContent).toContain('新增正文')
  })
})
