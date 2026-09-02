import { requestClient } from '#/api/request';

export interface AppOrder {
  activationAt?: string;
  amount: number;
  appUserId: number;
  createTime: string;
  durationDays: number;
  id: number;
  memberLevel: string;
  memberExpiresAt?: string;
  memberStartedAt?: string;
  membershipExpiresAt?: string;
  nickname: string;
  outTradeNo: string;
  paidAt: string;
  paymentError?: string;
  paymentProvider?: string;
  payChannel?: string;
  gatewayId?: string;
  providerStatus?: string;
  providerTradeNo?: string;
  lastQueryAt?: string;
  payUrl?: string;
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

export function reconcileAppOrderApi(id: number | string) {
  return requestClient.post<AppOrder>(`/app-orders/${id}/reconcile`);
}
