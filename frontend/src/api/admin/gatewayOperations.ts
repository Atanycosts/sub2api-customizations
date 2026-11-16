import { apiClient } from '../client'
import type { OperationsConfig, OperationsWallet, OperationsWalletInput, OperationsExperiment, OperationsExperimentInput, OperationsCacheUsage } from '@/types/gatewayOperations'
const root = '/admin/gateway-operations'
export const gatewayOperationsAPI = {
  config: async () => (await apiClient.get<OperationsConfig>(`${root}/config`)).data,
  saveConfig: async (input: OperationsConfig) => (await apiClient.put<OperationsConfig>(`${root}/config`, input)).data,
  wallets: async (offset = 0) => (await apiClient.get<OperationsWallet[]>(`${root}/wallets`, { params: { offset, limit: 50 } })).data,
  saveWallet: async (id: number, input: OperationsWalletInput) => (await (id ? apiClient.put<OperationsWallet>(`${root}/wallets/${id}`, input) : apiClient.post<OperationsWallet>(`${root}/wallets`, input))).data,
  deleteWallet: async (id: number) => { await apiClient.delete(`${root}/wallets/${id}`) },
  refreshWallet: async (id: number) => (await apiClient.post<OperationsWallet>(`${root}/wallets/${id}/refresh`)).data,
  experiments: async (offset = 0) => (await apiClient.get<OperationsExperiment[]>(`${root}/experiments`, { params: { offset, limit: 50 } })).data,
  runExperiment: async (input: OperationsExperimentInput) => (await apiClient.post<OperationsExperiment>(`${root}/experiments`, input, { timeout: 200000 })).data,
  cacheUsage: async (keyID: number, requestIDs: string[]) => (await apiClient.post<OperationsCacheUsage[]>(`${root}/cache-usage`, { key_id: keyID, request_ids: requestIDs })).data,
}
