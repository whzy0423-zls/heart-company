import { requestClient } from '#/api/request';

export interface AppChatAuditMessage {
  appUserId: number;
  cardId: number;
  cardName: string;
  content: string;
  createTime: string;
  favorite: boolean;
  feedback: string;
  id: number;
  nickname: string;
  phone: string;
  role: string;
  sessionId: number;
  sources: any[];
}

export interface AppMemoryAdminItem {
  appUserId: number;
  cardId: number;
  cardName: string;
  content: string;
  createTime: string;
  id: number;
  nickname: string;
  phone: string;
  sourceTime: string;
  status: string;
  updateTime: string;
}

export interface AppOpsPageResult<T> {
  items: T[];
  total: number;
}

export function grantAppOrderApi(id: number | string) {
  return requestClient.post<boolean>(`/app-orders/${id}/grant`);
}

export function getAppChatAuditMessagesApi(params?: Record<string, any>) {
  return requestClient.get<AppOpsPageResult<AppChatAuditMessage>>(
    '/app-chat/messages/list',
    { params },
  );
}

export function getAppMemoriesAdminApi(params?: Record<string, any>) {
  return requestClient.get<AppOpsPageResult<AppMemoryAdminItem>>(
    '/app-memories/list',
    {
      params,
    },
  );
}

export function updateAppMemoryStatusApi(id: number | string, status: string) {
  return requestClient.put<boolean>(`/app-memories/${id}/status`, { status });
}
