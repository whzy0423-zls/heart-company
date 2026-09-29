import type { AgentDistributionAnalytics, DistributionAnalytics } from '../../api/core/distribution';

import dayjs from 'dayjs';
import { describe, expect, it, vi } from 'vitest';

import { useDistributionAnalytics } from './distribution-analytics-loader';
import { distributionAnalyticsScope } from './distribution-analytics-scope';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, reject, resolve };
}

const now = dayjs('2026-10-02T12:00:00');
const range: [typeof now, typeof now] = [now.subtract(6, 'day'), now];
const previousScope = distributionAnalyticsScope(true, 'last7', range, now);
const todayScope = distributionAnalyticsScope(true, 'today', range, now);
const yesterdayScope = distributionAnalyticsScope(true, 'yesterday', range, now);
const emptyAnalytics: DistributionAnalytics = {
  agentRankings: [],
  summary: {
    activeAgents: 0, commissionRecordCount: 0, pausedAgents: 0,
    pendingCommissionAmount: 0, settledCommissionAmount: 0, totalAgents: 0,
    totalCommissionAmount: 0, totalOrderAmount: 0,
  },
  trend: [],
};

function agentData(totalCommissionAmount: number): AgentDistributionAnalytics {
  return {
    orders: [], users: [], trend: [],
    summary: { ...emptyAnalytics.summary, totalCommissionAmount },
  };
}

function setup() {
  const loadAdmin = vi.fn<() => Promise<DistributionAnalytics>>();
  const loadAgent = vi.fn<(params?: { startDate?: string; endDate?: string }) => Promise<AgentDistributionAnalytics>>();
  const onError = vi.fn();
  const loader = useDistributionAnalytics({
    initialAnalytics: emptyAnalytics,
    initialScope: previousScope,
    loadAdmin, loadAgent, onError,
  });
  return { ...loader, loadAdmin, loadAgent, onError };
}

describe('distribution analytics asynchronous loading', () => {
  it('uses the requested dates but keeps the loaded title until data succeeds', async () => {
    const state = setup();
    const request = deferred<AgentDistributionAnalytics>();
    state.loadAgent.mockReturnValue(request.promise);
    const result = state.load(todayScope);
    expect(state.loadAgent).toHaveBeenCalledWith(todayScope.params);
    expect(state.analyticsScope.value).toEqual(previousScope);
    expect(state.agentAnalytics.value).toBeNull();
    expect(state.analyticsLoading.value).toBe(true);

    request.resolve(agentData(1200));
    expect(await result).toBe(true);
    expect(state.analyticsScope.value).toEqual(todayScope);
    expect(state.agentAnalytics.value?.summary.totalCommissionAmount).toBe(1200);
    expect(state.analyticsLoading.value).toBe(false);
  });

  it('preserves the successful data and title when a changed date request fails', async () => {
    const state = setup();
    state.loadAgent.mockResolvedValueOnce(agentData(700));
    await state.load(previousScope);
    const previousData = state.agentAnalytics.value;
    state.loadAgent.mockRejectedValueOnce(new Error('network'));

    expect(await state.load(todayScope)).toBe(false);
    expect(state.analyticsScope.value).toEqual(previousScope);
    expect(state.agentAnalytics.value).toBe(previousData);
    expect(state.onError).toHaveBeenCalledOnce();
    expect(state.analyticsLoading.value).toBe(false);
  });

  it('ignores a slow earlier success after the newest dates have loaded', async () => {
    const state = setup();
    const oldRequest = deferred<AgentDistributionAnalytics>();
    state.loadAgent.mockReturnValueOnce(oldRequest.promise).mockResolvedValueOnce(agentData(200));
    const oldResult = state.load(yesterdayScope);
    expect(await state.load(todayScope)).toBe(true);
    oldRequest.resolve(agentData(100));

    expect(await oldResult).toBe(false);
    expect(state.analyticsScope.value).toEqual(todayScope);
    expect(state.agentAnalytics.value?.summary.totalCommissionAmount).toBe(200);
  });

  it('keeps loading and the prior display when an old response finishes during a newer request', async () => {
    const state = setup();
    const oldRequest = deferred<AgentDistributionAnalytics>();
    const newRequest = deferred<AgentDistributionAnalytics>();
    state.loadAgent.mockReturnValueOnce(oldRequest.promise).mockReturnValueOnce(newRequest.promise);
    const oldResult = state.load(yesterdayScope);
    const newResult = state.load(todayScope);
    oldRequest.resolve(agentData(100));
    expect(await oldResult).toBe(false);
    expect(state.analyticsLoading.value).toBe(true);
    expect(state.analyticsScope.value).toEqual(previousScope);
    expect(state.agentAnalytics.value).toBeNull();

    newRequest.resolve(agentData(200));
    expect(await newResult).toBe(true);
    expect(state.analyticsLoading.value).toBe(false);
    expect(state.analyticsScope.value).toEqual(todayScope);
  });

  it('suppresses obsolete errors after the newest request succeeds', async () => {
    const state = setup();
    const oldRequest = deferred<AgentDistributionAnalytics>();
    state.loadAgent.mockReturnValueOnce(oldRequest.promise).mockResolvedValueOnce(agentData(200));
    const oldResult = state.load(yesterdayScope);
    await state.load(todayScope);
    oldRequest.reject(new Error('obsolete network error'));

    expect(await oldResult).toBe(false);
    expect(state.onError).not.toHaveBeenCalled();
    expect(state.analyticsScope.value).toEqual(todayScope);
    expect(state.agentAnalytics.value?.summary.totalCommissionAmount).toBe(200);
    expect(state.analyticsLoading.value).toBe(false);
  });

  it('retains the previous successful display if the latest request fails and an older one succeeds later', async () => {
    const state = setup();
    state.loadAgent.mockResolvedValueOnce(agentData(700));
    await state.load(previousScope);
    const oldRequest = deferred<AgentDistributionAnalytics>();
    state.loadAgent.mockReturnValueOnce(oldRequest.promise).mockRejectedValueOnce(new Error('latest failed'));
    const oldResult = state.load(yesterdayScope);
    expect(await state.load(todayScope)).toBe(false);
    oldRequest.resolve(agentData(100));

    expect(await oldResult).toBe(false);
    expect(state.analyticsScope.value).toEqual(previousScope);
    expect(state.agentAnalytics.value?.summary.totalCommissionAmount).toBe(700);
    expect(state.onError).toHaveBeenCalledOnce();
  });

  it('loads admin totals through the admin endpoint and commits the supported global scope', async () => {
    const state = setup();
    const adminScope = distributionAnalyticsScope(false, 'today', range, now);
    const data = { ...emptyAnalytics, summary: { ...emptyAnalytics.summary, totalCommissionAmount: 900 } };
    state.loadAdmin.mockResolvedValue(data);

    expect(await state.load(adminScope)).toBe(true);
    expect(state.loadAdmin).toHaveBeenCalledWith();
    expect(state.loadAgent).not.toHaveBeenCalled();
    expect(state.analytics.value).toBe(data);
    expect(state.agentAnalytics.value).toBeNull();
    expect(state.analyticsScope.value).toEqual(adminScope);
  });

  it('invalidates pending responses when the page is disposed', async () => {
    const state = setup();
    const request = deferred<AgentDistributionAnalytics>();
    state.loadAgent.mockReturnValue(request.promise);
    const result = state.load(todayScope);
    state.dispose();
    request.resolve(agentData(1200));

    expect(await result).toBe(false);
    expect(state.agentAnalytics.value).toBeNull();
    expect(state.analyticsScope.value).toEqual(previousScope);
    expect(state.analyticsLoading.value).toBe(false);
    expect(state.onError).not.toHaveBeenCalled();
  });
});
