import type { RequestTiming } from '@/types/requestTimings'

export function requestTimingMetrics(trace: RequestTiming) {
  const spans = trace.admission.map(span => [span.start_ms, span.end_ms] as const).sort((a, b) => a[0] - b[0])
  let measuredWait = 0
  let start: number | null = null
  let end = 0
  for (const span of spans) {
    if (start === null) { start = span[0]; end = span[1] }
    else if (span[0] <= end) end = Math.max(end, span[1])
    else { measuredWait += end - start; start = span[0]; end = span[1] }
  }
  if (start !== null) measuredWait += end - start
  const wait = trace.admission_complete ? measuredWait : null
  const last = trace.attempts[trace.attempts.length - 1]
  const previous = trace.attempts.slice(0, -1)
  return {
    measuredWait,
    wait,
    withoutWait: wait !== null && trace.duration_ms !== null ? trace.duration_ms - wait : null,
    finalAttempt: last?.end_ms != null ? last.end_ms - last.start_ms : null,
    previousAttempts: previous.length === 0 ? 0 : previous.every(attempt => attempt.end_ms !== null) ? previous.reduce((sum, attempt) => sum + (attempt.end_ms! - attempt.start_ms), 0) : null
  }
}
