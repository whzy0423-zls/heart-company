import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}));

vi.mock('#/api/request', () => ({
  requestClient: { get: mocks.get, post: mocks.post, put: vi.fn() },
}));

describe('App customer trial credit API', () => {
  beforeEach(() => {
    mocks.get.mockReset();
    mocks.post.mockReset();
  });

  it('uses the user-scoped list, grant, and revoke endpoints', async () => {
    const {
      getAppTrialCreditsApi,
      grantAppTrialCreditsApi,
      revokeAppTrialCreditsApi,
    } = await import('./app-customer');

    await getAppTrialCreditsApi(42);
    await grantAppTrialCreditsApi(42, {
      amount: 20,
      expiresAt: '2026-09-17T10:00:00+08:00',
      idempotencyKey: 'grant-42-1',
      reason: '分享活动奖励',
    });
    await revokeAppTrialCreditsApi(42, 7);

    expect(mocks.get).toHaveBeenCalledWith(
      '/app-users/42/trial-chat-credits',
    );
    expect(mocks.post).toHaveBeenNthCalledWith(
      1,
      '/app-users/42/trial-chat-credits',
      expect.objectContaining({ amount: 20, idempotencyKey: 'grant-42-1' }),
    );
    expect(mocks.post).toHaveBeenNthCalledWith(
      2,
      '/app-users/42/trial-chat-credits/7/revoke',
    );
  });
});
