import { sanitizeUrl } from '@/utils/url'

export const DEFAULT_SITE_LOGO_URL = '/logo.svg?v=20261107'
export const DEFAULT_SITE_NAME = 'Sub2API'

export function updateFavicon(logoUrl: string): void {
  const faviconUrl = sanitizeUrl(logoUrl, {
    allowRelative: true,
    allowDataUrl: true,
  }) || DEFAULT_SITE_LOGO_URL

  let link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    document.head.appendChild(link)
  }

  if (/\.svg(?:[?#]|$)/i.test(faviconUrl) || faviconUrl.startsWith('data:image/svg+xml')) {
    link.type = 'image/svg+xml'
  } else {
    link.removeAttribute('type')
  }
  link.href = faviconUrl
}
