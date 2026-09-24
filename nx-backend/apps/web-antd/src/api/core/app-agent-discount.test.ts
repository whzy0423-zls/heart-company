import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  get: vi.fn(),
  put: vi.fn(),
}));

vi.mock('#/api/request', () => ({
  requestClient: { get: mocks.get, put: mocks.put },
}));

describe('App agent discount API', () => {
  beforeEach(() => {
    mocks.get.mockReset();
    mocks.put.mockReset();
  });

  it('reads and saves both audience rules through the admin endpoint', async () => {
    const { getAppAgentDiscountsApi, updateAppAgentDiscountsApi } =
      await import('./app-agent-discount');
    const payload = {
      rules: [
        {
          audience: 'agent_self' as const,
          enabled: true,
          mode: 'percent_off' as const,
          productId: 'vip_month',
          value: 1500,
        },
        {
          audience: 'invited_user' as const,
          enabled: true,
          mode: 'amount_off' as const,
          productId: 'vip_month',
          value: 500,
        },
      ],
    };

    await getAppAgentDiscountsApi();
    await updateAppAgentDiscountsApi(payload);

    expect(mocks.get).toHaveBeenCalledWith('/admin/app-agent-discounts');
    expect(mocks.put).toHaveBeenCalledWith(
      '/admin/app-agent-discounts',
      payload,
    );
  });
});
