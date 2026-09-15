import { requestClient } from '#/api/request';

export interface DistributionAgent {
  id: number;
  appUserId: number;
  agentCode: string;
  level: number;
  status: string;
}
export interface DistributionSettlement { id: number; agentId: number; periodStart: string; periodEnd: string; amount: number; status: string }
export const getDistributionAgentsApi = () => requestClient.get<{ items: DistributionAgent[]; total: number }>('/admin/distribution/agents');
export const createDistributionAgentApi = (data: { appUserId: number; agentCode?: string }) => requestClient.post<DistributionAgent>('/admin/distribution/agents', data);
export const updateDistributionAgentStatusApi = (id: number, status: string) => requestClient.put(`/admin/distribution/agents/${id}`, { status });
export const previewDistributionSettlementApi = (data: { agentId: number; start: string; end: string }) => requestClient.post('/admin/distribution/settlements/preview', data);
export const createDistributionSettlementApi = (data: { agentId: number; start: string; end: string }) => requestClient.post<DistributionSettlement>('/admin/distribution/settlements', data);
export const settleDistributionActionApi = (id: number, action: string) => requestClient.post(`/admin/distribution/settlements/${id}/${action}`, {});
