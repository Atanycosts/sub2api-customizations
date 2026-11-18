export interface OperationsPreviewOptions {
  scripts?: 'allow' | 'none'
}

export interface OperationsStaticPreview {
  document: string
  hasStaticVisual: boolean
}

const cspPrefix = '<!doctype html><meta http-equiv="Content-Security-Policy" content="default-src \'none\'; script-src '
const cspSuffix = '; style-src \'unsafe-inline\'; img-src data: blob:; font-src data:; media-src data: blob:; connect-src \'none\'; frame-src \'none\'; object-src \'none\'; form-action \'none\'; base-uri \'none\'"><meta name="referrer" content="no-referrer">'
const pauseCss = '<style>*,*::before,*::after{animation-play-state:paused!important;transition:none!important;scroll-behavior:auto!important}</style>'

function stripUnsafeAttributes(root: ParentNode): void {
  for (const element of root.querySelectorAll<Element>('*')) {
    for (const attribute of [...element.attributes]) {
      const name = attribute.name.toLowerCase()
      if (name.startsWith('on')) element.removeAttribute(attribute.name)
    }
  }
}

function parseSource(source: string): Document {
  return new DOMParser().parseFromString(source.replace(/^```(?:html|svg)?\s*\n?/i, '').replace(/\n?```\s*$/, ''), 'text/html')
}

// An opaque sandbox is mandatory in addition to this network-denying policy.
export function buildOperationsPreview(
  source: string,
  nonce: string,
  options: OperationsPreviewOptions = {},
): string {
  const document = parseSource(source)
  const validNonce = /^[A-Za-z0-9+/_=-]+$/.test(nonce)
  const allowScripts = options.scripts !== 'none'
  if (!allowScripts) {
    document.querySelectorAll('script, iframe, object, embed').forEach((element) => element.remove())
  }
  stripUnsafeAttributes(document)
  for (const script of document.querySelectorAll('script')) {
    script.removeAttribute('nonce')
    if (validNonce && !script.hasAttribute('src') && !script.hasAttribute('href') && !script.hasAttribute('xlink:href')) script.setAttribute('nonce', nonce)
  }
  for (const meta of document.querySelectorAll('meta[http-equiv]')) if (meta.getAttribute('http-equiv')?.toLowerCase() === 'refresh') meta.remove()
  const scriptPolicy = allowScripts ? "'unsafe-inline'" : "'none'"
  const prefix = cspPrefix + scriptPolicy + cspSuffix
  if (allowScripts) return prefix + document.documentElement.outerHTML
  return prefix + pauseCss + document.documentElement.outerHTML
}

function sanitizeSvg(svg: Element): boolean {
  svg.querySelectorAll('script, animate, animateMotion, animateTransform, set').forEach((element) => element.remove())
  for (const element of [svg, ...svg.querySelectorAll('*')]) {
    for (const attribute of [...element.attributes]) {
      const name = attribute.name.toLowerCase()
      if (name.startsWith('on')) element.removeAttribute(attribute.name)
      if (name === 'href' || name === 'xlink:href') {
        const value = attribute.value.trim()
        if (value && !value.startsWith('#') && !value.startsWith('data:image/')) element.removeAttribute(attribute.name)
      }
    }
  }
  return Boolean(
    svg.matches('path,rect,circle,ellipse,line,polyline,polygon,text,textPath,image,use,foreignObject')
      || svg.querySelector('path,rect,circle,ellipse,line,polyline,polygon,text,textPath,image,use,foreignObject'),
  )
}

function sanitizeCss(css: string): string {
  return css
    .replace(/@import\s+[^;]+;?/gi, '')
    .replace(/url\(\s*(['"]?)(?!#|data:)([^)'"]+)\1\s*\)/gi, 'none')
}

export function buildOperationsStaticPreview(source: string): OperationsStaticPreview {
  const parsed = parseSource(source)
  parsed.querySelectorAll('script, iframe, object, embed').forEach((element) => element.remove())
  stripUnsafeAttributes(parsed)
  const svg = parsed.querySelector('svg')
  if (svg) {
    const hasStaticVisual = sanitizeSvg(svg)
    if (hasStaticVisual) {
      const styles = [...parsed.querySelectorAll('style')]
        .map((style) => sanitizeCss(style.textContent || ''))
        .filter(Boolean)
        .join('\n')
      if (styles) {
        const style = document.createElementNS('http://www.w3.org/2000/svg', 'style')
        style.textContent = styles
        svg.insertBefore(style, svg.firstChild)
      }
      const pause = document.createElementNS('http://www.w3.org/2000/svg', 'style')
      pause.textContent = 'svg *,svg *::before,svg *::after{animation-play-state:paused!important;transition:none!important}'
      svg.appendChild(pause)
      svg.setAttribute('xmlns', 'http://www.w3.org/2000/svg')
      const image = document.createElement('img')
      image.setAttribute('src', `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg.outerHTML)}`)
      image.setAttribute('alt', '')
      return {
        hasStaticVisual: true,
        document: buildOperationsPreview(
          `<style>html,body{width:100%;height:100%;margin:0;background:transparent}body{display:flex;align-items:center;justify-content:center;overflow:hidden}img{display:block;width:100%;height:100%;object-fit:contain}</style>${image.outerHTML}`,
          '',
          { scripts: 'none' },
        ),
      }
    }
  }
  const hasStaticVisual = Boolean(
    parsed.body.querySelector('img,picture,video,audio,table')
      || parsed.body.textContent?.trim()
      || [...parsed.body.querySelectorAll<HTMLElement>('*')].some((element) =>
        element.matches('div,section,article,main,header,footer') && element.children.length > 0,
      ),
  )
  return {
    hasStaticVisual,
    document: hasStaticVisual
      ? buildOperationsPreview(source, '', { scripts: 'none' })
      : buildOperationsPreview('', '', { scripts: 'none' }),
  }
}
