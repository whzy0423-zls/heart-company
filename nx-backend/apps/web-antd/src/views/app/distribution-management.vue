<script setup lang="ts">
import { formatYuan } from './distribution-format';
import { distributionAnalyticsScope } from './distribution-analytics-scope';
import { useDistributionAnalytics } from './distribution-analytics-loader';
import type { Dayjs } from 'dayjs';

import type { AppCustomer } from '#/api';
import type {
  DistributionAgent,
  DistributionAnalytics,
  DistributionAnalyticsAgentRanking,
} from '#/api/core/distribution';

import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';

import { Page } from '@vben/common-ui';
import { useAccessStore } from '@vben/stores';
import {
  Alert,
  Button,
  Card,
  DatePicker,
  Col,
  Form,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Statistic,
  Table,
  Tag,
  Typography,
  message,
} from 'ant-design-vue';
import dayjs from 'dayjs';
import { BarChart, LineChart } from 'echarts/charts';
import {
  GridComponent,
  LegendComponent,
  TooltipComponent,
} from 'echarts/components';
import * as echarts from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';

import { getAppCustomerDetailApi, getAppCustomerListApi } from '#/api';
import DistributionPosterComposer from './distribution-poster-composer.vue';
import {
  createAgentDistributionChildApi,
  createDistributionAgentApi,
  getAgentDistributionAgentsApi,
  getAgentDistributionAnalyticsApi,
  getDistributionAgentsApi,
  getDistributionAnalyticsApi,
  updateDistributionAgentApi,
  updateDistributionAgentStatusApi,
} from '#/api/core/distribution';

echarts.use([
  BarChart,
  CanvasRenderer,
  GridComponent,
  LegendComponent,
  LineChart,
  TooltipComponent,
]);

const access = useAccessStore();
const canWrite = computed(() => access.accessCodes.includes('Customer:App:Write'));
const isAgentBackoffice = computed(() => access.accessCodes.includes('Agent:Distribution:View') && !access.accessCodes.includes('Customer:App:List'));
const canAgentWrite = computed(() => access.accessCodes.includes('Agent:Distribution:Write'));
const canCreateAgent = computed(() => canWrite.value || (canAgentWrite.value && currentAgent.value?.level === 2));

const emptyAnalytics: DistributionAnalytics = {
  agentRankings: [],
  summary: {
    activeAgents: 0,
    commissionRecordCount: 0,
    pausedAgents: 0,
    pendingCommissionAmount: 0,
    settledCommissionAmount: 0,
    totalAgents: 0,
    totalCommissionAmount: 0,
    totalOrderAmount: 0,
  },
  trend: [],
};

const agents = ref<DistributionAgent[]>([]);
const currentAgent = ref<DistributionAgent | null>(null);
const datePreset = ref<'custom' | 'last7' | 'today' | 'yesterday'>('last7');
const customDateRange = ref<[Dayjs, Dayjs]>([dayjs().subtract(6, 'day'), dayjs()]);
const {
  analytics,
  agentAnalytics,
  analyticsScope,
  analyticsLoading,
  load: fetchAnalytics,
  dispose: disposeAnalytics,
} = useDistributionAnalytics({
  initialAnalytics: emptyAnalytics,
  initialScope: queryAnalyticsScope(),
  loadAdmin: getDistributionAnalyticsApi,
  loadAgent: getAgentDistributionAnalyticsApi,
  onError: () => message.error('经营分析加载失败'),
});
const loading = ref(false);
const creating = ref(false);
const actionLoadingId = ref<number | null>(null);
const createAgentModalOpen = ref(false);
const editAgentModalOpen = ref(false);
const editingAgent = ref<DistributionAgent | null>(null);
const editingAgentCode = ref('');
const selectedParentAgent = ref<DistributionAgent | null>(null);
const selectedCustomerId = ref<number>();
const customerOptions = ref<AppCustomer[]>([]);
const customerSearching = ref(false);
const trendChartRef = ref<HTMLDivElement>();
const fixedTableScroll = { x: 820, y: 320 };
const fixedWideTableScroll = { x: 1240, y: 360 };
const userConsumptionColumns = [
  { dataIndex: 'id', title: '用户 ID', width: 100 },
  { dataIndex: 'nickname', title: '昵称', width: 140 },
  { dataIndex: 'memberLevel', title: '会员', width: 100 },
  { dataIndex: 'orderCount', title: '消费订单数', width: 120 },
  { dataIndex: 'orderAmount', title: '消费金额(元)', width: 140 },
  { dataIndex: 'boundAt', title: '绑定时间', width: 180 },
];
const orderColumns = [
  { dataIndex: 'id', title: '订单 ID', width: 100 },
  { dataIndex: 'outTradeNo', title: '订单号', width: 180 },
  { dataIndex: 'appUserId', title: '用户 ID', width: 100 },
  { dataIndex: 'amount', title: '消费金额(元)', width: 140 },
  { dataIndex: 'status', title: '状态', width: 100 },
  { dataIndex: 'paidAt', title: '支付时间', width: 180 },
];
let customerSearchRequestId = 0;
let trendChart: echarts.ECharts | undefined;

const columns = [
  { dataIndex: 'id', fixed: 'left' as const, title: '代理 ID', width: 100 },
  { dataIndex: 'appUserAccount', title: '代理人账号', width: 190 },
  { dataIndex: 'agentCode', title: '代理号', width: 180 },
  { dataIndex: 'appUserName', title: '用户名称', width: 160 },
  { dataIndex: 'level', title: '代理等级', width: 120 },
  { dataIndex: 'development', title: '发展概况', width: 260 },
  { dataIndex: 'status', title: '状态', width: 110 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 210 },
];

const childAgentColumns = [
  { dataIndex: 'id', title: '代理 ID', width: 100 },
  { dataIndex: 'appUserAccount', title: '代理人账号', width: 180 },
  { dataIndex: 'agentCode', title: '代理号', width: 160 },
  { dataIndex: 'appUserName', title: '用户名称', width: 160 },
  { dataIndex: 'level', title: '代理等级', width: 120 },
  { dataIndex: 'directUserCount', title: '直属客户', width: 110 },
  { dataIndex: 'status', title: '状态', width: 110 },
];

const rankingColumns = [
  { dataIndex: 'agentCode', title: '代理号', width: 150 },
  { dataIndex: 'appUserName', title: '用户名称', width: 160 },
  { dataIndex: 'directUserCount', title: '直属客户', width: 110 },
  { dataIndex: 'childAgentCount', title: '下级代理', width: 110 },
  { dataIndex: 'orderCount', title: '订单数', width: 100 },
  { dataIndex: 'orderAmount', title: '订单金额(元)', width: 130 },
  { dataIndex: 'commissionAmount', title: '分成金额(元)', width: 130 },
  { dataIndex: 'pendingCommissionAmount', title: '待结算金额(元)', width: 140 },
  { dataIndex: 'status', title: '状态', width: 100 },
];

// Rankings contain stable IDs; resolve the display name from the existing
// agent response so account identity and commission calculations stay intact.
const agentsByUserId = computed(() => new Map(
  [...agents.value, ...(currentAgent.value ? [currentAgent.value] : [])]
    .map((agent) => [agent.appUserId, agent]),
));
const rankingNames = ref(new Map<number, string>());
const missingRankingUserIds = computed(() => {
  if (isAgentBackoffice.value || !access.accessCodes.includes('Customer:App:List')) return [];
  return [...new Set(analytics.value.agentRankings.map((row) => row.appUserId))]
    .filter((id) => !agentsByUserId.value.has(id));
});

// The agent list is capped at 200; older agents can still appear in rankings.
watch([missingRankingUserIds, loading], async ([ids, listLoading], _old, onCleanup) => {
  let cancelled = false;
  onCleanup(() => { cancelled = true; });
  rankingNames.value = new Map(ids.map((id) => [id, '加载中…']));
  if (listLoading) return;
  const names = await Promise.all(ids.map(async (id): Promise<[number, string]> => {
    try {
      const customer = await getAppCustomerDetailApi(id);
      return [id, customer.nickname?.trim() || customer.account?.trim() || customer.phone?.trim() || '未设置名称'];
    } catch {
      return [id, '暂无名称'];
    }
  }));
  if (!cancelled) rankingNames.value = new Map(names);
});

function agentDisplayName(agent?: DistributionAgent) {
  if (!agent) return loading.value ? '加载中…' : '暂无名称';
  return agent.appUserNickname?.trim()
    || agent.appUserAccount?.trim()
    || agent.appUserPhone?.trim()
    || '未设置名称';
}

function rankingDisplayName(appUserId: number) {
  const agent = agentsByUserId.value.get(appUserId);
  return agent ? agentDisplayName(agent) : rankingNames.value.get(appUserId) || agentDisplayName();
}

const activeAgentCount = computed(
  () => agents.value.filter((item) => item.status === 'active').length,
);
const pausedAgentCount = computed(
  () => agents.value.filter((item) => item.status === 'paused').length,
);
const existingAgentCustomerIds = computed(
  () => new Set(agents.value.map((item) => item.appUserId)),
);
const selectedCustomer = computed(() =>
  customerOptions.value.find((item) => item.id === selectedCustomerId.value),
);
const childAgentsForSelectedParent = computed(() => {
  if (!selectedParentAgent.value) return [];
  return agents.value.filter(
    (item) => item.parentAgentId === selectedParentAgent.value?.id,
  );
});
const analyticsSummary = computed(() => agentAnalytics.value?.summary ?? analytics.value.summary);
const userConsumptionRows = computed(() => agentAnalytics.value?.users ?? []);
const orderRows = computed(() => agentAnalytics.value?.orders ?? []);
const businessMetricCards = computed(() => [
  {
    color: '#2563EB',
    title: analyticsScope.value.commissionTitle,
    value: formatYuan(analyticsSummary.value.totalCommissionAmount),
  },
  {
    color: '#F97316',
    title: '待结算金额',
    value: formatYuan(analyticsSummary.value.pendingCommissionAmount),
  },
  {
    color: '#0EA5E9',
    title: '订单金额',
    value: formatYuan(analyticsSummary.value.totalOrderAmount),
  },
  {
    color: '#16A34A',
    title: '佣金记录',
    value: analyticsSummary.value.commissionRecordCount,
  },
]);
const trendChartOption = computed(() => ({
  color: ['#2563EB', '#F97316', '#16A34A'],
  grid: {
    bottom: 28,
    containLabel: true,
    left: 12,
    right: 16,
    top: 46,
  },
  legend: {
    data: ['订单金额', '分成金额', '佣金笔数'],
    top: 6,
  },
  tooltip: {
    trigger: 'axis',
  },
  xAxis: {
    data: (agentAnalytics.value?.trend ?? analytics.value.trend).map((item) => item.date.slice(5)),
    type: 'category',
  },
  yAxis: [
    { name: '金额(元)', type: 'value' },
    { minInterval: 1, name: '笔数', type: 'value' },
  ],
  series: [
    {
      barMaxWidth: 28,
      data: (agentAnalytics.value?.trend ?? analytics.value.trend).map((item) => item.orderAmount / 100),
      name: '订单金额',
      type: 'bar',
    },
    {
      data: (agentAnalytics.value?.trend ?? analytics.value.trend).map((item) => item.commissionAmount / 100),
      name: '分成金额',
      smooth: true,
      type: 'line',
    },
    {
      data: (agentAnalytics.value?.trend ?? analytics.value.trend).map((item) => item.commissionCount),
      name: '佣金笔数',
      smooth: true,
      type: 'line',
      yAxisIndex: 1,
    },
  ],
}));

function agentOf(record: Record<string, any>) {
  return record as DistributionAgent;
}

function rankingOf(record: Record<string, any>) {
  return record as DistributionAnalyticsAgentRanking;
}


function formatDateTime(value: string | null | undefined) {
  if (!value) return '-';
  const parsed = dayjs(value);
  if (!parsed.isValid()) return value;
  return dayjs(value).format('YYYY-MM-DD HH:mm:ss');
}

function excelCell(value: unknown) {
  const text = value == null ? '' : String(value);
  const safeText = /^[=+\-@]/.test(text) ? `	${text}` : text;
  return safeText
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;');
}

function downloadExcelHtml(fileName: string, sheetName: string, rows: unknown[][]) {
  const tableRows = rows
    .map(
      (row) => `<tr>${row.map((cell) => `<td>${excelCell(cell)}</td>`).join('')}</tr>`,
    )
    .join('');
  const html = `﻿<html><head><meta charset="UTF-8" /></head><body><table><caption>${excelCell(sheetName)}</caption>${tableRows}</table></body></html>`;
  const blob = new Blob([html], {
    type: 'application/vnd.ms-excel;charset=utf-8',
  });
  const link = document.createElement('a');
  const url = URL.createObjectURL(blob);
  link.href = url;
  link.download = `${fileName}-${dayjs().format('YYYYMMDD-HHmmss')}.xls`;
  document.body.append(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function exportDistributionExcel(kind: 'orders' | 'users') {
  if (kind === 'users') {
    const rows = userConsumptionRows.value;
    if (rows.length === 0) {
      message.warning('暂无可导出的数据');
      return;
    }
    downloadExcelHtml('下级用户消费汇总', '下级用户消费汇总', [
      ['用户 ID', '昵称', '会员', '消费订单数', '消费金额(元)', '绑定时间'],
      ...rows.map((row) => [
        row.id,
        row.nickname,
        row.memberLevel,
        row.orderCount,
        formatYuan(row.orderAmount),
        formatDateTime(row.boundAt),
      ]),
    ]);
    return;
  }
  const rows = orderRows.value;
  if (rows.length === 0) {
    message.warning('暂无可导出的数据');
    return;
  }
  downloadExcelHtml('下级订单明细', '下级订单明细', [
    ['订单 ID', '订单号', '用户 ID', '消费金额(元)', '状态', '支付时间'],
    ...rows.map((row) => [
      row.id,
      row.outTradeNo,
      row.appUserId,
      formatYuan(row.amount),
      row.status,
      formatDateTime(row.paidAt),
    ]),
  ]);
}

function queryAnalyticsScope() {
  return distributionAnalyticsScope(
    isAgentBackoffice.value,
    datePreset.value,
    customDateRange.value,
    dayjs(),
  );
}

function isRankingMoneyColumn(dataIndex: unknown) {
  return ['orderAmount', 'commissionAmount', 'pendingCommissionAmount'].includes(
    String(dataIndex),
  );
}

function rankingMoneyValue(
  record: Record<string, any>,
  dataIndex: unknown,
) {
  const item = rankingOf(record);
  const key = String(dataIndex) as
    | 'commissionAmount'
    | 'orderAmount'
    | 'pendingCommissionAmount';
  return formatYuan(item[key]);
}

function levelText(level: number) {
  return level === 1 ? '一级代理' : `${level} 级代理`;
}

function statusText(status: string) {
  switch (status) {
    case 'active': {
      return '正常';
    }
    case 'paused': {
      return '已暂停';
    }
    default: {
      return status || '-';
    }
  }
}

function statusColor(status: string) {
  return status === 'active' ? 'green' : 'default';
}

async function copyCurrentAgentCode() {
  if (!currentAgent.value?.agentCode) {
    message.warning('当前代理号还在加载中');
    return;
  }
  await navigator.clipboard.writeText(currentAgent.value.agentCode);
  message.success('代理号已复制，可以分享给别人');
}

function openChildAgents(agent: DistributionAgent) {
  selectedParentAgent.value = agent;
}

function openEditAgentCode(agent: DistributionAgent) {
  editingAgent.value = agent;
  editingAgentCode.value = agent.agentCode;
  editAgentModalOpen.value = true;
}

async function saveAgentCode() {
  const agent = editingAgent.value;
  const code = editingAgentCode.value.trim().toUpperCase();
  if (!agent) return;
  if (!/^[ABCDEFGHJKLMNPQRSTUVWXYZ23456789]{6}$/.test(code)) {
    message.warning('代理号必须是 6 位大写字母或数字，且不含易混淆字符');
    return;
  }
  actionLoadingId.value = agent.id;
  try {
    await updateDistributionAgentApi(agent.id, { agentCode: code });
    agent.agentCode = code;
    if (currentAgent.value?.id === agent.id) currentAgent.value.agentCode = code;
    editAgentModalOpen.value = false;
    message.success('代理号已更新');
  } catch {
    message.error('代理号更新失败，可能已被其他代理使用');
  } finally {
    actionLoadingId.value = null;
  }
}

function developmentSummary(agent: DistributionAgent) {
  if (agent.level !== 1) {
    return `直属客户 ${agent.directUserCount || 0} 人`;
  }
  return `直属客户 ${agent.directUserCount || 0} 人，下级二级代理 ${agent.secondLevelAgentCount || 0} 人，下级三级代理 ${agent.thirdLevelAgentCount || 0} 人`;
}

function customerOptionLabel(customer: AppCustomer) {
  const nickname = customer.nickname || '未填写昵称';
  const phone = customer.phone || '未绑定手机号';
  const account = customer.account ? ` / ${customer.account}` : '';
  const existingAgentText = customerIsExistingAgent(customer.id)
    ? '｜已是代理'
    : '';
  return `#${customer.id}｜${nickname}｜${phone}${account}${existingAgentText}`;
}

function customerIsExistingAgent(customerId: number) {
  return existingAgentCustomerIds.value.has(customerId);
}

function resetCreateForm() {
  selectedCustomerId.value = undefined;
  customerOptions.value = [];
}

async function searchAppCustomers(keyword = '') {
  const currentRequestId = ++customerSearchRequestId;
  customerSearching.value = true;
  try {
    if (isAgentBackoffice.value) {
      const result = agentAnalytics.value ?? await getAgentDistributionAnalyticsApi(queryAnalyticsScope().params);
      if (currentRequestId !== customerSearchRequestId) return;
      customerOptions.value = result.users
        .filter((item) => item.directAgentId === currentAgent.value?.id && !customerIsExistingAgent(item.id))
        .map((item) => ({
          account: '',
          avatar: '',
          createTime: '',
          id: item.id,
          lastLoginAt: null,
          memberLevel: item.memberLevel,
          nickname: item.nickname || `用户${item.id}`,
          phone: '',
          registerSource: 'agent',
          status: 'active',
          updateTime: '',
        }));
      return;
    }
    const result = await getAppCustomerListApi({
      keyword: keyword || undefined,
      page: 1,
      pageSize: 20,
      status: 'active',
    });
    if (currentRequestId !== customerSearchRequestId) return;
    customerOptions.value = result.items;
  } catch {
    message.error('App 客户查询失败');
  } finally {
    if (currentRequestId === customerSearchRequestId) {
      customerSearching.value = false;
    }
  }
}

async function openCreateModal() {
  resetCreateForm();
  createAgentModalOpen.value = true;
  await searchAppCustomers();
}

async function load() {
  loading.value = true;
  try {
    if (isAgentBackoffice.value) {
      const result = await getAgentDistributionAgentsApi();
      agents.value = result.items;
      currentAgent.value = result.current;
    } else {
      agents.value = (await getDistributionAgentsApi()).items;
      currentAgent.value = null;
    }
  } catch {
    message.error('代理列表加载失败');
  } finally {
    loading.value = false;
  }
}

async function loadAnalytics() {
  if (await fetchAnalytics(queryAnalyticsScope())) {
    await nextTick();
    requestAnimationFrame(renderTrendChart);
  }
}

async function create() {
  if (!selectedCustomerId.value) {
    message.warning('请先选择 App 客户');
    return;
  }
  creating.value = true;
  try {
    if (isAgentBackoffice.value) {
      await createAgentDistributionChildApi({ appUserId: selectedCustomerId.value });
    } else {
      await createDistributionAgentApi({ appUserId: selectedCustomerId.value });
    }
    message.success(isAgentBackoffice.value ? '三级代理已创建' : '代理已创建，代理号由后台自动生成');
    createAgentModalOpen.value = false;
    resetCreateForm();
    await load();
  } catch {
    message.error('创建失败，请检查用户是否存在、是否已经是代理，或自动代理号是否重复');
  } finally {
    creating.value = false;
  }
}

async function toggle(agent: DistributionAgent) {
  if (!canWrite.value) {
    message.warning('当前账号没有代理编辑权限');
    return;
  }
  const status = agent.status === 'active' ? 'paused' : 'active';
  actionLoadingId.value = agent.id;
  try {
    await updateDistributionAgentStatusApi(agent.id, status);
    agent.status = status;
    message.success(status === 'active' ? '代理已恢复' : '代理已暂停');
  } catch {
    message.error('状态更新失败');
  } finally {
    actionLoadingId.value = null;
  }
}

function renderTrendChart() {
  if (!trendChartRef.value) return;
  trendChart ??= echarts.init(trendChartRef.value);
  trendChart.setOption(trendChartOption.value);
}

function handleResize() {
  trendChart?.resize();
}

onMounted(() => {
  load();
  loadAnalytics();
  window.addEventListener('resize', handleResize);
});

onBeforeUnmount(() => {
  disposeAnalytics();
  window.removeEventListener('resize', handleResize);
  trendChart?.dispose();
  trendChart = undefined;
});
</script>

<template>
  <Page title="分销代理管理">
    <div class="distribution-page">
      <Alert
        show-icon
        type="info"
        message="代理管理说明"
:description="isAgentBackoffice ? '这里用于查看你的经营数据。二级代理可以从自己直属邀请用户中开通三级代理，三级代理只查看数据。' : '这里用于把已有 App 用户开通为一级代理。创建后用户会立即获得一级代理身份，可使用后台自动生成的代理号邀请客户；暂停后该代理不能继续作为有效邀请人，但历史数据仍保留。'"
      />

      <Card v-if="isAgentBackoffice" :bordered="false" class="agent-share-card">
        <Space wrap align="center" size="middle">
          <Typography.Text type="secondary">我的代理号</Typography.Text>
          <Tag color="blue" class="agent-code-tag">{{ currentAgent?.agentCode || '加载中...' }}</Tag>
          <Button size="small" type="primary" :disabled="!currentAgent?.agentCode" @click="copyCurrentAgentCode">
            复制代理号
          </Button>
          <Typography.Text type="secondary">
            分享给别人后，对方可通过该代理号绑定到你的代理关系下。
          </Typography.Text>
        </Space>
      </Card>

      <DistributionPosterComposer
        v-if="isAgentBackoffice"
        :agent-code="currentAgent?.agentCode"
      />

      <Card :bordered="false" class="analytics-card">
        <template #title>经营数据分析</template>
        <template #extra>
          <Space wrap>
            <template v-if="isAgentBackoffice">
              <Button :type="datePreset === 'today' ? 'primary' : 'default'" @click="datePreset = 'today'; loadAnalytics()">今日</Button>
              <Button :type="datePreset === 'yesterday' ? 'primary' : 'default'" @click="datePreset = 'yesterday'; loadAnalytics()">昨日</Button>
              <Button :type="datePreset === 'last7' ? 'primary' : 'default'" @click="datePreset = 'last7'; loadAnalytics()">近7天</Button>
              <DatePicker.RangePicker
                v-model:value="customDateRange"
                :allow-clear="false"
                :placeholder="['开始日期', '结束日期']"
                @change="datePreset = 'custom'; loadAnalytics()"
              />
            </template>
            <Button :loading="analyticsLoading" @click="loadAnalytics">刷新数据</Button>
          </Space>
        </template>
        <Typography.Paragraph type="secondary">{{ analyticsScope.description }}</Typography.Paragraph>
        <Row :gutter="[16, 16]">
          <Col v-for="item in businessMetricCards" :key="item.title" :lg="6" :md="12" :xs="24">
            <div class="business-metric" :style="{ '--metric-color': item.color }">
              <span class="metric-dot"></span>
              <Statistic :title="item.title" :value="item.value" />
            </div>
          </Col>
        </Row>
        <Row :gutter="[16, 16]" class="analytics-content">
          <Col :lg="15" :xs="24">
            <Card :bordered="false" class="inner-card" :title="analyticsScope.trendTitle">
              <div class="trend-chart-wrap">
                <div ref="trendChartRef" class="trend-chart"></div>
                <div v-if="analyticsLoading" class="chart-mask">正在更新经营数据...</div>
              </div>
            </Card>
          </Col>
          <Col :lg="9" :xs="24">
            <Card :bordered="false" class="inner-card" title="经营概况">
              <div class="summary-grid">
                <div>
                  <span>代理总数</span>
                  <strong>{{ analyticsSummary.totalAgents }}</strong>
                </div>
                <div>
                  <span>正常代理</span>
                  <strong>{{ analyticsSummary.activeAgents }}</strong>
                </div>
                <div>
                  <span>已暂停</span>
                  <strong>{{ analyticsSummary.pausedAgents }}</strong>
                </div>
                <div>
                  <span>已结算金额</span>
                  <strong>{{ formatYuan(analyticsSummary.settledCommissionAmount) }}</strong>
                </div>
              </div>
            </Card>
          </Col>
        </Row>
      </Card>

      <Card :bordered="false">
        <template #title>{{ isAgentBackoffice ? '我的经营明细' : '代理经营排行' }}</template>
        <Table
          :columns="rankingColumns"
          :data-source="analytics.agentRankings"
          :loading="analyticsLoading"
          :pagination="false"
          :scroll="fixedWideTableScroll"
          row-key="agentId"
          size="small"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'agentCode'">
              <Tag color="blue">{{ rankingOf(record).agentCode }}</Tag>
            </template>
            <template v-else-if="column.dataIndex === 'appUserName'">
              {{ rankingDisplayName(rankingOf(record).appUserId) }}
            </template>
            <template v-else-if="isRankingMoneyColumn(column.dataIndex)">
              {{ rankingMoneyValue(record, column.dataIndex) }}
            </template>
            <template v-else-if="column.dataIndex === 'status'">
              <Tag :color="statusColor(rankingOf(record).status)">
                {{ statusText(rankingOf(record).status) }}
              </Tag>
            </template>
          </template>
        </Table>
      </Card>

      <Row v-if="isAgentBackoffice" :gutter="[16, 16]">
        <Col :lg="12" :xs="24">
          <Card :bordered="false">
            <template #title>下级用户消费汇总</template>
            <template #extra>
              <Button size="small" @click="exportDistributionExcel('users')">导出用户消费</Button>
            </template>
            <Table
              :columns="userConsumptionColumns"
              :data-source="userConsumptionRows"
              :loading="analyticsLoading"
              :pagination="false"
              :scroll="fixedTableScroll"
              row-key="id"
              size="small"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.dataIndex === 'orderAmount'">
                  {{ formatYuan(record.orderAmount) }}
                </template>
                <template v-else-if="column.dataIndex === 'boundAt'">
                  {{ formatDateTime(record.boundAt) }}
                </template>
              </template>
            </Table>
          </Card>
        </Col>
        <Col :lg="12" :xs="24">
          <Card :bordered="false">
            <template #title>下级订单明细</template>
            <template #extra>
              <Button size="small" @click="exportDistributionExcel('orders')">导出订单明细</Button>
            </template>
            <Table
              :columns="orderColumns"
              :data-source="orderRows"
              :loading="analyticsLoading"
              :pagination="false"
              :scroll="fixedTableScroll"
              row-key="id"
              size="small"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.dataIndex === 'amount'">
                  {{ formatYuan(record.amount) }}
                </template>
                <template v-else-if="column.dataIndex === 'paidAt'">
                  {{ formatDateTime(record.paidAt) }}
                </template>
                <template v-else-if="column.dataIndex === 'status'">
                  <Tag>{{ record.status }}</Tag>
                </template>
              </template>
            </Table>
          </Card>
        </Col>
      </Row>

      <Row :gutter="16">
        <Col :lg="8" :xs="24">
          <Card>
            <Statistic title="代理总数" :value="agents.length" />
          </Card>
        </Col>
        <Col :lg="8" :xs="24">
          <Card>
            <Statistic title="正常代理" :value="activeAgentCount" />
          </Card>
        </Col>
        <Col :lg="8" :xs="24">
          <Card>
            <Statistic title="已暂停" :value="pausedAgentCount" />
          </Card>
        </Col>
      </Row>

      <Card>
        <template #title>代理列表</template>
        <template #extra>
          <Button type="primary" :disabled="!canCreateAgent" @click="openCreateModal">
            {{ isAgentBackoffice ? '新增三级代理' : '新增代理' }}
          </Button>
        </template>

        <Table
          :columns="columns"
          :data-source="agents"
          :loading="loading"
          :pagination="false"
          :scroll="fixedWideTableScroll"
          row-key="id"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'appUserAccount'">
              <Space direction="vertical" size="small">
                <Typography.Text>
                  {{ agentOf(record).appUserAccount || agentOf(record).appUserPhone || agentOf(record).appUserNickname || `#${agentOf(record).appUserId}` }}
                </Typography.Text>
                <Typography.Text
                  v-if="agentOf(record).appUserNickname && agentOf(record).appUserNickname !== (agentOf(record).appUserAccount || agentOf(record).appUserPhone)"
                  type="secondary"
                >
                  {{ agentOf(record).appUserNickname }}
                </Typography.Text>
              </Space>
            </template>
            <template v-else-if="column.dataIndex === 'appUserName'">
              {{ agentDisplayName(agentOf(record)) }}
            </template>
            <template v-else-if="column.dataIndex === 'agentCode'">
              <Space>
                <Tag color="blue">{{ agentOf(record).agentCode }}</Tag>
                <Typography.Text type="secondary">后台自动生成</Typography.Text>
              </Space>
            </template>
            <template v-else-if="column.dataIndex === 'level'">
              <Tag color="purple">{{ levelText(agentOf(record).level) }}</Tag>
            </template>
            <template v-else-if="column.dataIndex === 'development'">
              <Space direction="vertical" size="small">
                <Space wrap>
                  <Tag color="cyan">直属客户 {{ agentOf(record).directUserCount || 0 }}</Tag>
                  <Tag v-if="agentOf(record).level === 1" color="geekblue">
                    下级二级代理 {{ agentOf(record).secondLevelAgentCount || 0 }}
                  </Tag>
                  <Tag v-if="agentOf(record).level === 1" color="volcano">
                    下级三级代理 {{ agentOf(record).thirdLevelAgentCount || 0 }}
                  </Tag>
                </Space>
                <Typography.Text type="secondary">
                  {{ developmentSummary(agentOf(record)) }}
                </Typography.Text>
              </Space>
            </template>
            <template v-else-if="column.dataIndex === 'status'">
              <Tag :color="statusColor(agentOf(record).status)">
                {{ statusText(agentOf(record).status) }}
              </Tag>
            </template>
            <template v-else-if="column.key === 'action'">
              <Space>
                <Button
                  v-if="agentOf(record).level === 1"
                  type="link"
                  @click="openChildAgents(agentOf(record))"
                >
                  查看下级
                  </Button>
                <Button
                  v-if="canWrite"
                  type="link"
                  :loading="actionLoadingId === agentOf(record).id"
                  @click="openEditAgentCode(agentOf(record))"
                >
                  编辑代理号
                </Button>
                <Popconfirm
                  :title="agentOf(record).status === 'active' ? '确认暂停该代理？' : '确认恢复该代理？'"
                  ok-text="确认"
                  cancel-text="取消"
                  @confirm="toggle(agentOf(record))"
                >
                  <Button
                    type="link"
                    :disabled="!canWrite"
                    :loading="actionLoadingId === agentOf(record).id"
                  >
                    {{ agentOf(record).status === 'active' ? '暂停' : '恢复' }}
                  </Button>
                </Popconfirm>
              </Space>
            </template>
          </template>
        </Table>
      </Card>



      <Modal
        :open="!!selectedParentAgent"
        title="下级代理明细"
        :footer="null"
        width="820px"
        @cancel="selectedParentAgent = null"
      >
        <Space v-if="selectedParentAgent" direction="vertical" size="middle" class="full-width">
          <Alert
            show-icon
            type="info"
            message="一级代理下级发展情况"
            :description="`一级代理 ${selectedParentAgent.agentCode} 当前有下级二级代理 ${selectedParentAgent.secondLevelAgentCount || 0} 人，下级三级代理 ${selectedParentAgent.thirdLevelAgentCount || 0} 人，直属客户 ${selectedParentAgent.directUserCount || 0} 人。`"
          />
          <Table
            :columns="childAgentColumns"
            :data-source="childAgentsForSelectedParent"
            :pagination="false"
            :scroll="{ x: 720, y: 280 }"
            row-key="id"
            size="small"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'appUserAccount'">
                {{ agentOf(record).appUserAccount || agentOf(record).appUserPhone || agentOf(record).appUserNickname || `#${agentOf(record).appUserId}` }}
              </template>
              <template v-else-if="column.dataIndex === 'agentCode'">
                <Tag color="blue">{{ agentOf(record).agentCode }}</Tag>
              </template>
              <template v-else-if="column.dataIndex === 'appUserName'">
                {{ agentDisplayName(agentOf(record)) }}
              </template>
              <template v-else-if="column.dataIndex === 'level'">
                <Tag color="purple">{{ levelText(agentOf(record).level) }}</Tag>
              </template>
              <template v-else-if="column.dataIndex === 'status'">
                <Tag :color="statusColor(agentOf(record).status)">
                  {{ statusText(agentOf(record).status) }}
                </Tag>
              </template>
            </template>
          </Table>
        </Space>
      </Modal>

      <Modal
        v-model:open="createAgentModalOpen"
        :title="isAgentBackoffice ? '新增三级代理' : '新增代理'"
        ok-text="确认开通"
        cancel-text="取消"
        :confirm-loading="creating"
        @cancel="resetCreateForm"
        @ok="create"
      >
        <Space direction="vertical" size="middle" class="full-width">
          <Alert
            show-icon
            type="warning"
            message="开通前请确认"
            description="请先搜索并选择当前 App 的客户。创建后用户会立即获得一级代理身份；代理号由后台自动生成，前端不需要填写。"
          />
          <Form layout="vertical">
            <Form.Item label="搜索并选择 App 客户" required>
              <Select
                v-model:value="selectedCustomerId"
                show-search
                allow-clear
                :filter-option="false"
                :loading="customerSearching"
                placeholder="输入手机号、昵称或账号搜索"
                class="full-width"
                @focus="searchAppCustomers()"
                @search="searchAppCustomers"
              >
                <Select.Option
                  v-for="customer in customerOptions"
                  :key="customer.id"
                  :value="customer.id"
                  :disabled="customerIsExistingAgent(customer.id)"
                >
                  {{ customerOptionLabel(customer) }}
                </Select.Option>
              </Select>
              <Typography.Text type="secondary">
                只能从当前 App 客户中选择，避免手填 ID 出错。
              </Typography.Text>
            </Form.Item>

            <Form.Item label="已选客户">
              <template v-if="selectedCustomer">
                <Space wrap>
                  <Tag color="blue">#{{ selectedCustomer.id }}</Tag>
                  <span>{{ selectedCustomer.nickname || '未填写昵称' }}</span>
                  <Typography.Text type="secondary">
                    {{ selectedCustomer.phone || '未绑定手机号' }}
                  </Typography.Text>
                </Space>
              </template>
              <Typography.Text v-else type="secondary">
                请先在上方搜索框里选择一个 App 客户。
              </Typography.Text>
            </Form.Item>

            <Form.Item label="代理号由后台自动生成">
              <Space>
                <Tag color="green">创建后由后台生成</Tag>
                <Typography.Text type="secondary">
                  开通成功后以列表返回的代理号为准。
                </Typography.Text>
              </Space>
            </Form.Item>
          </Form>
        </Space>
      </Modal>

      <Modal
        v-model:open="editAgentModalOpen"
        title="修改代理号"
        ok-text="保存"
        cancel-text="取消"
        :confirm-loading="actionLoadingId === editingAgent?.id"
        @ok="saveAgentCode"
      >
        <Alert
          class="mb-4"
          show-icon
          type="info"
          message="仅管理员可以修改代理号"
          description="客户端和代理本人只能查看，修改后立即生效；代理号必须唯一。"
        />
        <Form layout="vertical">
          <Form.Item label="代理号" required>
            <Input
              v-model:value="editingAgentCode"
              :maxlength="6"
              placeholder="请输入 6 位代理号"
              @input="editingAgentCode = editingAgentCode.toUpperCase()"
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  </Page>
</template>

<style scoped>
.distribution-page {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 16px;
  min-width: 0;
}
.distribution-page > * {
  min-width: 0;
}
.full-width {
  width: 100%;
}
.agent-share-card {
  border: 1px solid hsl(var(--border));
}
.agent-code-tag {
  padding: 4px 12px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono',
    monospace;
  font-size: 16px;
}
.analytics-card {
  overflow: hidden;
  background:
    linear-gradient(135deg, rgb(37 99 235 / 8%), rgb(249 115 22 / 8%)),
    hsl(var(--card));
}
.business-metric {
  position: relative;
  min-height: 88px;
  padding: 18px 18px 16px;
  overflow: hidden;
  background: hsl(var(--card) / 88%);
  border: 1px solid hsl(var(--border));
  border-radius: 14px;
  box-shadow: 0 10px 28px rgb(15 23 42 / 6%);
}
.business-metric::after {
  position: absolute;
  right: -22px;
  bottom: -24px;
  width: 76px;
  height: 76px;
  content: '';
  background: var(--metric-color);
  border-radius: 999px;
  opacity: 0.12;
}
.metric-dot {
  display: inline-flex;
  width: 10px;
  height: 10px;
  margin-bottom: 10px;
  background: var(--metric-color);
  border-radius: 999px;
  box-shadow: 0 0 0 6px color-mix(in srgb, var(--metric-color) 16%, transparent);
}
.business-metric :deep(.ant-statistic-title) {
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}
.business-metric :deep(.ant-statistic-content) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono',
    monospace;
  font-size: 24px;
  font-weight: 750;
  color: hsl(var(--foreground));
}
.analytics-content {
  margin-top: 16px;
}
.inner-card {
  height: 100%;
  background: hsl(var(--card) / 82%);
  border: 1px solid hsl(var(--border));
}
.trend-chart-wrap {
  position: relative;
  min-height: 320px;
}
.trend-chart {
  width: 100%;
  height: 320px;
}
.chart-mask {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: hsl(var(--muted-foreground));
  pointer-events: none;
  background: hsl(var(--card) / 58%);
}
.summary-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}
.summary-grid > div {
  min-height: 82px;
  padding: 14px;
  background: hsl(var(--accent) / 36%);
  border: 1px solid hsl(var(--border));
  border-radius: 12px;
}
.summary-grid span {
  display: block;
  margin-bottom: 8px;
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}
.summary-grid strong {
  font-size: 20px;
  color: hsl(var(--foreground));
}
</style>
