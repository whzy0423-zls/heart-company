import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({ get: vi.fn() }));

vi.mock('#/api/request', () => ({ requestClient: { get: mocks.get } }));

describe('miniapp customer api', () => {
  beforeEach(() => mocks.get.mockReset());

  it('uses the protected miniapp customer list and detail endpoints', async () => {
	const { getMiniappCustomerDetailApi, getMiniappCustomerListApi } = await import('./miniapp-customer');
	await getMiniappCustomerListApi({ page: 2, pageSize: 30, keyword: '小芯', channel: 'wechat' });
	await getMiniappCustomerDetailApi('7', { testPage: 2, testPageSize: 5, bookingPage: 3, bookingPageSize: 10 });
	expect(mocks.get).toHaveBeenNthCalledWith(1, '/miniapp/users', { params: { page: 2, pageSize: 30, keyword: '小芯', channel: 'wechat' } });
	expect(mocks.get).toHaveBeenNthCalledWith(2, '/miniapp/users/7', { params: { testPage: 2, testPageSize: 5, bookingPage: 3, bookingPageSize: 10 } });
  });
});
