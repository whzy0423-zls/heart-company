import { requestClient } from '#/api/request';

export type AppAgentDiscountAudience = 'agent_self' | 'invited_user';
export type AppAgentDiscountMode = 'amount_off' | 'percent_off';

export interface AppAgentDiscountRule {
  audience: AppAgentDiscountAudience;
  enabled: boolean;
  mode: AppAgentDiscountMode;
  productId: string;
  value: number;
}

export interface AppAgentDiscountConfig {
  rules: AppAgentDiscountRule[];
}

export function getAppAgentDiscountsApi() {
  return requestClient.get<AppAgentDiscountConfig>(
    '/admin/app-agent-discounts',
  );
}

export function updateAppAgentDiscountsApi(data: AppAgentDiscountConfig) {
  return requestClient.put<AppAgentDiscountConfig>(
    '/admin/app-agent-discounts',
    data,
  );
}
