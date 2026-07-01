<script setup lang="ts">
import type { VideoAnalysisJob } from '#/api';

import { onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { IconifyIcon } from '@vben/icons';
import { useAccessStore } from '@vben/stores';

import {
  Button,
  Card,
  Col,
  Form,
  Input,
  message,
  Row,
  Select,
  Space,
  Table,
  Tag,
  Upload,
} from 'ant-design-vue';

import {
  createVideoAnalysisApi,
  getVideoAnalysisJobsApi,
  retryVideoAnalysisApi,
  uploadFileApi,
} from '#/api';

import { withPreviewToken } from './asset-preview';

const loading = ref(false);
const uploading = ref(false);
const creating = ref(false);
const retryingId = ref('');
const accessStore = useAccessStore();
const jobs = ref<VideoAnalysisJob[]>([]);
const total = ref(0);

const query = reactive({
  page: 1,
  pageSize: 10,
  status: undefined as string | undefined,
});

const form = reactive({
  videoAssetId: '',
  videoName: '',
  videoUrl: '',
});

const ANALYSIS_UPLOAD_DIR = 'video/analysis';

const statusMeta: Record<string, { color: string; text: string }> = {
  completed: { color: 'success', text: '已完成' },
  failed: { color: 'error', text: '失败' },
  queued: { color: 'warning', text: '排队中' },
  running: { color: 'processing', text: '分析中' },
};

const columns = [
  { dataIndex: 'videoName', ellipsis: true, title: '视频', width: 220 },
  { dataIndex: 'status', title: '状态', width: 110 },
  { dataIndex: 'scenes', title: '场景', width: 260 },
  { dataIndex: 'characters', title: '人物', width: 260 },
  { dataIndex: 'assets', title: '资产', width: 280 },
  { dataIndex: 'speechTopics', title: '语音主题', width: 300 },
  { dataIndex: 'audioSummary', title: '语音摘要', width: 360 },
  { dataIndex: 'seedancePrompt', title: 'Seedance 提示词', width: 420 },
  { dataIndex: 'createTime', title: '创建时间', width: 180 },
];

const statusOptions = [
  { label: '全部状态', value: undefined },
  { label: '排队中', value: 'queued' },
  { label: '分析中', value: 'running' },
  { label: '已完成', value: 'completed' },
  { label: '失败', value: 'failed' },
];

function statusTag(status: string) {
  return statusMeta[status] ?? { color: 'default', text: status || '未知' };
}

function isPublicHttpUrl(value?: string) {
  if (!value) return false;
  try {
    const url = new URL(value);
    return (
      ['http:', 'https:'].includes(url.protocol) && isPublicHost(url.hostname)
    );
  } catch {
    return false;
  }
}

function isPublicHost(hostname: string) {
  const host = hostname.toLowerCase();
  if (host === 'localhost' || host === '127.0.0.1' || host === '::1') {
    return false;
  }
  const ipv4 = host.match(/^(\d+)\.(\d+)\.(\d+)\.(\d+)$/);
  if (!ipv4) {
    return true;
  }
  const parts = ipv4.slice(1).map(Number);
  if (parts.length !== 4) {
    return false;
  }
  const a = parts[0] ?? 0;
  const b = parts[1] ?? 0;
  if (a === 10 || a === 127 || a === 0 || a === 169) return false;
  if (a === 192 && b === 168) return false;
  if (a === 172 && b >= 16 && b <= 31) return false;
  return true;
}

function pickPublicUrl(res: { objectUrl?: string; url: string }) {
  if (isPublicHttpUrl(res.objectUrl)) {
    return res.objectUrl ?? '';
  }
  return isPublicHttpUrl(res.url) ? res.url : '';
}

const previewVideoUrl = (url?: string) =>
  url ? withPreviewToken(url, accessStore.accessToken) : '';

async function handleVideoUpload(file: File) {
  uploading.value = true;
  try {
    const res = await uploadFileApi(file, ANALYSIS_UPLOAD_DIR);
    const publicUrl = pickPublicUrl(res);
    if (!publicUrl) {
      message.error(
        '视频已上传到视频分析目录，但没有拿到文件桶公网地址，请检查文件桶公网访问配置',
      );
      return false;
    }
    form.videoAssetId = String(res.assetId || '');
    form.videoName = res.name || file.name;
    form.videoUrl = publicUrl;
    message.success(`视频「${file.name}」上传成功`);
  } catch (error: any) {
    const errorMessage =
      error?.response?.data?.error ||
      error?.response?.data?.message ||
      error?.message ||
      '视频上传失败，请重新上传';
    message.error(errorMessage);
  } finally {
    uploading.value = false;
  }
  return false;
}

async function createAnalysis() {
  if (!form.videoUrl) {
    message.warning('请先上传需要分析的视频');
    return;
  }
  creating.value = true;
  try {
    await createVideoAnalysisApi({
      videoAssetId: form.videoAssetId || undefined,
      videoName: form.videoName || undefined,
      videoUrl: form.videoUrl,
    });
    message.success('已创建视频分析任务，请稍后刷新查看结果');
    resetForm();
    await loadJobs();
    pollJobs();
  } finally {
    creating.value = false;
  }
}

function resetForm() {
  form.videoAssetId = '';
  form.videoName = '';
  form.videoUrl = '';
}

async function loadJobs() {
  loading.value = true;
  try {
    const result = await getVideoAnalysisJobsApi({
      page: query.page,
      pageSize: query.pageSize,
      status: query.status,
    });
    jobs.value = result.items;
    total.value = result.total;
  } finally {
    loading.value = false;
  }
}

function pollJobs(times = 6) {
  let remain = times;
  const timer = window.setInterval(async () => {
    remain -= 1;
    await loadJobs();
    const stillRunning = jobs.value.some((job) =>
      ['queued', 'running'].includes(job.status),
    );
    if (remain <= 0 || !stillRunning) {
      window.clearInterval(timer);
    }
  }, 5000);
}

function handleTableChange(pagination: {
  current?: number;
  pageSize?: number;
}) {
  query.page = pagination.current ?? 1;
  query.pageSize = pagination.pageSize ?? 10;
  loadJobs();
}

function handleStatusChange() {
  query.page = 1;
  loadJobs();
}

function fallbackCopyText(value: string) {
  const textarea = document.createElement('textarea');
  const selection = document.getSelection();
  const selectedRange =
    selection && selection.rangeCount > 0 ? selection.getRangeAt(0) : undefined;

  textarea.value = value;
  textarea.setAttribute('readonly', '');
  textarea.style.left = '-9999px';
  textarea.style.position = 'fixed';
  textarea.style.top = '0';
  document.body.append(textarea);
  textarea.select();
  textarea.setSelectionRange(0, textarea.value.length);

  try {
    return document.execCommand('copy');
  } catch {
    return false;
  } finally {
    textarea.remove();
    if (selection && selectedRange) {
      selection.removeAllRanges();
      selection.addRange(selectedRange);
    }
  }
}

async function copyClipboardText(value: string) {
  try {
    await navigator.clipboard.writeText(value);
    return true;
  } catch {
    return fallbackCopyText(value);
  }
}

async function copyPrompt(prompt: string) {
  if (!prompt) return;
  if (await copyClipboardText(prompt)) {
    message.success('Seedance 提示词已复制');
    return;
  }
  message.error('复制失败，请手动复制');
}

async function copyAnalysisItems(record: Record<string, any>, key: unknown) {
  const values = analysisList(record, key);
  if (values.length === 0) return;
  if (await copyClipboardText(values.join('\n'))) {
    message.success(`${analysisColumnTitle(key)}文案已复制`);
    return;
  }
  message.error('复制失败，请手动复制');
}

async function retryAnalysis(record: VideoAnalysisJob) {
  retryingId.value = record.id;
  try {
    await retryVideoAnalysisApi(record.id);
    message.success('已重新提交分析任务');
    await loadJobs();
  } finally {
    retryingId.value = '';
  }
}

function jobRecord(record: Record<string, any>): VideoAnalysisJob {
  return record as VideoAnalysisJob;
}

function analysisList(record: Record<string, any>, key: unknown) {
  const job = jobRecord(record);
  if (key === 'scenes') return job.scenes;
  if (key === 'characters') return job.characters;
  if (key === 'assets') return job.assets;
  if (key === 'speechTopics') return job.speechTopics;
  if (key === 'speechKeywords') return job.speechKeywords;
  return [];
}

function analysisColumnTitle(key: unknown) {
  if (key === 'scenes') return '场景';
  if (key === 'characters') return '人物';
  if (key === 'assets') return '资产';
  if (key === 'speechTopics') return '语音主题';
  return '分析';
}

onMounted(loadJobs);
</script>

<template>
  <Page
    description="上传参考视频后异步分析场景、人物、资产与语音主题，并生成 seedance2.0 参考提示词。"
    title="视频分析"
  >
    <Row :gutter="[16, 16]">
      <Col :lg="8" :xs="24">
        <Card :bordered="false" class="analysis-card">
          <div class="card-title">创建分析任务</div>
          <Form layout="vertical">
            <Form.Item label="上传视频" required>
              <Upload
                :before-upload="handleVideoUpload"
                :show-upload-list="false"
                accept="video/*"
              >
                <Button :loading="uploading">
                  <IconifyIcon class="mr-1" icon="lucide:upload" />
                  上传参考视频
                </Button>
              </Upload>
            </Form.Item>
            <Form.Item label="视频名称">
              <Input
                v-model:value="form.videoName"
                placeholder="上传后自动填充"
              />
            </Form.Item>
            <Form.Item label="视频地址">
              <Input.TextArea
                v-model:value="form.videoUrl"
                :auto-size="{ minRows: 3, maxRows: 5 }"
                placeholder="上传后自动填充，也可粘贴公网视频地址"
              />
            </Form.Item>
            <Button :loading="creating" type="primary" @click="createAnalysis">
              <IconifyIcon class="mr-1" icon="lucide:scan-search" />
              开始分析
            </Button>
          </Form>
        </Card>
      </Col>

      <Col :lg="16" :xs="24">
        <Card :bordered="false" class="analysis-card">
          <div class="history-head">
            <div>
              <div class="card-title">分析记录</div>
              <div class="card-desc">
                共 {{ total }} 条记录，完成后可复制 Seedance 提示词。
              </div>
            </div>
            <Space wrap>
              <Select
                v-model:value="query.status"
                :options="statusOptions"
                allow-clear
                class="status-select"
                placeholder="全部状态"
                @change="handleStatusChange"
              />
              <Button :loading="loading" @click="loadJobs">刷新列表</Button>
            </Space>
          </div>
          <Table
            :columns="columns"
            :data-source="jobs"
            :loading="loading"
            :pagination="{
              current: query.page,
              pageSize: query.pageSize,
              showSizeChanger: true,
              total,
            }"
            :scroll="{ x: 2190 }"
            table-layout="fixed"
            row-key="id"
            @change="handleTableChange"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'videoName'">
                <Space direction="vertical" size="small">
                  <span>{{ record.videoName || '-' }}</span>
                  <video
                    v-if="record.videoUrl"
                    :src="previewVideoUrl(record.videoUrl)"
                    class="row-video"
                    controls
                    preload="none"
                  ></video>
                </Space>
              </template>
              <template v-else-if="column.dataIndex === 'status'">
                <Tag :color="statusTag(record.status).color">
                  {{ statusTag(record.status).text }}
                </Tag>
                <div v-if="record.errorMessage" class="error-text">
                  {{ record.errorMessage }}
                </div>
                <Button
                  v-if="record.status === 'failed'"
                  :loading="retryingId === record.id"
                  class="retry-button"
                  size="small"
                  type="link"
                  @click="retryAnalysis(jobRecord(record))"
                >
                  重试
                </Button>
              </template>
              <template
                v-else-if="
                  ['scenes', 'characters', 'assets', 'speechTopics'].includes(
                    String(column.dataIndex),
                  )
                "
              >
                <div class="tag-cell">
                  <div class="tag-list">
                    <Tag
                      v-for="item in analysisList(record, column.dataIndex)"
                      :key="item"
                      :title="item"
                      class="analysis-tag"
                    >
                      {{ item }}
                    </Tag>
                  </div>
                  <Button
                    v-if="analysisList(record, column.dataIndex).length > 0"
                    class="cell-copy-button"
                    size="small"
                    type="link"
                    @click="copyAnalysisItems(record, column.dataIndex)"
                  >
                    <IconifyIcon class="mr-1" icon="lucide:copy" />
                    复制
                  </Button>
                </div>
              </template>
              <template v-else-if="column.dataIndex === 'audioSummary'">
                <div class="audio-cell">
                  <Tag :color="record.hasSpeech ? 'processing' : 'default'">
                    {{ record.hasSpeech ? '有人声' : '未识别语音' }}
                  </Tag>
                  <div class="audio-summary">
                    {{ record.audioSummary || '-' }}
                  </div>
                  <Space v-if="jobRecord(record).speechKeywords?.length" wrap>
                    <Tag
                      v-for="item in jobRecord(record).speechKeywords"
                      :key="item"
                      color="blue"
                    >
                      {{ item }}
                    </Tag>
                  </Space>
                  <ol
                    v-if="jobRecord(record).speechOutline?.length"
                    class="speech-outline"
                  >
                    <li
                      v-for="item in jobRecord(record).speechOutline"
                      :key="item"
                    >
                      {{ item }}
                    </li>
                  </ol>
                </div>
              </template>
              <template v-else-if="column.dataIndex === 'seedancePrompt'">
                <div class="prompt-cell">
                  <div class="prompt-text">
                    {{ record.seedancePrompt || '-' }}
                  </div>
                  <Button
                    v-if="record.seedancePrompt"
                    size="small"
                    type="link"
                    @click="copyPrompt(jobRecord(record).seedancePrompt)"
                  >
                    复制
                  </Button>
                </div>
              </template>
            </template>
          </Table>
        </Card>
      </Col>
    </Row>
  </Page>
</template>

<style scoped>
.analysis-card {
  border-radius: 8px;
}

.analysis-card :deep(.ant-table-cell) {
  overflow: hidden;
  vertical-align: top;
}

.card-title {
  margin-bottom: 16px;
  font-size: 18px;
  font-weight: 600;
  color: hsl(var(--foreground));
}

.card-desc {
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}

.history-head {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 16px;
}

.status-select {
  width: 120px;
}

.row-video {
  width: 180px;
  max-height: 110px;
  background: #111827;
  border-radius: 6px;
}

.error-text {
  max-width: 180px;
  margin-top: 6px;
  font-size: 12px;
  color: #ef4444;
  white-space: normal;
}

.retry-button {
  padding: 0;
  margin-top: 6px;
}

.prompt-cell {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  min-width: 0;
}

.tag-cell {
  display: flex;
  flex-direction: column;
  gap: 6px;
  align-items: flex-start;
  min-width: 0;
}

.tag-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  max-height: 96px;
  overflow: auto;
}

.analysis-tag {
  display: inline-block;
  max-width: 100%;
  margin-inline-end: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  line-height: 22px;
  white-space: nowrap;
}

.cell-copy-button {
  height: 24px;
  padding: 0;
  font-size: 12px;
}

.prompt-text {
  max-height: 96px;
  overflow: auto;
  font-size: 13px;
  line-height: 1.6;
  color: hsl(var(--foreground));
  white-space: pre-wrap;
}

.audio-cell {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: flex-start;
}

.audio-summary {
  max-height: 72px;
  overflow: auto;
  font-size: 13px;
  line-height: 1.6;
  color: hsl(var(--foreground));
  white-space: pre-wrap;
}

.speech-outline {
  max-height: 92px;
  padding-left: 18px;
  margin: 0;
  overflow: auto;
  font-size: 12px;
  line-height: 1.6;
  color: hsl(var(--muted-foreground));
}
</style>
