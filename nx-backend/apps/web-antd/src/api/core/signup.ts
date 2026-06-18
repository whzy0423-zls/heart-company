import { requestClient } from '#/api/request';

export interface SignupLead {
  contact: string;
  contactType: 'phone' | 'wechat' | string;
  createTime: string;
  id: string;
  interest: string;
  ip: string;
  message: string;
  name: string;
  userAgent: string;
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
