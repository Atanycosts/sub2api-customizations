<template>
  <BaseDialog
    :show="!!displayedAnnouncement"
    :title="t('announcements.title')"
    variant="announcement"
    width="normal"
    :z-index="120"
    close-on-click-outside
    @close="close"
  >
    <AnnouncementContent
      v-if="displayedAnnouncement"
      :announcement="displayedAnnouncement"
      :image-open-label="t('announcements.openImage')"
    />
    <template #footer>
      <button
        type="button"
        class="popup-action"
        :disabled="saving"
        data-testid="announcement-popup-dismiss"
        @click="handleDismiss"
      >
        {{ preview ? t('common.close') : t('announcements.markRead') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useAnnouncementStore } from '@/stores/announcements'
import type { Announcement } from '@/types'
import AnnouncementContent from './AnnouncementContent.vue'
import BaseDialog from './BaseDialog.vue'

type PreviewAnnouncement = Pick<Announcement, 'title' | 'content'> & Partial<Pick<Announcement, 'created_at'>>
const props = withDefaults(defineProps<{
  announcement?: PreviewAnnouncement | null
  preview?: boolean
}>(), { announcement: null, preview: false })
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const appStore = useAppStore()
const announcementStore = useAnnouncementStore()
const saving = ref(false)
const displayedAnnouncement = computed(() => props.preview ? props.announcement : announcementStore.currentPopup)

function close() {
  if (saving.value) return
  if (props.preview) emit('close')
  else announcementStore.deferPopup()
}

async function handleDismiss() {
  if (props.preview) { emit('close'); return }
  saving.value = true
  const success = await announcementStore.dismissPopup()
  saving.value = false
  if (!success) appStore.showError(t('announcements.failedToMarkRead'))
}
</script>

<style scoped>
.popup-action { min-height: 40px; padding: 0 20px; background: var(--cx-action-bg); color: var(--cx-action-ink); border: 1px solid var(--cx-action-bg); border-radius: var(--radius-public); font-size: 12px; }
.popup-action:focus-visible { outline: 2px solid rgb(var(--c-primary-500)); outline-offset: 3px; }
.popup-action:disabled { opacity: .6; }
</style>
