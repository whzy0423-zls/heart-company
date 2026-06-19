<script setup lang="ts">
import type { VoiceContentJob, VoiceOption } from '#/api';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { IconifyIcon } from '@vben/icons';

import {
  Button,
  Card,
  Col,
  Form,
  Input,
  Radio,
  Row,
  Select,
  Space,
  Table,
  Tag,
  Upload,
  message,
} from 'ant-design-vue';

import {
  generateVoiceContentApi,
  getVoiceContentJobsApi,
  getVoiceOptionsApi,
  uploadFileApi,
} from '#/api';

const loading = ref(false);
const generating = ref(false);
const uploading = ref(false);
const voiceOptions = ref<VoiceOption[]>([]);
const jobs = ref<VoiceContentJob[]>([]);
const latest = ref<VoiceContentJob>();
const total = ref(0);

const query = reactive({
  page: 1,
  pageSize: 10,
});

const form = reactive({
  model: 'speech-02-hd',
  sourceAssetId: '',
  sourceName: '',
  sourceType: 'courseware',
  sourceUrl: '',
  text: '',
  title: '',
  voiceKey: '',
});

const modelOptions = [
  { label: 'speech-02-hd（高清）', value: 'speech-02-hd' },
  { label: 'speech-02-turbo（快速）', value: 'speech-02-turbo' },
  { label: 'speech-01-hd', value: 'speech-01-hd' },
  { label: 'speech-01-turbo', value: 'speech-01-turbo' },
];

const sourceTypeOptions = [
  { label: '课件', value: 'courseware' },
  { label: '读书内容', value: 'book' },
  { label: '手动输入', value: 'manual' },
];

const groupedVoiceOptions = computed(() => [
  {
    label: 'MiniMax 官方音色',
    options: voiceOptions.value
      .filter((item) => item.source === 'official')
      .map((item) => ({ label: item.label, value: item.id })),
  },
  {
    label: '我的克隆音色',
    options: voiceOptions.value
      .filter((item) => item.source === 'clone')
      .map((item) => ({ label: item.label, value: item.id })),
  },
]);

const selectedVoice = computed(() =>
  voiceOptions.value.find((item) => item.id === form.voiceKey),
);

const columns = [
  { dataIndex: 'title', ellipsis: true, title: '标题', width: 180 },
  { dataIndex: 'sourceType', title: '类型', width: 110 },
  { dataIndex: 'voiceName', title: '音色', width: 180 },
  { dataIndex: 'audioUrl', title: '音频', width: 280 },
  { dataIndex: 'status', title: '状态', width: 90 },
  { dataIndex: 'createTime', title: '生成时间', width: 180 },
];

async function loadVoices() {
  voiceOptions.value = await getVoiceOptionsApi();
  if (!form.voiceKey && voiceOptions.value.length > 0) {
    form.voiceKey = voiceOptions.value[0]?.id ?? '';
  }
}

async function loadJobs() {
  loading.value = true;
  try {
    const result = await getVoiceContentJobsApi({
      page: query.page,
      pageSize: query.pageSize,
    });
    jobs.value = result.items;
    total.value = result.total;
  } finally {
    loading.value = false;
  }
}

async function uploadContent(options: { file: File; onSuccess?: () => void }) {
  const rawFile = options.file;
  if (!rawFile) {
    message.warning('没有读取到文件，请重新选择');
    return;
  }
  uploading.value = true;
  try {
    const result = await uploadFileApi(rawFile, 'voice/content-source');
    form.sourceAssetId = String(result.assetId || '');
    form.sourceName = result.name || rawFile.name;
    form.sourceUrl = result.url;
    if (!form.title) {
      form.title = trimExt(rawFile.name);
    }
    if (isTextFile(rawFile)) {
      form.text = await rawFile.text();
      message.success('文件已上传，文本内容已自动读取');
    } else {
      message.success('文件已上传，请在下方粘贴或整理要朗读的正文');
    }
    options.onSuccess?.();
  } catch (error: any) {
    message.error(error?.response?.data?.error || error?.message || '文件上传失败');
  } finally {
    uploading.value = false;
  }
}

async function generate() {
  const voice = selectedVoice.value;
  if (!voice) {
    message.warning('请选择音色');
    return;
  }
  if (!form.text.trim()) {
    message.warning('请输入或上传可转换的文本内容');
    return;
  }
  generating.value = true;
  try {
    const result = await generateVoiceContentApi({
      model: form.model,
      profileId: voice.source === 'clone' ? voice.id.replace('clone:', '') : '',
      sourceAssetId: form.sourceAssetId,
      sourceName: form.sourceName,
      sourceType: form.sourceType,
      sourceUrl: form.sourceUrl,
      text: form.text,
      title: form.title || '未命名内容',
      voiceId: voice.voiceId,
      voiceName: voice.voiceName,
      voiceSource: voice.source,
    });
    latest.value = result;
    message.success('音频已生成');
    await loadJobs();
  } catch (error: any) {
    message.error(
      error?.response?.data?.error ||
        error?.response?.data?.message ||
        error?.message ||
        '音频生成失败',
    );
  } finally {
    generating.value = false;
  }
}

function resetForm() {
  form.sourceAssetId = '';
  form.sourceName = '';
  form.sourceUrl = '';
  form.text = '';
  form.title = '';
}

function handleTableChange(pagination: { current?: number; pageSize?: number }) {
  query.page = pagination.current ?? 1;
  query.pageSize = pagination.pageSize ?? 10;
  loadJobs();
}

function isTextFile(file: File) {
  return (
    file.type.startsWith('text/') ||
    /\.(md|markdown|txt)$/i.test(file.name)
  );
}

function trimExt(name: string) {
  return name.replace(/\.[^.]+$/, '');
}

function sourceTypeLabel(value: string) {
  if (value === 'courseware') return '课件';
  if (value === 'book') return '读书';
  return '手动';
}

onMounted(async () => {
  await Promise.all([loadVoices(), loadJobs()]);
});
</script>

<template>
  <Page
    description="上传课件或读书内容，选择官方/克隆音色，将正文转换为可试听和留档的音频。"
    title="内容转语音"
  >
    <Row :gutter="[16, 16]">
      <Col :lg="9" :xs="24">
        <Card :bordered="false" class="content-card">
          <div class="card-title">内容生成</div>
          <Form layout="vertical">
            <Form.Item label="内容类型">
              <Radio.Group v-model:value="form.sourceType" :options="sourceTypeOptions" />
            </Form.Item>
            <Form.Item label="上传课件 / 读书内容">
              <Upload
                :before-upload="() => false"
                :custom-request="uploadContent"
                :max-count="1"
                accept=".txt,.md,.markdown,.pdf,.doc,.docx,.ppt,.pptx"
              >
                <Button :loading="uploading">
                  <IconifyIcon class="mr-1" icon="lucide:upload" />
                  上传文件
                </Button>
              </Upload>
              <div v-if="form.sourceName" class="source-file">
                {{ form.sourceName }}
              </div>
            </Form.Item>
            <Form.Item label="标题">
              <Input v-model:value="form.title" placeholder="例如：第一章导读" />
            </Form.Item>
            <Form.Item label="选择音色" required>
              <Select
                v-model:value="form.voiceKey"
                :options="groupedVoiceOptions"
                show-search
                option-filter-prop="label"
                placeholder="选择 MiniMax 官方音色或我的克隆音色"
              />
              <div v-if="voiceOptions.length === 0" class="form-hint">
                暂未读取到音色，请先检查 MiniMax 配置或添加克隆音色。
              </div>
            </Form.Item>
            <Form.Item label="模型">
              <Select v-model:value="form.model" :options="modelOptions" />
            </Form.Item>
            <Form.Item label="朗读正文" required>
              <Input.TextArea
                v-model:value="form.text"
                :maxlength="5000"
                :rows="10"
                show-count
                placeholder="粘贴课件讲稿、读书摘录或上传 txt/md 自动读取"
              />
            </Form.Item>
            <Space>
              <Button :loading="generating" type="primary" @click="generate">
                <IconifyIcon class="mr-1" icon="lucide:file-audio" />
                生成音频
              </Button>
              <Button @click="resetForm">清空内容</Button>
            </Space>
          </Form>
        </Card>
      </Col>

      <Col :lg="15" :xs="24">
        <Card :bordered="false" class="content-card result-card">
          <div class="card-title">最新结果</div>
          <div v-if="latest?.audioUrl" class="latest-result">
            <div class="latest-title">{{ latest.title }}</div>
            <div class="latest-meta">
              {{ latest.voiceName }} · {{ sourceTypeLabel(latest.sourceType) }}
            </div>
            <audio :src="latest.audioUrl" controls />
          </div>
          <div v-else class="empty-result">生成后会在这里播放最新音频。</div>
        </Card>

        <Card :bordered="false" class="content-card">
          <div class="history-head">
            <div>
              <div class="card-title">生成记录</div>
              <div class="card-desc">共 {{ total }} 条内容转语音记录。</div>
            </div>
            <Button :loading="loading" @click="loadJobs">刷新</Button>
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
            :scroll="{ x: 1020 }"
            row-key="id"
            @change="handleTableChange"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'sourceType'">
                <Tag>{{ sourceTypeLabel(record.sourceType) }}</Tag>
              </template>
              <template v-else-if="column.dataIndex === 'voiceName'">
                <div>{{ record.voiceName || record.voiceId }}</div>
                <Tag :color="record.voiceSource === 'clone' ? 'blue' : 'green'">
                  {{ record.voiceSource === 'clone' ? '克隆' : '官方' }}
                </Tag>
              </template>
              <template v-else-if="column.dataIndex === 'audioUrl'">
                <audio v-if="record.audioUrl" :src="record.audioUrl" class="row-audio" controls />
                <span v-else>-</span>
              </template>
              <template v-else-if="column.dataIndex === 'status'">
                <Tag :color="record.status === 'success' ? 'success' : 'error'">
                  {{ record.status === 'success' ? '成功' : '失败' }}
                </Tag>
              </template>
            </template>
          </Table>
        </Card>
      </Col>
    </Row>
  </Page>
</template>

<style scoped>
.content-card {
  border-radius: 8px;
}

.card-title {
  margin-bottom: 16px;
  font-size: 16px;
  font-weight: 600;
}

.card-desc,
.form-hint,
.latest-meta,
.source-file {
  color: #667085;
  font-size: 13px;
}

.form-hint {
  margin-top: 8px;
}

.source-file {
  padding: 8px 10px;
  margin-top: 10px;
  background: #f8fafc;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
}

.result-card {
  margin-bottom: 16px;
}

.latest-result {
  padding: 16px;
  background: #f8fafc;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
}

.latest-title {
  margin-bottom: 6px;
  font-weight: 600;
}

.latest-meta {
  margin-bottom: 12px;
}

.empty-result {
  display: flex;
  min-height: 120px;
  align-items: center;
  justify-content: center;
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

.latest-result audio,
.row-audio {
  width: 100%;
  min-width: 220px;
  height: 36px;
}
</style>
