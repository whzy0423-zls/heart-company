import { requestClient } from '#/api/request';

export interface AppCustomer {
  avatar: string;
  createTime: string;
  id: number;
  lastLoginAt: null | string;
  memberLevel: string;
  memberExpiresAt?: string;
  memberStartedAt?: string;
  nickname: string;
  phone: string;
  registerSource: string;
  remainingDays?: number;
  status: string;
  updateTime: string;
}

export interface AppCustomerPageResult<T> {
  items: T[];
  total: number;
}

export interface AppUserInsight {
  avatar: string;
  cardCount: number;
  centers: Record<string, any>[] | Record<string, any>;
  compatibilityCount: number;
  createTime: string;
  gender: string;
  id: number;
  lastLoginAt: string;
  latestChatTime: string;
  latestCompatibilitySummary: string;
  latestMemory: string;
  latestQuizTime: string;
  memberLevel: string;
  memoryCount: number;
  messageCount: number;
  nickname: string;
  phone: string;
  primaryType: number;
  profile: Record<string, any>;
  registerSource: string;
  score: Record<string, number>;
  secondType: number;
  sessionCount: number;
  status: string;
  updateTime: string;
  wingType: number;
}

export interface UpdateAppCustomerInput {
  memberLevel: string;
  status: string;
}

export function getAppCustomerListApi(params?: Record<string, any>) {
  return requestClient.get<AppCustomerPageResult<AppCustomer>>(
    '/app-users/list',
    { params },
  );
}

export function getAppUserInsightsApi(params?: Record<string, any>) {
  return requestClient.get<AppCustomerPageResult<AppUserInsight>>(
    '/app-users/insights',
    { params },
  );
}

export function getAppCustomerDetailApi(id: number | string) {
  return requestClient.get<AppCustomer>(`/app-users/${id}`);
}

export function updateAppCustomerApi(
  id: number | string,
  data: UpdateAppCustomerInput,
) {
  return requestClient.put<AppCustomer>(`/app-users/${id}`, data);
}
