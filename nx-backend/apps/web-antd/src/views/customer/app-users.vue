<script setup lang="ts">
import type { AppCustomer } from '#/api';

import { onMounted, reactive, ref } from 'vue';

import {
  Button,
  Card,
  Descriptions,
  Drawer,
  Input,
  Select,
  Space,
  Table,
  Tag,
} from 'ant-design-vue';

import { getAppCustomerDetailApi, getAppCustomerListApi } from '#/api';

import PageShell from '../system/components/page-shell.vue';

const statusOptions = [
  { color: 'success', label: '正常', value: 'active' },
  { color: 'error', label: '禁用', value: 'disabled' },
] satisfies StatusMeta[];

const defaultStatusMeta: StatusMeta = {
  color: 'default',
  label: '-',
  value: '',
};

interface StatusMeta {
  color: string;
  label: string;
  value: string;
}

const memberLevelLabels: Record<string, string> = {
  free: '普通用户',
  vip: 'VIP 会员',
  svip: '超级会员',
};

const loading = ref(false);
const detailLoading = ref(false);
const customers = ref<AppCustomer[]>([]);
const total = ref(0);
const detailOpen = ref(false);
const detail = ref<AppCustomer>();
const query = reactive({
  keyword: '',
  memberLevel: '',
  page: 1,
  pageSize: 20,
  status: '',
});
let requestId = 0;

const columns = [
  { dataIndex: 'phone', fixed: 'left' as const, title: '手机号', width: 160 },
  { dataIndex: 'nickname', title: '昵称', width: 160 },
  { dataIndex: 'memberLevel', title: '会员等级', width: 130 },
  { dataIndex: 'status', title: '状态', width: 100 },
  { dataIndex: 'registerSource', title: '注册来源', width: 130 },
  { dataIndex: 'lastLoginAt', title: '最后登录', width: 180 },
  { dataIndex: 'createTime', title: '注册时间', width: 180 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 100 },
];

function statusMeta(status?: string): StatusMeta {
  return (
    statusOptions.find((item) => item.value === status) ?? defaultStatusMeta
  );
}

function memberLevelLabel(value?: string) {
  if (!value) return '-';
  return memberLevelLabels[value] || value;
}

function sourceLabel(value?: string) {
  if (!value) return '-';
  if (value === 'app_sms') return 'App 短信登录';
  return value;
}

async function load() {
  const currentRequestId = ++requestId;
  loading.value = true;
  try {
    const result = await getAppCustomerListApi({
      keyword: query.keyword || undefined,
      memberLevel: query.memberLevel || undefined,
      page: query.page,
      pageSize: query.pageSize,
      status: query.status || undefined,
    });
    if (currentRequestId !== requestId) return;
    customers.value = result.items;
    total.value = result.total;
  } finally {
    if (currentRequestId === requestId) {
      loading.value = false;
    }
  }
}

async function openDetail(record: AppCustomer) {
  detailOpen.value = true;
  detailLoading.value = true;
  try {
    detail.value = await getAppCustomerDetailApi(record.id);
  } finally {
    detailLoading.value = false;
  }
}

function customerRecord(record: Record<string, any>): AppCustomer {
  return record as AppCustomer;
}

function handleTableChange(pagination: {
  current?: number;
  pageSize?: number;
}) {
  query.page = pagination.current ?? 1;
  query.pageSize = pagination.pageSize ?? 20;
  load();
}

function search() {
  query.page = 1;
  load();
}

onMounted(() => {
  load();
});
</script>

<template>
  <PageShell
    description="查看通过手机号登录 App 的客户，维护其会员等级与基础资料。"
    :loading="loading"
    title="App 客户"
    @refresh="load"
  >
    <div class="app-user-page">
      <Card :bordered="false" class="filter-card">
        <div class="filter-bar">
          <Input
            v-model:value="query.keyword"
            allow-clear
            class="keyword-input"
            placeholder="搜索手机号 / 昵称"
            @press-enter="search"
          />
          <Select
            v-model:value="query.memberLevel"
            allow-clear
            class="filter-select"
            :options="[
              { label: '普通用户', value: 'free' },
              { label: 'VIP 会员', value: 'vip' },
              { label: '超级会员', value: 'svip' },
            ]"
            placeholder="会员等级"
          />
          <Select
            v-model:value="query.status"
            allow-clear
            class="filter-select"
            :options="statusOptions"
            placeholder="状态"
          />
          <Space>
            <Button type="primary" @click="search">查询</Button>
          </Space>
        </div>
      </Card>

      <Card :bordered="false" class="table-card">
        <Table
          :columns="columns"
          :data-source="customers"
          :loading="loading"
          :pagination="{
            current: query.page,
            pageSize: query.pageSize,
            showSizeChanger: true,
            total,
          }"
          :scroll="{ x: 1040 }"
          row-key="id"
          table-layout="fixed"
          @change="handleTableChange"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'nickname'">
              {{ record.nickname || '-' }}
            </template>
            <template v-if="column.dataIndex === 'memberLevel'">
              <Tag>{{ memberLevelLabel(record.memberLevel) }}</Tag>
            </template>
            <template v-if="column.dataIndex === 'status'">
              <Tag :color="statusMeta(customerRecord(record).status).color">
                {{ statusMeta(customerRecord(record).status).label }}
              </Tag>
            </template>
            <template v-if="column.dataIndex === 'registerSource'">
              {{ sourceLabel(record.registerSource) }}
            </template>
            <template v-if="column.dataIndex === 'lastLoginAt'">
              {{ record.lastLoginAt || '-' }}
            </template>
            <template v-if="column.key === 'action'">
              <Button
                size="small"
                type="link"
                @click="openDetail(customerRecord(record))"
              >
                查看详情
              </Button>
            </template>
          </template>
        </Table>
      </Card>
    </div>

    <Drawer
      v-model:open="detailOpen"
      :loading="detailLoading"
      title="客户详情"
      width="520px"
    >
      <div v-if="detail" class="detail-layout">
        <div class="user-profile">
          <div class="profile-avatar">
            {{ (detail.nickname || detail.phone)?.slice(0, 1) || '客' }}
          </div>
          <div class="profile-main">
            <div class="profile-title-row">
              <h3>{{ detail.nickname || detail.phone }}</h3>
              <Tag :color="statusMeta(detail.status).color">
                {{ statusMeta(detail.status).label }}
              </Tag>
            </div>
            <div class="profile-meta">手机号：{{ detail.phone }}</div>
          </div>
        </div>

        <Descriptions :column="1" bordered size="small">
          <Descriptions.Item label="客户 ID">
            {{ detail.id }}
          </Descriptions.Item>
          <Descriptions.Item label="手机号">
            {{ detail.phone }}
          </Descriptions.Item>
          <Descriptions.Item label="昵称">
            {{ detail.nickname || '-' }}
          </Descriptions.Item>
          <Descriptions.Item label="会员等级">
            {{ memberLevelLabel(detail.memberLevel) }}
          </Descriptions.Item>
          <Descriptions.Item label="状态">
            {{ statusMeta(detail.status).label }}
          </Descriptions.Item>
          <Descriptions.Item label="注册来源">
            {{ sourceLabel(detail.registerSource) }}
          </Descriptions.Item>
          <Descriptions.Item label="最后登录">
            {{ detail.lastLoginAt || '-' }}
          </Descriptions.Item>
          <Descriptions.Item label="注册时间">
            {{ detail.createTime }}
          </Descriptions.Item>
          <Descriptions.Item label="更新时间">
            {{ detail.updateTime }}
          </Descriptions.Item>
        </Descriptions>
      </div>
    </Drawer>
  </PageShell>
</template>

<style scoped>
.app-user-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.filter-card :deep(.ant-card-body),
.table-card :deep(.ant-card-body) {
  padding: 16px;
}

.filter-bar {
  display: grid;
  grid-template-columns: minmax(220px, 360px) 160px 140px auto;
  gap: 10px;
  justify-content: start;
}

.keyword-input,
.filter-select {
  width: 100%;
}

.detail-layout {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.user-profile {
  display: flex;
  gap: 14px;
  align-items: center;
  padding: 16px;
  background: hsl(var(--accent) / 32%);
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
}

.profile-avatar {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  font-size: 20px;
  font-weight: 700;
  color: hsl(var(--primary));
  background: hsl(var(--primary) / 12%);
  border: 1px solid hsl(var(--primary) / 20%);
  border-radius: 8px;
}

.profile-main {
  min-width: 0;
}

.profile-title-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.profile-title-row h3 {
  margin: 0;
  font-size: 18px;
  line-height: 26px;
}

.profile-meta {
  margin-top: 4px;
  color: hsl(var(--muted-foreground));
}
</style>
