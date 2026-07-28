<script setup lang="ts">
import type {
  ChatPingResult,
  ModelConfigPayload,
  ModelConfigView,
  VoiceOption,
  XinzhiliModelConfigPayload,
  XinzhiliModelConfigView,
} from '#/api';

import { computed, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';

import {
  Alert,
  Button,
  Col,
  Divider,
  Form,
  Input,
  message,
  Row,
  Select,
  Switch,
} from 'ant-design-vue';

import {
  getModelConfigApi,
  getVoiceOptionsApi,
  getXinzhiliModelConfigApi,
  testChatModelApi,
  updateModelConfigApi,
  updateXinzhiliModelConfigApi,
} from '#/api';

import EditorShell from '../site-config/components/editor-shell.vue';

const loading = ref(true);
const saving = ref(false);
const route = useRoute();
const isAdminModelOnly = computed(() => route.path.includes('/settings/admin-model'));
const isXinzhiliModelOnly = computed(() =>
  route.path.includes('/settings/xinzhili-model'),
);
const pageTitle = computed(() =>
  isXinzhiliModelOnly.value
    ? '芯之力模型配置'
    : (isAdminModelOnly.value
      ? '管理端大模型配置'
      : '模型配置'),
);
const pageDescription = computed(() =>
  isXinzhiliModelOnly.value
    ? '配置芯之力 AI 语音合成能力，可直接复用声音管理里已经克隆完成的音色。'
    : (isAdminModelOnly.value
    ? '单独配置后台运营任务所用的大模型，包括每日 5 道画像校准题生成。'
    : '配置对话、视频生成、文生图与视频分析模型。视频分析固定复用语音生成的 MiniMax 地址与密钥，默认使用 MiniMax-M3 多模态模型。'),
);

// apiKey 留空表示不修改；apiKeySet 用于提示是否已配置过密钥
const form = ref<ModelConfigPayload & XinzhiliModelConfigPayload>({
  chat: {
    provider: 'openai-compatible',
    apiBase: 'https://coding-play.codes',
    apiKey: '',
    model: '',
  },
  video: { apiBase: '', apiKey: '', model: '' },
  image: { apiBase: '', apiKey: '', model: '' },
  analysis: { apiBase: '', apiKey: '', groupId: '', model: '' },
  admin: {
    apiBase: '',
    apiKey: '',
    groupId: '',
    model: '',
    provider: 'openai-compatible',
    timeoutSeconds: 30,
  },
  dailyQuiz: {
    apiBase: '',
    apiKey: '',
    groupId: '',
    model: '',
    provider: '',
    timeoutSeconds: 30,
  },
  tts: {
    apiKey: '',
    endpoint: 'https://dashscope.aliyuncs.com/api/v1',
    format: 'mp3',
    groupId: '',
    model: 'MiniMax/speech-2.8-turbo',
    provider: 'bailian',
    voice: '',
  },
  assist: { enabled: true, systemPrompt: '' },
});
const chatKeySet = ref(false);
const videoKeySet = ref(false);
const imageKeySet = ref(false);
const analysisKeySet = ref(false);
const adminKeySet = ref(false);
const dailyQuizKeySet = ref(false);
const ttsKeySet = ref(false);
const ttsVoiceOptions = ref<VoiceOption[]>([]);
const ttsVoiceOptionsLoading = ref(false);
const ttsVoiceOptionsError = ref('');
const selectedTtsVoiceOptionId = ref('');

const providerOptions = [
  { label: 'OpenAI 协议', value: 'openai-compatible' },
  { label: 'Anthropic 协议', value: 'anthropic-compatible' },
  { label: 'MiniMax 协议', value: 'minimax' },
];
const chatProviderOptions = [
  { label: 'OpenAI 协议', value: 'openai-compatible' },
  { label: 'Anthropic 协议', value: 'anthropic-compatible' },
];
const dailyQuizProviderOptions = [
  { label: '继承管理端', value: '' },
  ...providerOptions,
];
const ttsProviderOptions = [
  { label: '阿里百炼', value: 'bailian' },
  { label: 'MiniMax', value: 'minimax' },
];
const ttsFormatOptions = [{ label: 'MP3', value: 'mp3' }];
const aliyunBailianTtsPreset = {
  endpoint: 'https://dashscope.aliyuncs.com/api/v1',
  model: 'MiniMax/speech-2.8-turbo',
};
const legacyAliyunBailianTtsPreset = {
  endpoint: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
  model: 'qwen-audio-tts-latest',
};
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
const chatEndpointHint = computed(() => {
  const base = (form.value.chat.apiBase || 'https://coding-play.codes').replace(
    /\/$/,
    '',
  );
  return form.value.chat.provider === 'anthropic-compatible'
    ? `将请求 POST ${base}/v1/messages`
    : `将请求 POST ${base}/v1/chat/completions`;
});

onMounted(load);
watch(
  () => route.path,
  () => {
    load();
  },
);
watch(
  () => form.value.tts.voice,
  () => {
    syncSelectedTtsVoiceOption();
  },
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
  try {
    if (isXinzhiliModelOnly.value) {
      const data: XinzhiliModelConfigView = await getXinzhiliModelConfigApi();
      applyXinzhiliConfig(data);
      await loadTtsVoiceOptions();
      return;
    }
    const data: ModelConfigView = await getModelConfigApi();
    if (data) {
      const nextForm: ModelConfigPayload & XinzhiliModelConfigPayload = {
        chat: {
          provider: data.chat?.provider ?? 'openai-compatible',
          apiBase: data.chat?.apiBase || 'https://coding-play.codes',
          apiKey: '',
          model: data.chat?.model ?? '',
        },
        video: {
          apiBase: data.video?.apiBase ?? '',
          apiKey: '',
          model: data.video?.model ?? '',
        },
        image: {
          apiBase: data.image?.apiBase ?? '',
          apiKey: '',
          model: data.image?.model ?? '',
        },
        analysis: {
          apiBase: data.analysis?.apiBase ?? '',
          apiKey: '',
          groupId: data.analysis?.groupId ?? '',
          model: data.analysis?.model ?? '',
        },
        admin: {
          apiBase: data.admin?.apiBase ?? '',
          apiKey: '',
          groupId: data.admin?.groupId ?? '',
          model: data.admin?.model ?? '',
          provider: data.admin?.provider ?? 'openai-compatible',
          timeoutSeconds: data.admin?.timeoutSeconds ?? 30,
        },
        dailyQuiz: {
          apiBase: data.dailyQuiz?.apiBase ?? '',
          apiKey: '',
          groupId: data.dailyQuiz?.groupId ?? '',
          model: data.dailyQuiz?.model ?? '',
          provider: data.dailyQuiz?.provider ?? '',
          timeoutSeconds: data.dailyQuiz?.timeoutSeconds ?? 30,
        },
        tts: form.value.tts,
        assist: {
          enabled: data.assist?.enabled ?? true,
          systemPrompt: data.assist?.systemPrompt ?? '',
        },
      };
      form.value = nextForm;
      chatKeySet.value = data.chat?.apiKeySet ?? false;
      videoKeySet.value = data.video?.apiKeySet ?? false;
      imageKeySet.value = data.image?.apiKeySet ?? false;
      analysisKeySet.value = data.analysis?.apiKeySet ?? false;
      adminKeySet.value = data.admin?.apiKeySet ?? false;
      dailyQuizKeySet.value = data.dailyQuiz?.apiKeySet ?? false;
    }
  } finally {
    loading.value = false;
  }
}

async function save() {
  saving.value = true;
  try {
    if (isXinzhiliModelOnly.value) {
      const saved = await updateXinzhiliModelConfigApi({
        tts: {
          apiKey: form.value.tts.apiKey,
          endpoint: form.value.tts.endpoint,
          format: form.value.tts.format,
          groupId: form.value.tts.groupId,
          model: form.value.tts.model,
          provider: form.value.tts.provider,
          voice: form.value.tts.voice,
        },
      });
      form.value.tts.apiKey = '';
      ttsKeySet.value = saved.tts?.apiKeySet ?? false;
      applyXinzhiliConfig(saved);
      message.success('芯之力模型配置已保存并即时生效');
      return;
    }
    const payload: ModelConfigPayload = {
      chat: form.value.chat,
      video: form.value.video,
      image: form.value.image,
      analysis: {
        apiBase: '',
        apiKey: '',
        groupId: '',
        model: form.value.analysis.model,
      },
      admin: {
        apiBase: form.value.admin.apiBase,
        apiKey: form.value.admin.apiKey,
        groupId: form.value.admin.groupId,
        model: form.value.admin.model,
        provider: form.value.admin.provider,
        timeoutSeconds: Number(form.value.admin.timeoutSeconds || 30),
      },
      dailyQuiz: {
        apiBase: form.value.dailyQuiz.apiBase,
        apiKey: form.value.dailyQuiz.apiKey,
        groupId: form.value.dailyQuiz.groupId,
        model: form.value.dailyQuiz.model,
        provider: form.value.dailyQuiz.provider,
        timeoutSeconds: Number(form.value.dailyQuiz.timeoutSeconds || 30),
      },
      assist: form.value.assist,
    };
    const saved = await updateModelConfigApi(payload);
    // 保存后清空密钥输入，刷新「已配置」状态
    form.value.chat.apiKey = '';
    form.value.video.apiKey = '';
    form.value.image.apiKey = '';
    form.value.analysis.apiKey = '';
    form.value.admin.apiKey = '';
    form.value.dailyQuiz.apiKey = '';
    chatKeySet.value = saved.chat?.apiKeySet ?? false;
    videoKeySet.value = saved.video?.apiKeySet ?? false;
    imageKeySet.value = saved.image?.apiKeySet ?? false;
    analysisKeySet.value = saved.analysis?.apiKeySet ?? false;
    adminKeySet.value = saved.admin?.apiKeySet ?? false;
    dailyQuizKeySet.value = saved.dailyQuiz?.apiKeySet ?? false;
    message.success('模型配置已保存并即时生效');
  } finally {
    saving.value = false;
  }
}

function applyXinzhiliConfig(data: XinzhiliModelConfigView) {
  const provider = normalizeTtsProvider(data.tts?.provider);
  form.value.tts = {
    apiKey: '',
    endpoint:
      data.tts?.endpoint ||
      (provider === 'bailian' ? aliyunBailianTtsPreset.endpoint : ''),
    format: data.tts?.format || 'mp3',
    groupId: provider === 'bailian' ? '' : data.tts?.groupId ?? '',
    model:
      data.tts?.model ||
      (provider === 'bailian' ? aliyunBailianTtsPreset.model : 'speech-02-hd'),
    provider,
    voice: data.tts?.voice ?? '',
  };
  applyTtsProviderPreset(form.value.tts.provider);
  ttsKeySet.value = data.tts?.apiKeySet ?? false;
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

function handleTtsVoiceOptionChange(optionId?: any) {
  const selected = Array.isArray(optionId) ? optionId[0] : optionId;
  const selectedId =
    typeof selected === 'number' || typeof selected === 'string'
      ? String(selected)
      : '';
  selectedTtsVoiceOptionId.value = selectedId;
  const option = ttsVoiceOptions.value.find((item) => item.id === selectedId);
  if (!option) {
    return;
  }
  if (voiceOptionProvider(option) === 'bailian') {
    form.value.tts.provider = 'bailian';
    applyTtsProviderPreset('bailian', true);
  } else if (voiceOptionProvider(option) === 'minimax') {
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

function normalizeTtsProvider(provider?: string) {
  provider = (provider || '').trim();
  return provider === 'openai-compatible' ? 'bailian' : provider || 'bailian';
}

function applyTtsProviderPreset(provider = form.value.tts.provider, force = false) {
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

const testing = ref(false);
const pingResult = ref<ChatPingResult | null>(null);

async function testChat() {
  testing.value = true;
  pingResult.value = null;
  try {
    // 携带当前表单的对话配置（密钥留空则回退到已保存/环境基线）
    pingResult.value = await testChatModelApi(form.value.chat);
  } finally {
    testing.value = false;
  }
}
</script>

<template>
  <EditorShell
    :description="pageDescription"
    :loading="loading"
    :saving="saving"
    :title="pageTitle"
    @save="save"
  >
    <Form v-if="form" layout="vertical">
      <template v-if="isXinzhiliModelOnly">
        <Divider orientation="left">AI 语音合成 / TTS 配置</Divider>
        <Alert
          class="mb-4"
          type="info"
          show-icon
          message="可直接复用声音管理里的已克隆音色，也可以手动填写平台音色 ID"
          description="选择已有音色后会按当前 TTS provider 只展示同平台 ready 音色，并把最终 voiceId 写入配置；阿里百炼音色会自动填充百炼 endpoint 与模型。"
        />
        <Row :gutter="24">
          <Col :md="12" :xs="24">
            <Form.Item label="TTS provider">
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
                placeholder="可直接复用声音管理里的同平台已克隆音色"
                show-search
                @change="handleTtsVoiceOptionChange"
              />
              <span
                v-if="ttsVoiceOptionsError"
                class="mt-1 block text-xs text-red-500"
              >
                音色选项读取失败，可手动填写音色 ID
              </span>
            </Form.Item>
            <Form.Item label="手动音色 ID">
              <Input
                v-model:value="form.tts.voice"
                :placeholder="
                  form.tts.provider === 'bailian'
                    ? '阿里百炼复刻音色 ID'
                    : '可直接复用声音管理里的已克隆音色，也可以手动填写平台音色 ID'
                "
              />
            </Form.Item>
          </Col>
          <Col :md="12" :xs="24">
            <Form.Item label="接口地址 (Endpoint)">
              <Input
                v-model:value="form.tts.endpoint"
                :placeholder="
                  form.tts.provider === 'bailian'
                    ? 'https://dashscope.aliyuncs.com/api/v1'
                    : 'MiniMax API 地址，留空则使用服务端默认值'
                "
              />
            </Form.Item>
            <Form.Item v-if="form.tts.provider === 'minimax'" label="Group ID">
              <Input
                v-model:value="form.tts.groupId"
                placeholder="MiniMax Group ID，留空则使用服务端默认值"
              />
            </Form.Item>
            <Form.Item label="模型名 (Model)">
              <Input
                v-model:value="form.tts.model"
                :placeholder="
                  form.tts.provider === 'bailian'
                    ? 'MiniMax/speech-2.8-turbo'
                    : 'speech-02-hd'
                "
              />
            </Form.Item>
            <Form.Item label="音频格式">
              <Select
                v-model:value="form.tts.format"
                :options="ttsFormatOptions"
                placeholder="请选择音频格式"
              />
            </Form.Item>
            <Form.Item label="新密钥 (API Key)">
              <Input.Password
                v-model:value="form.tts.apiKey"
                :placeholder="
                  ttsKeySet ? '已配置，留空表示不修改' : '请输入 TTS API Key'
                "
                autocomplete="new-password"
              />
            </Form.Item>
          </Col>
        </Row>
      </template>

      <template v-else-if="!isAdminModelOnly">
        <Divider orientation="left">对话模型（手机端聊天窗口作答所用）</Divider>
        <Row :gutter="24">
          <Col :md="12" :xs="24">
            <Form.Item label="协议">
              <Select
                v-model:value="form.chat.provider"
                :options="chatProviderOptions"
               placeholder="请选择协议" />
            </Form.Item>
            <Form.Item label="接口地址 (API Base)">
              <Input
                v-model:value="form.chat.apiBase"
                placeholder="https://coding-play.codes"
              />
              <span class="mt-1 block text-xs text-gray-400">
                {{ chatEndpointHint }}
              </span>
            </Form.Item>
          </Col>
          <Col :md="12" :xs="24">
            <Form.Item label="模型名 (Model)">
              <Input
                v-model:value="form.chat.model"
                placeholder="如 gpt-5.6-sol / claude-sonnet-4-5"
              />
            </Form.Item>
            <Form.Item label="密钥 (API Key)">
              <Input.Password
                v-model:value="form.chat.apiKey"
                :placeholder="
                  chatKeySet ? '已配置，留空表示不修改' : '请输入 API Key'
                "
                autocomplete="new-password"
              />
            </Form.Item>
            <Form.Item label="连通性测试">
              <Button :loading="testing" @click="testChat">
                测试连通性
              </Button>
              <span class="ml-2 text-xs text-gray-400">
                按当前协议对中转站做一次最小请求
              </span>
            </Form.Item>
          </Col>
        </Row>

        <Alert
          v-if="pingResult"
          class="mt-2"
          :type="pingResult.ok ? 'success' : 'error'"
          show-icon
          :message="pingResult.ok ? '对话模型连通正常' : '对话模型连通失败'"
          :description="`${pingResult.message}${
            pingResult.ok ? `（耗时 ${pingResult.latencyMs}ms）` : ''
          }`"
        />

        <Divider orientation="left">视频模型</Divider>
        <Row :gutter="24">
          <Col :md="12" :xs="24">
            <Form.Item label="接口地址 (API Base)">
              <Input
                v-model:value="form.video.apiBase"
                placeholder="留空则使用环境变量默认值"
              />
            </Form.Item>
            <Form.Item label="模型名 (Model)">
              <Input
                v-model:value="form.video.model"
                placeholder="如 video-ds-2.0-fast"
              />
            </Form.Item>
          </Col>
          <Col :md="12" :xs="24">
            <Form.Item label="密钥 (API Key)">
              <Input.Password
                v-model:value="form.video.apiKey"
                :placeholder="
                  videoKeySet ? '已配置，留空表示不修改' : '请输入 API Key'
                "
                autocomplete="new-password"
              />
            </Form.Item>
          </Col>
        </Row>

        <Divider orientation="left">文生图模型（gpt-image-2 中转）</Divider>
        <Row :gutter="24">
          <Col :md="12" :xs="24">
            <Form.Item label="接口地址 (API Base)">
              <Input
                v-model:value="form.image.apiBase"
                placeholder="留空则使用环境变量默认值"
              />
            </Form.Item>
            <Form.Item label="模型名 (Model)">
              <Input
                v-model:value="form.image.model"
                placeholder="如 gpt-image-2"
              />
            </Form.Item>
          </Col>
          <Col :md="12" :xs="24">
            <Form.Item label="密钥 (API Key)">
              <Input.Password
                v-model:value="form.image.apiKey"
                :placeholder="
                  imageKeySet ? '已配置，留空表示不修改' : '请输入 API Key'
                "
                autocomplete="new-password"
              />
            </Form.Item>
          </Col>
        </Row>

        <Divider orientation="left">视频分析模型</Divider>
        <Alert
          class="mb-4"
          type="info"
          show-icon
          message="视频分析复用语音生成 MiniMax 配置"
          description="接口地址、Group ID 与 API Key 均来自服务端 MINIMAX_* 环境配置；这里只配置用于读取视频的多模态模型名。"
        />
        <Row :gutter="24">
          <Col :md="12" :xs="24">
            <Form.Item label="接口地址 (API Base，来自语音生成)">
              <Input
                v-model:value="form.analysis.apiBase"
                disabled
                placeholder="服务端 MINIMAX_API_BASE"
              />
            </Form.Item>
            <Form.Item label="模型名 (Model)">
              <Input
                v-model:value="form.analysis.model"
                placeholder="MiniMax-M3"
              />
            </Form.Item>
          </Col>
          <Col :md="12" :xs="24">
            <Form.Item label="Group ID（来自语音生成）">
              <Input
                v-model:value="form.analysis.groupId"
                disabled
                placeholder="服务端 MINIMAX_GROUP_ID"
              />
            </Form.Item>
            <Form.Item label="密钥状态（来自语音生成）">
              <Input.Password
                :value="analysisKeySet ? '已配置' : '未配置'"
                disabled
              />
            </Form.Item>
          </Col>
        </Row>
      </template>

      <template v-if="!isXinzhiliModelOnly">
        <Divider orientation="left">管理端大模型（后台每日题生成）</Divider>
      <Alert
        class="mb-4"
        type="info"
        show-icon
        message="用于后台自动生成每日 5 道画像校准题"
        description="服务端会读取公共知识库，再用这里配置的大模型生成题目；App 端仍只请求服务端，不会拿到密钥。"
      />
      <Row :gutter="24">
        <Col :md="12" :xs="24">
          <Form.Item label="协议">
            <Select
              v-model:value="form.admin.provider"
              :options="providerOptions"
             placeholder="请选择协议" />
          </Form.Item>
          <Form.Item label="接口地址 (API Base)">
            <Input
              v-model:value="form.admin.apiBase"
              placeholder="OpenAI 协议如 https://api.openai.com；Anthropic 如 https://api.anthropic.com"
            />
          </Form.Item>
          <Form.Item label="模型名 (Model)">
            <Input
              v-model:value="form.admin.model"
              placeholder="如 gpt-4.1-mini / claude-sonnet-4-5"
            />
          </Form.Item>
        </Col>
        <Col :md="12" :xs="24">
          <Form.Item label="Group ID（MiniMax 可选）">
            <Input
              v-model:value="form.admin.groupId"
              placeholder="仅 MiniMax 协议需要时填写"
            />
          </Form.Item>
          <Form.Item label="密钥状态">
            <Input :value="adminKeySet ? '已配置' : '未配置'" disabled />
          </Form.Item>
          <Form.Item label="新密钥 (API Key)">
            <Input.Password
              v-model:value="form.admin.apiKey"
              :placeholder="
                adminKeySet ? '已配置，留空表示不修改' : '请输入 API Key'
              "
              autocomplete="new-password"
            />
          </Form.Item>
          <Form.Item label="超时时间（秒）">
            <Input
              v-model:value="form.admin.timeoutSeconds"
              type="number"
              placeholder="默认 30"
            />
          </Form.Item>
        </Col>
      </Row>

      <Divider orientation="left">每日题生成模型（可选覆盖）</Divider>
      <Alert
        class="mb-4"
        type="info"
        show-icon
        message="默认继承管理端大模型"
        description="如果这里填写了协议、接口地址、模型名或密钥，则每日 5 道画像校准题会优先使用这里的配置；留空则继承上方管理端大模型。"
      />
      <Row :gutter="24">
        <Col :md="12" :xs="24">
          <Form.Item label="协议">
            <Select
              v-model:value="form.dailyQuiz.provider"
              :options="dailyQuizProviderOptions"
             placeholder="请选择协议" />
          </Form.Item>
          <Form.Item label="接口地址 (API Base)">
            <Input
              v-model:value="form.dailyQuiz.apiBase"
              placeholder="留空继承管理端大模型"
            />
          </Form.Item>
          <Form.Item label="模型名 (Model)">
            <Input
              v-model:value="form.dailyQuiz.model"
              placeholder="留空继承管理端大模型"
            />
          </Form.Item>
        </Col>
        <Col :md="12" :xs="24">
          <Form.Item label="Group ID（MiniMax 可选）">
            <Input
              v-model:value="form.dailyQuiz.groupId"
              placeholder="留空继承管理端大模型"
            />
          </Form.Item>
          <Form.Item label="密钥状态">
            <Input
              :value="dailyQuizKeySet ? '已单独配置' : '未单独配置'"
              disabled
            />
          </Form.Item>
          <Form.Item label="新密钥 (API Key)">
            <Input.Password
              v-model:value="form.dailyQuiz.apiKey"
              :placeholder="
                dailyQuizKeySet ? '已单独配置，留空表示不修改' : '留空继承管理端密钥'
              "
              autocomplete="new-password"
            />
          </Form.Item>
          <Form.Item label="超时时间（秒）">
            <Input
              v-model:value="form.dailyQuiz.timeoutSeconds"
              type="number"
              placeholder="默认继承管理端大模型"
            />
          </Form.Item>
        </Col>
      </Row>
      </template>

      <template v-if="!isAdminModelOnly && !isXinzhiliModelOnly">
        <Divider orientation="left">智能辅助作答</Divider>
        <Row :gutter="24">
          <Col :xs="24">
            <Form.Item label="开启智能辅助">
              <Switch v-model:checked="form.assist.enabled" />
              <span class="ml-3 text-xs text-gray-400">
                开启后，问答将结合资料库与专属模型作答；命中资料时结合资料回答，未命中时也能给出回答。关闭后仅返回固定文案。
              </span>
            </Form.Item>
            <Form.Item label="系统提示词 (人设与作答风格)">
              <Input.TextArea
                v-model:value="form.assist.systemPrompt"
                :auto-size="{ minRows: 4, maxRows: 12 }"
                placeholder="留空则使用服务端默认提示词。可在此设定专属模型的人设、语气与作答边界。"
              />
              <span class="mt-1 block text-xs text-gray-400">
                用于约束作答口吻与范围，对所有用户的问答生效；不影响资料库内容。
              </span>
            </Form.Item>
          </Col>
        </Row>
      </template>

      <Alert
        type="info"
        show-icon
        message="安全提示"
        description="出于安全考虑，已保存的密钥不会回显。如需更换密钥请重新填写，留空则保留原密钥。所有字段留空时将回退到服务端环境变量配置。"
      />
    </Form>
  </EditorShell>
</template>

<style scoped></style>
