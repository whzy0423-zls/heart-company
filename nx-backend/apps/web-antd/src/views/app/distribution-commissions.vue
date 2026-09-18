<script setup lang="ts">
import { formatYuan } from './distribution-format';
import { onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { Table, Tag, message } from 'ant-design-vue';

import { requestClient } from '#/api/request';

type Commission = {
  AgentID: number;
  Amount: number;
  AppUserID: number;
  CreatedAt: string;
  ID: number;
  Level: number;
  OrderAmount: number;
  OrderID: number;
  RateBPS: number;
  RuleVersion: number;
  Status: string;
};

const items = ref<Commission[]>([]);
const loading = ref(false);

const columns = [
  { dataIndex: 'ID', title: 'ID' },
  { dataIndex: 'OrderID', title: '订单' },
  { dataIndex: 'AgentID', title: '代理' },
  { dataIndex: 'OrderAmount', title: '订单金额(元)' },
  { dataIndex: 'Amount', title: '佣金(元)' },
  { dataIndex: 'RuleVersion', title: '规则版本' },
  { dataIndex: 'Status', title: '状态' },
];


async function load() {
  loading.value = true;
  try {
    items.value = (
      await requestClient.get<{ items: Commission[] }>(
        '/admin/distribution/commissions',
      )
    ).items;
  } catch {
    message.error('佣金明细加载失败');
  } finally {
    loading.value = false;
  }
}

onMounted(load);
</script>

<template>
  <Page title="佣金明细">
    <Table
      :columns="columns"
      :data-source="items"
      :loading="loading"
      :pagination="false"
      row-key="ID"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'OrderAmount'">
          {{ formatYuan(record.OrderAmount) }}
        </template>
        <template v-else-if="column.dataIndex === 'Amount'">
          {{ formatYuan(record.Amount) }}
        </template>
        <template v-else-if="column.dataIndex === 'Status'">
          <Tag>{{ record.Status }}</Tag>
        </template>
      </template>
    </Table>
  </Page>
</template>
