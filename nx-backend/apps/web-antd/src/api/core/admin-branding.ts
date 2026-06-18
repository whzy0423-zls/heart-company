import { requestClient } from '#/api/request';

/** 后台品牌配置（名称 / Logo / 启动加载文案） */
export interface AdminBranding {
  loadingText: string;
  logo: string;
  name: string;
}

/** 公开只读：登录前即可拉取（启动屏 / 登录页用）。 */
export function getAdminBrandingApi() {
  return requestClient.get<AdminBranding>('/public/admin-branding');
}

/** 保存后台品牌配置（需登录）。 */
export function updateAdminBrandingApi(data: AdminBranding) {
  return requestClient.put<AdminBranding>('/admin-branding', data);
}
