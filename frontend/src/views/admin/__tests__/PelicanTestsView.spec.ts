import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import PelicanTestsView from '../PelicanTestsView.vue'
import type {
  PelicanAdminRun,
  PelicanAdminPage,
  PelicanConfig,
  PelicanPlan,
  PelicanPlanInput,
} from '@/types/pelican'

const api = vi.hoisted(() => ({
  deletePelicanPlan: vi.fn(),
  getPelicanAdminResults: vi.fn(),
  getPelicanConfig: vi.fn(),
  getPelicanPlans: vi.fn(),
  getPelicanResults: vi.fn(),
  runPelicanPlan: vi.fn(),
  savePelicanConfig: vi.fn(),
  savePelicanPlan: vi.fn(),
  groupsGetAll: vi.fn(),
}))

vi.mock('@/api/pelican', () => ({
  deletePelicanPlan: api.deletePelicanPlan,
  getPelicanAdminResults: api.getPelicanAdminResults,
  getPelicanConfig: api.getPelicanConfig,
  getPelicanPlans: api.getPelicanPlans,
  getPelicanResults: api.getPelicanResults,
  runPelicanPlan: api.runPelicanPlan,
  savePelicanConfig: api.savePelicanConfig,
  savePelicanPlan: api.savePelicanPlan,
}))
vi.mock('@/api/admin', () => ({
  adminAPI: { groups: { getAll: api.groupsGetAll } },
}))
vi.mock('@/utils/operationsUUID', () => ({ operationsUUID: () => 'stable-submit-id' }))
vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: (_error: unknown, fallback: string) => fallback,
}))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))

const plan: PelicanPlan = {
  id: 9,
  group_id: 10,
  group_name: 'OpenAI API',
  model: 'custom-model',
  effort: 'high',
  protocol: 'responses',
  enabled: true,
  published: false,
  interval_minutes: 60,
  max_output_tokens: 8192,
  budget_usd: 1.25,
  last_run_at: null,
  next_run_at: '2026-11-18T12:00:00Z',
  running_until: null,
}

let currentPlan: PelicanPlan

const adminRun: PelicanAdminRun = {
  id: 'run-1',
  source: 'group',
  group_id: 10,
  group_name: 'Result Group',
  model: 'custom-model',
  effort: 'high',
  protocol: 'responses',
  prompt_version: 'pelican-bicycle-v1',
  max_output_tokens: 8192,
  state: 'completed',
  created_at: '2026-11-18T12:00:00Z',
  finished_at: '2026-11-18T12:00:32Z',
  duration_ms: 32000,
  terminal: 'stop',
  error_kind: '',
  output: '',
  plan_id: 9,
  requested_by: 99,
  frozen: plan,
  published: true,
  account_id: 800,
  upstream_model: 'custom-model',
  endpoint: '/v1/responses',
  raw_response: null,
  usage: null,
  error: '',
  cost_usd: 0.032,
  reserved_usd: 0.5,
  http_status: 200,
}

const resultPage: PelicanAdminPage = {
  items: [],
  total: 0,
  page: 1,
  page_size: 5,
  pages: 0,
  stats: { completed: 0, failed: 0, unknown: 0, running: 0 },
}

const savedPlan = (id: number | null, input: PelicanPlanInput): PelicanPlan => ({
  ...plan,
  ...input,
  id: id ?? plan.id,
})

const CompactPagerStub = {
  name: 'PelicanCompactPager',
  props: ['page', 'total', 'pageSize'],
  emits: ['update:page'],
  template: '<div class="compact-pager">{{ page }}</div>',
}

const stubs = {
  AppLayout: {
    props: ['viewport'],
    template: '<div><slot /></div>',
  },
  Icon: true,
  Select: {
    props: ['modelValue', 'options'],
    emits: ['update:modelValue', 'change'],
    template:
      '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value); $emit(\'change\')"><option value="">all</option><option value="failed">failed</option></select>',
  },
  PelicanCompactPager: CompactPagerStub,
  PelicanPlanDialog: true,
  PelicanAdminResultDialog: true,
  PelicanRulesDialog: true,
  RouterLink: {
    template: '<a><slot /></a>',
  },
}

const mountView = () => mount(PelicanTestsView, { global: { stubs } })

function buttonByTitle(wrapper: ReturnType<typeof mountView>, title: string) {
  const button = wrapper.findAll('button').find((item) => item.attributes('title') === title)
  if (!button) throw new Error(`Button not found: ${title}`)
  return button
}

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  })
  api.deletePelicanPlan.mockReset()
  api.getPelicanAdminResults.mockReset().mockResolvedValue(resultPage)
  api.getPelicanConfig.mockReset().mockResolvedValue({ enabled: false, daily_budget_usd: 0 })
  currentPlan = { ...plan }
  api.getPelicanPlans.mockReset().mockImplementation(async () => [{ ...currentPlan }])
  api.getPelicanResults.mockReset().mockResolvedValue({
    prompt: 'Server-owned fixed prompt',
    prompt_version: 'pelican-bicycle-v1',
  })
  api.runPelicanPlan.mockReset().mockResolvedValue({})
  api.savePelicanConfig.mockReset().mockImplementation(
    async (input: PelicanConfig): Promise<PelicanConfig> => input,
  )
  api.savePelicanPlan.mockReset().mockImplementation(async (id: number | null, input: PelicanPlanInput) => {
    currentPlan = savedPlan(id, input)
    return { ...currentPlan }
  })
  api.groupsGetAll.mockReset().mockResolvedValue([
    { id: 10, name: 'OpenAI API', platform: 'openai', status: 'active' },
  ])
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('Pelican admin view', () => {
  it('loads the server-owned fixed prompt when opening test rules', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(api.getPelicanResults).not.toHaveBeenCalled()

    await wrapper.get('button[aria-label="pelican.rules.title"]').trigger('click')
    await flushPromises()

    expect(api.getPelicanResults).toHaveBeenCalledWith({ page: 1, page_size: 1 })
    expect(wrapper.getComponent({ name: 'PelicanRulesDialog' }).attributes('prompt'))
      .toBe('Server-owned fixed prompt')
    wrapper.unmount()
  })

  it('gates manual runs on saved config and sends complete inputs for sequential plan toggles', async () => {
    const wrapper = mountView()
    await flushPromises()

    const runButton = () =>
      buttonByTitle(wrapper, wrapper.find('[title="pelican.admin.plans.enableBeforeRun"]').exists()
        ? 'pelican.admin.plans.enableBeforeRun'
        : 'pelican.admin.plans.runNow')

    expect(runButton().attributes('disabled')).toBeDefined()
    await wrapper.get('.pelican-config [role="switch"]').trigger('click')
    await wrapper.get('.pelican-config input[type="number"]').setValue('5')
    expect(runButton().attributes('disabled')).toBeDefined()
    expect(api.savePelicanConfig).not.toHaveBeenCalled()

    await wrapper.get('.pelican-config').trigger('submit')
    await flushPromises()
    expect(api.savePelicanConfig).toHaveBeenCalledWith({ enabled: true, daily_budget_usd: 5 })
    expect(runButton().attributes('disabled')).toBeUndefined()

    await runButton().trigger('click')
    await flushPromises()
    expect(api.runPelicanPlan).toHaveBeenCalledWith(9, 'stable-submit-id')

    await wrapper.get('button[aria-label*="pelican.admin.plans.scheduleEnabled"]').trigger('click')
    await flushPromises()
    expect(api.savePelicanPlan.mock.calls[0]).toEqual([
      9,
      {
        group_id: 10,
        model: 'custom-model',
        effort: 'high',
        protocol: 'responses',
        enabled: false,
        published: false,
        interval_minutes: 60,
        max_output_tokens: 8192,
        budget_usd: 1.25,
      },
    ])

    await wrapper.get('button[aria-label*="pelican.admin.plans.published"]').trigger('click')
    await flushPromises()
    expect(api.savePelicanPlan.mock.calls[1]).toEqual([
      9,
      {
        group_id: 10,
        model: 'custom-model',
        effort: 'high',
        protocol: 'responses',
        enabled: false,
        published: true,
        interval_minutes: 60,
        max_output_tokens: 8192,
        budget_usd: 1.25,
      },
    ])
    wrapper.unmount()
  })

  it('hides old result rows when an explicit filter request fails', async () => {
    api.getPelicanAdminResults
      .mockResolvedValueOnce({
        ...resultPage,
        items: [adminRun],
        total: 1,
        pages: 1,
        stats: { ...resultPage.stats, completed: 1 },
      })
      .mockRejectedValueOnce(new Error('filter failed'))
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('Result Group')

    await wrapper.findAll('select')[1].setValue('failed')
    await flushPromises()

    expect(api.getPelicanAdminResults.mock.lastCall?.[0]).toMatchObject({ state: 'failed' })
    expect(wrapper.text()).not.toContain('Result Group')
    expect(wrapper.text()).toContain('pelican.admin.results.loadFailed')
    wrapper.unmount()
  })

  it('clamps a shrunken result total to the last page and refetches it', async () => {
    api.getPelicanAdminResults
      .mockResolvedValueOnce({
        ...resultPage,
        items: [adminRun],
        total: 12,
        pages: 3,
        stats: { ...resultPage.stats, completed: 12 },
      })
      .mockResolvedValueOnce({ ...resultPage, page: 3, total: 1, pages: 1 })
      .mockResolvedValueOnce({
        ...resultPage,
        items: [adminRun],
        total: 1,
        pages: 1,
        stats: { ...resultPage.stats, completed: 1 },
      })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.findAllComponents(CompactPagerStub)[1].vm.$emit('update:page', 3)
    await flushPromises()

    expect(api.getPelicanAdminResults.mock.calls.map((call) => call[0].page)).toEqual([1, 3, 1])
    expect(wrapper.findAllComponents(CompactPagerStub)[1].props('page')).toBe(1)
    expect(wrapper.text()).toContain('Result Group')
    wrapper.unmount()
  })

  it('ignores an in-flight plan list read that started before a flag mutation', async () => {
    vi.useFakeTimers()
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    const runningResultPage = {
      ...resultPage,
      items: [{ ...adminRun, state: 'running' as const }],
      total: 1,
      pages: 1,
      stats: { ...resultPage.stats, running: 1 },
    }
    api.getPelicanAdminResults.mockResolvedValue(runningResultPage)
    let resolveStale!: (value: PelicanPlan[]) => void
    api.getPelicanPlans
      .mockResolvedValueOnce([{ ...plan }])
      .mockReturnValueOnce(new Promise((resolve) => {
        resolveStale = resolve
      }))
      .mockImplementation(async () => [{ ...currentPlan }])

    const wrapper = mountView()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(10_000)
    await flushPromises()
    expect(api.getPelicanPlans).toHaveBeenCalledTimes(2)

    await wrapper.get('button[aria-label*="pelican.admin.plans.scheduleEnabled"]').trigger('click')
    await flushPromises()
    expect(currentPlan.enabled).toBe(false)

    resolveStale([{ ...plan, enabled: true }])
    await flushPromises()
    expect(
      wrapper.get('button[aria-label*="pelican.admin.plans.scheduleEnabled"]').attributes('aria-checked'),
    ).toBe('false')
    wrapper.unmount()
  })

  it('discovers a scheduled completion without pressing Run or Refresh', async () => {
    vi.useFakeTimers()
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).not.toContain('Result Group')
    currentPlan = { ...currentPlan, last_run_at: adminRun.created_at }
    api.getPelicanAdminResults.mockResolvedValueOnce({
      ...resultPage,
      items: [{ ...adminRun, requested_by: 0 }],
      total: 1,
      pages: 1,
      stats: { ...resultPage.stats, completed: 1 },
    })
    await vi.advanceTimersByTimeAsync(30_000)
    expect(api.getPelicanPlans).toHaveBeenCalledTimes(2)
    expect(api.runPelicanPlan).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('Result Group')
    wrapper.unmount()
  })
})
