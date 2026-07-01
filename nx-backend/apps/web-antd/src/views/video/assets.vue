<script setup lang="ts">
import type { UploadChangeParam } from 'ant-design-vue';

import type { VideoAsset, VideoAssetType } from '#/api';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { IconifyIcon } from '@vben/icons';
import { useAccessStore } from '@vben/stores';

import {
  Button,
  Card,
  Col,
  Form,
  Image,
  Input,
  message,
  Modal,
  Row,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  Upload,
} from 'ant-design-vue';

import {
  createAssetApi,
  deleteAssetApi,
  generateImageAssetApi,
  generateVideoApi,
  getModelConfigApi,
  listAssetsApi,
  polishPromptApi,
  refreshVideoGenerationApi,
  uploadFileApi,
} from '#/api';

import {
  getAssetPreviewKind,
  getAssetPreviewSource,
  isImageAssetType,
  withPreviewToken,
} from './asset-preview';

const TabPane = Tabs.TabPane;

const TYPE_META: Record<
  VideoAssetType,
  { accept: string; color: string; label: string }
> = {
  scene: { accept: 'image/*', color: 'blue', label: '场景' },
  character: { accept: 'image/*', color: 'green', label: '人物' },
  prop: { accept: 'image/*', color: 'gold', label: '物品' },
  outfit: { accept: 'image/*', color: 'magenta', label: '服装' },
  style: { accept: 'image/*', color: 'cyan', label: '风格' },
  audio: { accept: 'audio/*', color: 'orange', label: '音频' },
  video: { accept: 'video/*', color: 'purple', label: '视频' },
};
const typeOptions = (Object.keys(TYPE_META) as VideoAssetType[]).map(
  (value) => ({ label: TYPE_META[value].label, value }),
);

const filterTypeOptions = [{ label: '全部类型', value: '' }, ...typeOptions];

const loading = ref(false);
const saving = ref(false);
const accessStore = useAccessStore();
const assets = ref<VideoAsset[]>([]);
const total = ref(0);
const uploadedUrl = ref('');
const uploadedName = ref('');
const uploadedPreviewUrl = computed(() =>
  withPreviewToken(uploadedUrl.value, accessStore.accessToken),
);
const videoPreview = reactive({
  name: '',
  open: false,
  url: '',
});

const query = reactive({
  keyword: '',
  page: 1,
  pageSize: 20,
  type: '' as '' | VideoAssetType,
});

const form = reactive({
  assetId: '',
  coverUrl: '',
  name: '',
  remark: '',
  type: 'scene' as VideoAssetType,
  url: '',
});

const activeTab = ref<'image' | 'upload' | 'video'>('image');

// 文生图（gpt-image-2）
const imageForm = reactive({
  model: 'gpt-image-2',
  name: '',
  prompt: '',
  remark: '',
  size: '1024x1024',
  type: 'scene' as VideoAssetType,
});
const imageGenerating = ref(false);
const imagePolishing = ref(false);
const imageSizeOptions = [
  { label: '1024 × 1024（正方形）', value: '1024x1024' },
  { label: '1024 × 1536（竖图）', value: '1024x1536' },
  { label: '1536 × 1024（横图）', value: '1536x1024' },
  { label: '自动', value: 'auto' },
];
const imageTypeOptions = [
  { label: '场景', value: 'scene' },
  { label: '人物', value: 'character' },
  { label: '物品', value: 'prop' },
  { label: '服装', value: 'outfit' },
  { label: '风格', value: 'style' },
];

// 文生视频（复用视频生成接口）
const videoForm = reactive({
  aspectRatio: '16:9',
  model: 'video-ds-2.0-fast',
  name: '',
  prompt: '',
  remark: '',
  seconds: 15,
});
const videoGenerating = ref(false);
const videoPolishing = ref(false);
const videoProgress = ref('');
const videoModelOptions = [
  { label: 'video-ds-2.0-fast（快速）', value: 'video-ds-2.0-fast' },
  { label: 'video-ds-2.0（标准）', value: 'video-ds-2.0' },
];
const videoSecondsOptions = [
  { label: '5 秒', value: 5 },
  { label: '10 秒', value: 10 },
  { label: '15 秒', value: 15 },
];
const videoAspectRatioOptions = [
  { label: '16:9（横屏）', value: '16:9' },
  { label: '9:16（竖屏）', value: '9:16' },
  { label: '1:1（方形）', value: '1:1' },
];

const isVideoGenerationSucceeded = (status: string) =>
  status === 'completed' || status === 'succeeded';

const columns = [
  { dataIndex: 'type', title: '类型', width: 90 },
  { dataIndex: 'name', title: '资产名称', width: 180 },
  { dataIndex: 'url', title: '预览', width: 220 },
  { dataIndex: 'remark', ellipsis: true, title: '备注' },
  { dataIndex: 'createTime', title: '创建时间', width: 180 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 110 },
];

const currentAccept = computed(() => TYPE_META[form.type].accept);
const canSubmit = computed(() => Boolean(form.name.trim() && form.url));

async function load() {
  loading.value = true;
  try {
    const result = await listAssetsApi({
      keyword: query.keyword || undefined,
      page: query.page,
      pageSize: query.pageSize,
      type: query.type || undefined,
    });
    assets.value = result.items;
    total.value = result.total;
  } finally {
    loading.value = false;
  }
}

function getRawFile(file: UploadChangeParam['file']) {
  return (file.originFileObj || file) as File | undefined;
}

async function uploadAsset({ file }: UploadChangeParam) {
  const rawFile = getRawFile(file);
  if (!rawFile) {
    message.warning('没有读取到文件，请重新选择');
    return;
  }
  saving.value = true;
  try {
    const result = await uploadFileApi(rawFile, `video/${form.type}`);
    form.assetId = String(result.assetId || '');
    form.url = result.objectUrl || result.url;
    if (!form.name.trim()) {
      form.name = result.name || rawFile.name;
    }
    uploadedName.value = result.name || rawFile.name;
    uploadedUrl.value = form.url;
    message.success('资产文件已上传');
  } catch (error: any) {
    form.assetId = '';
    form.url = '';
    uploadedName.value = '';
    uploadedUrl.value = '';
    const errorMessage =
      error?.response?.data?.error ||
      error?.response?.data?.message ||
      error?.message ||
      '文件上传失败，请重新上传';
    message.error(errorMessage);
  } finally {
    saving.value = false;
  }
}

async function submit() {
  if (!canSubmit.value) {
    message.warning('请填写资产名称并上传资产文件');
    return;
  }
  saving.value = true;
  try {
    await createAssetApi({
      assetId: form.assetId || undefined,
      coverUrl: form.coverUrl || undefined,
      name: form.name,
      remark: form.remark || undefined,
      type: form.type,
      url: form.url,
    });
    message.success('资产已保存');
    resetForm();
    await load();
  } finally {
    saving.value = false;
  }
}

function assetRecord(record: Record<string, any>): VideoAsset {
  return record as VideoAsset;
}

async function polishImagePrompt() {
  if (!imageForm.prompt.trim()) {
    message.warning('请先填写一个方向或草稿提示词');
    return;
  }
  imagePolishing.value = true;
  try {
    const { prompt } = await polishPromptApi({
      kind: 'image',
      prompt: imageForm.prompt,
    });
    imageForm.prompt = prompt;
    message.success('提示词已润色');
  } catch (error: any) {
    const errorMessage =
      error?.response?.data?.error ||
      error?.response?.data?.message ||
      error?.message ||
      '润色失败，请稍后重试';
    message.error(errorMessage);
  } finally {
    imagePolishing.value = false;
  }
}

async function generateImage() {
  if (!imageForm.prompt.trim()) {
    message.warning('请填写图片描述（提示词）');
    return;
  }
  if (!imageForm.name.trim()) {
    message.warning('请填写资产名称');
    return;
  }
  imageGenerating.value = true;
  try {
    await generateImageAssetApi({
      model: imageForm.model || undefined,
      name: imageForm.name,
      prompt: imageForm.prompt,
      remark: imageForm.remark || undefined,
      size: imageForm.size || undefined,
      type: imageForm.type,
    });
    message.success('文生图资产已生成并入库');
    imageForm.prompt = '';
    imageForm.name = '';
    imageForm.remark = '';
    await load();
  } catch (error: any) {
    const errorMessage =
      error?.response?.data?.error ||
      error?.response?.data?.message ||
      error?.message ||
      '文生图失败，请稍后重试';
    message.error(errorMessage);
  } finally {
    imageGenerating.value = false;
  }
}

function sleep(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function polishVideoPrompt() {
  if (!videoForm.prompt.trim()) {
    message.warning('请先填写一个方向或草稿提示词');
    return;
  }
  videoPolishing.value = true;
  try {
    const { prompt } = await polishPromptApi({
      kind: 'video',
      prompt: videoForm.prompt,
    });
    videoForm.prompt = prompt;
    message.success('提示词已润色');
  } catch (error: any) {
    const errorMessage =
      error?.response?.data?.error ||
      error?.response?.data?.message ||
      error?.message ||
      '润色失败，请稍后重试';
    message.error(errorMessage);
  } finally {
    videoPolishing.value = false;
  }
}

async function generateVideo() {
  if (!videoForm.prompt.trim()) {
    message.warning('请填写视频描述（提示词）');
    return;
  }
  if (!videoForm.name.trim()) {
    message.warning('请填写资产名称');
    return;
  }
  videoGenerating.value = true;
  videoProgress.value = '正在提交生成任务…';
  try {
    const created = await generateVideoApi({
      aspectRatio: videoForm.aspectRatio,
      model: videoForm.model,
      prompt: videoForm.prompt,
      seconds: videoForm.seconds,
    });

    let current = created;
    // 轮询直到完成或失败（最多约 5 分钟）
    for (let i = 0; i < 60; i += 1) {
      if (isVideoGenerationSucceeded(current.status) && current.videoUrl) {
        break;
      }
      if (current.status === 'failed') {
        throw new Error(current.errorMessage || '视频生成失败');
      }
      videoProgress.value = `生成中…（已等待约 ${i * 5} 秒）`;
      await sleep(5000);
      current = await refreshVideoGenerationApi(current.id);
    }

    if (!isVideoGenerationSucceeded(current.status) || !current.videoUrl) {
      throw new Error('视频生成超时，请稍后在视频生成页查看结果');
    }

    videoProgress.value = '生成完成，正在入库…';
    await createAssetApi({
      name: videoForm.name,
      remark: videoForm.remark || undefined,
      type: 'video',
      url: current.videoUrl,
    });
    message.success('文生视频资产已生成并入库');
    videoForm.prompt = '';
    videoForm.name = '';
    videoForm.remark = '';
    await load();
  } catch (error: any) {
    const errorMessage =
      error?.response?.data?.error ||
      error?.response?.data?.message ||
      error?.message ||
      '文生视频失败，请稍后重试';
    message.error(errorMessage);
  } finally {
    videoGenerating.value = false;
    videoProgress.value = '';
  }
}

function removeAsset(record: VideoAsset) {
  Modal.confirm({
    content: `确认删除「${record.name}」吗？`,
    onOk: async () => {
      await deleteAssetApi(record.id);
      message.success('已删除');
      await load();
    },
    title: '删除资产',
  });
}

function resetForm() {
  form.assetId = '';
  form.coverUrl = '';
  form.name = '';
  form.remark = '';
  form.url = '';
  uploadedName.value = '';
  uploadedUrl.value = '';
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

function typeColor(type: VideoAssetType) {
  return TYPE_META[type]?.color ?? 'default';
}

function typeLabel(type: VideoAssetType) {
  return TYPE_META[type]?.label ?? type;
}

function previewKind(record: VideoAsset) {
  return getAssetPreviewKind(record.type, getAssetPreviewSource(record));
}

function previewSource(record: VideoAsset) {
  return withPreviewToken(
    getAssetPreviewSource(record),
    accessStore.accessToken,
  );
}

function openVideoPreview(record: VideoAsset) {
  const source = previewSource(record);
  if (!source) return;
  videoPreview.name = record.name || '视频预览';
  videoPreview.url = source;
  videoPreview.open = true;
}

async function loadModelConfig() {
  try {
    const config = await getModelConfigApi();
    const imageModel = config.image?.model?.trim();
    const videoModel = config.video?.model?.trim();
    if (imageModel) {
      imageForm.model = imageModel;
    }
    if (videoModel) {
      videoForm.model = videoModel;
    }
  } catch {
    // 模型配置读取失败时保留页面默认值。
  }
}

onMounted(() => {
  loadModelConfig();
  load();
});
</script>

<template>
  <Page
    description="集中管理场景、人物、音频、视频等资产，供视频生成时按类型选择并 @ 引用。"
    title="资产库"
  >
    <Row :gutter="[16, 16]">
      <Col :lg="8" :xs="24">
        <Card :bordered="false" class="asset-card">
          <div class="card-title">新增资产</div>
          <Tabs v-model:active-key="activeTab">
            <TabPane key="image" tab="文生图">
              <Form layout="vertical">
                <Form.Item label="资产类型" required>
                  <Select
                    v-model:value="imageForm.type"
                    :options="imageTypeOptions"
                  />
                </Form.Item>
                <Form.Item label="资产名称" required>
                  <Input
                    v-model:value="imageForm.name"
                    placeholder="例如：海边日落场景"
                  />
                </Form.Item>
                <Form.Item label="图片描述（提示词）" required>
                  <Input.TextArea
                    v-model:value="imageForm.prompt"
                    :rows="4"
                    placeholder="描述想要生成的画面，越具体越好"
                  />
                </Form.Item>
                <Form.Item label="尺寸">
                  <Select
                    v-model:value="imageForm.size"
                    :options="imageSizeOptions"
                  />
                </Form.Item>
                <Form.Item label="模型">
                  <Input
                    v-model:value="imageForm.model"
                    placeholder="gpt-image-2"
                  />
                </Form.Item>
                <Form.Item label="备注">
                  <Input.TextArea
                    v-model:value="imageForm.remark"
                    :rows="2"
                    placeholder="记录来源、适用场景等"
                  />
                </Form.Item>
                <div class="flex gap-2">
                  <Button :loading="imagePolishing" @click="polishImagePrompt">
                    <IconifyIcon class="mr-1" icon="lucide:wand-sparkles" />
                    一键润色
                  </Button>
                  <Button
                    :loading="imageGenerating"
                    type="primary"
                    @click="generateImage"
                  >
                    <IconifyIcon class="mr-1" icon="lucide:sparkles" />
                    生成图片并入库
                  </Button>
                </div>
              </Form>
            </TabPane>

            <TabPane key="video" tab="文生视频">
              <Form layout="vertical">
                <Form.Item label="资产名称" required>
                  <Input
                    v-model:value="videoForm.name"
                    placeholder="例如：开场动画"
                  />
                </Form.Item>
                <Form.Item label="视频描述（提示词）" required>
                  <Input.TextArea
                    v-model:value="videoForm.prompt"
                    :rows="4"
                    placeholder="描述想要生成的视频内容"
                  />
                </Form.Item>
                <Form.Item label="模型">
                  <Select
                    v-model:value="videoForm.model"
                    :options="videoModelOptions"
                  />
                </Form.Item>
                <Form.Item label="时长">
                  <Select
                    v-model:value="videoForm.seconds"
                    :options="videoSecondsOptions"
                  />
                </Form.Item>
                <Form.Item label="画面比例">
                  <Select
                    v-model:value="videoForm.aspectRatio"
                    :options="videoAspectRatioOptions"
                  />
                </Form.Item>
                <Form.Item label="备注">
                  <Input.TextArea
                    v-model:value="videoForm.remark"
                    :rows="2"
                    placeholder="记录来源、适用场景等"
                  />
                </Form.Item>
                <div v-if="videoProgress" class="video-progress">
                  {{ videoProgress }}
                </div>
                <div class="flex gap-2">
                  <Button :loading="videoPolishing" @click="polishVideoPrompt">
                    <IconifyIcon class="mr-1" icon="lucide:wand-sparkles" />
                    一键润色
                  </Button>
                  <Button
                    :loading="videoGenerating"
                    type="primary"
                    @click="generateVideo"
                  >
                    <IconifyIcon class="mr-1" icon="lucide:clapperboard" />
                    生成视频并入库
                  </Button>
                </div>
              </Form>
            </TabPane>

            <TabPane key="upload" tab="自行上传">
              <Form layout="vertical">
                <Form.Item label="资产类型" required>
                  <Select v-model:value="form.type" :options="typeOptions" />
                </Form.Item>
                <Form.Item label="资产名称" required>
                  <Input
                    v-model:value="form.name"
                    placeholder="例如：海边日落场景 / 主持人小艾"
                  />
                </Form.Item>
                <Form.Item label="资产文件" required>
                  <Upload
                    :accept="currentAccept"
                    :before-upload="() => false"
                    :max-count="1"
                    @change="uploadAsset"
                  >
                    <Button :loading="saving">
                      <IconifyIcon class="mr-1" icon="lucide:upload" />
                      上传文件
                    </Button>
                  </Upload>
                  <div v-if="uploadedUrl" class="asset-preview">
                    <div class="asset-name">{{ uploadedName }}</div>
                    <Image
                      v-if="isImageAssetType(form.type)"
                      :src="uploadedPreviewUrl"
                      class="preview-img"
                    />
                    <audio
                      v-else-if="form.type === 'audio'"
                      :src="uploadedPreviewUrl"
                      controls
                    ></audio>
                    <video
                      v-else
                      :src="uploadedPreviewUrl"
                      class="preview-video"
                      controls
                    ></video>
                  </div>
                </Form.Item>
                <Form.Item label="备注">
                  <Input.TextArea
                    v-model:value="form.remark"
                    :rows="3"
                    placeholder="记录来源、适用场景等"
                  />
                </Form.Item>
                <Space>
                  <Button :loading="saving" type="primary" @click="submit">
                    保存资产
                  </Button>
                  <Button @click="resetForm">重置</Button>
                </Space>
              </Form>
            </TabPane>
          </Tabs>
        </Card>
      </Col>

      <Col :lg="16" :xs="24">
        <Card :bordered="false" class="asset-card">
          <div class="table-head">
            <div>
              <div class="card-title">资产列表</div>
              <div class="card-desc">
                共 {{ total }} 个资产，可在视频生成中 @ 引用。
              </div>
            </div>
            <Space wrap>
              <Select
                v-model:value="query.type"
                :options="filterTypeOptions"
                class="type-select"
              />
              <Input
                v-model:value="query.keyword"
                allow-clear
                class="keyword-input"
                placeholder="搜索资产名称"
                @press-enter="search"
              />
              <Button type="primary" @click="search">查询</Button>
              <Button :loading="loading" @click="load">刷新</Button>
            </Space>
          </div>

          <Table
            :columns="columns"
            :data-source="assets"
            :loading="loading"
            :pagination="{
              current: query.page,
              pageSize: query.pageSize,
              showSizeChanger: true,
              total,
            }"
            :scroll="{ x: 1080 }"
            row-key="id"
            @change="handleTableChange"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'type'">
                <Tag :color="typeColor(record.type)">
                  {{ typeLabel(record.type) }}
                </Tag>
              </template>
              <template v-else-if="column.dataIndex === 'url'">
                <Image
                  v-if="previewKind(assetRecord(record)) === 'image'"
                  :src="previewSource(assetRecord(record))"
                  class="row-img"
                />
                <audio
                  v-else-if="previewKind(assetRecord(record)) === 'audio'"
                  :src="previewSource(assetRecord(record))"
                  class="row-audio"
                  controls
                ></audio>
                <button
                  v-else-if="previewKind(assetRecord(record)) === 'video'"
                  class="row-video-thumb"
                  type="button"
                  @click="openVideoPreview(assetRecord(record))"
                >
                  <video
                    :src="previewSource(assetRecord(record))"
                    muted
                    preload="metadata"
                  ></video>
                  <span class="row-video-play">
                    <IconifyIcon icon="lucide:play" />
                  </span>
                </button>
                <span v-else>-</span>
              </template>
              <template v-else-if="column.key === 'action'">
                <Button
                  danger
                  size="small"
                  type="link"
                  @click="removeAsset(assetRecord(record))"
                >
                  删除
                </Button>
              </template>
            </template>
          </Table>
        </Card>
      </Col>
    </Row>
    <Modal
      v-model:open="videoPreview.open"
      :footer="null"
      :title="videoPreview.name"
      :width="760"
      destroy-on-close
    >
      <video
        v-if="videoPreview.url"
        :src="videoPreview.url"
        class="large-video-preview"
        controls
      ></video>
    </Modal>
  </Page>
</template>

<style scoped>
.asset-card {
  border-radius: 8px;
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

.table-head {
  display: flex;
  gap: 16px;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 16px;
}

.keyword-input {
  width: 200px;
}

.type-select {
  width: 120px;
}

.asset-preview {
  padding: 12px;
  margin-top: 12px;
  background: #f8fafc;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
}

.asset-name {
  margin-bottom: 8px;
  font-size: 13px;
  color: #344054;
}

.video-progress {
  padding: 8px 12px;
  margin-bottom: 12px;
  font-size: 13px;
  color: #1d4ed8;
  background: #eff6ff;
  border: 1px solid #bfdbfe;
  border-radius: 6px;
}

.preview-img :deep(img),
.preview-img {
  max-width: 100%;
  max-height: 180px;
  border-radius: 6px;
}

.preview-video {
  width: 100%;
  max-height: 200px;
}

.asset-preview audio {
  width: 100%;
  height: 36px;
}

.row-img :deep(img),
.row-img {
  width: 120px;
  height: 80px;
  object-fit: cover;
  border-radius: 4px;
}

.row-audio {
  width: 200px;
  height: 36px;
}

.row-video-thumb {
  position: relative;
  width: 144px;
  height: 82px;
  padding: 0;
  overflow: hidden;
  cursor: pointer;
  background: #0f172a;
  border: 0;
  border-radius: 6px;
}

.row-video-thumb video {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.row-video-play {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
  color: #fff;
  background: rgb(15 23 42 / 32%);
}

.large-video-preview {
  width: 100%;
  max-height: 70vh;
  background: #000;
  border-radius: 6px;
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
  .type-select {
    width: 100%;
  }
}
</style>
