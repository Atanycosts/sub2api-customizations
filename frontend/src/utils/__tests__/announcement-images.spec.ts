import { describe, expect, it } from 'vitest'
import {
  findAnnouncementImages,
  isSafeAnnouncementImageUrl,
  renderAnnouncementImage,
  updateAnnouncementImage,
} from '../announcement-images'

describe('announcement image editing', () => {
  it.each([
    ['/uploads/image.png?x=1&y=2', true],
    ['https://images.example.test/a.png?x=1&y=2', true],
    ['http://images.example.test/a.png', false],
    ['//images.example.test/a.png', false],
    ['data:image/png;base64,AAAA', false],
    ['blob:https://example.test/id', false],
    ['/images/a b.png', false],
    ['/images\\a.png', false],
    ['https://user:pass@images.example.test/a.png', false],
  ])('validates image URL %s', (url, expected) => {
    expect(isSafeAnnouncementImageUrl(url)).toBe(expected)
  })

  it('keeps ampersands and quoted alt text stable through repeated edits', () => {
    const original = renderAnnouncementImage(
      'https://images.example.test/a.png?x=1&y=2',
      100,
      'center',
      'Tom & Jerry "quote" <tag>'
    )
    expect(findAnnouncementImages(original)[0]).toMatchObject({
      src: 'https://images.example.test/a.png?x=1&y=2',
      alt: 'Tom & Jerry "quote" <tag>',
    })

    let content = original
    for (const width of [75, 50, 25, 100] as const) {
      const image = findAnnouncementImages(content)[0]
      const updated = updateAnnouncementImage(content, image, { width })
      content = updated.content
      expect(findAnnouncementImages(content)[0]).toMatchObject({
        src: 'https://images.example.test/a.png?x=1&y=2',
        alt: 'Tom & Jerry "quote" <tag>',
        width,
      })
      expect(content).not.toContain('&amp;amp;')
      expect(content).not.toContain('&amp;quot;')
    }

    for (const align of ['left', 'right', 'center'] as const) {
      const image = findAnnouncementImages(content)[0]
      content = updateAnnouncementImage(content, image, { align }).content
      expect(findAnnouncementImages(content)[0]).toMatchObject({
        src: 'https://images.example.test/a.png?x=1&y=2',
        alt: 'Tom & Jerry "quote" <tag>',
        align,
      })
    }
    expect(content).toContain('src="https://images.example.test/a.png?x=1&amp;y=2"')
    expect(content).toContain('alt="Tom &amp; Jerry &quot;quote&quot; &lt;tag&gt;"')
  })
})
