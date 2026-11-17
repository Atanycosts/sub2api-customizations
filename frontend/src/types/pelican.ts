export type PelicanProtocol = 'chat' | 'responses'
export type PelicanEffort = '' | 'none' | 'minimal' | 'low' | 'medium' | 'high' | 'xhigh' | 'max'
export type PelicanRunState = 'running' | 'completed' | 'failed' | 'unknown'
export type PelicanErrorKind =
  | ''
  | 'unavailable'
  | 'upstream_error'
  | 'incomplete'
  | 'timeout'
  | 'output_limit'
  | 'interrupted'
  | 'invalid_output'

export interface PelicanGroup {
  id: number
  name: string
  platform: string
}

export interface PelicanStats {
  completed: number
  failed: number
  unknown: number
  running: number
}

export interface PelicanPublicRun {
  id: string
  source: 'group'
  group_id: number
  group_name: string
  model: string
  effort: PelicanEffort
  protocol: PelicanProtocol
  prompt_version: string
  max_output_tokens: number
  state: PelicanRunState
  created_at: string
  finished_at: string | null
  duration_ms: number
  terminal: string
  error_kind: PelicanErrorKind
}

export interface PelicanPublicRunDetail extends PelicanPublicRun {
  output?: string
}

export interface PelicanPublicPage {
  groups: PelicanGroup[]
  items: PelicanPublicRun[]
  total: number
  page: number
  page_size: number
  pages: number
  stats: PelicanStats
  prompt_version: string
  prompt: string
}

export interface PelicanPublicFilters {
  group_id?: number
  model?: string
  effort?: PelicanEffort
  page?: number
  page_size?: number
}

export interface PelicanConfig {
  enabled: boolean
  daily_budget_usd: number
}

export interface PelicanPlanInput {
  group_id: number
  model: string
  effort: PelicanEffort
  protocol: PelicanProtocol
  enabled: boolean
  published: boolean
  interval_minutes: number
  max_output_tokens: number
  budget_usd: number
}

export interface PelicanPlan extends PelicanPlanInput {
  id: number
  group_name: string
  last_run_at: string | null
  next_run_at: string
  running_until: string | null
}

export interface PelicanAdminRun extends PelicanPublicRunDetail {
  plan_id: number
  requested_by: number
  frozen: PelicanPlanInput
  published: boolean
  account_id: number
  upstream_model: string
  endpoint: string
  transport?: 'sse' | 'json'
  raw_response: unknown
  usage: unknown
  error: string
  cost_usd: number | null
  budget_usd?: number
  reserved_usd: number
  http_status: number
}

export interface PelicanAdminFilters {
  plan_id?: number
  state?: PelicanRunState
  page?: number
  page_size?: number
}

export interface PelicanAdminPage {
  items: PelicanAdminRun[]
  total: number
  page: number
  page_size: number
  pages: number
  stats: PelicanStats
}
