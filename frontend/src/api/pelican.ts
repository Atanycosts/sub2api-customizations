import { apiClient } from './client'
import type {
  PelicanAdminFilters,
  PelicanAdminPage,
  PelicanAdminRun,
  PelicanConfig,
  PelicanPlan,
  PelicanPlanInput,
  PelicanPublicFilters,
  PelicanPublicPage,
  PelicanPublicRunDetail,
} from '@/types/pelican'

export async function getPelicanResults(
  filters: PelicanPublicFilters = {},
  options?: { signal?: AbortSignal },
): Promise<PelicanPublicPage> {
  const { data } = await apiClient.get<PelicanPublicPage>('/pelican', {
    params: filters,
    signal: options?.signal,
  })
  return data
}

export async function getPelicanResult(
  id: string,
  options?: { signal?: AbortSignal },
): Promise<PelicanPublicRunDetail> {
  const { data } = await apiClient.get<PelicanPublicRunDetail>(`/pelican/${id}`, {
    signal: options?.signal,
  })
  return data
}

const adminRoot = '/admin/pelican'

export async function getPelicanConfig(): Promise<PelicanConfig> {
  const { data } = await apiClient.get<PelicanConfig>(`${adminRoot}/config`)
  return data
}

export async function savePelicanConfig(input: PelicanConfig): Promise<PelicanConfig> {
  const { data } = await apiClient.put<PelicanConfig>(`${adminRoot}/config`, input)
  return data
}

export async function getPelicanPlans(): Promise<PelicanPlan[]> {
  const { data } = await apiClient.get<PelicanPlan[]>(`${adminRoot}/plans`)
  return data
}

export async function savePelicanPlan(
  id: number | null,
  input: PelicanPlanInput,
): Promise<PelicanPlan> {
  const { data } = await (id
    ? apiClient.put<PelicanPlan>(`${adminRoot}/plans/${id}`, input)
    : apiClient.post<PelicanPlan>(`${adminRoot}/plans`, input))
  return data
}

export async function deletePelicanPlan(id: number): Promise<void> {
  await apiClient.delete(`${adminRoot}/plans/${id}`)
}

export async function runPelicanPlan(
  id: number,
  submitId: string,
): Promise<PelicanAdminRun> {
  const { data } = await apiClient.post<PelicanAdminRun>(`${adminRoot}/plans/${id}/run`, {
    submit_id: submitId,
  })
  return data
}

export async function getPelicanAdminResults(
  filters: PelicanAdminFilters = {},
): Promise<PelicanAdminPage> {
  const { data } = await apiClient.get<PelicanAdminPage>(`${adminRoot}/results`, {
    params: filters,
  })
  return data
}

export async function getPelicanAdminResult(id: string): Promise<PelicanAdminRun> {
  const { data } = await apiClient.get<PelicanAdminRun>(`${adminRoot}/results/${id}`)
  return data
}

export const pelicanAPI = {
  getPelicanResults,
  getPelicanResult,
  getPelicanConfig,
  savePelicanConfig,
  getPelicanPlans,
  savePelicanPlan,
  deletePelicanPlan,
  runPelicanPlan,
  getPelicanAdminResults,
  getPelicanAdminResult,
}

export default pelicanAPI
