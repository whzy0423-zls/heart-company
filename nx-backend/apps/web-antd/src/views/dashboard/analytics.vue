<script setup lang="ts">
import type { AnalyticsOverview, AnalyticsSeriesPoint } from '#/api';

import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';

import { Page } from '@vben/common-ui';
import { IconifyIcon } from '@vben/icons';

import { BarChart, LineChart } from 'echarts/charts';
import {
  GridComponent,
  LegendComponent,
  TooltipComponent,
} from 'echarts/components';
import * as echarts from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';

import {
  Button,
  Card,
  Col,
  DatePicker,
  Row,
  Space,
  Statistic,
  message,
} from 'ant-design-vue';
import dayjs, { type Dayjs } from 'dayjs';

import { getAnalyticsOverviewApi } from '#/api';

echarts.use([
  BarChart,
  CanvasRenderer,
  GridComponent,
  LegendComponent,
  LineChart,
  TooltipComponent,
]);

type DateRange = [Dayjs, Dayjs];

const chartRef = ref<HTMLDivElement>();
let chart: echarts.ECharts | undefined;

const loading = ref(false);
const dateRange = ref<DateRange>([dayjs().subtract(6, 'day'), dayjs()]);
const overview = ref<AnalyticsOverview>({
  rangeLeads: 0,
  rangeVisits: 0,
  series: [],
  todayLeads: 0,
  todayVisits: 0,
  totalLeads: 0,
  totalVisits: 0,
});

const stats = computed(() => [
  {
    color: '#2563eb',
    icon: 'lucide:users',
    label: '累计浏览人数',
    value: overview.value.totalVisits,
  },
  {
    color: '#0f766e',
    icon: 'lucide:calendar-check',
    label: '今日浏览人数',
    value: overview.value.todayVisits,
  },
  {
    color: '#16a34a',
    icon: 'lucide:mouse-pointer-click',
    label: '累计询盘客户',
    value: overview.value.totalLeads,
  },
  {
    color: '#ea580c',
    icon: 'lucide:trending-up',
    label: '今日询盘客户',
    value: overview.value.todayLeads,
  },
]);

const rangeStats = computed(() => [
  {
    color: '#2563eb',
    icon: 'lucide:users',
    label: '区间浏览人数',
    value: overview.value.rangeVisits,
  },
  {
    color: '#16a34a',
    icon: 'lucide:mouse-pointer-click',
    label: '区间询盘客户',
    value: overview.value.rangeLeads,
  },
  {
    color: '#ea580c',
    icon: 'lucide:trending-up',
    label: '区间询盘转化率',
    suffix: '%',
    value: conversionRate(overview.value.rangeLeads, overview.value.rangeVisits),
  },
]);

const analysisText = computed(() => {
  const series = overview.value.series;
  if (series.length === 0) {
    return '当前筛选区间暂无访问和询盘数据，可以先通过官网访问或提交报名表单积累数据。';
  }
  const topVisit = maxBy(series, 'visits');
  const topLead = maxBy(series, 'leads');
  const rate = conversionRate(overview.value.rangeLeads, overview.value.rangeVisits);
  return `当前区间共 ${overview.value.rangeVisits} 位访客，产生 ${overview.value.rangeLeads} 条询盘，区间转化率 ${rate}%。访问峰值出现在 ${topVisit.date}（${topVisit.visits} 位访客），询盘峰值出现在 ${topLead.date}（${topLead.leads} 条询盘）。`;
});

async function loadOverview() {
  loading.value = true;
  try {
    const [start, end] = dateRange.value;
    overview.value = await getAnalyticsOverviewApi({
      endDate: end.format('YYYY-MM-DD'),
      startDate: start.format('YYYY-MM-DD'),
    });
  } catch (error) {
    message.error('数据概览加载失败，请稍后重试');
  } finally {
    loading.value = false;
    await nextTick();
    requestAnimationFrame(() => {
      renderChart();
      chart?.resize();
    });
  }
}

function renderChart() {
  if (!chartRef.value) return;
  chart ??= echarts.init(chartRef.value);
  const dates = overview.value.series.map((item) => item.date.slice(5));
  chart.setOption({
    color: ['#2563eb', '#16a34a'],
    grid: {
      bottom: 30,
      containLabel: true,
      left: 12,
      right: 16,
      top: 48,
    },
    legend: {
      data: ['浏览人数', '询盘客户'],
      top: 8,
    },
    tooltip: {
      trigger: 'axis',
    },
    xAxis: {
      axisTick: { alignWithLabel: true },
      data: dates,
      type: 'category',
    },
    yAxis: {
      minInterval: 1,
      type: 'value',
    },
    series: [
      {
        barMaxWidth: 28,
        data: overview.value.series.map((item) => item.visits),
        name: '浏览人数',
        type: 'bar',
      },
      {
        data: overview.value.series.map((item) => item.leads),
        name: '询盘客户',
        smooth: true,
        type: 'line',
      },
    ],
  });
}

function handleResize() {
  chart?.resize();
}

function conversionRate(leads: number, visits: number) {
  if (visits <= 0) return 0;
  return Number(((leads / visits) * 100).toFixed(1));
}

function maxBy(items: AnalyticsSeriesPoint[], key: 'leads' | 'visits') {
  return items.reduce((best, item) => (item[key] > best[key] ? item : best), items[0]);
}

watch(dateRange, () => {
  loadOverview();
});

onMounted(() => {
  loadOverview();
  window.addEventListener('resize', handleResize);
});

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize);
  chart?.dispose();
  chart = undefined;
});
</script>

<template>
  <Page
    description="查看官网访问与报名询盘数据，便于快速判断线索趋势和转化情况。"
    title="数据概览"
  >
    <div class="analytics-actions">
      <Space :size="12" wrap>
        <span class="toolbar-label">统计时间</span>
        <div class="toolbar-range">
          <DatePicker.RangePicker
            v-model:value="dateRange"
            :allow-clear="false"
            :disabled-date="(current) => current && current > dayjs().endOf('day')"
          />
        </div>
        <Button :loading="loading" @click="loadOverview">刷新</Button>
      </Space>
    </div>

    <Row :gutter="[16, 16]" class="metrics-row">
      <Col v-for="item in stats" :key="item.label" :lg="6" :md="12" :xs="24">
        <Card :bordered="false" class="metric-panel">
          <div class="metric-card">
            <span class="metric-icon" :style="{ color: item.color }">
              <IconifyIcon :icon="item.icon" />
            </span>
            <Statistic :title="item.label" :value="item.value" />
          </div>
        </Card>
      </Col>
    </Row>

    <Row :gutter="[16, 16]" class="analytics-section">
      <Col :lg="16" :xs="24">
        <Card :bordered="false" title="访问与询盘趋势">
          <div class="chart-wrap">
            <div ref="chartRef" class="analytics-chart"></div>
            <div v-if="loading" class="chart-loading">正在更新数据...</div>
          </div>
        </Card>
      </Col>
      <Col :lg="8" :xs="24">
        <Card :bordered="false" title="区间分析">
          <div class="range-stats">
            <div
              v-for="item in rangeStats"
              :key="item.label"
              class="range-stat-item"
            >
              <span class="range-stat-icon" :style="{ color: item.color }">
                <IconifyIcon :icon="item.icon" />
              </span>
              <Statistic
                :suffix="item.suffix"
                :title="item.label"
                :value="item.value"
              />
            </div>
          </div>
          <p class="analysis-copy">{{ analysisText }}</p>
        </Card>
      </Col>
    </Row>
  </Page>
</template>

<style scoped>
.analytics-actions {
  display: flex;
  justify-content: flex-end;
  width: 100%;
  margin: -2px 0 18px;
}

.toolbar-label {
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}

.toolbar-range {
  min-width: 264px;
}

.metrics-row {
  margin-bottom: 0;
}

.analytics-section {
  margin-top: 16px;
}

.chart-wrap {
  position: relative;
  min-height: 360px;
}

.analytics-chart {
  width: 100%;
  height: 360px;
}

.chart-loading {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: hsl(var(--muted-foreground));
  pointer-events: none;
  background: hsl(var(--card) / 62%);
}

.metric-card {
  display: flex;
  min-height: 76px;
  gap: 16px;
  align-items: center;
}

.metric-panel :deep(.ant-card-body) {
  padding: 20px 24px;
}

.metric-card :deep(.ant-statistic-title) {
  margin-bottom: 6px;
  font-size: 13px;
}

.metric-card :deep(.ant-statistic-content) {
  line-height: 1;
}

.metric-icon {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 42px;
  height: 42px;
  background: hsl(var(--accent) / 52%);
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
}

.range-stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}

.range-stat-item {
  min-width: 0;
  padding: 12px;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
}

.range-stat-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  margin-bottom: 8px;
  background: hsl(var(--accent) / 42%);
  border-radius: 8px;
}

.analysis-copy {
  padding: 14px;
  margin: 18px 0 0;
  font-size: 14px;
  line-height: 1.75;
  color: hsl(var(--foreground));
  background: hsl(var(--accent) / 34%);
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
}

@media (max-width: 640px) {
  .analytics-actions {
    justify-content: flex-start;
  }

  .toolbar-range {
    width: 100%;
    min-width: 0;
  }

  .toolbar-range :deep(.ant-picker) {
    width: 100%;
  }

  .analytics-chart {
    height: 300px;
  }

  .range-stats {
    grid-template-columns: 1fr;
  }
}
</style>
