import { requestClient } from '#/api/request';

import type { PageResult } from './signup';

export interface SystemMessage {
  businessId: string;
  businessType: string;
  content: string;
  createTime: string;
  id: string;
  isRead: boolean;
  targetPath: string;
  title: string;
  type: string;
}

export interface MessageQuery {
  keyword?: string;
  page?: number;
  pageSize?: number;
  read?: boolean | string;
  type?: string;
}

export function getMessageListApi(params?: MessageQuery) {
  return requestClient.get<PageResult<SystemMessage>>('/messages/list', {
    params,
  });
}

export function markMessagesApi(data: { ids?: string[]; read: boolean }) {
  return requestClient.put<boolean>('/messages/read', data);
}
