<script setup lang="ts">
import type { UploadChangeParam } from 'ant-design-vue';

import type { VoiceProfile } from '#/api';

import type { BailianCredentialsCardStatus } from './bailian-credentials-card.vue';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { IconifyIcon } from '@vben/icons';
import { useAccessStore } from '@vben/stores';

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
  Table,
  Tag,
  Upload,
} from 'ant-design-vue';

import {
  cloneVoiceProfileApi,
  copyVoiceProfileToBailianApi,
  createVoiceProfileApi,
  deleteVoiceProfileApi,
  getVoiceProfilesApi,
  uploadFileApi,
} from '#/api';
import { ellipsisColumn } from '#/components/ellipsis-tooltip/table';
import {
  useUploadAssetPreviewResolver,
  useUploadAssetPreviewUrl,
} from '#/utils/upload-asset-preview';

import BailianCredentialsCard from './bailian-credentials-card.vue';
import {
  getBailianCopyFeedback,
  getBailianCloneFeedback,
  updateCopyingProfileIds,
} from './profiles-copy-feedback';

const accessStore = useAccessStore();
const audioPreview = useUploadAssetPreviewResolver(
  () => accessStore.accessToken,
);
const loading = ref(false);
const formSaving = ref(false);
const credentialStatus = ref<BailianCredentialsCardStatus>({
  apiKeySet: false,
  error: null,
  loading: true,
  saving: false,
  source: 'none',
  version: 0,
});
const busyProfileIds = ref(new Set<string>());
const profiles = ref<VoiceProfile[]>([]);
const total = ref(0);
const uploadedAudioUrl = ref('');
const uploadedAudioName = ref('');
const uploadedAudioPreviewUrl = useUploadAssetPreviewUrl(
  () => uploadedAudioUrl.value,
  () => accessStore.accessToken,
);

const query = reactive({
  keyword: '',
  page: 1,
  pageSize: 20,
  status: '',
});

const form = reactive({
  name: '',
  provider: 'bailian',
  remark: '',
  sampleAssetId: '',
  sampleName: '',
  sampleUrl: '',
  voiceId: '',
});

const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '可使用', value: 'ready' },
  { label: '克隆中', value: 'cloning' },
  { label: '失败', value: 'failed' },
  { label: '草稿', value: 'draft' },
  { label: '已迁移', value: 'migrated' },
];

const columns = [
  { dataIndex: 'name', title: '人声名称', width: 180 },
  { dataIndex: 'voiceId', title: 'Voice ID', width: 240 },
  { dataIndex: 'provider', title: '平台', width: 140 },
  { dataIndex: 'model', title: '模型', width: 220 },
  { dataIndex: 'status', title: '状态', width: 110 },
  { dataIndex: 'sampleUrl', title: '样本预览', width: 260 },
  ellipsisColumn('remark', '备注', { lines: 2 }),
  { dataIndex: 'createTime', title: '创建时间', width: 180 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 190 },
];

const canCloneVoice = computed(
  () =>
    credentialStatus.value.apiKeySet &&
    !credentialStatus.value.loading &&
    !credentialStatus.value.error &&
    !credentialStatus.value.saving,
);
const canSubmit = computed(
  () =>
    canCloneVoice.value &&
    Boolean(form.name.trim()) &&
    Boolean(form.sampleAssetId) &&
    !formSaving.value,
);
const cloneGateMessage = computed(() => {
  if (credentialStatus.value.error) {
    return '百炼凭证读取失败，可在上方重新加载';
  }
  if (credentialStatus.value.loading) {
    return '正在读取百炼公共 API Key，请稍候';
  }
  if (credentialStatus.value.saving) {
    return '百炼公共 API Key 正在保存，请稍候';
  }
  return '请先保存百炼公共 API Key';
});

function handleCredentialStatusChange(status: BailianCredentialsCardStatus) {
  credentialStatus.value = status;
}

function ensureCanCloneVoice() {
  if (canCloneVoice.value) return true;
  message.warning(cloneGateMessage.value);
  return false;
}

async function load() {
  loading.value = true;
  try {
    const result = await getVoiceProfilesApi({
      keyword: query.keyword,
      page: query.page,
      pageSize: query.pageSize,
      status: query.status || undefined,
    });
    profiles.value = result.items;
    total.value = result.total;
  } finally {
    loading.value = false;
  }
}

async function uploadAudio({ file }: UploadChangeParam) {
  if (formSaving.value) return;
  if (!ensureCanCloneVoice()) return;
  const rawFile = getRawFile(file);
  if (!rawFile) {
    message.warning('没有读取到音频文件，请重新选择');
    return;
  }
  if (!isAudioFile(rawFile)) {
    message.warning('请上传 mp3、wav、m4a 等音频文件');
    return;
  }
  formSaving.value = true;
  try {
    const result = await uploadFileApi(rawFile, 'voice/samples');
    form.sampleAssetId = String(result.assetId || '');
    form.sampleName = result.name || rawFile.name;
    form.sampleUrl = result.url;
    uploadedAudioName.value = result.name || rawFile.name;
    uploadedAudioUrl.value = result.url;
    message.success('音频样本已上传');
  } catch (error: any) {
    form.sampleAssetId = '';
    form.sampleName = '';
    form.sampleUrl = '';
    uploadedAudioName.value = '';
    uploadedAudioUrl.value = '';
    const errorMessage =
      error?.response?.data?.error ||
      error?.response?.data?.message ||
      error?.message ||
      '音频上传失败，请重新上传';
    message.error(errorMessage);
  } finally {
    formSaving.value = false;
  }
}

function getRawFile(file: UploadChangeParam['file']) {
  return (file.originFileObj || file) as File | undefined;
}

function isAudioFile(file: File) {
  if (file.type?.startsWith('audio/')) {
    return true;
  }
  return /\.(aac|flac|m4a|mp3|ogg|wav|webm)$/i.test(file.name);
}

function previewAudioUrl(source?: string) {
  return audioPreview.resolve(source);
}

async function submit() {
  if (!ensureCanCloneVoice()) return;
  if (!canSubmit.value) {
    message.warning('请填写人声名称并上传音频样本');
    return;
  }
  formSaving.value = true;
  try {
    const result = await createVoiceProfileApi({
      name: form.name,
      provider: form.provider,
      remark: form.remark,
      sampleAssetId: form.sampleAssetId,
      sampleName: form.sampleName,
      sampleUrl: form.sampleUrl,
      voiceId: form.voiceId,
    });
    showBailianCloneFeedback(result);
    resetForm();
    await load();
  } finally {
    formSaving.value = false;
  }
}

async function retryClone(record: VoiceProfile) {
  if (!ensureCanCloneVoice()) return;
  if (!beginProfileOperation(record.id)) {
    message.warning('该人声正在处理，请稍候');
    return;
  }
  try {
    const result = await cloneVoiceProfileApi(record.id);
    showBailianCloneFeedback(result);
    await load();
  } finally {
    endProfileOperation(record.id);
  }
}

function isProfileBusy(profileId: string) {
  return busyProfileIds.value.has(profileId);
}

function setProfileBusy(profileId: string, isBusy: boolean) {
  busyProfileIds.value = updateCopyingProfileIds(
    busyProfileIds.value,
    profileId,
    isBusy,
  );
}

function beginProfileOperation(profileId: string) {
  if (isProfileBusy(profileId)) return false;
  setProfileBusy(profileId, true);
  return true;
}

function endProfileOperation(profileId: string) {
  setProfileBusy(profileId, false);
}

function showBailianCopyFeedback(result: VoiceProfile) {
  const feedback = getBailianCopyFeedback(result);
  message[feedback.type](feedback.content);
}

function showBailianCloneFeedback(result: VoiceProfile) {
  const feedback = getBailianCloneFeedback(result);
  message[feedback.type](feedback.content);
}

function copyProfileToBailian(record: VoiceProfile) {
  if (!ensureCanCloneVoice()) return;
  if (isProfileBusy(record.id)) {
    message.warning('该人声正在处理，请稍候');
    return;
  }
  Modal.confirm({
    content: `将复用原音频样本创建「${record.name}」的阿里百炼 Qwen 音色，迁移成功后停用原 MiniMax 音色。确认继续吗？`,
    onOk: async () => {
      if (!ensureCanCloneVoice()) {
        throw new Error('bailian_voice_clone_unavailable');
      }
      if (!beginProfileOperation(record.id)) {
        message.warning('该人声正在处理，请稍候');
        throw new Error('voice_profile_busy');
      }
      try {
        const result = await copyVoiceProfileToBailianApi(record.id);
        showBailianCopyFeedback(result);
        await load();
      } catch {
        // requestClient's shared interceptor displays the backend error once.
      } finally {
        endProfileOperation(record.id);
      }
    },
    title: '迁移到百炼 Qwen',
  });
}

function profileRecord(record: Record<string, any>): VoiceProfile {
  return record as VoiceProfile;
}

function removeProfile(record: VoiceProfile) {
  Modal.confirm({
    content: `确认删除「${record.name}」吗？`,
    onOk: async () => {
      await deleteVoiceProfileApi(record.id);
      message.success('已删除');
      await load();
    },
    title: '删除人声',
  });
}

function resetForm() {
  form.name = '';
  form.provider = 'bailian';
  form.remark = '';
  form.sampleAssetId = '';
  form.sampleName = '';
  form.sampleUrl = '';
  form.voiceId = '';
  uploadedAudioName.value = '';
  uploadedAudioUrl.value = '';
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

function statusColor(status: string) {
  if (status === 'ready') return 'success';
  if (status === 'failed') return 'error';
  if (status === 'cloning') return 'processing';
  return 'default';
}

function statusLabel(status: string) {
  if (status === 'ready') return '可使用';
  if (status === 'failed') return '失败';
  if (status === 'cloning') return '克隆中';
  if (status === 'draft') return '草稿';
  if (status === 'migrated') return '已迁移';
  return status || '-';
}

function platformLabel(provider: string) {
  if (provider === 'bailian') return '阿里百炼';
  if (provider === 'minimax') return 'MiniMax';
  return provider || '-';
}

function platformColor(provider: string) {
  if (provider === 'bailian') return 'blue';
  if (provider === 'minimax') return 'green';
  return 'default';
}

onMounted(load);
</script>

<template>
  <Page
    description="新音色统一使用阿里百炼 Qwen 声音复刻；历史 MiniMax 音色保留运行兼容并可迁移。"
    title="人声管理"
  >
    <Row :gutter="[16, 16]">
      <Col :lg="8" :xs="24">
        <BailianCredentialsCard
          class="credential-card"
          @status-change="handleCredentialStatusChange"
        />
        <Card :bordered="false" class="voice-card">
          <div class="card-title">新增人声</div>
          <Form layout="vertical">
            <Form.Item label="人声名称" required>
              <Input
                v-model:value="form.name"
                placeholder="例如：课程老师女声"
              />
            </Form.Item>
            <Alert
              class="mb-4"
              type="info"
              show-icon
              message="阿里百炼 Qwen 声音复刻"
              description="固定使用 qwen3-tts-vc-2026-01-22；保存公共 Key → 上传样本 → 克隆 → 芯之力选择"
            />
            <Alert
              v-if="!canCloneVoice"
              class="mb-4"
              :message="cloneGateMessage"
              show-icon
              type="warning"
            />
            <Form.Item label="Voice ID">
              <Input
                v-model:value="form.voiceId"
                placeholder="可选，留空自动生成"
              />
            </Form.Item>
            <Form.Item label="音频样本" required>
              <Upload
                :before-upload="() => false"
                :disabled="!canCloneVoice || formSaving"
                :max-count="1"
                accept="audio/*"
                @change="uploadAudio"
              >
                <Button
                  :disabled="!canCloneVoice || formSaving"
                  :loading="formSaving"
                >
                  <IconifyIcon class="mr-1" icon="lucide:upload" />
                  上传音频
                </Button>
              </Upload>
              <div v-if="uploadedAudioUrl" class="audio-preview">
                <div class="audio-name">{{ uploadedAudioName }}</div>
                <audio :src="uploadedAudioPreviewUrl" controls></audio>
              </div>
            </Form.Item>
            <Form.Item label="备注">
              <Input.TextArea
                v-model:value="form.remark"
                :rows="3"
                placeholder="记录授权来源、适用场景等"
              />
            </Form.Item>
            <Space>
              <Button
                :disabled="!canSubmit"
                :loading="formSaving"
                type="primary"
                @click="submit"
              >
                保存并克隆
              </Button>
              <Button @click="resetForm">重置</Button>
            </Space>
          </Form>
        </Card>
      </Col>

      <Col :lg="16" :xs="24">
        <Card :bordered="false" class="voice-card">
          <div class="table-head">
            <div>
              <div class="card-title">人声列表</div>
              <div class="card-desc">
                共 {{ total }} 个音色，状态为可使用后可去声音测试。
              </div>
            </div>
            <Space wrap>
              <Select
                v-model:value="query.status"
                :options="statusOptions"
                class="status-select"
                placeholder="请选择备注"
              />
              <Input
                v-model:value="query.keyword"
                allow-clear
                class="keyword-input"
                placeholder="搜索名称 / Voice ID"
                @press-enter="search"
              />
              <Button type="primary" @click="search">查询</Button>
              <Button :loading="loading" @click="load">刷新</Button>
            </Space>
          </div>

          <Table
            :columns="columns"
            :data-source="profiles"
            :loading="loading"
            :pagination="{
              current: query.page,
              pageSize: query.pageSize,
              showSizeChanger: true,
              total,
            }"
            :scroll="{ x: 1180 }"
            row-key="id"
            @change="handleTableChange"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'status'">
                <Tag :color="statusColor(record.status)">
                  {{ statusLabel(record.status) }}
                </Tag>
                <div v-if="record.lastError" class="error-text">
                  {{ record.lastError }}
                </div>
              </template>
              <template v-else-if="column.dataIndex === 'sampleUrl'">
                <audio
                  v-if="record.sampleUrl"
                  :src="previewAudioUrl(record.sampleUrl)"
                  class="row-audio"
                  controls
                ></audio>
                <span v-else>-</span>
              </template>
              <template v-else-if="column.dataIndex === 'provider'">
                <Tag :color="platformColor(record.provider)">
                  {{ platformLabel(record.provider) }}
                </Tag>
              </template>
              <template v-else-if="column.key === 'action'">
                <Space>
                  <Button
                    v-if="['draft', 'failed'].includes(record.status)"
                    :disabled="!canCloneVoice || isProfileBusy(record.id)"
                    :loading="isProfileBusy(record.id)"
                    size="small"
                    type="link"
                    @click="retryClone(profileRecord(record))"
                  >
                    重新克隆
                  </Button>
                  <Button
                    v-if="
                      record.provider === 'minimax' &&
                      record.sampleAssetId &&
                      record.status !== 'migrated'
                    "
                    :disabled="!canCloneVoice || isProfileBusy(record.id)"
                    :loading="isProfileBusy(record.id)"
                    size="small"
                    type="link"
                    @click="copyProfileToBailian(profileRecord(record))"
                  >
                    迁移到百炼 Qwen
                  </Button>
                  <Button
                    danger
                    size="small"
                    type="link"
                    @click="removeProfile(profileRecord(record))"
                  >
                    删除
                  </Button>
                </Space>
              </template>
            </template>
          </Table>
        </Card>
      </Col>
    </Row>
  </Page>
</template>

<style scoped>
.credential-card {
  display: block;
  margin-bottom: 16px;
}

.voice-card {
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
  width: 220px;
}

.status-select {
  width: 120px;
}

.audio-preview {
  padding: 12px;
  margin-top: 12px;
  background: #f8fafc;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
}

.audio-name {
  margin-bottom: 8px;
  font-size: 13px;
  color: #344054;
}

.audio-preview audio,
.row-audio {
  width: 100%;
  min-width: 220px;
  height: 36px;
}

.error-text {
  max-width: 260px;
  margin-top: 6px;
  font-size: 12px;
  color: #cf1322;
  white-space: normal;
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
