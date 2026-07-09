<script setup lang="ts">
import type {
  DailyQuizOption,
  DailyQuizQuestionVersion,
  DailyQuizSet,
} from '#/api/core/daily-quiz';

import dayjs from 'dayjs';
import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Alert,
  Button,
  Card,
  Col,
  Empty,
  Form,
  Input,
  message,
  Modal,
  Row,
  Space,
  Statistic,
  Tag,
  Textarea,
} from 'ant-design-vue';

import {
  generateDailyQuizSetApi,
  getDailyQuizSetApi,
  getTodayDailyQuizSetApi,
  replaceDailyQuizQuestionApi,
} from '#/api/core/daily-quiz';

const DAILY_QUIZ_SLOT_COUNT = 5;

const loading = ref(false);
const generating = ref(false);
const replacing = ref(false);
const loadError = ref('');
const quizSet = ref<DailyQuizSet>();
const selectedDate = ref(dayjs().format('YYYY-MM-DD'));
const replaceModalOpen = ref(false);
const replacingQuestion = ref<DailyQuizQuestionVersion>();
let requestId = 0;

const replaceForm = reactive({
  reason: '',
});

const isSelectedToday = computed(
  () => selectedDate.value === dayjs().format('YYYY-MM-DD'),
);

const generateButtonText = computed(() =>
  isSelectedToday.value ? '生成今日题目' : '生成当前日期题目',
);

const activeQuestions = computed(() => {
  const slots: Array<DailyQuizQuestionVersion | undefined> = Array.from({
    length: DAILY_QUIZ_SLOT_COUNT,
  });
  const ordered = [...(quizSet.value?.questions ?? [])].sort((a, b) => {
    if (a.slotNo !== b.slotNo) return a.slotNo - b.slotNo;
    if (a.isActive !== b.isActive) return a.isActive ? -1 : 1;
    return b.versionNo - a.versionNo;
  });
  for (const question of ordered) {
    const index = question.slotNo - 1;
    if (index < 0 || index >= DAILY_QUIZ_SLOT_COUNT) continue;
    if (!slots[index] || question.isActive) {
      slots[index] = question;
    }
  }
  return slots.map((question, index) => ({
    question,
    slotNo: index + 1,
  }));
});

const hasAnyAnswered = computed(() =>
  (quizSet.value?.questions ?? []).some(
    (question) => Number(question.answeredCount ?? 0) > 0,
  ),
);

const generatedStatusText = computed(() => quizSet.value?.status || '未生成');
const sourceText = computed(() => formatSource(quizSet.value?.source));
const modelText = computed(() => {
  const provider = quizSet.value?.modelProvider || '—';
  const name = quizSet.value?.modelName || '—';
  if (provider === '—' && name === '—') return '—';
  return `${provider} / ${name}`;
});
const questionCountText = computed(
  () => `${activeQuestions.value.filter((item) => item.question).length}/${DAILY_QUIZ_SLOT_COUNT}`,
);
const pushedText = computed(() => (quizSet.value?.pushedAt ? '已推送' : '未推送'));

function setQuizSet(nextSet?: DailyQuizSet) {
  quizSet.value = nextSet;
  if (nextSet?.date) {
    selectedDate.value = nextSet.date;
  }
}

async function loadToday() {
  await loadSet({ today: true });
}

async function loadSet(options: { today?: boolean } = {}) {
  const currentRequestId = ++requestId;
  loading.value = true;
  loadError.value = '';
  try {
    const result = options.today
      ? await getTodayDailyQuizSetApi()
      : await getDailyQuizSetApi({ date: selectedDate.value });
    if (currentRequestId !== requestId) return;
    setQuizSet(result);
  } catch {
    if (currentRequestId !== requestId) return;
    quizSet.value = undefined;
    loadError.value = '该日期题目尚未生成或加载失败，可刷新重试或点击生成题目。';
  } finally {
    if (currentRequestId === requestId) {
      loading.value = false;
    }
  }
}

function refresh() {
  void loadSet();
}

function goToday() {
  selectedDate.value = dayjs().format('YYYY-MM-DD');
  void loadToday();
}

function onDateInput(event: Event) {
  const value = (event.target as HTMLInputElement).value;
  if (!value || value === selectedDate.value) return;
  selectedDate.value = value;
  void loadSet();
}

async function generateSet() {
  generating.value = true;
  try {
    const result = await generateDailyQuizSetApi({ date: selectedDate.value });
    setQuizSet(result);
    loadError.value = '';
    message.success('每日题目已生成');
  } finally {
    generating.value = false;
  }
}

function openReplaceModal(question: DailyQuizQuestionVersion) {
  replacingQuestion.value = question;
  replaceForm.reason = '';
  replaceModalOpen.value = true;
}

async function submitReplace() {
  if (!replacingQuestion.value || !quizSet.value) return;
  replacing.value = true;
  try {
    const result = await replaceDailyQuizQuestionApi(
      quizSet.value.id,
      replacingQuestion.value.slotNo,
      { reason: replaceForm.reason.trim() },
    );
    setQuizSet(result);
    replaceModalOpen.value = false;
    replacingQuestion.value = undefined;
    message.success('题目已更换');
  } finally {
    replacing.value = false;
  }
}

function formatSource(source?: string) {
  if (!source) return '—';
  const sourceMap: Record<string, string> = {
    admin_replace: '人工更换',
    ai: 'AI 生成',
    fallback: '兜底题库',
    generated: 'AI 生成',
  };
  return sourceMap[source] || source;
}

function statusTag(status?: string) {
  const value = status || 'missing';
  const statusMap: Record<string, { color: string; text: string }> = {
    fallback: { color: 'warning', text: '兜底生成' },
    generated: { color: 'success', text: '已生成' },
    missing: { color: 'default', text: '未生成' },
    published: { color: 'processing', text: '已发布' },
  };
  return statusMap[value] || { color: 'default', text: value };
}

function answerTag(question?: DailyQuizQuestionVersion) {
  const count = Number(question?.answeredCount ?? 0);
  if (count > 0) {
    return { color: 'warning', text: `已有答题 ${count}` };
  }
  return { color: 'success', text: '未答题' };
}

function canReplaceQuestion(question?: DailyQuizQuestionVersion) {
  return Boolean(question) && !hasAnyAnswered.value && Number(question?.answeredCount ?? 0) <= 0;
}

function versionsForSlot(slotNo: number) {
  return [...(quizSet.value?.questions ?? [])]
    .filter((question) => question.slotNo === slotNo)
    .sort((a, b) => b.versionNo - a.versionNo);
}

function optionKey(option: DailyQuizOption, index: number) {
  return option.id || option.label || String(index);
}

function optionTitle(option: DailyQuizOption, index: number) {
  return option.label || option.id || String.fromCharCode(65 + index);
}

function optionWeightText(option: DailyQuizOption) {
  const weights = option.typeWeights || option.weights;
  if (!weights || Object.keys(weights).length === 0) return '';
  return Object.entries(weights)
    .map(([key, value]) => `${key}:${value}`)
    .join(' / ');
}

onMounted(loadToday);
</script>

<template>
  <Page
    description="按业务日期查看、生成和单题更换每日画像校准题。"
    title="每日题库管理"
  >
    <Card :bordered="false" class="daily-quiz-bank-card">
      <div class="toolbar">
        <div>
          <div class="card-title">每日 5 题题集</div>
          <div class="card-desc">
            默认查看今日题目；切换日期可查看历史题集，已有答题的题目不可更换。
          </div>
        </div>
        <Space wrap>
          <input
            aria-label="选择题集日期"
            class="date-input"
            type="date"
            :value="selectedDate"
            @change="onDateInput"
          />
          <Button @click="goToday">今日</Button>
          <Button :loading="loading" @click="refresh">刷新</Button>
          <Button type="primary" :loading="generating" @click="generateSet">
            {{ generateButtonText }}
          </Button>
        </Space>
      </div>

      <Alert v-if="loadError" class="section-gap" show-icon type="warning" :message="loadError">
        <template #action>
          <Button size="small" type="link" @click="refresh">重试</Button>
        </template>
      </Alert>

      <Row :gutter="[12, 12]" class="section-gap">
        <Col :lg="5" :md="8" :sm="12" :xs="24">
          <Card size="small">
            <Statistic title="题集状态" :value="generatedStatusText" />
          </Card>
        </Col>
        <Col :lg="5" :md="8" :sm="12" :xs="24">
          <Card size="small">
            <Statistic title="来源" :value="sourceText" />
          </Card>
        </Col>
        <Col :lg="6" :md="8" :sm="12" :xs="24">
          <Card size="small">
            <Statistic title="模型" :value="modelText" />
          </Card>
        </Col>
        <Col :lg="4" :md="8" :sm="12" :xs="24">
          <Card size="small">
            <Statistic title="题目数" :value="questionCountText" />
          </Card>
        </Col>
        <Col :lg="4" :md="8" :sm="12" :xs="24">
          <Card size="small">
            <Statistic title="推送状态" :value="pushedText" />
          </Card>
        </Col>
      </Row>

      <Card v-if="quizSet" class="set-summary" size="small">
        <Space wrap>
          <span>日期：{{ quizSet.date }}</span>
          <Tag :color="statusTag(quizSet.status).color">
            {{ statusTag(quizSet.status).text }}
          </Tag>
          <span>生成时间：{{ quizSet.generatedAt || '—' }}</span>
          <span>发布时间：{{ quizSet.publishedAt || '—' }}</span>
          <span>推送时间：{{ quizSet.pushedAt || '—' }}</span>
        </Space>
        <Alert
          v-if="quizSet.errorMessage"
          class="summary-error"
          show-icon
          type="warning"
          :message="quizSet.errorMessage"
        />
        <Alert
          v-if="hasAnyAnswered"
          class="summary-error"
          show-icon
          type="warning"
          message="整套题已锁定"
          description="当前日期已有用户答题，为避免同一天不同用户题目不一致，服务端会拒绝继续更换任意题目。"
        />
      </Card>

      <Empty v-else-if="!loading" class="empty-state" description="该日期暂无题集">
        <Button type="primary" :loading="generating" @click="generateSet">
          {{ generateButtonText }}
        </Button>
      </Empty>

      <div class="question-grid section-gap">
        <Card
          v-for="slot in activeQuestions"
          :key="slot.slotNo"
          class="question-card"
          :title="`第 ${slot.slotNo} 题`"
        >
          <template #extra>
            <Space v-if="slot.question" :size="6" wrap>
              <Tag color="blue">v{{ slot.question.versionNo }}</Tag>
              <Tag :color="answerTag(slot.question).color">
                {{ answerTag(slot.question).text }}
              </Tag>
            </Space>
          </template>

          <template v-if="slot.question">
            <div class="question-meta">
              <Tag :color="slot.question.isActive ? 'success' : 'default'">
                {{ slot.question.isActive ? '当前版本' : '历史版本' }}
              </Tag>
              <span>维度：{{ slot.question.question.dimension || '—' }}</span>
              <span>来源：{{ formatSource(slot.question.source) }}</span>
            </div>
            <div class="question-body">{{ slot.question.question.body }}</div>
            <div class="option-list">
              <div
                v-for="(option, index) in slot.question.question.options"
                :key="optionKey(option, index)"
                class="option-item"
              >
                <span class="option-label">{{ optionTitle(option, index) }}</span>
                <span class="option-text">{{ option.text }}</span>
                <span v-if="optionWeightText(option)" class="option-weights">
                  {{ optionWeightText(option) }}
                </span>
              </div>
            </div>
            <div class="question-footer">
              <div class="version-info">
                <span>题目 ID：{{ slot.question.question.id }}</span>
                <span>版本记录：v{{ slot.question.versionNo }}</span>
                <span v-if="slot.question.replaceReason">
                  更换原因：{{ slot.question.replaceReason }}
                </span>
              </div>
              <Button
                :disabled="!canReplaceQuestion(slot.question)"
                @click="openReplaceModal(slot.question)"
              >
                更换本题
              </Button>
            </div>
            <div
              v-if="versionsForSlot(slot.slotNo).length > 0"
              class="version-history"
            >
              <div class="version-history-title">版本记录</div>
              <div
                v-for="version in versionsForSlot(slot.slotNo)"
                :key="version.id"
                class="version-history-item"
              >
                <div class="version-history-meta">
                  <Tag :color="version.isActive ? 'success' : 'default'">
                    v{{ version.versionNo }}
                    {{ version.isActive ? '当前版本' : '历史版本' }}
                  </Tag>
                  <span>题目 ID：{{ version.question.id }}</span>
                  <span>来源：{{ formatSource(version.source) }}</span>
                  <span v-if="version.operator">
                    操作人：{{ version.operator }}
                  </span>
                  <span v-if="version.createTime">
                    时间：{{ version.createTime }}
                  </span>
                  <span>已答：{{ version.answeredCount || 0 }}</span>
                </div>
                <div class="version-history-body">
                  {{ version.question.body }}
                </div>
                <div v-if="version.replaceReason" class="version-history-reason">
                  更换原因：{{ version.replaceReason }}
                </div>
              </div>
            </div>
          </template>

          <Empty v-else description="该题槽暂无题目" />
        </Card>
      </div>
    </Card>

    <Modal
      v-if="replaceModalOpen"
      v-model:open="replaceModalOpen"
      :confirm-loading="replacing"
      :title="`更换第 ${replacingQuestion?.slotNo || ''} 题`"
      width="min(560px, calc(100vw - 32px))"
      @ok="submitReplace"
    >
      <div class="modal-title-text">
        更换第 {{ replacingQuestion?.slotNo || '' }} 题
      </div>
      <Alert
        show-icon
        type="info"
        message="更换后将生成新的题目版本"
        description="已有用户答题的题目服务端会拒绝更换；原因会写入版本记录，便于审计。"
      />
      <Form layout="vertical" class="replace-form">
        <Form.Item label="当前题目">
          <Input :value="replacingQuestion?.question.body || ''" readonly />
        </Form.Item>
        <Form.Item label="更换原因">
          <Textarea
            v-model:value="replaceForm.reason"
            :maxlength="200"
            :rows="4"
            placeholder="请输入更换原因，如题目表达不够精准"
          />
          <div class="field-hint">可留空；填写后会随版本记录保存。</div>
        </Form.Item>
      </Form>
    </Modal>
  </Page>
</template>

<style scoped>
.daily-quiz-bank-card {
  border-radius: 8px;
}

.toolbar {
  display: flex;
  gap: 16px;
  align-items: flex-start;
  justify-content: space-between;
}

.card-title {
  font-size: 16px;
  font-weight: 600;
}

.card-desc,
.field-hint,
.version-info {
  color: hsl(var(--muted-foreground));
  font-size: 12px;
}

.date-input {
  min-height: 32px;
  padding: 4px 11px;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
}

.section-gap {
  margin-top: 16px;
}

.set-summary {
  margin-top: 16px;
  background: hsl(var(--muted) / 40%);
}

.summary-error {
  margin-top: 12px;
}

.empty-state {
  padding: 32px 0;
}

.question-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 16px;
}

.question-card :deep(.ant-card-head-title) {
  font-weight: 600;
}

.question-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  color: hsl(var(--muted-foreground));
  font-size: 12px;
}

.question-body {
  margin-top: 12px;
  color: hsl(var(--foreground));
  font-size: 15px;
  font-weight: 600;
  line-height: 1.6;
}

.option-list {
  display: grid;
  gap: 8px;
  margin-top: 12px;
}

.option-item {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: 8px;
  align-items: flex-start;
  padding: 8px 10px;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
  background: hsl(var(--background));
}

.option-label {
  display: inline-flex;
  min-width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  background: hsl(var(--primary) / 10%);
  color: hsl(var(--primary));
  font-weight: 600;
}

.option-text {
  line-height: 1.6;
}

.option-weights {
  color: hsl(var(--muted-foreground));
  font-size: 12px;
  white-space: nowrap;
}

.question-footer {
  display: flex;
  gap: 12px;
  align-items: flex-end;
  justify-content: space-between;
  margin-top: 16px;
}

.version-info {
  display: grid;
  gap: 4px;
}

.replace-form {
  margin-top: 16px;
}

.version-history {
  display: grid;
  gap: 10px;
  margin-top: 16px;
  padding-top: 12px;
  border-top: 1px dashed hsl(var(--border));
}

.version-history-title {
  font-size: 13px;
  font-weight: 600;
}

.version-history-item {
  display: grid;
  gap: 6px;
  padding: 10px;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
  background: hsl(var(--muted) / 25%);
}

.version-history-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  color: hsl(var(--muted-foreground));
  font-size: 12px;
}

.version-history-body {
  line-height: 1.6;
}

.version-history-reason {
  color: hsl(var(--muted-foreground));
  font-size: 12px;
}

.modal-title-text {
  margin-bottom: 12px;
  font-weight: 600;
}

.field-hint {
  margin-top: 6px;
}

@media (max-width: 768px) {
  .toolbar,
  .question-footer {
    display: block;
  }

  .toolbar :deep(.ant-space),
  .question-footer :deep(.ant-btn) {
    width: 100%;
    margin-top: 12px;
  }

  .question-grid {
    grid-template-columns: 1fr;
  }

  .option-item {
    grid-template-columns: auto minmax(0, 1fr);
  }

  .option-weights {
    grid-column: 2;
    white-space: normal;
  }
}
</style>
