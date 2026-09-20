import { requestClient } from '#/api/request';

export interface AppPlan {
  badge: string;
  cardLimit: number;
  code: string;
  companionEnabled: boolean;
  dailyChatLimit: number;
  deepChatEnabled: boolean;
  durationDays: number;
  enabled: boolean;
  features: string[];
  memberPosterEnabled: boolean;
  name: string;
  originalPriceCents: number;
  priceCents: number;
  sortOrder: number;
  storyMonthlyLimit: number;
  subtitle: string;
  planLevel: 'free' | 'vip' | 'svip';
  billingCycle: 'none' | 'month' | 'quarter' | 'year';
  featureFlags?: Record<string, boolean>;
  limits?: Record<string, number>;
}

export function getAppPlansApi() {
  return requestClient.get<AppPlan[]>('/admin/app-plans');
}

export function updateAppPlanApi(code: string, data: AppPlan) {
  return requestClient.put<AppPlan>(`/admin/app-plans/${code}`, data);
}
