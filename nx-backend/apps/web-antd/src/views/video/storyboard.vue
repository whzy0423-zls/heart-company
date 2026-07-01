<script setup lang="ts">
import type {
  VideoAnalysisJob,
  VideoStoryboard,
  VideoStoryboardShot,
} from '#/api';

import { onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { IconifyIcon } from '@vben/icons';

import {
  Button,
  Card,
  Col,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Table,
  Tag,
  message,
} from 'ant-design-vue';

import {
  createVideoStoryboardApi,
  deleteVideoStoryboardApi,
  getVideoAnalysisJobsApi,
  getVideoStoryboardsApi,
  retryVideoStoryboardApi,
  updateVideoStoryboardApi,
} from '#/api';

const loading = ref(false);
const creating = ref(false);
const saving = ref(false);
const retryingId = ref('');
const storyboards = ref<VideoStoryboard[]>([]);
const analyses = ref<VideoAnalysisJob[]>([]);
const total = ref(0);
const editorOpen = ref(false);

const query = reactive({
  keyword: '',
  page: 1,
  pageSize: 10,
  status: undefined as string | undefined,
});

const form = reactive({
  analysisJobId: '',
  theme: '',
  title: '',
});

const editor = reactive<{
  globalPrompt: string;
  id: string;
  shots: StoryboardEditorShot[];
  styleGuideText: string;
  theme: string;
  title: string;
}>({
  globalPrompt: '',
  id: '',
  shots: [],
  styleGuideText: '',
  theme: '',
  title: '',
});

const statusMeta: Record<string, { color: string; text: string }> = {
  completed: { color: 'success', text: '已完成' },
  failed: { color: 'error', text: '失败' },
  queued: { color: 'warning', text: '排队中' },
  running: { color: 'processing', text: '生成中' },
};

type StoryboardEditorShot = VideoStoryboardShot & { _key: string };

const columns = [
  { dataIndex: 'title', ellipsis: true, title: '分镜方案', width: 220 },
  { dataIndex: 'theme', ellipsis: true, title: '主题', width: 240 },
  { dataIndex: 'status', title: '状态', width: 110 },
  { dataIndex: 'shots', title: '分镜', width: 140 },
  { dataIndex: 'globalPrompt', title: '全局提示词', width: 420 },
  { dataIndex: 'createTime', title: '创建时间', width: 180 },
  {
    fixed: 'right' as const,
    key: 'storyboardOperate',
    title: '操作',
    width: 230,
  },
];

const shotColumns = [
  { dataIndex: 'index', title: '镜号', width: 70 },
  { dataIndex: 'duration', title: '时长', width: 100 },
  { dataIndex: 'title', title: '标题', width: 180 },
  { dataIndex: 'scene', title: '场景', width: 220 },
  { dataIndex: 'characters', title: '人物', width: 220 },
  { dataIndex: 'assets', title: '资产', width: 240 },
  { dataIndex: 'action', key: 'shotAction', title: '动作', width: 260 },
  { dataIndex: 'camera', title: '镜头', width: 240 },
  { dataIndex: 'composition', title: '构图', width: 240 },
  { dataIndex: 'lighting', title: '光影风格', width: 240 },
  { dataIndex: 'audio', title: '音频', width: 220 },
  { dataIndex: 'dialogue', title: '台词/旁白', width: 240 },
  { dataIndex: 'seedancePrompt', title: 'Seedance 提示词', width: 420 },
  { fixed: 'right' as const, key: 'shotOperate', title: '操作', width: 80 },
];

const statusOptions = [
  { label: '全部状态', value: undefined },
  { label: '排队中', value: 'queued' },
  { label: '生成中', value: 'running' },
  { label: '已完成', value: 'completed' },
  { label: '失败', value: 'failed' },
];

function statusTag(status: string) {
  return statusMeta[status] ?? { color: 'default', text: status || '未知' };
}

function storyboardRecord(record: Record<string, any>): VideoStoryboard {
  return record as VideoStoryboard;
}

function shotRecord(record: Record<string, any>): StoryboardEditorShot {
  return record as StoryboardEditorShot;
}

function analysisOptions() {
  return analyses.value.map((item) => ({
    label: `${item.videoName || '未命名视频'} · ${item.createTime}`,
    value: item.id,
  }));
}

function splitLines(value: string) {
  return value
    .split('\n')
    .map((item) => item.trim())
    .filter(Boolean);
}

function joinLines(values: string[]) {
  return values.join('\n');
}

function updateShotList(
  shot: VideoStoryboardShot,
  key: 'assets' | 'characters',
  event: Event,
) {
  const target = event.target as HTMLTextAreaElement | null;
  shot[key] = splitLines(target?.value || '');
}

function shotKey(index: number) {
  return `${Date.now()}-${index}-${Math.random().toString(36).slice(2, 8)}`;
}

function emptyShot(index: number): StoryboardEditorShot {
  return {
    _key: shotKey(index),
    action: '',
    assets: [],
    audio: '',
    camera: '',
    characters: [],
    composition: '',
    dialogue: '',
    duration: 3,
    index,
    lighting: '',
    scene: '',
    seedancePrompt: '',
    title: '',
  };
}

async function loadAnalyses() {
  const result = await getVideoAnalysisJobsApi({
    page: 1,
    pageSize: 100,
    status: 'completed',
  });
  analyses.value = result.items;
}

async function loadStoryboards() {
  loading.value = true;
  try {
    const result = await getVideoStoryboardsApi({
      keyword: query.keyword || undefined,
      page: query.page,
      pageSize: query.pageSize,
      status: query.status,
    });
    storyboards.value = result.items;
    total.value = result.total;
  } finally {
    loading.value = false;
  }
}

async function createStoryboard() {
  if (!form.analysisJobId) {
    message.warning('请选择已完成的视频分析记录');
    return;
  }
  if (!form.theme.trim()) {
    message.warning('请输入分镜主题');
    return;
  }
  creating.value = true;
  try {
    await createVideoStoryboardApi({
      analysisJobId: form.analysisJobId,
      theme: form.theme,
      title: form.title || undefined,
    });
    message.success('已创建分镜设计任务，请稍后刷新查看结果');
    form.theme = '';
    form.title = '';
    await loadStoryboards();
    pollStoryboards();
  } finally {
    creating.value = false;
  }
}

function openEditor(record: VideoStoryboard) {
  editor.id = record.id;
  editor.title = record.title;
  editor.theme = record.theme;
  editor.globalPrompt = record.globalPrompt;
  editor.styleGuideText = joinLines(record.styleGuide);
  editor.shots = record.shots.map((shot) => ({
    ...emptyShot(shot.index),
    ...shot,
    _key: shotKey(shot.index),
    assets: [...(shot.assets || [])],
    characters: [...(shot.characters || [])],
  }));
  editorOpen.value = true;
}

function addShot() {
  editor.shots.push(emptyShot(editor.shots.length + 1));
}

function removeShot(index: number) {
  editor.shots.splice(index, 1);
  editor.shots.forEach((shot, i) => {
    shot.index = i + 1;
  });
}

async function saveStoryboard() {
  if (!editor.title.trim() || !editor.theme.trim()) {
    message.warning('请填写标题和主题');
    return;
  }
  saving.value = true;
  try {
    await updateVideoStoryboardApi(editor.id, {
      globalPrompt: editor.globalPrompt,
      shots: editor.shots.map(({ _key, ...shot }) => shot),
      styleGuide: splitLines(editor.styleGuideText),
      theme: editor.theme,
      title: editor.title,
    });
    message.success('分镜方案已保存');
    editorOpen.value = false;
    await loadStoryboards();
  } finally {
    saving.value = false;
  }
}

function pollStoryboards(times = 6) {
  let remain = times;
  const timer = window.setInterval(async () => {
    remain -= 1;
    await loadStoryboards();
    const stillRunning = storyboards.value.some((item) =>
      ['queued', 'running'].includes(item.status),
    );
    if (remain <= 0 || !stillRunning) {
      window.clearInterval(timer);
    }
  }, 5000);
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

async function copyText(value: string, label = '文案') {
  if (!value.trim()) return;
  if (await copyClipboardText(value)) {
    message.success(`${label}已复制`);
    return;
  }
  message.error('复制失败，请手动复制');
}

async function copyAllShots(record: VideoStoryboard) {
  const text = record.shots
    .map((shot) => `镜头 ${shot.index}：${shot.seedancePrompt}`)
    .join('\n\n');
  await copyText(text, '整套分镜');
}

async function retryStoryboard(record: VideoStoryboard) {
  retryingId.value = record.id;
  try {
    await retryVideoStoryboardApi(record.id);
    message.success('已重新提交分镜设计任务');
    await loadStoryboards();
  } finally {
    retryingId.value = '';
  }
}

async function removeStoryboard(record: VideoStoryboard) {
  await deleteVideoStoryboardApi(record.id);
  message.success('分镜方案已删除');
  await loadStoryboards();
}

function handleTableChange(pagination: {
  current?: number;
  pageSize?: number;
}) {
  query.page = pagination.current ?? 1;
  query.pageSize = pagination.pageSize ?? 10;
  loadStoryboards();
}

function search() {
  query.page = 1;
  loadStoryboards();
}

function handleStatusChange() {
  query.page = 1;
  loadStoryboards();
}

onMounted(async () => {
  await Promise.all([loadAnalyses(), loadStoryboards()]);
});
</script>

<template>
  <Page
    description="基于已完成的视频分析和指定主题，生成可编辑的 Seedance 2.0 分镜方案。"
    title="分镜设计"
  >
    <Row :gutter="[16, 16]">
      <Col :lg="8" :xs="24">
        <Card :bordered="false" class="storyboard-card">
          <div class="card-title">创建分镜方案</div>
          <Form layout="vertical">
            <Form.Item label="视频分析记录" required>
              <Select
                v-model:value="form.analysisJobId"
                :options="analysisOptions()"
                placeholder="选择已完成的视频分析"
                show-search
              />
            </Form.Item>
            <Form.Item label="方案标题">
              <Input
                v-model:value="form.title"
                placeholder="例如：九型课程开场分镜"
              />
            </Form.Item>
            <Form.Item label="主题/方向" required>
              <Input.TextArea
                v-model:value="form.theme"
                :rows="4"
                placeholder="例如：围绕九型人格自我认知主题，设计一个温暖、有课程感的开场短片"
              />
            </Form.Item>
            <Button
              :loading="creating"
              type="primary"
              @click="createStoryboard"
            >
              <IconifyIcon class="mr-1" icon="lucide:panels-top-left" />
              生成分镜设计
            </Button>
          </Form>
        </Card>
      </Col>

      <Col :lg="16" :xs="24">
        <Card :bordered="false" class="storyboard-card">
          <div class="history-head">
            <div>
              <div class="card-title">分镜方案</div>
              <div class="card-desc">
                共 {{ total }} 套方案，可编辑镜头并复制提示词。
              </div>
            </div>
            <Space wrap>
              <Input
                v-model:value="query.keyword"
                allow-clear
                class="keyword-input"
                placeholder="搜索标题/主题"
                @press-enter="search"
              />
              <Select
                v-model:value="query.status"
                :options="statusOptions"
                allow-clear
                class="status-select"
                placeholder="全部状态"
                @change="handleStatusChange"
              />
              <Button type="primary" @click="search">查询</Button>
              <Button :loading="loading" @click="loadStoryboards">刷新</Button>
            </Space>
          </div>
          <Table
            :columns="columns"
            :data-source="storyboards"
            :loading="loading"
            :pagination="{
              current: query.page,
              pageSize: query.pageSize,
              showSizeChanger: true,
              total,
            }"
            :scroll="{ x: 1490 }"
            row-key="id"
            @change="handleTableChange"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'status'">
                <Tag :color="statusTag(record.status).color">
                  {{ statusTag(record.status).text }}
                </Tag>
                <div v-if="record.errorMessage" class="error-text">
                  {{ record.errorMessage }}
                </div>
              </template>
              <template v-else-if="column.dataIndex === 'shots'">
                {{ storyboardRecord(record).shots.length }} 镜
              </template>
              <template v-else-if="column.dataIndex === 'globalPrompt'">
                <div class="prompt-preview">
                  {{ record.globalPrompt || '-' }}
                </div>
              </template>
              <template v-else-if="column.key === 'storyboardOperate'">
                <Space wrap>
                  <Button
                    size="small"
                    type="link"
                    @click="openEditor(storyboardRecord(record))"
                  >
                    编辑
                  </Button>
                  <Button
                    v-if="storyboardRecord(record).shots.length"
                    size="small"
                    type="link"
                    @click="copyAllShots(storyboardRecord(record))"
                  >
                    复制整套
                  </Button>
                  <Button
                    v-if="record.status === 'failed'"
                    :loading="retryingId === record.id"
                    size="small"
                    type="link"
                    @click="retryStoryboard(storyboardRecord(record))"
                  >
                    重试
                  </Button>
                  <Popconfirm
                    title="确定删除这套分镜方案吗？"
                    @confirm="removeStoryboard(storyboardRecord(record))"
                  >
                    <Button danger size="small" type="link">删除</Button>
                  </Popconfirm>
                </Space>
              </template>
            </template>
          </Table>
        </Card>
      </Col>
    </Row>

    <Modal
      v-model:open="editorOpen"
      :confirm-loading="saving"
      :width="1180"
      destroy-on-close
      title="编辑分镜方案"
      @ok="saveStoryboard"
    >
      <Form layout="vertical">
        <Row :gutter="12">
          <Col :md="12" :xs="24">
            <Form.Item label="标题" required>
              <Input v-model:value="editor.title" />
            </Form.Item>
          </Col>
          <Col :md="12" :xs="24">
            <Form.Item label="主题" required>
              <Input v-model:value="editor.theme" />
            </Form.Item>
          </Col>
        </Row>
        <Form.Item label="统一风格">
          <Input.TextArea
            v-model:value="editor.styleGuideText"
            :auto-size="{ minRows: 2, maxRows: 4 }"
            placeholder="每行一个风格要点"
          />
        </Form.Item>
        <Form.Item label="全局 Seedance 提示词">
          <Input.TextArea
            v-model:value="editor.globalPrompt"
            :auto-size="{ minRows: 2, maxRows: 5 }"
          />
          <Button
            v-if="editor.globalPrompt"
            class="copy-button"
            size="small"
            type="link"
            @click="copyText(editor.globalPrompt, '全局提示词')"
          >
            <IconifyIcon class="mr-1" icon="lucide:copy" />
            复制
          </Button>
        </Form.Item>
      </Form>

      <div class="shot-toolbar">
        <Button @click="addShot">
          <IconifyIcon class="mr-1" icon="lucide:plus" />
          添加镜头
        </Button>
      </div>
      <Table
        :columns="shotColumns"
        :data-source="editor.shots"
        :pagination="false"
        :scroll="{ x: 3310, y: 420 }"
        row-key="_key"
        size="small"
      >
        <template #bodyCell="{ column, record, index }">
          <template v-if="column.dataIndex === 'index'">
            {{ shotRecord(record).index }}
          </template>
          <template v-else-if="column.dataIndex === 'duration'">
            <InputNumber
              v-model:value="shotRecord(record).duration"
              :min="0"
              :precision="1"
              :step="0.5"
              class="duration-input"
            />
          </template>
          <template v-else-if="column.dataIndex === 'title'">
            <Input v-model:value="shotRecord(record).title" />
          </template>
          <template v-else-if="column.dataIndex === 'scene'">
            <Input.TextArea
              v-model:value="shotRecord(record).scene"
              :auto-size="{ minRows: 2, maxRows: 4 }"
            />
          </template>
          <template v-else-if="column.dataIndex === 'characters'">
            <Input.TextArea
              :value="joinLines(shotRecord(record).characters)"
              :auto-size="{ minRows: 2, maxRows: 4 }"
              placeholder="每行一个人物/主体"
              @change="updateShotList(shotRecord(record), 'characters', $event)"
            />
          </template>
          <template v-else-if="column.dataIndex === 'assets'">
            <Input.TextArea
              :value="joinLines(shotRecord(record).assets)"
              :auto-size="{ minRows: 2, maxRows: 4 }"
              placeholder="每行一个资产"
              @change="updateShotList(shotRecord(record), 'assets', $event)"
            />
          </template>
          <template v-else-if="column.dataIndex === 'action'">
            <Input.TextArea
              v-model:value="shotRecord(record).action"
              :auto-size="{ minRows: 2, maxRows: 4 }"
            />
          </template>
          <template v-else-if="column.dataIndex === 'camera'">
            <Input.TextArea
              v-model:value="shotRecord(record).camera"
              :auto-size="{ minRows: 2, maxRows: 4 }"
            />
          </template>
          <template v-else-if="column.dataIndex === 'composition'">
            <Input.TextArea
              v-model:value="shotRecord(record).composition"
              :auto-size="{ minRows: 2, maxRows: 4 }"
            />
          </template>
          <template v-else-if="column.dataIndex === 'lighting'">
            <Input.TextArea
              v-model:value="shotRecord(record).lighting"
              :auto-size="{ minRows: 2, maxRows: 4 }"
            />
          </template>
          <template v-else-if="column.dataIndex === 'audio'">
            <Input.TextArea
              v-model:value="shotRecord(record).audio"
              :auto-size="{ minRows: 2, maxRows: 4 }"
            />
          </template>
          <template v-else-if="column.dataIndex === 'dialogue'">
            <Input.TextArea
              v-model:value="shotRecord(record).dialogue"
              :auto-size="{ minRows: 2, maxRows: 4 }"
            />
          </template>
          <template v-else-if="column.dataIndex === 'seedancePrompt'">
            <Input.TextArea
              v-model:value="shotRecord(record).seedancePrompt"
              :auto-size="{ minRows: 3, maxRows: 6 }"
            />
            <Button
              v-if="shotRecord(record).seedancePrompt"
              class="copy-button"
              size="small"
              type="link"
              @click="
                copyText(
                  shotRecord(record).seedancePrompt,
                  `镜头 ${shotRecord(record).index}`,
                )
              "
            >
              复制
            </Button>
          </template>
          <template v-else-if="column.key === 'shotOperate'">
            <Button danger size="small" type="link" @click="removeShot(index)">
              删除
            </Button>
          </template>
        </template>
      </Table>
    </Modal>
  </Page>
</template>

<style scoped>
.storyboard-card {
  border-radius: 8px;
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

.keyword-input {
  width: 220px;
}

.status-select {
  width: 120px;
}

.prompt-preview {
  max-height: 72px;
  overflow: auto;
  font-size: 13px;
  line-height: 1.6;
  color: hsl(var(--foreground));
  white-space: pre-wrap;
}

.error-text {
  max-width: 180px;
  margin-top: 6px;
  font-size: 12px;
  color: #ef4444;
  white-space: normal;
}

.shot-toolbar {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 12px;
}

.duration-input {
  width: 78px;
}

.copy-button {
  height: 24px;
  padding: 0;
  font-size: 12px;
}

@media (max-width: 768px) {
  .history-head {
    display: block;
  }

  .history-head :deep(.ant-space) {
    width: 100%;
    margin-top: 12px;
  }

  .keyword-input,
  .status-select {
    width: 100%;
  }
}
</style>
