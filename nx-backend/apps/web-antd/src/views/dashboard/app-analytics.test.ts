import { describe, expect, it, vi } from 'vitest';

import { requestClient } from '#/api/request';
import { getAppAnalyticsOverviewApi } from '#/api/core/app-analytics';

import {
  appAnalyticsStatCards,
  distributionRows,
  formatRecentMemoryTime,
  formatAppAnalyticsPercent,
  normalizeRecentRows,
} from './app-analytics';

vi.mock('#/api/request', () => ({
  requestClient: {
    get: vi.fn(),
  },
}));

describe('app analytics api', () => {
  it('loads App analytics overview from backend overview endpoint', async () => {
    const get = vi.mocked(requestClient.get);
    get.mockResolvedValue({ totalUsers: 0 });

    await getAppAnalyticsOverviewApi();

    expect(get).toHaveBeenCalledWith('/app-analytics/overview');
  });
});

describe('app analytics display helpers', () => {
  it('builds stable stat cards from overview totals', () => {
    expect(
      appAnalyticsStatCards({
        activeUsers: 12,
        extractedUsers: 7,
        newUsersToday: 3,
        totalUsers: 99,
      }).map((item) => [item.label, item.value]),
    ).toEqual([
      ['累计用户', 99],
      ['今日新增', 3],
      ['活跃用户', 12],
      ['已提炼用户', 7],
    ]);
  });

  it('shows backend active users and enabled status users as separate cards', () => {
    expect(
      appAnalyticsStatCards({
        activeUsers: 12,
        enabledUsers: 40,
        extractedUsers: 7,
        newUsersToday: 3,
        totalUsers: 99,
      } as any).map((item) => [item.label, item.value]),
    ).toEqual([
      ['累计用户', 99],
      ['今日新增', 3],
      ['活跃用户', 12],
      ['正常状态用户', 40],
      ['已提炼用户', 7],
    ]);
  });

  it('labels legacy activeUsers status counts as normal status users when status distribution confirms it', () => {
    expect(
      appAnalyticsStatCards({
        activeUsers: 12,
        extractedUsers: 7,
        newUsersToday: 3,
        statusDistribution: { active: 12, disabled: 1 },
        totalUsers: 13,
      }).map((item) => [item.label, item.value]),
    ).toEqual([
      ['累计用户', 13],
      ['今日新增', 3],
      ['正常状态用户', 12],
      ['已提炼用户', 7],
    ]);
  });

  it('formats member and status distribution rows with percentages', () => {
    expect(
      distributionRows([
        { count: 8, label: 'VIP 会员', value: 'vip' },
        { count: 2, label: '普通用户', value: 'free' },
      ]),
    ).toEqual([
      { count: 8, label: 'VIP 会员', percent: '80%', value: 'vip' },
      { count: 2, label: '普通用户', percent: '20%', value: 'free' },
    ]);
    expect(formatAppAnalyticsPercent(1, 3)).toBe('33.3%');
    expect(formatAppAnalyticsPercent(0, 0)).toBe('0%');
    expect(distributionRows({ active: 2, disabled: 1 }, { active: '正常' })).toEqual([
      { count: 2, label: '正常', percent: '66.7%', value: 'active' },
      { count: 1, label: 'disabled', percent: '33.3%', value: 'disabled' },
    ]);
  });

  it('normalizes recent rows and falls back to readable names', () => {
    expect(
      normalizeRecentRows([
        { id: 1, nickname: '', phone: '13800000000' },
        { id: 2, nickname: '小九', phone: '' },
      ]),
    ).toEqual([
      { id: 1, nickname: '', phone: '13800000000', title: '13800000000' },
      { id: 2, nickname: '小九', phone: '', title: '小九' },
    ]);
  });

  it('formats recent sediment time from lastExtractedAt before legacy lastMemoryAt', () => {
    expect(
      formatRecentMemoryTime({
        lastExtractedAt: '2026/07/04 11:00:00',
        lastMemoryAt: '2026/07/04 10:00:00',
      } as any),
    ).toBe('2026/07/04 11:00:00');
    expect(
      formatRecentMemoryTime({
        lastMemoryAt: '2026/07/04 10:00:00',
      }),
    ).toBe('2026/07/04 10:00:00');
    expect(formatRecentMemoryTime({ id: 2, latestMemory: 'legacy' } as any)).toBe(
      '-',
    );
  });
});
