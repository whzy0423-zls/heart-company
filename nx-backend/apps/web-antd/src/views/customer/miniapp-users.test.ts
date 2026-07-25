import { beforeEach, describe, expect, it, vi } from 'vitest';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

const mocks = vi.hoisted(() => ({
	detail: vi.fn(), list: vi.fn(), push: vi.fn(), routeQuery: {} as Record<string, string>,
}));

vi.mock('ant-design-vue', async () => import('#/test-utils/antd-stubs'));
vi.mock('vue-router', () => ({
	useRoute: () => ({ query: mocks.routeQuery }),
	useRouter: () => ({ push: mocks.push }),
}));
vi.mock('../system/components/page-shell.vue', () => ({ default: { name: 'PageShell', template: '<main><slot /></main>' } }));
vi.mock('#/api', () => ({ getMiniappCustomerDetailApi: mocks.detail, getMiniappCustomerListApi: mocks.list }));

import MiniappUsers from './miniapp-users.vue';

const user = { id: '7', nickname: '小芯', avatar: '', phone: '138****5678', gender: 'female', mainType: 9, memberLevel: 1, channel: 'wechat', scene: 'qr', createTime: '2026/07/24 10:00:00', lastLoginAt: '2026/07/24 10:00:00' };
const detail = { user, testRecords: { items: [{ id: '11', gender: 'female', resultType: 9, secondType: 1, scores: {}, centers: [], createTime: '2026/07/24 10:01:00' }], total: 1 }, bookings: { items: [{ id: '13', signupId: '91', kind: 'consult', contactName: '张三', phone: '138****5678', intent: '', preferredTime: '', message: '', status: 'pending', createTime: '2026/07/24 10:02:00' }], total: 1 } };

describe('miniapp customer page', () => {
  beforeEach(() => {
	mocks.routeQuery = {};
	mocks.push.mockReset(); mocks.list.mockReset(); mocks.detail.mockReset();
	mocks.list.mockResolvedValue({ items: [user], total: 1 });
	mocks.detail.mockResolvedValue(detail);
  });

  it('renders masked users and opens their paged detail', async () => {
	const wrapper = mountVueComponent(MiniappUsers);
	await flushVuePromises();
	expect(wrapper.text()).toContain('138****5678');
	wrapper.button('查看详情')?.click();
	await flushVuePromises();
	expect(mocks.detail).toHaveBeenCalledWith('7', { testPage: 1, testPageSize: 20, bookingPage: 1, bookingPageSize: 20 });
	expect(wrapper.text()).toContain('测评记录');
	expect(wrapper.text()).toContain('关联报名');
	wrapper.button('关联报名')?.click();
	expect(mocks.push).toHaveBeenCalledWith({ path: '/customer/signups', query: { leadId: '91', open: 'detail' } });
	wrapper.unmount();
  });

  it('opens a routed test record and reports a missing user', async () => {
	mocks.routeQuery = { userId: '7', testRecordId: '11', open: 'test' };
	const wrapper = mountVueComponent(MiniappUsers);
	await flushVuePromises();
	expect(mocks.detail).toHaveBeenCalled();
	expect(wrapper.text()).toContain('测评记录 #11');
	wrapper.unmount();

	mocks.detail.mockRejectedValueOnce({ response: { status: 404 } });
	mocks.routeQuery = { userId: '99', open: 'detail' };
	const missing = mountVueComponent(MiniappUsers);
	await flushVuePromises();
	expect(missing.text()).toContain('小程序客户不存在');
	missing.unmount();
  });
});
