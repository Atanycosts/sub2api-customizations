<template>
  <article class="announcement-content">
    <time v-if="announcement.created_at" class="announcement-date" :datetime="announcement.created_at">
      {{ formatDateOnly(announcement.created_at) }}
    </time>
    <h2 class="announcement-title">{{ announcement.title }}</h2>
    <div
      class="announcement-body"
      :class="{ 'is-editable': editable }"
      v-html="renderedContent"
      @click="handleContentClick"
    ></div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Announcement } from '@/types'
import { formatDateOnly } from '@/utils/format'
import { renderAnnouncementContentHtml } from '@/utils/announcement-content'
import '@/styles/announcement-content.css'

const props = defineProps<{
  announcement: Pick<Announcement, 'title' | 'content'> & Partial<Pick<Announcement, 'created_at'>>
  imageOpenLabel: string
  editable?: boolean
}>()
const emit = defineEmits<{ selectImage: [src: string] }>()

const renderedContent = computed(() => {
  return renderAnnouncementContentHtml(props.announcement.content, {
    editable: !!props.editable,
    imageOpenLabel: props.imageOpenLabel,
  })
})

function handleContentClick(event: MouseEvent) {
  if (!props.editable) return
  const image = (event.target as HTMLElement).closest<HTMLImageElement>('img')
  if (!image) return
  event.preventDefault()
  emit('selectImage', image.getAttribute('src') ?? '')
}

</script>
