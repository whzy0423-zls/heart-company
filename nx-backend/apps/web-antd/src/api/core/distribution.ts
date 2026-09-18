import { requestClient } from '#/api/request';

export interface DistributionAgent {
  id: number;
  appUserId: number;
  agentCode: string;
  level: number;
  parentAgentId: number;
  rootAgentId: number;
  path: string;
  status: string;
  directUserCount: number;
  secondLevelAgentCount: number;
  thirdLevelAgentCount: number;
}
export interface DistributionSettlement { id: number; agentId: number; periodStart: string; periodEnd: string; amount: number; status: string; createdAt: string }
export interface DistributionAnalyticsSummary {
  totalAgents: number;
  activeAgents: number;
  pausedAgents: number;
  totalOrderAmount: number;
  totalCommissionAmount: number;
  pendingCommissionAmount: number;
  settledCommissionAmount: number;
  commissionRecordCount: number;
}
export interface DistributionAnalyticsTrendItem {
  date: string;
  orderAmount: number;
  commissionAmount: number;
  commissionCount: number;
}
export interface DistributionAnalyticsAgentRanking {
  agentId: number;
  agentCode: string;
  appUserId: number;
  status: string;
  directUserCount: number;
  childAgentCount: number;
  orderCount: number;
  orderAmount: number;
  commissionAmount: number;
  pendingCommissionAmount: number;
}
export interface AgentDistributionAnalytics {
  summary: DistributionAnalyticsSummary;
  trend: DistributionAnalyticsTrendItem[];
  orders: Array<{
    id: number;
    outTradeNo: string;
    appUserId: number;
    amount: number;
    status: string;
    paidAt: string;
  }>;
  users: Array<{
    id: number;
    nickname: string;
    memberLevel: string;
    directAgentId: number;
    boundAt: string;
    orderAmount: number;
    orderCount: number;
  }>;
}
export interface DistributionAnalytics {
  summary: DistributionAnalyticsSummary;
  trend: DistributionAnalyticsTrendItem[];
  agentRankings: DistributionAnalyticsAgentRanking[];
}
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
export const getDistributionAnalyticsApi = () => requestClient.get<DistributionAnalytics>('/admin/distribution/analytics');
export const getAgentDistributionProfileApi = () => requestClient.get<DistributionAgent>('/agent/distribution/profile');
export const getAgentDistributionAgentsApi = () => requestClient.get<{ current: DistributionAgent; items: DistributionAgent[]; total: number }>('/agent/distribution/agents');
export const createAgentDistributionChildApi = (data: { appUserId: number }) => requestClient.post<DistributionAgent>('/agent/distribution/agents', data);
export const getAgentDistributionAnalyticsApi = (params?: { endDate?: string; startDate?: string }) => requestClient.get<AgentDistributionAnalytics>('/agent/distribution/analytics', { params });
export const createDistributionAgentApi = (data: { appUserId: number }) => requestClient.post<DistributionAgent>('/admin/distribution/agents', data);
export const updateDistributionAgentStatusApi = (id: number, status: string) => requestClient.put(`/admin/distribution/agents/${id}`, { status });
export const getDistributionRulesApi = () => requestClient.get<{ items: DistributionRule[]; total: number }>('/admin/distribution/rules');
export const createDistributionRuleApi = (data: { name: string; rates: Record<number, number> }) => requestClient.post<DistributionRule>('/admin/distribution/rules', data);
export const activateDistributionRuleApi = (id: number) => requestClient.post(`/admin/distribution/rules/${id}/activate`, {});
export const previewDistributionSettlementApi = (data: { agentId: number; start: string; end: string }) => requestClient.post<{ agentId: number; amount: number; count: number; start: string; end: string }>('/admin/distribution/settlements/preview', data);
export const createDistributionSettlementApi = (data: { agentId: number; start: string; end: string }) => requestClient.post<{ settlementId: number; amount: number; count: number }>('/admin/distribution/settlements', data);
export const settleDistributionActionApi = (id: number, action: string, data: { paymentReference?: string; reason?: string } = {}) => requestClient.post(`/admin/distribution/settlements/${id}/${action}`, data);
export const getDistributionSettlementsApi = () => requestClient.get<{ items: DistributionSettlement[]; total: number }>('/admin/distribution/settlements');
