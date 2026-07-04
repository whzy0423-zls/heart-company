import { describe, expect, it, vi } from 'vitest';

import { requestClient } from '#/api/request';
import { getAppUserInsightsApi } from '#/api/core/app-customer';

import {
  enneagramLabel,
  getCenterSummary,
  getProfileSummary,
  getScoreTags,
  getUserInsightStatus,
} from './user-insights';

vi.mock('#/api/request', () => ({
  requestClient: {
    get: vi.fn(),
  },
}));

describe('user insights api', () => {
  it('loads app user insights with filters', async () => {
    const get = vi.mocked(requestClient.get);
    get.mockResolvedValue({ items: [], total: 0 });

    await getAppUserInsightsApi({ keyword: '138', page: 1, pageSize: 20 });

    expect(get).toHaveBeenCalledWith('/app-users/insights', {
      params: { keyword: '138', page: 1, pageSize: 20 },
    });
  });
});

describe('user insight display helpers', () => {
  it('formats enneagram type labels', () => {
    expect(enneagramLabel(5)).toBe('5号');
    expect(enneagramLabel(0)).toBe('-');
    expect(enneagramLabel(undefined)).toBe('-');
  });

  it('extracts profile summary from structured profile data', () => {
    expect(getProfileSummary({ summary: '理性且敏锐' })).toBe('理性且敏锐');
    expect(getProfileSummary({ title: '观察者' })).toBe('观察者');
    expect(getProfileSummary({})).toBe('暂无画像摘要');
  });

  it('summarizes whether an insight has extracted data', () => {
    expect(getUserInsightStatus({ memoryCount: 1, primaryType: 0 })).toBe(
      '已有沉淀',
    );
    expect(getUserInsightStatus({ memoryCount: 0, primaryType: 5 })).toBe(
      '已有画像',
    );
    expect(getUserInsightStatus({ memoryCount: 0, primaryType: 0 })).toBe(
      '待沉淀',
    );
  });

  it('formats center percentages from quiz results', () => {
    expect(
      getCenterSummary([
        { key: 'gut', name: '本能中心', pct: 22 },
        { key: 'heart', name: '情感中心', pct: 32 },
        { key: 'head', name: '思维中心', pct: 46 },
      ]),
    ).toBe('本能中心 22% / 情感中心 32% / 思维中心 46%');
    expect(getCenterSummary([])).toBe('-');
  });

  it('formats enneagram score tags in type order', () => {
    expect(getScoreTags({ 1: 6, 2: 4, 5: 8, 6: 9 })).toEqual([
      '1号 6分',
      '2号 4分',
      '5号 8分',
      '6号 9分',
    ]);
    expect(getScoreTags({})).toEqual([]);
  });
});
