import { apiClient } from './client'

export type MemoryArchiveState = 'pending' | 'processing' | 'submitted' | 'retry_wait' | 'needs_reconciliation' | 'completed' | 'failed' | 'cancelled'

export interface AccountMemoryOverview {
  configured: boolean
  state: 'unconfigured' | 'active' | 'suspended'
  supported_endpoint?: 'responses_http_text'
  archive_mode?: 'source_diary'
  runtime_health_checked: boolean
  archived: number
  pending: number
  attention: number
  recent: Array<{
    event_id: string
    state: MemoryArchiveState
    created_at: string
    updated_at: string
  }>
}

export const memoryAPI = {
  async overview(options?: { signal?: AbortSignal }): Promise<AccountMemoryOverview> {
    const { data } = await apiClient.get<AccountMemoryOverview>('/user/memory', options)
    return data
  }
}
