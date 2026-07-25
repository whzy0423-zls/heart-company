import { requestClient } from '#/api/request';

export type AppReleaseStatus = 'archived' | 'draft' | 'published';
export interface AppRelease {
  appName: string; createdAt: string; fileAvailable: boolean; fileName: string;
  fileSize: number; iconUrl: string; id: number; packageName: string;
  platform: 'android'; publishedAt: null | string; releaseNotes: string;
  sha256: string; status: AppReleaseStatus; versionCode: number;
  versionName: string;
}
export interface AppReleaseListResult {
  current: AppRelease | null; items: AppRelease[]; page: number; pageSize: number;
  total: number; totalFileSize: number;
}
export function getAppReleaseListApi(params: { page: number; pageSize: number }) {
  return requestClient.get<AppReleaseListResult>('/app-releases/list', { params });
}
export function uploadAppReleaseApi(file: File, releaseNotes: string, onUploadProgress?: (event: { loaded: number; total?: number }) => void) {
  return requestClient.upload<AppRelease>('/app-releases/upload', { file, release_notes: releaseNotes }, {
    onUploadProgress,
    timeout: 1_800_000,
  });
}
export function publishAppReleaseApi(id: number) { return requestClient.post<AppRelease>(`/app-releases/${id}/publish`); }
export function archiveAppReleaseApi(id: number) { return requestClient.post<AppRelease>(`/app-releases/${id}/archive`); }
