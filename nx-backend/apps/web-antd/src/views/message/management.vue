<script setup lang="ts">
import type { SystemMessage } from '#/api';

import { onMounted, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';

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
  message,
} from 'ant-design-vue';

import { getMessageListApi, markMessagesApi } from '#/api';

const router = useRouter();

const loading = ref(false);
const messages = ref<SystemMessage[]>([]);
const total = ref(0);
const detailOpen = ref(false);
const current = ref<SystemMessage>();
const query = reactive({
  keyword: '',
  page: 1,
  pageSize: 20,
  read: '',
  type: 'signup',
});

const messageTypeOptions = [
  { label: '全部类型', value: '' },
  { label: '报名信息', value: 'signup' },
];

const readOptions = [
  { label: '全部状态', value: '' },
  { label: '未读', value: 'false' },
  { label: '已读', value: 'true' },
];

const columns = [
  { dataIndex: 'type', title: '消息类型', width: 120 },
  { dataIndex: 'title', title: '消息标题', width: 240 },
  { dataIndex: 'content', ellipsis: true, title: '消息内容' },
  { dataIndex: 'isRead', title: '状态', width: 100 },
  { dataIndex: 'businessType', title: '关联业务', width: 130 },
  { dataIndex: 'createTime', title: '创建时间', width: 180 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 190 },
];

async function load() {
  loading.value = true;
  try {
    const result = await getMessageListApi({
      keyword: query.keyword,
      page: query.page,
      pageSize: query.pageSize,
      read: query.read || undefined,
      type: query.type || undefined,
    });
    messages.value = result.items;
    total.value = result.total;
  } finally {
    loading.value = false;
  }
}

function typeLabel(type?: string) {
  if (type === 'signup') return '报名信息';
  return type || '系统消息';
}

function businessLabel(type?: string) {
  if (type === 'signup') return '报名线索';
  return type || '-';
}

async function setRead(record: SystemMessage, read: boolean) {
  await markMessagesApi({ ids: [record.id], read });
  message.success(read ? '已标记为已读' : '已标记为未读');
  await load();
}

async function markAllRead() {
  await markMessagesApi({ read: true });
  message.success('全部消息已标记为已读');
  await load();
}

function openDetail(record: SystemMessage) {
  current.value = record;
  detailOpen.value = true;
}

async function goTarget(record: SystemMessage) {
  if (!record.isRead) {
    await markMessagesApi({ ids: [record.id], read: true });
  }
  if (record.targetPath) {
    await router.push(record.targetPath);
  }
}

function handleTableChange(pagination: { current?: number; pageSize?: number }) {
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
  <Page
    description="集中查看系统消息，可按类型、已读状态和关键词筛选。"
    title="消息管理"
  >
    <div class="message-page">
      <Card :bordered="false">
        <div class="card-header">
          <div>
            <div class="card-title">消息列表</div>
            <div class="card-desc">
              当前共 {{ total }} 条消息，后续可继续扩展更多业务类型。
            </div>
          </div>
          <Space wrap>
            <Button :loading="loading" @click="load">刷新</Button>
            <Button type="primary" @click="markAllRead">全部已读</Button>
          </Space>
        </div>

        <div class="toolbar">
          <Select
            v-model:value="query.type"
            :options="messageTypeOptions"
            class="type-select"
          />
          <Select
            v-model:value="query.read"
            :options="readOptions"
            class="read-select"
          />
          <Input
            v-model:value="query.keyword"
            allow-clear
            class="keyword-input"
            placeholder="搜索标题 / 内容"
            @press-enter="search"
          />
          <Button type="primary" @click="search">查询</Button>
        </div>

        <Table
          :columns="columns"
          :data-source="messages"
          :loading="loading"
          :pagination="{
            current: query.page,
            pageSize: query.pageSize,
            showSizeChanger: true,
            total,
          }"
          :scroll="{ x: 1120 }"
          row-key="id"
          table-layout="fixed"
          @change="handleTableChange"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'type'">
              <Tag color="blue">{{ typeLabel(record.type) }}</Tag>
            </template>
            <template v-if="column.dataIndex === 'title'">
              <div class="message-title">{{ record.title || '-' }}</div>
            </template>
            <template v-if="column.dataIndex === 'content'">
              <div class="message-content">{{ record.content || '-' }}</div>
            </template>
            <template v-if="column.dataIndex === 'isRead'">
              <Tag :color="record.isRead ? 'default' : 'red'">
                {{ record.isRead ? '已读' : '未读' }}
              </Tag>
            </template>
            <template v-if="column.dataIndex === 'businessType'">
              {{ businessLabel(record.businessType) }}
            </template>
            <template v-if="column.key === 'action'">
              <Space :size="4">
                <Button size="small" type="link" @click="openDetail(record)">
                  详情
                </Button>
                <Button
                  size="small"
                  type="link"
                  @click="setRead(record, !record.isRead)"
                >
                  {{ record.isRead ? '未读' : '已读' }}
                </Button>
                <Button size="small" type="link" @click="goTarget(record)">
                  查看业务
                </Button>
              </Space>
            </template>
          </template>
        </Table>
      </Card>
    </div>

    <Drawer v-model:open="detailOpen" title="消息详情" width="560px">
      <Descriptions v-if="current" :column="1" bordered size="small">
        <Descriptions.Item label="消息类型">
          {{ typeLabel(current.type) }}
        </Descriptions.Item>
        <Descriptions.Item label="消息标题">
          {{ current.title || '-' }}
        </Descriptions.Item>
        <Descriptions.Item label="消息内容">
          <div class="detail-text">{{ current.content || '-' }}</div>
        </Descriptions.Item>
        <Descriptions.Item label="状态">
          {{ current.isRead ? '已读' : '未读' }}
        </Descriptions.Item>
        <Descriptions.Item label="关联业务">
          {{ businessLabel(current.businessType) }}
        </Descriptions.Item>
        <Descriptions.Item label="业务 ID">
          {{ current.businessId || '-' }}
        </Descriptions.Item>
        <Descriptions.Item label="创建时间">
          {{ current.createTime }}
        </Descriptions.Item>
      </Descriptions>
    </Drawer>
  </Page>
</template>

<style scoped>
.message-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.card-header {
  align-items: flex-start;
  display: flex;
  gap: 16px;
  justify-content: space-between;
  margin-bottom: 16px;
}

.card-title {
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
}

.card-desc {
  color: hsl(var(--muted-foreground));
  font-size: 13px;
  line-height: 20px;
  margin-top: 4px;
}

.toolbar {
  display: grid;
  gap: 8px;
  grid-template-columns: 150px 130px minmax(220px, 420px) auto;
  justify-content: start;
  margin-bottom: 16px;
}

.type-select,
.read-select,
.keyword-input {
  width: 100%;
}

.message-title,
.message-content {
  line-height: 20px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.message-title {
  display: -webkit-box;
  font-weight: 600;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.message-content {
  white-space: nowrap;
}

.detail-text {
  white-space: pre-wrap;
  word-break: break-word;
}

@media (max-width: 768px) {
  .card-header {
    align-items: stretch;
    flex-direction: column;
  }

  .toolbar {
    grid-template-columns: 1fr;
  }
}
</style>
