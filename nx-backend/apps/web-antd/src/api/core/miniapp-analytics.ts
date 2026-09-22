import { requestClient } from '#/api/request';

export interface MiniappAnalyticsSummary {
  averageOrder: number;
  closedOrders: number;
  paidAmount: number;
  paidOrders: number;
  pendingOrders: number;
  successRate: number;
  totalOrders: number;
}

export interface MiniappAnalyticsHour {
  hour: number;
  paidAmount: number;
  paidOrders: number;
}

export interface MiniappAnalyticsStatus {
  count: number;
  status: string;
}

export interface MiniappAnalyticsProduct {
  paidAmount: number;
  paidOrders: number;
  product: string;
  title: string;
}

export interface MiniappAnalyticsRecentOrder {
  amount: number;
  outTradeNo: string;
  paidAt: string;
  title: string;
  transactionId: string;
}

export interface MiniappAnalyticsResult {
  date: string;
  hours: MiniappAnalyticsHour[];
  products: MiniappAnalyticsProduct[];
  recentOrders: MiniappAnalyticsRecentOrder[];
  statuses: MiniappAnalyticsStatus[];
  summary: MiniappAnalyticsSummary;
}

export function getMiniappAnalyticsApi(params?: { date?: string }) {
  return requestClient.get<MiniappAnalyticsResult>('/miniapp-orders/analytics', { params });
}
