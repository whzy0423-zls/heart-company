import { requestClient } from '#/api/request';

export interface RAGDocument {
  content: string;
  createTime: string;
  id: string;
  sort: number;
  source: string;
  status: 'disabled' | 'enabled' | string;
  tags: string[];
  title: string;
  updateTime: string;
}

export interface RAGDocumentInput {
  content: string;
  id?: string;
  sort?: number;
  source?: string;
  status?: 'disabled' | 'enabled' | string;
  tags?: string[];
  title: string;
}

interface RAGPageResult<T> {
  items: T[];
  page: number;
  pageSize: number;
  total: number;
}

export function getRAGDocumentsApi(params?: Record<string, any>) {
  return requestClient.get<RAGPageResult<RAGDocument>>('/rag/documents', {
    params,
  });
}

export function createRAGDocumentApi(data: RAGDocumentInput) {
  return requestClient.post<RAGDocument>('/rag/documents', data);
}

export function updateRAGDocumentApi(id: string, data: RAGDocumentInput) {
  return requestClient.put<RAGDocument>(`/rag/documents/${id}`, data);
}

export function deleteRAGDocumentApi(id: string) {
  return requestClient.delete<boolean>(`/rag/documents/${id}`);
}
