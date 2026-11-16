// An opaque sandbox is mandatory in addition to this network-denying policy.
export function buildOperationsPreview(source: string, nonce: string): string {
  const document = new DOMParser().parseFromString(source.replace(/^```(?:html|svg)?\s*\n?/i, '').replace(/\n?```\s*$/, ''), 'text/html')
  const validNonce = /^[A-Za-z0-9+/_=-]+$/.test(nonce)
  for (const script of document.querySelectorAll('script')) {
    script.removeAttribute('nonce')
    if (validNonce && !script.hasAttribute('src') && !script.hasAttribute('href') && !script.hasAttribute('xlink:href')) script.setAttribute('nonce', nonce)
  }
  for (const meta of document.querySelectorAll('meta[http-equiv]')) if (meta.getAttribute('http-equiv')?.toLowerCase() === 'refresh') meta.remove()
  return '<!doctype html><meta http-equiv="Content-Security-Policy" content="default-src \'none\'; script-src \'unsafe-inline\'; style-src \'unsafe-inline\'; img-src data: blob:; font-src data:; media-src data: blob:; connect-src \'none\'; frame-src \'none\'; object-src \'none\'; form-action \'none\'; base-uri \'none\'"><meta name="referrer" content="no-referrer">' + document.documentElement.outerHTML
}
