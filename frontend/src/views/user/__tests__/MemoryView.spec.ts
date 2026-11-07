import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import MemoryView from '../MemoryView.vue'
import type { AccountMemoryOverview } from '@/api/memory'

const { overview } = vi.hoisted(() => ({ overview: vi.fn() }))
vi.mock('@/api/memory', () => ({ memoryAPI: { overview } }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/utils/format', () => ({ formatDate: (value: string) => value }))

const active: AccountMemoryOverview = {
  configured: true, state: 'active', runtime_health_checked: false,
  supported_endpoint: 'responses_http_text', archive_mode: 'source_diary',
  archived: 12, pending: 1, attention: 1,
  recent: [{ event_id: 'own-record', state: 'needs_reconciliation', created_at: '2026-11-07T00:00:00Z', updated_at: '2026-11-07T00:01:00Z' }]
}
const wrappers: ReturnType<typeof mount>[] = []
function render() {
  const wrapper = mount(MemoryView, { global: { stubs: { Icon: true } } })
  wrappers.push(wrapper)
  return wrapper
}
beforeEach(() => { vi.clearAllMocks(); overview.mockResolvedValue(active) })
afterEach(() => wrappers.splice(0).forEach(wrapper => wrapper.unmount()))

describe('account memory', () => {
  it('shows receipt status and limited coverage without claiming Runtime health', async () => {
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('accountMemory.active')
    expect(wrapper.text()).toContain('accountMemory.coverage')
    expect(wrapper.text()).toContain('own-record')
    expect(wrapper.text()).toContain('accountMemory.states.needs_reconciliation')
    expect(wrapper.findAll('dd').map(item => item.text())).toEqual(['12', '1', '1'])
    expect(wrapper.find('input').exists()).toBe(false)
  })

  it.each(['unconfigured', 'suspended'] as const)('keeps %s concise and hides archive metrics', async state => {
    overview.mockResolvedValue({ ...active, state, configured: state === 'suspended', recent: [] })
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain(`accountMemory.${state}Hint`)
    expect(wrapper.find('dl').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('accountMemory.empty')
  })

  it('hides stale receipts on refresh failure and recovers with retry', async () => {
    const wrapper = render()
    await flushPromises()
    overview.mockRejectedValueOnce(new Error('server unavailable'))
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('accountMemory.loadFailed')
    expect(wrapper.text()).not.toContain('own-record')
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('own-record')
  })
})
