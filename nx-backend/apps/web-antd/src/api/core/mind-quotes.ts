import { requestClient } from '#/api/request';

export interface MindGroup {
  createTime?: string;
  id: string;
  intro: string;
  name: string;
  quoteCount?: number;
  sort: number;
  status: string;
  updateTime?: string;
}

export interface MindGroupInput {
  id?: string;
  intro?: string;
  name: string;
  sort?: number;
  status?: string;
}

export interface MindQuote {
  content: string;
  createTime?: string;
  groupId: string;
  id: string;
  prompt: string;
  sort: number;
  status: string;
  title: string;
  updateTime?: string;
}

export interface MindQuoteInput {
  content?: string;
  groupId?: string;
  id?: string;
  prompt?: string;
  sort?: number;
  status?: string;
  title: string;
}

interface ListWrap<T> {
  items: T[];
}

interface PageWrap<T> {
  items: T[];
  total: number;
}

// ---- 分组 ----
export function getMindGroupsApi() {
  return requestClient.get<ListWrap<MindGroup>>('/mind-groups');
}

export function saveMindGroupApi(data: MindGroupInput) {
  return requestClient.post<MindGroup>('/mind-groups', data);
}

export function deleteMindGroupApi(id: string) {
  return requestClient.delete<boolean>('/mind-groups', { params: { id } });
}

// ---- 心语 ----
export function getMindQuotesApi(params?: Record<string, any>) {
  return requestClient.get<PageWrap<MindQuote>>('/mind-quotes', { params });
}

export function getMindQuoteApi(id: string) {
  return requestClient.get<MindQuote>(`/mind-quotes/${id}`);
}

export function createMindQuoteApi(data: MindQuoteInput) {
  return requestClient.post<MindQuote>('/mind-quotes', data);
}

export function updateMindQuoteApi(id: string, data: MindQuoteInput) {
  return requestClient.put<MindQuote>(`/mind-quotes/${id}`, data);
}

export function deleteMindQuoteApi(id: string) {
  return requestClient.delete<boolean>(`/mind-quotes/${id}`);
}
