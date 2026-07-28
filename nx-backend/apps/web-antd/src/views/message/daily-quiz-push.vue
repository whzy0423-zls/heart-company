<script setup lang="ts">
import type {
  DailyQuizPushRecord,
  DailyQuizPushStats,
  PushAudienceCountResult,
} from '#/api/core/push';

import dayjs from 'dayjs';
import { computed, onMounted, reactive, ref, watch } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Alert,
  Button,
  Card,
  Col,
  Form,
  Input,
  message,
  Modal,
  Row,
  Select,
  Space,
  Statistic,
  Table,
  Tag,
  Textarea,
} from 'ant-design-vue';

import {
  getDailyQuizPushRecordsApi,
  getDailyQuizPushStatsApi,
  getPushAudienceCountApi,
  sendPushApi,
} from '#/api/core/push';
import { ellipsisColumn } from '#/components/ellipsis-tooltip/table';

import {
  audienceCountDetailLabel,
  buildPushAudienceCountParams,
  formatNoPushAudienceMessage,
  formatPushSendAcceptedMessage,
  formatPushSendError,
  isValidPushMemberLevel,
  pushMemberLevelOptions,
  refreshPushRecordsAfterSendAttempt,
} from './push-target';

const loading = ref(false);
const loadError = ref('');
const records = ref<DailyQuizPushRecord[]>([]);
const total = ref(0);
const stats = ref<DailyQuizPushStats>();
const testModalOpen = ref(false);
const sending = ref(false);
const audienceLoading = ref(false);
const audienceCount = ref<PushAudienceCountResult>();
let requestId = 0;
let audienceRequestId = 0;

const query = reactive({
  date: dayjs().format('YYYY-MM-DD'),
  page: 1,
  pageSize: 20,
});

const testPushForm = reactive({
  content: '今天 5 道题等你完成，花 1 分钟让系统更懂你。',
  deepLink: '/daily-quiz',
  targetType: 'all',
  targetValue: '',
  title: '今日画像校准题已准备好',
});

const targetTypeOptions = [
  { label: '全部用户', value: 'all' },
  { label: '按会员等级', value: 'level' },
];

const columns = [
  { dataIndex: 'quizDate', title: '题目日期', width: 120 },
  ellipsisColumn('nickname', '用户昵称', { width: 140 }),
  { dataIndex: 'phone', title: '手机号', width: 140 },
  ellipsisColumn('cardName', '画像卡片', { width: 160 }),
  { dataIndex: 'batchId', title: '批次', width: 80 },
  { dataIndex: 'pushed', title: '是否推送', width: 100 },
  { dataIndex: 'answeredCount', title: '答题数', width: 90 },
  { dataIndex: 'completed', title: '完成状态', width: 110 },
  { dataIndex: 'pushSentAt', title: '推送时间', width: 170 },
  { dataIndex: 'completedAt', title: '完成时间', width: 170 },
];

const safeStats = computed<DailyQuizPushStats>(() => ({
  answeredUsers: stats.value?.answeredUsers ?? 0,
  completedUsers: stats.value?.completedUsers ?? 0,
  date: stats.value?.date || query.date,
  eligibleUsers: stats.value?.eligibleUsers ?? 0,
  pendingReassessmentReports: stats.value?.pendingReassessmentReports ?? 0,
  pushed: Boolean(stats.value?.pushed || (stats.value?.pushedUsers ?? 0) > 0),
  pushedUsers: stats.value?.pushedUsers ?? 0,
  totalAnswers: stats.value?.totalAnswers ?? 0,
}));

const audienceLabel = computed(() =>
  audienceCountDetailLabel(audienceCount.value),
);

async function load(options: { rethrow?: boolean } = {}) {
  const currentRequestId = ++requestId;
  loading.value = true;
  loadError.value = '';
  try {
    const params = {
      date: query.date,
      page: query.page,
      pageSize: query.pageSize,
    };
    const [statsRes, recordsRes] = await Promise.all([
      getDailyQuizPushStatsApi({ date: query.date }),
      getDailyQuizPushRecordsApi(params),
    ]);
    if (currentRequestId !== requestId) return;
    stats.value = statsRes;
    records.value = recordsRes?.items || [];
    total.value = recordsRes?.total || 0;
  } catch (error) {
    if (currentRequestId === requestId) {
      loadError.value = '每日题推送记录加载失败，请稍后重试';
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

function onDateInput(event: Event) {
  const value = (event.target as HTMLInputElement).value;
  if (!value) return;
  query.date = value;
  query.page = 1;
  retryLoad();
}

function onPageChange(page: number, pageSize: number) {
  query.page = page;
  query.pageSize = pageSize;
  retryLoad();
}

function pushTag(record: { pushed?: boolean }) {
  return record.pushed
    ? { color: 'green', text: '已推送' }
    : { color: 'default', text: '未推送' };
}

function completeTag(record: { answeredCount?: number; completed?: boolean }) {
  const answeredCount = Number(record.answeredCount ?? 0);
  if (record.completed || answeredCount >= 5) {
    return { color: 'green', text: '已完成' };
  }
  if (answeredCount > 0) {
    return { color: 'processing', text: '答题中' };
  }
  return { color: 'default', text: '未答题' };
}

function clearAudienceCount() {
  audienceRequestId++;
  audienceLoading.value = false;
  audienceCount.value = undefined;
}

function resetTestPushForm() {
  testPushForm.title = '今日画像校准题已准备好';
  testPushForm.content = '今天 5 道题等你完成，花 1 分钟让系统更懂你。';
  testPushForm.deepLink = '/daily-quiz';
  testPushForm.targetType = 'all';
  testPushForm.targetValue = '';
  clearAudienceCount();
}

function openTestPushModal() {
  resetTestPushForm();
  testModalOpen.value = true;
}

function normalizeAudienceResult(res?: PushAudienceCountResult) {
  return {
    deviceCount: Number(res?.deviceCount ?? 0),
    targetType: res?.targetType,
    targetValue: res?.targetValue,
    userCount: Number(res?.userCount ?? 0),
  };
}

async function refreshAudienceCountForCurrentTarget() {
  const params = buildPushAudienceCountParams(testPushForm);
  if (params.targetType === 'level' && !params.targetValue) {
    message.warning('请选择有效会员等级后再预估');
    return undefined;
  }
  const currentAudienceRequestId = ++audienceRequestId;
  audienceLoading.value = true;
  try {
    const res = await getPushAudienceCountApi(params);
    if (currentAudienceRequestId !== audienceRequestId) return undefined;
    audienceCount.value = normalizeAudienceResult(res);
    return audienceCount.value;
  } catch {
    if (currentAudienceRequestId === audienceRequestId) {
      audienceCount.value = undefined;
      message.error('受众预估失败，请稍后重试');
    }
    return undefined;
  } finally {
    if (currentAudienceRequestId === audienceRequestId) {
      audienceLoading.value = false;
    }
  }
}

async function estimateAudience() {
  await refreshAudienceCountForCurrentTarget();
}

async function sendTestPush() {
  const title = testPushForm.title.trim();
  const content = testPushForm.content.trim();
  const targetValue = testPushForm.targetValue.trim();
  const deepLink = testPushForm.deepLink.trim();
  if (!title || !content) {
    message.warning('请填写标题和内容');
    return;
  }
  if (testPushForm.targetType === 'level' && !targetValue) {
    message.warning('请填写会员等级');
    return;
  }
  if (
    testPushForm.targetType === 'level' &&
    !isValidPushMemberLevel(targetValue)
  ) {
    message.warning('请选择有效会员等级');
    return;
  }

  const latestAudience = await refreshAudienceCountForCurrentTarget();
  const noAudienceMessage = formatNoPushAudienceMessage(latestAudience);
  if (noAudienceMessage) {
    message.warning(noAudienceMessage);
    return;
  }
  if (!latestAudience) return;

  sending.value = true;
  try {
    const res = await sendPushApi({
      content,
      deepLink: deepLink || undefined,
      targetType: testPushForm.targetType,
      targetValue: targetValue || undefined,
      title,
    });
    message.success(formatPushSendAcceptedMessage(res));
    testModalOpen.value = false;
    void refreshPushRecordsAfterSendAttempt(
      () => load({ rethrow: true }),
      message.warning,
    );
  } catch (error) {
    message.error(formatPushSendError(error));
  } finally {
    sending.value = false;
  }
}

watch(
  () => [testPushForm.targetType, testPushForm.targetValue],
  () => {
    clearAudienceCount();
  },
);

onMounted(retryLoad);
</script>

<template>
  <Page title="每日题推送记录" description="查看每日画像校准题是否推送，以及用户答题完成情况">
    <Card>
      <template #extra>
        <Space>
          <input
            class="date-input"
            type="date"
            :value="query.date"
            @change="onDateInput"
          />
          <Button :loading="loading" @click="retryLoad">刷新</Button>
          <Button type="primary" @click="openTestPushModal">测试推送</Button>
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

      <Row :gutter="[12, 12]" class="stats-row">
        <Col :lg="4" :md="8" :sm="12" :xs="24">
          <Card size="small">
            <Statistic title="今日是否推送" :value="safeStats.pushed ? '已推送' : '未推送'" />
          </Card>
        </Col>
        <Col :lg="4" :md="8" :sm="12" :xs="24">
          <Card size="small">
            <Statistic title="应推用户" :value="safeStats.eligibleUsers" />
          </Card>
        </Col>
        <Col :lg="4" :md="8" :sm="12" :xs="24">
          <Card size="small">
            <Statistic title="已推送用户" :value="safeStats.pushedUsers" />
          </Card>
        </Col>
        <Col :lg="4" :md="8" :sm="12" :xs="24">
          <Card size="small">
            <Statistic title="已答题用户" :value="safeStats.answeredUsers" />
          </Card>
        </Col>
        <Col :lg="4" :md="8" :sm="12" :xs="24">
          <Card size="small">
            <Statistic title="完成 5/5 用户" :value="safeStats.completedUsers" />
          </Card>
        </Col>
        <Col :lg="4" :md="8" :sm="12" :xs="24">
          <Card size="small">
            <Statistic title="总答题数" :value="safeStats.totalAnswers" />
          </Card>
        </Col>
      </Row>

      <Alert
        class="summary-alert"
        :message="`待查看复评报告：${safeStats.pendingReassessmentReports} 个`"
        show-icon
        type="info"
      />

      <Table
        :columns="columns"
        :data-source="records"
        :loading="loading"
        :pagination="{
          current: query.page,
          pageSize: query.pageSize,
          total,
          showSizeChanger: true,
          onChange: onPageChange,
        }"
        row-key="batchId"
        :scroll="{ x: 1280 }"
        size="middle"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'pushed'">
            <Tag :color="pushTag(record).color">
              {{ pushTag(record).text }}
            </Tag>
          </template>
          <template v-if="column.dataIndex === 'completed'">
            <Tag :color="completeTag(record).color">
              {{ completeTag(record).text }}
            </Tag>
          </template>
          <template v-if="column.dataIndex === 'answeredCount'">
            {{ record.answeredCount }}/5
          </template>
        </template>
      </Table>
    </Card>

    <Modal
      v-if="testModalOpen"
      v-model:open="testModalOpen"
      title="发送每日题测试推送"
      :confirm-loading="sending"
      width="min(560px, calc(100vw - 32px))"
      @ok="sendTestPush"
    >
      <Alert
        message="发送每日题测试推送"
        description="用于临时验证每日画像校准题推送链路，默认跳转 /daily-quiz。"
        show-icon
        type="info"
      />
      <Form layout="vertical" style="margin-top: 16px">
        <Form.Item label="标题" required>
          <Input
            v-model:value="testPushForm.title"
            placeholder="推送标题"
            :maxlength="40"
          />
        </Form.Item>
        <Form.Item label="内容" required>
          <Textarea
            v-model:value="testPushForm.content"
            placeholder="推送正文"
            :rows="3"
            :maxlength="200"
          />
        </Form.Item>
        <Form.Item label="跳转地址">
          <Input v-model:value="testPushForm.deepLink" placeholder="/daily-quiz" />
          <div class="template-hint">
            {{ testPushForm.title }} · {{ testPushForm.deepLink }}
          </div>
        </Form.Item>
        <Form.Item label="推送目标">
          <Space class="target-controls">
            <Select
              v-model:value="testPushForm.targetType"
              :options="targetTypeOptions"
             placeholder="请选择推送目标"/>
            <Select
              v-if="testPushForm.targetType === 'level'"
              v-model:value="testPushForm.targetValue"
              :options="pushMemberLevelOptions"
              placeholder="选择会员等级"
              style="min-width: 160px"
            />
          </Space>
        </Form.Item>
        <Form.Item label="受众预估">
          <Space>
            <Button :loading="audienceLoading" @click="estimateAudience">
              预估受众
            </Button>
            <span>{{ audienceLabel }}</span>
          </Space>
        </Form.Item>
      </Form>
    </Modal>
  </Page>
</template>

<style scoped>
.date-input {
  min-height: 32px;
  padding: 4px 11px;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
}

.stats-row {
  margin-bottom: 12px;
}

.summary-alert {
  margin-bottom: 12px;
}

.target-controls {
  display: flex;
  width: 100%;
}

.template-hint {
  margin-top: 6px;
  color: #667085;
  font-size: 12px;
}
</style>
