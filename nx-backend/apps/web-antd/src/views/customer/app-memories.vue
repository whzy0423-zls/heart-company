<script setup lang="ts">
import type { AppMemoryAdminItem } from '#/api';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useAccessStore } from '@vben/stores';

import {
  Alert,
  Button,
  Card,
  Descriptions,
  Drawer,
  Input,
  message,
  Select,
  Space,
  Table,
  Tag,
} from 'ant-design-vue';

import { getAppMemoriesAdminApi, updateAppMemoryStatusApi } from '#/api';
import { ellipsisColumn } from '#/components/ellipsis-tooltip/table';

const APP_MEMORY_WRITE_PERMISSION = 'Customer:AppMemory:Write';

const accessStore = useAccessStore();
const canManageMemory = computed(() =>
  accessStore.accessCodes.includes(APP_MEMORY_WRITE_PERMISSION),
);
const loading = ref(false);
const loadError = ref('');
const items = ref<AppMemoryAdminItem[]>([]);
const total = ref(0);
const detailOpen = ref(false);
const current = ref<AppMemoryAdminItem>();
const query = reactive({ keyword: '', status: '', page: 1, pageSize: 20 });
let requestId = 0;

const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '启用', value: 'active' },
  { label: '停用', value: 'disabled' },
];
const columns = [
  { dataIndex: 'updateTime', title: '更新时间', width: 180 },
  { dataIndex: 'phone', title: '手机号', width: 150 },
  { dataIndex: 'cardName', title: '卡片', width: 140 },
  ellipsisColumn('content', '记忆内容', { lines: 2 }),
  { dataIndex: 'status', title: '状态', width: 100 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 170 },
];

function row(r: Record<string, any>) {
  return r as AppMemoryAdminItem;
}

function color(s?: string) {
  return s === 'active' ? 'success' : 'default';
}

function label(s?: string) {
  return s === 'active' ? '启用' : s === 'disabled' ? '停用' : s || '-';
}

async function load() {
  const currentRequestId = ++requestId;
  loading.value = true;
  loadError.value = '';
  try {
    const r = await getAppMemoriesAdminApi({
      keyword: query.keyword || undefined,
      page: query.page,
      pageSize: query.pageSize,
      status: query.status || undefined,
    });
    if (currentRequestId !== requestId) return;
    items.value = r.items;
    total.value = r.total;
  } catch {
    if (currentRequestId === requestId) {
      loadError.value = '私库记忆加载失败，请稍后重试';
    }
  } finally {
    if (currentRequestId === requestId) {
      loading.value = false;
    }
  }
}

function search() {
  query.page = 1;
  void load();
}

function change(p: { current?: number; pageSize?: number }) {
  query.page = p.current ?? 1;
  query.pageSize = p.pageSize ?? 20;
  void load();
}

function open(r: AppMemoryAdminItem) {
  current.value = r;
  detailOpen.value = true;
}

async function setStatus(r: AppMemoryAdminItem, status: string) {
  try {
    await updateAppMemoryStatusApi(r.id, status);
    message.success(status === 'active' ? '已启用' : '已停用');
    await load();
  } catch (error) {
    message.error(error instanceof Error ? error.message : '记忆状态更新失败');
  }
}

onMounted(() => {
  void load();
});
</script>

<template>
  <Page
    description="查看并停用异常 App 私库记忆，避免低质量记忆参与后续回答。"
    title="私库记忆"
  >
    <Card :bordered="false">
      <Alert
        v-if="loadError"
        :message="loadError"
        show-icon
        type="error"
      >
        <template #action>
          <Button size="small" type="link" @click="load">重试</Button>
        </template>
      </Alert>

      <div class="toolbar">
        <Input
          v-model:value="query.keyword"
          allow-clear
          class="keyword"
          placeholder="搜索记忆 / 手机号 / 昵称"
          @press-enter="search"
        />
        <Select
          v-model:value="query.status"
          :options="statusOptions"
          class="filter"
          @change="search"
        />
        <Space>
          <Button type="primary" @click="search">查询</Button>
          <Button :loading="loading" @click="load">刷新</Button>
        </Space>
      </div>

      <Table
        :columns="columns"
        :data-source="items"
        :loading="loading"
        :pagination="{
          current: query.page,
          pageSize: query.pageSize,
          showSizeChanger: true,
          total,
        }"
        row-key="id"
        :scroll="{ x: 1040 }"
        @change="change"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'status'">
            <Tag :color="color(record.status)">{{ label(record.status) }}</Tag>
          </template>
          <template v-if="column.key === 'action'">
            <Space>
              <Button size="small" type="link" @click="open(row(record))">
                详情
              </Button>
              <Button
                v-if="canManageMemory && record.status === 'active'"
                danger
                size="small"
                type="link"
                @click="setStatus(row(record), 'disabled')"
              >
                停用
              </Button>
              <Button
                v-else-if="canManageMemory"
                size="small"
                type="link"
                @click="setStatus(row(record), 'active')"
              >
                启用
              </Button>
            </Space>
          </template>
        </template>
      </Table>
    </Card>

    <Drawer
      v-model:open="detailOpen"
      title="私库记忆详情"
      width="min(680px, calc(100vw - 32px))"
    >
      <Descriptions v-if="current" bordered :column="1" size="small">
        <Descriptions.Item label="用户">
          {{ current.phone }} / {{ current.nickname || '-' }} / #{{ current.appUserId }}
        </Descriptions.Item>
        <Descriptions.Item label="卡片">
          {{ current.cardName || '-' }} / #{{ current.cardId }}
        </Descriptions.Item>
        <Descriptions.Item label="状态">{{ label(current.status) }}</Descriptions.Item>
        <Descriptions.Item label="来源时间">{{ current.sourceTime || '-' }}</Descriptions.Item>
        <Descriptions.Item label="更新时间">{{ current.updateTime }}</Descriptions.Item>
        <Descriptions.Item label="内容">
          <pre>{{ current.content }}</pre>
        </Descriptions.Item>
      </Descriptions>
    </Drawer>
  </Page>
</template>

<style scoped>
.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin: 16px 0;
}

.keyword {
  width: min(100%, 280px);
}

.filter {
  width: min(100%, 150px);
}

pre {
  line-height: 1.7;
  white-space: pre-wrap;
}

@media (max-width: 640px) {
  .toolbar,
  .toolbar :deep(.ant-space) {
    width: 100%;
  }

  .keyword,
  .filter {
    width: 100%;
  }
}
</style>
