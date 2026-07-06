<script setup lang="ts">
import type { SelectValue } from 'ant-design-vue/es/select';

import type {
  PushAudienceCountResult,
  PushNotification,
} from '#/api/core/push';

import { computed, onMounted, reactive, ref, watch } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Alert,
  Button,
  Card,
  Form,
  Input,
  message,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  Textarea,
} from 'ant-design-vue';

import {
  getPushAudienceCountApi,
  getPushListApi,
  sendPushApi,
} from '#/api/core/push';
import { ellipsisColumn } from '#/components/ellipsis-tooltip/table';
import EllipsisTooltip from '#/components/ellipsis-tooltip/ellipsis-tooltip.vue';

import {
  audienceCountDetailLabel,
  buildPushAudienceCountParams,
  formatPushRecordError,
  formatPushSendAcceptedMessage,
  formatPushSendError,
  isValidPushMemberLevel,
  pushMemberLevelOptions,
  pushTemplates,
  refreshPushRecordsAfterSendAttempt,
} from './push-target';

const loading = ref(false);
const loadError = ref('');
const items = ref<PushNotification[]>([]);
const total = ref(0);
const query = reactive({ page: 1, pageSize: 20 });
const sendModalOpen = ref(false);
const sending = ref(false);
const audienceLoading = ref(false);
const audienceCount = ref<PushAudienceCountResult>();
const templateKey = ref<string>();
let requestId = 0;
let audienceRequestId = 0;
const form = reactive({
  content: '',
  deepLink: '',
  targetType: 'all',
  targetValue: '',
  title: '',
});

const targetTypeOptions = [
  { label: '全部用户', value: 'all' },
  { label: '按会员等级', value: 'level' },
];

const templateOptions = pushTemplates.map((item) => ({
  label: item.title,
  value: item.key,
}));

const selectedTemplate = computed(() =>
  pushTemplates.find((item) => item.key === templateKey.value),
);

const audienceLabel = computed(() =>
  audienceCountDetailLabel(audienceCount.value),
);

const deepLinkOptions = [
  { label: '无跳转', value: '' },
  { label: '每日练习', value: '/daily' },
  { label: '成长任务', value: '/tasks' },
  { label: '成长周报', value: '/reports' },
  { label: '关系合盘', value: '/compatibility' },
];

const columns = [
  ellipsisColumn('title', '标题', { width: 180 }),
  ellipsisColumn('content', '内容', { lines: 2 }),
  { dataIndex: 'targetType', title: '目标', width: 100 },
  { dataIndex: 'sentCount', title: '发送数', width: 80 },
  { dataIndex: 'status', title: '状态', width: 80 },
  ellipsisColumn('operator', '操作者', { width: 100 }),
  { dataIndex: 'createTime', title: '发送时间', width: 170 },
];

const statusMeta: Record<string, { color: string; text: string }> = {
  failed: { color: 'red', text: '失败' },
  pending: { color: 'default', text: '待发送' },
  sending: { color: 'processing', text: '发送中' },
  success: { color: 'green', text: '成功' },
};

function pushStatus(status: string) {
  return statusMeta[status] || { color: 'default', text: status || '未知' };
}

async function load(options: { rethrow?: boolean } = {}) {
  const currentRequestId = ++requestId;
  loading.value = true;
  loadError.value = '';
  try {
    const res = await getPushListApi(query);
    if (currentRequestId !== requestId) return;
    items.value = res?.items || [];
    total.value = res?.total || 0;
  } catch (error) {
    if (currentRequestId === requestId) {
      loadError.value = '推送记录加载失败，请稍后重试';
    }
    if (options.rethrow) throw error;
  } finally {
    if (currentRequestId === requestId) {
      loading.value = false;
    }
  }
}

function retryLoad() {
  void load();
}

function onPageChange(page: number, pageSize: number) {
  query.page = page;
  query.pageSize = pageSize;
  retryLoad();
}

function clearAudienceCount() {
  audienceRequestId++;
  audienceLoading.value = false;
  audienceCount.value = undefined;
}

function applyTemplate(key?: SelectValue) {
  const normalizedKey = typeof key === 'string' ? key : '';
  const template = pushTemplates.find((item) => item.key === normalizedKey);
  if (!template) return;
  form.title = template.title;
  form.content = template.content;
  form.deepLink = template.deepLink || '';
  form.targetType = template.targetType || 'all';
  form.targetValue = template.targetValue || '';
  clearAudienceCount();
}

async function estimateAudience() {
  const params = buildPushAudienceCountParams(form);
  if (params.targetType === 'level' && !params.targetValue) {
    message.warning('请选择有效会员等级后再预估');
    return;
  }
  const currentAudienceRequestId = ++audienceRequestId;
  audienceLoading.value = true;
  try {
    const res = await getPushAudienceCountApi(params);
    if (currentAudienceRequestId !== audienceRequestId) return;
    audienceCount.value = {
      deviceCount: Number(res?.deviceCount ?? 0),
      targetType: res?.targetType,
      targetValue: res?.targetValue,
      userCount: Number(res?.userCount ?? 0),
    };
  } catch {
    if (currentAudienceRequestId === audienceRequestId) {
      audienceCount.value = undefined;
      message.error('受众预估失败，请稍后重试');
    }
  } finally {
    if (currentAudienceRequestId === audienceRequestId) {
      audienceLoading.value = false;
    }
  }
}

function openSendModal() {
  form.title = '';
  form.content = '';
  form.targetType = 'all';
  form.targetValue = '';
  form.deepLink = '';
  templateKey.value = undefined;
  clearAudienceCount();
  sendModalOpen.value = true;
}

async function handleSend() {
  const title = form.title.trim();
  const content = form.content.trim();
  const targetValue = form.targetValue.trim();
  const deepLink = form.deepLink.trim();

  if (!title || !content) {
    message.warning('请填写标题和内容');
    return;
  }
  if (form.targetType === 'level' && !targetValue) {
    message.warning('请填写会员等级');
    return;
  }
  if (form.targetType === 'level' && !isValidPushMemberLevel(targetValue)) {
    message.warning('请选择有效会员等级');
    return;
  }
  sending.value = true;
  try {
    const res = await sendPushApi({
      content,
      deepLink: deepLink || undefined,
      targetType: form.targetType,
      targetValue: targetValue || undefined,
      title,
    });
    message.success(formatPushSendAcceptedMessage(res));
    sendModalOpen.value = false;
    void refreshPushRecordsAfterSendAttempt(
      () => load({ rethrow: true }),
      message.warning,
    );
  } catch (error) {
    message.error(formatPushSendError(error));
    void refreshPushRecordsAfterSendAttempt(
      () => load({ rethrow: true }),
      message.warning,
    );
  } finally {
    sending.value = false;
  }
}

watch(
  () => [form.targetType, form.targetValue],
  () => {
    clearAudienceCount();
  },
);

onMounted(retryLoad);
</script>

<template>
  <Page title="推送管理" description="管理和发送 App 推送通知">
    <Card>
      <template #extra>
        <Space>
          <Button :loading="loading" @click="retryLoad">刷新</Button>
          <Button type="primary" @click="openSendModal">发送推送</Button>
        </Space>
      </template>

      <Alert
        v-if="loadError"
        :message="loadError"
        show-icon
        type="error"
      >
        <template #action>
          <Button size="small" type="link" @click="retryLoad">重试</Button>
        </template>
      </Alert>

      <Table
        :columns="columns"
        :data-source="items"
        :loading="loading"
        :pagination="{
          current: query.page,
          pageSize: query.pageSize,
          total,
          showSizeChanger: true,
          onChange: onPageChange,
        }"
        row-key="id"
        :scroll="{ x: 890 }"
        size="middle"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'status'">
            <Tag :color="pushStatus(record.status).color">
              {{ pushStatus(record.status).text }}
            </Tag>
            <EllipsisTooltip
              v-if="formatPushRecordError(record)"
              class="push-error-text"
              :text="formatPushRecordError(record)"
            />
          </template>
          <template v-if="column.dataIndex === 'targetType'">
            {{
              record.targetType === 'all'
                ? '全部'
                : record.targetType === 'level'
                  ? `等级: ${record.targetValue}`
                  : record.targetType
            }}
          </template>
        </template>
      </Table>
    </Card>

    <Modal
      v-model:open="sendModalOpen"
      title="发送推送"
      :confirm-loading="sending"
      width="min(560px, calc(100vw - 32px))"
      @ok="handleSend"
    >
      <Form layout="vertical" style="margin-top: 16px">
        <Form.Item label="内置模板">
          <Select
            v-model:value="templateKey"
            allow-clear
            :options="templateOptions"
            placeholder="选择后自动填充标题、正文和跳转"
            @change="applyTemplate"
          />
          <div v-if="selectedTemplate" class="template-hint">
            {{ selectedTemplate.content }}
          </div>
        </Form.Item>
        <Form.Item label="标题" required>
          <Input
            v-model:value="form.title"
            placeholder="推送标题"
            :maxlength="40"
          />
        </Form.Item>
        <Form.Item label="内容" required>
          <Textarea
            v-model:value="form.content"
            placeholder="推送正文"
            :rows="3"
            :maxlength="200"
          />
        </Form.Item>
        <Form.Item label="推送目标">
          <Space class="target-controls">
            <Select
              v-model:value="form.targetType"
              :options="targetTypeOptions"
            />
            <Select
              v-if="form.targetType === 'level'"
              v-model:value="form.targetValue"
              :options="pushMemberLevelOptions"
              placeholder="会员等级"
            />
          </Space>
        </Form.Item>
        <Form.Item label="受众预估">
          <Space class="audience-row">
            <Tag color="processing">{{ audienceLabel }}</Tag>
            <Button
              size="small"
              :loading="audienceLoading"
              @click="estimateAudience"
            >
              {{ audienceCount === undefined ? '预估受众' : '刷新预估' }}
            </Button>
          </Space>
        </Form.Item>
        <Form.Item label="点击跳转">
          <Select
            v-model:value="form.deepLink"
            :options="deepLinkOptions"
            class="deep-link-select"
          />
        </Form.Item>
      </Form>
    </Modal>
  </Page>
</template>

<style scoped>
.target-controls {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  width: 100%;
}

.target-controls :deep(.ant-space-item),
.target-controls :deep(.ant-select),
.deep-link-select {
  width: min(220px, 100%);
}

.audience-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.template-hint {
  margin-top: 6px;
  color: hsl(var(--muted-foreground));
  font-size: 12px;
  line-height: 18px;
}

.push-error-text {
  max-width: 180px;
  margin-top: 4px;
  overflow: hidden;
  color: #d4380d;
  font-size: 12px;
  line-height: 18px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 640px) {
  .target-controls,
  .target-controls :deep(.ant-space-item),
  .target-controls :deep(.ant-select),
  .deep-link-select {
    width: 100%;
  }
}
</style>
