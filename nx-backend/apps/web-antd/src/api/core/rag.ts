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

export interface RAGReindexResult {
  done: number;
  failed: number;
  pending: number;
}

export function reindexRAGDocumentsApi() {
  return requestClient.post<RAGReindexResult>('/rag/reindex', {});
}

export interface PublicKnowledgeSource {
  category: string;
  createTime: string;
  datasetId: string;
  enabled: boolean;
  extractStatus: string;
  fileFormat: string;
  id: string;
  importedChunks: number;
  qualityStatus: 'needs_review' | 'pending' | 'ready';
  sourceChunks: number;
  sourceKind: 'book' | 'story';
  sourceRecordId: number;
  textChars: number;
  title: string;
  updateTime: string;
}

export interface PublicKnowledgeSourcesQuery {
  category?: string;
  datasetId?: string;
  enabled?: boolean;
  keyword?: string;
  page: number;
  pageSize: number;
  qualityStatus?: string;
}

export interface PublicKnowledgeSourcesResult extends RAGPageResult<PublicKnowledgeSource> {
  categories: { count: number; name: string }[];
}

export interface PublicKnowledgeSourceChunk {
  content: string;
  id: string;
  importBatchId: string;
  locator: Record<string, unknown>;
  title: string;
}

export function getPublicKnowledgeSourcesApi(
  params: PublicKnowledgeSourcesQuery,
) {
  return requestClient.get<PublicKnowledgeSourcesResult>('/rag/sources', {
    params,
  });
}

export function updatePublicKnowledgeSourcesStatusApi(data: {
  enabled: boolean;
  ids: string[];
}) {
  return requestClient.post<{ updated: number }>('/rag/sources/status', data);
}

export function getPublicKnowledgeSourceChunksApi(id: string) {
  return requestClient.get<{
    items: PublicKnowledgeSourceChunk[];
    total: number;
  }>(`/rag/sources/${encodeURIComponent(id)}/chunks`);
}
