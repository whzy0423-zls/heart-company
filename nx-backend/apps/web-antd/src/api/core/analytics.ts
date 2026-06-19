import { requestClient } from '#/api/request';

export interface AnalyticsOverview {
  dueFollowups: number;
  followupItems: FollowupItem[];
  overdueFollowups: number;
  pendingLeads: number;
  rangeLeads: number;
  rangeVisits: number;
  series: AnalyticsSeriesPoint[];
  todayFollowups: number;
  todayLeads: number;
  todayVisits: number;
  totalLeads: number;
  totalVisits: number;
}

export interface AnalyticsSeriesPoint {
  date: string;
  leads: number;
  visits: number;
}

export interface FollowupItem {
  contact: string;
  contactType: string;
  followStatus: string;
  id: string;
  interest: string;
  name: string;
  nextFollowTime: string;
  owner: string;
}

export function getAnalyticsOverviewApi(params?: {
  endDate?: string;
  startDate?: string;
}) {
  return requestClient.get<AnalyticsOverview>('/analytics/overview', {
    params,
  });
}
