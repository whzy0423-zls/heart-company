<script setup lang="ts">
import type { SignupLead } from '#/api';

import { onMounted, onUnmounted, ref } from 'vue';

import {
  Button,
  Descriptions,
  Drawer,
  Input,
  Space,
  Table,
  Tag,
} from 'ant-design-vue';

import { getSignupLeadListApi } from '#/api';

import PageShell from '../system/components/page-shell.vue';

const loading = ref(false);
const leads = ref<SignupLead[]>([]);
const total = ref(0);
const detailOpen = ref(false);
const current = ref<SignupLead>();
const query = ref({ keyword: '', page: 1, pageSize: 20 });
let refreshTimer: number | undefined;
let requestId = 0;

const columns = [
  { dataIndex: 'name', title: '称呼', width: 140 },
  { dataIndex: 'contactType', title: '联系类型', width: 110 },
  { dataIndex: 'contact', title: '联系方式', width: 180 },
  { dataIndex: 'interest', title: '兴趣方向', width: 180 },
  { dataIndex: 'message', ellipsis: true, title: '咨询需求' },
  { dataIndex: 'createTime', title: '提交时间', width: 180 },
  { key: 'action', title: '操作', width: 100 },
];

async function load(options: { silent?: boolean } = {}) {
  const currentRequestId = ++requestId;
  if (!options.silent) {
    loading.value = true;
  }
  try {
    const result = await getSignupLeadListApi(query.value);
    if (currentRequestId !== requestId) return;
    leads.value = result.items;
    total.value = result.total;
  } finally {
    if (!options.silent && currentRequestId === requestId) {
      loading.value = false;
    }
  }
}

async function refreshLatest() {
  query.value.keyword = '';
  query.value.page = 1;
  await load();
}

async function refreshSilently() {
  await load({ silent: true });
}

function openDetail(record: SignupLead) {
  current.value = record;
  detailOpen.value = true;
}

function contactTypeLabel(type?: string) {
  return type === 'wechat' ? '微信号' : '手机号';
}

function handleTableChange(pagination: { current?: number; pageSize?: number }) {
  query.value.page = pagination.current ?? 1;
  query.value.pageSize = pagination.pageSize ?? 20;
  load();
}

function search() {
  query.value.page = 1;
  load();
}

onMounted(() => {
  load();
  refreshTimer = window.setInterval(() => {
    refreshSilently();
  }, 5000);
  window.addEventListener('focus', refreshSilently);
});

onUnmounted(() => {
  if (refreshTimer) {
    window.clearInterval(refreshTimer);
  }
  window.removeEventListener('focus', refreshSilently);
});
</script>

<template>
  <PageShell
    description="查看官网报名咨询表单提交的数据，便于后续跟进联系。"
    :loading="loading"
    title="报名信息"
    @refresh="load"
  >
    <Space class="toolbar">
      <Input
        v-model:value="query.keyword"
        allow-clear
        placeholder="搜索称呼 / 联系方式 / 兴趣 / 需求"
        @press-enter="search"
      />
      <Button type="primary" @click="search">查询</Button>
      <Button @click="refreshLatest">最新报名</Button>
    </Space>
    <Table
      :columns="columns"
      :data-source="leads"
      :pagination="{
        current: query.page,
        pageSize: query.pageSize,
        showSizeChanger: true,
        total,
      }"
      row-key="id"
      @change="handleTableChange"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'interest'">
          <Tag v-if="record.interest">{{ record.interest }}</Tag>
          <span v-else>-</span>
        </template>
        <template v-if="column.dataIndex === 'contactType'">
          <Tag :color="record.contactType === 'wechat' ? 'green' : 'blue'">
            {{ contactTypeLabel(record.contactType) }}
          </Tag>
        </template>
        <template v-if="column.dataIndex === 'message'">
          <span>{{ record.message || '-' }}</span>
        </template>
        <template v-if="column.key === 'action'">
          <Button size="small" type="link" @click="openDetail(record)">
            查看
          </Button>
        </template>
      </template>
    </Table>

    <Drawer v-model:open="detailOpen" title="报名详情" width="520px">
      <Descriptions v-if="current" :column="1" bordered size="small">
        <Descriptions.Item label="称呼">{{ current.name }}</Descriptions.Item>
        <Descriptions.Item label="联系类型">
          {{ contactTypeLabel(current.contactType) }}
        </Descriptions.Item>
        <Descriptions.Item label="联系方式">
          {{ current.contact }}
        </Descriptions.Item>
        <Descriptions.Item label="兴趣方向">
          {{ current.interest || '-' }}
        </Descriptions.Item>
        <Descriptions.Item label="咨询需求">
          <div class="message-text">{{ current.message || '-' }}</div>
        </Descriptions.Item>
        <Descriptions.Item label="提交时间">
          {{ current.createTime }}
        </Descriptions.Item>
        <Descriptions.Item label="IP">{{ current.ip || '-' }}</Descriptions.Item>
        <Descriptions.Item label="User Agent">
          <div class="ua-text">{{ current.userAgent || '-' }}</div>
        </Descriptions.Item>
      </Descriptions>
    </Drawer>
  </PageShell>
</template>

<style scoped>
.toolbar {
  margin-bottom: 16px;
}

.message-text,
.ua-text {
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
