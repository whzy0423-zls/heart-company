import { requestClient } from '#/api/request';

export interface AppOrder {
  activationAt?: string;
  amount: number;
  appUserId: number;
  createTime: string;
  id: number;
  memberLevel: string;
  memberExpiresAt?: string;
  memberStartedAt?: string;
  membershipExpiresAt?: string;
  nickname: string;
  outTradeNo: string;
  paidAt: string;
  phone: string;
  productId: string;
  remainingDays?: number;
  status: string;
  title: string;
  transactionId: string;
  updateTime: string;
}

export interface AppOrderPageResult<T> {
  items: T[];
  total: number;
}

export function getAppOrderListApi(params?: Record<string, any>) {
  return requestClient.get<AppOrderPageResult<AppOrder>>('/app-orders/list', {
    params,
  });
}
