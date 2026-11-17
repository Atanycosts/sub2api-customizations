import type { PelicanRunState } from '@/types/pelican'

export function formatPelicanDuration(durationMs: number): string {
  if (!Number.isFinite(durationMs) || durationMs <= 0) return '—'
  if (durationMs < 1000) return `${Math.round(durationMs)} ms`
  return `${(durationMs / 1000).toFixed(durationMs < 10_000 ? 1 : 0)} s`
}

export function formatPelicanFreshness(value: string, now = Date.now()): string {
  const timestamp = Date.parse(value)
  if (!Number.isFinite(timestamp)) return value
  const seconds = Math.round((timestamp - now) / 1000)
  const abs = Math.abs(seconds)
  if (abs < 60) return `${Math.max(1, abs)}s`
  if (abs < 3600) return `${Math.round(abs / 60)}m`
  if (abs < 86400) return `${Math.round(abs / 3600)}h`
  return `${Math.round(abs / 86400)}d`
}

export function pelicanStateClass(state: PelicanRunState): string {
  switch (state) {
    case 'completed':
      return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/70 dark:bg-emerald-950/40 dark:text-emerald-300'
    case 'failed':
      return 'border-rose-200 bg-rose-50 text-rose-700 dark:border-rose-900/70 dark:bg-rose-950/40 dark:text-rose-300'
    case 'unknown':
      return 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900/70 dark:bg-amber-950/40 dark:text-amber-300'
    default:
      return 'border-sky-200 bg-sky-50 text-sky-700 dark:border-sky-900/70 dark:bg-sky-950/40 dark:text-sky-300'
  }
}
