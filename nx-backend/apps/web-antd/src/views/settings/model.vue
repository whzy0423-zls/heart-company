<script setup lang="ts">
import type {
  ChatPingResult,
  ModelConfigPayload,
  ModelConfigView,
} from '#/api';

import { onMounted, ref } from 'vue';

import {
  Alert,
  Button,
  Col,
  Divider,
  Form,
  Input,
  message,
  Row,
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

// apiKey 留空表示不修改；apiKeySet 用于提示是否已配置过密钥
const form = ref<ModelConfigPayload>({
  chat: { apiBase: '', apiKey: '', groupId: '', model: '' },
  video: { apiBase: '', apiKey: '', model: '' },
  image: { apiBase: '', apiKey: '', model: '' },
  analysis: { apiBase: '', apiKey: '', groupId: '', model: '' },
  assist: { enabled: true, systemPrompt: '' },
});
const chatKeySet = ref(false);
const videoKeySet = ref(false);
const imageKeySet = ref(false);
const analysisKeySet = ref(false);

onMounted(load);

async function load() {
  loading.value = true;
  try {
    const data: ModelConfigView = await getModelConfigApi();
    if (data) {
      const nextForm: ModelConfigPayload = {
        chat: {
          apiBase: data.chat?.apiBase ?? '',
          apiKey: '',
          groupId: data.chat?.groupId ?? '',
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
    }
  } finally {
    loading.value = false;
  }
}

async function save() {
  saving.value = true;
  try {
    const saved = await updateModelConfigApi(form.value);
    // 保存后清空密钥输入，刷新「已配置」状态
    form.value.chat.apiKey = '';
    form.value.video.apiKey = '';
    form.value.image.apiKey = '';
    form.value.analysis.apiKey = '';
    chatKeySet.value = saved.chat?.apiKeySet ?? false;
    videoKeySet.value = saved.video?.apiKeySet ?? false;
    imageKeySet.value = saved.image?.apiKeySet ?? false;
    analysisKeySet.value = saved.analysis?.apiKeySet ?? false;
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
    pingResult.value = await testChatModelApi(form.value.chat);
  } finally {
    testing.value = false;
  }
}
</script>

<template>
  <EditorShell
    description="配置对话、视频生成、文生图与视频分析模型。视频分析固定复用语音生成的 MiniMax 地址与密钥，默认使用 MiniMax-M3 多模态模型。"
    :loading="loading"
    :saving="saving"
    title="模型配对"
    @save="save"
  >
    <Form v-if="form" layout="vertical">
      <Divider orientation="left">对话模型（手机端聊天窗口作答所用）</Divider>
      <Row :gutter="24">
        <Col :md="12" :xs="24">
          <Form.Item label="接口地址 (API Base)">
            <Input
              v-model:value="form.chat.apiBase"
              placeholder="留空则使用环境变量默认值"
            />
          </Form.Item>
          <Form.Item label="模型名 (Model)">
            <Input
              v-model:value="form.chat.model"
              placeholder="如 abab6.5s-chat"
            />
          </Form.Item>
        </Col>
        <Col :md="12" :xs="24">
          <Form.Item label="Group ID">
            <Input
              v-model:value="form.chat.groupId"
              placeholder="对话模型网关分配的 Group ID"
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
            <Button :loading="testing" @click="testChat"> 测试连通性 </Button>
            <span class="ml-2 text-xs text-gray-400">
              对网关做一次轻量探活，不消耗生成额度
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
