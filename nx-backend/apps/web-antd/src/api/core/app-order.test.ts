import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}));

vi.mock('#/api/request', () => ({
  requestClient: mocks,
}));

describe('app order api', () => {
  beforeEach(() => {
    mocks.get.mockReset();
    mocks.post.mockReset();
  });

  it('reconciles an online order through the admin endpoint', async () => {
    const { reconcileAppOrderApi } = await import('./app-order');

    await reconcileAppOrderApi(42);

    expect(mocks.post).toHaveBeenCalledWith('/app-orders/42/reconcile');
  });
});
