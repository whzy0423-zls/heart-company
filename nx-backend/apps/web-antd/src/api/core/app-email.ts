import { requestClient } from '#/api/request';

export interface AppEmailConfig {
  enabled: boolean;
  host: string;
  port: number;
  username: string;
  password?: string;
  from: string;
  fromName: string;
}

export function getAppEmailConfigApi() {
  return requestClient.get<AppEmailConfig>('/app/email-config');
}

export function updateAppEmailConfigApi(data: AppEmailConfig) {
  return requestClient.put<AppEmailConfig>('/app/email-config', data);
}
