<script setup lang="ts">
import type { VoiceGeneration, VoiceProfile } from '#/api';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { IconifyIcon } from '@vben/icons';

import {
  Button,
  Card,
  Col,
  Form,
  Input,
  Row,
  Select,
  Space,
  Table,
  Tag,
  message,
} from 'ant-design-vue';

import {
  generateVoiceApi,
  getVoiceGenerationsApi,
  getVoiceProfilesApi,
} from '#/api';

const loading = ref(false);
const generating = ref(false);
const profiles = ref<VoiceProfile[]>([]);
const generations = ref<VoiceGeneration[]>([]);
const total = ref(0);
const latest = ref<VoiceGeneration>();

const query = reactive({
  page: 1,
  pageSize: 10,
});

const form = reactive({
  model: 'speech-02-hd',
  profileId: '',
  text: '你好，欢迎来到九型人格成长课程。这里是一段人声克隆测试音频。',
});

const modelOptions = [
  { label: 'speech-02-hd（高清）', value: 'speech-02-hd' },
  { label: 'speech-02-turbo（快速）', value: 'speech-02-turbo' },
  { label: 'speech-01-hd', value: 'speech-01-hd' },
  { label: 'speech-01-turbo', value: 'speech-01-turbo' },
];

const profileOptions = computed(() =>
  profiles.value
    .filter((item) => item.status === 'ready')
    .map((item) => ({
      label: `${item.name}（${item.voiceId}）`,
      value: item.id,
    })),
);

const currentProfile = computed(() =>
  profiles.value.find((item) => item.id === form.profileId),
);

const columns = [
  { dataIndex: 'text', ellipsis: true, title: '测试文本' },
  { dataIndex: 'voiceId', title: 'Voice ID', width: 220 },
  { dataIndex: 'audioUrl', title: '音频', width: 280 },
  { dataIndex: 'status', title: '状态', width: 100 },
  { dataIndex: 'createTime', title: '生成时间', width: 180 },
];

async function loadProfiles() {
  const result = await getVoiceProfilesApi({ page: 1, pageSize: 100, status: 'ready' });
  profiles.value = result.items;
  if (!form.profileId && result.items.length > 0) {
    form.profileId = result.items[0]?.id ?? '';
  }
}

async function loadGenerations() {
  loading.value = true;
  try {
    const result = await getVoiceGenerationsApi({
      page: query.page,
      pageSize: query.pageSize,
    });
    generations.value = result.items;
    total.value = result.total;
  } finally {
    loading.value = false;
  }
}

async function generate() {
  if (!form.profileId) {
    message.warning('请选择一个可使用的人声');
    return;
  }
  if (!form.text.trim()) {
    message.warning('请输入测试文本');
    return;
  }
  generating.value = true;
  try {
    const result = await generateVoiceApi({
      model: form.model,
      profileId: form.profileId,
      text: form.text,
      voiceId: currentProfile.value?.voiceId,
    });
    latest.value = result;
    message.success('音频已生成');
    await loadGenerations();
  } finally {
    generating.value = false;
  }
}

function handleTableChange(pagination: { current?: number; pageSize?: number }) {
  query.page = pagination.current ?? 1;
  query.pageSize = pagination.pageSize ?? 10;
  loadGenerations();
}

onMounted(async () => {
  await loadProfiles();
  await loadGenerations();
});
</script>

<template>
  <Page
    description="选择已克隆的人声，输入测试文本，生成音频并试听效果。"
    title="声音测试"
  >
    <Row :gutter="[16, 16]">
      <Col :lg="9" :xs="24">
        <Card :bordered="false" class="voice-card">
          <div class="card-title">生成测试音频</div>
          <Form layout="vertical">
            <Form.Item label="选择音色" required>
              <Select
                v-model:value="form.profileId"
                :options="profileOptions"
                placeholder="请选择可使用的人声"
              />
              <div v-if="currentProfile" class="voice-meta">
                <Tag color="success">可使用</Tag>
                <span>{{ currentProfile.voiceId }}</span>
              </div>
            </Form.Item>
            <Form.Item label="模型">
              <Select v-model:value="form.model" :options="modelOptions" />
            </Form.Item>
            <Form.Item label="测试文本" required>
              <Input.TextArea
                v-model:value="form.text"
                :maxlength="1000"
                :rows="7"
                show-count
                placeholder="输入要合成的中文文本"
              />
            </Form.Item>
            <Button :loading="generating" type="primary" @click="generate">
              <IconifyIcon class="mr-1" icon="lucide:wand-sparkles" />
              生成音频
            </Button>
          </Form>
        </Card>
      </Col>

      <Col :lg="15" :xs="24">
        <Card :bordered="false" class="voice-card result-card">
          <div class="card-title">测试结果</div>
          <div v-if="latest?.audioUrl" class="latest-result">
            <div class="latest-title">最新生成</div>
            <div class="latest-text">{{ latest.text }}</div>
            <audio :src="latest.audioUrl" controls />
          </div>
          <div v-else class="empty-result">
            生成后会在这里播放最新音频。
          </div>
        </Card>

        <Card :bordered="false" class="voice-card history-card">
          <div class="history-head">
            <div>
              <div class="card-title">生成记录</div>
              <div class="card-desc">共 {{ total }} 条测试记录，音频已保存可回放。</div>
            </div>
            <Button :loading="loading" @click="loadGenerations">刷新</Button>
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
            :scroll="{ x: 960 }"
            row-key="id"
            @change="handleTableChange"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'audioUrl'">
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
.voice-card {
  border-radius: 8px;
}

.card-title {
  margin-bottom: 16px;
  font-size: 16px;
  font-weight: 600;
}

.card-desc {
  margin-top: 4px;
  color: #667085;
  font-size: 13px;
}

.voice-meta {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-top: 8px;
  color: #667085;
  font-size: 12px;
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

.latest-title {
  margin-bottom: 8px;
  color: #344054;
  font-weight: 600;
}

.latest-text {
  margin-bottom: 12px;
  color: #475467;
  line-height: 1.7;
}

.empty-result {
  display: flex;
  min-height: 128px;
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
