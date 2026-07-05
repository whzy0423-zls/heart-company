import { useAccessStore } from '@vben/stores';

import { baseRequestClient, requestClient } from '#/api/request';

export namespace AuthApi {
  /** 登录接口参数 */
  export interface LoginParams {
    password?: string;
    username?: string;
  }

  /** 登录接口返回值 */
  export interface LoginResult {
    accessToken: string;
  }

  export interface RefreshTokenEnvelope {
    code: number;
    data: string;
    message?: string;
  }

  export interface RefreshTokenHTTPResponse {
    data: RefreshTokenEnvelope | string;
  }
}

export function extractRefreshToken(response: unknown) {
  const candidate =
    typeof response === 'string'
      ? response
      : (response as any)?.data?.data || (response as any)?.data;
  if (typeof candidate !== 'string' || candidate.trim() === '') {
    throw new Error('Invalid refresh token response');
  }
  return candidate;
}

/**
 * 登录
 */
export async function loginApi(data: AuthApi.LoginParams) {
  return requestClient.post<AuthApi.LoginResult>('/auth/login', data);
}

/**
 * 刷新accessToken
 */
export async function refreshTokenApi() {
  const accessStore = useAccessStore();
  const headers: Record<string, string> = {};
  if (accessStore.accessToken) {
    headers.Authorization = `Bearer ${accessStore.accessToken}`;
  }

  const response = await baseRequestClient.post<AuthApi.RefreshTokenHTTPResponse>(
    '/auth/refresh',
    undefined,
    {
      headers,
      withCredentials: true,
    },
  );
  return extractRefreshToken(response);
}

/**
 * 退出登录
 */
export async function logoutApi() {
  return baseRequestClient.post('/auth/logout', undefined, {
    withCredentials: true,
  });
}

/**
 * 获取用户权限码
 */
export async function getAccessCodesApi() {
  return requestClient.get<string[]>('/auth/codes');
}
