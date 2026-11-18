// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { buildOperationsPreview } from '../operationsPreview'
describe('observation artifact isolation', () => {
  it('places the network policy before model markup and removes refresh navigation', () => {
    const source = '<script>fetch("https://evil.example")</script><meta http-equiv="refresh" content="0;url=https://evil.example"><svg></svg>'
    const result = buildOperationsPreview(source, 'valid-nonce')
    expect(result.indexOf('connect-src')).toBeLessThan(result.indexOf('<script'))
    expect(result).not.toContain('http-equiv="refresh"')
    expect(result).toContain('script nonce="valid-nonce"')
  })
  it('never grants the app nonce to an external script', () => {
    const result = buildOperationsPreview('<script nonce="forged" src="https://evil.example/a.js"></script><script nonce="forged">animate()</script>', 'real-nonce')
    const doc = new DOMParser().parseFromString(result, 'text/html')
    expect(doc.querySelector('script[src]')?.hasAttribute('nonce')).toBe(false)
    expect(doc.querySelector('script:not([src])')?.getAttribute('nonce')).toBe('real-nonce')
  })
})
