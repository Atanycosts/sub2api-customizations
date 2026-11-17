import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put, remove } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  remove: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put, delete: remove },
}))

import {
  deletePelicanPlan,
  getPelicanAdminResult,
  getPelicanAdminResults,
  getPelicanConfig,
  getPelicanPlans,
  getPelicanResult,
  getPelicanResults,
  runPelicanPlan,
  savePelicanConfig,
  savePelicanPlan,
} from '@/api/pelican'

describe('pelican API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
    remove.mockReset()
  })

  it('fetches public metadata and one detail through the public endpoints', async () => {
    const signal = new AbortController().signal
    get
      .mockResolvedValueOnce({ data: { items: [], total: 0 } })
      .mockResolvedValueOnce({ data: { id: 'run-1', output: '<svg/>' } })

    await getPelicanResults(
      { group_id: 10, model: 'gpt-5', effort: 'high', page: 2, page_size: 12 },
      { signal },
    )
    await getPelicanResult('run-1', { signal })

    expect(get).toHaveBeenNthCalledWith(1, '/pelican', {
      params: { group_id: 10, model: 'gpt-5', effort: 'high', page: 2, page_size: 12 },
      signal,
    })
    expect(get).toHaveBeenNthCalledWith(2, '/pelican/run-1', { signal })
  })

  it('uses the standalone admin contract for config, plans, runs and results', async () => {
    get
      .mockResolvedValueOnce({ data: { enabled: false, daily_budget_usd: 0 } })
      .mockResolvedValueOnce({ data: [] })
      .mockResolvedValueOnce({ data: { items: [], stats: {} } })
      .mockResolvedValueOnce({ data: { id: 'run-2' } })
    put
      .mockResolvedValueOnce({ data: { enabled: true, daily_budget_usd: 2 } })
      .mockResolvedValueOnce({ data: { id: 9 } })
    post
      .mockResolvedValueOnce({ data: { id: 9 } })
      .mockResolvedValueOnce({ data: { id: 'run-2' } })
    remove.mockResolvedValueOnce({ data: {} })

    await getPelicanConfig()
    await getPelicanPlans()
    await getPelicanAdminResults({ plan_id: 9, state: 'running', page: 1, page_size: 12 })
    await getPelicanAdminResult('run-2')
    await savePelicanConfig({ enabled: true, daily_budget_usd: 2 })

    const planInput = {
      group_id: 10,
      model: 'gpt-5',
      effort: 'high' as const,
      protocol: 'responses' as const,
      enabled: true,
      published: true,
      interval_minutes: 60,
      max_output_tokens: 4096,
      budget_usd: 1,
    }
    await savePelicanPlan(null, planInput)
    await savePelicanPlan(9, planInput)
    await runPelicanPlan(9, '11111111-1111-4111-8111-111111111111')
    await deletePelicanPlan(9)

    expect(get).toHaveBeenNthCalledWith(1, '/admin/pelican/config')
    expect(get).toHaveBeenNthCalledWith(2, '/admin/pelican/plans')
    expect(get).toHaveBeenNthCalledWith(3, '/admin/pelican/results', {
      params: { plan_id: 9, state: 'running', page: 1, page_size: 12 },
    })
    expect(get).toHaveBeenNthCalledWith(4, '/admin/pelican/results/run-2')
    expect(put).toHaveBeenNthCalledWith(1, '/admin/pelican/config', {
      enabled: true,
      daily_budget_usd: 2,
    })
    expect(post).toHaveBeenNthCalledWith(1, '/admin/pelican/plans', planInput)
    expect(put).toHaveBeenNthCalledWith(2, '/admin/pelican/plans/9', planInput)
    expect(post).toHaveBeenNthCalledWith(2, '/admin/pelican/plans/9/run', {
      submit_id: '11111111-1111-4111-8111-111111111111',
    })
    expect(remove).toHaveBeenCalledWith('/admin/pelican/plans/9')
  })
})
