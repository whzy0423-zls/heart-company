import { requestClient } from '#/api/request';

export interface PushNotification {
  content: string;
  createTime: string;
  deepLink: string;
  errorMessage?: string;
  id: number;
  operator: string;
  sentCount: number;
  status: string;
  targetType: string;
  targetValue: string;
  title: string;
}

export interface PushListResult {
  items: PushNotification[];
  total: number;
}

export interface PushAudienceCountResult {
  deviceCount: number;
  targetType?: string;
  targetValue?: string;
  userCount: number;
}

export interface PushSendParams {
  content: string;
  deepLink?: string;
  targetType?: string;
  targetValue?: string;
  title: string;
}

export interface PushSendResult {
  message?: string;
  recordId: number;
  status: string;
}

export interface DailyQuizPushStats {
  answeredUsers: number;
  completedUsers: number;
  date: string;
  eligibleUsers: number;
  pendingReassessmentReports: number;
  pushed: boolean;
  pushedUsers: number;
  totalAnswers: number;
}

export interface DailyQuizPushRecord {
  answeredCount: number;
  appUserId: number;
  batchId: number;
  cardId: number;
  cardName: string;
  completed: boolean;
  completedAt: string;
  nickname: string;
  phone: string;
  pushSentAt: string;
  pushed: boolean;
  quizDate: string;
  status?: string;
}

export interface DailyQuizPushRecordListResult {
  items: DailyQuizPushRecord[];
  total: number;
}

export function getPushListApi(params?: { page?: number; pageSize?: number }) {
  return requestClient.get<PushListResult>('/push/list', { params });
}

export function getPushAudienceCountApi(params?: {
  targetType?: string;
  targetValue?: string;
}) {
  return requestClient.get<PushAudienceCountResult>('/push/audience-count', {
    params,
  });
}

export function sendPushApi(data: PushSendParams) {
  return requestClient.post<PushSendResult>('/push/send', data);
}

export function getDailyQuizPushStatsApi(params?: { date?: string }) {
  return requestClient.get<DailyQuizPushStats>('/push/daily-quiz/stats', {
    params,
  });
}

export function getDailyQuizPushRecordsApi(params?: {
  date?: string;
  page?: number;
  pageSize?: number;
}) {
  return requestClient.get<DailyQuizPushRecordListResult>(
    '/push/daily-quiz/records',
    { params },
  );
}
