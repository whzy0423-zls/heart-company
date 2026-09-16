import { requestClient } from '#/api/request';

export interface DistributionAgent {
  id: number;
  appUserId: number;
  agentCode: string;
  level: number;
  status: string;
}
export interface DistributionSettlement { id: number; agentId: number; periodStart: string; periodEnd: string; amount: number; status: string; createdAt: string }
export interface DistributionRule {
  id: number;
  version: number;
  status: 'active' | 'archived' | 'draft' | string;
  createdAt: string;
  activatedAt: string;
  rates: Record<number, number>;
  totalRateBps: number;
}
export const getDistributionAgentsApi = () => requestClient.get<{ items: DistributionAgent[]; total: number }>('/admin/distribution/agents');
export const createDistributionAgentApi = (data: { appUserId: number }) => requestClient.post<DistributionAgent>('/admin/distribution/agents', data);
export const updateDistributionAgentStatusApi = (id: number, status: string) => requestClient.put(`/admin/distribution/agents/${id}`, { status });
export const getDistributionRulesApi = () => requestClient.get<{ items: DistributionRule[]; total: number }>('/admin/distribution/rules');
export const createDistributionRuleApi = (data: { name: string; rates: Record<number, number> }) => requestClient.post<DistributionRule>('/admin/distribution/rules', data);
export const activateDistributionRuleApi = (id: number) => requestClient.post(`/admin/distribution/rules/${id}/activate`, {});
export const previewDistributionSettlementApi = (data: { agentId: number; start: string; end: string }) => requestClient.post('/admin/distribution/settlements/preview', data);
export const createDistributionSettlementApi = (data: { agentId: number; start: string; end: string }) => requestClient.post<DistributionSettlement>('/admin/distribution/settlements', data);
export const settleDistributionActionApi = (id: number, action: string) => requestClient.post(`/admin/distribution/settlements/${id}/${action}`, {});
export const getDistributionSettlementsApi = () => requestClient.get<{ items: DistributionSettlement[]; total: number }>('/admin/distribution/settlements');
