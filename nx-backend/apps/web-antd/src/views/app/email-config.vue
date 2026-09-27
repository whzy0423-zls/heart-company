<script setup lang="ts">
import type { AppEmailConfig } from '#/api';

import { onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { Alert, Button, Card, Form, Input, InputNumber, Switch, message } from 'ant-design-vue';

import { getAppEmailConfigApi, updateAppEmailConfigApi } from '#/api';

const loading = ref(true);
const saving = ref(false);
const error = ref('');
const form = reactive<AppEmailConfig>({
  enabled: false,
  host: '',
  port: 587,
  username: '',
  password: '',
  from: '',
  fromName: '',
});

onMounted(load);

async function load() {
  loading.value = true;
  error.value = '';
  try {
    Object.assign(form, await getAppEmailConfigApi(), { password: '' });
  } catch {
    error.value = 'SMTP 配置加载失败，请重新加载';
  } finally {
    loading.value = false;
  }
}

async function save() {
  if (saving.value) return;
  saving.value = true;
  try {
    Object.assign(form, await updateAppEmailConfigApi({ ...form }));
    form.password = '';
    message.success('SMTP 配置已保存');
  } catch (cause) {
    message.error(cause instanceof Error ? cause.message : 'SMTP 配置保存失败');
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <Page title="邮箱与 SMTP" description="用于 App 已绑定邮箱的密码找回验证码邮件。" :loading="loading">
    <Alert v-if="error" type="error" show-icon :message="error" />
    <Card :bordered="false" style="max-width: 760px">
      <Form layout="vertical" :model="form">
        <Form.Item label="启用邮箱找回">
          <Switch v-model:checked="form.enabled" />
        </Form.Item>
        <Form.Item label="SMTP 服务器" required>
          <Input v-model:value="form.host" placeholder="例如 smtp.example.com" />
        </Form.Item>
        <Form.Item label="SMTP 端口" required>
          <InputNumber v-model:value="form.port" :min="1" :max="65535" style="width: 100%" placeholder="例如 465" />
        </Form.Item>
        <Form.Item label="SMTP 用户名">
          <Input v-model:value="form.username" autocomplete="off" placeholder="SMTP 登录用户名" />
        </Form.Item>
        <Form.Item label="SMTP 密码">
          <Input v-model:value="form.password" type="password" autocomplete="new-password" placeholder="留空表示保持原密码" />
        </Form.Item>
        <Form.Item label="发件人邮箱" required>
          <Input v-model:value="form.from" placeholder="例如 no-reply@example.com" />
        </Form.Item>
        <Form.Item label="发件人名称">
          <Input v-model:value="form.fromName" placeholder="芯之力" />
        </Form.Item>
        <Button type="primary" :loading="saving" @click="save">保存 SMTP 配置</Button>
      </Form>
    </Card>
  </Page>
</template>
