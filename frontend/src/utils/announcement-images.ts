export type AnnouncementImageWidth = 25 | 50 | 75 | 100
export type AnnouncementImageAlign = 'left' | 'center' | 'right'

export interface AnnouncementImageMatch {
  start: number
  end: number
  src: string
  alt: string
  width: AnnouncementImageWidth
  align: AnnouncementImageAlign
}

const imageTagPattern = /<img\b[^>]*>/gi

export function isSafeAnnouncementImageUrl(raw: string): boolean {
  const value = raw.trim()
  if (!value || value.startsWith('//') || value.includes('\\')) return false
  if (value.startsWith('/')) return !/\s/.test(value)
  try {
    const parsed = new URL(value)
    return parsed.protocol === 'https:' && !parsed.username && !parsed.password
  } catch {
    return false
  }
}

export function normalizeAnnouncementImageWidth(raw: string | undefined): AnnouncementImageWidth {
  const width = Number(raw)
  return width === 25 || width === 50 || width === 75 ? width : 100
}

export function normalizeAnnouncementImageAlign(raw: string | undefined): AnnouncementImageAlign {
  return raw === 'left' || raw === 'right' ? raw : 'center'
}

export function renderAnnouncementImage(
  src: string,
  width: AnnouncementImageWidth = 100,
  align: AnnouncementImageAlign = 'center',
  alt = ''
): string {
  return `<img src="${escapeAttribute(src)}" alt="${escapeAttribute(alt)}" data-announcement-width="${width}" data-announcement-align="${align}">`
}

export function findAnnouncementImages(content: string): AnnouncementImageMatch[] {
  const images: AnnouncementImageMatch[] = []
  imageTagPattern.lastIndex = 0
  for (const match of content.matchAll(imageTagPattern)) {
    const start = match.index ?? 0
    const attributes = parseAttributes(match[0])
    const src = attributes.get('src')?.trim()
    if (!src) continue
    images.push({
      start,
      end: start + match[0].length,
      src,
      alt: attributes.get('alt') ?? '',
      width: normalizeAnnouncementImageWidth(attributes.get('data-announcement-width')),
      align: normalizeAnnouncementImageAlign(attributes.get('data-announcement-align')),
    })
  }
  return images
}

export function findAnnouncementImageAtCursor(content: string, cursor: number): AnnouncementImageMatch | null {
  const images = findAnnouncementImages(content)
  return images.find(image => cursor >= image.start && cursor <= image.end)
    ?? [...images].reverse().find(image => image.end <= cursor)
    ?? null
}

export function locateAnnouncementImage(
  content: string,
  src: string,
  preferredStart?: number
): AnnouncementImageMatch | null {
  const matches = findAnnouncementImages(content).filter(image => image.src === src)
  if (!matches.length) return null
  if (preferredStart === undefined) return matches[0]
  return matches.reduce((closest, image) =>
    Math.abs(image.start - preferredStart) < Math.abs(closest.start - preferredStart) ? image : closest
  )
}

export function updateAnnouncementImage(
  content: string,
  image: AnnouncementImageMatch,
  updates: { src?: string; width?: AnnouncementImageWidth; align?: AnnouncementImageAlign }
): { content: string; image: AnnouncementImageMatch | null } {
  const current = locateAnnouncementImage(content, image.src, image.start)
  if (!current) return { content, image: null }
  const markup = renderAnnouncementImage(
    updates.src ?? current.src,
    updates.width ?? current.width,
    updates.align ?? current.align,
    current.alt
  )
  return {
    content: content.slice(0, current.start) + markup + content.slice(current.end),
    image: {
      start: current.start,
      end: current.start + markup.length,
      src: updates.src ?? current.src,
      alt: current.alt,
      width: updates.width ?? current.width,
      align: updates.align ?? current.align,
    },
  }
}

export function removeAnnouncementImage(
  content: string,
  image: AnnouncementImageMatch
): { content: string; removed: boolean } {
  const current = locateAnnouncementImage(content, image.src, image.start)
  if (!current) return { content, removed: false }
  return {
    content: content.slice(0, current.start) + content.slice(current.end),
    removed: true,
  }
}

export function insertAtSelection(content: string, start: number, end: number, text: string): string {
  const safeStart = Math.max(0, Math.min(start, content.length))
  const safeEnd = Math.max(safeStart, Math.min(end, content.length))
  const before = content.slice(0, safeStart)
  const after = content.slice(safeEnd)
  const leading = before && !before.endsWith('\n') ? '\n\n' : ''
  const trailing = after && !after.startsWith('\n') ? '\n\n' : ''
  return before + leading + text + trailing + after
}

function parseAttributes(tag: string): Map<string, string> {
  const attributes = new Map<string, string>()
  const parsed = new DOMParser().parseFromString(tag, 'text/html')
  const image = parsed.querySelector('img')
  for (const attribute of Array.from(image?.attributes ?? [])) {
    attributes.set(attribute.name.toLowerCase(), attribute.value)
  }
  return attributes
}

function escapeAttribute(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}
