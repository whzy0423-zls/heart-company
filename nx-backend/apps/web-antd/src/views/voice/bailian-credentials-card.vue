<script setup lang="ts">
import type { BailianCredentialSource, BailianCredentialView } from '#/api';

import { computed, onMounted, ref } from 'vue';

import {
  Alert,
  Button,
  Card,
  Input,
  message,
  Modal,
  Space,
} from 'ant-design-vue';

import { getBailianCredentialsApi, updateBailianCredentialsApi } from '#/api';

export interface BailianCredentialsCardStatus {
  apiKeySet: boolean;
  error: null | string;
  loading: boolean;
  source: BailianCredentialSource;
  version: number;
}

const emit = defineEmits<{
  'status-change': [status: BailianCredentialsCardStatus];
}>();

const credential = ref<BailianCredentialView>({
  apiKeySet: false,
  apiKeySuffix: '',
  source: 'none',
  version: 0,
});
const apiKey = ref('');
const loading = ref(true);
const saving = ref(false);
const error = ref<null | string>(null);

const sourceLabel = computed(() => {
  if (credential.value.source === 'shared') return '公共配置';
  if (credential.value.source === 'legacy-asr') return '来源旧 ASR 配置';
  if (credential.value.source === 'legacy-tts') return '来源旧 TTS 配置';
  return '未配置';
});

const statusLabel = computed(() => {
  if (credential.value.apiKeySet) {
    return credential.value.apiKeySuffix
      ? `已配置（${credential.value.apiKeySuffix}）`
      : '已配置';
  }
  return '未配置';
});

function emitStatus() {
  emit('status-change', {
    apiKeySet: credential.value.apiKeySet,
    error: error.value,
    loading: loading.value,
    source: credential.value.source,
    version: credential.value.version,
  });
}

function applyCredential(next: BailianCredentialView) {
  credential.value = next;
  error.value = null;
  emitStatus();
}

function isConflict(errorValue: unknown) {
  const status =
    (errorValue as { response?: { status?: number }; status?: number })
      ?.response?.status ?? (errorValue as { status?: number })?.status;
  return status === 409;
}

async function load() {
  loading.value = true;
  error.value = null;
  emitStatus();
  try {
    applyCredential(await getBailianCredentialsApi());
  } catch {
    error.value = '百炼凭证读取失败，请重新加载';
    emitStatus();
  } finally {
    loading.value = false;
    emitStatus();
  }
}

async function save(clearApiKey = false) {
  if (loading.value || error.value) return;

  saving.value = true;
  try {
    const updated = await updateBailianCredentialsApi({
      apiKey: clearApiKey ? '' : apiKey.value,
      clearApiKey,
      expectedVersion: credential.value.version,
    });
    apiKey.value = '';
    applyCredential(updated);
    message.success(
      clearApiKey ? '百炼公共 API Key 已清空' : '百炼公共 API Key 已保存',
    );
  } catch (errorValue) {
    error.value = isConflict(errorValue)
      ? '配置已更新，请重新加载后再保存'
      : '百炼凭证保存失败，请稍后重试';
    emitStatus();
  } finally {
    saving.value = false;
    emitStatus();
  }
}

function confirmClear() {
  Modal.confirm({
    content:
      '清空后将停止使用旧配置回退，Paraformer、Qwen 克隆音色和 Qwen TTS 都将不可用。',
    okButtonProps: { danger: true },
    okText: '确认清空',
    onOk: () => save(true),
    title: '清空百炼公共 API Key',
  });
}

onMounted(load);
</script>

<template>
  <Card :bordered="false" class="bailian-credentials-card">
    <div class="card-title">百炼公共 API Key</div>
    <div class="card-description">
      同一个 Key 用于 Paraformer 实时识别、Qwen 克隆音色和 Qwen TTS
    </div>

    <Alert
      v-if="error"
      class="status-alert"
      :message="error"
      show-icon
      type="error"
    />
    <Alert
      v-else
      class="status-alert"
      :description="`当前状态：${statusLabel}；${sourceLabel}；版本 ${credential.version}`"
      :message="loading ? '正在读取百炼公共 API Key' : statusLabel"
      show-icon
      :type="credential.apiKeySet ? 'success' : 'info'"
    />

    <template v-if="!error">
      <label class="input-label" for="bailian-api-key-input"
        >公共 API Key</label
      >
      <Input.Password
        id="bailian-api-key-input"
        v-model:value="apiKey"
        data-testid="bailian-api-key-input"
        placeholder="留空表示保留现有 Key"
      />
      <div class="input-tip">输入新 Key 后保存；留空保存不会覆盖当前 Key。</div>
    </template>

    <Space class="actions" wrap>
      <Button v-if="error" :loading="loading" @click="load"> 重新加载 </Button>
      <template v-else>
        <Button :loading="saving" type="primary" @click="save()">保存</Button>
        <Button :disabled="loading || saving" danger @click="confirmClear">
          清空 Key
        </Button>
        <Button :loading="loading" :disabled="saving" @click="load">
          重新加载
        </Button>
      </template>
    </Space>
  </Card>
</template>

<style scoped>
.bailian-credentials-card {
  border-radius: 8px;
}

.card-title {
  font-size: 16px;
  font-weight: 600;
}

.card-description,
.input-tip {
  margin-top: 6px;
  color: #667085;
  font-size: 13px;
}

.status-alert {
  margin: 16px 0;
}

.input-label {
  display: block;
  margin-bottom: 8px;
  font-weight: 500;
}

.actions {
  margin-top: 16px;
}
</style>
