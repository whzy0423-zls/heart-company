<script setup lang="ts">
import type {
  XinzhiliMode,
  XinzhiliModelConfigPayload,
  XinzhiliModelConfigView,
} from '#/api';

import { onMounted, ref } from 'vue';

import {
  Alert,
  Col,
  Divider,
  Form,
  Input,
  InputNumber,
  message,
  Row,
  Select,
  Switch,
  Textarea,
} from 'ant-design-vue';

import {
  getXinzhiliModelConfigApi,
  updateXinzhiliModelConfigApi,
} from '#/api';

import EditorShell from '../site-config/components/editor-shell.vue';

const modes: Array<{ description: string; label: string; value: XinzhiliMode }> = [
  { description: '基础实时语音交流', label: '正常模式', value: 'normal' },
  { description: '允许 AI 在合适时机抢答', label: '抬杠模式', value: 'argument' },
  { description: '在用户沉默或低落时主动安慰', label: '安慰模式', value: 'comfort' },
  { description: '给予用户更长的表达空间', label: '深度倾听', value: 'deep_listening' },
];

const ttsProviderOptions = [
  { label: 'OpenAI 兼容协议', value: 'openai-compatible' },
  { label: 'MiniMax', value: 'minimax' },
];

const loading = ref(true);
const saving = ref(false);
const loadError = ref('');
const version = ref(0);
const asrKeySet = ref(false);
const asrKeySuffix = ref('');
const ttsKeySet = ref(false);
const ttsKeySuffix = ref('');

const form = ref<XinzhiliModelConfigPayload>(createEmptyForm());

function createEmptyForm(): XinzhiliModelConfigPayload {
  return {
    commonPrompt: '',
    enabled: false,
    enabledModes: ['normal'],
    expectedVersion: 0,
    modePrompts: {},
    realtimeAsr: {
      apiKey: '',
      endpoint: 'wss://dashscope.aliyuncs.com/api-ws/v1/inference',
      model: 'paraformer-realtime-v2',
      provider: 'aliyun-bailian',
      region: 'cn-beijing',
    },
    timing: {
      argumentCandidateSilenceMs: 350,
      comfortEndSilenceMs: 1200,
      comfortFirstPromptMs: 5000,
      comfortSecondPromptMs: 12000,
      deepListeningEndSilenceMs: 1500,
      deepListeningPromptMs: 12000,
      maxProactivePrompts: 2,
      normalEndSilenceMs: 700,
      partialStableMs: 150,
    },
    tts: {
      apiKey: '',
      endpoint: '',
      format: 'mp3',
      groupId: '',
      model: '',
      provider: 'openai-compatible',
      voice: '',
    },
  };
}

onMounted(load);

async function load() {
  loading.value = true;
  loadError.value = '';
  try {
    applyView(await getXinzhiliModelConfigApi());
  } catch {
    loadError.value = '芯之力模型配置加载失败，请重新加载';
  } finally {
    loading.value = false;
  }
}

function applyView(data: XinzhiliModelConfigView) {
  version.value = data.version;
  asrKeySet.value = data.realtimeAsr.apiKeySet;
  asrKeySuffix.value = data.realtimeAsr.apiKeySuffix;
  ttsKeySet.value = data.tts.apiKeySet;
  ttsKeySuffix.value = data.tts.apiKeySuffix;
  form.value = {
    commonPrompt: data.commonPrompt ?? '',
    enabled: data.enabled,
    enabledModes: data.enabledModes.includes('normal')
      ? [...data.enabledModes]
      : ['normal', ...data.enabledModes],
    expectedVersion: data.version,
    modePrompts: { ...data.modePrompts },
    realtimeAsr: {
      apiKey: '',
      endpoint: data.realtimeAsr.endpoint,
      model: 'paraformer-realtime-v2',
      provider: 'aliyun-bailian',
      region: data.realtimeAsr.region,
    },
    timing: { ...data.timing },
    tts: {
      apiKey: '',
      endpoint: data.tts.endpoint,
      format: 'mp3',
      groupId: data.tts.groupId ?? '',
      model: data.tts.model,
      provider: data.tts.provider,
      voice: data.tts.voice,
    },
  };
}

function isModeEnabled(mode: XinzhiliMode) {
  return form.value.enabledModes.includes(mode);
}

function setModeEnabled(mode: XinzhiliMode, checked: boolean | number | string) {
  if (mode === 'normal') return;
  const enabled = checked === true;
  form.value.enabledModes = enabled
    ? [...new Set([...form.value.enabledModes, mode])]
    : form.value.enabledModes.filter((item) => item !== mode);
}

function errorStatus(error: unknown) {
  return (error as { response?: { status?: number } })?.response?.status;
}

async function save() {
  if (loadError.value) {
    message.warning('请先重新加载芯之力模型配置');
    return;
  }
  saving.value = true;
  try {
    form.value.enabledModes = [
      'normal',
      ...form.value.enabledModes.filter((mode) => mode !== 'normal'),
    ];
    form.value.expectedVersion = version.value;
    const saved = await updateXinzhiliModelConfigApi(form.value);
    applyView(saved);
    form.value.realtimeAsr.apiKey = '';
    form.value.tts.apiKey = '';
    version.value = saved.version;
    message.success(`芯之力模型配置已保存，当前版本 ${saved.version}`);
  } catch (error) {
    const status = errorStatus(error);
    if (status === 409) {
      message.warning('配置已被其他管理员修改，请重新加载');
    }
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <EditorShell
    description="单独配置芯之力实时语音识别、语音合成和对话模式；不影响普通聊天语音。"
    :loading="loading"
    :save-disabled="!!loadError"
    :saving="saving"
    title="芯之力模型配置"
    @save="save"
  >
    <Alert
      v-if="loadError"
      :message="loadError"
      show-icon
      type="error"
      style="margin-bottom: 16px"
    />

    <Form layout="vertical">
      <Form.Item label="启用芯之力实时语音">
        <Switch v-model:checked="form.enabled" />
        <span class="ml-3 text-xs text-gray-400">配置版本：{{ version }}</span>
      </Form.Item>

      <Divider orientation="left">实时语音识别（阿里云百炼）</Divider>
      <Row :gutter="24">
        <Col :md="12" :xs="24">
          <Form.Item label="WebSocket Endpoint">
            <Input v-model:value="form.realtimeAsr.endpoint" />
          </Form.Item>
          <Form.Item label="区域">
            <Input
              v-model:value="form.realtimeAsr.region"
              placeholder="cn-beijing"
            />
          </Form.Item>
        </Col>
        <Col :md="12" :xs="24">
          <Form.Item label="模型">
            <Input :value="form.realtimeAsr.model" disabled />
          </Form.Item>
          <Form.Item label="API Key">
            <Input.Password
              v-model:value="form.realtimeAsr.apiKey"
              :placeholder="
                asrKeySet
                  ? `已配置${asrKeySuffix ? `（尾号 ${asrKeySuffix}）` : ''}，留空表示不修改`
                  : '请输入 API Key'
              "
              autocomplete="new-password"
            />
          </Form.Item>
        </Col>
      </Row>

      <Divider orientation="left">AI 语音合成</Divider>
      <Row :gutter="24">
        <Col :md="12" :xs="24">
          <Form.Item label="协议">
            <Select
              v-model:value="form.tts.provider"
              :options="ttsProviderOptions"
            />
          </Form.Item>
          <Form.Item label="Endpoint">
            <Input v-model:value="form.tts.endpoint" />
          </Form.Item>
          <Form.Item v-if="form.tts.provider === 'minimax'" label="Group ID">
            <Input v-model:value="form.tts.groupId" />
          </Form.Item>
        </Col>
        <Col :md="12" :xs="24">
          <Form.Item label="模型">
            <Input v-model:value="form.tts.model" />
          </Form.Item>
          <Form.Item label="音色">
            <Input v-model:value="form.tts.voice" />
          </Form.Item>
          <Form.Item label="API Key">
            <Input.Password
              v-model:value="form.tts.apiKey"
              :placeholder="
                ttsKeySet
                  ? `已配置${ttsKeySuffix ? `（尾号 ${ttsKeySuffix}）` : ''}，留空表示不修改`
                  : '请输入 API Key'
              "
              autocomplete="new-password"
            />
          </Form.Item>
        </Col>
      </Row>

      <Divider orientation="left">对话模式</Divider>
      <Row :gutter="24">
        <Col v-for="mode in modes" :key="mode.value" :md="12" :xs="24">
          <Form.Item :label="mode.label">
            <Switch
              :checked="isModeEnabled(mode.value)"
              :disabled="mode.value === 'normal'"
              @change="(checked) => setModeEnabled(mode.value, checked)"
            />
            <span class="ml-3 text-xs text-gray-400">{{ mode.description }}</span>
          </Form.Item>
          <Form.Item :label="`${mode.label}提示词`">
            <Textarea v-model:value="form.modePrompts[mode.value]" :rows="3" />
          </Form.Item>
        </Col>
      </Row>

      <Form.Item label="公共提示词">
        <Textarea v-model:value="form.commonPrompt" :rows="4" />
      </Form.Item>

      <Divider orientation="left">时序参数（毫秒）</Divider>
      <Row :gutter="16">
        <Col :md="8" :xs="12">
          <Form.Item label="识别文本稳定">
            <InputNumber v-model:value="form.timing.partialStableMs" :min="100" :max="1000" />
          </Form.Item>
        </Col>
        <Col :md="8" :xs="12">
          <Form.Item label="抬杠候选静默">
            <InputNumber v-model:value="form.timing.argumentCandidateSilenceMs" :min="250" :max="600" />
          </Form.Item>
        </Col>
        <Col :md="8" :xs="12">
          <Form.Item label="正常结束静默">
            <InputNumber v-model:value="form.timing.normalEndSilenceMs" :min="350" :max="2000" />
          </Form.Item>
        </Col>
        <Col :md="8" :xs="12">
          <Form.Item label="安慰结束静默">
            <InputNumber v-model:value="form.timing.comfortEndSilenceMs" :min="700" :max="3000" />
          </Form.Item>
        </Col>
        <Col :md="8" :xs="12">
          <Form.Item label="深度倾听结束静默">
            <InputNumber v-model:value="form.timing.deepListeningEndSilenceMs" :min="1000" :max="5000" />
          </Form.Item>
        </Col>
        <Col :md="8" :xs="12">
          <Form.Item label="首次安慰提示">
            <InputNumber v-model:value="form.timing.comfortFirstPromptMs" :min="3000" :max="30000" />
          </Form.Item>
        </Col>
        <Col :md="8" :xs="12">
          <Form.Item label="二次安慰提示">
            <InputNumber v-model:value="form.timing.comfortSecondPromptMs" :min="3001" :max="60000" />
          </Form.Item>
        </Col>
        <Col :md="8" :xs="12">
          <Form.Item label="深度倾听提示">
            <InputNumber v-model:value="form.timing.deepListeningPromptMs" :min="5000" :max="60000" />
          </Form.Item>
        </Col>
        <Col :md="8" :xs="12">
          <Form.Item label="最多主动提示次数">
            <InputNumber v-model:value="form.timing.maxProactivePrompts" :min="0" :max="5" />
          </Form.Item>
        </Col>
      </Row>
    </Form>
  </EditorShell>
</template>
