export interface OperationsConfig { worker_enabled: boolean; refresh_minutes: number; daily_budget_usd: number }
export interface OperationsWallet {
  id: number; name: string; provider: 'sub2api' | 'newapi'; base_url: string; remote_user_id: string;
  account_ids: number[]; currency: 'USD' | 'CNY' | 'quota_units'; threshold: number | null; notify_email: string;
  balance: number | null; refreshed_at: string | null; last_attempt_at: string | null; last_error: string; stale: boolean
}
export type OperationsWalletInput = Pick<OperationsWallet, 'name' | 'provider' | 'base_url' | 'remote_user_id' | 'account_ids' | 'currency' | 'threshold' | 'notify_email'> & { credential_value: string }
export interface OperationsExperimentInput { submit_id: string; kind: 'pelican' | 'experiment'; account_id: number; model: string; effort: string; protocol: 'chat' | 'responses'; prompt: string; prompt_version: string; max_output_tokens: number; budget_usd: number }
export interface OperationsExperiment { id: string; created_at: string; finished_at: string | null; requested_by: number; frozen: OperationsExperimentInput; upstream_model: string; endpoint: string; state: string; output: string; error: string; usage: unknown; raw_response: unknown; terminal: string; cost_usd: number | null; reserved_usd: number; duration_ms: number; http_status: number }
export interface OperationsCacheUsage { id: number; request_id: string; account_id: number; group_id: number | null; model: string; input_tokens: number; output_tokens: number; cache_read_tokens: number; cache_creation_tokens: number; actual_cost: number; cache_fields_known: boolean }
