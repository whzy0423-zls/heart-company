<script setup lang="ts">
import type {
  XinzhiliMode,
  XinzhiliModelConfigPayload,
  VoiceOption,
  XinzhiliModelConfigView,
} from '#/api';

import { computed, onMounted, ref, watch } from 'vue';

import {
  Alert,
  Button,
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
  getVoiceOptionsApi,
  getXinzhiliModelConfigApi,
  updateXinzhiliModelConfigApi,
} from '#/api';

import EditorShell from '../site-config/components/editor-shell.vue';
import { normalizeXinzhiliModelConfigView } from './xinzhili-model-normalize';

type XinzhiliTtsProvider = XinzhiliModelConfigPayload['tts']['provider'];

const modes: Array<{ description: string; label: string; value: XinzhiliMode }> = [
  { description: '基础实时语音交流', label: '正常模式', value: 'normal' },
  { description: '允许 AI 在合适时机抢答', label: '抬杠模式', value: 'argument' },
  { description: '在用户沉默或低落时主动安慰', label: '安慰模式', value: 'comfort' },
  { description: '给予用户更长的表达空间', label: '深度倾听', value: 'deep_listening' },
];

const ttsProviderOptions = [
  { label: '阿里百炼', value: 'bailian' },
  { label: 'MiniMax', value: 'minimax' },
  { label: 'OpenAI 兼容协议', value: 'openai-compatible' },
];
const aliyunBailianTtsPreset = {
  endpoint: 'https://dashscope.aliyuncs.com/api/v1',
  model: 'MiniMax/speech-2.8-turbo',
};
const legacyAliyunBailianTtsPreset = {
  endpoint: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
  model: 'qwen-audio-tts-latest',
};

const loading = ref(true);
const saving = ref(false);
const loadError = ref('');
const version = ref(0);
const asrKeySet = ref(false);
const asrKeySuffix = ref('');
const ttsKeySet = ref(false);
const ttsKeySuffix = ref('');
const ttsVoiceOptions = ref<VoiceOption[]>([]);
const ttsVoiceOptionsLoading = ref(false);
const ttsVoiceOptionsError = ref('');
const selectedTtsVoiceOptionId = ref('');

const canSelectExistingTtsVoice = computed(() =>
  ['bailian', 'minimax'].includes(form.value.tts.provider),
);
const currentTtsVoiceProvider = computed(() =>
  form.value.tts.provider === 'minimax' ? 'minimax' : 'bailian',
);
const filteredTtsVoiceOptions = computed(() =>
  ttsVoiceOptions.value.filter(
    (item) => voiceOptionProvider(item) === currentTtsVoiceProvider.value,
  ),
);
const groupedTtsVoiceOptions = computed(() =>
  [
    {
      label: 'MiniMax 官方音色',
      options: filteredTtsVoiceOptions.value
        .filter((item) => item.source === 'official')
        .map((item) => ({ label: item.label, value: item.id })),
    },
    {
      label:
        currentTtsVoiceProvider.value === 'bailian'
          ? '阿里百炼 · 已复刻音色'
          : '声音管理 · 已克隆音色',
      options: filteredTtsVoiceOptions.value
        .filter((item) => item.source === 'clone')
        .map((item) => ({ label: item.label, value: item.id })),
    },
  ].filter((group) => group.options.length > 0),
);

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
watch(
  () => form.value.tts.voice,
  () => syncSelectedTtsVoiceOption(),
);
watch(
  () => form.value.tts.provider,
  (provider) => {
    applyTtsProviderPreset(provider);
    syncSelectedTtsVoiceOption();
  },
);

async function load() {
  loading.value = true;
  loadError.value = '';
  try {
    applyView(await getXinzhiliModelConfigApi());
    await loadTtsVoiceOptions();
  } catch {
    loadError.value = '芯之力模型配置加载失败，请重新加载';
  } finally {
    loading.value = false;
  }
}

function applyView(data: XinzhiliModelConfigView) {
  const normalized = normalizeXinzhiliModelConfigView(data);
  const provider = normalizeTtsProvider(
    normalized.tts.provider,
    normalized.tts.endpoint,
  );
  version.value = normalized.version;
  asrKeySet.value = normalized.realtimeAsr.apiKeySet;
  asrKeySuffix.value = normalized.realtimeAsr.apiKeySuffix;
  ttsKeySet.value = normalized.tts.apiKeySet;
  ttsKeySuffix.value = normalized.tts.apiKeySuffix;
  form.value = {
    commonPrompt: normalized.commonPrompt ?? '',
    enabled: normalized.enabled,
    enabledModes: normalized.enabledModes,
    expectedVersion: normalized.version,
    modePrompts: normalized.modePrompts,
    realtimeAsr: {
      apiKey: '',
      endpoint: normalized.realtimeAsr.endpoint,
      model: 'paraformer-realtime-v2',
      provider: 'aliyun-bailian',
      region: normalized.realtimeAsr.region,
    },
    timing: { ...normalized.timing },
    tts: {
      apiKey: '',
      endpoint: normalized.tts.endpoint,
      format: 'mp3',
      groupId: normalized.tts.groupId ?? '',
      model: normalized.tts.model,
      provider,
      voice: normalized.tts.voice,
    },
  };
  applyTtsProviderPreset(provider);
  syncSelectedTtsVoiceOption();
}

async function loadTtsVoiceOptions() {
  ttsVoiceOptionsLoading.value = true;
  ttsVoiceOptionsError.value = '';
  try {
    ttsVoiceOptions.value = await getVoiceOptionsApi();
    syncSelectedTtsVoiceOption();
  } catch {
    ttsVoiceOptions.value = [];
    selectedTtsVoiceOptionId.value = '';
    ttsVoiceOptionsError.value = '音色选项读取失败，可手动填写音色 ID';
  } finally {
    ttsVoiceOptionsLoading.value = false;
  }
}

function handleTtsVoiceOptionChange(optionId?: unknown) {
  const selected = Array.isArray(optionId) ? optionId[0] : optionId;
  const selectedId =
    typeof selected === 'number' || typeof selected === 'string'
      ? String(selected)
      : '';
  selectedTtsVoiceOptionId.value = selectedId;
  const option = ttsVoiceOptions.value.find((item) => item.id === selectedId);
  if (!option) return;

  const provider = voiceOptionProvider(option);
  if (provider === 'bailian') {
    form.value.tts.provider = 'bailian';
    applyTtsProviderPreset('bailian', true);
  } else if (provider === 'minimax') {
    form.value.tts.provider = 'minimax';
    applyTtsProviderPreset('minimax');
  }
  form.value.tts.voice = option.voiceId;
}

function syncSelectedTtsVoiceOption() {
  const matched = filteredTtsVoiceOptions.value.find(
    (item) => item.voiceId === form.value.tts.voice,
  );
  selectedTtsVoiceOptionId.value = matched?.id ?? '';
}

function voiceOptionProvider(item: VoiceOption) {
  return item.provider || 'minimax';
}

function normalizeTtsProvider(
  provider?: string,
  endpoint?: string,
): XinzhiliTtsProvider {
  const normalizedProvider = (provider || '').trim();
  const normalizedEndpoint = (endpoint || '').trim().toLowerCase();
  if (
    normalizedProvider === 'openai-compatible' &&
    normalizedEndpoint.includes('dashscope.aliyuncs.com/compatible-mode')
  ) {
    return 'bailian';
  }
  if (normalizedProvider === 'bailian' || normalizedProvider === 'minimax') {
    return normalizedProvider;
  }
  return 'openai-compatible';
}

function applyTtsProviderPreset(
  provider: XinzhiliTtsProvider = form.value.tts.provider,
  force = false,
) {
  if (provider === 'bailian') {
    form.value.tts.endpoint =
      force ||
      !form.value.tts.endpoint ||
      form.value.tts.endpoint === legacyAliyunBailianTtsPreset.endpoint
        ? aliyunBailianTtsPreset.endpoint
        : form.value.tts.endpoint;
    form.value.tts.model =
      force ||
      !form.value.tts.model ||
      form.value.tts.model === 'speech-02-hd' ||
      form.value.tts.model === legacyAliyunBailianTtsPreset.model
        ? aliyunBailianTtsPreset.model
        : form.value.tts.model;
    form.value.tts.groupId = '';
    form.value.tts.format = 'mp3';
    return;
  }
  if (provider === 'minimax') {
    if (
      form.value.tts.endpoint === aliyunBailianTtsPreset.endpoint ||
      form.value.tts.endpoint === legacyAliyunBailianTtsPreset.endpoint
    ) {
      form.value.tts.endpoint = '';
    }
    if (
      !form.value.tts.model ||
      form.value.tts.model === aliyunBailianTtsPreset.model ||
      form.value.tts.model === legacyAliyunBailianTtsPreset.model
    ) {
      form.value.tts.model = 'speech-02-hd';
    }
    form.value.tts.format = 'mp3';
  }
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

function applyFreeTtsPreset() {
  form.value.tts = {
    ...form.value.tts,
    endpoint: 'https://api.siliconflow.cn/v1',
    format: 'mp3',
    model: 'FunAudioLLM/CosyVoice2-0.5B',
    provider: 'openai-compatible',
    voice: 'FunAudioLLM/CosyVoice2-0.5B:alex',
  };
  message.success('已填充硅基流动免费额度 TTS 预设，请确认 API Key 后保存');
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

      <Divider orientation="left">AI 语音合成 / TTS 配置</Divider>
      <Alert
        description="实时 ASR 继续使用阿里云百炼 Paraformer；语音合成可使用硅基流动免费额度。预设按钮只填写 TTS 协议、地址、模型、音色和格式，按钮不会覆盖已填写的 API Key。"
        message="免费额度配置说明"
        show-icon
        type="info"
        style="margin-bottom: 16px"
      >
        <template #action>
          <Button size="small" type="primary" @click="applyFreeTtsPreset">
            填充免费额度 TTS 预设
          </Button>
        </template>
      </Alert>
      <Row :gutter="24">
        <Col :md="12" :xs="24">
          <Form.Item label="协议">
            <Select
              v-model:value="form.tts.provider"
              :options="ttsProviderOptions"
              placeholder="请选择 TTS provider"
            />
          </Form.Item>
          <Form.Item
            v-if="canSelectExistingTtsVoice"
            label="选择已有音色"
          >
            <Select
              v-model:value="selectedTtsVoiceOptionId"
              allow-clear
              :loading="ttsVoiceOptionsLoading"
              :options="groupedTtsVoiceOptions"
              option-filter-prop="label"
              placeholder="可直接复用声音管理里的已克隆音色"
              show-search
              @change="handleTtsVoiceOptionChange"
            />
            <span
              v-if="ttsVoiceOptionsError"
              class="mt-1 block text-xs text-red-500"
            >
              音色选项读取失败，可手动填写音色 ID
            </span>
            <span v-else class="mt-1 block text-xs text-gray-400">
              可直接复用声音管理里的已克隆音色，也可以手动填写平台音色 ID
            </span>
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
          <Form.Item label="手动音色 ID">
            <Input
              v-model:value="form.tts.voice"
              :placeholder="
                form.tts.provider === 'bailian'
                  ? '阿里百炼复刻音色 ID'
                  : '也可以手动填写平台音色 ID'
              "
            />
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
