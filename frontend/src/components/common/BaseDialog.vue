<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="show"
        class="modal-overlay"
        :class="{ 'announcement-dialog': variant === 'announcement' }"
        :style="zIndexStyle"
        :aria-labelledby="dialogId"
        role="dialog"
        aria-modal="true"
        @click.self="handleClose"
      >
        <!-- Modal panel -->
        <div ref="dialogRef" :class="['modal-content', widthClasses]" tabindex="-1" @click.stop>
          <!-- Header -->
          <div class="modal-header">
            <h3 :id="dialogId" class="modal-title">
              {{ title }}
            </h3>
            <div class="flex items-center gap-4">
              <slot name="header-actions"></slot>
              <button
                v-if="showCloseButton"
                @click="emit('close')"
                class="-mr-2 rounded-xl p-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 focus-visible:ring-offset-2 dark:text-dark-500 dark:hover:bg-dark-700 dark:hover:text-dark-300 dark:focus-visible:ring-offset-dark-900"
                aria-label="Close modal"
              >
                <Icon name="x" size="md" />
              </button>
            </div>
          </div>

          <!-- Body -->
          <div ref="modalBodyRef" class="modal-body">
            <slot></slot>
          </div>

          <!-- Footer -->
          <div v-if="$slots.footer" class="modal-footer">
            <slot name="footer"></slot>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script lang="ts">
let dialogIdCounter = 0
const openDialogs = new Set<string>()
</script>

<script setup lang="ts">
import { computed, watch, onMounted, onUnmounted, ref, nextTick } from 'vue'
import Icon from '@/components/icons/Icon.vue'

// 生成唯一ID以避免多个对话框时ID冲突
const dialogId = `modal-title-${++dialogIdCounter}`

// 焦点管理
const dialogRef = ref<HTMLElement | null>(null)
const modalBodyRef = ref<HTMLElement | null>(null)
let previousActiveElement: HTMLElement | null = null

type DialogWidth = 'narrow' | 'normal' | 'wide' | 'extra-wide' | 'full'

interface Props {
  show: boolean
  title: string
  width?: DialogWidth
  closeOnEscape?: boolean
  closeOnClickOutside?: boolean
  showCloseButton?: boolean
  zIndex?: number
  variant?: 'default' | 'announcement'
}

interface Emits {
  (e: 'close'): void
}

const props = withDefaults(defineProps<Props>(), {
  width: 'normal',
  closeOnEscape: true,
  closeOnClickOutside: false,
  showCloseButton: true,
  zIndex: 50,
  variant: 'default'
})

const emit = defineEmits<Emits>()

// Custom z-index style (overrides the default z-50 from CSS)
const zIndexStyle = computed(() => {
  return props.zIndex !== 50 ? { zIndex: props.zIndex } : undefined
})

const widthClasses = computed(() => {
  // Width guidance: narrow=confirm/short prompts, normal=standard forms,
  // wide=multi-section forms or rich content, extra-wide=analytics/tables,
  // full=full-screen or very dense layouts.
  const widths: Record<DialogWidth, string> = {
    narrow: 'max-w-md',
    normal: 'max-w-lg',
    wide: 'w-full sm:max-w-2xl md:max-w-3xl lg:max-w-4xl',
    'extra-wide': 'w-full sm:max-w-3xl md:max-w-4xl lg:max-w-5xl xl:max-w-6xl',
    full: 'w-full sm:max-w-4xl md:max-w-5xl lg:max-w-6xl xl:max-w-7xl'
  }
  return widths[props.width]
})

const handleClose = () => {
  if (props.closeOnClickOutside) {
    emit('close')
  }
}

const focusableElements = () => {
  const elements = dialogRef.value?.querySelectorAll<HTMLElement>(
    'button, a[href], input:not([type="hidden"]), select, textarea, [tabindex], [contenteditable="true"]'
  )
  return Array.from(elements ?? []).filter((element) => {
    if (element.tabIndex < 0 || element.matches(':disabled') ||
      element.closest('[hidden], [inert], [aria-hidden="true"]')) return false
    for (let current: HTMLElement | null = element; current; current = current.parentElement) {
      const style = window.getComputedStyle(current)
      if (style.display === 'none' || style.visibility === 'hidden') return false
      if (current === dialogRef.value) break
    }
    return true
  })
}

const handleKeydown = (event: KeyboardEvent) => {
  if (event.defaultPrevented || !props.show || [...openDialogs].pop() !== dialogId) return
  if (props.closeOnEscape && event.key === 'Escape') {
    emit('close')
  }
  if (event.key !== 'Tab') return

  const elements = focusableElements()
  const first = elements[0]
  const last = elements.at(-1)
  const active = document.activeElement
  if (!first) {
    event.preventDefault()
    dialogRef.value?.focus()
  } else if (!elements.includes(active as HTMLElement) ||
    (event.shiftKey ? active === first : active === last)) {
    event.preventDefault()
    ;(event.shiftKey ? last : first)?.focus()
  }
}

const restoreFocus = () => {
  if (previousActiveElement?.isConnected) previousActiveElement.focus()
  previousActiveElement = null
}

const updateScrollLock = (isOpen: boolean) => {
  if (isOpen) openDialogs.add(dialogId)
  else openDialogs.delete(dialogId)
  document.body.classList.toggle('modal-open', openDialogs.size > 0)
}

// Prevent body scroll when modal is open and manage focus
watch(
  () => props.show,
  async (isOpen) => {
    if (isOpen) {
      // 保存当前焦点元素
      previousActiveElement = document.activeElement as HTMLElement
      // 使用CSS类而不是直接操作style,更易于管理多个对话框
      updateScrollLock(true)

      // 等待DOM更新后设置焦点到对话框
      await nextTick()
      if (!props.show || [...openDialogs].pop() !== dialogId) return
      if (modalBodyRef.value) {
        modalBodyRef.value.scrollTop = 0
      }
      const initialFocus = focusableElements()[0] ?? dialogRef.value
      initialFocus?.focus()
    } else {
      const wasTopmost = [...openDialogs].pop() === dialogId
      updateScrollLock(false)
      // 恢复之前的焦点
      if (wasTopmost) restoreFocus()
      else previousActiveElement = null
    }
  },
  { immediate: true }
)

onMounted(() => {
  document.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown)
  // 确保组件卸载时移除滚动锁定
  const wasTopmost = [...openDialogs].pop() === dialogId
  updateScrollLock(false)
  if (wasTopmost) restoreFocus()
})
</script>

<style scoped>
.announcement-dialog .modal-content {
  background: rgb(var(--c-surface));
  border: 1px solid rgb(var(--c-ink) / .28);
  border-radius: var(--radius-public);
  box-shadow: 0 18px 64px rgb(0 0 0 / .12);
  min-height: 0;
}
.announcement-dialog .modal-header,
.announcement-dialog .modal-footer {
  border-color: rgb(var(--c-ink) / .13);
  padding: 18px 24px;
}
.announcement-dialog .modal-title { font-size: 15px; font-weight: 500; color: rgb(var(--c-ink)); }
.announcement-dialog .modal-header button { border-radius: var(--radius-public); }
.announcement-dialog .modal-body { padding: 0; min-height: 0; }
@media (max-width: 640px) {
  .announcement-dialog .modal-header,
  .announcement-dialog .modal-footer { padding: 14px 20px; }
}
</style>
