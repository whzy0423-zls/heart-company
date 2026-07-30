import { requestClient } from '#/api/request';

export type ClassroomAccessLevel =
  | 'inherit'
  | 'login'
  | 'member'
  | 'paid'
  | 'public';
export type ClassroomContentStatus =
  | 'draft'
  | 'failed'
  | 'offline'
  | 'processing'
  | 'published'
  | 'ready';
export type ClassroomContentType = 'audio' | 'video';
export type ClassroomCoverAspectRatio = '1:1' | '16:9' | '9:16';
export type ClassroomCoverSource =
  | 'audio-default'
  | 'generated'
  | 'legacy'
  | 'manual'
  | 'none';
export type ClassroomSeriesStatus = 'draft' | 'offline' | 'published';
export type ClassroomUploadStatus =
  | 'aborted'
  | 'cleaning'
  | 'completed'
  | 'completing'
  | 'expired'
  | 'failed'
  | 'initiated'
  | 'initiating'
  | 'uploading';
export type ClassroomMediaStatus =
  | 'deleted'
  | 'failed'
  | 'pending'
  | 'processing'
  | 'ready'
  | 'uploaded';
export type ClassroomUploadMimeType =
  | 'audio/mp4'
  | 'audio/mpeg'
  | 'audio/x-m4a'
  | 'video/mp4';

export interface ClassroomPage<T> {
  items: T[];
  page: number;
  pageSize: number;
  total: number;
}

export interface ClassroomSeries {
  accessLevel: Exclude<ClassroomAccessLevel, 'inherit'>;
  coverAspectRatio: ClassroomCoverAspectRatio;
  coverUrl: string;
  createdAt: string;
  id: number;
  manualCoverObjectKey: string;
  playbackBlocked: boolean;
  priceCents: number;
  publishedAt?: string;
  sortOrder: number;
  status: ClassroomSeriesStatus;
  summary: string;
  teacherKey: string;
  teacherName: string;
  title: string;
  updatedAt: string;
}

export interface ClassroomContent {
  accessLevel: ClassroomAccessLevel;
  badge: string;
  contentType: ClassroomContentType;
  coverUrl: string;
  coverAspectRatio: ClassroomCoverAspectRatio;
  coverSource: ClassroomCoverSource;
  createdAt: string;
  description: string;
  durationSeconds: number;
  effectiveAccessLevel: Exclude<ClassroomAccessLevel, 'inherit'>;
  effectivePriceCents: number;
  episodeNo: number;
  id: number;
  mediaAssetId?: number;
  manualCoverObjectKey: string;
  playbackBlocked: boolean;
  priceCents: number;
  publishedAt?: string;
  purchaseTarget?: 'content' | 'series';
  recordedAt?: string;
  seriesId?: number;
  showAsStandalone: boolean;
  sortOrder: number;
  status: ClassroomContentStatus;
  tags: string[];
  teacherKey: string;
  teacherName: string;
  title: string;
  updatedAt: string;
}

export interface ClassroomSeriesCreatePayload {
  coverAssetId?: number;
  coverUrl?: string;
  sortOrder?: number;
  summary?: string;
  teacherKey?: string;
  teacherName?: string;
  title: string;
}

export interface ClassroomSeriesUpdatePayload extends ClassroomSeriesCreatePayload {
  expectedUpdatedAt: string;
}

export interface ClassroomContentCreatePayload {
  badge?: string;
  contentType: ClassroomContentType;
  coverUrl?: string;
  coverAspectRatio?: ClassroomCoverAspectRatio;
  description?: string;
  durationSeconds?: number;
  episodeNo?: number;
  recordedAt?: string;
  seriesId?: number;
  showAsStandalone?: boolean;
  sortOrder?: number;
  tags?: string[];
  teacherKey?: string;
  teacherName?: string;
  title: string;
}

export interface ClassroomContentUpdatePayload extends ClassroomContentCreatePayload {
  expectedUpdatedAt: string;
}

export interface ClassroomUploadTask {
  attemptCount: number;
  cleanupStatus: 'cleaned' | 'failed' | 'pending' | 'retained' | string;
  contentId: number;
  createdAt: string;
  expectedSize: number;
  originalFilename: string;
  expectedChecksum: string;
  completedParts?: number;
  completedBytes?: number;
  totalBytes?: number;
  progressPercent?: number;
  expiresAt: string;
  failureReason?: string;
  id: number;
  maxParts: number;
  mediaAssetId?: number;
  partSize: number;
  status: ClassroomUploadStatus;
  updatedAt: string;
}

export interface ClassroomUploadProgress extends ClassroomUploadTask {
  completedBytes: number;
  completedParts: number;
  progressPercent: number;
  totalParts: number;
}

export interface ClassroomUploadInitiatePayload {
  checksum: string;
  contentId: number;
  contentType: ClassroomUploadMimeType;
  filename: string;
  sizeBytes: number;
}

export interface ClassroomUploadInitiateResult {
  task: ClassroomUploadTask;
}

export interface ClassroomUploadCompleteResult {
  content: {
    durationSeconds: number;
    id: number;
    mediaAssetId?: number;
    status: ClassroomContentStatus;
  };
  media: {
    contentType: ClassroomContentType;
    durationSeconds: number;
    height: number;
    id: number;
    sizeBytes: number;
    status: ClassroomMediaStatus;
    width: number;
  };
  task: ClassroomUploadTask;
}

export interface ClassroomActionPayload {
  expectedUpdatedAt: string;
  reason?: string;
}

export interface ClassroomPricePayload extends ClassroomActionPayload {
  accessLevel: ClassroomAccessLevel;
  priceCents: number;
}

export interface ClassroomSeriesPricePayload extends ClassroomActionPayload {
  accessLevel: Exclude<ClassroomAccessLevel, 'inherit'>;
  priceCents: number;
}

export interface ClassroomSignedPart {
  expiresAt: string;
  partNumber: number;
  url: string;
}

export interface ClassroomCompletedPart {
  etag: string;
  partNumber: number;
}

export class ClassroomApiError extends Error {
  constructor(
    message: string,
    public readonly code?: string | number,
    public readonly status?: number,
  ) {
    super(message);
    this.name = 'ClassroomApiError';
  }
}

export function normalizeClassroomError(error: unknown): ClassroomApiError {
  if (error instanceof ClassroomApiError) return error;
  const source = (error ?? {}) as {
    code?: number | string;
    message?: string;
    response?: {
      data?: { code?: number | string; error?: string; message?: string };
      status?: number;
    };
  };
  const data = source.response?.data;
  return new ClassroomApiError(
    data?.message || data?.error || source.message || '老师课堂请求失败',
    data?.code ?? source.code,
    source.response?.status,
  );
}

function classroomRequest<T>(request: Promise<T>): Promise<T> {
  return request.catch((error: unknown) => {
    throw normalizeClassroomError(error);
  });
}

export function getClassroomSeriesApi(params?: {
  accessLevel?: ClassroomAccessLevel;
  page?: number;
  pageSize?: number;
  status?: ClassroomSeriesStatus;
}) {
  return classroomRequest(
    requestClient.get<ClassroomPage<ClassroomSeries>>(
      '/admin/classroom/series',
      { params },
    ),
  );
}

export function getClassroomSeriesDetailApi(id: number) {
  return classroomRequest(
    requestClient.get<ClassroomSeries>(`/admin/classroom/series/${id}`),
  );
}

export function createClassroomSeriesApi(data: ClassroomSeriesCreatePayload) {
  return classroomRequest(
    requestClient.post<ClassroomSeries>('/admin/classroom/series', data),
  );
}

export function updateClassroomSeriesApi(
  id: number,
  data: ClassroomSeriesUpdatePayload,
) {
  return classroomRequest(
    requestClient.put<ClassroomSeries>(`/admin/classroom/series/${id}`, data),
  );
}

export function publishClassroomSeriesApi(
  id: number,
  data: ClassroomActionPayload,
) {
  return classroomRequest(
    requestClient.post<ClassroomSeries>(
      `/admin/classroom/series/${id}/publish`,
      data,
    ),
  );
}

export function offlineClassroomSeriesApi(
  id: number,
  data: ClassroomActionPayload,
) {
  return classroomRequest(
    requestClient.post<ClassroomSeries>(
      `/admin/classroom/series/${id}/offline`,
      data,
    ),
  );
}

export function setClassroomSeriesPriceApi(
  id: number,
  data: ClassroomSeriesPricePayload,
) {
  return classroomRequest(
    requestClient.post<ClassroomSeries>(
      `/admin/classroom/series/${id}/price`,
      data,
    ),
  );
}

export function setClassroomSeriesPlaybackBlockedApi(
  id: number,
  blocked: boolean,
  expectedUpdatedAt: string,
  reason?: string,
) {
  return classroomRequest(
    requestClient.post<ClassroomSeries>(
      `/admin/classroom/series/${id}/playback-blocked`,
      { blocked, expectedUpdatedAt, reason },
    ),
  );
}

export function uploadClassroomSeriesCoverApi(
  id: number,
  file: File,
  expectedUpdatedAt: string,
) {
  const formData = new FormData();
  formData.set('file', file);
  formData.set('expectedUpdatedAt', expectedUpdatedAt);
  return classroomRequest(
    requestClient.post<ClassroomSeries>(
      `/admin/classroom/series/${id}/cover`,
      formData,
    ),
  );
}

export function deleteClassroomSeriesCoverApi(
  id: number,
  expectedUpdatedAt: string,
) {
  return classroomRequest(
    requestClient.delete<ClassroomSeries>(
      `/admin/classroom/series/${id}/cover`,
      { params: { expectedUpdatedAt } },
    ),
  );
}

export function setClassroomSeriesCoverSettingsApi(
  id: number,
  coverAspectRatio: ClassroomCoverAspectRatio,
  expectedUpdatedAt: string,
) {
  return classroomRequest(
    requestClient.put<ClassroomSeries>(
      `/admin/classroom/series/${id}/cover-settings`,
      { coverAspectRatio, expectedUpdatedAt },
    ),
  );
}

export function getClassroomContentsApi(params?: {
  contentType?: ClassroomContentType;
  page?: number;
  pageSize?: number;
  seriesId?: number;
  standaloneOnly?: boolean;
  status?: ClassroomContentStatus;
}) {
  return classroomRequest(
    requestClient.get<ClassroomPage<ClassroomContent>>(
      '/admin/classroom/contents',
      { params },
    ),
  );
}

export function getClassroomContentDetailApi(id: number) {
  return classroomRequest(
    requestClient.get<ClassroomContent>(`/admin/classroom/contents/${id}`),
  );
}

export function createClassroomContentApi(data: ClassroomContentCreatePayload) {
  return classroomRequest(
    requestClient.post<ClassroomContent>('/admin/classroom/contents', data),
  );
}

export function updateClassroomContentApi(
  id: number,
  data: ClassroomContentUpdatePayload,
) {
  return classroomRequest(
    requestClient.put<ClassroomContent>(
      `/admin/classroom/contents/${id}`,
      data,
    ),
  );
}

export function publishClassroomContentApi(
  id: number,
  data: ClassroomActionPayload,
) {
  return classroomRequest(
    requestClient.post<ClassroomContent>(
      `/admin/classroom/contents/${id}/publish`,
      data,
    ),
  );
}

export function offlineClassroomContentApi(
  id: number,
  data: ClassroomActionPayload,
) {
  return classroomRequest(
    requestClient.post<ClassroomContent>(
      `/admin/classroom/contents/${id}/offline`,
      data,
    ),
  );
}

export function setClassroomContentPriceApi(
  id: number,
  data: ClassroomPricePayload,
) {
  return classroomRequest(
    requestClient.post<ClassroomContent>(
      `/admin/classroom/contents/${id}/price`,
      data,
    ),
  );
}

export function setClassroomContentPlaybackBlockedApi(
  id: number,
  blocked: boolean,
  expectedUpdatedAt: string,
  reason?: string,
) {
  return classroomRequest(
    requestClient.post<ClassroomContent>(
      `/admin/classroom/contents/${id}/playback-blocked`,
      { blocked, expectedUpdatedAt, reason },
    ),
  );
}

export function uploadClassroomContentCoverApi(
  id: number,
  file: File,
  expectedUpdatedAt: string,
) {
  const formData = new FormData();
  formData.set('file', file);
  formData.set('expectedUpdatedAt', expectedUpdatedAt);
  return classroomRequest(
    requestClient.post<ClassroomContent>(
      `/admin/classroom/contents/${id}/cover`,
      formData,
    ),
  );
}

export function deleteClassroomContentCoverApi(
  id: number,
  expectedUpdatedAt: string,
) {
  return classroomRequest(
    requestClient.delete<ClassroomContent>(
      `/admin/classroom/contents/${id}/cover`,
      { params: { expectedUpdatedAt } },
    ),
  );
}

export function setClassroomContentCoverSettingsApi(
  id: number,
  coverAspectRatio: ClassroomCoverAspectRatio,
  expectedUpdatedAt: string,
) {
  return classroomRequest(
    requestClient.put<ClassroomContent>(
      `/admin/classroom/contents/${id}/cover-settings`,
      { coverAspectRatio, expectedUpdatedAt },
    ),
  );
}

export function getClassroomUploadTasksApi(params?: {
  page?: number;
  pageSize?: number;
}) {
  return classroomRequest(
    requestClient.get<ClassroomPage<ClassroomUploadTask>>(
      '/admin/classroom/upload-tasks',
      { params },
    ),
  );
}

export function initiateClassroomUploadApi(
  data: ClassroomUploadInitiatePayload,
) {
  return classroomRequest(
    requestClient.post<ClassroomUploadInitiateResult>(
      '/admin/classroom/uploads/initiate',
      data,
    ),
  );
}

export function signClassroomUploadPartApi(id: number, partNumber: number) {
  return classroomRequest(
    requestClient.post<ClassroomSignedPart>(
      `/admin/classroom/uploads/${id}/parts/${partNumber}/sign`,
      {},
    ),
  );
}

export function reportClassroomUploadProgressApi(
  id: number,
  data: { completedParts: number; completedBytes: number },
) {
  return classroomRequest(
    requestClient.post<ClassroomUploadTask>(
      `/admin/classroom/uploads/${id}/progress`,
      data,
    ),
  );
}

export function completeClassroomUploadApi(
  id: number,
  parts: ClassroomCompletedPart[],
) {
  return classroomRequest(
    requestClient.post<ClassroomUploadCompleteResult>(
      `/admin/classroom/uploads/${id}/complete`,
      { parts },
      { timeout: 180_000 },
    ),
  );
}

export function deleteClassroomSeriesApi(
  id: number,
  expectedUpdatedAt: string,
  reason?: string,
) {
  return classroomRequest(
    requestClient.delete<{ deleted: boolean }>(
      `/admin/classroom/series/${id}`,
      { params: { expectedUpdatedAt, reason } },
    ),
  );
}

export function deleteClassroomContentApi(
  id: number,
  expectedUpdatedAt: string,
  reason?: string,
) {
  return classroomRequest(
    requestClient.delete<{ deleted: boolean }>(
      `/admin/classroom/contents/${id}`,
      { params: { expectedUpdatedAt, reason } },
    ),
  );
}

export function abortClassroomUploadApi(id: number) {
  return classroomRequest(
    requestClient.post<ClassroomUploadTask>(
      `/admin/classroom/uploads/${id}/abort`,
      {},
    ),
  );
}
