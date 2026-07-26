<script setup lang="ts">
import type {
  ChatPingResult,
  ModelConfigPayload,
  ModelConfigUpdatePayload,
  ModelConfigView,
} from '#/api';

import { computed, onMounted, ref } from 'vue';
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
  testChatModelApi,
  updateModelConfigApi,
} from '#/api';

import EditorShell from '../site-config/components/editor-shell.vue';

const loading = ref(true);
const saving = ref(false);
const route = useRoute();
const isAdminModelOnly = computed(() =>
  route.path.includes('/settings/admin-model'),
);
const pageTitle = computed(() =>
  isAdminModelOnly.value ? '管理端大模型配置' : '模型配置',
);
const pageDescription = computed(() =>
  isAdminModelOnly.value
    ? '单独配置后台运营任务所用的大模型，包括每日 5 道画像校准题生成。'
    : '配置对话、视频生成、文生图与视频分析模型。视频分析固定复用语音生成的 MiniMax 地址与密钥，默认使用 MiniMax-M3 多模态模型。',
);

// apiKey 留空表示不修改；apiKeySet 用于提示是否已配置过密钥
const form = ref<ModelConfigPayload>({
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
  xinzhiliVoice: {
    enabled: false,
    asr: {
      provider: 'openai-compatible',
      apiBase: '',
      apiKey: '',
      model: '',
      language: 'zh',
      timeoutSeconds: 30,
    },
    tts: {
      provider: 'openai-compatible',
      apiBase: '',
      apiKey: '',
      model: '',
      voice: '',
      speed: 1,
      responseFormat: 'mp3',
      timeoutSeconds: 45,
    },
    interaction: {
      endSilenceMs: 700,
      minSpeechMs: 300,
      maxTurnSeconds: 60,
      autoRelisten: true,
      tapToInterrupt: true,
    },
    systemPrompt: '',
  },
  assist: { enabled: true, systemPrompt: '' },
});
const chatKeySet = ref(false);
const loadedChatProvider = ref('');
const videoKeySet = ref(false);
const imageKeySet = ref(false);
const analysisKeySet = ref(false);
const adminKeySet = ref(false);
const dailyQuizKeySet = ref(false);
const xinzhiliAsrKeySet = ref(false);
const xinzhiliTtsKeySet = ref(false);

const chatProviderOptions = [
  { label: 'OpenAI 协议', value: 'openai-compatible' },
  { label: 'Anthropic 协议', value: 'anthropic-compatible' },
];
const chatProviderValues = new Set(
  chatProviderOptions.map((option) => option.value),
);
const chatKeyReusable = computed(
  () =>
    chatKeySet.value &&
    chatProviderValues.has(form.value.chat.provider) &&
    form.value.chat.provider === loadedChatProvider.value,
);
const adminProviderOptions = [
  ...chatProviderOptions,
  { label: 'MiniMax 协议', value: 'minimax' },
];
const chatProviderOptions = [
  { label: 'OpenAI 协议', value: 'openai-compatible' },
  { label: 'Anthropic 协议', value: 'anthropic-compatible' },
];
const dailyQuizProviderOptions = [
  { label: '继承管理端', value: '' },
  ...adminProviderOptions,
];
const chatTimeoutError = '对话模型超时时间必须是大于 0 的整数';

function parseChatTimeout(value: unknown) {
  if (value === '') return null;
  const timeout = Number(value);
  return Number.isInteger(timeout) && timeout > 0 ? timeout : null;
}

const chatTimeoutRules = [
  { message: '请输入超时时间（秒）', required: true },
  {
    validator: (_rule: unknown, value: unknown) => {
      return parseChatTimeout(value) !== null
        ? Promise.resolve()
        : Promise.reject(new Error(chatTimeoutError));
    },
  },
];
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

async function load() {
  loading.value = true;
  try {
    const data: ModelConfigView = await getModelConfigApi();
    if (data) {
      const nextForm: ModelConfigPayload = {
        chat: {
          provider: data.chat?.provider ?? 'openai-compatible',
          apiBase: data.chat?.apiBase || 'https://coding-play.codes',
          apiKey: '',
          model: data.chat?.model ?? '',
          provider: data.chat?.provider ?? '',
          timeoutSeconds: data.chat?.timeoutSeconds ?? 30,
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
        xinzhiliVoice: {
          enabled: data.xinzhiliVoice?.enabled ?? false,
          asr: {
            provider: 'openai-compatible',
            apiBase: data.xinzhiliVoice?.asr.apiBase ?? '',
            apiKey: '',
            model: data.xinzhiliVoice?.asr.model ?? '',
            language: data.xinzhiliVoice?.asr.language ?? 'zh',
            timeoutSeconds: data.xinzhiliVoice?.asr.timeoutSeconds ?? 30,
          },
          tts: {
            provider: 'openai-compatible',
            apiBase: data.xinzhiliVoice?.tts.apiBase ?? '',
            apiKey: '',
            model: data.xinzhiliVoice?.tts.model ?? '',
            voice: data.xinzhiliVoice?.tts.voice ?? '',
            speed: data.xinzhiliVoice?.tts.speed ?? 1,
            responseFormat: data.xinzhiliVoice?.tts.responseFormat ?? 'mp3',
            timeoutSeconds: data.xinzhiliVoice?.tts.timeoutSeconds ?? 45,
          },
          interaction: {
            endSilenceMs:
              data.xinzhiliVoice?.interaction.endSilenceMs ?? 700,
            minSpeechMs: data.xinzhiliVoice?.interaction.minSpeechMs ?? 300,
            maxTurnSeconds:
              data.xinzhiliVoice?.interaction.maxTurnSeconds ?? 60,
            autoRelisten:
              data.xinzhiliVoice?.interaction.autoRelisten ?? true,
            tapToInterrupt:
              data.xinzhiliVoice?.interaction.tapToInterrupt ?? true,
          },
          systemPrompt: data.xinzhiliVoice?.systemPrompt ?? '',
        },
        assist: {
          enabled: data.assist?.enabled ?? true,
          systemPrompt: data.assist?.systemPrompt ?? '',
        },
      };
      form.value = nextForm;
      loadedChatProvider.value = data.chat?.provider ?? '';
      chatKeySet.value = data.chat?.apiKeySet ?? false;
      videoKeySet.value = data.video?.apiKeySet ?? false;
      imageKeySet.value = data.image?.apiKeySet ?? false;
      analysisKeySet.value = data.analysis?.apiKeySet ?? false;
      adminKeySet.value = data.admin?.apiKeySet ?? false;
      dailyQuizKeySet.value = data.dailyQuiz?.apiKeySet ?? false;
      xinzhiliAsrKeySet.value =
        data.xinzhiliVoice?.asr.apiKeySet ?? false;
      xinzhiliTtsKeySet.value =
        data.xinzhiliVoice?.tts.apiKeySet ?? false;
    }
  } finally {
    loading.value = false;
  }
}

function currentChatPayload(): ModelConfigPayload['chat'] | null {
  const provider = form.value.chat.provider;
  if (!chatProviderValues.has(provider)) {
    message.error('请选择 OpenAI 或 Anthropic 协议');
    return null;
  }
  if (!form.value.chat.apiKey.trim() && !chatKeyReusable.value) {
    message.error(
      provider !== loadedChatProvider.value
        ? '切换协议后请重新填写 API Key'
        : '请填写对话模型 API Key',
    );
    return null;
  }
  const timeoutSeconds = parseChatTimeout(form.value.chat.timeoutSeconds);
  if (timeoutSeconds === null) {
    message.error(chatTimeoutError);
    return null;
  }
  return {
    apiBase: form.value.chat.apiBase,
    apiKey: form.value.chat.apiKey,
    model: form.value.chat.model,
    provider,
    timeoutSeconds,
  };
}

async function save() {
  saving.value = true;
  try {
    const admin = {
      apiBase: form.value.admin.apiBase,
      apiKey: form.value.admin.apiKey,
      groupId: form.value.admin.groupId,
      model: form.value.admin.model,
      provider: form.value.admin.provider,
      timeoutSeconds: Number(form.value.admin.timeoutSeconds || 30),
    };
    const dailyQuiz = {
      apiBase: form.value.dailyQuiz.apiBase,
      apiKey: form.value.dailyQuiz.apiKey,
      groupId: form.value.dailyQuiz.groupId,
      model: form.value.dailyQuiz.model,
      provider: form.value.dailyQuiz.provider,
      timeoutSeconds: Number(form.value.dailyQuiz.timeoutSeconds || 30),
    };
    let chat: ModelConfigPayload['chat'] | null = null;
    let payload: ModelConfigUpdatePayload;
    if (isAdminModelOnly.value) {
      payload = { admin, dailyQuiz };
    } else {
      chat = currentChatPayload();
      if (!chat) return;
      payload = {
        ...form.value,
        chat,
        analysis: {
          apiBase: '',
          apiKey: '',
          groupId: '',
          model: form.value.analysis.model,
        },
        admin,
        dailyQuiz,
      };
    }
    const saved = await updateModelConfigApi(payload);
    // 保存后清空密钥输入，刷新「已配置」状态
    form.value.admin.apiKey = '';
    form.value.dailyQuiz.apiKey = '';
    adminKeySet.value = saved.admin?.apiKeySet ?? false;
    dailyQuizKeySet.value = saved.dailyQuiz?.apiKeySet ?? false;
    if (chat) {
      form.value.chat.apiKey = '';
      form.value.video.apiKey = '';
      form.value.image.apiKey = '';
      form.value.analysis.apiKey = '';
      loadedChatProvider.value = saved.chat?.provider ?? chat.provider;
      chatKeySet.value = saved.chat?.apiKeySet ?? false;
      videoKeySet.value = saved.video?.apiKeySet ?? false;
      imageKeySet.value = saved.image?.apiKeySet ?? false;
      analysisKeySet.value = saved.analysis?.apiKeySet ?? false;
    }
    message.success('模型配置已保存并即时生效');
  } finally {
    saving.value = false;
  }
}

const testing = ref(false);
const pingResult = ref<ChatPingResult | null>(null);

async function testChat() {
  testing.value = true;
  pingResult.value = null;
  try {
    // 携带当前表单的对话配置（密钥留空则回退到已保存/环境基线）
    const chat = currentChatPayload();
    if (!chat) return;
    pingResult.value = await testChatModelApi(chat);
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
      <template v-if="!isAdminModelOnly">
        <Divider orientation="left">对话模型（手机端聊天窗口作答所用）</Divider>
        <Row :gutter="24">
          <Col :md="12" :xs="24">
            <Form.Item label="协议">
              <Select
                v-model:value="form.chat.provider"
                :options="chatProviderOptions"
              />
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
        </section>

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
              :options="adminProviderOptions"
            />
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
            />
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
                dailyQuizKeySet
                  ? '已单独配置，留空表示不修改'
                  : '留空继承管理端密钥'
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

      <template v-if="!isAdminModelOnly">
        <Divider orientation="left">芯之力语音配置</Divider>
        <Alert
          class="mb-4"
          type="info"
          show-icon
          message="电话式低延迟语音链路"
          description="ASR 与 TTS 使用 OpenAI 兼容协议；中间回答继续使用页面顶部配置的对话模型。任一语音模型未配置时，App 会明确提示先完成配置。"
        />
        <Form.Item label="启用芯之力语音">
          <Switch v-model:checked="form.xinzhiliVoice.enabled" />
        </Form.Item>
        <Row :gutter="24">
          <Col :md="12" :xs="24">
            <Divider dashed orientation="left">ASR 语音识别</Divider>
            <Form.Item label="协议">
              <Input value="OpenAI 兼容协议" disabled />
            </Form.Item>
            <Form.Item label="接口地址 (API Base)">
              <Input
                v-model:value="form.xinzhiliVoice.asr.apiBase"
                placeholder="如 https://api.openai.com/v1"
              />
            </Form.Item>
            <Form.Item label="模型名 (Model)">
              <Input
                v-model:value="form.xinzhiliVoice.asr.model"
                placeholder="如 whisper-1 / SenseVoice"
              />
            </Form.Item>
            <Form.Item label="语言">
              <Input
                v-model:value="form.xinzhiliVoice.asr.language"
                placeholder="zh"
              />
            </Form.Item>
            <Form.Item label="新密钥 (API Key)">
              <Input.Password
                v-model:value="form.xinzhiliVoice.asr.apiKey"
                :placeholder="
                  xinzhiliAsrKeySet
                    ? '已配置，留空表示不修改'
                    : '请输入 ASR API Key'
                "
                autocomplete="new-password"
              />
            </Form.Item>
            <Form.Item label="超时时间（秒）">
              <Input
                v-model:value="form.xinzhiliVoice.asr.timeoutSeconds"
                type="number"
              />
            </Form.Item>
          </Col>
          <Col :md="12" :xs="24">
            <Divider dashed orientation="left">TTS 语音回复</Divider>
            <Form.Item label="协议">
              <Input value="OpenAI 兼容协议" disabled />
            </Form.Item>
            <Form.Item label="接口地址 (API Base)">
              <Input
                v-model:value="form.xinzhiliVoice.tts.apiBase"
                placeholder="如 https://api.openai.com/v1"
              />
            </Form.Item>
            <Form.Item label="模型名 (Model)">
              <Input
                v-model:value="form.xinzhiliVoice.tts.model"
                placeholder="如 tts-1"
              />
            </Form.Item>
            <Form.Item label="音色 (Voice)">
              <Input
                v-model:value="form.xinzhiliVoice.tts.voice"
                placeholder="如 alloy / nova"
              />
            </Form.Item>
            <Form.Item label="新密钥 (API Key)">
              <Input.Password
                v-model:value="form.xinzhiliVoice.tts.apiKey"
                :placeholder="
                  xinzhiliTtsKeySet
                    ? '已配置，留空表示不修改'
                    : '请输入 TTS API Key'
                "
                autocomplete="new-password"
              />
            </Form.Item>
            <Row :gutter="12">
              <Col :span="8">
                <Form.Item label="语速">
                  <Input
                    v-model:value="form.xinzhiliVoice.tts.speed"
                    type="number"
                  />
                </Form.Item>
              </Col>
              <Col :span="8">
                <Form.Item label="格式">
                  <Input
                    v-model:value="form.xinzhiliVoice.tts.responseFormat"
                    placeholder="mp3"
                  />
                </Form.Item>
              </Col>
              <Col :span="8">
                <Form.Item label="超时（秒）">
                  <Input
                    v-model:value="form.xinzhiliVoice.tts.timeoutSeconds"
                    type="number"
                  />
                </Form.Item>
              </Col>
            </Row>
          </Col>
        </Row>
        <Divider dashed orientation="left">交互与断句</Divider>
        <Row :gutter="16">
          <Col :md="8" :xs="24">
            <Form.Item label="结束静音（毫秒）">
              <Input
                v-model:value="form.xinzhiliVoice.interaction.endSilenceMs"
                type="number"
              />
            </Form.Item>
          </Col>
          <Col :md="8" :xs="24">
            <Form.Item label="最短有效声音（毫秒）">
              <Input
                v-model:value="form.xinzhiliVoice.interaction.minSpeechMs"
                type="number"
              />
            </Form.Item>
          </Col>
          <Col :md="8" :xs="24">
            <Form.Item label="单轮最长（秒）">
              <Input
                v-model:value="form.xinzhiliVoice.interaction.maxTurnSeconds"
                type="number"
              />
            </Form.Item>
          </Col>
        </Row>
        <Row :gutter="24">
          <Col :md="12" :xs="24">
            <Form.Item label="语音播完后自动复听">
              <Switch
                v-model:checked="form.xinzhiliVoice.interaction.autoRelisten"
              />
            </Form.Item>
          </Col>
          <Col :md="12" :xs="24">
            <Form.Item label="点击球体打断 AI">
              <Switch
                v-model:checked="form.xinzhiliVoice.interaction.tapToInterrupt"
              />
            </Form.Item>
          </Col>
        </Row>
        <Form.Item label="芯之力专属系统提示词">
          <Input.TextArea
            v-model:value="form.xinzhiliVoice.systemPrompt"
            :auto-size="{ minRows: 4, maxRows: 10 }"
            placeholder="约束电话式回复的语气、长度和九型知识使用方式。"
          />
        </Form.Item>

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
