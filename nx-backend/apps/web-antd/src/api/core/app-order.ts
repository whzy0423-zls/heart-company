import { requestClient } from '#/api/request';

export interface AppOrder {
  amount: number;
  appUserId: number;
  createTime: string;
  id: number;
  memberLevel: string;
  nickname: string;
  outTradeNo: string;
  paidAt: string;
  phone: string;
  productId: string;
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
