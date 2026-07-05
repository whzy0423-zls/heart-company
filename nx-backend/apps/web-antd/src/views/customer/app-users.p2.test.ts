import type { AppCustomer } from '#/api';

import { beforeEach, describe, expect, it, vi } from 'vitest';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

const mocks = vi.hoisted(() => ({
  accessCodes: [] as string[],
  routeQuery: undefined as unknown as Record<string, string>,
  routerPush: vi.fn(),
}));

vi.mock('ant-design-vue', async () => import('#/test-utils/antd-stubs'));

vi.mock('@vben/stores', () => ({
  useAccessStore: () => ({
    get accessCodes() {
      return mocks.accessCodes;
    },
  }),
}));

vi.mock('vue-router', async () => {
  const { reactive } = await import('vue');
  mocks.routeQuery = reactive({}) as Record<string, string>;
  return {
    useRoute: () => ({
      query: mocks.routeQuery,
    }),
    useRouter: () => ({ push: mocks.routerPush }),
  };
});

vi.mock('../system/components/page-shell.vue', () => ({
  default: {
    name: 'PageShell',
    template: '<main><slot /></main>',
  },
}));

vi.mock('#/api', () => ({
  getAppCustomerDetailApi: vi.fn(),
  getAppCustomerListApi: vi.fn(),
  getAppUserInsightsApi: vi.fn(),
  updateAppCustomerApi: vi.fn(),
}));

import {
  getAppCustomerDetailApi,
  getAppCustomerListApi,
  getAppUserInsightsApi,
  updateAppCustomerApi,
} from '#/api';

import AppUsers from './app-users.vue';
import UserInsights from './user-insights.vue';

function customer(input?: Partial<AppCustomer>): AppCustomer {
  return {
    avatar: '',
    createTime: '2026/01/01 10:00:00',
    id: 1,
    lastLoginAt: null,
    memberLevel: 'free',
    nickname: '测试客户',
    phone: '13800000001',
    registerSource: 'app_sms',
    status: 'active',
    updateTime: '2026/01/01 10:00:00',
    ...input,
  };
}

function insight(input?: Partial<ReturnType<typeof baseInsight>>): ReturnType<typeof baseInsight> {
  return {
    ...baseInsight(),
    ...input,
  };
}

function setRouteQuery(query: Record<string, string>) {
  for (const key of Object.keys(mocks.routeQuery)) {
    delete mocks.routeQuery[key];
  }
  Object.assign(mocks.routeQuery, query);
}

function baseInsight() {
  return {
    avatar: '',
    cardCount: 0,
    centers: [],
    compatibilityCount: 0,
    createTime: '2026/01/01 10:00:00',
    gender: '',
    id: 1,
    lastLoginAt: '2026/01/01 10:00:00',
    latestChatTime: '',
    latestCompatibilitySummary: '',
    latestMemory: '',
    latestQuizTime: '',
    memberLevel: 'free',
    memoryCount: 0,
    messageCount: 0,
    nickname: '测试提炼用户',
    phone: '13800000001',
    primaryType: 0,
    profile: {},
    registerSource: 'app_sms',
    score: {},
    secondType: 0,
    sessionCount: 0,
    status: 'active',
    updateTime: '2026/01/01 10:00:00',
    wingType: 0,
  };
}

describe('App customer list P2 behavior', () => {
  beforeEach(() => {
    mocks.accessCodes = [];
    setRouteQuery({});
    mocks.routerPush.mockReset();
    vi.mocked(getAppCustomerDetailApi).mockReset();
    vi.mocked(getAppCustomerListApi).mockReset();
    vi.mocked(getAppUserInsightsApi).mockReset();
    vi.mocked(updateAppCustomerApi).mockReset();
    vi.mocked(getAppCustomerListApi).mockResolvedValue({
      items: [customer()],
      total: 1,
    });
  });

  it('hides only the 360 action without Customer:UserInsights:List permission', async () => {
    mocks.accessCodes = ['Customer:App:Write'];

    const wrapper = mountVueComponent(AppUsers);
    await flushVuePromises();

    expect(wrapper.text()).not.toContain('360');
    expect(wrapper.text()).toContain('查看详情');
    expect(wrapper.text()).toContain('编辑');
    wrapper.unmount();
  });

  it('opens user insights with userId and open marker from App customer 360 action', async () => {
    mocks.accessCodes = ['Customer:UserInsights:List'];

    const wrapper = mountVueComponent(AppUsers);
    await flushVuePromises();

    wrapper.button('360')?.click();

    expect(mocks.routerPush).toHaveBeenCalledWith({
      path: '/customer/user-insights',
      query: { keyword: '13800000001', open: '1', userId: '1' },
    });
    wrapper.unmount();
  });

  it('shows an inline error and retry action when the list load fails', async () => {
    vi.mocked(getAppCustomerListApi)
      .mockRejectedValueOnce(new Error('list down'))
      .mockResolvedValueOnce({ items: [customer({ nickname: '重试成功' })], total: 1 });

    const wrapper = mountVueComponent(AppUsers);
    await flushVuePromises();

    expect(wrapper.text()).toContain('App 客户列表加载失败，请稍后重试');
    expect(wrapper.text()).toContain('重试');

    wrapper.button('重试')?.click();
    await flushVuePromises();

    expect(wrapper.text()).toContain('重试成功');
    wrapper.unmount();
  });

  it('keeps the latest customer detail when an older detail request resolves later', async () => {
    let resolveFirst!: (value: AppCustomer) => void;
    vi.mocked(getAppCustomerListApi).mockResolvedValue({
      items: [
        customer({ id: 1, nickname: '客户A', phone: '13800000001' }),
        customer({ id: 2, nickname: '客户B', phone: '13800000002' }),
      ],
      total: 2,
    });
    vi.mocked(getAppCustomerDetailApi)
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveFirst = resolve;
        }) as any,
      )
      .mockResolvedValueOnce(customer({ id: 2, nickname: '客户B详情', phone: '13800000002' }));

    const wrapper = mountVueComponent(AppUsers);
    await flushVuePromises();

    const detailButtons = [...document.body.querySelectorAll('button')].filter(
      (item) => item.textContent?.trim() === '查看详情',
    );
    detailButtons[0]?.click();
    detailButtons[1]?.click();
    await flushVuePromises();

    expect(wrapper.text()).toContain('客户B详情');

    resolveFirst(customer({ id: 1, nickname: '客户A详情', phone: '13800000001' }));
    await flushVuePromises();

    expect(wrapper.text()).toContain('客户B详情');
    expect(wrapper.text()).not.toContain('客户A详情');
    wrapper.unmount();
  });

});


describe('User insights route auto-open behavior', () => {
  beforeEach(() => {
    setRouteQuery({});
    vi.mocked(getAppUserInsightsApi).mockReset();
  });

  it('auto-opens the detail drawer when route userId matches a loaded user', async () => {
    setRouteQuery({ keyword: '13800000002', open: '1', userId: '2' });
    vi.mocked(getAppUserInsightsApi).mockResolvedValue({
      items: [
        insight({ id: 1, latestMemory: '不应打开的记忆', phone: '13800000001' }),
        insight({ id: 2, latestMemory: '用户二的提炼详情', phone: '13800000002' }),
      ],
      total: 2,
    });

    const wrapper = mountVueComponent(UserInsights);
    await flushVuePromises();

    expect(getAppUserInsightsApi).toHaveBeenCalledWith(
      expect.objectContaining({ userId: '2' }),
    );
    expect(wrapper.text()).toContain('用户二的提炼详情');
    expect(wrapper.text()).not.toContain('不应打开的记忆');
    wrapper.unmount();
  });

  it('auto-opens the detail drawer when a route keyword matches exactly one user', async () => {
    setRouteQuery({ keyword: '13800000003' });
    vi.mocked(getAppUserInsightsApi).mockResolvedValue({
      items: [insight({ id: 3, latestMemory: '唯一匹配的提炼详情', phone: '13800000003' })],
      total: 1,
    });

    const wrapper = mountVueComponent(UserInsights);
    await flushVuePromises();

    expect(wrapper.text()).toContain('唯一匹配的提炼详情');
    wrapper.unmount();
  });

  it('reloads and auto-opens again when only route userId/open changes in a cached page', async () => {
    setRouteQuery({ keyword: '同名用户', open: '1', userId: '1' });
    vi.mocked(getAppUserInsightsApi)
      .mockResolvedValueOnce({
        items: [insight({ id: 1, latestMemory: '用户一的提炼详情', phone: '13800000001' })],
        total: 1,
      })
      .mockResolvedValueOnce({
        items: [insight({ id: 2, latestMemory: '用户二的提炼详情', phone: '13800000002' })],
        total: 1,
      });

    const wrapper = mountVueComponent(UserInsights);
    await flushVuePromises();

    expect(wrapper.text()).toContain('用户一的提炼详情');

    setRouteQuery({ keyword: '同名用户', open: '2', userId: '2' });
    await flushVuePromises();

    expect(getAppUserInsightsApi).toHaveBeenLastCalledWith(
      expect.objectContaining({ userId: '2' }),
    );
    expect(wrapper.text()).toContain('用户二的提炼详情');
    wrapper.unmount();
  });
});
