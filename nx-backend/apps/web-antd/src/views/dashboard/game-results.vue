<script setup lang="ts">
import type { GameNameValue, GameOverview, GameTypeGenderItem } from '#/api';

import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { IconifyIcon } from '@vben/icons';

import { Button, Card, Col, message, Row, Table, Tag } from 'ant-design-vue';
import { BarChart, PieChart } from 'echarts/charts';
import {
  GridComponent,
  LegendComponent,
  TooltipComponent,
} from 'echarts/components';
import * as echarts from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';

import { getGameOverviewApi } from '#/api';

echarts.use([
  BarChart,
  CanvasRenderer,
  GridComponent,
  LegendComponent,
  PieChart,
  TooltipComponent,
]);

const palette = {
  center: ['#4f7cff', '#34b67a', '#f59e0b', '#8b5cf6', '#14b8a6'],
  female: '#e879b2',
  male: '#2aa6a1',
  primary: '#5b7cfa',
  unknown: '#aab4c3',
};

const typeChartRef = ref<HTMLDivElement>();
const centerChartRef = ref<HTMLDivElement>();
const genderChartRef = ref<HTMLDivElement>();
let typeChart: echarts.ECharts | undefined;
let centerChart: echarts.ECharts | undefined;
let genderChart: echarts.ECharts | undefined;

const loading = ref(false);
const overview = ref<GameOverview>({
  centerItems: [],
  genderItems: [],
  total: 0,
  typeGenderItems: [],
  typeItems: [],
});

const topType = computed(() => maxItem(overview.value.typeItems));
const topCenter = computed(() => maxItem(overview.value.centerItems));
const maleCount = computed(() => genderValue('male'));
const femaleCount = computed(() => genderValue('female'));
const typeRows = computed(() =>
  normalizeTypeGenderItems(overview.value.typeGenderItems),
);

const statCards = computed(() => [
  {
    color: palette.primary,
    icon: 'lucide:gamepad-2',
    label: '累计完成测试',
    value: overview.value.total,
  },
  {
    color: palette.male,
    icon: 'lucide:mars',
    label: '男生完成',
    value: maleCount.value,
  },
  {
    color: palette.female,
    icon: 'lucide:venus',
    label: '女生完成',
    value: femaleCount.value,
  },
  {
    color: '#34b67a',
    icon: 'lucide:badge-check',
    label: '最高九型结果',
    value: topType.value?.name ? `${topType.value.name}号` : '-',
  },
  {
    color: '#f59e0b',
    icon: 'lucide:pie-chart',
    label: '最高中心',
    value: topCenter.value?.name || '-',
  },
]);

const typeColumns = [
  { dataIndex: 'name', title: '九型类型', width: 120 },
  { dataIndex: 'total', title: '总次数', width: 100 },
  { dataIndex: 'male', title: '男生', width: 100 },
  { dataIndex: 'female', title: '女生', width: 100 },
  { dataIndex: 'unknown', title: '未知', width: 100 },
  { dataIndex: 'rate', title: '占比' },
];

async function loadOverview() {
  loading.value = true;
  try {
    overview.value = await getGameOverviewApi();
  } catch {
    message.error('小游戏统计加载失败，请稍后重试');
  } finally {
    loading.value = false;
    await nextTick();
    requestAnimationFrame(renderCharts);
  }
}

function renderCharts() {
  renderTypeChart();
  renderCenterChart();
  renderGenderChart();
}

function renderTypeChart() {
  if (!typeChartRef.value) return;
  typeChart ??= echarts.init(typeChartRef.value);
  const items = normalizeTypeGenderItems(overview.value.typeGenderItems);
  typeChart.setOption({
    color: [palette.male, palette.female, palette.unknown],
    grid: { bottom: 24, containLabel: true, left: 12, right: 16, top: 36 },
    legend: { top: 0 },
    tooltip: { trigger: 'axis' },
    xAxis: {
      data: items.map((item) => item.name),
      type: 'category',
    },
    yAxis: { minInterval: 1, type: 'value' },
    series: [
      {
        barMaxWidth: 30,
        itemStyle: { borderRadius: [6, 6, 0, 0] },
        data: items.map((item) => item.male),
        name: '男生',
        stack: 'total',
        type: 'bar',
      },
      {
        barMaxWidth: 30,
        itemStyle: { borderRadius: [6, 6, 0, 0] },
        data: items.map((item) => item.female),
        name: '女生',
        stack: 'total',
        type: 'bar',
      },
      {
        barMaxWidth: 30,
        itemStyle: { borderRadius: [6, 6, 0, 0] },
        data: items.map((item) => item.unknown),
        name: '未知',
        stack: 'total',
        type: 'bar',
      },
    ],
  });
}

function renderCenterChart() {
  if (!centerChartRef.value) return;
  centerChart ??= echarts.init(centerChartRef.value);
  centerChart.setOption({
    color: palette.center,
    legend: { bottom: 0 },
    tooltip: { trigger: 'item' },
    series: [
      {
        avoidLabelOverlap: true,
        data: overview.value.centerItems,
        name: '中心分布',
        radius: ['42%', '68%'],
        type: 'pie',
      },
    ],
  });
}

function renderGenderChart() {
  if (!genderChartRef.value) return;
  genderChart ??= echarts.init(genderChartRef.value);
  genderChart.setOption({
    color: [palette.male, palette.female, palette.unknown],
    legend: { bottom: 0 },
    tooltip: { trigger: 'item' },
    series: [
      {
        avoidLabelOverlap: true,
        data: normalizeGenderItems(overview.value.genderItems),
        name: '性别分布',
        radius: ['42%', '68%'],
        type: 'pie',
      },
    ],
  });
}

function normalizeTypeGenderItems(items: GameTypeGenderItem[]) {
  const total = overview.value.total || 0;
  return items.map((item) => ({
    ...item,
    name: item.name ? `${item.name}号` : '未知',
    rate: total > 0 ? `${((item.total / total) * 100).toFixed(1)}%` : '0%',
  }));
}

function normalizeGenderItems(items: GameNameValue[]) {
  return items.map((item) => ({
    ...item,
    name: genderLabel(item.name),
  }));
}

function genderLabel(value?: string) {
  if (value === 'male') return '男生';
  if (value === 'female') return '女生';
  return '未知';
}

function genderValue(value: string) {
  return (
    overview.value.genderItems.find((item) => item.name === value)?.value ?? 0
  );
}

function maxItem(items: GameNameValue[]) {
  let best: GameNameValue = { name: '', value: 0 };
  for (const item of items) {
    if (item.value > best.value) {
      best = item;
    }
  }
  return best;
}

function handleResize() {
  typeChart?.resize();
  centerChart?.resize();
  genderChart?.resize();
}

onMounted(() => {
  loadOverview();
  window.addEventListener('resize', handleResize);
});

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize);
  typeChart?.dispose();
  centerChart?.dispose();
  genderChart?.dispose();
  typeChart = undefined;
  centerChart = undefined;
  genderChart = undefined;
});
</script>

<template>
  <Page
    description="查看官网九型小游戏的完成次数、性别分布、九型结果分布和三中心分布。"
    title="小游戏统计"
  >
    <div class="game-actions">
      <Button :loading="loading" type="primary" @click="loadOverview">
        刷新
      </Button>
    </div>

    <div class="metrics-grid">
      <div v-for="item in statCards" :key="item.label">
        <Card :bordered="false" class="metric-panel">
          <div class="metric-card">
            <span class="metric-icon" :style="{ color: item.color }">
              <IconifyIcon :icon="item.icon" />
            </span>
            <div class="metric-content">
              <div class="metric-label">{{ item.label }}</div>
              <div class="metric-value">{{ item.value }}</div>
            </div>
          </div>
        </Card>
      </div>
    </div>

    <Row :gutter="[16, 16]" class="chart-section">
      <Col :xl="12" :xs="24">
        <Card :bordered="false" title="九型结果分布（按性别）">
          <div class="chart-wrap">
            <div ref="typeChartRef" class="chart"></div>
            <div v-if="loading" class="chart-loading">正在更新数据...</div>
          </div>
        </Card>
      </Col>
      <Col :xl="6" :md="12" :xs="24">
        <Card :bordered="false" title="性别分布">
          <div class="chart-wrap">
            <div ref="genderChartRef" class="chart"></div>
            <div v-if="loading" class="chart-loading">正在更新数据...</div>
          </div>
        </Card>
      </Col>
      <Col :xl="6" :md="12" :xs="24">
        <Card :bordered="false" title="三中心分布">
          <div class="chart-wrap">
            <div ref="centerChartRef" class="chart"></div>
            <div v-if="loading" class="chart-loading">正在更新数据...</div>
          </div>
        </Card>
      </Col>
    </Row>

    <Card :bordered="false" class="table-card" title="九型明细">
      <Table
        :columns="typeColumns"
        :data-source="typeRows"
        :loading="loading"
        :pagination="false"
        row-key="name"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'male'">
            <Tag color="green">{{ record.male }}</Tag>
          </template>
          <template v-if="column.dataIndex === 'female'">
            <Tag color="pink">{{ record.female }}</Tag>
          </template>
          <template v-if="column.dataIndex === 'unknown'">
            <Tag>{{ record.unknown }}</Tag>
          </template>
        </template>
      </Table>
    </Card>
  </Page>
</template>

<style scoped>
.game-actions {
  display: flex;
  justify-content: flex-end;
  margin: -2px 0 18px;
}

.metrics-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 16px;
}

.metric-card {
  display: flex;
  gap: 16px;
  align-items: center;
  min-height: 76px;
}

.metric-panel :deep(.ant-card-body) {
  padding: 20px 18px;
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

.metric-content {
  min-width: 0;
}

.metric-label {
  margin-bottom: 6px;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 13px;
  color: hsl(var(--muted-foreground));
  white-space: nowrap;
}

.metric-value {
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 24px;
  font-weight: 600;
  line-height: 1.1;
  color: hsl(var(--foreground));
  white-space: nowrap;
}

.chart-section {
  margin-top: 16px;
}

.chart-wrap {
  position: relative;
  min-height: 340px;
}

.chart {
  width: 100%;
  height: 340px;
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

.table-card {
  margin-top: 16px;
}

@media (max-width: 640px) {
  .game-actions {
    justify-content: flex-start;
  }

  .metrics-grid {
    grid-template-columns: 1fr;
  }

  .chart {
    height: 300px;
  }
}

@media (min-width: 641px) and (max-width: 1280px) {
  .metrics-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
