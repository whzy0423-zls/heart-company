import type { Dayjs } from 'dayjs';

import dayjs from 'dayjs';

export type DistributionDatePreset = 'custom' | 'last7' | 'today' | 'yesterday';

export function distributionAnalyticsScope(
  isAgent: boolean,
  preset: DistributionDatePreset,
  customRange: [Dayjs, Dayjs],
  now = dayjs(),
) {
  if (!isAgent) {
    return {
      commissionTitle: '累计分成金额',
      description: '指标与代理排行为全局累计，趋势展示近 30 天。',
      params: undefined,
      showDateFilter: false,
      trendTitle: '近 30 天经营趋势',
    };
  }
  const range: [Dayjs, Dayjs] = preset === 'custom'
    ? customRange
    : preset === 'yesterday'
      ? [now.subtract(1, 'day'), now.subtract(1, 'day')]
      : preset === 'today'
        ? [now, now]
        : [now.subtract(6, 'day'), now];
  const startDate = range[0].format('YYYY-MM-DD');
  const endDate = range[1].format('YYYY-MM-DD');
  return {
    commissionTitle: '区间分成金额',
    description: '金额、订单与趋势按所选日期统计，代理人数为当前人数。',
    params: { endDate, startDate },
    showDateFilter: true,
    trendTitle: `${startDate} 至 ${endDate} 经营趋势`,
  };
}
