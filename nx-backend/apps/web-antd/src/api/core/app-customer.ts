import { requestClient } from '#/api/request';

export interface AppCustomer {
  avatar: string;
  createTime: string;
  id: number;
  lastLoginAt: null | string;
  memberLevel: string;
  nickname: string;
  phone: string;
  registerSource: string;
  status: string;
  updateTime: string;
}

export interface AppCustomerPageResult<T> {
  items: T[];
  total: number;
}

export function getAppCustomerListApi(params?: Record<string, any>) {
  return requestClient.get<AppCustomerPageResult<AppCustomer>>(
    '/app-users/list',
    { params },
  );
}

export function getAppCustomerDetailApi(id: number | string) {
  return requestClient.get<AppCustomer>(`/app-users/${id}`);
}
