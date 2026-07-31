<script setup lang="ts">
import type { BailianCredentialsCardStatus } from '../voice/bailian-credentials-card.vue';

import type {
  VoiceOption,
  XinzhiliMode,
  XinzhiliModelConfigPayload,
  XinzhiliModelConfigView,
} from '#/api';

import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';

import { useAccessStore } from '@vben/stores';

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
import BailianCredentialsCard from '../voice/bailian-credentials-card.vue';
import { normalizeXinzhiliModelConfigView } from './xinzhili-model-normalize';

type XinzhiliTtsProvider = XinzhiliModelConfigPayload['tts']['provider'];

const modes: Array<{
  description: string;
  label: string;
  value: XinzhiliMode;
}> = [
  { description: '基础实时语音交流', label: '正常模式', value: 'normal' },
  {
    description: '允许 AI 在合适时机抢答',
    label: '抬杠模式',
    value: 'argument',
  },
  {
    description: '在用户沉默或低落时主动安慰',
    label: '安慰模式',
    value: 'comfort',
  },
  {
    description: '给予用户更长的表达空间',
    label: '深度倾听',
    value: 'deep_listening',
  },
];

const ttsProviderOptions = [
  { label: '阿里百炼', value: 'bailian' },
  { label: 'MiniMax', value: 'minimax' },
  { label: 'OpenAI 兼容协议', value: 'openai-compatible' },
];
const aliyunBailianTtsPreset = {
  endpoint: 'https://dashscope.aliyuncs.com/api/v1',
  model: 'qwen3-tts-vc-2026-01-22',
};
const legacyAliyunBailianTtsPreset = {
  endpoint: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
  model: 'qwen-audio-tts-latest',
};
const miniMaxTtsPreset = {
  endpoint: 'https://api.minimax.chat/v1/t2a_v2',
  model: 'speech-02-hd',
};

const accessStore = useAccessStore();
const loading = ref(true);
const saving = ref(false);
const loadError = ref('');
const version = ref(0);
const ttsKeySet = ref(false);
const ttsKeySuffix = ref('');
const credentialStatus = ref<BailianCredentialsCardStatus>({
  apiKeySet: false,
  error: null,
  loading: true,
  saving: false,
  source: 'none',
  version: 0,
});
const ttsVoiceOptions = ref<VoiceOption[]>([]);
const ttsVoiceOptionsLoading = ref(false);
const ttsVoiceOptionsError = ref('');
const selectedTtsVoiceOptionId = ref('');
let loadSequence = 0;
let unmounted = false;

const canManageVoiceProfiles = computed(() =>
  accessStore.accessCodes.includes('Voice:Profile:Manage'),
);
const usesSharedBailianTts = computed(
  () =>
    ['bailian', 'openai-compatible'].includes(form.value.tts.provider) &&
    isOfficialDashScopeTtsEndpoint(form.value.tts.endpoint),
);
const sharedCredentialGateMessage = computed(() => {
  if (credentialStatus.value.error) {
    return '百炼凭证读取失败，可在上方重新加载';
  }
  if (credentialStatus.value.loading) {
    return '正在读取百炼公共 API Key，请稍候';
  }
  if (credentialStatus.value.saving) {
    return '百炼公共 API Key 正在保存，请稍候';
  }
  return '请先在上方保存百炼公共 API Key';
});
const sharedCredentialUnavailable = computed(
  () =>
    !credentialStatus.value.apiKeySet ||
    credentialStatus.value.loading ||
    Boolean(credentialStatus.value.error),
);
const saveDisabled = computed(
  () =>
    Boolean(loadError.value) ||
    (form.value.enabled &&
      (credentialStatus.value.saving || sharedCredentialUnavailable.value)),
);

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
    enabledModes: ['normal', 'argument', 'comfort', 'deep_listening'],
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
      argumentCandidateSilenceMs: 300,
      comfortEndSilenceMs: 900,
      comfortFirstPromptMs: 5000,
      comfortSecondPromptMs: 12_000,
      deepListeningEndSilenceMs: 1200,
      deepListeningPromptMs: 12_000,
      maxProactivePrompts: 2,
      normalEndSilenceMs: 500,
      partialStableMs: 120,
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
onBeforeUnmount(() => {
  unmounted = true;
  loadSequence += 1;
});
watch(
  () => form.value.tts.voice,
  () => syncSelectedTtsVoiceOption(),
);
watch(
  () => form.value.tts.endpoint,
  (endpoint) => {
    const nextOrigin = ttsEndpointOrigin(endpoint);
    if (nextOrigin !== currentTtsEndpointOrigin) resetTtsPrivateSecret();
    currentTtsEndpointOrigin = nextOrigin;
  },
);

let currentTtsEndpointOrigin = '';

async function load() {
  if (unmounted || saving.value) return;
  const currentSequence = ++loadSequence;
  loading.value = true;
  loadError.value = '';
  try {
    const next = await getXinzhiliModelConfigApi();
    if (unmounted || currentSequence !== loadSequence) return;
    applyView(next);
    await loadTtsVoiceOptions(currentSequence);
  } catch {
    if (unmounted || currentSequence !== loadSequence) return;
    loadError.value = '芯之力模型配置加载失败，请重新加载';
  } finally {
    if (!unmounted && currentSequence === loadSequence) {
      loading.value = false;
    }
  }
}

function applyView(data: XinzhiliModelConfigView) {
  const normalized = normalizeXinzhiliModelConfigView(data);
  const provider = normalizeTtsProvider(
    normalized.tts.provider,
    normalized.tts.endpoint,
    normalized.tts.model,
  );
  version.value = normalized.version;
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
  currentTtsEndpointOrigin = ttsEndpointOrigin(form.value.tts.endpoint);
  syncSelectedTtsVoiceOption();
}

async function loadTtsVoiceOptions(currentSequence = loadSequence) {
  if (unmounted || currentSequence !== loadSequence) return;
  ttsVoiceOptionsLoading.value = true;
  ttsVoiceOptionsError.value = '';
  try {
    const options = await getVoiceOptionsApi();
    if (unmounted || currentSequence !== loadSequence) return;
    ttsVoiceOptions.value = options;
    syncSelectedTtsVoiceOption();
  } catch {
    if (unmounted || currentSequence !== loadSequence) return;
    ttsVoiceOptions.value = [];
    selectedTtsVoiceOptionId.value = '';
    ttsVoiceOptionsError.value = '音色选项读取失败，可手动填写音色 ID';
  } finally {
    if (!unmounted && currentSequence === loadSequence) {
      ttsVoiceOptionsLoading.value = false;
    }
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
    changeTtsProvider('bailian');
    applyTtsProviderPreset('bailian', true);
    form.value.tts.model = option.model || aliyunBailianTtsPreset.model;
  } else if (provider === 'minimax') {
    changeTtsProvider('minimax');
    applyTtsProviderPreset('minimax', true);
  }
  form.value.tts.voice = option.voiceId;
}

function handleTtsProviderChange(providerValue?: unknown) {
  const selected = Array.isArray(providerValue)
    ? providerValue[0]
    : providerValue;
  if (
    selected !== 'bailian' &&
    selected !== 'minimax' &&
    selected !== 'openai-compatible'
  ) {
    return;
  }
  changeTtsProvider(selected, true);
}

function changeTtsProvider(provider: XinzhiliTtsProvider, force = false) {
  if (!force && provider === form.value.tts.provider) return;
  form.value.tts.provider = provider;
  resetTtsPrivateSecret();
  form.value.tts.voice = '';
  selectedTtsVoiceOptionId.value = '';
  applyTtsProviderPreset(provider, true);
  currentTtsEndpointOrigin = ttsEndpointOrigin(form.value.tts.endpoint);
}

function resetTtsPrivateSecret() {
  form.value.tts.apiKey = '';
  ttsKeySet.value = false;
  ttsKeySuffix.value = '';
}

function ttsEndpointOrigin(endpoint?: string) {
  const value = (endpoint || '').trim();
  if (!value) return '';
  try {
    return new URL(value).origin.toLowerCase();
  } catch {
    return `invalid:${value.toLowerCase()}`;
  }
}

function isOfficialDashScopeTtsEndpoint(endpoint?: string) {
  const value = (endpoint || '').trim();
  if (!value) return false;
  try {
    const url = new URL(value);
    const rawPath = rawEndpointPath(value);
    if (rawPath.includes('%') || rawPath !== url.pathname) return false;
    const endpointPath = url.pathname.endsWith('/')
      ? url.pathname.slice(0, -1)
      : url.pathname;
    return (
      url.protocol === 'https:' &&
      url.hostname.toLowerCase() === 'dashscope.aliyuncs.com' &&
      url.port === '' &&
      url.username === '' &&
      url.password === '' &&
      url.search === '' &&
      url.hash === '' &&
      [
        '/api/v1',
        '/compatible-mode/v1',
        '/api/v1/services/aigc/multimodal-generation/generation',
      ].includes(endpointPath)
    );
  } catch {
    return false;
  }
}

function rawEndpointPath(value: string) {
  const authorityStart = value.indexOf('://') + 3;
  const pathStart = value.indexOf('/', authorityStart);
  if (authorityStart < 3 || pathStart < 0) return '';
  const queryStart = value.indexOf('?', pathStart);
  const fragmentStart = value.indexOf('#', pathStart);
  const pathEnd = [queryStart, fragmentStart]
    .filter((index) => index >= 0)
    .reduce((current, index) => Math.min(current, index), value.length);
  return value.slice(pathStart, pathEnd);
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
  model?: string,
): XinzhiliTtsProvider {
  const normalizedProvider = (provider || '').trim();
  const normalizedEndpoint = (endpoint || '').trim().toLowerCase();
  const normalizedModel = (model || '').trim();
  if (
    normalizedProvider === 'openai-compatible' &&
    (isOfficialDashScopeTtsEndpoint(endpoint) ||
      (!normalizedEndpoint && !normalizedModel))
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
    form.value.tts.endpoint =
      force ||
      form.value.tts.endpoint === aliyunBailianTtsPreset.endpoint ||
      form.value.tts.endpoint === legacyAliyunBailianTtsPreset.endpoint
        ? miniMaxTtsPreset.endpoint
        : form.value.tts.endpoint;
    if (
      force ||
      !form.value.tts.model ||
      form.value.tts.model === aliyunBailianTtsPreset.model ||
      form.value.tts.model === legacyAliyunBailianTtsPreset.model
    ) {
      form.value.tts.model = miniMaxTtsPreset.model;
    }
    form.value.tts.format = 'mp3';
    return;
  }
  if (provider === 'openai-compatible' && force) {
    form.value.tts.endpoint = '';
    form.value.tts.model = '';
    form.value.tts.groupId = '';
    form.value.tts.format = 'mp3';
  }
}

function isModeEnabled(mode: XinzhiliMode) {
  return form.value.enabledModes.includes(mode);
}

function setModeEnabled(
  mode: XinzhiliMode,
  checked: boolean | number | string,
) {
  if (mode === 'normal') return;
  const enabled = checked === true;
  form.value.enabledModes = enabled
    ? [...new Set([...form.value.enabledModes, mode])]
    : form.value.enabledModes.filter((item) => item !== mode);
}

function handleCredentialStatusChange(status: BailianCredentialsCardStatus) {
  credentialStatus.value = status;
}

function isConfigVersionConflict(errorValue: unknown) {
  const marker = 'config_version_conflict';
  const root = errorValue as {
    code?: number | string;
    error?: string;
    message?: string;
    response?: {
      data?: {
        code?: number | string;
        error?: string;
        message?: string;
      };
      status?: number;
    };
    status?: number;
  };
  const responseBody = root?.response?.data;
  const values = [
    root?.error,
    root?.message,
    root?.code,
    responseBody?.error,
    responseBody?.message,
    responseBody?.code,
  ];

  return (
    root?.status === 409 ||
    root?.response?.status === 409 ||
    root?.code === 409 ||
    root?.code === '409' ||
    responseBody?.code === 409 ||
    responseBody?.code === '409' ||
    values.some((value) => typeof value === 'string' && value.includes(marker))
  );
}

async function save() {
  if (saving.value) return;
  if (loadError.value) {
    message.warning('请先重新加载芯之力模型配置');
    return;
  }
  if (form.value.enabled && credentialStatus.value.saving) {
    message.warning(sharedCredentialGateMessage.value);
    return;
  }
  if (form.value.enabled && sharedCredentialUnavailable.value) {
    message.warning(sharedCredentialGateMessage.value);
    return;
  }
  saving.value = true;
  try {
    const enabledModes = [
      'normal',
      ...form.value.enabledModes.filter((mode) => mode !== 'normal'),
    ] as XinzhiliMode[];
    const payload: XinzhiliModelConfigPayload = {
      commonPrompt: form.value.commonPrompt,
      enabled: form.value.enabled,
      enabledModes,
      expectedVersion: version.value,
      modePrompts: { ...form.value.modePrompts },
      realtimeAsr: {
        ...form.value.realtimeAsr,
        apiKey: '',
      },
      timing: { ...form.value.timing },
      tts: {
        ...form.value.tts,
        apiKey: usesSharedBailianTts.value ? '' : form.value.tts.apiKey,
      },
    };
    const saved = await updateXinzhiliModelConfigApi(payload);
    applyView(saved);
    form.value.realtimeAsr.apiKey = '';
    form.value.tts.apiKey = '';
    version.value = saved.version;
    message.success(`芯之力模型配置已保存，当前版本 ${saved.version}`);
  } catch (error) {
    if (isConfigVersionConflict(error)) {
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
    :save-disabled="saveDisabled"
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

    <BailianCredentialsCard
      class="mb-4"
      @status-change="handleCredentialStatusChange"
    />
    <Alert
      class="mb-4"
      description="公共 Key 统一用于 Paraformer 实时识别、Qwen 克隆音色和 Qwen TTS，不会写入芯之力模型配置。"
      message="百炼实时语音公共凭证"
      show-icon
      type="info"
    >
      <template v-if="canManageVoiceProfiles" #action>
        <Button href="/voice/profiles" size="small">
          前往人声管理克隆音色
        </Button>
      </template>
    </Alert>
    <Alert
      v-if="
        form.enabled && (sharedCredentialUnavailable || credentialStatus.saving)
      "
      class="mb-4"
      :message="sharedCredentialGateMessage"
      show-icon
      type="warning"
    />

    <Form layout="vertical">
      <Form.Item label="启用芯之力实时语音">
        <Switch v-model:checked="form.enabled" />
        <span class="ml-3 text-xs text-gray-400">配置版本：{{ version }}</span>
        <Button class="ml-3" :disabled="saving" size="small" @click="load">
          重新加载配置
        </Button>
      </Form.Item>

      <Divider orientation="left">实时语音识别（阿里云百炼）</Divider>
      <Row :gutter="24">
        <Col :md="12" :xs="24">
          <Form.Item label="WebSocket Endpoint">
            <Input
              v-model:value="form.realtimeAsr.endpoint"
              data-testid="realtime-asr-endpoint"
              placeholder="wss://dashscope.aliyuncs.com/api-ws/v1/inference"
            />
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
        </Col>
      </Row>

      <Divider orientation="left">AI 语音合成 / TTS 配置</Divider>
      <Row :gutter="24">
        <Col :md="12" :xs="24">
          <Form.Item label="协议">
            <Select
              :value="form.tts.provider"
              :options="ttsProviderOptions"
              placeholder="请选择 TTS provider"
              @change="handleTtsProviderChange"
            />
          </Form.Item>
          <Form.Item v-if="canSelectExistingTtsVoice" label="选择已有音色">
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
            <Input
              v-model:value="form.tts.endpoint"
              data-testid="tts-endpoint"
              placeholder="https://dashscope.aliyuncs.com/api/v1"
            />
          </Form.Item>
          <Form.Item v-if="form.tts.provider === 'minimax'" label="Group ID">
            <Input
              v-model:value="form.tts.groupId"
              placeholder="MiniMax Group ID"
            />
          </Form.Item>
        </Col>
        <Col :md="12" :xs="24">
          <Form.Item label="模型">
            <Input
              v-model:value="form.tts.model"
              data-testid="tts-model"
              placeholder="如 qwen3-tts-vc-2026-01-22"
            />
          </Form.Item>
          <Form.Item label="手动音色 ID">
            <Input
              v-model:value="form.tts.voice"
              data-testid="tts-voice"
              :placeholder="
                form.tts.provider === 'bailian'
                  ? '阿里百炼复刻音色 ID'
                  : '也可以手动填写平台音色 ID'
              "
            />
          </Form.Item>
          <Form.Item v-if="!usesSharedBailianTts" label="API Key">
            <Input.Password
              v-model:value="form.tts.apiKey"
              data-testid="private-tts-api-key"
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
            <span class="ml-3 text-xs text-gray-400">{{
              mode.description
            }}</span>
          </Form.Item>
          <Form.Item :label="`${mode.label}提示词`">
            <Textarea
              v-model:value="form.modePrompts[mode.value]"
              :data-testid="`mode-prompt-${mode.value}`"
              :rows="3"
              placeholder="可选：填写该模式的专属提示词"
            />
          </Form.Item>
        </Col>
      </Row>

      <Form.Item label="公共提示词">
        <Textarea
          v-model:value="form.commonPrompt"
          data-testid="common-prompt"
          :rows="4"
          placeholder="可选：填写所有模式共用的系统提示词"
        />
      </Form.Item>

      <Divider orientation="left">时序参数（毫秒）</Divider>
      <Row :gutter="16">
        <Col :md="8" :xs="12">
          <Form.Item label="识别文本稳定">
            <InputNumber
              v-model:value="form.timing.partialStableMs"
              data-testid="partial-stable-ms"
              :min="100"
              :max="1000"
              placeholder="如 150"
            />
          </Form.Item>
        </Col>
        <Col :md="8" :xs="12">
          <Form.Item label="抬杠候选静默">
            <InputNumber
              v-model:value="form.timing.argumentCandidateSilenceMs"
              :min="250"
              :max="600"
              placeholder="如 350"
            />
          </Form.Item>
        </Col>
        <Col :md="8" :xs="12">
          <Form.Item label="正常结束静默">
            <InputNumber
              v-model:value="form.timing.normalEndSilenceMs"
              :min="350"
              :max="2000"
              placeholder="如 700"
            />
          </Form.Item>
        </Col>
        <Col :md="8" :xs="12">
          <Form.Item label="安慰结束静默">
            <InputNumber
              v-model:value="form.timing.comfortEndSilenceMs"
              :min="700"
              :max="3000"
              placeholder="如 1200"
            />
          </Form.Item>
        </Col>
        <Col :md="8" :xs="12">
          <Form.Item label="深度倾听结束静默">
            <InputNumber
              v-model:value="form.timing.deepListeningEndSilenceMs"
              :min="1000"
              :max="5000"
              placeholder="如 1500"
            />
          </Form.Item>
        </Col>
        <Col :md="8" :xs="12">
          <Form.Item label="首次安慰提示">
            <InputNumber
              v-model:value="form.timing.comfortFirstPromptMs"
              :min="3000"
              :max="30000"
              placeholder="如 5000"
            />
          </Form.Item>
        </Col>
        <Col :md="8" :xs="12">
          <Form.Item label="二次安慰提示">
            <InputNumber
              v-model:value="form.timing.comfortSecondPromptMs"
              :min="3001"
              :max="60000"
              placeholder="如 12000"
            />
          </Form.Item>
        </Col>
        <Col :md="8" :xs="12">
          <Form.Item label="深度倾听提示">
            <InputNumber
              v-model:value="form.timing.deepListeningPromptMs"
              :min="5000"
              :max="60000"
              placeholder="如 12000"
            />
          </Form.Item>
        </Col>
        <Col :md="8" :xs="12">
          <Form.Item label="最多主动提示次数">
            <InputNumber
              v-model:value="form.timing.maxProactivePrompts"
              :min="0"
              :max="5"
              placeholder="如 2"
            />
          </Form.Item>
        </Col>
      </Row>
    </Form>
  </EditorShell>
</template>
