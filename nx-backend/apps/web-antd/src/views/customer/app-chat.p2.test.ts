import type { AppChatAuditMessage } from '#/api';

import { beforeEach, describe, expect, it, vi } from 'vitest';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

vi.mock('ant-design-vue', async () => import('#/test-utils/antd-stubs'));

vi.mock('@vben/common-ui', () => ({
  Page: {
    name: 'Page',
    template: '<main><slot /></main>',
  },
}));

vi.mock('#/api', () => ({
  getAppChatAuditMessagesApi: vi.fn(),
}));

import { getAppChatAuditMessagesApi } from '#/api';

import AppChat from './app-chat.vue';

function message(input?: Partial<AppChatAuditMessage>): AppChatAuditMessage {
  return {
    appUserId: 1,
    cardId: 2,
    cardName: '成长卡',
    content: '我最近总是拖延怎么办？',
    createTime: '2026/07/04 10:00:00',
    favorite: false,
    feedback: '',
    id: 20,
    nickname: '测试用户',
    phone: '13800000001',
    role: 'user',
    sessionId: 3,
    sources: [],
    ...input,
  };
}

describe('App chat audit P2 behavior', () => {
  beforeEach(() => {
    vi.mocked(getAppChatAuditMessagesApi).mockReset();
    vi.mocked(getAppChatAuditMessagesApi).mockResolvedValue({
      items: [message()],
      total: 1,
    });
  });

  it('shows an inline error and retry action when the chat audit list load fails', async () => {
    vi.mocked(getAppChatAuditMessagesApi)
      .mockRejectedValueOnce(new Error('chat audit down'))
      .mockResolvedValueOnce({
        items: [message({ content: '重试后的聊天内容' })],
        total: 1,
      });

    const wrapper = mountVueComponent(AppChat);
    await flushVuePromises();

    expect(wrapper.text()).toContain('聊天质检列表加载失败，请稍后重试');
    expect(wrapper.text()).toContain('重试');

    wrapper.button('重试')?.click();
    await flushVuePromises();

    expect(wrapper.text()).toContain('重试后的聊天内容');
    wrapper.unmount();
  });

  it('keeps the latest chat audit list when an older request fails later', async () => {
    let rejectFirst!: (error: Error) => void;
    vi.mocked(getAppChatAuditMessagesApi)
      .mockReturnValueOnce(
        new Promise((_, reject) => {
          rejectFirst = reject;
        }) as any,
      )
      .mockResolvedValueOnce({
        items: [message({ content: '新的聊天质检列表' })],
        total: 1,
      });

    const wrapper = mountVueComponent(AppChat);
    await flushVuePromises();
    wrapper.button('刷新')?.click();
    await flushVuePromises();

    rejectFirst(new Error('old chat audit failed'));
    await flushVuePromises();

    expect(wrapper.text()).toContain('新的聊天质检列表');
    expect(wrapper.text()).not.toContain('聊天质检列表加载失败');
    wrapper.unmount();
  });
});
