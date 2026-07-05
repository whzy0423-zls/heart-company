import { beforeEach, describe, expect, it, vi } from 'vitest';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

const mocks = vi.hoisted(() => ({
  accessCodes: [] as string[],
  routerPush: vi.fn(),
}));

vi.mock('ant-design-vue', async () => import('#/test-utils/antd-stubs'));

vi.mock('@vben/common-ui', () => ({
  Page: {
    name: 'Page',
    template: '<main><slot /></main>',
  },
}));

vi.mock('@vben/stores', () => ({
  useAccessStore: () => ({
    get accessCodes() {
      return mocks.accessCodes;
    },
  }),
}));

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: mocks.routerPush }),
}));

vi.mock('#/api', () => ({
  getAppAnalyticsOverviewApi: vi.fn(),
}));

import { getAppAnalyticsOverviewApi } from '#/api';

import AppDashboard from './app.vue';

describe('App dashboard P2 permission behavior', () => {
  beforeEach(() => {
    mocks.accessCodes = [];
    mocks.routerPush.mockReset();
    vi.mocked(getAppAnalyticsOverviewApi).mockReset();
    vi.mocked(getAppAnalyticsOverviewApi).mockResolvedValue({
      activeUsers: 1,
      recentExtractedUsers: [
        {
          id: 2,
          lastMemoryAt: '2026/07/04 10:00:00',
          memoryCount: 3,
          phone: '13800000002',
          primaryType: 5,
        },
      ],
      recentUsers: [
        {
          id: 1,
          createTime: '2026/07/04 09:00:00',
          phone: '13800000001',
        },
      ],
      totalUsers: 2,
    });
  });

  it('hides App customer and 360 jump buttons when list permissions are missing', async () => {
    const wrapper = mountVueComponent(AppDashboard);
    await flushVuePromises();

    expect(wrapper.text()).not.toContain('查看 App 客户');
    expect(wrapper.text()).not.toContain('360');
    wrapper.unmount();
  });

  it('shows App customer and 360 jump buttons when the corresponding permissions exist', async () => {
    mocks.accessCodes = ['Customer:App:List', 'Customer:UserInsights:List'];

    const wrapper = mountVueComponent(AppDashboard);
    await flushVuePromises();

    expect(wrapper.text()).toContain('查看 App 客户');
    expect(wrapper.text()).toContain('360');
    wrapper.unmount();
  });

  it('falls back to recentMemoryUsers when the backend has not sent recentExtractedUsers yet', async () => {
    mocks.accessCodes = ['Customer:UserInsights:List'];
    vi.mocked(getAppAnalyticsOverviewApi).mockResolvedValue({
      activeUsers: 1,
      recentMemoryUsers: [
        {
          id: 9,
          lastMemoryAt: '2026/07/04 11:00:00',
          memoryCount: 2,
          phone: '13800000009',
          primaryType: 6,
        },
      ],
      recentUsers: [],
      totalUsers: 1,
    });

    const wrapper = mountVueComponent(AppDashboard);
    await flushVuePromises();

    expect(wrapper.text()).toContain('13800000009');
    expect(wrapper.text()).toContain('6号');
    wrapper.unmount();
  });

  it('opens user insights with userId and open marker from dashboard 360 action', async () => {
    mocks.accessCodes = ['Customer:UserInsights:List'];

    const wrapper = mountVueComponent(AppDashboard);
    await flushVuePromises();

    wrapper.button('360')?.click();

    expect(mocks.routerPush).toHaveBeenCalledWith({
      path: '/customer/user-insights',
      query: { keyword: '13800000001', open: '1', userId: '1' },
    });
    wrapper.unmount();
  });

  it('keeps the latest refresh result when an older request fails later', async () => {
    let rejectFirst!: (error: Error) => void;
    vi.mocked(getAppAnalyticsOverviewApi)
      .mockReturnValueOnce(
        new Promise((_, reject) => {
          rejectFirst = reject;
        }) as any,
      )
      .mockResolvedValueOnce({
        activeUsers: 2,
        recentUsers: [{ id: 3, phone: '13900000003' }],
        totalUsers: 3,
      });

    const wrapper = mountVueComponent(AppDashboard);
    await flushVuePromises();
    wrapper.button('刷新')?.click();
    await flushVuePromises();
    rejectFirst(new Error('old request failed'));
    await flushVuePromises();

    expect(wrapper.text()).toContain('13900000003');
    expect(wrapper.text()).not.toContain('App 数据概览加载失败');
    wrapper.unmount();
  });
});
