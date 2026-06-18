import { requestClient } from '#/api/request';

export interface SignupLead {
  contact: string;
  contactType: 'phone' | 'wechat' | string;
  createTime: string;
  followNote: string;
  followStatus: SignupFollowStatus | string;
  id: string;
  interest: string;
  ip: string;
  message: string;
  name: string;
  nextFollowTime: string;
  owner: string;
  userAgent: string;
}

export type SignupFollowStatus =
  | 'contacted'
  | 'deal'
  | 'interested'
  | 'invalid'
  | 'pending';

export interface SignupTimelineItem {
  content: string;
  createTime: string;
  nextFollowTime: string;
  operator: string;
  owner: string;
  status: SignupFollowStatus | string;
  type: 'created' | 'followup' | string;
}

export interface SignupDetail {
  lead: SignupLead;
  timeline: SignupTimelineItem[];
}

export interface SignupFollowInput {
  content?: string;
  followNote?: string;
  nextFollowTime?: string;
  owner?: string;
  status?: SignupFollowStatus | string;
}

export interface PageResult<T> {
  items: T[];
  total: number;
}

export function getSignupLeadListApi(params?: Record<string, any>) {
  return requestClient.get<PageResult<SignupLead>>('/signups/list', {
    params,
  });
}

export function getSignupDetailApi(id: string) {
  return requestClient.get<SignupDetail>('/signups/detail', {
    params: { id },
  });
}

export function saveSignupFollowApi(id: string, data: SignupFollowInput) {
  return requestClient.put<SignupLead>('/signups/follow', data, {
    params: { id },
  });
}
