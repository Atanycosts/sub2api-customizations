<template>
  <div
    class="announcement-editor"
    :class="{ 'is-dragging': imageDragActive, 'announcement-content': true }"
    data-testid="announcement-image-tools"
    @dragenter.prevent="imageDragActive = true"
    @dragover.prevent
    @dragleave.self.prevent="imageDragActive = false"
    @drop.prevent="handleDrop"
  >
    <div class="editor-toolbar">
      <div class="format-tools" :aria-label="t('admin.announcements.form.formatting.title')">
        <button
          v-for="tool in formatTools"
          :key="tool.command"
          type="button"
          class="editor-tool format-tool"
          :data-testid="`announcement-format-${tool.command}`"
          :title="tool.label"
          :aria-label="tool.label"
          :aria-pressed="activeFormats[tool.command] || undefined"
          :disabled="disabled"
          @mousedown.prevent
          @click="format(tool.command, tool.value)"
        >
          <span :class="tool.className">{{ tool.text }}</span>
        </button>
      </div>
      <div class="image-tools" :aria-label="t('admin.announcements.form.images.title')">
        <button
          type="button"
          class="editor-tool"
          data-testid="announcement-image-link-open"
          :disabled="disabled || uploading"
          @click="openImageLink"
        >
          <Icon name="link" size="sm" />{{ t('admin.announcements.form.images.insertImage') }}
        </button>
        <label
          class="editor-tool"
          data-testid="announcement-image-upload-label"
          :class="{ 'is-disabled': disabled || uploading }"
        >
          <input
            ref="imageFileInput"
            data-testid="announcement-image-file"
            type="file"
            accept="image/png,image/jpeg,image/gif,image/webp"
            multiple
            class="sr-only"
            :disabled="disabled || uploading"
            @change="handleFileInput"
          />
          <Icon name="upload" size="sm" />{{ uploading ? t('admin.announcements.form.images.uploading') : t('admin.announcements.form.images.upload') }}
        </label>
        <span class="editor-tool-hint">{{ t('admin.announcements.form.images.dropHint') }}</span>
      </div>
    </div>

    <div v-if="linkPanel" class="announcement-link-panel">
      <input
        ref="linkInput"
        v-model="linkValue"
        data-testid="announcement-text-link"
        type="url"
        class="input"
        :aria-label="t('admin.announcements.form.formatting.linkAddress')"
        :placeholder="t('admin.announcements.form.formatting.linkPlaceholder')"
        @keydown.enter.prevent="applyTextLink"
        @keydown.esc.prevent="closeLinkPanel"
      />
      <button type="button" class="btn btn-primary btn-sm" data-testid="announcement-text-link-apply" :disabled="disabled || !linkValue.trim()" @click="applyTextLink">
        {{ t('admin.announcements.form.formatting.apply') }}
      </button>
    </div>

    <div v-if="imageLinkOpen" class="announcement-link-panel">
      <input
        ref="imageLinkInput"
        v-model="imageLinkValue"
        data-testid="announcement-image-link"
        type="url"
        class="input"
        :aria-label="t('admin.announcements.form.images.link')"
        :placeholder="t('admin.announcements.form.images.linkPlaceholder')"
        @keydown.enter.prevent="insertImageLink"
        @keydown.esc.prevent="imageLinkOpen = false"
      />
      <button
        type="button"
        class="btn btn-primary btn-sm"
        data-testid="announcement-image-link-insert"
        :disabled="disabled || !imageLinkValue.trim()"
        @click="insertImageLink"
      >
        {{ t('admin.announcements.form.images.insert') }}
      </button>
    </div>

    <div v-if="selectedImage" class="announcement-image-controls" data-testid="announcement-image-controls">
      <span>{{ t('admin.announcements.form.images.selected') }}</span>
      <div role="group" :aria-label="t('admin.announcements.form.images.width')">
        <button
          v-for="width in imageWidths"
          :key="width"
          type="button"
          :data-width="width"
          :aria-pressed="selectedImage.width === width"
          :disabled="disabled"
          @click="setImageWidth(width)"
        >
          {{ width }}%
        </button>
      </div>
      <div role="group" :aria-label="t('admin.announcements.form.images.align')">
        <button
          v-for="align in imageAligns"
          :key="align"
          type="button"
          :data-align="align"
          :aria-pressed="selectedImage.align === align"
          :disabled="disabled"
          @click="setImageAlign(align)"
        >
          {{ t(`admin.announcements.form.images.${align}`) }}
        </button>
      </div>
      <button type="button" class="image-remove" data-testid="announcement-image-remove" :disabled="disabled" @click="removeSelectedImage">
        {{ t('admin.announcements.form.images.remove') }}
      </button>
    </div>

    <div
      id="announcement-content"
      ref="editorElement"
      class="announcement-visual-editor announcement-body"
      data-testid="announcement-visual-editor"
      role="textbox"
      aria-multiline="true"
      :aria-label="t('admin.announcements.form.content')"
      :aria-disabled="disabled || undefined"
      :data-placeholder="t('admin.announcements.form.contentPlaceholder')"
      :contenteditable="!disabled"
      :data-selection="selectionState"
      :data-has-content="hasContent"
      @compositionstart="composing = true"
      @compositionend="handleCompositionEnd"
      @input="handleInput"
      @keydown="handleKeydown"
      @keyup="updateActiveFormats"
      @mouseup="handleSelectionChange"
      @click="handleEditorClick"
      @paste="handlePaste"
      @focus="restoreSavedRange"
    ></div>

    <div v-if="uploading || failedUploads.length" class="announcement-image-status" role="status">
      <span v-if="uploading">{{ t('admin.announcements.form.images.uploadingCount', { count: pendingUploads.length }) }}</span>
      <span v-else>{{ t('admin.announcements.form.images.failed') }}</span>
      <button v-if="failedUploads.length" type="button" data-testid="announcement-image-retry" :disabled="disabled" @click="retryFailedUploads">
        {{ t('admin.announcements.form.images.retry') }}
      </button>
      <button
        v-for="upload in failedUploads"
        :key="upload.marker"
        type="button"
        data-testid="announcement-image-discard"
        :disabled="disabled"
        @click="discardFailedUpload(upload)"
      >
        {{ t('admin.announcements.form.images.discard', { name: upload.file.name }) }}
      </button>
    </div>
    <div v-if="imageDragActive" class="announcement-image-dropzone" data-testid="announcement-image-dropzone">
      {{ t('admin.announcements.form.images.drop') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import Icon from '@/components/icons/Icon.vue'
import {
  isSafeAnnouncementImageUrl,
  renderAnnouncementImage,
  type AnnouncementImageAlign,
  type AnnouncementImageWidth,
} from '@/utils/announcement-images'
import {
  normalizeEditorImages,
  preparePastedHtml,
  renderAnnouncementContentHtml,
  sanitizeAnnouncementEditorHtml,
} from '@/utils/announcement-content'
import '@/styles/announcement-content.css'

const props = defineProps<{
  modelValue: string
  session: number
  disabled?: boolean
}>()
const emit = defineEmits<{
  'update:modelValue': [value: string]
  dirty: []
  uploading: [value: boolean]
  'upload-failed': [value: boolean]
}>()

const { t } = useI18n()
const appStore = useAppStore()
const editorElement = ref<HTMLElement | null>(null)
const imageFileInput = ref<HTMLInputElement | null>(null)
const imageLinkInput = ref<HTMLInputElement | null>(null)
const linkInput = ref<HTMLInputElement | null>(null)
const imageLinkOpen = ref(false)
const imageLinkValue = ref('')
const linkPanel = ref(false)
const linkValue = ref('')
const composing = ref(false)
const imageDragActive = ref(false)
const selectedImage = ref<{ element: HTMLImageElement; width: AnnouncementImageWidth; align: AnnouncementImageAlign } | null>(null)
const imageWidths: AnnouncementImageWidth[] = [25, 50, 75, 100]
const imageAligns: AnnouncementImageAlign[] = ['left', 'center', 'right']
const activeFormats = ref<Record<string, boolean>>({})
const savedRange = ref<Range | null>(null)
const hasContent = ref(false)
let lastEmittedHtml = props.modelValue
let imageSequence = 0
let linkRange: Range | null = null

interface PendingUpload {
  file: File
  marker: string
  controller: AbortController
  card: HTMLElement
  generation: number
}
const pendingUploads = ref<PendingUpload[]>([])
const failedUploads = ref<PendingUpload[]>([])
const activeRuns = new Set<Promise<void>>()
const uploading = computed(() => pendingUploads.value.length > 0)

const formatTools = computed(() => [
  { command: 'bold', value: undefined, text: 'B', className: 'format-bold', label: t('admin.announcements.form.formatting.bold') },
  { command: 'formatBlock', value: 'h2', text: 'H', className: 'format-heading', label: t('admin.announcements.form.formatting.heading') },
  { command: 'insertUnorderedList', value: undefined, text: '•', className: 'format-list', label: t('admin.announcements.form.formatting.list') },
  { command: 'createLink', value: undefined, text: '↗', className: 'format-link', label: t('admin.announcements.form.formatting.link') },
])

const selectionState = computed(() => selectedImage.value ? 'image' : savedRange.value ? 'text' : 'none')

watch(uploading, value => emit('uploading', value), { immediate: true })
watch(() => failedUploads.value.length, value => emit('upload-failed', value > 0), { immediate: true })
watch(() => props.session, value => {
  void value
  loadContent(props.modelValue)
})

onMounted(() => loadContent(props.modelValue))
onBeforeUnmount(() => {
  saveSelection()
  for (const upload of pendingUploads.value) upload.controller.abort()
})

function loadContent(content: string) {
  if (!editorElement.value) return
  lastEmittedHtml = content
  editorElement.value.innerHTML = renderAnnouncementContentHtml(content, {
    editable: true,
    imageOpenLabel: t('admin.announcements.form.images.open'),
  })
  normalizeEditorImages(editorElement.value, t('admin.announcements.form.images.open'))
  const editor = editorElement.value
  hasContent.value = (editor.textContent?.trim().length ?? 0) > 0 || !!editor.querySelector('img,table')
  selectedImage.value = null
  savedRange.value = null
}

function handleInput() {
  if (composing.value || !editorElement.value) return
  const html = sanitizeAnnouncementEditorHtml(editorElement.value.innerHTML)
  lastEmittedHtml = html
  const editor = editorElement.value
  hasContent.value = (editor.textContent?.trim().length ?? 0) > 0 || !!editor.querySelector('img,table')
  emit('update:modelValue', html)
  emit('dirty')
  saveSelection()
  updateActiveFormats()
}

function handleCompositionEnd() {
  composing.value = false
  handleInput()
}

function saveSelection() {
  const selection = window.getSelection()
  if (!selection?.rangeCount || !editorElement.value) return
  const range = selection.getRangeAt(0)
  if (!editorElement.value.contains(range.commonAncestorContainer)) return
  savedRange.value = range.cloneRange()
}

function restoreSavedRange() {
  const editor = editorElement.value
  if (!editor) return
  editor.focus()
  if (!savedRange.value || !editor.contains(savedRange.value.commonAncestorContainer)) {
    const range = document.createRange()
    range.selectNodeContents(editor)
    range.collapse(false)
    savedRange.value = range
  }
  const selection = window.getSelection()
  selection?.removeAllRanges()
  selection?.addRange(savedRange.value)
}

function focusEditorAt(range: Range) {
  editorElement.value?.focus()
  const selection = window.getSelection()
  selection?.removeAllRanges()
  selection?.addRange(range)
  savedRange.value = range.cloneRange()
}

function format(command: string, value?: string) {
  if (props.disabled) return
  if (command === 'createLink') {
    linkRange = savedRange.value?.cloneRange() ?? null
    if (!linkRange && editorElement.value) {
      linkRange = document.createRange()
      linkRange.selectNodeContents(editorElement.value)
      linkRange.collapse(false)
    }
    linkPanel.value = true
    nextTick(() => linkInput.value?.focus())
    return
  }
  restoreSavedRange()
  document.execCommand('styleWithCSS', false, 'false')
  document.execCommand(command, false, value)
  handleInput()
  updateActiveFormats()
}

function applyTextLink() {
  const url = linkValue.value.trim()
  if (!isSafeTextLink(url) || !editorElement.value) {
    appStore.showError(t('admin.announcements.form.formatting.invalidLink'))
    return
  }
  if (!linkRange) return
  const selection = window.getSelection()
  selection?.removeAllRanges()
  selection?.addRange(linkRange)
  focusEditorAt(linkRange)
  document.execCommand('createLink', false, url)
  const appliedRange = window.getSelection()?.getRangeAt(0)
  for (const anchor of editorElement.value.querySelectorAll<HTMLAnchorElement>('a[href]')) {
    if (appliedRange?.intersectsNode(anchor)) {
      anchor.target = '_blank'
      anchor.rel = 'noopener noreferrer'
    }
  }
  closeLinkPanel()
  handleInput()
}

function closeLinkPanel() {
  linkPanel.value = false
  linkValue.value = ''
  linkRange = null
}

function isSafeTextLink(raw: string) {
  const value = raw.trim()
  if (value.startsWith('/') && !value.startsWith('//') && !/\s/.test(value)) return true
  try {
    const parsed = new URL(value)
    return parsed.protocol === 'https:' && !parsed.username && !parsed.password
  } catch {
    return false
  }
}

function handleSelectionChange() {
  saveSelection()
  const selection = window.getSelection()
  const node = selection?.anchorNode
  const element = node instanceof Element ? node : node?.parentElement
  const image = element?.closest('img')
  if (image && editorElement.value?.contains(image)) {
    selectImage(image)
  } else {
    selectedImage.value = null
  }
  updateActiveFormats()
}

function handleEditorClick(event: MouseEvent) {
  if (event.target instanceof HTMLImageElement) {
    event.preventDefault()
    selectImage(event.target)
    saveSelection()
    return
  }
  selectedImage.value = null
  saveSelection()
  updateActiveFormats()
}

function selectImage(image: HTMLImageElement) {
  editorElement.value?.querySelectorAll('.is-selected').forEach(element => element.classList.remove('is-selected'))
  const frame = image.closest<HTMLElement>('.announcement-image-frame') ?? image
  frame.classList.add('is-selected')
  const width = Number(frame.dataset.announcementWidth)
  selectedImage.value = {
    element: image,
    width: ([25, 50, 75, 100].includes(width) ? width : 100) as AnnouncementImageWidth,
    align: ['left', 'center', 'right'].includes(frame.dataset.announcementAlign ?? '')
      ? frame.dataset.announcementAlign as AnnouncementImageAlign
      : 'center',
  }
}

function setImageWidth(width: AnnouncementImageWidth) {
  if (!selectedImage.value) return
  selectedImage.value.element.parentElement!.dataset.announcementWidth = String(width)
  selectedImage.value.width = width
  handleInput()
}

function setImageAlign(align: AnnouncementImageAlign) {
  if (!selectedImage.value) return
  selectedImage.value.element.parentElement!.dataset.announcementAlign = align
  selectedImage.value.align = align
  handleInput()
}

function removeSelectedImage() {
  if (!selectedImage.value) return
  const range = document.createRange()
  const frame = selectedImage.value.element.parentElement!
  range.selectNode(frame)
  focusEditorAt(range)
  document.execCommand('delete')
  selectedImage.value = null
  handleInput()
}

function openImageLink() {
  imageLinkOpen.value = true
  nextTick(() => imageLinkInput.value?.focus())
}

function insertImageLink() {
  const url = imageLinkValue.value.trim()
  if (!isSafeAnnouncementImageUrl(url)) {
    appStore.showError(t('admin.announcements.form.images.invalidLink'))
    return
  }
  insertHtmlAtSelection(renderAnnouncementImage(url))
  imageLinkValue.value = ''
  imageLinkOpen.value = false
}

function handleFileInput(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files ?? [])
  input.value = ''
  queueUploads(files)
}

function handleDrop(event: DragEvent) {
  imageDragActive.value = false
  const files = Array.from(event.dataTransfer?.files ?? []).filter(file => file.type.startsWith('image/'))
  if (files.length) queueUploads(files)
}

function handlePaste(event: ClipboardEvent) {
  if (composing.value) return
  saveSelection()
  const files = Array.from(event.clipboardData?.files ?? []).filter(file => file.type.startsWith('image/'))
  if (files.length) {
    event.preventDefault()
    queueUploads(files)
    return
  }
  const html = event.clipboardData?.getData('text/html')
  const text = event.clipboardData?.getData('text/plain')
  event.preventDefault()
  if (html) {
    insertHtmlAtSelection(preparePastedHtml(html, t('admin.announcements.form.images.open')))
  } else if (text) {
    insertTextAtSelection(text)
  }
}

function queueUploads(files: File[]) {
  const accepted = files.filter(file => {
    if (!['image/png', 'image/jpeg', 'image/gif', 'image/webp'].includes(file.type)) {
      appStore.showError(t('admin.announcements.form.images.invalidFile'))
      return false
    }
    if (file.size > 8 * 1024 * 1024) {
      appStore.showError(t('admin.announcements.form.images.tooLarge'))
      return false
    }
    return true
  })
  if (!accepted.length || !editorElement.value) return
  saveSelection()
  const generation = props.session
  const markers = accepted.map(() => `announcement-upload-${++imageSequence}`)
  const html = accepted.map((file, index) =>
    `<span class="announcement-upload-card" contenteditable="false" data-upload-marker="${markers[index]}">${escapeHtml(t('admin.announcements.form.images.uploadingOne', { name: file.name }))}</span>`
  ).join('')
  restoreSavedRange()
  document.execCommand('insertHTML', false, html)
  const cards = accepted.map((file, index) => {
    const marker = markers[index]
    if (!marker) return null
    const card = editorElement.value!.querySelector<HTMLElement>(`[data-upload-marker="${marker}"]`)!
    return {
      file,
      marker,
      controller: new AbortController(),
      card,
      generation,
    }
  })
  const uploads = cards.filter((card): card is PendingUpload => !!card)
  pendingUploads.value.push(...uploads)
  const run = uploadQueued(uploads)
  activeRuns.add(run)
  void run.finally(() => activeRuns.delete(run))
  handleInput()
}

async function uploadQueued(uploads: PendingUpload[]) {
  for (const upload of uploads) {
    if (upload.controller.signal.aborted || upload.generation !== props.session) continue
    try {
      const result = await adminAPI.announcements.uploadImage(upload.file, upload.controller.signal)
      if (upload.controller.signal.aborted || upload.generation !== props.session) continue
      if (!upload.card.isConnected) continue
      const frame = document.createElement('span')
      frame.className = 'announcement-image-frame'
      frame.dataset.announcementWidth = '100'
      frame.dataset.announcementAlign = 'center'
      const image = document.createElement('img')
      image.src = result.url
      image.alt = upload.file.name.replace(/\.[^.]+$/, '')
      frame.append(image)
      upload.card.replaceWith(frame)
    } catch (error: any) {
      if (error?.name === 'AbortError' || error?.code === 'ERR_CANCELED') continue
      if (upload.generation === props.session) {
        upload.card.textContent = t('admin.announcements.form.images.failedOne', { name: upload.file.name })
        failedUploads.value.push(upload)
      }
    }
  }
  pendingUploads.value = pendingUploads.value.filter(item =>
    !uploads.some(upload => upload.marker === item.marker)
  )
  handleInput()
}

function retryFailedUploads() {
  const generation = props.session
  const retries = failedUploads.value.map(upload => {
    const card = upload.card
    card.textContent = t('admin.announcements.form.images.uploadingOne', { name: upload.file.name })
    return { ...upload, controller: new AbortController(), card, generation }
  })
  failedUploads.value = []
  pendingUploads.value.push(...retries)
  const run = uploadQueued(retries)
  activeRuns.add(run)
  void run.finally(() => activeRuns.delete(run))
}

function discardFailedUpload(upload: PendingUpload) {
  upload.card.remove()
  failedUploads.value = failedUploads.value.filter(item => item !== upload)
  handleInput()
}

function insertHtmlAtSelection(html: string) {
  restoreSavedRange()
  document.execCommand('insertHTML', false, html)
  normalizeEditorImages(editorElement.value!, t('admin.announcements.form.images.open'))
  handleInput()
}

function insertTextAtSelection(text: string) {
  restoreSavedRange()
  document.execCommand('insertText', false, text)
  handleInput()
}

function escapeHtml(value: string) {
  return value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

function updateActiveFormats() {
  if (!editorElement.value?.contains(document.activeElement) && document.activeElement !== editorElement.value) return
  activeFormats.value = {
    bold: document.queryCommandState('bold'),
    insertUnorderedList: document.queryCommandState('insertUnorderedList'),
  }
}

function handleKeydown(event: KeyboardEvent) {
  if (composing.value) return
  if (event.key === 'Escape') {
    imageLinkOpen.value = false
    closeLinkPanel()
  }
  nextTick(handleSelectionChange)
}

async function waitForUploads() {
  while (activeRuns.size) await Promise.allSettled([...activeRuns])
}

function serialize() {
  return editorElement.value ? sanitizeAnnouncementEditorHtml(editorElement.value.innerHTML) : lastEmittedHtml
}

defineExpose({ waitForUploads, serialize, hasContent, hasPendingUploads: computed(() => uploading.value), hasFailedUploads: computed(() => failedUploads.value.length > 0) })
</script>

<style scoped>
.announcement-editor { position: relative; min-width: 0; padding: 0; border: 1px solid rgb(var(--c-ink) / .28); border-radius: var(--radius-public); background: rgb(var(--c-surface)); }
.editor-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 16px; padding: 8px 10px; border-bottom: 1px solid rgb(var(--c-ink) / .13); }
.format-tools, .image-tools { display: flex; flex-wrap: wrap; align-items: center; gap: 3px; }
.image-tools { margin-left: auto; }
.editor-tool { display: inline-flex; align-items: center; justify-content: center; gap: 5px; min-width: 30px; min-height: 30px; padding: 0 6px; border-radius: var(--radius-public); font-size: 11px; color: rgb(var(--c-ink-muted)); cursor: pointer; }
.editor-tool:hover { color: rgb(var(--c-primary-600)); background: rgb(var(--c-primary-500) / .07); }
.editor-tool[aria-pressed="true"] { color: rgb(var(--c-primary-700)); background: rgb(var(--c-primary-500) / .1); }
.format-tool { border: 1px solid transparent; }
.format-bold { font-weight: 700; }
.format-heading { font-family: Georgia, serif; font-size: 14px; font-weight: 700; }
.format-list { font-size: 19px; line-height: 1; }
.format-link { font-size: 15px; }
.editor-tool.is-disabled { opacity: .55; pointer-events: none; }
.editor-tool-hint { margin-left: 6px; font-size: 10px; color: rgb(var(--c-ink-muted)); }
.announcement-link-panel { display: flex; gap: 8px; padding: 8px 10px; border-bottom: 1px solid rgb(var(--c-ink) / .13); }
.announcement-link-panel .input { min-width: 0; flex: 1; min-height: 34px; font-size: 12px; }
.announcement-visual-editor { min-height: 330px; max-height: 540px; padding: 20px 22px; overflow-y: auto; outline: none; color: rgb(var(--c-ink-muted)); font-size: 14px; line-height: 1.9; overflow-wrap: anywhere; }
.announcement-visual-editor:focus { box-shadow: inset 0 0 0 1px rgb(var(--c-primary-500) / .55); }
.announcement-visual-editor:empty::before { content: attr(data-placeholder); color: rgb(var(--c-ink-muted) / .65); pointer-events: none; }
.announcement-visual-editor :deep(.announcement-upload-card) { display: block; margin: 14px 0; padding: 14px; border: 1px dashed rgb(var(--c-ink) / .28); background: rgb(var(--c-ink) / .035); font-size: 12px; }
.announcement-visual-editor :deep(.announcement-image-frame.is-selected) { outline: 2px solid rgb(var(--c-primary-500)); outline-offset: 3px; }
.announcement-image-controls { display: flex; flex-wrap: wrap; align-items: center; gap: 8px 14px; padding: 8px 10px; border-bottom: 1px solid rgb(var(--c-ink) / .13); font-size: 10px; color: rgb(var(--c-ink-muted)); }
.announcement-image-controls > div { display: flex; gap: 2px; }
.announcement-image-controls button { min-height: 25px; padding: 0 7px; border: 1px solid rgb(var(--c-ink) / .13); border-radius: var(--radius-public); font-size: 10px; color: rgb(var(--c-ink-muted)); }
.announcement-image-controls button[aria-pressed="true"] { border-color: rgb(var(--c-primary-600)); color: rgb(var(--c-primary-700)); background: rgb(var(--c-primary-500) / .07); }
.announcement-image-controls .image-remove { margin-left: auto; color: rgb(185 28 28); }
.announcement-image-status { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; padding: 8px 10px; border-top: 1px solid rgb(var(--c-ink) / .13); font-size: 11px; color: rgb(var(--c-ink-muted)); }
.announcement-image-status button { color: rgb(var(--c-primary-700)); text-decoration: underline; text-underline-offset: 2px; }
.announcement-image-dropzone { position: absolute; inset: 0; z-index: 2; display: grid; place-items: center; border: 1px dashed rgb(var(--c-primary-500)); border-radius: var(--radius-public); background: rgb(var(--c-surface) / .94); color: rgb(var(--c-primary-700)); font-size: 12px; pointer-events: none; }
@media (max-width: 640px) {
  .editor-toolbar { align-items: flex-start; }
  .image-tools { margin-left: 0; }
  .editor-tool-hint { display: none; }
  .announcement-visual-editor { min-height: 260px; padding: 16px; }
}
</style>
