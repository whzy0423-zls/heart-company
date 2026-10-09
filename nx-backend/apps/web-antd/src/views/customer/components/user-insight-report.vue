<script setup lang="ts">
import type {
  AppUserReport,
  AppUserReportClaim,
  AppUserReportList,
} from '#/api/core/app-user-report';

import { computed, onBeforeUnmount, ref, watch } from 'vue';

import { useAccessStore } from '@vben/stores';

import { ReloadOutlined } from '@ant-design/icons-vue';
import { Alert, Button, Drawer, Select, Tag, Tooltip } from 'ant-design-vue';

import {
  getAppUserReportApi,
  getAppUserReportsApi,
  requestAppUserReportApi,
} from '#/api/core/app-user-report';

const props = defineProps<{
  appUserId: number;
  open: boolean;
  userName?: string;
}>();
const emit = defineEmits<{ 'update:open': [value: boolean] }>();
const access = useAccessStore();
const canGenerate = computed(() =>
  access.accessCodes.includes('Customer:UserInsights:Generate'),
);
const state = ref<AppUserReportList>();
const report = ref<AppUserReport>();
const selectedId = ref<number>();
const loading = ref(false);
const detailLoading = ref(false);
const generating = ref(false);
const listError = ref('');
const detailError = ref('');
const generationError = ref('');
const generationNotice = ref('');
const queued = ref(false);
let contextRevision = 0;
let listRequest = 0;
let detailRequest = 0;

const statusLabels: Record<AppUserReportList['status'], string> = {
  analyzing: '分析中',
  disabled: '未开启',
  failed: '分析失败',
  insufficient_data: '资料不足',
  pending: '待分析',
  ready: '已有报告',
};
const historyOptions = computed(() =>
  (state.value?.reports ?? []).map((item) => ({
    label: `版本 ${item.version} · ${formatTime(item.generatedAt)}`,
    value: item.id,
  })),
);
const sections = computed<
  Array<{ claims: AppUserReportClaim[]; title: string }>
>(() => {
  const analysis = report.value?.analysis;
  if (!analysis) return [];
  return [
    { claims: [analysis.summary], title: '分析摘要' },
    {
      claims: analysis.weeklyReview ? [analysis.weeklyReview] : [],
      title: '期间复盘',
    },
    {
      claims: analysis.trendExplanation ? [analysis.trendExplanation] : [],
      title: '趋势解释',
    },
    { claims: analysis.observations, title: '用户陈述' },
    { claims: analysis.patterns, title: '可能的模式' },
    { claims: analysis.goals, title: '目标' },
    { claims: analysis.changes, title: '变化' },
    { claims: analysis.uncertainties, title: '不确定性' },
    { claims: analysis.recommendations, title: '行动建议' },
    { claims: analysis.strengths ?? [], title: '优势' },
    { claims: analysis.stressPoints ?? [], title: '压力点' },
    { claims: analysis.awarenessPrompts ?? [], title: '觉察提示' },
    {
      claims: (analysis.actions ?? []).map((action) => ({
        evidenceRefs: action.evidenceRefs,
        text: `${action.title}\n${action.detail}`,
      })),
      title: '具体行动',
    },
  ].map((section) => ({
    ...section,
    claims: (section.claims ?? []).filter((claim) => claim?.text),
  }));
});

function formatTime(value: string, timeZone?: string) {
  if (!value) return '-';
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? value
    : date.toLocaleString('zh-CN', { hour12: false, timeZone });
}

function evidenceAnchor(id: string) {
  const index =
    report.value?.evidence.findIndex((item) => item.id === id) ?? -1;
  return index >= 0
    ? `growth-evidence-${report.value?.id}-${index}`
    : undefined;
}

function scrollToEvidence(id: string) {
  const anchor = evidenceAnchor(id);
  const target = anchor ? document.getElementById(anchor) : null;
  target?.scrollIntoView({ block: 'nearest' });
  target?.focus({ preventScroll: true });
}

function isCurrent(revision: number, userId: number) {
  return (
    props.open && contextRevision === revision && props.appUserId === userId
  );
}

async function loadDetail(id: number) {
  const request = ++detailRequest;
  const revision = contextRevision;
  const userId = props.appUserId;
  selectedId.value = id;
  report.value = undefined;
  detailError.value = '';
  detailLoading.value = true;
  try {
    const result = await getAppUserReportApi(id);
    if (!isCurrent(revision, userId) || request !== detailRequest) return;
    if (result.appUserId !== userId || result.id !== id)
      throw new Error('Report ownership mismatch');
    report.value = result;
  } catch {
    if (isCurrent(revision, userId) && request === detailRequest) {
      detailError.value = '报告详情加载失败，请重试';
    }
  } finally {
    if (isCurrent(revision, userId) && request === detailRequest)
      detailLoading.value = false;
  }
}

async function loadHistory() {
  if (!props.open || props.appUserId <= 0) return;
  const request = ++listRequest;
  const revision = contextRevision;
  const userId = props.appUserId;
  const previousId = selectedId.value;
  ++detailRequest;
  state.value = undefined;
  report.value = undefined;
  selectedId.value = undefined;
  loading.value = true;
  detailLoading.value = false;
  listError.value = '';
  detailError.value = '';
  try {
    const result = await getAppUserReportsApi(userId);
    if (!isCurrent(revision, userId) || request !== listRequest) return;
    state.value = result;
    if (!result.enabled) return;
    const selected =
      result.reports.find((item) => item.id === previousId) ??
      result.reports[0];
    if (selected) await loadDetail(selected.id);
  } catch {
    if (isCurrent(revision, userId) && request === listRequest) {
      state.value = undefined;
      listError.value = '报告记录加载失败，请重试';
    }
  } finally {
    if (isCurrent(revision, userId) && request === listRequest)
      loading.value = false;
  }
}

async function generate() {
  if (
    !canGenerate.value ||
    !state.value?.enabled ||
    loading.value ||
    generating.value
  )
    return;
  const revision = contextRevision;
  const userId = props.appUserId;
  generating.value = true;
  generationError.value = '';
  generationNotice.value = '';
  queued.value = false;
  try {
    const result = await requestAppUserReportApi(userId);
    if (!isCurrent(revision, userId)) return;
    if (!result.queued && result.reason === 'no_new_evidence') {
      generationNotice.value = '当前没有待分析的新资料';
      return;
    }
    if (!result.queued) throw new Error('Analysis was not queued');
    queued.value = true;
    await loadHistory();
  } catch {
    if (isCurrent(revision, userId))
      generationError.value = '分析请求失败，请稍后重试';
  } finally {
    if (isCurrent(revision, userId)) generating.value = false;
  }
}

watch(
  () => [props.open, props.appUserId],
  () => {
    ++contextRevision;
    ++listRequest;
    ++detailRequest;
    state.value = undefined;
    report.value = undefined;
    selectedId.value = undefined;
    listError.value = '';
    detailError.value = '';
    generationError.value = '';
    generationNotice.value = '';
    loading.value = false;
    detailLoading.value = false;
    generating.value = false;
    queued.value = false;
    void loadHistory();
  },
  { immediate: true },
);

onBeforeUnmount(() => {
  ++contextRevision;
});
</script>

<template>
  <Drawer
    :open="open"
    title="成长分析报告"
    width="min(880px, 100vw)"
    :body-style="{ padding: '20px' }"
    @update:open="emit('update:open', $event)"
  >
    <div
      v-if="open"
      class="report-layout"
      :aria-busy="loading || detailLoading"
    >
      <header class="report-toolbar">
        <div class="report-identity">
          <h3>{{ userName || `用户 ${appUserId}` }}</h3>
          <Tag v-if="state">{{
            statusLabels[state.status] || state.status
          }}</Tag>
        </div>
        <div class="report-commands">
          <Tooltip title="刷新报告">
            <Button
              aria-label="刷新报告"
              :disabled="loading"
              @click="loadHistory"
            >
              <template #icon><ReloadOutlined /></template>
            </Button>
          </Tooltip>
          <Button
            v-if="canGenerate"
            :disabled="!state?.enabled || loading || generating"
            :loading="generating"
            @click="generate"
            >请求分析</Button
          >
        </div>
      </header>

      <Alert
        v-if="generationError"
        :message="generationError"
        show-icon
        type="error"
      />
      <Alert
        v-else-if="generationNotice"
        :message="generationNotice"
        show-icon
        type="info"
      />
      <Alert
        v-else-if="queued"
        message="已加入分析队列"
        show-icon
        type="success"
      />
      <Alert v-if="listError" :message="listError" show-icon type="error">
        <template #action
          ><Button size="small" @click="loadHistory">重试</Button></template
        >
      </Alert>
      <p v-else-if="loading && !state" role="status">报告记录加载中...</p>
      <template v-else-if="state">
        <Alert
          v-if="!state.enabled"
          message="用户未开启个人分析"
          show-icon
          type="info"
        />
        <template v-else>
          <p v-if="state.reports.length === 0" class="report-empty">
            暂无分析报告
          </p>
          <template v-else>
            <div class="report-history">
              <label id="report-history-label">历史版本</label>
              <Select
                :value="selectedId"
                aria-labelledby="report-history-label"
                class="report-select"
                :options="historyOptions"
                placeholder="选择报告版本"
                @change="(value) => loadDetail(Number(value))"
              />
            </div>
            <p v-if="detailLoading" role="status">报告详情加载中...</p>
            <Alert
              v-else-if="detailError"
              :message="detailError"
              show-icon
              type="error"
            >
              <template #action>
                <Button
                  size="small"
                  @click="selectedId && loadDetail(selectedId)"
                  >重试</Button
                >
              </template>
            </Alert>
            <article v-else-if="report" class="report-content">
              <div class="report-publication">
                <strong>版本 {{ report.version }}</strong>
                <Tag :color="report.published ? 'success' : 'default'">{{
                  report.published ? '当前已发布' : '内部版本'
                }}</Tag>
              </div>
              <dl class="report-metadata">
                <dt>生成时间</dt>
                <dd>{{ formatTime(report.generatedAt) }}</dd>
                <dt>主体卡片</dt>
                <dd>{{ report.cardId }}</dd>
                <dt>复盘期间</dt>
                <dd>
                  {{ formatTime(report.periodStart, report.timezone) }} 至
                  {{ formatTime(report.periodEnd, report.timezone) }} ({{
                    report.timezone
                  }})
                </dd>
                <dt>证据时间</dt>
                <dd>
                  {{ formatTime(report.sourceFrom) }} 至
                  {{ formatTime(report.sourceThrough) }}
                </dd>
                <dt>证据数量</dt>
                <dd>{{ report.evidenceCount }}</dd>
                <dt>采样覆盖</dt>
                <dd>
                  {{ report.coverage.windowDays }} 天，{{
                    report.coverage.includedCount
                  }}
                  / {{ report.coverage.eligibleCount }} 条
                </dd>
                <dt>未纳入数量</dt>
                <dd>{{ report.coverage.omittedCount }}</dd>
                <dt>文本截断</dt>
                <dd>
                  {{ report.coverage.truncated ? '已截断' : '未截断' }}，{{
                    report.coverage.truncatedCount
                  }}
                  条
                </dd>
              </dl>
              <section
                v-for="section in sections"
                :key="section.title"
                class="report-section"
              >
                <h4>{{ section.title }}</h4>
                <p v-if="section.claims.length === 0" class="report-muted">
                  暂无
                </p>
                <ul v-else class="claim-list">
                  <li v-for="(claim, index) in section.claims" :key="index">
                    <p>{{ claim.text }}</p>
                    <div class="evidence-references">
                      <span>证据</span>
                      <template v-for="id in claim.evidenceRefs" :key="id">
                        <a
                          v-if="evidenceAnchor(id)"
                          :href="`#${evidenceAnchor(id)}`"
                          @click.prevent="scrollToEvidence(id)"
                          >{{ id }}</a
                        >
                        <span v-else>{{ id }}</span>
                      </template>
                    </div>
                  </li>
                </ul>
              </section>
              <section class="report-section">
                <h4>原始证据</h4>
                <p v-if="report.evidence.length === 0" class="report-muted">
                  暂无
                </p>
                <ol v-else class="source-list">
                  <li
                    v-for="(item, index) in report.evidence"
                    :id="`growth-evidence-${report.id}-${index}`"
                    :key="item.id"
                    tabindex="-1"
                  >
                    <div class="source-meta">
                      <strong>{{ item.id }}</strong
                      ><Tag>{{ item.kind }}</Tag>
                    </div>
                    <time class="report-muted">{{
                      formatTime(item.occurredAt)
                    }}</time>
                    <p>{{ item.text }}</p>
                  </li>
                </ol>
              </section>
            </article>
          </template>
        </template>
      </template>
    </div>
  </Drawer>
</template>

<style scoped>
.report-layout,
.report-content {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 20px;
}
.report-toolbar,
.report-identity,
.report-commands,
.report-publication,
.source-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.report-toolbar {
  justify-content: space-between;
}
.report-identity {
  min-width: 0;
  flex: 1 1 180px;
}
.report-identity h3 {
  margin: 0;
  font-size: 16px;
  overflow-wrap: anywhere;
}
.report-commands {
  flex: 0 0 auto;
}
.report-history {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 8px;
}
.report-select {
  width: 100%;
  min-width: 0;
}
.report-metadata {
  display: grid;
  grid-template-columns: 88px minmax(0, 1fr);
  gap: 10px 12px;
  margin: 0;
  font-size: 13px;
}
.report-metadata dt,
.report-muted {
  color: hsl(var(--muted-foreground));
}
.report-metadata dd {
  min-width: 0;
  margin: 0;
}
.report-section {
  border-top: 1px solid hsl(var(--border));
  padding-top: 16px;
}
.report-section h4 {
  margin: 0 0 12px;
  font-size: 14px;
}
.report-layout p {
  margin: 0;
  white-space: pre-wrap;
}
.report-content,
.report-layout p,
.evidence-references,
.source-meta {
  overflow-wrap: anywhere;
}
.claim-list,
.source-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin: 0;
  padding-left: 20px;
}
.evidence-references {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 10px;
  margin-top: 6px;
  font-size: 12px;
}
.source-list li {
  scroll-margin-top: 20px;
}
.source-list li p {
  margin-top: 6px;
}
.report-empty {
  padding: 20px 0;
  text-align: center;
}
@media (max-width: 420px) {
  .report-metadata {
    grid-template-columns: 72px minmax(0, 1fr);
    gap: 10px 8px;
  }
  .report-identity {
    flex-basis: 100%;
  }
  .report-commands {
    margin-left: auto;
  }
}
</style>
