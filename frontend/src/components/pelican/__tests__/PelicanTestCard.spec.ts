import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import PelicanTestCard from '../PelicanTestCard.vue'
import type { PelicanPublicRun } from '@/types/pelican'

const api = vi.hoisted(() => ({ getPelicanResult: vi.fn() }))
vi.mock('@/api/pelican', () => api)
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key })
}))
const run: PelicanPublicRun = {
  id: 'first',
  source: 'group',
  group_id: 7,
  group_name: 'Group',
  model: 'sol',
  effort: 'high',
  protocol: 'responses',
  prompt_version: 'v1',
  max_output_tokens: 1000,
  state: 'completed',
  created_at: '2026-11-18T00:00:00Z',
  finished_at: null,
  duration_ms: 800,
  terminal: '',
  error_kind: ''
}
const svg = (text: string) => `<svg><text>${text}</text></svg>`
function mountCard(value = run) {
  return mount(PelicanTestCard, {
    props: { run: value, selected: false, comparisonFull: false },
    global: { stubs: { Icon: true, LoadingSpinner: true } }
  })
}

beforeEach(() => {
  api.getPelicanResult.mockReset()
  vi.stubGlobal('IntersectionObserver', undefined)
  // Browsers without IntersectionObserver still load the real preview.
  delete (window as unknown as Record<string, unknown>).IntersectionObserver
})

describe('Pelican cards', () => {
  it('keeps failures explicit and never requests a substituted artwork', async () => {
    const wrapper = mountCard({
      ...run,
      state: 'failed',
      error_kind: 'timeout'
    })
    await flushPromises()
    expect(api.getPelicanResult).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('pelican.preview.failed')
    expect(wrapper.text()).toContain('pelican.errorKind.timeout')
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.find('input[type="checkbox"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('loads a newly completed visible record and preserves sandboxed static preview', async () => {
    api.getPelicanResult.mockResolvedValue({
      ...run,
      output: svg('real-output')
    })
    const wrapper = mountCard({ ...run, state: 'running' })
    expect(api.getPelicanResult).not.toHaveBeenCalled()
    await wrapper.setProps({ run })
    await flushPromises()
    expect(api.getPelicanResult).toHaveBeenCalledWith('first', {
      signal: expect.any(AbortSignal)
    })
    const frame = wrapper.get('iframe')
    expect(frame.attributes('sandbox')).toBe('')
    expect(frame.attributes('srcdoc')).toContain('real-output')
    await wrapper.setProps({ comparisonFull: true })
    expect(wrapper.get('input').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('cancels stale output on run changes and unmount', async () => {
    let resolveFirst!: (value: unknown) => void
    api.getPelicanResult.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveFirst = resolve
      })
    )
    api.getPelicanResult.mockResolvedValueOnce({
      ...run,
      output: svg('current-output')
    })
    const wrapper = mountCard()
    const firstSignal = api.getPelicanResult.mock.calls[0][1].signal as AbortSignal
    await wrapper.setProps({ run: { ...run, id: 'second' } })
    await flushPromises()
    resolveFirst({ ...run, output: svg('stale-output') })
    await flushPromises()
    expect(firstSignal.aborted).toBe(true)
    const output = wrapper.get('iframe').attributes('srcdoc')
    expect(output).toContain('current-output')
    expect(output).not.toContain('stale-output')
    const currentSignal = api.getPelicanResult.mock.lastCall?.[1].signal as AbortSignal
    wrapper.unmount()
    expect(currentSignal.aborted).toBe(true)
  })
})
