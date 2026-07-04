import { requestClient } from '#/api/request';

export interface AppAnalyticsDistributionItem {
  count: number;
  label?: string;
  value: string;
}

export type AppAnalyticsDistribution =
  | AppAnalyticsDistributionItem[]
  | Record<string, number>;

export interface AppAnalyticsRecentUser {
  createTime?: string;
  id: number | string;
  lastLoginAt?: string;
  lastMemoryAt?: string;
  latestMemory?: string;
  memoryCount?: number;
  memberLevel?: string;
  nickname?: string;
  phone?: string;
  primaryType?: number;
  status?: string;
  updateTime?: string;
}

export interface AppAnalyticsOverview {
  activeUsers: number;
  cards?: number;
  chatMessages?: number;
  chatSessions?: number;
  compatibilityReports?: number;
  disabledUsers?: number;
  extractedUsers?: number;
  memberDistribution?: AppAnalyticsDistribution;
  memberLevelDistribution?: AppAnalyticsDistribution;
  memberUsers?: number;
  memories?: number;
  newUsersToday?: number;
  quizSubmissions?: number;
  recentExtractedUsers?: AppAnalyticsRecentUser[];
  recentMemoryUsers?: AppAnalyticsRecentUser[];
  recentUsers: AppAnalyticsRecentUser[];
  statusDistribution?: AppAnalyticsDistribution;
  totalUsers: number;
}

export function getAppAnalyticsOverviewApi() {
  return requestClient.get<AppAnalyticsOverview>('/app-analytics/overview');
}
