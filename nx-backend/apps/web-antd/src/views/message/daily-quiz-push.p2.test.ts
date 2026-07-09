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
  getDailyQuizPushRecordsApi: vi.fn(),
  getDailyQuizPushStatsApi: vi.fn(),
  getPushAudienceCountApi: vi.fn(),
  sendPushApi: vi.fn(),
}));

import {
  getDailyQuizPushRecordsApi,
  getDailyQuizPushStatsApi,
  getPushAudienceCountApi,
  sendPushApi,
} from '#/api/core/push';

import DailyQuizPushRecords from './daily-quiz-push.vue';

describe('Daily quiz push records page', () => {
  beforeEach(() => {
    vi.mocked(getDailyQuizPushStatsApi).mockReset();
    vi.mocked(getDailyQuizPushRecordsApi).mockReset();
    vi.mocked(getPushAudienceCountApi).mockReset();
    vi.mocked(sendPushApi).mockReset();
    vi.mocked(getDailyQuizPushStatsApi).mockResolvedValue({
      answeredUsers: 4,
      completedUsers: 2,
      date: '2026-07-09',
      eligibleUsers: 12,
      pendingReassessmentReports: 3,
      pushed: true,
      pushedUsers: 9,
      totalAnswers: 17,
    });
    vi.mocked(getDailyQuizPushRecordsApi).mockResolvedValue({
      items: [
        {
          answeredCount: 5,
          appUserId: 7,
          batchId: 88,
          cardId: 123,
          cardName: '本人人格卡',
          completed: true,
          completedAt: '2026/07/09 09:05:00',
          nickname: '测试用户',
          phone: '13800000000',
          pushSentAt: '2026/07/09 09:00:00',
          pushed: true,
          quizDate: '2026-07-09',
        },
      ],
      total: 1,
    });
  });

  it('loads daily push stats and answered records', async () => {
    const wrapper = mountVueComponent(DailyQuizPushRecords);
    await flushVuePromises();

    expect(getDailyQuizPushStatsApi).toHaveBeenCalledWith({
      date: expect.any(String),
    });
    expect(getDailyQuizPushRecordsApi).toHaveBeenCalledWith({
      date: expect.any(String),
      page: 1,
      pageSize: 20,
    });
    expect(wrapper.text()).toContain('已推送用户');
    expect(wrapper.text()).toContain('9');
    expect(wrapper.text()).toContain('已答题用户');
    expect(wrapper.text()).toContain('4');
    expect(wrapper.text()).toContain('测试用户');
    expect(wrapper.text()).toContain('已完成');
    wrapper.unmount();
  });

  it('opens a prefilled daily quiz test push dialog', async () => {
    vi.mocked(getPushAudienceCountApi).mockResolvedValue({
      deviceCount: 9,
      targetType: 'all',
      userCount: 8,
    });
    vi.mocked(sendPushApi).mockResolvedValue({
      recordId: 66,
      status: 'pending',
    });

    const wrapper = mountVueComponent(DailyQuizPushRecords);
    await flushVuePromises();

    wrapper.button('测试推送')?.click();
    await flushVuePromises();

    expect(wrapper.text()).toContain('发送每日题测试推送');
    expect(wrapper.text()).toContain('今日画像校准题已准备好');
    expect(wrapper.text()).toContain('/daily-quiz');

    wrapper.button('预估受众')?.click();
    await flushVuePromises();
    expect(wrapper.text()).toContain('预计 8 人 / 9 台设备');

    wrapper.button('OK')?.click();
    await flushVuePromises();
    expect(sendPushApi).toHaveBeenCalledWith(
      expect.objectContaining({
        deepLink: '/daily-quiz',
        targetType: 'all',
        title: '今日画像校准题已准备好',
      }),
    );
    wrapper.unmount();
  });
});
