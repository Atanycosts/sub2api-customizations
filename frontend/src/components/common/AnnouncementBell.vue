<template>
  <div>
    <button
      type="button"
      class="announcement-bell"
      :aria-label="unreadCount ? t('announcements.unreadCount', { count: unreadCount }) : t('announcements.title')"
      :aria-expanded="isModalOpen"
      aria-haspopup="dialog"
      @click="openModal"
    >
      <Icon name="bell" size="md" />
      <span v-if="unreadCount" class="bell-dot" aria-hidden="true"></span>
    </button>

    <BaseDialog
      :show="isModalOpen"
      :title="t('announcements.title')"
      width="wide"
      variant="announcement"
      :z-index="100"
      close-on-click-outside
      @close="isModalOpen = false"
    >
      <template #header-actions>
        <button v-if="unreadCount" type="button" class="mark-all" :disabled="loading || marking" @click="markAllAsRead">
          {{ t('announcements.markAllRead') }}
        </button>
      </template>
      <div class="announcement-inbox" :class="{ 'show-detail': mobileDetail }">
        <aside class="announcement-index">
          <div class="announcement-filters" :aria-label="t('announcements.title')">
            <button type="button" :aria-pressed="!unreadOnly" @click="unreadOnly = false">{{ t('announcements.all') }}</button>
            <button type="button" :aria-pressed="unreadOnly" @click="unreadOnly = true">{{ t('announcements.unread') }}</button>
          </div>
          <div v-if="loadError" class="announcement-list-state" role="alert">
            <p>{{ t('announcements.failedToLoad') }}</p>
            <button type="button" class="mark-all mt-3" @click="announcementStore.fetchAnnouncements(true)">{{ t('common.tryAgain') }}</button>
          </div>
          <p v-else-if="loading && !announcements.length" class="announcement-list-state" role="status">{{ t('common.loading') }}</p>
          <div v-else ref="listRef" class="announcement-list">
            <button
              v-for="item in visibleAnnouncements"
              :key="item.id"
              type="button"
              class="announcement-item"
              :data-announcement-id="item.id"
              :aria-current="selectedId === item.id ? 'true' : undefined"
              :aria-label="`${item.title} · ${item.read_at ? t('announcements.read') : t('announcements.unread')}`"
              @click="openDetail(item)"
            >
              <span class="item-title">{{ item.title }}<span v-if="!item.read_at" class="unread-dot" aria-hidden="true"></span></span>
              <time :datetime="item.created_at">{{ formatDateOnly(item.created_at) }}</time>
            </button>
            <p v-if="!visibleAnnouncements.length" class="announcement-list-state">{{ t(unreadOnly ? 'announcements.emptyUnread' : 'announcements.empty') }}</p>
          </div>
        </aside>
        <section class="announcement-reader" :aria-label="t('announcements.title')">
          <button type="button" class="reader-back" @click="backToList"><Icon name="arrowLeft" size="sm" />{{ t('common.back') }}</button>
          <template v-if="selectedAnnouncement">
            <div class="reader-scroll">
              <AnnouncementContent
                :announcement="selectedAnnouncement"
                :image-open-label="t('announcements.openImage')"
              />
            </div>
            <footer class="reader-footer">
              <button
                type="button"
                class="announcement-action"
                data-testid="announcement-mark-read"
                :disabled="!!selectedAnnouncement.read_at || marking || loading"
                @click="markSelectedRead"
              >
                {{ selectedAnnouncement.read_at ? t('announcements.read') : t('announcements.markRead') }}
              </button>
            </footer>
          </template>
          <p v-else class="announcement-list-state">{{ t('announcements.empty') }}</p>
        </section>
      </div>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useAnnouncementStore } from '@/stores/announcements'
import { formatDateOnly } from '@/utils/format'
import type { UserAnnouncement } from '@/types'
import BaseDialog from './BaseDialog.vue'
import AnnouncementContent from './AnnouncementContent.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()
const announcementStore = useAnnouncementStore()
const { announcements, loading, loadError, unreadCount } = storeToRefs(announcementStore)
const isModalOpen = ref(false)
const selectedId = ref<number | null>(null)
const mobileDetail = ref(false)
const unreadOnly = ref(false)
const marking = ref(false)
const listRef = ref<HTMLElement | null>(null)
const visibleAnnouncements = computed(() => announcements.value.filter(item => !unreadOnly.value || !item.read_at))
const selectedAnnouncement = computed(() => announcements.value.find(item => item.id === selectedId.value))

async function openModal() {
  selectedId.value = announcements.value[0]?.id ?? null
  mobileDetail.value = false
  isModalOpen.value = true
  await announcementStore.fetchAnnouncements(true)
  if (!announcements.value.some(item => item.id === selectedId.value)) selectedId.value = announcements.value[0]?.id ?? null
}

async function backToList() {
  mobileDetail.value = false
  await nextTick()
  const button = listRef.value?.querySelector<HTMLButtonElement>(`[data-announcement-id="${selectedId.value}"]`)
    ?? listRef.value?.querySelector<HTMLButtonElement>('button')
  button?.focus()
}

async function openDetail(item: UserAnnouncement) {
  selectedId.value = item.id
  mobileDetail.value = true
  await nextTick()
  if (window.matchMedia('(max-width: 640px)').matches) {
    document.querySelector<HTMLButtonElement>('.reader-back')?.focus()
  }
}

async function markSelectedRead() {
  if (!selectedAnnouncement.value) return
  marking.value = true
  const success = await announcementStore.markAsRead(selectedAnnouncement.value.id)
  marking.value = false
  if (success) appStore.showSuccess(t('announcements.markedAsRead'))
  else appStore.showError(t('announcements.failedToMarkRead'))
}

async function markAllAsRead() {
  try {
    await announcementStore.markAllAsRead()
    appStore.showSuccess(t('announcements.allMarkedAsRead'))
  } catch {
    appStore.showError(t('announcements.failedToMarkRead'))
  }
}
</script>

<style scoped>
.announcement-bell { position: relative; display: grid; place-items: center; width: 36px; height: 36px; border-radius: var(--radius-control); color: rgb(var(--c-ink-muted)); }
.announcement-bell:hover { color: rgb(var(--c-primary-600)); background: rgb(var(--c-ink) / .04); }
.announcement-bell:focus-visible, .announcement-inbox button:focus-visible, .mark-all:focus-visible { outline: 2px solid rgb(var(--c-primary-500)); outline-offset: -3px; }
.bell-dot { position: absolute; top: 5px; right: 6px; width: 5px; height: 5px; border-radius: 50%; background: rgb(var(--c-primary-500)); }
.mark-all { font-size: 11px; color: rgb(var(--c-ink-muted)); }
.mark-all:hover { color: rgb(var(--c-primary-600)); }
.mark-all:disabled { opacity: .5; }
.announcement-inbox { display: grid; grid-template-columns: 238px minmax(0, 1fr); height: min(560px, 68dvh); min-height: 0; }
.announcement-index { display: flex; flex-direction: column; border-right: 1px solid rgb(var(--c-ink) / .13); min-height: 0; }
.announcement-filters { display: flex; gap: 22px; height: 53px; margin: 0 22px; border-bottom: 1px solid rgb(var(--c-ink) / .13); flex-shrink: 0; }
.announcement-filters button { position: relative; font-size: 11px; color: rgb(var(--c-ink-muted)); }
.announcement-filters button[aria-pressed="true"] { color: rgb(var(--c-ink)); }
.announcement-filters button[aria-pressed="true"]::after { content: ''; position: absolute; left: 0; right: 0; bottom: -1px; height: 1px; background: rgb(var(--c-primary-500)); }
.announcement-list { padding: 8px 0; overflow-y: auto; }
.announcement-item { display: flex; flex-direction: column; gap: 10px; width: 100%; padding: 20px 22px; border-left: 2px solid transparent; text-align: left; }
.announcement-item:hover, .announcement-item[aria-current="true"] { background: rgb(var(--c-primary-500) / .05); }
.announcement-item[aria-current="true"] { border-color: rgb(var(--c-primary-500)); }
.item-title { display: flex; align-items: center; justify-content: space-between; gap: 10px; width: 100%; overflow-wrap: anywhere; font-size: 13px; color: rgb(var(--c-ink)); }
.announcement-item[aria-current="true"] .item-title { color: rgb(var(--c-primary-600)); }
.dark .announcement-item[aria-current="true"] .item-title { color: rgb(var(--c-primary-400)); }
.announcement-item time { font: 10px ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; color: rgb(var(--c-ink-muted)); }
.unread-dot { width: 4px; height: 4px; border-radius: 50%; flex: none; background: rgb(var(--c-primary-500)); }
.announcement-reader { display: flex; flex-direction: column; min-width: 0; min-height: 0; }
.reader-scroll { flex: 1; min-height: 0; overflow-y: auto; }
.reader-footer { display: flex; justify-content: flex-end; padding: 16px 24px; border-top: 1px solid rgb(var(--c-ink) / .13); flex-shrink: 0; }
.announcement-action { min-height: 40px; padding: 0 20px; border: 1px solid var(--cx-action-bg); border-radius: var(--radius-public); background: var(--cx-action-bg); color: var(--cx-action-ink); font-size: 12px; }
.announcement-action:disabled { background: transparent; border-color: rgb(var(--c-ink) / .13); color: rgb(var(--c-ink-muted)); }
.announcement-list-state { padding: 48px 22px; font-size: 12px; color: rgb(var(--c-ink-muted)); text-align: center; }
.reader-back { display: none; }
@media (max-width: 640px) {
  .announcement-inbox { display: block; height: min(650px, 74dvh); }
  .announcement-index, .announcement-reader { height: 100%; border: 0; }
  .announcement-inbox:not(.show-detail) .announcement-reader, .announcement-inbox.show-detail .announcement-index { display: none; }
  .reader-back { display: flex; align-items: center; gap: 7px; padding: 13px 22px 0; color: rgb(var(--c-ink-muted)); font-size: 11px; }
}
</style>
