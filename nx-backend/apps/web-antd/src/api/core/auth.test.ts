import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  basePost: vi.fn(),
  requestPost: vi.fn(),
  accessToken: 'old-jwt-token' as null | string,
}));

vi.mock('@vben/stores', () => ({
  useAccessStore: () => ({
    get accessToken() {
      return mocks.accessToken;
    },
  }),
}));

vi.mock('#/api/request', () => ({
  baseRequestClient: {
    post: mocks.basePost,
  },
  requestClient: {
    get: vi.fn(),
    post: mocks.requestPost,
  },
}));

describe('auth api credentials config', () => {
  beforeEach(() => {
    mocks.basePost.mockReset();
    mocks.requestPost.mockReset();
    mocks.accessToken = 'old-jwt-token';
  });

  it('passes withCredentials as config for refresh token request', async () => {
    mocks.basePost.mockResolvedValue({
      data: {
        code: 0,
        data: 'jwt-token',
        message: 'ok',
      },
    });
    const { refreshTokenApi } = await import('./auth');

    const token = await refreshTokenApi();

    expect(mocks.basePost).toHaveBeenCalledWith('/auth/refresh', undefined, {
      headers: { Authorization: 'Bearer old-jwt-token' },
      withCredentials: true,
    });
    expect(token).toBe('jwt-token');
  });

  it('omits Authorization for refresh when no access token exists', async () => {
    mocks.accessToken = null;
    mocks.basePost.mockResolvedValue({ data: { code: 0, data: 'jwt-token' } });
    const { refreshTokenApi } = await import('./auth');

    await refreshTokenApi();

    expect(mocks.basePost).toHaveBeenCalledWith('/auth/refresh', undefined, {
      headers: {},
      withCredentials: true,
    });
  });

  it('passes withCredentials as config for logout request', async () => {
    const { logoutApi } = await import('./auth');

    await logoutApi();

    expect(mocks.basePost).toHaveBeenCalledWith('/auth/logout', undefined, {
      withCredentials: true,
    });
  });

  it('rejects malformed refresh token responses', async () => {
    mocks.basePost.mockResolvedValue({
      data: {
        code: 0,
        data: { unexpected: true },
      },
    });
    const { refreshTokenApi } = await import('./auth');

    await expect(refreshTokenApi()).rejects.toThrow('Invalid refresh token');
  });
});
