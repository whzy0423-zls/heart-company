import { requestClient } from '#/api/request';

export interface EnneagramTypeSummary {
  activeReleaseId?: number;
  currentVersion: number;
  importId: number;
  itemCount: number;
  libraryId: number;
  libraryKey: string;
  status: string;
  title: string;
  type: number;
  updatedAt?: string;
}

export interface EnneagramSourcePage {
  enneagramType: number;
  manualReviewStatus: string;
  ocrStatus: string;
  ocrTextHash: string;
  pageNumber: number;
  sourceId: string;
}

export interface EnneagramTypeItem {
  contentKey: string;
  dimension: string;
  provenanceKind: string;
  sourcePages: EnneagramSourcePage[];
  text: string;
}

export interface EnneagramTypeDetail {
  contentDigest: string;
  items: EnneagramTypeItem[];
  reviewNotes: string;
  sourceChapter: string;
  summary: EnneagramTypeSummary;
}

export interface EnneagramDraftInput {
  contentDigest?: string;
  items: Array<{ contentKey: string; text: string }>;
  sourceChapter: string;
  title: string;
}

export interface EnneagramPreview {
  hits: Array<{ contentKey: string; dimension: string; score: number; text: string }>;
  query: string;
  type: number;
}

export interface EnneagramPublishResult {
  importId: number;
  libraryId: number;
  libraryKey: string;
  releaseId: number;
  version: number;
}

export interface EnneagramVersion {
  activatedAt?: string;
  cardCount: number;
  chunkCount: number;
  createdAt: string;
  indexVersion: string;
  releaseId: number;
  status: string;
  version: number;
}

export function getEnneagramTypesApi() {
  return requestClient.get<EnneagramTypeSummary[]>('/enneagram-library/types');
}

export function getEnneagramTypeDetailApi(type: number) {
  return requestClient.get<EnneagramTypeDetail>(`/enneagram-library/types/${type}`);
}

export function saveEnneagramDraftApi(type: number, data: EnneagramDraftInput) {
  return requestClient.put<EnneagramTypeDetail>(`/enneagram-library/types/${type}/draft`, data);
}

export function submitEnneagramReviewApi(type: number) {
  return requestClient.post(`/enneagram-library/types/${type}/submit`, {});
}

export function approveEnneagramTypeApi(type: number, notes = '') {
  return requestClient.post(`/enneagram-library/types/${type}/approve`, { notes });
}

export function previewEnneagramTypeApi(type: number, query: string) {
  return requestClient.post<EnneagramPreview>(`/enneagram-library/types/${type}/preview`, { query });
}

export function publishEnneagramTypeApi(type: number) {
  return requestClient.post<EnneagramPublishResult>(`/enneagram-library/types/${type}/publish`, {});
}

export function getEnneagramVersionsApi(type: number) {
  return requestClient.get<EnneagramVersion[]>(`/enneagram-library/types/${type}/versions`);
}

export function rollbackEnneagramTypeApi(type: number, version: number) {
  return requestClient.post<EnneagramPublishResult>(`/enneagram-library/types/${type}/rollback`, { version });
}

// Verbose aliases keep call sites self-documenting while preserving the
// concise API names used by the rest of the admin client.
export const getEnneagramLibraryOverviewApi = getEnneagramTypesApi;
export const getEnneagramLibraryDetailApi = getEnneagramTypeDetailApi;
export const saveEnneagramLibraryDraftApi = saveEnneagramDraftApi;
export const submitEnneagramLibraryReviewApi = submitEnneagramReviewApi;
export const publishEnneagramLibraryApi = publishEnneagramTypeApi;
export const getEnneagramLibraryVersionsApi = getEnneagramVersionsApi;
export const rollbackEnneagramLibraryApi = rollbackEnneagramTypeApi;
