import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import PelicanDetailDialog from '../PelicanDetailDialog.vue'
import type { PelicanPublicRunDetail } from '@/types/pelican'

const api = vi.hoisted(() => ({ getPelicanResult: vi.fn() }))
vi.mock('@/api/pelican', () => api)
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key })
}))
const run: PelicanPublicRunDetail = {
  id: 'actual-history-run',
  source: 'group',
  group_id: 7,
  group_name: 'Selected channel',
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
  error_kind: '',
  output: '<svg><text>actual artwork</text></svg>'
}
function mountDetail() {
  return mount(PelicanDetailDialog, {
    props: { show: true, runId: run.id },
    global: {
      stubs: {
        BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
        LoadingSpinner: true
      }
    }
  })
}
beforeEach(() => {
  api.getPelicanResult.mockReset()
  api.getPelicanResult.mockResolvedValue(run)
})

describe('compact user observation detail', () => {
  it('places actual sandboxed artwork after compact metadata and before secondary facts', async () => {
    const wrapper = mountDetail()
    await flushPromises()
    expect(api.getPelicanResult).toHaveBeenCalledWith(run.id, { signal: expect.any(AbortSignal) })
    const frame = wrapper.get('iframe')
    expect(frame.attributes('sandbox')).toBe('allow-scripts')
    expect(frame.attributes('srcdoc')).toContain('actual artwork')
    expect(wrapper.find('dl').findAll('div')).toHaveLength(2)
    const html = wrapper.html()
    expect(html.indexOf('<iframe')).toBeLessThan(html.indexOf('pelican.fields.outputCap'))
    expect(html.indexOf('<iframe')).toBeLessThan(html.indexOf('pelican.preview.isolationNotice'))
    expect(wrapper.text()).toContain('Selected channel')
    expect(wrapper.text()).toContain('sol')
    wrapper.unmount()
  })

  it('does not insert an artwork frame when the selected run has no output', async () => {
    api.getPelicanResult.mockResolvedValue({
      ...run,
      state: 'failed',
      error_kind: 'timeout',
      output: undefined
    })
    const wrapper = mountDetail()
    await flushPromises()
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.text()).toContain('pelican.errorKind.timeout')
    wrapper.unmount()
  })
})
