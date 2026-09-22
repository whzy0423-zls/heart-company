import { requestClient } from '#/api/request';

export interface MiniappOrder {
  id: number;
  outTradeNo: string;
  wxUserId: number;
  phone: string;
  nickname: string;
  product: string;
  title: string;
  amount: number;
  status: string;
  transactionId: string;
  createTime: string;
  updateTime: string;
  paidAt?: string;
}

export interface MiniappOrderPageResult {
  items: MiniappOrder[];
  summary: {
    paid: number;
    paidAmount: number;
    pending: number;
    total: number;
  };
  total: number;
}

export interface MiniappOrderReconcileResult {
  checked: number;
  closed: number;
  failed: number;
  paid: number;
}

export function getMiniappOrderListApi(params?: Record<string, any>) {
  return requestClient.get<MiniappOrderPageResult>('/miniapp-orders/list', { params });
}

export function reconcileMiniappOrdersApi() {
  return requestClient.post<MiniappOrderReconcileResult>('/miniapp-orders/reconcile');
}
