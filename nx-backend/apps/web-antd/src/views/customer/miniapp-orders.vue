<script setup lang="ts">
import type { MiniappOrder } from '#/api';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { IconifyIcon } from '@vben/icons';

import {
  Avatar,
  Button,
  Card,
  Descriptions,
  Drawer,
  Empty,
  Input,
  message,
  Select,
  Table,
  Tag,
  Tooltip,
} from 'ant-design-vue';

import {
  getMiniappOrderListApi,
  reconcileMiniappOrdersApi,
} from '#/api';

const orders = ref<MiniappOrder[]>([]);
const total = ref(0);
const loading = ref(false);
const reconciling = ref(false);
const detailOpen = ref(false);
const current = ref<MiniappOrder>();
const summary = reactive({ total: 0, paid: 0, pending: 0, paidAmount: 0 });
const query = reactive({ keyword: '', product: '', status: '', page: 1, pageSize: 20 });

const columns = [
  { dataIndex: 'outTradeNo', title: '商户订单号', width: 220 },
  { dataIndex: 'nickname', title: '用户', width: 190 },
  { dataIndex: 'title', title: '商品', width: 160 },
  { dataIndex: 'amount', title: '金额', width: 100 },
  { dataIndex: 'status', title: '状态', width: 110 },
  { dataIndex: 'transactionId', title: '微信交易单号', width: 220 },
  { dataIndex: 'createTime', title: '创建时间', width: 180 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 72 },
];

const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '待支付', value: 'pending' },
  { label: '已支付', value: 'paid' },
  { label: '已关闭', value: 'closed' },
  { label: '已退款', value: 'refunded' },
];

const productOptions = [
  { label: '全部商品', value: '' },
  { label: '微信支付测试', value: 'wechat_pay_test' },
  { label: '深度报告', value: 'report' },
  { label: '课程系列', value: 'classroom_series' },
  { label: '单节课件', value: 'classroom_content' },
  { label: '会员', value: 'member' },
];

const summaryItems = computed(() => [
  { icon: 'lucide:receipt-text', label: '全部订单', tone: 'blue', value: summary.total },
  { icon: 'lucide:circle-check-big', label: '已支付', tone: 'green', value: summary.paid },
  { icon: 'lucide:clock-3', label: '待支付', tone: 'amber', value: summary.pending },
  { icon: 'lucide:wallet-cards', label: '实收金额', tone: 'cyan', value: amountYuan(summary.paidAmount) },
]);

async function load() {
  loading.value = true;
  try {
    const result = await getMiniappOrderListApi({
      ...query,
      keyword: query.keyword || undefined,
      product: query.product || undefined,
      status: query.status || undefined,
    });
    orders.value = result.items;
    total.value = result.total;
    Object.assign(summary, result.summary);
  } catch (error) {
    orders.value = [];
    message.error(error instanceof Error ? error.message : '订单加载失败');
  } finally {
    loading.value = false;
  }
}

async function reconcile(silent = false) {
  reconciling.value = true;
  try {
    const result = await reconcileMiniappOrdersApi();
    if (!silent) {
      if (result.paid > 0 || result.closed > 0) {
        message.success(`同步完成：${result.paid} 笔已支付，${result.closed} 笔已关闭`);
      } else if (result.failed > 0) {
        message.warning(`已查询 ${result.checked} 笔，${result.failed} 笔查询失败`);
      } else {
        message.info('支付状态已是最新');
      }
    }
  } catch (error) {
    if (!silent) message.error(error instanceof Error ? error.message : '支付状态同步失败');
  } finally {
    reconciling.value = false;
    await load();
  }
}

function search() {
  query.page = 1;
  void load();
}

function resetFilters() {
  Object.assign(query, { keyword: '', product: '', status: '', page: 1 });
  void load();
}

function pageChange(pagination: { current?: number; pageSize?: number }) {
  query.page = pagination.current ?? 1;
  query.pageSize = pagination.pageSize ?? 20;
  void load();
}

function openDetail(order: MiniappOrder | Record<string, any>) {
  current.value = order as MiniappOrder;
  detailOpen.value = true;
}

function amountYuan(amount = 0) {
  return `¥${(Number(amount) / 100).toFixed(2)}`;
}

function productLabel(product?: string) {
  const labels: Record<string, string> = {
    classroom_content: '单节课件',
    classroom_series: '课程系列',
    member: '会员',
    report: '深度报告',
    wechat_pay_test: '微信支付测试',
  };
  return labels[product || ''] || product || '-';
}

function statusLabel(status?: string) {
  const labels: Record<string, string> = {
    closed: '已关闭',
    paid: '已支付',
    pending: '待支付',
    refunded: '已退款',
  };
  return labels[status || ''] || status || '-';
}

function statusColor(status?: string) {
  if (status === 'paid') return 'success';
  if (status === 'pending') return 'processing';
  if (status === 'refunded') return 'warning';
  return 'default';
}

function userInitial(order: MiniappOrder | Record<string, any>) {
  return String(order.nickname || order.phone || '微').slice(0, 1);
}

onMounted(() => {
  void reconcile(true);
});
</script>

<template>
  <Page description="统一查看微信支付测试、报告和课堂订单，并与微信支付主动核对交易状态。" title="小程序订单">
    <div class="order-workspace">
      <div class="summary-grid">
        <div v-for="item in summaryItems" :key="item.label" class="summary-item">
          <span :class="['summary-icon', `summary-icon--${item.tone}`]">
            <IconifyIcon :icon="item.icon" />
          </span>
          <div class="summary-copy">
            <span class="summary-label">{{ item.label }}</span>
            <strong class="summary-value">{{ item.value }}</strong>
          </div>
        </div>
      </div>

      <Card :bordered="false" class="orders-panel">
        <div class="toolbar">
          <Input v-model:value="query.keyword" allow-clear class="keyword-input" placeholder="搜索订单号、昵称或手机号" @press-enter="search">
            <template #prefix><IconifyIcon icon="lucide:search" /></template>
          </Input>
          <Select v-model:value="query.product" class="filter-select" :options="productOptions" placeholder="请选择商品" />
          <Select v-model:value="query.status" class="status-select" :options="statusOptions" placeholder="请选择订单状态" />
          <Button type="primary" @click="search"><IconifyIcon icon="lucide:list-filter" />查询</Button>
          <Button @click="resetFilters">重置</Button>
          <div class="toolbar-spacer"></div>
          <Tooltip title="向微信支付查询待支付订单的真实交易状态">
            <Button :loading="reconciling" @click="reconcile(false)"><IconifyIcon icon="lucide:refresh-cw" />同步支付状态</Button>
          </Tooltip>
        </div>

        <Table row-key="id" :columns="columns" :data-source="orders" :loading="loading" :pagination="{ current: query.page, pageSize: query.pageSize, total, showSizeChanger: true, showTotal: (value: number) => `共 ${value} 笔` }" :scroll="{ x: 1260 }" size="middle" @change="pageChange">
          <template #emptyText><Empty description="暂无符合条件的小程序订单" /></template>
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'outTradeNo'">
              <Tooltip :title="record.outTradeNo"><span class="order-number">{{ record.outTradeNo }}</span></Tooltip>
            </template>
            <template v-else-if="column.dataIndex === 'nickname'">
              <div class="user-cell">
                <Avatar :size="32">{{ userInitial(record) }}</Avatar>
                <div class="user-copy">
                  <span class="user-name">{{ record.nickname || '微信用户' }}</span>
                  <span class="user-phone">{{ record.phone || `用户 #${record.wxUserId}` }}</span>
                </div>
              </div>
            </template>
            <template v-else-if="column.dataIndex === 'title'">
              <span class="product-name">{{ record.title || productLabel(record.product) }}</span>
              <span class="product-code">{{ productLabel(record.product) }}</span>
            </template>
            <template v-else-if="column.dataIndex === 'amount'"><span class="amount-text">{{ amountYuan(record.amount) }}</span></template>
            <template v-else-if="column.dataIndex === 'status'"><Tag :color="statusColor(record.status)">{{ statusLabel(record.status) }}</Tag></template>
            <template v-else-if="column.dataIndex === 'transactionId'">
              <Tooltip v-if="record.transactionId" :title="record.transactionId"><span class="transaction-id">{{ record.transactionId }}</span></Tooltip>
              <span v-else class="muted-text">-</span>
            </template>
            <template v-else-if="column.key === 'action'">
              <Tooltip title="查看订单详情"><Button aria-label="查看订单详情" size="small" type="text" @click="openDetail(record)"><IconifyIcon icon="lucide:eye" /></Button></Tooltip>
            </template>
          </template>
        </Table>
      </Card>
    </div>

    <Drawer v-model:open="detailOpen" title="小程序订单详情" width="min(620px, calc(100vw - 32px))">
      <Descriptions v-if="current" bordered :column="1" size="small">
        <Descriptions.Item label="商户订单号">{{ current.outTradeNo }}</Descriptions.Item>
        <Descriptions.Item label="商品">{{ current.title || productLabel(current.product) }}</Descriptions.Item>
        <Descriptions.Item label="金额">{{ amountYuan(current.amount) }}</Descriptions.Item>
        <Descriptions.Item label="用户">{{ current.nickname || '微信用户' }} / {{ current.phone || `#${current.wxUserId}` }}</Descriptions.Item>
        <Descriptions.Item label="状态"><Tag :color="statusColor(current.status)">{{ statusLabel(current.status) }}</Tag></Descriptions.Item>
        <Descriptions.Item label="微信交易单号">{{ current.transactionId || '-' }}</Descriptions.Item>
        <Descriptions.Item label="创建时间">{{ current.createTime }}</Descriptions.Item>
        <Descriptions.Item label="更新时间">{{ current.updateTime }}</Descriptions.Item>
        <Descriptions.Item label="支付时间">{{ current.paidAt || '-' }}</Descriptions.Item>
      </Descriptions>
    </Drawer>
  </Page>
</template>

<style scoped>
.order-workspace { display: flex; flex-direction: column; gap: 16px; }
.summary-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); overflow: hidden; background: var(--ant-color-bg-container); border: 1px solid var(--ant-color-border-secondary); border-radius: 8px; }
.summary-item { display: flex; min-height: 92px; padding: 18px 20px; align-items: center; border-right: 1px solid var(--ant-color-border-secondary); }
.summary-item:last-child { border-right: 0; }
.summary-icon { display: inline-flex; width: 38px; height: 38px; margin-right: 12px; align-items: center; justify-content: center; border-radius: 8px; font-size: 19px; }
.summary-icon--blue { color: #2563eb; background: #eff6ff; }
.summary-icon--green { color: #059669; background: #ecfdf5; }
.summary-icon--amber { color: #d97706; background: #fffbeb; }
.summary-icon--cyan { color: #0891b2; background: #ecfeff; }
.summary-copy { display: flex; min-width: 0; flex-direction: column; }
.summary-label { color: var(--ant-color-text-secondary); font-size: 13px; }
.summary-value { margin-top: 2px; color: var(--ant-color-text); font-size: 24px; line-height: 1.2; }
.orders-panel { min-height: 420px; }
.toolbar { display: flex; flex-wrap: wrap; gap: 10px; margin-bottom: 16px; align-items: center; }
.toolbar-spacer { flex: 1 1 auto; }
.keyword-input { width: min(320px, 100%); }
.filter-select { width: 160px; }
.status-select { width: 130px; }
.order-number, .transaction-id { display: block; overflow: hidden; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.user-cell { display: flex; min-width: 0; align-items: center; gap: 10px; }
.user-copy { display: flex; min-width: 0; flex-direction: column; }
.user-name { overflow: hidden; color: var(--ant-color-text); font-weight: 500; text-overflow: ellipsis; white-space: nowrap; }
.user-phone, .product-code, .muted-text { color: var(--ant-color-text-tertiary); font-size: 12px; }
.product-name { display: block; color: var(--ant-color-text); font-weight: 500; }
.product-code { display: block; margin-top: 2px; }
.amount-text { font-weight: 600; font-variant-numeric: tabular-nums; }
@media (prefers-color-scheme: dark) {
  .summary-icon--blue { background: rgb(37 99 235 / 16%); }
  .summary-icon--green { background: rgb(5 150 105 / 16%); }
  .summary-icon--amber { background: rgb(217 119 6 / 16%); }
  .summary-icon--cyan { background: rgb(8 145 178 / 16%); }
}
@media (max-width: 900px) {
  .summary-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .summary-item:nth-child(2) { border-right: 0; }
  .summary-item:nth-child(-n + 2) { border-bottom: 1px solid var(--ant-color-border-secondary); }
  .toolbar-spacer { display: none; }
}
@media (max-width: 560px) {
  .summary-item { min-height: 78px; padding: 14px; }
  .summary-value { font-size: 20px; }
  .keyword-input, .filter-select, .status-select { width: 100%; }
  .toolbar :deep(.ant-btn) { flex: 1 1 auto; }
}
</style>
