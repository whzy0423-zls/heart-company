<script setup lang="ts">
import type { UploadChangeParam } from 'ant-design-vue';
import type { VoiceProfile } from '#/api';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { IconifyIcon } from '@vben/icons';

import {
  Button,
  Card,
  Col,
  Form,
  Input,
  Modal,
  Row,
  Select,
  Space,
  Table,
  Tag,
  Upload,
  message,
} from 'ant-design-vue';

import {
  cloneVoiceProfileApi,
  createVoiceProfileApi,
  deleteVoiceProfileApi,
  getVoiceProfilesApi,
  uploadFileApi,
} from '#/api';

const loading = ref(false);
const saving = ref(false);
const profiles = ref<VoiceProfile[]>([]);
const total = ref(0);
const uploadedAudioUrl = ref('');
const uploadedAudioName = ref('');

const query = reactive({
  keyword: '',
  page: 1,
  pageSize: 20,
  status: '',
});

const form = reactive({
  name: '',
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
];

const columns = [
  { dataIndex: 'name', title: '人声名称', width: 180 },
  { dataIndex: 'voiceId', title: 'Voice ID', width: 240 },
  { dataIndex: 'status', title: '状态', width: 110 },
  { dataIndex: 'sampleUrl', title: '样本预览', width: 260 },
  { dataIndex: 'remark', ellipsis: true, title: '备注' },
  { dataIndex: 'createTime', title: '创建时间', width: 180 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 190 },
];

const canSubmit = computed(() => form.name.trim() && form.sampleAssetId);

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
  const rawFile = getRawFile(file);
  if (!rawFile) {
    message.warning('没有读取到音频文件，请重新选择');
    return;
  }
  if (!isAudioFile(rawFile)) {
    message.warning('请上传 mp3、wav、m4a 等音频文件');
    return;
  }
  saving.value = true;
  try {
    const result = await uploadFileApi(rawFile, 'voice/samples');
    form.sampleAssetId = String(result.assetId || '');
    form.sampleName = result.name || rawFile.name;
    form.sampleUrl = result.url;
    uploadedAudioName.value = result.name || rawFile.name;
    uploadedAudioUrl.value = result.url;
    message.success('音频样本已上传');
  } catch (error) {
    form.sampleAssetId = '';
    form.sampleName = '';
    form.sampleUrl = '';
    uploadedAudioName.value = '';
    uploadedAudioUrl.value = '';
    message.error('音频上传失败，请重新上传');
  } finally {
    saving.value = false;
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

async function submit() {
  if (!canSubmit.value) {
    message.warning('请填写人声名称并上传音频样本');
    return;
  }
  saving.value = true;
  try {
    await createVoiceProfileApi({
      name: form.name,
      remark: form.remark,
      sampleAssetId: form.sampleAssetId,
      sampleName: form.sampleName,
      sampleUrl: form.sampleUrl,
      voiceId: form.voiceId,
    });
    message.success('人声已提交克隆');
    resetForm();
    await load();
  } finally {
    saving.value = false;
  }
}

async function retryClone(record: VoiceProfile) {
  saving.value = true;
  try {
    await cloneVoiceProfileApi(record.id);
    message.success('已重新提交克隆');
    await load();
  } finally {
    saving.value = false;
  }
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

function handleTableChange(pagination: { current?: number; pageSize?: number }) {
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
  return status || '-';
}

onMounted(load);
</script>

<template>
  <Page
    description="上传授权音频样本，调用 MiniMax 中文版克隆音色，并保存可复用的人声档案。"
    title="人声管理"
  >
    <Row :gutter="[16, 16]">
      <Col :lg="8" :xs="24">
        <Card :bordered="false" class="voice-card">
          <div class="card-title">新增人声</div>
          <Form layout="vertical">
            <Form.Item label="人声名称" required>
              <Input v-model:value="form.name" placeholder="例如：课程老师女声" />
            </Form.Item>
            <Form.Item label="Voice ID">
              <Input
                v-model:value="form.voiceId"
                placeholder="可选，留空自动生成"
              />
            </Form.Item>
            <Form.Item label="音频样本" required>
              <Upload
                :before-upload="() => false"
                :max-count="1"
                accept="audio/*"
                @change="uploadAudio"
              >
                <Button :loading="saving">
                  <IconifyIcon class="mr-1" icon="lucide:upload" />
                  上传音频
                </Button>
              </Upload>
              <div v-if="uploadedAudioUrl" class="audio-preview">
                <div class="audio-name">{{ uploadedAudioName }}</div>
                <audio :src="uploadedAudioUrl" controls />
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
                :loading="saving"
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
              <div class="card-desc">共 {{ total }} 个音色，状态为可使用后可去声音测试。</div>
            </div>
            <Space wrap>
              <Select
                v-model:value="query.status"
                :options="statusOptions"
                class="status-select"
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
                  :src="record.sampleUrl"
                  class="row-audio"
                  controls
                />
                <span v-else>-</span>
              </template>
              <template v-else-if="column.key === 'action'">
                <Space>
                  <Button
                    :loading="saving"
                    size="small"
                    type="link"
                    @click="retryClone(record)"
                  >
                    重新克隆
                  </Button>
                  <Button danger size="small" type="link" @click="removeProfile(record)">
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
.voice-card {
  border-radius: 8px;
}

.card-title {
  font-size: 16px;
  font-weight: 600;
}

.card-desc {
  margin-top: 4px;
  color: #667085;
  font-size: 13px;
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
  color: #344054;
  font-size: 13px;
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
  color: #cf1322;
  font-size: 12px;
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
