import { apiClient } from '../client'
import type { RequestTiming, RequestTimingList, RequestTimingQuery } from '@/types/requestTimings'

export async function listRequestTimings(params: RequestTimingQuery = {}): Promise<RequestTimingList> {
  const { data } = await apiClient.get<RequestTimingList>('/admin/request-timings', { params })
  return data
}
export async function getRequestTiming(traceID: string): Promise<RequestTiming> {
  const { data } = await apiClient.get<RequestTiming>(`/admin/request-timings/${encodeURIComponent(traceID)}`)
  return data
}
