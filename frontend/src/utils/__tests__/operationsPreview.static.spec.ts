// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { buildOperationsStaticPreview } from '../operationsPreview'

function decodeSVG(previewDocument: string): string {
  const parsed = new DOMParser().parseFromString(previewDocument, 'text/html')
  const src = parsed.querySelector('img')?.getAttribute('src') || ''
  const prefix = 'data:image/svg+xml;charset=utf-8,'
  expect(src.startsWith(prefix)).toBe(true)
  return decodeURIComponent(src.slice(prefix.length))
}

describe('pelican static preview', () => {
  it('keeps document styles while removing scripts, handlers, and SMIL', () => {
    const artifact = `<html><head><style>.frame { fill: none; stroke: coral; }</style></head>
      <body><svg viewBox="0 0 100 100"><path class="frame" d="M10 10L90 90" onclick="alert(1)" />
      <animate attributeName="x" /><script>alert(1)</script></svg></body></html>`
    const preview = buildOperationsStaticPreview(
      artifact,
    )
    const svg = decodeSVG(preview.document)

    expect(preview.hasStaticVisual).toBe(true)
    expect(preview.document).toContain("script-src 'none'")
    expect(svg).toContain('.frame {')
    expect(svg).toContain('class="frame"')
    expect(svg).not.toContain('<script')
    expect(svg).not.toContain('onclick=')
    expect(svg).not.toContain('<animate')
    expect(svg.lastIndexOf('animation-play-state')).toBeGreaterThan(svg.lastIndexOf('.frame {'))
  })

  it('reports script-only canvas output as dynamic instead of presenting a blank thumbnail', () => {
    const preview = buildOperationsStaticPreview(
      '<canvas id="stage"></canvas><script>drawScene()</script>',
    )

    expect(preview.hasStaticVisual).toBe(false)
    expect(preview.document).toContain("script-src 'none'")
    expect(preview.document).not.toContain('<script')
  })
})
