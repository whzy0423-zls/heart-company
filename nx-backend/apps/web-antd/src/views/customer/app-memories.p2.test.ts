import type { AppMemoryAdminItem } from '#/api';

import { beforeEach, describe, expect, it, vi } from 'vitest';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

const mocks = vi.hoisted(() => ({
  accessCodes: [] as string[],
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

vi.mock('#/api', () => ({
  getAppMemoriesAdminApi: vi.fn(),
  updateAppMemoryStatusApi: vi.fn(),
}));

import { getAppMemoriesAdminApi, updateAppMemoryStatusApi } from '#/api';

import AppMemories from './app-memories.vue';

function memory(input?: Partial<AppMemoryAdminItem>): AppMemoryAdminItem {
  return {
    appUserId: 1,
    cardId: 2,
    cardName: '成长卡',
    content: '用户在压力下会回避冲突',
    createTime: '2026/07/04 09:00:00',
    id: 10,
    nickname: '测试用户',
    phone: '13800000001',
    sourceTime: '2026/07/04 09:00:00',
    status: 'active',
    updateTime: '2026/07/04 10:00:00',
    ...input,
  };
}

describe('App memories admin P2 behavior', () => {
  beforeEach(() => {
    mocks.accessCodes = [];
    vi.mocked(getAppMemoriesAdminApi).mockReset();
    vi.mocked(updateAppMemoryStatusApi).mockReset();
    vi.mocked(getAppMemoriesAdminApi).mockResolvedValue({
      items: [memory()],
      total: 1,
    });
  });

  it('hides memory status actions without Customer:AppMemory:Write permission', async () => {
    const wrapper = mountVueComponent(AppMemories);
    await flushVuePromises();

    expect(wrapper.text()).toContain('详情');
    expect(wrapper.text()).not.toContain('停用');
    wrapper.unmount();
  });

  it('shows memory status actions with Customer:AppMemory:Write permission', async () => {
    mocks.accessCodes = ['Customer:AppMemory:Write'];

    const wrapper = mountVueComponent(AppMemories);
    await flushVuePromises();

    expect(wrapper.text()).toContain('停用');
    wrapper.unmount();
  });

  it('shows an inline error and retry action when the memory list load fails', async () => {
    vi.mocked(getAppMemoriesAdminApi)
      .mockRejectedValueOnce(new Error('memory list down'))
      .mockResolvedValueOnce({
        items: [memory({ content: '重试后的记忆' })],
        total: 1,
      });

    const wrapper = mountVueComponent(AppMemories);
    await flushVuePromises();

    expect(wrapper.text()).toContain('私库记忆加载失败，请稍后重试');
    expect(wrapper.text()).toContain('重试');

    wrapper.button('重试')?.click();
    await flushVuePromises();

    expect(wrapper.text()).toContain('重试后的记忆');
    wrapper.unmount();
  });

  it('keeps the latest memory list when an older request fails later', async () => {
    let rejectFirst!: (error: Error) => void;
    vi.mocked(getAppMemoriesAdminApi)
      .mockReturnValueOnce(
        new Promise((_, reject) => {
          rejectFirst = reject;
        }) as any,
      )
      .mockResolvedValueOnce({
        items: [memory({ content: '新的记忆列表' })],
        total: 1,
      });

    const wrapper = mountVueComponent(AppMemories);
    await flushVuePromises();
    wrapper.button('刷新')?.click();
    await flushVuePromises();

    rejectFirst(new Error('old memory list failed'));
    await flushVuePromises();

    expect(wrapper.text()).toContain('新的记忆列表');
    expect(wrapper.text()).not.toContain('私库记忆加载失败');
    wrapper.unmount();
  });
});
