import { requestClient } from '#/api/request';

export type TeacherReviewStatus =
  | 'draft'
  | 'offline'
  | 'pending_review'
  | 'published'
  | 'rejected';
export type TeacherFeedType = 'course' | 'daily';

export interface TeacherContactConfig {
  description?: string;
  qrCode?: string;
  wechat?: string;
  workTime?: string;
}

export interface TeacherOfflineConfig {
  city?: string;
  description?: string;
  enabled?: boolean;
  types?: string[];
}

export interface TeacherProfile {
  appUserId?: number;
  avatar?: string;
  bio?: string;
  cover?: string;
  createdAt?: string;
  customerServiceConfig?: TeacherContactConfig;
  customerService?: TeacherContactConfig;
  detailIntro?: string;
  enabled: boolean;
  experience?: string;
  introVideoUrl?: string;
  key: string;
  name: string;
  offlineServiceConfig?: TeacherOfflineConfig;
  offlineService?: TeacherOfflineConfig;
  shortIntro?: string;
  showInDrawer: boolean;
  showOnHome: boolean;
  sortOrder: number;
  tags: string[];
  expertise: string[];
  title?: string;
  updatedAt?: string;
}

export interface TeacherPage<T> {
  items: T[];
  page: number;
  pageSize: number;
  total: number;
}

export type TeacherCreatePayload = Partial<Omit<TeacherProfile, 'key'>> & {
  key: string;
  name: string;
};
export type TeacherUpdatePayload = Partial<Omit<TeacherProfile, 'key'>>;
export interface TeacherBindingPayload {
  appUserId: number;
  enabled: boolean;
}
export interface TeacherReviewItem {
  contentId?: number;
  contentType?: 'audio' | 'video';
  feedType?: TeacherFeedType;
  id: number;
  replacedContentId?: number;
  reviewReason?: string;
  reviewStatus: TeacherReviewStatus;
  seriesId?: number;
  teacherKey: string;
  teacherName: string;
  title: string;
  updatedAt: string;
}

export interface TeacherReviewQuery {
  feedType?: TeacherFeedType;
  page?: number;
  pageSize?: number;
  reviewStatus?: TeacherReviewStatus;
  status?: TeacherReviewStatus;
  teacherKey?: string;
}

export function getTeachersApi(params?: {
  enabled?: boolean;
  page?: number;
  pageSize?: number;
  keyword?: string;
}) {
  return requestClient.get<TeacherPage<TeacherProfile>>('/admin/teachers', {
    params,
  });
}

export function getTeacherApi(key: string) {
  return requestClient.get<TeacherProfile>(`/admin/teachers/${encodeURIComponent(key)}`);
}

export function createTeacherApi(data: TeacherCreatePayload) {
  return requestClient.post<TeacherProfile>('/admin/teachers', data);
}

export function updateTeacherApi(key: string, data: TeacherUpdatePayload) {
  return requestClient.put<TeacherProfile>(
    `/admin/teachers/${encodeURIComponent(key)}`,
    data,
  );
}

export function setTeacherEnabledApi(key: string, enabled: boolean) {
  return requestClient.put<TeacherProfile>(
    `/admin/teachers/${encodeURIComponent(key)}/status`,
    { enabled },
  );
}

export function bindTeacherUserApi(key: string, data: TeacherBindingPayload) {
  return requestClient.put<TeacherProfile>(
    `/admin/teachers/${encodeURIComponent(key)}/binding`,
    data,
  );
}

export function getTeacherReviewQueueApi(params?: TeacherReviewQuery) {
  return requestClient.get<TeacherPage<TeacherReviewItem>>(
    '/admin/teacher-reviews',
    { params },
  );
}

export function approveTeacherReviewApi(
  id: number,
  data: { expectedUpdatedAt?: string } = {},
) {
  return requestClient.post<TeacherReviewItem>(
    `/admin/teacher-reviews/${id}/approve`,
    data,
  );
}

export function rejectTeacherReviewApi(id: number, data: { reason: string }) {
  return requestClient.post<TeacherReviewItem>(
    `/admin/teacher-reviews/${id}/reject`,
    data,
  );
}

export function offlineTeacherReviewApi(id: number, data: { reason?: string } = {}) {
  return requestClient.post<TeacherReviewItem>(
    `/admin/teacher-reviews/${id}/offline`,
    data,
  );
}
