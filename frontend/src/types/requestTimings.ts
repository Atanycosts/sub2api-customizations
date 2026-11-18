export interface RequestTimingAttempt {
  number: number
  account_id: number
  endpoint: string
  start_ms: number
  end_ms: number | null
  events_ms: Record<string, number>
  connection_reused: boolean | null
  status: number
  outcome: string
}
export interface RequestTiming {
  trace_id: string
  request_id: string
  usage_request_id?: string
  usage_id: number | null
  user_id: number
  api_key_id: number
  started_at: string
  duration_ms: number | null
  path: string
  status: number
  outcome: string
  completeness: 'complete' | 'partial'
  first_semantic_ms: number | null
  first_text_ms: number | null
  first_content_flush_ms: number | null
  admission_complete: boolean
  admission: { kind: string; start_ms: number; end_ms: number }[]
  attempts: RequestTimingAttempt[]
}
export interface RequestTimingStats { queued: number; dropped: number; write_failed: number }
export interface RequestTimingQuery { request_id?: string; account_id?: number; outcome?: string; before?: string; limit?: number }
export interface RequestTimingList { items: RequestTiming[]; stats: RequestTimingStats; retention_days: number }
