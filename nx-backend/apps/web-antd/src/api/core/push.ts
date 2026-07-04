import { requestClient } from '#/api/request';

export interface PushNotification {
  content: string;
  createTime: string;
  deepLink: string;
  errorMessage?: string;
  id: number;
  operator: string;
  sentCount: number;
  status: string;
  targetType: string;
  targetValue: string;
  title: string;
}

export interface PushListResult {
  items: PushNotification[];
  total: number;
}

export interface PushSendParams {
  content: string;
  deepLink?: string;
  targetType?: string;
  targetValue?: string;
  title: string;
}

export function getPushListApi(params?: { page?: number; pageSize?: number }) {
  return requestClient.get<PushListResult>('/push/list', { params });
}

export function sendPushApi(data: PushSendParams) {
  return requestClient.post<{ msgId: string; sent: number }>(
    '/push/send',
    data,
  );
}
