import type {
  AppUserReport,
  AppUserReportList,
} from '#/api/core/app-user-report';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h, reactive } from 'vue';

import {
  getAppUserReportApi,
  getAppUserReportsApi,
  requestAppUserReportApi,
} from '#/api/core/app-user-report';
import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

import UserInsightReport from './user-insight-report.vue';

const access = vi.hoisted(() => ({ codes: [] as string[] }));
vi.mock('@vben/stores', () => ({
  useAccessStore: () => ({
    get accessCodes() {
      return access.codes;
    },
  }),
}));
vi.mock('ant-design-vue', async () => import('#/test-utils/antd-stubs'));
vi.mock('#/api/core/app-user-report', () => ({
  getAppUserReportApi: vi.fn(),
  getAppUserReportsApi: vi.fn(),
  requestAppUserReportApi: vi.fn(),
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, reject, resolve };
}

function report(id = 11, appUserId = 1): AppUserReport {
  const claim = {
    evidenceRefs: ['message:1'],
    text: '<script>private</script>',
  };
  return {
    analysis: {
      changes: [],
      goals: [],
      observations: [claim],
      patterns: [],
      recommendations: [{ evidenceRefs: ['message:1'], text: '先记录事实' }],
      summary: {
        evidenceRefs: ['message:1'],
        text: `用户 ${appUserId} 的报告`,
      },
      uncertainties: [{ evidenceRefs: ['message:1'], text: '样本仍有限' }],
    },
    appUserId,
    cardId: 3,
    coverage: {
      eligibleCount: 8,
      includedCount: 1,
      omittedCount: 7,
      truncated: true,
      truncatedCount: 1,
      windowDays: 30,
    },
    evidence: [
      {
        id: 'message:1',
        kind: 'message',
        occurredAt: '2026-10-08T00:00:00Z',
        text: '我记录了今天的感受',
      },
    ],
    evidenceCount: 1,
    generatedAt: '2026-10-09T00:00:00Z',
    id,
    periodEnd: '2026-10-12T00:00:00Z',
    periodStart: '2026-10-05T00:00:00Z',
    published: true,
    sourceFrom: '2026-10-08T00:00:00Z',
    sourceThrough: '2026-10-08T00:00:00Z',
    timezone: 'Asia/Shanghai',
    version: id,
  };
}

function history(id = 11): AppUserReportList {
  return { enabled: true, reports: [report(id)], status: 'ready' };
}

const cleanups: Array<() => void> = [];
function mount(open = true) {
  const props = reactive({ appUserId: 1, open, userName: '测试用户' });
  const wrapper = mountVueComponent(
    defineComponent({
      setup: () => () => h(UserInsightReport, props),
    }),
  );
  cleanups.push(wrapper.unmount);
  return { props, wrapper };
}

async function settle() {
  await flushVuePromises();
  await flushVuePromises();
}

describe('admin growth report drawer', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    access.codes = [];
    vi.mocked(getAppUserReportsApi).mockResolvedValue(history());
    vi.mocked(getAppUserReportApi).mockResolvedValue(report());
    vi.mocked(requestAppUserReportApi).mockResolvedValue({ queued: true });
  });
  afterEach(() => cleanups.splice(0).forEach((cleanup) => cleanup()));

  it('loads lazily when opened and displays structured claims, sources, period and coverage', async () => {
    const { props, wrapper } = mount(false);
    await settle();
    expect(getAppUserReportsApi).not.toHaveBeenCalled();
    props.open = true;
    await settle();
    expect(getAppUserReportsApi).toHaveBeenCalledWith(1);
    expect(getAppUserReportApi).toHaveBeenCalledWith(11);
    expect(wrapper.text()).toContain('用户 1 的报告');
    expect(wrapper.text()).toContain('样本仍有限');
    expect(wrapper.text()).toContain('我记录了今天的感受');
    expect(wrapper.text()).toContain('message:1');
    expect(wrapper.text()).toContain('Asia/Shanghai');
    expect(wrapper.text()).toContain('已截断');
    expect(document.querySelector('script')).toBeNull();
    expect(wrapper.text()).toContain('<script>private</script>');
  });

  it('shows loading without incorrectly presenting the user as opted out', async () => {
    const pending = deferred<AppUserReportList>();
    vi.mocked(getAppUserReportsApi).mockReturnValue(pending.promise);
    const { wrapper } = mount();
    await settle();
    expect(wrapper.text()).toContain('加载中');
    expect(wrapper.text()).not.toContain('用户未开启个人分析');
    pending.resolve(history());
  });

  it('shows list failure and allows retry without false empty state', async () => {
    vi.mocked(getAppUserReportsApi).mockRejectedValueOnce(new Error('network'));
    const { wrapper } = mount();
    await settle();
    expect(wrapper.text()).toContain('报告记录加载失败');
    expect(wrapper.text()).not.toContain('暂无分析报告');
    wrapper.button('重试')?.click();
    await settle();
    expect(wrapper.text()).toContain('用户 1 的报告');
  });

  it('displays consent off and prevents administrators from generating', async () => {
    access.codes = ['Customer:UserInsights:Generate'];
    vi.mocked(getAppUserReportsApi).mockResolvedValue({
      enabled: false,
      reports: [],
      status: 'disabled',
    });
    const { wrapper } = mount();
    await settle();
    expect(wrapper.text()).toContain('用户未开启个人分析');
    wrapper.button('请求分析')?.click();
    await settle();
    expect(requestAppUserReportApi).not.toHaveBeenCalled();
  });

  it('requires the distinct generation permission', async () => {
    access.codes = ['Customer:UserInsights:List'];
    const { wrapper } = mount();
    await settle();
    expect(wrapper.button('请求分析')).toBeUndefined();
  });

  it('queues a permitted request and reloads the same user state', async () => {
    access.codes = ['Customer:UserInsights:Generate'];
    const { wrapper } = mount();
    await settle();
    wrapper.button('请求分析')?.click();
    await settle();
    expect(requestAppUserReportApi).toHaveBeenCalledWith(1);
    expect(wrapper.text()).toContain('已加入分析队列');
    expect(getAppUserReportsApi).toHaveBeenCalledTimes(2);
  });

  it('shows generation failure and keeps the existing report', async () => {
    access.codes = ['Customer:UserInsights:Generate'];
    vi.mocked(requestAppUserReportApi).mockRejectedValue(new Error('denied'));
    const { wrapper } = mount();
    await settle();
    wrapper.button('请求分析')?.click();
    await settle();
    expect(wrapper.text()).toContain('分析请求失败');
    expect(wrapper.text()).toContain('用户 1 的报告');
    expect(wrapper.text()).not.toContain('已加入分析队列');
  });

  it('shows insufficient data as an empty state', async () => {
    vi.mocked(getAppUserReportsApi).mockResolvedValue({
      enabled: true,
      reports: [],
      status: 'insufficient_data',
    });
    const { wrapper } = mount();
    await settle();
    expect(wrapper.text()).toContain('资料不足');
    expect(wrapper.text()).toContain('暂无分析报告');
    expect(getAppUserReportApi).not.toHaveBeenCalled();
  });

  it('discards a previous user history response after switching users', async () => {
    const previous = deferred<AppUserReportList>();
    vi.mocked(getAppUserReportsApi)
      .mockReturnValueOnce(previous.promise)
      .mockResolvedValueOnce(history(22));
    vi.mocked(getAppUserReportApi).mockResolvedValue(report(22, 2));
    const { props, wrapper } = mount();
    props.appUserId = 2;
    await settle();
    previous.resolve(history());
    await settle();
    expect(wrapper.text()).toContain('用户 2 的报告');
    expect(getAppUserReportApi).not.toHaveBeenCalledWith(11);
  });

  it('discards stale report details after switching users or closing the drawer', async () => {
    const previous = deferred<AppUserReport>();
    vi.mocked(getAppUserReportApi)
      .mockReturnValueOnce(previous.promise)
      .mockResolvedValueOnce(report(22, 2));
    const { props, wrapper } = mount();
    await settle();
    vi.mocked(getAppUserReportsApi).mockResolvedValueOnce(history(22));
    props.appUserId = 2;
    await settle();
    previous.resolve(report());
    await settle();
    expect(wrapper.text()).toContain('用户 2 的报告');
    expect(wrapper.text()).not.toContain('用户 1 的报告');
    props.open = false;
    await settle();
    expect(wrapper.text()).not.toContain('用户 2 的报告');
  });

  it('rejects report detail whose owner does not match the selected user', async () => {
    vi.mocked(getAppUserReportApi).mockResolvedValue(report(11, 2));
    const { wrapper } = mount();
    await settle();
    expect(wrapper.text()).not.toContain('用户 2 的报告');
    expect(wrapper.text()).toContain('报告详情加载失败');
  });

  it('formats the review period in the report timezone instead of the browser timezone', async () => {
    const fixture = { ...report(), timezone: 'UTC' };
    vi.mocked(getAppUserReportApi).mockResolvedValue(fixture);
    const { wrapper } = mount();
    await settle();
    const start = new Date(fixture.periodStart).toLocaleString('zh-CN', {
      hour12: false,
      timeZone: 'UTC',
    });
    const end = new Date(fixture.periodEnd).toLocaleString('zh-CN', {
      hour12: false,
      timeZone: 'UTC',
    });
    expect(wrapper.text()).toContain(`${start} 至 ${end}`);
  });

  it('discards generation completion after switching users', async () => {
    access.codes = ['Customer:UserInsights:Generate'];
    const previous = deferred<{ queued: boolean }>();
    vi.mocked(requestAppUserReportApi).mockReturnValue(previous.promise);
    const { props, wrapper } = mount();
    await settle();
    wrapper.button('请求分析')?.click();
    vi.mocked(getAppUserReportsApi).mockResolvedValueOnce(history(22));
    vi.mocked(getAppUserReportApi).mockResolvedValueOnce(report(22, 2));
    props.appUserId = 2;
    await settle();
    previous.resolve({ queued: true });
    await settle();
    expect(wrapper.text()).toContain('用户 2 的报告');
    expect(wrapper.text()).not.toContain('已加入分析队列');
    expect(getAppUserReportsApi).toHaveBeenCalledTimes(2);
  });

  it('does not show a queued success when the server did not queue work', async () => {
    access.codes = ['Customer:UserInsights:Generate'];
    vi.mocked(requestAppUserReportApi).mockResolvedValue({ queued: false });
    const { wrapper } = mount();
    await settle();
    wrapper.button('请求分析')?.click();
    await settle();
    expect(wrapper.text()).toContain('分析请求失败');
    expect(wrapper.text()).not.toContain('已加入分析队列');
  });

  it('shows a neutral result when no new evidence needs analysis', async () => {
    access.codes = ['Customer:UserInsights:Generate'];
    vi.mocked(requestAppUserReportApi).mockResolvedValue({
      queued: false,
      reason: 'no_new_evidence',
    });
    const { wrapper } = mount();
    await settle();
    wrapper.button('请求分析')?.click();
    await settle();
    expect(wrapper.text()).toContain('当前没有待分析的新资料');
    expect(wrapper.text()).not.toContain('分析请求失败');
    expect(wrapper.text()).not.toContain('已加入分析队列');
    expect(wrapper.text()).toContain('用户 1 的报告');
  });

  it('keeps evidence navigation inside the drawer without changing the app route', async () => {
    mount();
    await settle();
    const link = document.querySelector<HTMLAnchorElement>(
      '.evidence-references a',
    );
    const event = new MouseEvent('click', { bubbles: true, cancelable: true });
    link?.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
  });

  it('renders optional portrait claims and concrete actions with their evidence', async () => {
    const fixture = report();
    const evidenceRefs = ['message:1'];
    vi.mocked(getAppUserReportApi).mockResolvedValue({
      ...fixture,
      analysis: {
        ...fixture.analysis,
        actions: [
          { title: '沟通前暂停', detail: '记下一个事实与需要', evidenceRefs },
        ],
        awarenessPrompts: [{ text: '此刻需要什么支持', evidenceRefs }],
        strengths: [{ text: '能够区分事实和判断', evidenceRefs }],
        stressPoints: [{ text: '时间紧张时容易急于回应', evidenceRefs }],
      },
    });
    const { wrapper } = mount();
    await settle();
    for (const text of [
      '能够区分事实和判断',
      '时间紧张时容易急于回应',
      '此刻需要什么支持',
      '沟通前暂停',
      '记下一个事实与需要',
    ]) {
      expect(wrapper.text()).toContain(text);
    }
    const actionSection = [
      ...document.querySelectorAll('.report-section'),
    ].find((section) => section.textContent?.includes('具体行动'));
    expect(actionSection?.querySelector('a')?.textContent).toBe('message:1');
  });
});
