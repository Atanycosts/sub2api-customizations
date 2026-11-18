import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { computed, ref, type Ref } from 'vue'
import PelicanView from '../PelicanView.vue'
import PelicanHistoryDialog from '@/components/pelican/PelicanHistoryDialog.vue'
import PelicanTestCard from '@/components/pelican/PelicanTestCard.vue'
import PelicanCompareDialog from '@/components/pelican/PelicanCompareDialog.vue'
import PelicanCompactPager from '@/components/pelican/PelicanCompactPager.vue'
import type { PelicanPublicPage, PelicanPublicRun } from '@/types/pelican'

const api = vi.hoisted(() => ({ getPelicanResults: vi.fn() }))
vi.mock('@/api/pelican', () => api)
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key })
}))
const galleryHeight = ref(600)
vi.mock('@/components/pelican/usePelicanGalleryLayout', () => ({
  usePelicanGalleryLayout: (_element: unknown, listMode: Ref<boolean>) =>
    computed(() => {
      const rows = galleryHeight.value >= 420 ? 2 : 1
      return listMode.value
        ? { compact: true, columns: 1, rows: 5, pageSize: 5 }
        : { compact: false, columns: 4, rows, pageSize: 4 * rows }
    })
}))
const run: PelicanPublicRun = {
  id: 'latest-failed',
  source: 'group',
  group_id: 7,
  group_name: 'Group 7',
  model: 'sol',
  effort: 'high',
  protocol: 'responses',
  prompt_version: 'v1',
  max_output_tokens: 1000,
  state: 'failed',
  created_at: '2026-11-18T00:00:00Z',
  finished_at: null,
  duration_ms: 800,
  terminal: '',
  error_kind: 'timeout',
  history_count: 12
}
const result = (items = [run]): PelicanPublicPage => ({
  groups: [{ id: 7, name: 'Group 7', platform: 'openai' }],
  items,
  total: 12,
  page: 1,
  page_size: 8,
  pages: 2,
  stats: { completed: 0, failed: 1, running: 0, unknown: 0 },
  prompt: 'Real fixed prompt',
  prompt_version: 'v1'
})
const stubs = {
  AppLayout: { template: '<main><slot /></main>' },
  Select: {
    props: ['modelValue', 'options'],
    emits: ['update:modelValue', 'change'],
    template:
      '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value); $emit(\'change\')"><option value="">All</option><option value="7">Group</option><option value="high">high</option></select>'
  },
  PelicanTestCard: {
    props: ['run'],
    emits: ['history', 'detail', 'toggle-compare'],
    template:
      '<article><span>{{ run.state }}</span><button class="history" @click="$emit(\'history\')">History</button></article>'
  },
  PelicanHistoryDialog: true,
  PelicanRulesDialog: true,
  PelicanDetailDialog: true,
  PelicanCompareDialog: true
}

afterEach(() => {
  vi.useRealTimers()
})

beforeEach(() => {
  galleryHeight.value = 600
  api.getPelicanResults.mockReset()
  api.getPelicanResults.mockResolvedValue(result())
})

describe('compact Pelican gallery', () => {
  it('keeps one-channel results loaded when container height changes within the same capacity', async () => {
    api.getPelicanResults.mockResolvedValue({ ...result(), total: 1, pages: 1 })
    const wrapper = mount(PelicanView, { global: { stubs } })
    await flushPromises()
    expect(api.getPelicanResults).toHaveBeenCalledTimes(1)

    galleryHeight.value = 560
    await flushPromises()
    galleryHeight.value = 540
    await flushPromises()

    expect(api.getPelicanResults).toHaveBeenCalledTimes(1)
    expect(wrapper.get('article').text()).toContain('failed')
    wrapper.unmount()
  })

  it('requests server-grouped results and keeps the latest failure visible', async () => {
    const wrapper = mount(PelicanView, { global: { stubs } })
    await flushPromises()
    expect(api.getPelicanResults).toHaveBeenCalledWith(
      { page: 1, page_size: 8, latest_per_group: true },
      { signal: expect.any(AbortSignal) }
    )
    expect(wrapper.get('article').text()).toContain('failed')
    await wrapper.findComponent(PelicanCompactPager).vm.$emit('update:page', 2)
    expect(api.getPelicanResults.mock.lastCall?.[0]).toMatchObject({
      page: 2,
      page_size: 8,
      latest_per_group: true
    })
    wrapper.unmount()
  })

  it('passes the applied model and effort filters to independent group history', async () => {
    const wrapper = mount(PelicanView, { global: { stubs } })
    await flushPromises()
    await wrapper.get('input').setValue('sol')
    await wrapper.get('input').trigger('change')
    await wrapper.findAll('select')[1].setValue('high')
    await flushPromises()
    expect(api.getPelicanResults.mock.lastCall?.[0]).toMatchObject({
      latest_per_group: true,
      model: 'sol',
      effort: 'high'
    })
    await wrapper.get('.history').trigger('click')
    expect(wrapper.findComponent(PelicanHistoryDialog).props()).toMatchObject({
      show: true,
      groupId: 7,
      model: 'sol',
      effort: 'high'
    })
    wrapper.unmount()
  })

  it('switches the list request to its available row count and resets pagination', async () => {
    const wrapper = mount(PelicanView, { global: { stubs } })
    await flushPromises()
    wrapper.findComponent(PelicanCompactPager).vm.$emit('update:page', 2)
    await flushPromises()
    await wrapper.get('button[aria-pressed]').trigger('click')
    await flushPromises()
    expect(api.getPelicanResults.mock.lastCall?.[0]).toMatchObject({
      page: 1,
      page_size: 5,
      latest_per_group: true
    })
    expect(wrapper.get('button[aria-pressed]').attributes('aria-pressed')).toBe('true')
    wrapper.unmount()
  })

  it('blocks old cards during changed queries and keeps them hidden after failure', async () => {
    const wrapper = mount(PelicanView, { global: { stubs } })
    await flushPromises()
    let rejectQuery!: (reason: Error) => void
    api.getPelicanResults.mockReturnValue(
      new Promise((_resolve, reject) => {
        rejectQuery = reject
      })
    )
    await wrapper.get('input').setValue('new-model')
    await wrapper.get('input').trigger('change')
    expect(wrapper.find('article').exists()).toBe(false)
    expect(wrapper.get('section').attributes('aria-busy')).toBe('true')
    expect(wrapper.findComponent(PelicanHistoryDialog).props('show')).toBe(false)
    rejectQuery(new Error('query failed'))
    await flushPromises()
    expect(wrapper.find('article').exists()).toBe(false)
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.findAll('select')[0].text()).toContain('Group')
    wrapper.unmount()
  })

  it('refetches the last valid page when the server result count shrinks', async () => {
    const wrapper = mount(PelicanView, { global: { stubs } })
    await flushPromises()
    api.getPelicanResults.mockResolvedValueOnce({ ...result([]), page: 2, pages: 1, total: 1 })
    api.getPelicanResults.mockResolvedValueOnce({ ...result(), pages: 1, total: 1 })
    wrapper.findComponent(PelicanCompactPager).vm.$emit('update:page', 2)
    await flushPromises()
    expect(api.getPelicanResults.mock.calls.slice(-2).map(([filters]) => filters.page)).toEqual([
      2, 1
    ])
    expect(wrapper.findComponent(PelicanCompactPager).props('page')).toBe(1)
    expect(wrapper.find('article').exists()).toBe(true)
    wrapper.unmount()
  })

  it('clears page-scoped comparison on filters, paging, and layout changes', async () => {
    api.getPelicanResults.mockResolvedValue(result([{ ...run, state: 'completed' }]))
    const wrapper = mount(PelicanView, { global: { stubs } })
    await flushPromises()
    const selectCard = async () => {
      wrapper.findComponent(PelicanTestCard).vm.$emit('toggle-compare', run.id)
      await wrapper.vm.$nextTick()
      expect(wrapper.findComponent(PelicanCompareDialog).props('ids')).toEqual([run.id])
    }
    await selectCard()
    await wrapper.get('input').setValue('sol')
    await wrapper.get('input').trigger('change')
    await flushPromises()
    expect(wrapper.findComponent(PelicanCompareDialog).props('ids')).toEqual([])
    await selectCard()
    wrapper.findComponent(PelicanCompactPager).vm.$emit('update:page', 2)
    await flushPromises()
    expect(wrapper.findComponent(PelicanCompareDialog).props('ids')).toEqual([])
    await selectCard()
    await wrapper.get('button[aria-pressed]').trigger('click')
    await flushPromises()
    expect(wrapper.findComponent(PelicanCompareDialog).props('ids')).toEqual([])
    wrapper.unmount()
  })

  it('retains only currently completed comparison IDs after a background poll', async () => {
    vi.useFakeTimers()
    const second = { ...run, id: 'second', state: 'completed' as const }
    const initial = {
      ...result([{ ...run, state: 'completed' }, second]),
      stats: { completed: 2, failed: 0, unknown: 0, running: 1 }
    }
    api.getPelicanResults.mockResolvedValueOnce(initial)
    const wrapper = mount(PelicanView, { global: { stubs } })
    await flushPromises()
    wrapper
      .findAllComponents(PelicanTestCard)
      .forEach((card) => card.vm.$emit('toggle-compare', card.props('run').id))
    await wrapper.vm.$nextTick()
    expect(wrapper.findComponent(PelicanCompareDialog).props('ids')).toEqual([run.id, second.id])
    api.getPelicanResults.mockResolvedValueOnce(result([second]))
    await vi.advanceTimersByTimeAsync(10_000)
    await flushPromises()
    expect(wrapper.findComponent(PelicanCompareDialog).props('ids')).toEqual([second.id])
    wrapper.unmount()
  })

  it('discovers scheduled results while idle and refreshes immediately on returning to the tab', async () => {
    vi.useFakeTimers()
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    const wrapper = mount(PelicanView, { global: { stubs } })
    try {
      await flushPromises()
      api.getPelicanResults.mockResolvedValueOnce({
        ...result([{ ...run, id: 'scheduled', state: 'running' }]),
        stats: { completed: 0, failed: 0, unknown: 0, running: 1 },
      })
      await vi.advanceTimersByTimeAsync(30_000)
      expect(wrapper.get('article').text()).toContain('running')

      visibility.mockReturnValue('hidden')
      document.dispatchEvent(new Event('visibilitychange'))
      await vi.advanceTimersByTimeAsync(30_000)
      expect(api.getPelicanResults).toHaveBeenCalledTimes(2)

      api.getPelicanResults.mockResolvedValueOnce(result([{ ...run, id: 'scheduled', state: 'completed' }]))
      visibility.mockReturnValue('visible')
      document.dispatchEvent(new Event('visibilitychange'))
      await vi.advanceTimersByTimeAsync(0)
      expect(wrapper.get('article').text()).toContain('completed')
    } finally {
      wrapper.unmount()
      visibility.mockRestore()
    }
    await vi.advanceTimersByTimeAsync(60_000)
    expect(api.getPelicanResults).toHaveBeenCalledTimes(3)
  })

  it('ignores a superseded response and cancels the current request on unmount', async () => {
    let resolveFirst!: (value: PelicanPublicPage) => void
    api.getPelicanResults.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveFirst = resolve
      })
    )
    const wrapper = mount(PelicanView, { global: { stubs } })
    const firstSignal = api.getPelicanResults.mock.calls[0][1].signal as AbortSignal
    await wrapper.get('input').setValue('sol')
    await wrapper.get('input').trigger('change')
    await flushPromises()
    resolveFirst(result([{ ...run, id: 'stale', state: 'completed' }]))
    await flushPromises()
    expect(firstSignal.aborted).toBe(true)
    expect(wrapper.get('article').text()).toContain('failed')
    const currentSignal = api.getPelicanResults.mock.lastCall?.[1].signal as AbortSignal
    wrapper.unmount()
    expect(currentSignal.aborted).toBe(true)
  })
})

describe('group history', () => {
  const mountHistory = () =>
    mount(PelicanHistoryDialog, {
      props: {
        show: true,
        groupId: 7,
        groupName: 'Group 7',
        model: 'sol',
        effort: 'high'
      },
      global: {
        stubs: {
          BaseDialog: {
            props: ['show'],
            template: '<div v-if="show"><slot /></div>'
          },
          LoadingSpinner: true
        }
      }
    })

  it('queries all matching history, paginates and opens the actual selected run', async () => {
    const wrapper = mountHistory()
    await flushPromises()
    expect(api.getPelicanResults.mock.lastCall?.[0]).toEqual({
      group_id: 7,
      model: 'sol',
      effort: 'high',
      page: 1,
      page_size: 8
    })
    await wrapper.get('li button').trigger('click')
    expect(wrapper.emitted('detail')).toEqual([['latest-failed']])
    wrapper.findComponent(PelicanCompactPager).vm.$emit('update:page', 2)
    expect(api.getPelicanResults.mock.lastCall?.[0]).toMatchObject({
      group_id: 7,
      page: 2
    })
    wrapper.unmount()
  })

  it('returns history to a valid page after published records shrink', async () => {
    const wrapper = mountHistory()
    await flushPromises()
    api.getPelicanResults.mockResolvedValueOnce({ ...result([]), page: 2, pages: 0, total: 0 })
    api.getPelicanResults.mockResolvedValueOnce({ ...result([]), pages: 0, total: 0 })
    wrapper.findComponent(PelicanCompactPager).vm.$emit('update:page', 2)
    await flushPromises()
    expect(api.getPelicanResults.mock.calls.slice(-2).map(([filters]) => filters.page)).toEqual([
      2, 1
    ])
    expect(wrapper.findComponent(PelicanCompactPager).props()).toMatchObject({ page: 1, total: 0 })
    wrapper.unmount()
  })

  it('aborts when closed and ignores an old group response', async () => {
    let resolveOld!: (value: PelicanPublicPage) => void
    api.getPelicanResults.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveOld = resolve
      })
    )
    const wrapper = mountHistory()
    const oldSignal = api.getPelicanResults.mock.calls[0][1].signal as AbortSignal
    await wrapper.setProps({ groupId: 9 })
    await flushPromises()
    resolveOld(result([{ ...run, model: 'stale-model' }]))
    await flushPromises()
    expect(oldSignal.aborted).toBe(true)
    expect(wrapper.text()).not.toContain('stale-model')
    const currentSignal = api.getPelicanResults.mock.lastCall?.[1].signal as AbortSignal
    await wrapper.setProps({ show: false })
    expect(currentSignal.aborted).toBe(true)
    wrapper.unmount()
  })
})
