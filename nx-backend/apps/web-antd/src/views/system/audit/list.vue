<script setup lang="ts">
import type { AdminAuditLog } from '#/api';

import { onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';

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

import { getAdminAuditLogsApi } from '#/api';
import { ellipsisColumn } from '#/components/ellipsis-tooltip/table';

const loading = ref(false);
const logs = ref<AdminAuditLog[]>([]);
const total = ref(0);
const detailOpen = ref(false);
const current = ref<AdminAuditLog>();

const query = reactive({
  action: '',
  operator: '',
  page: 1,
  pageSize: 20,
  targetType: '',
});

const actionOptions = [
  { label: '全部动作', value: '' },
  { label: 'App 客户更新', value: 'app_user.update' },
  { label: '模型配置更新', value: 'model_config.update' },
  { label: '推送发送', value: 'push.send' },
];

const targetTypeOptions = [
  { label: '全部对象', value: '' },
  { label: 'App 客户', value: 'app_user' },
  { label: '模型配置', value: 'model_config' },
  { label: '推送通知', value: 'push_notification' },
];

const columns = [
  { dataIndex: 'createTime', title: '时间', width: 180 },
  { dataIndex: 'operatorName', title: '操作者', width: 140 },
  { dataIndex: 'action', title: '动作', width: 170 },
  { dataIndex: 'targetType', title: '对象类型', width: 140 },
  { dataIndex: 'targetId', title: '对象 ID', width: 120 },
  ellipsisColumn('summary', '摘要', { lines: 2 }),
  { dataIndex: 'ip', title: 'IP', width: 150 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 100 },
];

function auditRecord(record: Record<string, any>): AdminAuditLog {
  return record as AdminAuditLog;
}

function actionLabel(action?: string) {
  const option = actionOptions.find((item) => item.value === action);
  return option?.label || action || '-';
}

function targetTypeLabel(targetType?: string) {
  const option = targetTypeOptions.find((item) => item.value === targetType);
  return option?.label || targetType || '-';
}

function pretty(value: unknown) {
  if (!value) return '{}';
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

async function load() {
  loading.value = true;
  try {
    const result = await getAdminAuditLogsApi({
      action: query.action || undefined,
      operator: query.operator || undefined,
      page: query.page,
      pageSize: query.pageSize,
      targetType: query.targetType || undefined,
    });
    logs.value = result.items;
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

function openDetail(record: AdminAuditLog) {
  current.value = record;
  detailOpen.value = true;
}

onMounted(() => {
  load();
});
</script>

<template>
  <Page description="追踪后台关键操作，便于安全审计和问题回溯。" title="操作审计">
    <Card :bordered="false">
      <div class="toolbar">
        <Select
          v-model:value="query.action"
          :options="actionOptions"
          class="filter-select"
          @change="search"
        />
        <Select
          v-model:value="query.targetType"
          :options="targetTypeOptions"
          class="filter-select"
          @change="search"
        />
        <Input
          v-model:value="query.operator"
          allow-clear
          class="operator-input"
          placeholder="搜索操作者"
          @press-enter="search"
        />
        <Space>
          <Button type="primary" @click="search">查询</Button>
          <Button :loading="loading" @click="load">刷新</Button>
        </Space>
      </div>

      <Table
        :columns="columns"
        :data-source="logs"
        :loading="loading"
        :pagination="{
          current: query.page,
          pageSize: query.pageSize,
          showSizeChanger: true,
          total,
        }"
        row-key="id"
        :scroll="{ x: 1180 }"
        @change="handleTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'action'">
            <Tag color="processing">{{ actionLabel(record.action) }}</Tag>
          </template>
          <template v-if="column.dataIndex === 'targetType'">
            {{ targetTypeLabel(record.targetType) }}
          </template>
          <template v-if="column.key === 'action'">
            <Button size="small" type="link" @click="openDetail(auditRecord(record))">
              详情
            </Button>
          </template>
        </template>
      </Table>
    </Card>

    <Drawer v-model:open="detailOpen" title="操作审计详情" width="720">
      <Descriptions v-if="current" bordered :column="1" size="small">
        <Descriptions.Item label="时间">{{ current.createTime }}</Descriptions.Item>
        <Descriptions.Item label="操作者">
          {{ current.operatorName || '-' }} (#{{ current.operatorId || '-' }})
        </Descriptions.Item>
        <Descriptions.Item label="动作">{{ actionLabel(current.action) }}</Descriptions.Item>
        <Descriptions.Item label="对象">
          {{ targetTypeLabel(current.targetType) }} / {{ current.targetId || '-' }}
        </Descriptions.Item>
        <Descriptions.Item label="IP">{{ current.ip || '-' }}</Descriptions.Item>
        <Descriptions.Item label="User-Agent">
          {{ current.userAgent || '-' }}
        </Descriptions.Item>
        <Descriptions.Item label="摘要">{{ current.summary || '-' }}</Descriptions.Item>
      </Descriptions>
      <div v-if="current" class="json-blocks">
        <div>
          <h4>变更前</h4>
          <pre>{{ pretty(current.before) }}</pre>
        </div>
        <div>
          <h4>变更后</h4>
          <pre>{{ pretty(current.after) }}</pre>
        </div>
      </div>
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

.filter-select {
  width: 180px;
}

.operator-input {
  width: 220px;
}

.json-blocks {
  display: grid;
  gap: 16px;
  margin-top: 16px;
}

pre {
  max-height: 260px;
  padding: 12px;
  overflow: auto;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  background: hsl(var(--muted));
  border-radius: 8px;
}
</style>
