import { onBeforeUnmount, onMounted } from 'vue'

export function usePelicanRefresh(refresh: () => void, interval: () => number | null) {
  let timer: number | undefined
  let mounted = false

  function clear() {
    window.clearTimeout(timer)
    timer = undefined
  }

  function schedule(immediate = false) {
    clear()
    const delay = interval()
    if (mounted && delay !== null && document.visibilityState === 'visible') {
      timer = window.setTimeout(refresh, immediate ? 0 : delay)
    }
  }

  function onVisibilityChange() {
    schedule(true)
  }

  onMounted(() => {
    mounted = true
    document.addEventListener('visibilitychange', onVisibilityChange)
  })
  onBeforeUnmount(() => {
    mounted = false
    clear()
    document.removeEventListener('visibilitychange', onVisibilityChange)
  })

  return { clear, schedule }
}
