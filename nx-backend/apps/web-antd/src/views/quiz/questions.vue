<script setup lang="ts">
import type { QuizOption, QuizQuestion } from '#/api';

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
  createQuizQuestionApi,
  deleteQuizQuestionApi,
  getQuizQuestionsApi,
  updateQuizQuestionApi,
} from '#/api';
import { ellipsisColumn } from '#/components/ellipsis-tooltip/table';

import { parseAndValidateQuizOptions } from './question-options';

const loading = ref(false);
const saving = ref(false);
const drawerOpen = ref(false);
const questions = ref<QuizQuestion[]>([]);
const form = reactive({
  body: '',
  dimension: '',
  id: 0,
  optionsText: '',
  sort: 0,
  status: 'enabled',
});

const statusOptions = [
  { label: '启用', value: 'enabled' },
  { label: '停用', value: 'disabled' },
];

const columns = [
  { dataIndex: 'sort', title: '排序', width: 90 },
  ellipsisColumn('body', '题目', { lines: 2 }),
  { dataIndex: 'dimension', title: '维度', width: 120 },
  { dataIndex: 'options', title: '选项', width: 100 },
  { dataIndex: 'quizVersion', title: '版本', width: 100 },
  { dataIndex: 'status', title: '状态', width: 100 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 170 },
];

function questionRecord(record: Record<string, any>) {
  return record as QuizQuestion;
}

function prettyOptions(options: QuizOption[]) {
  return JSON.stringify(options || [], null, 2);
}

function resetForm() {
  form.body = '';
  form.dimension = '';
  form.id = 0;
  form.optionsText = prettyOptions([
    { id: 'a', text: '', weights: {} },
    { id: 'b', text: '', weights: {} },
  ]);
  form.sort = questions.value.length + 1;
  form.status = 'enabled';
}

function openCreate() {
  resetForm();
  drawerOpen.value = true;
}

function openEdit(record: QuizQuestion) {
  form.body = record.body || '';
  form.dimension = record.dimension || '';
  form.id = record.id;
  form.optionsText = prettyOptions(record.options || []);
  form.sort = Number(record.sort || 0);
  form.status = record.status || 'enabled';
  drawerOpen.value = true;
}

async function load() {
  loading.value = true;
  try {
    questions.value = await getQuizQuestionsApi();
  } finally {
    loading.value = false;
  }
}

async function submit() {
  if (!form.body.trim()) {
    message.warning('请填写题目内容');
    return;
  }
  let options: QuizOption[] = [];
  try {
    options = parseAndValidateQuizOptions(form.optionsText, form.status);
  } catch (error) {
    message.error(error instanceof Error ? error.message : '选项 JSON 格式不正确');
    return;
  }
  saving.value = true;
  try {
    const payload = {
      body: form.body.trim(),
      dimension: form.dimension.trim(),
      options,
      sort: Number(form.sort || 0),
      status: form.status,
    };
    if (form.id) {
      await updateQuizQuestionApi(form.id, payload);
      message.success('题目已更新');
    } else {
      await createQuizQuestionApi(payload);
      message.success('题目已新增');
    }
    drawerOpen.value = false;
    await load();
  } finally {
    saving.value = false;
  }
}

function removeQuestion(record: QuizQuestion) {
  Modal.confirm({
    content: `确认停用题目「${record.body}」吗？停用后 App 测评不再下发该题。`,
    okType: 'danger',
    onOk: async () => {
      await deleteQuizQuestionApi(record.id);
      message.success('题目已停用');
      await load();
    },
    title: '停用测评题目',
  });
}

function statusColor(status: string) {
  return status === 'enabled' ? 'success' : 'default';
}

function statusLabel(status: string) {
  return status === 'enabled' ? '启用' : '停用';
}

onMounted(load);
</script>

<template>
  <Page
    description="维护 App 九型测评题库。选项 weights 为服务端算分权重，请谨慎调整。"
    title="测评题库"
  >
    <Card :bordered="false" class="quiz-card">
      <div class="table-head">
        <div>
          <div class="card-title">测评题目</div>
          <div class="card-desc">共 {{ questions.length }} 道题，停用题目不会下发给 App。</div>
        </div>
        <Space wrap>
          <Button :loading="loading" @click="load">刷新</Button>
          <Button type="primary" @click="openCreate">新增题目</Button>
        </Space>
      </div>

      <Table
        :columns="columns"
        :data-source="questions"
        :loading="loading"
        :pagination="{ pageSize: 20, showSizeChanger: true }"
        :scroll="{ x: 1100 }"
        row-key="id"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'options'">
            {{ record.options?.length || 0 }} 个
          </template>
          <template v-else-if="column.dataIndex === 'status'">
            <Tag :color="statusColor(record.status)">{{ statusLabel(record.status) }}</Tag>
          </template>
          <template v-else-if="column.key === 'action'">
            <Space :size="4">
              <Button size="small" type="link" @click="openEdit(questionRecord(record))">
                编辑
              </Button>
              <Button danger size="small" type="link" @click="removeQuestion(questionRecord(record))">
                停用
              </Button>
            </Space>
          </template>
        </template>
      </Table>
    </Card>

    <Drawer
      v-model:open="drawerOpen"
      :title="form.id ? '编辑题目' : '新增题目'"
      width="min(760px, calc(100vw - 32px))"
    >
      <Form layout="vertical">
        <Form.Item label="题目内容" required>
          <Input.TextArea v-model:value="form.body" :rows="3" placeholder="请输入测评题目" />
        </Form.Item>
        <Space align="start" wrap>
          <Form.Item label="排序">
            <InputNumber v-model:value="form.sort" :min="0" class="number-input"  placeholder="请输入排序"/>
          </Form.Item>
          <Form.Item label="维度">
            <Input v-model:value="form.dimension" class="dimension-input" placeholder="如 core" />
          </Form.Item>
          <Form.Item label="状态">
            <Select v-model:value="form.status" :options="statusOptions" class="status-input"  placeholder="请选择状态"/>
          </Form.Item>
        </Space>
        <Form.Item label="选项 JSON" required>
          <Input.TextArea
            v-model:value="form.optionsText"
            :rows="14"
            class="json-editor"
            placeholder='[{"id":"a","text":"选项","weights":{"1":2}}]'
          />
        </Form.Item>
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
.quiz-card {
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
  color: hsl(var(--muted-foreground));
}

.number-input,
.status-input {
  width: 120px;
}

.dimension-input {
  width: 180px;
}

.json-editor {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.drawer-footer {
  display: flex;
  gap: 8px;
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
}
</style>
