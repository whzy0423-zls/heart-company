import type {
  AppAnalyticsDistribution,
  AppAnalyticsDistributionItem,
  AppAnalyticsOverview,
  AppAnalyticsRecentUser,
} from '#/api/core/app-analytics';

export interface AppAnalyticsStatCard {
  color: string;
  label: string;
  value: number;
}

export interface AppAnalyticsDistributionRow extends AppAnalyticsDistributionItem {
  percent: string;
}

export interface NormalizedRecentUser extends AppAnalyticsRecentUser {
  title: string;
}

const statMeta = [
  { color: '#2563eb', key: 'totalUsers', label: '累计用户' },
  { color: '#16a34a', key: 'newUsersToday', label: '今日新增' },
  { color: '#f59e0b', key: 'activeUsers', label: '活跃用户' },
  { color: '#8b5cf6', key: 'extractedUsers', label: '已提炼用户' },
] as const;

export function appAnalyticsStatCards(
  overview: Pick<
    Partial<AppAnalyticsOverview>,
    'activeUsers' | 'extractedUsers' | 'newUsersToday' | 'totalUsers'
  >,
): AppAnalyticsStatCard[] {
  return statMeta.map((item) => ({
    color: item.color,
    label: item.label,
    value: Number(overview[item.key] ?? 0),
  }));
}

export function formatAppAnalyticsPercent(count: number, total: number) {
  if (total <= 0) return '0%';
  const value = (count / total) * 100;
  return `${Number.isInteger(value) ? value.toFixed(0) : value.toFixed(1)}%`;
}

export function distributionRows(
  items: AppAnalyticsDistribution = [],
  labels: Record<string, string> = {},
): AppAnalyticsDistributionRow[] {
  const normalized = Array.isArray(items)
    ? items
    : Object.entries(items).map(([value, count]) => ({
        count,
        label: labels[value] || value || '-',
        value,
      }));
  const total = normalized.reduce(
    (sum, item) => sum + Number(item.count || 0),
    0,
  );
  return normalized.map((item) => ({
    ...item,
    count: Number(item.count || 0),
    label: item.label || labels[item.value] || item.value || '-',
    percent: formatAppAnalyticsPercent(Number(item.count || 0), total),
  }));
}

export function normalizeRecentRows(
  items: AppAnalyticsRecentUser[] = [],
): NormalizedRecentUser[] {
  return items.map((item) => ({
    ...item,
    title: item.nickname?.trim() || item.phone?.trim() || `用户 ${item.id}`,
  }));
}

export function emptyAppAnalyticsOverview(): AppAnalyticsOverview {
  return {
    activeUsers: 0,
    cards: 0,
    chatMessages: 0,
    chatSessions: 0,
    compatibilityReports: 0,
    disabledUsers: 0,
    extractedUsers: 0,
    memberDistribution: {},
    memberLevelDistribution: [],
    memberUsers: 0,
    memories: 0,
    newUsersToday: 0,
    quizSubmissions: 0,
    recentExtractedUsers: [],
    recentMemoryUsers: [],
    recentUsers: [],
    statusDistribution: {},
    totalUsers: 0,
  };
}
