import { beforeEach, describe, expect, it, vi } from 'vitest';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

vi.mock('ant-design-vue', async () => import('#/test-utils/antd-stubs'));

vi.mock('@vben/common-ui', () => ({
  Page: {
    name: 'Page',
    template: '<main><slot /></main>',
  },
}));

vi.mock('#/api/core/push', () => ({
  getPushAudienceCountApi: vi.fn(),
  getPushListApi: vi.fn(),
  sendPushApi: vi.fn(),
}));

import { getPushAudienceCountApi, getPushListApi, sendPushApi } from '#/api/core/push';

import PushManagement from './push.vue';

describe('Push management P2 behavior', () => {
  beforeEach(() => {
    vi.mocked(getPushAudienceCountApi).mockReset();
    vi.mocked(getPushListApi).mockReset();
    vi.mocked(sendPushApi).mockReset();
    vi.mocked(getPushListApi).mockResolvedValue({ items: [], total: 0 });
  });

  it('shows an inline error and retry action when the push list load fails', async () => {
    vi.mocked(getPushListApi)
      .mockRejectedValueOnce(new Error('push list down'))
      .mockResolvedValueOnce({
        items: [
          {
            content: '重试后的推送内容',
            createTime: '2026/07/04 10:00:00',
            deepLink: '',
            id: 1,
            operator: 'admin',
            sentCount: 1,
            status: 'success',
            targetType: 'all',
            targetValue: '',
            title: '重试成功',
          },
        ],
        total: 1,
      });

    const wrapper = mountVueComponent(PushManagement);
    await flushVuePromises();

    expect(wrapper.text()).toContain('推送记录加载失败，请稍后重试');
    expect(wrapper.text()).toContain('重试');

    wrapper.button('重试')?.click();
    await flushVuePromises();

    expect(wrapper.text()).toContain('重试成功');
    wrapper.unmount();
  });

  it('keeps the latest list result when an older request fails later', async () => {
    let rejectFirst!: (error: Error) => void;
    vi.mocked(getPushListApi)
      .mockReturnValueOnce(
        new Promise((_, reject) => {
          rejectFirst = reject;
        }) as any,
      )
      .mockResolvedValueOnce({
        items: [
          {
            content: '新的列表内容',
            createTime: '2026/07/04 10:00:00',
            deepLink: '',
            id: 2,
            operator: 'admin',
            sentCount: 0,
            status: 'pending',
            targetType: 'all',
            targetValue: '',
            title: '最新请求',
          },
        ],
        total: 1,
      });

    const wrapper = mountVueComponent(PushManagement);
    await flushVuePromises();
    wrapper.button('刷新')?.click();
    await flushVuePromises();
    rejectFirst(new Error('old push list failed'));
    await flushVuePromises();

    expect(wrapper.text()).toContain('最新请求');
    expect(wrapper.text()).not.toContain('推送记录加载失败');
    wrapper.unmount();
  });

  it('keeps the latest audience estimate when an older estimate resolves later', async () => {
    let resolveFirst!: (value: { deviceCount: number; userCount: number }) => void;
    vi.mocked(getPushAudienceCountApi)
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveFirst = resolve;
        }) as any,
      )
      .mockResolvedValueOnce({ deviceCount: 20, userCount: 20 });

    const wrapper = mountVueComponent(PushManagement);
    await flushVuePromises();

    wrapper.button('预估受众')?.click();
    wrapper.button('预估受众')?.click();
    await flushVuePromises();

    expect(wrapper.text()).toContain('预计 20 人 / 20 台设备');

    resolveFirst({ deviceCount: 1, userCount: 1 });
    await flushVuePromises();

    expect(wrapper.text()).toContain('预计 20 人 / 20 台设备');
    expect(wrapper.text()).not.toContain('预计 1 人 / 1 台设备');
    wrapper.unmount();
  });

  it('renders audience estimate target echo returned by the backend', async () => {
    vi.mocked(getPushAudienceCountApi).mockResolvedValue({
      deviceCount: 6,
      targetType: 'level',
      targetValue: 'vip',
      userCount: 5,
    } as any);

    const wrapper = mountVueComponent(PushManagement);
    await flushVuePromises();

    wrapper.button('预估受众')?.click();
    await flushVuePromises();

    expect(wrapper.text()).toContain('预计 5 人 / 6 台设备（会员等级：VIP 会员）');
    wrapper.unmount();
  });

});
