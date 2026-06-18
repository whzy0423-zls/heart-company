import type { UserInfo } from '@vben/types';

import { requestClient } from '#/api/request';

export interface UpdateUserProfileParams {
  avatar?: string;
  email?: string;
  phone?: string;
  realName: string;
  remark?: string;
  username: string;
}

/**
 * 获取用户信息
 */
export async function getUserInfoApi() {
  return requestClient.get<UserInfo>('/user/info');
}

/**
 * 更新当前登录用户资料
 */
export async function updateUserProfileApi(data: UpdateUserProfileParams) {
  return requestClient.put<UserInfo>('/user/profile', data);
}
