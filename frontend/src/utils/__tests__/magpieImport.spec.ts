import { describe, expect, it } from 'vitest'
import { buildMagpieImportLink } from '../magpieImport'
import type { ApiKey } from '@/types'
const row = { id: 12, name: 'My key', key: 'secret&key', group_id: 3, group: { platform: 'openai', allow_messages_dispatch: false } } as ApiKey
function build(overrides = {}) { return buildMagpieImportLink({ dashboardOrigin: 'https://gateway.example', row, ...overrides }) }
describe('Magpie import contract', () => {
  it('uses the existing import scheme with encoded key and exact endpoints', () => {
    const result = build(); expect('url' in result).toBe(true); if (!('url' in result)) return
    const url = new URL(result.url); expect(url.protocol).toBe('magpie:'); expect(url.hostname).toBe('import'); expect(url.searchParams.get('key')).toBe('secret&key'); expect(url.searchParams.get('chat')).toBe('https://gateway.example/v1'); expect(url.searchParams.get('responses')).toBe('https://gateway.example/v1'); expect(url.searchParams.has('anthropic')).toBe(false)
  })
  it('rejects ambiguous grouping and external insecure endpoints', () => {
    expect(build({ row: { ...row, group_ids: [3, 4] } })).toEqual({ error: 'unsupportedGroup' })
    expect(build({ baseUrl: 'http://gateway.example' })).toEqual({ error: 'invalidBase' })
    expect(build({ baseUrl: 'https://user:pass@gateway.example' })).toEqual({ error: 'invalidBase' })
  })
  it('does not derive provider identity from the credential', () => {
    const first = build(), second = build({ row: { ...row, key: 'replacement' } }); if ('url' in first && 'url' in second) expect(new URL(first.url).searchParams.get('id')).toBe(new URL(second.url).searchParams.get('id'))
  })
})
