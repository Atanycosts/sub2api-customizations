import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import AnnouncementContent from '../AnnouncementContent.vue'

const styles = readFileSync(resolve(process.cwd(), 'src/styles/announcement-content.css'), 'utf8')

describe('backend-authored announcement content', () => {
  it('keeps the exact title and renders only the provided body without invented metadata', async () => {
    const wrapper = mount(AnnouncementContent, {
      props: {
        announcement: { title: '<标题>\nAPI · 配置原文', content: '**后台正文**\n\n[文档](https://example.com)' },
        imageOpenLabel: '打开原图',
      },
    })
    expect(wrapper.get('.announcement-title').text()).toBe('<标题>\nAPI · 配置原文')
    expect(wrapper.find('.announcement-title span').exists()).toBe(false)
    expect(wrapper.get('strong').text()).toBe('后台正文')
    expect(wrapper.find('time').exists()).toBe(false)
    expect(wrapper.find('.article-tag').exists()).toBe(false)
    await wrapper.setProps({ announcement: { title: '新标题', content: '新正文' } })
    expect(wrapper.get('.announcement-title').text()).toBe('新标题')
    expect(wrapper.get('.announcement-body').text()).toBe('新正文')
  })

  it('preserves Markdown tables and HTML while removing scripts and unsafe links', () => {
    const wrapper = mount(AnnouncementContent, {
      props: {
        announcement: { title: '公告', content: '| 列 |\n| --- |\n| 内容 |\n\n<div>HTML 原文</div><script>window.__xss = true</script><a href="javascript:alert(1)">链接</a><img src=x onerror="alert(1)">' },
        imageOpenLabel: '打开原图',
      },
    })
    expect(wrapper.get('td').text()).toBe('内容')
    expect(wrapper.text()).toContain('HTML 原文')
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.get('a').attributes('href')).toBeUndefined()
    expect(wrapper.get('img').attributes('onerror')).toBeUndefined()
  })

  it('moves image sizing to one wrapper and opens the original image', () => {
    const wrapper = mount(AnnouncementContent, {
      props: {
        announcement: {
          title: '公告',
          content: '<img src="https://images.example.test/a?x=1&amp;y=2" alt="宽图 &amp; 高图" data-announcement-width="50" data-announcement-align="right">',
        },
        imageOpenLabel: '打开原图',
      },
    })
    const link = wrapper.get('.announcement-image-link')
    const image = wrapper.get('img')
    expect(link.attributes('data-announcement-width')).toBe('50')
    expect(link.attributes('data-announcement-align')).toBe('right')
    expect(link.attributes('href')).toBe('https://images.example.test/a?x=1&y=2')
    expect(image.attributes('data-announcement-width')).toBeUndefined()
    expect(image.attributes('data-announcement-align')).toBeUndefined()
    expect(styles).toContain(':is(.announcement-image-link, .announcement-image-frame)[data-announcement-width="50"] { width: 50%; }')
    expect(styles).toContain(':is(.announcement-image-link, .announcement-image-frame) img { width: 100%; margin: 0; }')
    expect(styles).not.toContain('img[data-announcement-width="50"]')
  })

  it('uses the localized label when an image has no alt text', () => {
    const wrapper = mount(AnnouncementContent, {
      props: {
        announcement: { title: '公告', content: '<img src="https://images.example.test/no-alt.png">' },
        imageOpenLabel: '打开原图',
      },
    })
    expect(wrapper.get('.announcement-image-link').attributes('aria-label')).toBe('打开原图')
  })

  it('adds layout to existing image links and does not make unsafe image URLs navigable', () => {
    const wrapper = mount(AnnouncementContent, {
      props: {
        announcement: {
          title: '公告',
          content: [
            '<a href="https://example.test"><img src="https://images.example.test/linked.png" data-announcement-width="25" data-announcement-align="left"></a>',
            '<img src="data:image/png;base64,AAAA" data-announcement-width="75">',
          ].join(''),
        },
        imageOpenLabel: '打开原图',
      },
    })
    const linked = wrapper.findAll('a')
    expect(linked).toHaveLength(1)
    expect(linked[0].classes()).toContain('announcement-image-link')
    expect(linked[0].attributes('data-announcement-width')).toBe('25')
    const unsafe = wrapper.get('.announcement-image-frame')
    expect(unsafe.element.tagName).toBe('SPAN')
    expect(unsafe.attributes('data-announcement-width')).toBe('75')
  })

  it('emits the selected image in editor mode without opening it', async () => {
    const wrapper = mount(AnnouncementContent, {
      props: {
        announcement: {
          title: '公告',
          content: '<img src="https://images.example.test/editable.png" data-announcement-width="50">',
        },
        imageOpenLabel: '打开原图',
        editable: true,
      },
    })
    expect(wrapper.find('a').exists()).toBe(false)
    await wrapper.get('img').trigger('click')
    expect(wrapper.emitted('selectImage')).toEqual([['https://images.example.test/editable.png']])
  })

  it('keeps ordinary text links outside the image layout', () => {
    const wrapper = mount(AnnouncementContent, {
      props: {
        announcement: { title: '公告', content: '[文档](https://example.test/docs)' },
        imageOpenLabel: '打开原图',
      },
    })
    const link = wrapper.get('a')
    expect(link.text()).toBe('文档')
    expect(link.classes()).not.toContain('announcement-image-link')
  })

  it('wraps an existing editor image frame as a safe reader link', () => {
    const wrapper = mount(AnnouncementContent, {
      props: {
        announcement: {
          title: '公告',
          content: '<span class="announcement-image-frame" data-announcement-width="50" data-announcement-align="right"><img src="https://images.example.test/frame.png" alt="Frame"></span>',
        },
        imageOpenLabel: '打开原图',
      },
    })
    const link = wrapper.get('.announcement-image-link')
    expect(link.attributes('href')).toBe('https://images.example.test/frame.png')
    expect(link.attributes('data-announcement-width')).toBe('50')
    expect(link.attributes('data-announcement-align')).toBe('right')
    expect(wrapper.find('.announcement-image-frame').exists()).toBe(false)
  })
})
