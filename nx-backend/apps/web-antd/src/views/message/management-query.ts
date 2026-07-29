import type { MessageQuery } from '#/api';

export interface MessageManagementFilters {
  category: '' | 'signup';
  keyword: string;
  page: number;
  pageSize: number;
  read: string;
}

export function buildMessageListParams(
  filters: MessageManagementFilters,
): MessageQuery {
  return {
    businessType: filters.category || undefined,
    keyword: filters.keyword,
    page: filters.page,
    pageSize: filters.pageSize,
    read: filters.read || undefined,
  };
}
