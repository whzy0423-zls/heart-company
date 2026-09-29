import dayjs from 'dayjs';
import { describe, expect, it } from 'vitest';

import { distributionAnalyticsScope } from './distribution-analytics-scope';

describe('distribution analytics date scope', () => {
  const now = dayjs('2026-10-02T12:00:00');
  const customRange: [ReturnType<typeof dayjs>, ReturnType<typeof dayjs>] = [
    dayjs('2026-09-03'), dayjs('2026-09-08'),
  ];

  it('keeps admin totals global and its trend at the supported 30-day scope', () => {
    const scope = distributionAnalyticsScope(false, 'custom', customRange, now);
    expect(scope.showDateFilter).toBe(false);
    expect(scope.params).toBeUndefined();
    expect(scope.trendTitle).toBe('近 30 天经营趋势');
    expect(scope.commissionTitle).toBe('累计分成金额');
    expect(scope.description).toContain('全局累计');
  });

  it.each([
    ['today', '2026-10-02', '2026-10-02'],
    ['yesterday', '2026-10-01', '2026-10-01'],
    ['last7', '2026-09-26', '2026-10-02'],
    ['custom', '2026-09-03', '2026-09-08'],
  ] as const)('aligns the agent %s filter and displayed period', (preset, startDate, endDate) => {
    const scope = distributionAnalyticsScope(true, preset, customRange, now);
    expect(scope.showDateFilter).toBe(true);
    expect(scope.params).toEqual({ startDate, endDate });
    expect(scope.trendTitle).toBe(`${startDate} 至 ${endDate} 经营趋势`);
    expect(scope.commissionTitle).toBe('区间分成金额');
    expect(scope.description).toContain('所选日期');
  });
});
