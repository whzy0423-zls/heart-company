import { requestClient } from '#/api/request';

export interface DistributionAgent { id:number; appUserId:number; agentCode:string; level:number; parentAgentId:number; rootAgentId:number; path:string; status:string }
export interface DistributionCommission { id:number; orderId:number; agentId:number; appUserId:number; agentLevel:number; orderAmount:number; rateBPS:number; commissionAmount:number; ruleVersion:number; status:string; createdAt:string }
export const getDistributionAgentsApi = () => requestClient.get<{items:DistributionAgent[];total:number}>('/admin/distribution/agents');
export const createDistributionAgentApi = (data:{appUserId:number;agentCode?:string}) => requestClient.post<DistributionAgent>('/admin/distribution/agents',data);
export const updateDistributionAgentStatusApi = (id:number,status:string) => requestClient.put(`/admin/distribution/agents/${id}`,{status});
export const getDistributionCommissionsApi = () => requestClient.get<{items:DistributionCommission[];total:number}>('/admin/distribution/commissions');
export const reverseDistributionCommissionApi = (id:number,reason:string) => requestClient.post(`/admin/distribution/commissions/${id}`,{reason});
