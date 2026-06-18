import { requestClient } from '#/api/request';

export interface AnalyticsOverview {
  rangeLeads: number;
  rangeVisits: number;
  series: AnalyticsSeriesPoint[];
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

export function getAnalyticsOverviewApi(params?: {
  endDate?: string;
  startDate?: string;
}) {
  return requestClient.get<AnalyticsOverview>('/analytics/overview', {
    params,
  });
}
