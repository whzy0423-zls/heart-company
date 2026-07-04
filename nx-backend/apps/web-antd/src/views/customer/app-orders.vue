<script setup lang="ts">
import type { AppOrder } from '#/api';

import { onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Button,
  Card,
  message,
  Modal,
  Descriptions,
  Drawer,
  Input,
  Select,
  Space,
  Table,
  Tag,
} from 'ant-design-vue';

import { getAppOrderListApi, grantAppOrderApi } from '#/api';

const loading = ref(false);
const orders = ref<AppOrder[]>([]);
const total = ref(0);
const detailOpen = ref(false);
const current = ref<AppOrder>();

const query = reactive({
  keyword: '',
  page: 1,
  pageSize: 20,
  productId: '',
  status: '',
});

const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '待配置支付', value: 'not_configured' },
  { label: '待支付', value: 'pending' },
  { label: '已支付', value: 'paid' },
  { label: '已关闭', value: 'closed' },
];

const productOptions = [
  { label: '全部商品', value: '' },
  { label: '月卡会员', value: 'vip_month' },
  { label: '季卡会员', value: 'vip_quarter' },
  { label: '年卡会员', value: 'vip_year' },
  { label: '深度报告', value: 'deep_report' },
];

const columns = [
  { dataIndex: 'outTradeNo', fixed: 'left' as const, title: '订单号', width: 230 },
  { dataIndex: 'phone', title: '手机号', width: 150 },
  { dataIndex: 'title', title: '商品', width: 140 },
  { dataIndex: 'amount', title: '金额', width: 100 },
  { dataIndex: 'status', title: '状态', width: 120 },
  { dataIndex: 'memberLevel', title: '会员', width: 100 },
  { dataIndex: 'createTime', title: '创建时间', width: 180 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 100 },
];

function orderRecord(record: Record<string, any>): AppOrder {
  return record as AppOrder;
}

function statusLabel(status?: string) {
  return statusOptions.find((item) => item.value === status)?.label || status || '-';
}

function statusColor(status?: string) {
  if (status === 'paid') return 'success';
  if (status === 'not_configured') return 'warning';
  if (status === 'closed') return 'default';
  return 'processing';
}

function memberLabel(level?: string) {
  if (level === 'vip') return 'VIP';
  if (level === 'svip') return 'SVIP';
  return '普通';
}

function amountText(amount?: number) {
  return `¥${((amount ?? 0) / 100).toFixed(2)}`;
}

async function load() {
  loading.value = true;
  try {
    const result = await getAppOrderListApi({
      keyword: query.keyword || undefined,
      page: query.page,
      pageSize: query.pageSize,
      productId: query.productId || undefined,
      status: query.status || undefined,
    });
    orders.value = result.items;
    total.value = result.total;
  } finally {
    loading.value = false;
  }
}

function search() {
  query.page = 1;
  load();
}

function handleTableChange(pagination: {
  current?: number;
  pageSize?: number;
}) {
  query.page = pagination.current ?? 1;
  query.pageSize = pagination.pageSize ?? 20;
  load();
}

function openDetail(record: AppOrder) {
  current.value = record;
  detailOpen.value = true;
}

async function grantOrder(record: AppOrder) {
  Modal.confirm({
    title: '确认补发权益？',
    content: `将订单 ${record.outTradeNo} 标记为已支付，并按商品发放权益。`,
    async onOk() {
      await grantAppOrderApi(record.id);
      message.success('订单权益已补发');
      await load();
    },
  });
}

onMounted(() => {
  load();
});
</script>

<template>
  <Page description="查看 App 用户创建的订单，后续可扩展补发权益和异常处理。" title="App 订单">
    <Card :bordered="false">
      <div class="toolbar">
        <Input
          v-model:value="query.keyword"
          allow-clear
          class="keyword-input"
          placeholder="搜索订单号 / 手机号 / 昵称"
          @press-enter="search"
        />
        <Select
          v-model:value="query.status"
          :options="statusOptions"
          class="filter-select"
          @change="search"
        />
        <Select
          v-model:value="query.productId"
          :options="productOptions"
          class="filter-select"
          @change="search"
        />
        <Space>
          <Button type="primary" @click="search">查询</Button>
          <Button :loading="loading" @click="load">刷新</Button>
        </Space>
      </div>

      <Table
        :columns="columns"
        :data-source="orders"
        :loading="loading"
        :pagination="{
          current: query.page,
          pageSize: query.pageSize,
          showSizeChanger: true,
          total,
        }"
        row-key="id"
        :scroll="{ x: 1120 }"
        @change="handleTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'amount'">
            {{ amountText(record.amount) }}
          </template>
          <template v-if="column.dataIndex === 'status'">
            <Tag :color="statusColor(record.status)">{{ statusLabel(record.status) }}</Tag>
          </template>
          <template v-if="column.dataIndex === 'memberLevel'">
            <Tag>{{ memberLabel(record.memberLevel) }}</Tag>
          </template>
          <template v-if="column.key === 'action'">
            <Space>
              <Button size="small" type="link" @click="openDetail(orderRecord(record))">
                详情
              </Button>
              <Button
                v-if="record.status !== 'paid'"
                size="small"
                type="link"
                @click="grantOrder(orderRecord(record))"
              >
                补发
              </Button>
            </Space>
          </template>
        </template>
      </Table>
    </Card>

    <Drawer v-model:open="detailOpen" title="订单详情" width="620">
      <Descriptions v-if="current" bordered :column="1" size="small">
        <Descriptions.Item label="订单号">{{ current.outTradeNo }}</Descriptions.Item>
        <Descriptions.Item label="用户">
          {{ current.phone || '-' }} / {{ current.nickname || '-' }} / #{{ current.appUserId }}
        </Descriptions.Item>
        <Descriptions.Item label="商品">{{ current.title }}（{{ current.productId }}）</Descriptions.Item>
        <Descriptions.Item label="金额">{{ amountText(current.amount) }}</Descriptions.Item>
        <Descriptions.Item label="状态">{{ statusLabel(current.status) }}</Descriptions.Item>
        <Descriptions.Item label="交易号">{{ current.transactionId || '-' }}</Descriptions.Item>
        <Descriptions.Item label="创建时间">{{ current.createTime }}</Descriptions.Item>
        <Descriptions.Item label="更新时间">{{ current.updateTime }}</Descriptions.Item>
        <Descriptions.Item label="支付时间">{{ current.paidAt || '-' }}</Descriptions.Item>
      </Descriptions>
    </Drawer>
  </Page>
</template>

<style scoped>
.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 16px;
}

.keyword-input {
  width: 260px;
}

.filter-select {
  width: 160px;
}
</style>
