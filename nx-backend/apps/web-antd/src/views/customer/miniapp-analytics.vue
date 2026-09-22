<script setup lang="ts">
import type { MiniappAnalyticsResult } from '#/api';
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import { Page } from '@vben/common-ui';
import { IconifyIcon } from '@vben/icons';
import { Button, Card, DatePicker, Empty, Table, Tag, message } from 'ant-design-vue';
import dayjs, { type Dayjs } from 'dayjs';
import { LineChart, PieChart, BarChart } from 'echarts/charts';
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components';
import * as echarts from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';
import { getMiniappAnalyticsApi } from '#/api';

echarts.use([BarChart, CanvasRenderer, GridComponent, LegendComponent, LineChart, PieChart, TooltipComponent]);
const selectedDate = ref<Dayjs>(dayjs());
const loading = ref(false);
const data = ref<MiniappAnalyticsResult>({ date: dayjs().format('YYYY-MM-DD'), hours: Array.from({ length: 24 }, (_, hour) => ({ hour, paidAmount: 0, paidOrders: 0 })), products: [], recentOrders: [], statuses: [], summary: { averageOrder: 0, closedOrders: 0, paidAmount: 0, paidOrders: 0, pendingOrders: 0, successRate: 0, totalOrders: 0 } });
const trendRef = ref<HTMLDivElement>(); const statusRef = ref<HTMLDivElement>(); const productRef = ref<HTMLDivElement>();
let trendChart: echarts.ECharts | undefined; let statusChart: echarts.ECharts | undefined; let productChart: echarts.ECharts | undefined;
const summaryCards = computed(() => [
  { label: '支付金额', value: `¥${(data.value.summary.paidAmount / 100).toFixed(2)}`, icon: 'lucide:wallet-cards', color: '#0ea5e9', note: '已支付订单累计' },
  { label: '支付笔数', value: data.value.summary.paidOrders, icon: 'lucide:badge-check', color: '#22c55e', note: `共 ${data.value.summary.totalOrders} 笔订单` },
  { label: '支付成功率', value: `${data.value.summary.successRate.toFixed(1)}%`, icon: 'lucide:trending-up', color: '#f59e0b', note: '成功订单 / 全部订单' },
  { label: '平均客单价', value: `¥${(data.value.summary.averageOrder / 100).toFixed(2)}`, icon: 'lucide:receipt', color: '#a78bfa', note: '按支付订单计算' },
]);
const statusLabel: Record<string, string> = { paid: '已支付', pending: '待支付', closed: '已关闭', refunded: '已退款' };
const productLabel: Record<string, string> = { wechat_pay_test: '微信支付测试', report: '深度报告', member: '会员服务', classroom_series: '课堂系列', classroom_content: '课堂内容' };
const orderColumns = [
  { title: '商户订单号', dataIndex: 'outTradeNo', key: 'outTradeNo', ellipsis: true }, { title: '商品', dataIndex: 'title', key: 'title' },
  { title: '金额', dataIndex: 'amount', key: 'amount', customRender: ({ text }: { text: number }) => `¥${(text / 100).toFixed(2)}` }, { title: '支付时间', dataIndex: 'paidAt', key: 'paidAt' },
];
async function loadData() {
  loading.value = true;
  try { data.value = await getMiniappAnalyticsApi({ date: selectedDate.value.format('YYYY-MM-DD') }); await nextTick(); requestAnimationFrame(renderCharts); }
  catch (error) { message.error(error instanceof Error ? error.message : '数据分析加载失败'); }
  finally { loading.value = false; }
}
function renderCharts() {
  if (trendRef.value) { trendChart ??= echarts.init(trendRef.value); trendChart.setOption({ color: ['#38bdf8', '#a78bfa'], grid: { bottom: 28, left: 58, right: 44, top: 42 }, legend: { right: 8, top: 0, textStyle: { color: '#94a3b8' }, data: ['支付金额', '支付笔数'] }, tooltip: { trigger: 'axis' }, xAxis: { axisLabel: { color: '#64748b' }, axisLine: { lineStyle: { color: '#273244' } }, boundaryGap: false, data: data.value.hours.map((point) => `${String(point.hour).padStart(2, '0')}:00`), type: 'category' }, yAxis: [{ axisLabel: { color: '#64748b', formatter: (value: number) => `¥${(value / 100).toFixed(value < 100 ? 2 : 0)}` }, splitLine: { lineStyle: { color: '#1f2937' } }, type: 'value' }, { axisLabel: { color: '#64748b' }, splitLine: { show: false }, type: 'value' }], series: [{ name: '支付金额', smooth: true, symbol: 'none', type: 'line', areaStyle: { color: 'rgba(56,189,248,.12)' }, data: data.value.hours.map((point) => point.paidAmount), yAxisIndex: 0 }, { name: '支付笔数', smooth: true, symbol: 'none', type: 'line', data: data.value.hours.map((point) => point.paidOrders), yAxisIndex: 1 }] }); }
  if (statusRef.value) { statusChart ??= echarts.init(statusRef.value); statusChart.setOption({ color: ['#22c55e', '#38bdf8', '#64748b', '#f59e0b'], tooltip: { trigger: 'item' }, legend: { bottom: 0, textStyle: { color: '#94a3b8' } }, series: [{ data: data.value.statuses.map((item) => ({ name: statusLabel[item.status] ?? item.status, value: item.count })), radius: ['52%', '76%'], center: ['50%', '44%'], label: { color: '#cbd5e1' }, type: 'pie' }] }); }
  if (productRef.value) { productChart ??= echarts.init(productRef.value); productChart.setOption({ color: '#818cf8', grid: { bottom: 28, left: 72, right: 20, top: 12 }, tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } }, xAxis: { axisLabel: { color: '#64748b', formatter: (value: number) => `¥${(value / 100).toFixed(0)}` }, splitLine: { lineStyle: { color: '#1f2937' } }, type: 'value' }, yAxis: { axisLabel: { color: '#cbd5e1' }, data: data.value.products.map((item) => productLabel[item.product] ?? item.title), type: 'category' }, series: [{ barMaxWidth: 18, data: data.value.products.map((item) => item.paidAmount), type: 'bar', itemStyle: { borderRadius: [0, 5, 5, 0] } }] }); }
}
function resizeCharts() { trendChart?.resize(); statusChart?.resize(); productChart?.resize(); }
onMounted(() => { void loadData(); window.addEventListener('resize', resizeCharts); });
onBeforeUnmount(() => { window.removeEventListener('resize', resizeCharts); trendChart?.dispose(); statusChart?.dispose(); productChart?.dispose(); });
</script>

<template>
  <Page title="小程序数据分析" description="按北京时间查看小程序支付趋势、订单结构与商品表现。">
    <div class="analytics-page">
      <div class="toolbar"><div><div class="eyebrow">PAYMENT INSIGHTS</div><h1>支付数据分析</h1><p>聚焦当天真实支付表现，快速识别高峰时段和订单转化。</p></div><div class="toolbar-actions"><DatePicker v-model:value="selectedDate" :allow-clear="false" format="YYYY-MM-DD" placeholder="请选择统计日期" /><Button :loading="loading" @click="loadData"><IconifyIcon icon="lucide:refresh-cw" />刷新数据</Button></div></div>
      <div class="metric-grid"><div v-for="card in summaryCards" :key="card.label" class="metric-card"><div class="metric-icon" :style="{ color: card.color, background: `${card.color}18` }"><IconifyIcon :icon="card.icon" /></div><div><div class="metric-label">{{ card.label }}</div><div class="metric-value">{{ card.value }}</div><div class="metric-note">{{ card.note }}</div></div></div></div>
      <Card class="chart-card" :loading="loading" :bordered="false"><template #title><div class="card-heading"><span>小时支付趋势</span><small>{{ data.date }} · 北京时间</small></div></template><div ref="trendRef" class="trend-chart" /></Card>
      <div class="lower-grid"><Card class="chart-card" :loading="loading" :bordered="false"><template #title>订单状态分布</template><div v-if="!data.statuses.length && !loading" class="empty-chart"><Empty description="当天暂无订单" /></div><div v-else ref="statusRef" class="small-chart" /></Card><Card class="chart-card" :loading="loading" :bordered="false"><template #title>商品支付排行</template><div v-if="!data.products.length && !loading" class="empty-chart"><Empty description="当天暂无支付" /></div><div v-else ref="productRef" class="small-chart" /></Card></div>
      <Card class="table-card" :loading="loading" :bordered="false"><template #title><div class="card-heading"><span>最近支付订单</span><Tag color="blue">{{ data.recentOrders.length }} 笔</Tag></div></template><Table :columns="orderColumns" :data-source="data.recentOrders" :pagination="false" row-key="outTradeNo" size="middle"><template #bodyCell="{ column, record }"><template v-if="column.key === 'outTradeNo'"><span class="mono">{{ record.outTradeNo }}</span></template></template></Table></Card>
    </div>
  </Page>
</template>

<style scoped>
.analytics-page { padding: 4px 8px 28px; color: #e2e8f0; } .toolbar { display:flex; align-items:flex-end; justify-content:space-between; gap:24px; margin-bottom:22px; } .eyebrow { color:#38bdf8; font-size:11px; font-weight:700; letter-spacing:.12em; } h1 { margin:6px 0 4px; font-size:26px; } .toolbar p { color:#64748b; margin:0; } .toolbar-actions { display:flex; align-items:center; gap:10px; } .metric-grid { display:grid; grid-template-columns:repeat(4,minmax(0,1fr)); gap:14px; margin-bottom:16px; } .metric-card { display:flex; gap:13px; align-items:center; border:1px solid rgba(100,116,139,.18); border-radius:10px; padding:18px; background:linear-gradient(135deg,rgba(15,23,42,.76),rgba(15,23,42,.38)); } .metric-icon { display:grid; place-items:center; width:42px; height:42px; border-radius:10px; font-size:21px; } .metric-label,.metric-note { color:#94a3b8; font-size:12px; } .metric-value { color:#f8fafc; font-size:24px; font-weight:700; line-height:1.35; } .chart-card,.table-card { background:rgba(15,23,42,.56); border:1px solid rgba(100,116,139,.15); border-radius:10px; margin-bottom:16px; } .card-heading { display:flex; align-items:center; gap:10px; } .card-heading small { color:#64748b; font-size:12px; font-weight:400; } .trend-chart { height:340px; } .lower-grid { display:grid; grid-template-columns:minmax(0,.9fr) minmax(0,1.1fr); gap:16px; } .small-chart,.empty-chart { height:280px; } .empty-chart { display:grid; place-items:center; } .mono { font-family:ui-monospace,SFMono-Regular,Menlo,monospace; font-size:12px; } @media(max-width:900px){.metric-grid{grid-template-columns:repeat(2,minmax(0,1fr));}.toolbar{align-items:flex-start;flex-direction:column}.toolbar-actions{width:100%}.toolbar-actions .ant-picker{flex:1}} @media(max-width:640px){.analytics-page{padding:0 0 20px}.metric-grid,.lower-grid{grid-template-columns:1fr}.trend-chart{height:280px}.small-chart,.empty-chart{height:240px}h1{font-size:22px}}
</style>
