import { requestClient } from '#/api/request';

export interface TheoryLibrarySummary {
  activatedAt?: string;
  activeReleaseId?: number;
  cardCount: number;
  chunkCount: number;
  currentVersion: number;
  description: string;
  id: number;
  key: string;
  name: string;
  publishedCards: number;
  status: string;
}

export interface TheoryLibraryCard {
  canonicalKey: string;
  canonicalName: string;
  cardKind: string;
  domain: string;
  id: number;
  status: string;
  summary: string;
  updateTime: string;
  version: number;
}

export interface TheoryLibraryPublishResult {
  cardCount: number;
  chunkCount: number;
  libraryId: number;
  releaseId: number;
  releaseVersion: number;
}

export function getTheoryLibrariesApi() {
  return requestClient.get<{ libraries: TheoryLibrarySummary[] }>(
    '/theory-libraries',
  );
}

export function getTheoryLibraryCardsApi(libraryId: number) {
  return requestClient.get<TheoryLibraryCard[]>(
    `/theory-libraries/${libraryId}/cards`,
  );
}

export function publishTheoryLibraryApi(libraryId: number) {
  return requestClient.post<TheoryLibraryPublishResult>(
    `/theory-libraries/${libraryId}/publish`,
    {},
  );
}
