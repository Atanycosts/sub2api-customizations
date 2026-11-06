import { beforeEach, describe, expect, it } from 'vitest'
import { DEFAULT_SITE_LOGO_URL, updateFavicon } from '@/utils/branding'

describe('updateFavicon', () => {
  beforeEach(() => {
    document.head.innerHTML = '<link rel="icon" href="/logo.svg">'
  })

  it('replaces the default favicon with the configured logo', () => {
    updateFavicon('https://example.com/custom-logo.png')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.href).toBe('https://example.com/custom-logo.png')
  })

  it('uses the shared default for unsafe logo URLs', () => {
    updateFavicon('javascript:alert(1)')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.getAttribute('href')).toBe(DEFAULT_SITE_LOGO_URL)
  })

  it('restores the shared default when a custom logo is removed', () => {
    updateFavicon('https://example.com/custom-logo.png')
    updateFavicon('')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.getAttribute('href')).toBe(DEFAULT_SITE_LOGO_URL)
    expect(link?.getAttribute('type')).toBe('image/svg+xml')
  })

  it('recognizes SVG URLs with a cache version', () => {
    updateFavicon('/custom.svg?v=2')

    expect(document.querySelector('link[rel="icon"]')?.getAttribute('type')).toBe('image/svg+xml')
  })
})
