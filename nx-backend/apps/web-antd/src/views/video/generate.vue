<script setup lang="ts">
import type { VideoAsset, VideoAssetType, VideoGeneration } from '#/api';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { IconifyIcon } from '@vben/icons';
import { useAccessStore } from '@vben/stores';

import {
  Button,
  Card,
  Col,
  Form,
  message,
  Row,
  Select,
  Table,
  Tag,
  Tooltip,
  Upload,
} from 'ant-design-vue';

import {
  generateVideoApi,
  getModelConfigApi,
  getVideoGenerationsApi,
  polishPromptApi,
  refreshVideoGenerationApi,
  uploadFileApi,
} from '#/api';

import { withPreviewToken } from './asset-preview';
import AssetPicker from './components/AssetPicker.vue';

// @ 资产的内联彩色标签样式：不同类型不同配色，与资产库 Tag 颜色保持一致。
const CHIP_STYLE: Record<VideoAssetType, { bg: string; fg: string }> = {
  scene: { bg: '#e6f0ff', fg: '#1d4ed8' },
  character: { bg: '#e7f6ec', fg: '#15803d' },
  prop: { bg: '#fffbeb', fg: '#b45309' },
  outfit: { bg: '#fdf4ff', fg: '#a21caf' },
  style: { bg: '#ecfeff', fg: '#0e7490' },
  audio: { bg: '#fff2e0', fg: '#c2410c' },
  video: { bg: '#f3e8ff', fg: '#7c3aed' },
};

const loading = ref(false);
const generating = ref(false);
const polishing = ref(false);
const refreshingId = ref('');
const accessStore = useAccessStore();
const generations = ref<VideoGeneration[]>([]);
const total = ref(0);
const latest = ref<VideoGeneration>();

const query = reactive({
  page: 1,
  pageSize: 10,
});

const form = reactive({
  aspectRatio: '16:9',
  audios: [] as string[],
  images: [] as string[],
  model: 'video-ds-2.0-fast',
  seconds: 15,
  videos: [] as string[],
});

const audioUploading = ref(false);
const imageUploading = ref(false);
const videoUploading = ref(false);

// 参考图片上限：最多 14 张。
const MAX_IMAGES = 14;

// 参考素材的「从资产库选择」弹窗：按区分配可选类型，选中后写入对应数组。
const refPickerOpen = ref(false);
const refPickerTarget = ref<'audio' | 'image' | 'video'>('image');
const refPickerTypes = computed<VideoAssetType[]>(() => {
  switch (refPickerTarget.value) {
    case 'audio': {
      return ['audio'];
    }
    case 'video': {
      return ['video'];
    }
    // 图片参考来自场景 / 人物 / 物品 / 服装 / 风格五类素材。
    default: {
      return ['scene', 'character', 'prop', 'outfit', 'style'];
    }
  }
});

function openRefPicker(target: 'audio' | 'image' | 'video') {
  refPickerTarget.value = target;
  refPickerOpen.value = true;
}

// 从资产库选中参考素材后，按当前区追加到对应数组（去重 + 图片上限校验）。
function onPickRefAsset(asset: VideoAsset) {
  if (refPickerTarget.value === 'image') {
    if (form.images.includes(asset.url)) return;
    if (form.images.length >= MAX_IMAGES) {
      message.warning(`参考图片最多 ${MAX_IMAGES} 张`);
      return;
    }
    form.images.push(asset.url);
  } else if (refPickerTarget.value === 'video') {
    if (!form.videos.includes(asset.url)) form.videos.push(asset.url);
  } else if (!form.audios.includes(asset.url)) {
    form.audios.push(asset.url);
  }
}

// 富文本提示词编辑器：contenteditable，@资产 以内联彩色标签呈现。
const editorRef = ref<HTMLDivElement>();
const pickerOpen = ref(false);
// 记录失焦前的光标位置，插入标签时还原，避免标签总是落到末尾。
let savedRange: null | Range = null;

function saveSelection() {
  const sel = window.getSelection();
  if (!sel || sel.rangeCount === 0) return;
  const range = sel.getRangeAt(0);
  if (editorRef.value?.contains(range.commonAncestorContainer)) {
    savedRange = range.cloneRange();
  }
}

// 监听 @ 键：直接弹出资产库，省去额外按钮的操作成本（按钮也保留）。
function handleEditorKeydown(event: KeyboardEvent) {
  if (event.key === '@') {
    event.preventDefault();
    saveSelection();
    pickerOpen.value = true;
  }
}

function openPicker() {
  saveSelection();
  pickerOpen.value = true;
}

// 选中资产后插入一个不可编辑的彩色标签，并在其后补一个空格便于继续输入。
function onPickAsset(asset: VideoAsset) {
  const el = editorRef.value;
  if (!el) return;
  const style = CHIP_STYLE[asset.type];
  const chip = document.createElement('span');
  chip.className = 'asset-chip';
  chip.contentEditable = 'false';
  chip.dataset.type = asset.type;
  chip.dataset.url = asset.url;
  chip.dataset.name = asset.name;
  chip.textContent = `@${asset.name}`;
  chip.style.backgroundColor = style.bg;
  chip.style.color = style.fg;

  const sel = window.getSelection();
  let range = savedRange;
  if (!range || !el.contains(range.commonAncestorContainer)) {
    el.focus();
    range = document.createRange();
    range.selectNodeContents(el);
    range.collapse(false);
  }
  range.deleteContents();
  range.insertNode(chip);
  const spacer = document.createTextNode(' ');
  chip.after(spacer);
  // 光标移动到空格之后。
  range.setStartAfter(spacer);
  range.collapse(true);
  sel?.removeAllRanges();
  sel?.addRange(range);
  savedRange = range.cloneRange();
  el.focus();
}

// 序列化编辑器 DOM：还原纯文本提示词，并按类型收集 @资产 的 URL。
function readEditor(): {
  audios: string[];
  images: string[];
  prompt: string;
  videos: string[];
} {
  const buckets: Record<VideoAssetType, Set<string>> = {
    audio: new Set(),
    character: new Set(),
    outfit: new Set(),
    prop: new Set(),
    scene: new Set(),
    style: new Set(),
    video: new Set(),
  };
  let text = '';

  const walk = (node: Node) => {
    if (node.nodeType === Node.TEXT_NODE) {
      text += (node.textContent ?? '').replaceAll(' ', ' ');
      return;
    }
    if (node.nodeType !== Node.ELEMENT_NODE) return;
    const el = node as HTMLElement;
    if (el.classList.contains('asset-chip')) {
      const name = el.dataset.name ?? el.textContent?.replace(/^@/, '') ?? '';
      text += `@${name}`;
      const type = el.dataset.type as undefined | VideoAssetType;
      const url = el.dataset.url;
      if (type && url) buckets[type].add(url);
      return;
    }
    if (el.tagName === 'BR') {
      text += '\n';
      return;
    }
    const isBlock = el.tagName === 'DIV' || el.tagName === 'P';
    if (isBlock && text && !text.endsWith('\n')) text += '\n';
    el.childNodes.forEach((child) => walk(child));
  };

  editorRef.value?.childNodes.forEach((child) => walk(child));

  const unique = (...lists: string[][]) => [...new Set(lists.flat())];
  return {
    // 音频标签 → audios。
    audios: unique(form.audios, [...buckets.audio]),
    // 场景/人物/物品/服装/风格标签均为图片素材 → images。
    images: unique(
      form.images,
      [...buckets.scene],
      [...buckets.character],
      [...buckets.prop],
      [...buckets.outfit],
      [...buckets.style],
    ),
    prompt: text.trim(),
    // 视频标签 → videos。
    videos: unique(form.videos, [...buckets.video]),
  };
}

function setEditorText(value: string) {
  if (!editorRef.value) return;
  editorRef.value.textContent = value;
}

async function polishPrompt() {
  const { prompt } = readEditor();
  if (!prompt) {
    message.warning('请先填写一个方向或草稿提示词');
    return;
  }
  polishing.value = true;
  try {
    const result = await polishPromptApi({ kind: 'video', prompt });
    setEditorText(result.prompt);
    message.success('提示词已润色');
  } catch (error: any) {
    const errorMessage =
      error?.response?.data?.error ||
      error?.response?.data?.message ||
      error?.message ||
      '润色失败，请稍后重试';
    message.error(errorMessage);
  } finally {
    polishing.value = false;
  }
}

// 外部视频网关只能拉取公网 objectUrl，本地代理 url 仅用于后台预览。
function requirePublicObjectUrl(
  res: { objectUrl?: string; url: string },
  label: string,
) {
  if (
    res.objectUrl?.startsWith('http://') ||
    res.objectUrl?.startsWith('https://')
  ) {
    return res.objectUrl;
  }
  throw new Error(
    `${label}需要文件桶公网地址，请配置 OSS_PUBLIC_URL 后重新上传`,
  );
}

// 参考图片上传：写入 form.images，再随生成参数传给网关的 images 数组。
async function handleImageUpload(file: File) {
  if (form.images.length >= MAX_IMAGES) {
    message.warning(`参考图片最多 ${MAX_IMAGES} 张`);
    return false;
  }
  imageUploading.value = true;
  try {
    const res = await uploadFileApi(file, 'video');
    form.images.push(requirePublicObjectUrl(res, '参考图片'));
    message.success(`参考图片「${file.name}」上传成功`);
  } catch (error: any) {
    message.error(error?.message || `参考图片「${file.name}」上传失败`);
  } finally {
    imageUploading.value = false;
  }
  return false;
}

// 参考视频上传：写入 form.videos，再随生成参数传给网关的 videos 数组。
async function handleVideoUpload(file: File) {
  videoUploading.value = true;
  try {
    const res = await uploadFileApi(file, 'video');
    form.videos.push(requirePublicObjectUrl(res, '参考视频'));
    message.success(`参考视频「${file.name}」上传成功`);
  } catch (error: any) {
    message.error(error?.message || `参考视频「${file.name}」上传失败`);
  } finally {
    videoUploading.value = false;
  }
  return false;
}

// 参考音频上传：写入 form.audios，再随生成参数传给网关的 audios 数组。
async function handleAudioUpload(file: File) {
  audioUploading.value = true;
  try {
    const res = await uploadFileApi(file, 'video');
    form.audios.push(requirePublicObjectUrl(res, '参考音频'));
    message.success(`参考音频「${file.name}」上传成功`);
  } catch (error: any) {
    message.error(error?.message || `参考音频「${file.name}」上传失败`);
  } finally {
    audioUploading.value = false;
  }
  return false;
}

function removeImage(index: number) {
  form.images.splice(index, 1);
}

function removeVideo(index: number) {
  form.videos.splice(index, 1);
}

function removeAudio(index: number) {
  form.audios.splice(index, 1);
}

const modelOptions = [
  { label: 'video-ds-2.0-fast（快速）', value: 'video-ds-2.0-fast' },
  { label: 'video-ds-2.0（标准）', value: 'video-ds-2.0' },
];

const secondsOptions = [
  { label: '5 秒', value: 5 },
  { label: '10 秒', value: 10 },
  { label: '15 秒', value: 15 },
];

const aspectRatioOptions = [
  { label: '16:9', value: '16:9' },
  { label: '9:16', value: '9:16' },
  { label: '1:1', value: '1:1' },
];

const statusMeta: Record<string, { color: string; text: string }> = {
  completed: { color: 'success', text: '已完成' },
  failed: { color: 'error', text: '失败' },
  in_progress: { color: 'processing', text: '生成中' },
  queued: { color: 'warning', text: '排队中' },
  succeeded: { color: 'success', text: '已完成' },
  unknown: { color: 'default', text: '未知' },
};

function statusTag(status: string) {
  return statusMeta[status] ?? { color: 'default', text: status || '未知' };
}

const isPending = (status: string) =>
  status === 'queued' || status === 'in_progress';

const canRefresh = (record: VideoGeneration) =>
  isPending(record.status) || (!!record.taskId && !record.videoUrl);

const isSuccessStatus = (status: string) =>
  status === 'completed' || status === 'succeeded';

const generationParams = (record: VideoGeneration) =>
  `${record.seconds || '-'} 秒 · ${record.aspectRatio || '-'}`;

const previewVideoUrl = (url?: string) =>
  url ? withPreviewToken(url, accessStore.accessToken) : '';

const columns = [
  { dataIndex: 'prompt', ellipsis: true, title: '提示词' },
  { dataIndex: 'taskId', ellipsis: true, title: '任务 ID', width: 260 },
  { dataIndex: 'model', title: '模型', width: 150 },
  { dataIndex: 'params', title: '参数', width: 130 },
  { dataIndex: 'videoUrl', title: '视频', width: 260 },
  { dataIndex: 'status', title: '状态', width: 110 },
  { dataIndex: 'errorMessage', ellipsis: true, title: '失败原因', width: 180 },
  { dataIndex: 'createTime', title: '生成时间', width: 180 },
  { dataIndex: 'action', title: '操作', width: 100 },
];

async function loadGenerations() {
  loading.value = true;
  try {
    const result = await getVideoGenerationsApi({
      page: query.page,
      pageSize: query.pageSize,
    });
    generations.value = result.items.filter((item) => item.taskId);
    total.value = result.total;
  } finally {
    loading.value = false;
  }
}

async function loadModelConfig() {
  try {
    const config = await getModelConfigApi();
    const model = config.video?.model?.trim();
    if (model) {
      form.model = model;
    }
  } catch {
    // 模型配置读取失败时保留页面默认值。
  }
}

async function generate() {
  const { audios, images, prompt, videos } = readEditor();
  if (!prompt) {
    message.warning('请输入视频提示词');
    return;
  }
  generating.value = true;
  try {
    const result = await generateVideoApi({
      aspectRatio: form.aspectRatio,
      audios: audios.length > 0 ? audios : undefined,
      images: images.length > 0 ? images : undefined,
      model: form.model,
      prompt,
      seconds: form.seconds,
      videos: videos.length > 0 ? videos : undefined,
    });
    latest.value = result;
    message.success('已提交生成任务，稍后可刷新查看结果');
    await loadGenerations();
  } finally {
    generating.value = false;
  }
}

async function refresh(record: VideoGeneration) {
  refreshingId.value = record.id;
  try {
    const result = await refreshVideoGenerationApi(record.id);
    if (latest.value?.id === result.id) {
      latest.value = result;
    }
    const tip = statusTag(result.status).text;
    message.success(`任务状态：${tip}`);
    await loadGenerations();
  } finally {
    refreshingId.value = '';
  }
}

function handleTableChange(pagination: {
  current?: number;
  pageSize?: number;
}) {
  query.page = pagination.current ?? 1;
  query.pageSize = pagination.pageSize ?? 10;
  loadGenerations();
}

const latestStatus = computed(() =>
  latest.value ? statusTag(latest.value.status) : undefined,
);

onMounted(() => {
  loadModelConfig();
  loadGenerations();
});
</script>

<template>
  <Page
    description="输入提示词（可选参考图片/视频/音频），调用视频模型生成视频。任务为异步，提交后可刷新查看进度。"
    title="视频生成"
  >
    <Row :gutter="[16, 16]">
      <Col :lg="9" :xs="24">
        <Card :bordered="false" class="video-card">
          <div class="card-title">创建生成任务</div>
          <Form layout="vertical">
            <Form.Item label="模型">
              <Select v-model:value="form.model" :options="modelOptions" />
            </Form.Item>
            <Form.Item label="视频时长">
              <Select v-model:value="form.seconds" :options="secondsOptions" />
            </Form.Item>
            <Form.Item label="画幅比例">
              <Select
                v-model:value="form.aspectRatio"
                :options="aspectRatioOptions"
              />
            </Form.Item>
            <Form.Item :label="`参考图片（可选，最多 ${MAX_IMAGES} 张）`">
              <div class="ref-actions">
                <Upload
                  :before-upload="handleImageUpload"
                  :show-upload-list="false"
                  accept="image/*"
                >
                  <Button :loading="imageUploading">
                    <IconifyIcon class="mr-1" icon="lucide:image-plus" />
                    上传参考图片
                  </Button>
                </Upload>
                <Button @click="openRefPicker('image')">
                  <IconifyIcon class="mr-1" icon="lucide:folder-open" />
                  从资产库选择
                </Button>
                <span class="ref-count"
                  >{{ form.images.length }} / {{ MAX_IMAGES }}</span
                >
              </div>
              <div v-if="form.images.length > 0" class="upload-list">
                <div
                  v-for="(item, index) in form.images"
                  :key="item"
                  class="upload-item"
                >
                  <span class="upload-name" :title="item">{{ item }}</span>
                  <Button
                    danger
                    size="small"
                    type="text"
                    @click="removeImage(index)"
                  >
                    移除
                  </Button>
                </div>
              </div>
            </Form.Item>
            <Form.Item label="参考视频（可选）">
              <div class="ref-actions">
                <Upload
                  :before-upload="handleVideoUpload"
                  :show-upload-list="false"
                  accept="video/*"
                >
                  <Button :loading="videoUploading">
                    <IconifyIcon class="mr-1" icon="lucide:film" />
                    上传参考视频
                  </Button>
                </Upload>
                <Button @click="openRefPicker('video')">
                  <IconifyIcon class="mr-1" icon="lucide:folder-open" />
                  从资产库选择
                </Button>
              </div>
              <div v-if="form.videos.length > 0" class="upload-list">
                <div
                  v-for="(item, index) in form.videos"
                  :key="item"
                  class="upload-item"
                >
                  <span class="upload-name" :title="item">{{ item }}</span>
                  <Button
                    danger
                    size="small"
                    type="text"
                    @click="removeVideo(index)"
                  >
                    移除
                  </Button>
                </div>
              </div>
            </Form.Item>
            <Form.Item label="参考音频（可选）">
              <div class="ref-actions">
                <Upload
                  :before-upload="handleAudioUpload"
                  :show-upload-list="false"
                  accept="audio/*"
                >
                  <Button :loading="audioUploading">
                    <IconifyIcon class="mr-1" icon="lucide:file-audio" />
                    上传参考音频
                  </Button>
                </Upload>
                <Button @click="openRefPicker('audio')">
                  <IconifyIcon class="mr-1" icon="lucide:folder-open" />
                  从资产库选择
                </Button>
              </div>
              <div v-if="form.audios.length > 0" class="upload-list">
                <div
                  v-for="(item, index) in form.audios"
                  :key="item"
                  class="upload-item"
                >
                  <span class="upload-name" :title="item">{{ item }}</span>
                  <Button
                    danger
                    size="small"
                    type="text"
                    @click="removeAudio(index)"
                  >
                    移除
                  </Button>
                </div>
              </div>
            </Form.Item>
            <Form.Item required>
              <template #label>
                <div class="prompt-label">
                  <span>提示词</span>
                  <Tooltip title="一键润色">
                    <Button
                      :loading="polishing"
                      size="small"
                      type="text"
                      @click="polishPrompt"
                    >
                      <IconifyIcon icon="lucide:sparkles" />
                    </Button>
                  </Tooltip>
                </div>
              </template>
              <div class="prompt-editor-wrap">
                <div
                  ref="editorRef"
                  class="prompt-editor"
                  contenteditable="true"
                  data-placeholder="描述想要生成的视频画面、镜头与风格，输入 @ 或点击下方按钮插入资产"
                  @keydown="handleEditorKeydown"
                ></div>
                <div class="prompt-editor-toolbar">
                  <Button size="small" @click="openPicker">
                    <IconifyIcon class="mr-1" icon="lucide:at-sign" />
                    插入资产
                  </Button>
                  <span class="prompt-editor-tip"
                    >支持 @ 场景/人物/音频/视频资产，不同类型以颜色区分</span
                  >
                </div>
              </div>
            </Form.Item>
            <Button :loading="generating" type="primary" @click="generate">
              <IconifyIcon class="mr-1" icon="lucide:wand-sparkles" />
              提交生成
            </Button>
          </Form>
        </Card>
      </Col>

      <Col :lg="15" :xs="24">
        <Card :bordered="false" class="video-card result-card">
          <div class="card-title">最新任务</div>
          <div v-if="latest" class="latest-result">
            <div class="latest-head">
              <Tag :color="latestStatus?.color">{{ latestStatus?.text }}</Tag>
              <span class="latest-model">{{ latest.model }}</span>
            </div>
            <div class="latest-text">{{ latest.prompt }}</div>
            <video
              v-if="latest.videoUrl"
              :src="previewVideoUrl(latest.videoUrl)"
              class="latest-video"
              controls
            ></video>
            <div v-if="latest" class="latest-meta">
              {{ latest.seconds }} 秒 · {{ latest.aspectRatio }}
            </div>
            <div
              v-if="latest.status === 'failed' && latest.errorMessage"
              class="latest-error"
            >
              {{ latest.errorMessage }}
            </div>
            <div
              v-else-if="!isSuccessStatus(latest.status)"
              class="latest-pending"
            >
              任务处理中，请点击下方记录中的「刷新」获取最新进度。
            </div>
          </div>
          <div v-else class="empty-result">提交后会在这里展示最新任务。</div>
        </Card>

        <Card :bordered="false" class="video-card history-card">
          <div class="history-head">
            <div>
              <div class="card-title">生成记录</div>
              <div class="card-desc">
                共 {{ total }} 条记录，完成后可在线预览。
              </div>
            </div>
            <Button :loading="loading" @click="loadGenerations"
              >刷新列表</Button
            >
          </div>
          <Table
            :columns="columns"
            :data-source="generations"
            :loading="loading"
            :pagination="{
              current: query.page,
              pageSize: query.pageSize,
              showSizeChanger: true,
              total,
            }"
            :scroll="{ x: 1260 }"
            row-key="id"
            @change="handleTableChange"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'taskId'">
                <span class="task-id" :title="record.taskId">
                  {{ record.taskId || '-' }}
                </span>
              </template>
              <template v-else-if="column.dataIndex === 'params'">
                <span class="generation-params">
                  {{ generationParams(record as VideoGeneration) }}
                </span>
              </template>
              <template v-else-if="column.dataIndex === 'videoUrl'">
                <video
                  v-if="record.videoUrl"
                  :src="previewVideoUrl(record.videoUrl)"
                  class="row-video"
                  controls
                ></video>
                <span v-else>-</span>
              </template>
              <template v-else-if="column.dataIndex === 'status'">
                <Tag :color="statusTag(record.status).color">
                  {{ statusTag(record.status).text }}
                </Tag>
              </template>
              <template v-else-if="column.dataIndex === 'errorMessage'">
                <Tooltip
                  v-if="record.status === 'failed' && record.errorMessage"
                  :title="record.errorMessage"
                >
                  <span class="error-text">{{ record.errorMessage }}</span>
                </Tooltip>
                <span v-else>-</span>
              </template>
              <template v-else-if="column.dataIndex === 'action'">
                <Button
                  v-if="canRefresh(record as VideoGeneration)"
                  :loading="refreshingId === record.id"
                  size="small"
                  type="link"
                  @click="refresh(record as VideoGeneration)"
                >
                  刷新
                </Button>
                <span v-else>-</span>
              </template>
            </template>
          </Table>
        </Card>
      </Col>
    </Row>
    <AssetPicker v-model:open="pickerOpen" @pick="onPickAsset" />
    <AssetPicker
      v-model:open="refPickerOpen"
      :allow-types="refPickerTypes"
      @pick="onPickRefAsset"
    />
  </Page>
</template>

<style scoped>
.video-card {
  border-radius: 8px;
}

.prompt-editor-wrap {
  width: 100%;
}

.prompt-editor {
  min-height: 150px;
  padding: 8px 11px;
  overflow-y: auto;
  font-size: 14px;
  line-height: 1.7;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
  cursor: text;
  border: 1px solid hsl(var(--border));
  border-radius: 6px;
  transition: border-color 0.2s;
}

.prompt-editor:focus {
  outline: none;
  border-color: hsl(var(--primary));
}

.prompt-editor:empty::before {
  color: hsl(var(--muted-foreground));
  pointer-events: none;
  content: attr(data-placeholder);
}

.prompt-editor :deep(.asset-chip) {
  display: inline-block;
  padding: 1px 8px;
  margin: 0 2px;
  font-size: 13px;
  font-weight: 500;
  vertical-align: baseline;
  white-space: nowrap;
  user-select: none;
  border-radius: 6px;
}

.prompt-editor-toolbar {
  display: flex;
  gap: 10px;
  align-items: center;
  margin-top: 8px;
}

.prompt-editor-tip {
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}

.card-title {
  margin-bottom: 16px;
  font-size: 16px;
  font-weight: 600;
}

.card-desc {
  margin-top: 4px;
  font-size: 13px;
  color: #667085;
}

.prompt-label {
  display: inline-flex;
  gap: 8px;
  align-items: center;
}

.result-card {
  margin-bottom: 16px;
}

.latest-result {
  padding: 16px;
  background: linear-gradient(135deg, #f8fbff 0%, #f8fafc 100%);
  border: 1px solid #e5e7eb;
  border-radius: 8px;
}

.latest-head {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
}

.latest-model {
  font-size: 12px;
  color: #667085;
}

.latest-text {
  margin-bottom: 12px;
  line-height: 1.7;
  color: #475467;
}

.latest-error {
  padding: 12px;
  color: #b42318;
  background: #fef3f2;
  border-radius: 6px;
}

.latest-pending {
  color: #98a2b3;
}

.empty-result {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 128px;
  color: #98a2b3;
  background: #f8fafc;
  border: 1px dashed #d0d5dd;
  border-radius: 8px;
}

.history-head {
  display: flex;
  gap: 16px;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 16px;
}

.latest-video {
  width: 100%;
  max-height: 360px;
  border-radius: 8px;
}

.row-video {
  width: 100%;
  min-width: 220px;
  max-height: 120px;
}

.upload-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 8px;
}

.ref-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.ref-count {
  font-size: 12px;
  color: #98a2b3;
}

.upload-item {
  display: flex;
  gap: 8px;
  align-items: center;
  justify-content: space-between;
  padding: 6px 10px;
  background: #f8fafc;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
}

.upload-name {
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 13px;
  color: #475467;
  white-space: nowrap;
}
</style>
