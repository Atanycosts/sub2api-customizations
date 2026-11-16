import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getRequestTiming, listRequestTimings } from './requestTimings'
import { apiClient } from '../client'
vi.mock('../client', () => ({ apiClient: { get: vi.fn() } }))
describe('request timing API', () => {
  beforeEach(() => vi.clearAllMocks())
  it('preserves filters and missing measurements', async () => {
    const result = { items: [{ first_text_ms: null, completeness: 'partial' }], stats: { queued: 0, dropped: 3, write_failed: 1 }, retention_days: 7 }
    vi.mocked(apiClient.get).mockResolvedValue({ data: result })
    expect(await listRequestTimings({ account_id: 19, request_id: 'request' })).toEqual(result)
    expect(apiClient.get).toHaveBeenCalledWith('/admin/request-timings', { params: { account_id: 19, request_id: 'request' } })
  })
  it('encodes detail identifiers', async () => {
    vi.mocked(apiClient.get).mockResolvedValue({ data: { trace_id: 'a/b' } })
    await getRequestTiming('a/b')
    expect(apiClient.get).toHaveBeenCalledWith('/admin/request-timings/a%2Fb')
  })
})
