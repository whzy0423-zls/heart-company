import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

vi.mock('ant-design-vue', async () => import('#/test-utils/antd-stubs'));

vi.mock('@vben/common-ui', () => ({
  Page: {
    name: 'Page',
    props: ['description', 'title'],
    template: '<main><h1>{{ title }}</h1><slot /></main>',
  },
}));

vi.mock('#/api/core/daily-quiz', () => ({
  generateDailyQuizSetApi: vi.fn(),
  getDailyQuizSetApi: vi.fn(),
  getTodayDailyQuizSetApi: vi.fn(),
  replaceDailyQuizQuestionApi: vi.fn(),
}));

import {
  generateDailyQuizSetApi,
  getTodayDailyQuizSetApi,
  replaceDailyQuizQuestionApi,
} from '#/api/core/daily-quiz';

import DailyQuizBank from './daily-quiz-bank.vue';

const baseSet = {
  date: '2026-07-09',
  errorMessage: '',
  generatedAt: '2026/07/09 08:00:00',
  id: 12,
  modelName: 'MiniMax-M3',
  modelProvider: 'minimax',
  publishedAt: '',
  pushedAt: '',
  questionIds: [101, 102, 103, 104, 105],
  questions: [
    {
      answeredCount: 0,
      createTime: '2026/07/09 08:00:00',
      id: 1001,
      isActive: true,
      modelName: 'MiniMax-M3',
      modelProvider: 'minimax',
      operator: '',
      question: {
        body: '你在团队讨论中更倾向于如何回应？',
        dimension: 'core',
        id: 101,
        options: [
          { id: 'a', label: 'A', text: '先保持观察' },
          { id: 'b', label: 'B', text: '直接提出判断' },
        ],
      },
      questionId: 101,
      replaceReason: '',
      setId: 12,
      slotNo: 1,
      source: 'ai',
      versionNo: 1,
    },
  ],
  rawResponse: '',
  source: 'ai',
  status: 'generated',
};

describe('Daily quiz bank page', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-07-09T08:00:00+08:00'));
    vi.mocked(getTodayDailyQuizSetApi).mockReset();
    vi.mocked(generateDailyQuizSetApi).mockReset();
    vi.mocked(replaceDailyQuizQuestionApi).mockReset();
    vi.mocked(getTodayDailyQuizSetApi).mockResolvedValue(baseSet as any);
    vi.mocked(generateDailyQuizSetApi).mockResolvedValue(baseSet as any);
    const firstQuestion = baseSet.questions[0]!;
    vi.mocked(replaceDailyQuizQuestionApi).mockResolvedValue({
      ...baseSet,
      questions: [
        {
          ...firstQuestion,
          question: {
            ...firstQuestion.question,
            body: '替换后的校准题目',
          },
          versionNo: 2,
        },
      ],
    } as any);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('loads today set and renders status plus active question cards', async () => {
    const wrapper = mountVueComponent(DailyQuizBank);
    await flushVuePromises();

    expect(getTodayDailyQuizSetApi).toHaveBeenCalledOnce();
    expect(wrapper.text()).toContain('每日题库管理');
    expect(wrapper.text()).toContain('generated');
    expect(wrapper.text()).toContain('MiniMax-M3');
    expect(wrapper.text()).toContain('你在团队讨论中更倾向于如何回应？');
    expect(wrapper.text()).toContain('先保持观察');
    expect(wrapper.text()).toContain('v1');
    expect(wrapper.text()).toContain('未答题');
    expect(wrapper.text()).toContain('更换本题');

    wrapper.unmount();
  });

  it('generates the selected date set and replaces an unanswered question', async () => {
    const wrapper = mountVueComponent(DailyQuizBank);
    await flushVuePromises();

    wrapper.button('生成今日题目')?.click();
    await flushVuePromises();
    expect(generateDailyQuizSetApi).toHaveBeenCalledWith({ date: '2026-07-09' });

    wrapper.button('更换本题')?.click();
    await flushVuePromises();
    expect(wrapper.text()).toContain('更换第 1 题');

    wrapper.button('OK')?.click();
    await flushVuePromises();

    expect(replaceDailyQuizQuestionApi).toHaveBeenCalledWith(12, 1, {
      reason: '',
    });
    expect(wrapper.text()).toContain('替换后的校准题目');
    expect(wrapper.text()).toContain('v2');

    wrapper.unmount();
  });

  it('shows version history and locks replacement after any user answered', async () => {
    const firstQuestion = baseSet.questions[0]!;
    vi.mocked(getTodayDailyQuizSetApi).mockResolvedValueOnce({
      ...baseSet,
      questions: [
        {
          ...firstQuestion,
          answeredCount: 1,
          versionNo: 2,
          replaceReason: '优化表达',
          createTime: '2026/07/09 09:00:00',
        },
        {
          ...firstQuestion,
          answeredCount: 0,
          isActive: false,
          question: {
            ...firstQuestion.question,
            body: '历史版本题干',
          },
          versionNo: 1,
        },
      ],
    } as any);

    const wrapper = mountVueComponent(DailyQuizBank);
    await flushVuePromises();

    expect(wrapper.text()).toContain('整套题已锁定');
    expect(wrapper.text()).toContain('版本记录');
    expect(wrapper.text()).toContain('历史版本题干');
    expect(wrapper.text()).toContain('优化表达');
    expect(wrapper.text()).not.toContain('更换后将生成新的题目版本');

    wrapper.unmount();
  });
});
