import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAnnouncementStore } from '../announcements'

const { list, markRead } = vi.hoisted(() => ({ list: vi.fn(), markRead: vi.fn() }))
vi.mock('@/api', () => ({ announcementsAPI: { list, markRead } }))
const notice = (id: number) => ({ id, title: `公告 ${id}`, content: '后台原文', notify_mode: 'popup' as const, created_at: '2026-11-09T00:00:00Z', updated_at: '2026-11-09T00:00:00Z' })

beforeEach(() => { setActivePinia(createPinia()); vi.clearAllMocks() })

describe('announcement popup confirmation', () => {
  it('retains the popup when marking fails and advances only after a successful retry', async () => {
    const store = useAnnouncementStore()
    list.mockResolvedValue([notice(1), notice(2)])
    await store.fetchAnnouncements()
    const log = vi.spyOn(console, 'error').mockImplementation(() => {})
    markRead.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ message: 'ok' })
    expect(await store.dismissPopup()).toBe(false)
    expect(store.currentPopup?.id).toBe(1)
    expect(store.unreadCount).toBe(2)
    expect(await store.dismissPopup()).toBe(true)
    expect(store.currentPopup?.id).toBe(2)
    expect(store.unreadCount).toBe(1)
    log.mockRestore()
  })

  it('can close without marking read and does not restore an old session popup', async () => {
    const store = useAnnouncementStore()
    list.mockResolvedValue([notice(1), notice(2)])
    await store.fetchAnnouncements()
    store.deferPopup()
    expect(store.currentPopup?.id).toBe(2)
    expect(markRead).not.toHaveBeenCalled()
    let finish!: () => void
    markRead.mockReturnValue(new Promise<void>(resolve => { finish = resolve }))
    const request = store.dismissPopup()
    store.reset()
    finish()
    expect(await request).toBe(false)
    expect(store.currentPopup).toBeNull()
    expect(store.announcements).toEqual([])
  })
})
