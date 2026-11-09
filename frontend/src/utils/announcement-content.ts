import DOMPurify from 'dompurify'
import { marked } from 'marked'
import { isSafeAnnouncementImageUrl } from './announcement-images'

const allowedTags = [
  'a', 'b', 'blockquote', 'br', 'code', 'div', 'em', 'h1', 'h2', 'h3', 'h4',
  'hr', 'i', 'img', 'li', 'ol', 'p', 'pre', 's', 'span', 'strong', 'table',
  'tbody', 'td', 'th', 'thead', 'tr', 'u', 'ul',
]

const sanitizeOptions = {
  ALLOWED_TAGS: allowedTags,
  ALLOWED_ATTR: [
    'alt', 'class', 'colspan', 'data-announcement-align', 'data-announcement-width',
    'href', 'rel', 'rowspan', 'src', 'target', 'title', 'type',
  ],
}

export function sanitizeAnnouncementHtml(html: string): string {
  return DOMPurify.sanitize(html, sanitizeOptions)
}

export function renderAnnouncementContentHtml(
  content: string,
  options: { editable: boolean; imageOpenLabel: string }
): string {
  const html = marked.parse(content, { breaks: true, gfm: true }) as string
  const parsed = new DOMParser().parseFromString(sanitizeAnnouncementHtml(html), 'text/html')
  normalizeImages(parsed, options)
  return parsed.body.innerHTML
}

export function sanitizeAnnouncementEditorHtml(html: string): string {
  const parsed = new DOMParser().parseFromString(sanitizeAnnouncementHtml(html), 'text/html')
  parsed.querySelectorAll('[contenteditable]').forEach(element => element.removeAttribute('contenteditable'))
  parsed.querySelectorAll('.announcement-upload-card').forEach(element => element.remove())
  parsed.querySelectorAll('.is-selected').forEach(element => element.classList.remove('is-selected'))
  parsed.querySelectorAll('[class=""]').forEach(element => element.removeAttribute('class'))
  return parsed.body.innerHTML
}

export function normalizeEditorImages(container: ParentNode, imageOpenLabel: string): void {
  normalizeImages(container, { editable: true, imageOpenLabel })
}

export function preparePastedHtml(html: string, imageOpenLabel: string): string {
  const parsed = new DOMParser().parseFromString(sanitizeAnnouncementHtml(html), 'text/html')
  normalizeImages(parsed, { editable: true, imageOpenLabel })
  return parsed.body.innerHTML
}

function normalizeImages(
  container: ParentNode,
  options: { editable: boolean; imageOpenLabel: string }
): void {
  for (const image of Array.from(container.querySelectorAll<HTMLImageElement>('img'))) {
    const width = Number(image.dataset.announcementWidth)
    if ([25, 50, 75, 100].includes(width)) {
      image.dataset.announcementWidth = String(Math.round(width))
    } else {
      delete image.dataset.announcementWidth
    }
    if (!['left', 'center', 'right'].includes(image.dataset.announcementAlign ?? '')) {
      delete image.dataset.announcementAlign
    }
    image.removeAttribute('width')
    image.removeAttribute('height')
    image.removeAttribute('style')
    if (!image.alt) image.alt = ''

    const existingLink = image.parentElement?.tagName === 'A'
      ? image.parentElement as HTMLAnchorElement
      : null
    if (existingLink) {
      existingLink.classList.toggle('announcement-image-frame', options.editable)
      existingLink.classList.toggle('announcement-image-link', !options.editable)
      moveImageLayoutToContainer(image, existingLink)
      continue
    }

    const existingFrame = image.parentElement?.classList.contains('announcement-image-frame')
      ? image.parentElement as HTMLElement
      : null
    if (existingFrame) {
      if (options.editable) {
        moveImageLayoutToContainer(image, existingFrame)
        continue
      }
      const src = image.getAttribute('src') ?? ''
      const safe = isSafeAnnouncementImageUrl(src)
      const wrapper = image.ownerDocument.createElement(safe ? 'a' : 'span')
      wrapper.className = safe ? 'announcement-image-link' : 'announcement-image-frame'
      copyLayout(existingFrame, wrapper)
      moveImageLayoutToContainer(image, wrapper)
      if (wrapper instanceof HTMLAnchorElement) {
        wrapper.href = src
        wrapper.target = '_blank'
        wrapper.rel = 'noopener noreferrer'
        wrapper.setAttribute('aria-label', image.alt || options.imageOpenLabel)
      }
      existingFrame.replaceWith(wrapper)
      wrapper.append(image)
      continue
    }

    const src = image.getAttribute('src') ?? ''
    const safe = isSafeAnnouncementImageUrl(src)
    const canOpen = safe && !options.editable
    const wrapper = image.ownerDocument.createElement(canOpen ? 'a' : 'span')
    wrapper.className = canOpen ? 'announcement-image-link' : 'announcement-image-frame'
    moveImageLayoutToContainer(image, wrapper)
    if (wrapper instanceof HTMLAnchorElement) {
      wrapper.href = src
      wrapper.target = '_blank'
      wrapper.rel = 'noopener noreferrer'
      wrapper.setAttribute('aria-label', image.alt || options.imageOpenLabel)
    }
    image.replaceWith(wrapper)
    wrapper.append(image)
  }
}

function moveImageLayoutToContainer(image: HTMLImageElement, container: HTMLElement): void {
  if (image.dataset.announcementWidth) {
    container.dataset.announcementWidth = image.dataset.announcementWidth
    delete image.dataset.announcementWidth
  }
  if (image.dataset.announcementAlign) {
    container.dataset.announcementAlign = image.dataset.announcementAlign
    delete image.dataset.announcementAlign
  }
}

function copyLayout(source: HTMLElement, target: HTMLElement): void {
  if (source.dataset.announcementWidth) target.dataset.announcementWidth = source.dataset.announcementWidth
  if (source.dataset.announcementAlign) target.dataset.announcementAlign = source.dataset.announcementAlign
}
