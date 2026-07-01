<script setup lang="ts">
import type { RAGDocument } from '#/api';

import { onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Button,
  Card,
  Drawer,
  Form,
  Input,
  InputNumber,
  message,
  Modal,
  Select,
  Space,
  Table,
  Tag,
} from 'ant-design-vue';

import {
  createRAGDocumentApi,
  deleteRAGDocumentApi,
  getRAGDocumentsApi,
  updateRAGDocumentApi,
} from '#/api';

const loading = ref(false);
const saving = ref(false);
const drawerOpen = ref(false);
const documents = ref<RAGDocument[]>([]);
const total = ref(0);

const query = reactive({
  keyword: '',
  page: 1,
  pageSize: 20,
  status: '',
});

const form = reactive({
  content: '',
  id: '',
  sort: 0,
  status: 'enabled',
  tagsText: '',
  title: '',
});

const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '启用', value: 'enabled' },
  { label: '停用', value: 'disabled' },
];

const editStatusOptions = [
  { label: '启用', value: 'enabled' },
  { label: '停用', value: 'disabled' },
];

const columns = [
  { dataIndex: 'title', title: '标题', width: 220 },
  { dataIndex: 'status', title: '状态', width: 100 },
  { dataIndex: 'tags', title: '标签', width: 260 },
  { dataIndex: 'content', ellipsis: true, title: '内容摘要' },
  { dataIndex: 'sort', title: '排序', width: 90 },
  { dataIndex: 'updateTime', title: '更新时间', width: 180 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 220 },
];

async function load() {
  loading.value = true;
  try {
    const result = await getRAGDocumentsApi({
      keyword: query.keyword,
      page: query.page,
      pageSize: query.pageSize,
      status: query.status || undefined,
    });
    documents.value = result.items;
    total.value = result.total;
  } finally {
    loading.value = false;
  }
}

function tagsFromText(text: string) {
  return text
    .split(/[,，\n]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function resetForm() {
  form.content = '';
  form.id = '';
  form.sort = 0;
  form.status = 'enabled';
  form.tagsText = '';
  form.title = '';
}

function openCreate() {
  resetForm();
  drawerOpen.value = true;
}

function openEdit(record: RAGDocument) {
  form.content = record.content || '';
  form.id = record.id;
  form.sort = Number(record.sort || 0);
  form.status = record.status || 'enabled';
  form.tagsText = (record.tags || []).join('\n');
  form.title = record.title || '';
  drawerOpen.value = true;
}

function asRAGDocument(record: Record<string, any>) {
  return record as RAGDocument;
}

async function submit() {
  if (!form.title.trim() || !form.content.trim()) {
    message.warning('请填写标题和知识内容');
    return;
  }
  saving.value = true;
  try {
    const payload = {
      content: form.content,
      sort: form.sort,
      source: 'manual',
      status: form.status,
      tags: tagsFromText(form.tagsText),
      title: form.title,
    };
    if (form.id) {
      await updateRAGDocumentApi(form.id, payload);
      message.success('知识文档已更新');
    } else {
      await createRAGDocumentApi(payload);
      message.success('知识文档已新增');
    }
    drawerOpen.value = false;
    await load();
  } finally {
    saving.value = false;
  }
}

async function toggleStatus(record: RAGDocument) {
  const nextStatus = record.status === 'enabled' ? 'disabled' : 'enabled';
  await updateRAGDocumentApi(record.id, {
    content: record.content,
    sort: record.sort,
    source: record.source || 'manual',
    status: nextStatus,
    tags: record.tags || [],
    title: record.title,
  });
  message.success(nextStatus === 'enabled' ? '已启用' : '已停用');
  await load();
}

function removeDocument(record: RAGDocument) {
  Modal.confirm({
    content: `确认删除「${record.title}」吗？删除后小程序 RAG 不会再检索这条资料。`,
    onOk: async () => {
      await deleteRAGDocumentApi(record.id);
      message.success('已删除');
      await load();
    },
    title: '删除知识文档',
  });
}

function statusColor(status: string) {
  return status === 'enabled' ? 'success' : 'default';
}

function statusLabel(status: string) {
  return status === 'enabled' ? '启用' : '停用';
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

onMounted(load);
</script>

<template>
  <Page
    description="维护小程序 AI 对话会检索的补充知识。启用后的文档会进入 RAG 检索。"
    title="知识库管理"
  >
    <Card :bordered="false" class="knowledge-card">
      <div class="table-head">
        <div>
          <div class="card-title">RAG 知识文档</div>
          <div class="card-desc">
            共 {{ total }} 条资料。建议每条聚焦一个主题，方便小程序问答命中。
          </div>
        </div>
        <Space wrap>
          <Select
            v-model:value="query.status"
            :options="statusOptions"
            class="status-select"
          />
          <Input
            v-model:value="query.keyword"
            allow-clear
            class="keyword-input"
            placeholder="搜索标题 / 内容"
            @press-enter="search"
          />
          <Button type="primary" @click="search">查询</Button>
          <Button :loading="loading" @click="load">刷新</Button>
          <Button type="primary" @click="openCreate">新增知识</Button>
        </Space>
      </div>

      <Table
        :columns="columns"
        :data-source="documents"
        :loading="loading"
        :pagination="{
          current: query.page,
          pageSize: query.pageSize,
          showSizeChanger: true,
          total,
        }"
        :scroll="{ x: 1280 }"
        row-key="id"
        table-layout="fixed"
        @change="handleTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'status'">
            <Tag :color="statusColor(record.status)">
              {{ statusLabel(record.status) }}
            </Tag>
          </template>
          <template v-else-if="column.dataIndex === 'tags'">
            <Space :size="4" wrap>
              <Tag v-for="tag in record.tags" :key="tag" color="blue">
                {{ tag }}
              </Tag>
              <span v-if="!record.tags?.length">-</span>
            </Space>
          </template>
          <template v-else-if="column.dataIndex === 'content'">
            <div class="content-preview">{{ record.content || '-' }}</div>
          </template>
          <template v-else-if="column.key === 'action'">
            <Space :size="4">
              <Button
                size="small"
                type="link"
                @click="openEdit(asRAGDocument(record))"
              >
                编辑
              </Button>
              <Button
                size="small"
                type="link"
                @click="toggleStatus(asRAGDocument(record))"
              >
                {{ record.status === 'enabled' ? '停用' : '启用' }}
              </Button>
              <Button
                danger
                size="small"
                type="link"
                @click="removeDocument(asRAGDocument(record))"
              >
                删除
              </Button>
            </Space>
          </template>
        </template>
      </Table>
    </Card>

    <Drawer
      v-model:open="drawerOpen"
      :title="form.id ? '编辑知识' : '新增知识'"
      class="knowledge-drawer"
      width="720px"
    >
      <Form layout="vertical">
        <Form.Item label="标题" required>
          <Input
            v-model:value="form.title"
            placeholder="例如：企业沟通课适用场景"
          />
        </Form.Item>
        <Form.Item label="知识内容" required>
          <Input.TextArea
            v-model:value="form.content"
            :rows="10"
            placeholder="输入会被小程序 RAG 检索的知识正文"
          />
        </Form.Item>
        <Form.Item label="标签">
          <Input.TextArea
            v-model:value="form.tagsText"
            :rows="3"
            placeholder="一行一个标签，或用逗号分隔"
          />
        </Form.Item>
        <Space align="start">
          <Form.Item label="状态">
            <Select
              v-model:value="form.status"
              :options="editStatusOptions"
              class="drawer-select"
            />
          </Form.Item>
          <Form.Item label="排序">
            <InputNumber
              v-model:value="form.sort"
              :min="0"
              class="sort-input"
            />
          </Form.Item>
        </Space>
      </Form>
      <template #footer>
        <div class="drawer-footer">
          <Button @click="drawerOpen = false">取消</Button>
          <Button :loading="saving" type="primary" @click="submit">保存</Button>
        </div>
      </template>
    </Drawer>
  </Page>
</template>

<style scoped>
.knowledge-card {
  border-radius: 8px;
}

.table-head {
  display: flex;
  gap: 16px;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 16px;
}

.card-title {
  font-size: 16px;
  font-weight: 600;
}

.card-desc {
  margin-top: 4px;
  font-size: 13px;
  color: #667085;
}

.keyword-input {
  width: 220px;
}

.status-select,
.drawer-select {
  width: 120px;
}

.sort-input {
  width: 120px;
}

.content-preview {
  display: -webkit-box;
  overflow: hidden;
  -webkit-line-clamp: 2;
  color: #344054;
  white-space: normal;
  -webkit-box-orient: vertical;
}

.knowledge-drawer :deep(.ant-drawer-footer) {
  padding: 12px 24px;
}

.drawer-footer {
  display: flex;
  gap: 8px;
  align-items: center;
  justify-content: flex-end;
}

@media (max-width: 768px) {
  .table-head {
    display: block;
  }

  .table-head :deep(.ant-space) {
    width: 100%;
    margin-top: 12px;
  }

  .keyword-input,
  .status-select {
    width: 100%;
  }
}
</style>
