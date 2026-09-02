<script setup lang="ts">
import type { AppOrder } from '#/api';

import { computed, onMounted, reactive, ref } from 'vue';
import dayjs, { type Dayjs } from 'dayjs';

import { Page } from '@vben/common-ui';
import { useAccessStore } from '@vben/stores';

import {
  Button,
  Card,
  message,
  Modal,
  Descriptions,
  DatePicker,
  Drawer,
  Input,
  Select,
  Space,
  Table,
  Tag,
} from 'ant-design-vue';

import {
  getAppOrderListApi,
  grantAppOrderApi,
  reconcileAppOrderApi,
} from '#/api';
import { ellipsisColumn } from '#/components/ellipsis-tooltip/table';
import {
  buildMembershipGrantPayload,
  memberPlanLabel,
  membershipStatusLabel,
  previewMembershipExpiry,
} from './app-membership';

const APP_ORDER_WRITE_PERMISSION = 'Customer:AppOrders:Write';

const accessStore = useAccessStore();
const canGrantOrder = computed(() =>
  accessStore.accessCodes.includes(APP_ORDER_WRITE_PERMISSION),
);
const loading = ref(false);
const orders = ref<AppOrder[]>([]);
const total = ref(0);
const detailOpen = ref(false);
const current = ref<AppOrder>();
const grantOpen = ref(false);
const grantSaving = ref(false);
const grantRecord = ref<AppOrder>();
const reconcilingOrderId = ref<number>();
const activationAt = ref<Dayjs>(dayjs());
const estimatedExpiry = computed(() => {
  const record = grantRecord.value;
  if (!record || !activationAt.value) return undefined;
  return previewMembershipExpiry(
    record.memberExpiresAt,
    activationAt.value.toDate(),
    record.durationDays,
  );
});

const query = reactive({
  keyword: '',
  page: 1,
  pageSize: 20,
  productId: '',
  status: '',
});

const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '待支付', value: 'pending' },
  { label: '支付处理中', value: 'paying' },
  { label: '待客服确认', value: 'pending_confirmation' },
  { label: '已开通', value: 'paid' },
  { label: '支付失败', value: 'failed' },
  { label: '已关闭', value: 'closed' },
];

const productOptions = [
  { label: '全部商品', value: '' },
  { label: '月卡会员', value: 'vip_month' },
  { label: '季卡会员', value: 'vip_quarter' },
  { label: '年卡会员', value: 'vip_year' },
];

const columns = [
  ellipsisColumn('outTradeNo', '订单号', { fixed: 'left', width: 230 }),
  { dataIndex: 'phone', title: '手机号', width: 150 },
  ellipsisColumn('title', '商品', { width: 140 }),
  { dataIndex: 'amount', title: '金额', width: 100 },
  { dataIndex: 'paymentProvider', title: '支付来源', width: 120 },
  { dataIndex: 'payChannel', title: '支付渠道', width: 110 },
  { dataIndex: 'gatewayId', title: '网关', width: 90 },
  { dataIndex: 'providerStatus', title: '平台状态', width: 130 },
  { dataIndex: 'status', title: '状态', width: 120 },
  { dataIndex: 'memberLevel', title: '会员', width: 100 },
  { dataIndex: 'memberExpiresAt', title: '会员到期', width: 180 },
  { dataIndex: 'remainingDays', title: '剩余天数', width: 100 },
  { dataIndex: 'createTime', title: '创建时间', width: 180 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 156 },
];

function orderRecord(record: Record<string, any>): AppOrder {
  return record as AppOrder;
}

function statusLabel(status?: string) {
  const labels: Record<string, string> = {
    failed: '支付失败',
    paying: '支付处理中',
    pending: '待支付',
  };
  return labels[status || ''] || membershipStatusLabel(status);
}

function statusColor(status?: string) {
  const normalized = status?.toLowerCase();
  if (
    status === 'paid' ||
    normalized === 'success' ||
    normalized === 'trade_success'
  )
    return 'success';
  if (status === 'not_configured') return 'warning';
  if (normalized === 'failed') return 'error';
  if (normalized === 'closed' || normalized === 'cancelled') return 'default';
  return 'processing';
}

function memberLabel(level?: string) {
  return memberPlanLabel(level);
}

function amountText(amount?: number) {
  return `¥${((amount ?? 0) / 100).toFixed(2)}`;
}

function dateText(value?: Date) {
  return value ? dayjs(value).format('YYYY/MM/DD HH:mm:ss') : '-';
}

function paymentProviderLabel(provider?: string) {
  const labels: Record<string, string> = {
    customer_service: '客服转账',
    manual: '人工订单',
    xzn: '星之柠支付',
    xzn_pay: '星之柠支付',
    xznpay: '星之柠支付',
  };
  return labels[provider || 'manual'] || provider || '人工订单';
}

function payChannelLabel(channel?: string) {
  const labels: Record<string, string> = {
    alipay: '支付宝',
    wechat: '微信支付',
    wxpay: '微信支付',
  };
  return labels[channel || ''] || channel || '-';
}

function providerStatusLabel(status?: string) {
  const labels: Record<string, string> = {
    closed: '已关闭',
    failed: '支付失败',
    paying: '支付处理中',
    pending: '待支付',
    success: '支付成功',
    trade_success: '支付成功',
    trade_closed: '订单关闭',
    trade_refund: '已退款',
    trade_freeze: '风控审核中',
    trade_unfreeze: '已解除风控',
    wait_buyer_pay: '待支付',
  };
  return labels[status?.toLowerCase() || ''] || status || '-';
}

function isLegacyManualOrder(record: AppOrder) {
  const provider = record.paymentProvider?.trim().toLowerCase();
  return !provider || provider === 'manual' || provider === 'customer_service';
}

function isOnlineOrder(record: AppOrder) {
  return !isLegacyManualOrder(record);
}

function canReconcileOrder(record: AppOrder) {
  return (
    canGrantOrder.value &&
    isOnlineOrder(record) &&
    record.status !== 'paid'
  );
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

function grantOrder(record: AppOrder) {
  grantRecord.value = record;
  activationAt.value = dayjs();
  grantOpen.value = true;
}

async function confirmGrant() {
  if (!grantRecord.value || !activationAt.value) return;
  grantSaving.value = true;
  try {
    const result = await grantAppOrderApi(
      grantRecord.value.id,
      buildMembershipGrantPayload(activationAt.value.toDate()),
    );
    message.success(
      result.alreadyGranted
        ? '该订单已经开通过会员'
        : `会员已开通，有效期至 ${result.expiresAt}`,
    );
    grantOpen.value = false;
    await load();
  } finally {
    grantSaving.value = false;
  }
}

async function reconcileOrder(record: AppOrder) {
  reconcilingOrderId.value = record.id;
  try {
    const result = await reconcileAppOrderApi(record.id);
    message.success(
      result.status === 'paid'
        ? '查单完成，订单已开通会员'
        : '查单完成，订单状态已更新',
    );
    await load();
  } catch (error) {
    message.error(
      error instanceof Error ? error.message : '主动查单失败，请稍后重试',
    );
  } finally {
    reconcilingOrderId.value = undefined;
  }
}

onMounted(() => {
  load();
});
</script>

<template>
  <Page
    description="查看在线支付状态；历史客服转账订单仍可由有权限的人员人工开通。"
    title="App 订单"
  >
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
          placeholder="请选择状态"
        />
        <Select
          v-model:value="query.productId"
          :options="productOptions"
          class="filter-select"
          @change="search"
          placeholder="请选择商品"
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
        :scroll="{ x: 1880 }"
        @change="handleTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'amount'">
            {{ amountText(record.amount) }}
          </template>
          <template v-if="column.dataIndex === 'status'">
            <Tag :color="statusColor(record.status)">{{
              statusLabel(record.status)
            }}</Tag>
          </template>
          <template v-if="column.dataIndex === 'paymentProvider'">
            {{ paymentProviderLabel(record.paymentProvider) }}
          </template>
          <template v-if="column.dataIndex === 'payChannel'">
            {{ payChannelLabel(record.payChannel) }}
          </template>
          <template v-if="column.dataIndex === 'gatewayId'">
            {{ record.gatewayId || '-' }}
          </template>
          <template v-if="column.dataIndex === 'providerStatus'">
            <Tag
              v-if="record.providerStatus"
              :color="statusColor(record.providerStatus)"
            >
              {{ providerStatusLabel(record.providerStatus) }}
            </Tag>
            <span v-else>-</span>
          </template>
          <template v-if="column.dataIndex === 'memberLevel'">
            <Tag>{{ memberLabel(record.memberLevel) }}</Tag>
          </template>
          <template v-if="column.key === 'action'">
            <Space>
              <Button
                size="small"
                type="link"
                @click="openDetail(orderRecord(record))"
              >
                详情
              </Button>
              <Button
                v-if="canReconcileOrder(orderRecord(record))"
                :loading="reconcilingOrderId === record.id"
                size="small"
                type="link"
                @click="reconcileOrder(orderRecord(record))"
              >
                主动查单
              </Button>
              <Button
                v-if="
                  isLegacyManualOrder(orderRecord(record)) &&
                  record.status !== 'paid' &&
                  canGrantOrder
                "
                size="small"
                type="link"
                @click="grantOrder(orderRecord(record))"
              >
                确认开通
              </Button>
            </Space>
          </template>
        </template>
      </Table>
    </Card>

    <Drawer v-model:open="detailOpen" title="订单详情" width="620">
      <Descriptions v-if="current" bordered :column="1" size="small">
        <Descriptions.Item label="订单号">{{
          current.outTradeNo
        }}</Descriptions.Item>
        <Descriptions.Item label="用户">
          {{ current.phone || '-' }} / {{ current.nickname || '-' }} / #{{
            current.appUserId
          }}
        </Descriptions.Item>
        <Descriptions.Item label="商品"
          >{{ current.title }}（{{ current.productId }}）</Descriptions.Item
        >
        <Descriptions.Item label="会员时长"
          >{{ current.durationDays }} 天</Descriptions.Item
        >
        <Descriptions.Item label="金额">{{
          amountText(current.amount)
        }}</Descriptions.Item>
        <Descriptions.Item label="状态">{{
          statusLabel(current.status)
        }}</Descriptions.Item>
        <Descriptions.Item label="支付来源">{{
          paymentProviderLabel(current.paymentProvider)
        }}</Descriptions.Item>
        <Descriptions.Item label="支付渠道">{{
          payChannelLabel(current.payChannel)
        }}</Descriptions.Item>
        <Descriptions.Item label="网关 ID">{{
          current.gatewayId || '-'
        }}</Descriptions.Item>
        <Descriptions.Item label="平台支付状态">{{
          providerStatusLabel(current.providerStatus)
        }}</Descriptions.Item>
        <Descriptions.Item label="平台交易号">{{
          current.providerTradeNo || '-'
        }}</Descriptions.Item>
        <Descriptions.Item label="最近查单时间">{{
          current.lastQueryAt || '-'
        }}</Descriptions.Item>
        <Descriptions.Item label="支付错误">{{
          current.paymentError || '-'
        }}</Descriptions.Item>
        <Descriptions.Item label="交易号">{{
          current.transactionId || '-'
        }}</Descriptions.Item>
        <Descriptions.Item label="创建时间">{{
          current.createTime
        }}</Descriptions.Item>
        <Descriptions.Item label="更新时间">{{
          current.updateTime
        }}</Descriptions.Item>
        <Descriptions.Item label="支付时间">{{
          current.paidAt || '-'
        }}</Descriptions.Item>
        <Descriptions.Item label="会员生效时间">{{
          current.activationAt || '-'
        }}</Descriptions.Item>
        <Descriptions.Item label="本单开通后到期">{{
          current.membershipExpiresAt || '-'
        }}</Descriptions.Item>
        <Descriptions.Item label="当前会员到期">{{
          current.memberExpiresAt || '-'
        }}</Descriptions.Item>
      </Descriptions>
    </Drawer>

    <Modal
      v-model:open="grantOpen"
      :confirm-loading="grantSaving"
      ok-text="确认收款并开通"
      title="确认会员开通"
      @ok="confirmGrant"
    >
      <Descriptions v-if="grantRecord" bordered :column="1" size="small">
        <Descriptions.Item label="套餐">{{
          grantRecord.title
        }}</Descriptions.Item>
        <Descriptions.Item label="用户手机号">{{
          grantRecord.phone || '-'
        }}</Descriptions.Item>
        <Descriptions.Item label="订单号">{{
          grantRecord.outTradeNo
        }}</Descriptions.Item>
        <Descriptions.Item label="当前会员到期">
          {{ grantRecord.memberExpiresAt || '当前无有效会员' }}
        </Descriptions.Item>
        <Descriptions.Item label="续费时长"
          >{{ grantRecord.durationDays }} 天</Descriptions.Item
        >
        <Descriptions.Item label="预计续费后到期">
          {{ dateText(estimatedExpiry) }}
        </Descriptions.Item>
      </Descriptions>
      <div class="activation-field">
        <div class="activation-label">会员生效时间</div>
        <DatePicker
          v-model:value="activationAt"
          show-time
          class="activation-picker"
          format="YYYY-MM-DD HH:mm:ss"
          placeholder="请选择生效时间"
        />
        <div class="activation-help">
          仍在有效期内的会员会从原到期时间继续顺延。
        </div>
      </div>
    </Modal>
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

.activation-field {
  margin-top: 20px;
}

.activation-label {
  margin-bottom: 8px;
  font-weight: 600;
}

.activation-picker {
  width: 100%;
}

.activation-help {
  margin-top: 8px;
  color: var(--ant-color-text-secondary);
  font-size: 13px;
}
</style>
