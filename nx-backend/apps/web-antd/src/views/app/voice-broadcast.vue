<script setup lang="ts">
import type {
  VoiceBroadcastConfigPayload,
  VoiceBroadcastConfigView,
} from '#/api';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Alert,
  Button,
  Card,
  Form,
  Input,
  message,
  Select,
  Space,
  Switch,
  Tag,
} from 'ant-design-vue';

import {
  getVoiceBroadcastConfigApi,
  testVoiceBroadcastConfigApi,
  updateVoiceBroadcastConfigApi,
} from '#/api';

const loading = ref(true);
const saving = ref(false);
const testing = ref(false);
const loadError = ref('');
const apiKey = ref('');
const config = ref<VoiceBroadcastConfigView>(emptyView());
const form = reactive<VoiceBroadcastConfigPayload>(emptyPayload());

const voiceOptions = computed(() =>
  config.value.voices.map((voice) => ({
    label: `${voice.name}（${voice.id}）`,
    value: voice.id,
  })),
);
const keyStatus = computed(() =>
  config.value.apiKeySet
    ? `已配置${config.value.apiKeySuffix ? `（${config.value.apiKeySuffix}）` : ''}`
    : '未配置',
);
const healthType = computed(() => {
  if (config.value.health.status === 'ok') return 'success';
  if (config.value.health.status === 'error') return 'error';
  return 'info';
});
const healthMessage = computed(() => {
  const health = config.value.health;
  if (health.status === 'ok') {
    return `最近测试通过${health.latencyMs ? `，耗时 ${health.latencyMs} ms` : ''}`;
  }
  return health.message || '尚未测试';
});

onMounted(load);

function emptyView(): VoiceBroadcastConfigView {
  return {
    apiKeySet: false,
    apiKeySuffix: '',
    currentVoice: 'Cherry',
    defaultVoice: 'Cherry',
    enabled: false,
    health: { latencyMs: 0, message: '尚未测试', ok: false, status: 'unknown' },
    model: 'qwen3-tts-instruct-flash',
    provider: 'aliyun-bailian',
    region: 'cn-beijing',
    version: 0,
    voices: [
      {
        id: 'Cherry',
        model: 'qwen3-tts-instruct-flash',
        name: 'Cherry（默认女声）',
        provider: 'aliyun-bailian',
      },
    ],
    workspaceId: '',
  };
}

function emptyPayload(): VoiceBroadcastConfigPayload {
  return {
    apiKey: '',
    clearApiKey: false,
    currentVoice: 'Cherry',
    defaultVoice: 'Cherry',
    enabled: false,
    expectedVersion: 0,
    model: 'qwen3-tts-instruct-flash',
    provider: 'aliyun-bailian',
    region: 'cn-beijing',
    workspaceId: '',
  };
}

function applyView(next: VoiceBroadcastConfigView) {
  config.value = next;
  Object.assign(form, {
    currentVoice: next.currentVoice,
    defaultVoice: next.defaultVoice,
    enabled: next.enabled,
    expectedVersion: next.version,
    model: next.model,
    provider: next.provider,
    region: next.region,
    voices: next.voices,
    workspaceId: next.workspaceId,
  });
  apiKey.value = '';
}

async function load() {
  loading.value = true;
  loadError.value = '';
  try {
    applyView(await getVoiceBroadcastConfigApi());
  } catch {
    loadError.value = '语音播报配置加载失败，请重新加载';
  } finally {
    loading.value = false;
  }
}

function payload(): VoiceBroadcastConfigPayload {
  return {
    ...form,
    apiKey: apiKey.value.trim(),
    clearApiKey: false,
    expectedVersion: config.value.version,
  };
}

function isConflict(error: unknown) {
  const root = error as {
    code?: number | string;
    message?: string;
    response?: { data?: { code?: number | string; message?: string }; status?: number };
    status?: number;
  };
  const values = [
    root?.code,
    root?.message,
    root?.response?.data?.code,
    root?.response?.data?.message,
  ];
  return (
    root?.status === 409 ||
    root?.response?.status === 409 ||
    values.some(
      (value) =>
        typeof value === 'string' && value.includes('voice_broadcast_config_version_conflict'),
    )
  );
}

async function save() {
  if (loading.value || saving.value || testing.value || loadError.value) return;
  saving.value = true;
  try {
    applyView(await updateVoiceBroadcastConfigApi(payload()));
    message.success('语音播报配置已保存');
  } catch (error) {
    if (isConflict(error)) {
      message.warning('配置已被其他管理员更新，已重新加载最新版本');
      await load();
    } else {
      message.error('语音播报配置保存失败');
    }
  } finally {
    saving.value = false;
  }
}

async function testSynthesis() {
  if (loading.value || saving.value || testing.value || loadError.value) return;
  testing.value = true;
  try {
    const tested = await testVoiceBroadcastConfigApi(payload());
    // Keep unsaved form values (especially a write-only API key) while
    // accepting the server's health result and the new CAS version.
    config.value = tested;
    form.expectedVersion = tested.version;
    message.success('语音合成测试已完成');
  } catch (error) {
    if (isConflict(error)) {
      message.warning('配置已被其他管理员更新，已重新加载最新版本');
      await load();
    } else {
      message.error('语音合成测试失败');
    }
  } finally {
    testing.value = false;
  }
}
</script>

<template>
  <Page title="语音播报配置" description="配置会话文字回复的阿里百炼自动语音播报。">
    <Card :bordered="false" :loading="loading">
      <Alert
        v-if="loadError"
        class="mb-4"
        :message="loadError"
        show-icon
        type="error"
      />
      <template v-else>
        <Alert
          class="mb-4"
          :description="`当前 API Key：${keyStatus}；配置版本 ${config.version}`"
          message="语音播报能力"
          show-icon
          :type="config.enabled ? 'success' : 'info'"
        />
        <Form layout="vertical">
          <Form.Item label="全局启用">
            <Switch
              v-model:checked="form.enabled"
              data-testid="voice-broadcast-enabled"
              checked-children="启用"
              un-checked-children="停用"
            />
          </Form.Item>
          <Form.Item label="供应商">
            <Input v-model:value="form.provider" disabled />
          </Form.Item>
          <Form.Item label="地域">
            <Input v-model:value="form.region" data-testid="voice-broadcast-region" />
          </Form.Item>
          <Form.Item label="Workspace ID">
            <Input v-model:value="form.workspaceId" />
          </Form.Item>
          <Form.Item label="TTS 模型">
            <Input v-model:value="form.model" />
          </Form.Item>
          <Form.Item label="阿里百炼 API Key">
            <Input.Password
              v-model:value="apiKey"
              data-testid="voice-broadcast-api-key"
              placeholder="留空表示保留现有 Key"
            />
            <div class="form-tip">当前状态：{{ keyStatus }}。密钥只写入，不会回显。</div>
          </Form.Item>
          <Form.Item label="默认女声音色">
            <Input :value="form.defaultVoice" disabled />
          </Form.Item>
          <Form.Item label="当前启用音色">
            <Select
              v-model:value="form.currentVoice"
              :options="voiceOptions"
              data-testid="voice-broadcast-current-voice"
              show-search
            />
          </Form.Item>
        </Form>
        <Alert
          class="health-alert"
          :description="healthMessage"
          message="最近一次合成测试"
          show-icon
          :type="healthType"
        >
          <template #message>
            <Space>
              <span>最近一次合成测试</span>
              <Tag v-if="config.health.status === 'ok'" color="success">正常</Tag>
              <Tag v-else-if="config.health.status === 'error'" color="error">失败</Tag>
              <Tag v-else color="default">未测试</Tag>
            </Space>
          </template>
        </Alert>
        <Space class="actions" wrap>
          <Button
            type="primary"
            :loading="saving"
            data-testid="voice-broadcast-save"
            @click="save"
          >
            保存配置
          </Button>
          <Button
            :loading="testing"
            :disabled="saving"
            data-testid="voice-broadcast-test"
            @click="testSynthesis"
          >
            测试合成
          </Button>
          <Button :disabled="saving || testing" @click="load">重新加载</Button>
        </Space>
      </template>
    </Card>
  </Page>
</template>

<style scoped>
.form-tip {
  color: #667085;
  font-size: 13px;
  margin-top: 6px;
}

.health-alert {
  margin-top: 20px;
}

.actions {
  margin-top: 20px;
}
</style>
