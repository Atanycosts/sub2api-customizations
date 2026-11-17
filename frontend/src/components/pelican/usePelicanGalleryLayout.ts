import { computed, type Ref } from 'vue'
import { useElementSize } from '@vueuse/core'

export function pelicanGalleryLayout(width: number, height: number, listMode = false) {
  const mobile = width <= 620
  const compact = mobile || listMode
  let columns = 1
  if (!compact) {
    if (width >= 1400) columns = 6
    else if (width >= 980) columns = 4
    else if (width >= 700) columns = 3
    else columns = 2
  }

  let rows = height >= 420 ? 2 : 1
  if (mobile) rows = Math.max(1, Math.min(width <= 350 ? 3 : 4, Math.floor((height + 8) / 108)))
  else if (listMode) rows = Math.max(1, Math.min(8, Math.floor((height + 12) / 120)))
  return { compact, columns, rows, pageSize: columns * rows }
}

export function usePelicanGalleryLayout(element: Ref<HTMLElement | null>, listMode: Ref<boolean>) {
  const { width, height } = useElementSize(element)
  // A desktop-sized first request is replaced once the mounted container is measured.
  return computed(() =>
    pelicanGalleryLayout(width.value || 1100, height.value || 540, listMode.value)
  )
}
