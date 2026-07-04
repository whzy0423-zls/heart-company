import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  basePost: vi.fn(),
  requestPost: vi.fn(),
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
      withCredentials: true,
    });
    expect(token).toBe('jwt-token');
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
