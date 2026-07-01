<script setup lang="ts">
import type { UploadRequestOption } from 'ant-design-vue/es/vc-upload/interface';

import type { UpdateUserProfileParams } from '#/api';

import { computed, onMounted, reactive, shallowRef } from 'vue';

import { useUserStore } from '@vben/stores';

import { Avatar, Button, Form, Input, message, Upload } from 'ant-design-vue';

import { getUserInfoApi, updateUserProfileApi, uploadFileApi } from '#/api';

const userStore = useUserStore();
const loading = shallowRef(false);
const uploading = shallowRef(false);
const avatarLoadFailed = shallowRef(false);

const form = reactive<UpdateUserProfileParams>({
  avatar: '',
  email: '',
  phone: '',
  realName: '',
  remark: '',
  username: '',
});

function fillForm(data: Partial<UpdateUserProfileParams>) {
  form.avatar = data.avatar ?? '';
  form.email = data.email ?? '';
  form.phone = data.phone ?? '';
  form.realName = data.realName ?? '';
  form.remark = data.remark ?? '';
  form.username = data.username ?? '';
  avatarLoadFailed.value = false;
}

async function loadProfile() {
  loading.value = true;
  try {
    const data = await getUserInfoApi();
    fillForm(data);
  } finally {
    loading.value = false;
  }
}

function toProfilePayload(
  overrides: Partial<UpdateUserProfileParams> = {},
): UpdateUserProfileParams {
  return {
    avatar: form.avatar,
    email: form.email,
    phone: form.phone,
    realName: form.realName,
    remark: form.remark,
    username: form.username,
    ...overrides,
  };
}

async function saveProfile(overrides: Partial<UpdateUserProfileParams> = {}) {
  const data = await updateUserProfileApi(toProfilePayload(overrides));
  fillForm(data);
  userStore.setUserInfo(data);
  return data;
}

async function customRequest(options: UploadRequestOption) {
  const file = options.file as File;
  uploading.value = true;
  try {
    const result = await uploadFileApi(file, 'user-avatars');
    await saveProfile({ avatar: result.url });
    options.onSuccess?.(result, file as any);
    message.success('头像已上传并保存');
  } catch (error) {
    options.onError?.(error as Error);
    message.error('头像上传失败，请稍后再试');
  } finally {
    uploading.value = false;
  }
}

const displayName = computed(
  () => form.realName || form.username || '九型用户',
);
const avatarText = computed(
  () => form.realName?.slice(0, 1) || form.username?.slice(0, 1) || '九',
);
const avatarSrc = computed(() =>
  form.avatar && !avatarLoadFailed.value ? form.avatar : undefined,
);
const contactItems = computed(() => [
  {
    label: '用户名',
    value: form.username || '-',
  },
  {
    label: '邮箱',
    value: form.email || '-',
  },
  {
    label: '手机号',
    value: form.phone || '-',
  },
]);

async function handleSubmit() {
  if (!form.username.trim()) {
    message.warning('请输入用户名');
    return;
  }
  if (!form.realName.trim()) {
    message.warning('请输入姓名');
    return;
  }

  loading.value = true;
  try {
    await saveProfile();
    message.success('个人信息已更新');
  } finally {
    loading.value = false;
  }
}

onMounted(loadProfile);
</script>

<template>
  <div class="profile-settings">
    <aside class="profile-card profile-card--summary">
      <Upload
        accept="image/*"
        :custom-request="customRequest"
        :disabled="uploading"
        :max-count="1"
        :show-upload-list="false"
      >
        <div
          class="profile-avatar-wrap"
          :class="{ 'profile-avatar-wrap--uploading': uploading }"
          role="button"
          tabindex="0"
        >
          <Avatar
            class="profile-avatar"
            :size="96"
            :src="avatarSrc"
            @error="avatarLoadFailed = true"
          >
            {{ avatarText }}
          </Avatar>
        </div>
      </Upload>

      <div class="profile-summary">
        <h2>{{ displayName }}</h2>
        <p>
          {{ form.remark || '完善资料后，团队成员能更清楚地识别当前账号。' }}
        </p>
      </div>

      <div class="profile-upload">
        <Upload
          accept="image/*"
          :custom-request="customRequest"
          :disabled="uploading"
          :max-count="1"
          :show-upload-list="false"
        >
          <Button block :loading="uploading" type="primary"> 上传头像 </Button>
        </Upload>
      </div>

      <dl class="profile-meta">
        <div v-for="item in contactItems" :key="item.label">
          <dt>{{ item.label }}</dt>
          <dd>{{ item.value }}</dd>
        </div>
      </dl>
    </aside>

    <section class="profile-card profile-card--form">
      <div class="profile-card__header">
        <div>
          <h2>基础资料</h2>
          <p>更新账号显示信息和联系方式。</p>
        </div>
      </div>

      <Form layout="vertical" :model="form" @finish="handleSubmit">
        <div class="profile-form-grid">
          <Form.Item
            label="用户名"
            name="username"
            :rules="[{ required: true, message: '请输入用户名' }]"
          >
            <Input v-model:value="form.username" allow-clear />
          </Form.Item>

          <Form.Item
            label="姓名"
            name="realName"
            :rules="[{ required: true, message: '请输入姓名' }]"
          >
            <Input v-model:value="form.realName" allow-clear />
          </Form.Item>

          <Form.Item label="邮箱" name="email">
            <Input v-model:value="form.email" allow-clear />
          </Form.Item>

          <Form.Item label="手机号" name="phone">
            <Input v-model:value="form.phone" allow-clear />
          </Form.Item>
        </div>

        <Form.Item label="个人简介" name="remark">
          <Input.TextArea
            v-model:value="form.remark"
            :auto-size="{ minRows: 5, maxRows: 8 }"
            allow-clear
          />
        </Form.Item>

        <div class="profile-actions">
          <Button html-type="submit" :loading="loading" type="primary">
            保存修改
          </Button>
        </div>
      </Form>
    </section>
  </div>
</template>

<style scoped>
.profile-settings {
  display: grid;
  grid-template-columns: minmax(260px, 320px) minmax(0, 1fr);
  gap: 18px;
  max-width: 1120px;
  margin: 0 auto;
}

.profile-card {
  background: hsl(var(--card));
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
  box-shadow: 0 12px 32px hsl(var(--foreground) / 6%);
}

.profile-card--summary {
  align-self: start;
  padding: 24px;
}

.profile-avatar-wrap {
  display: flex;
  justify-content: center;
  padding: 8px 0 18px;
  cursor: pointer;
  transition: opacity 0.2s ease;
}

.profile-avatar-wrap--uploading {
  cursor: wait;
  opacity: 0.72;
}

.profile-avatar {
  font-size: 34px;
  font-weight: 650;
  color: hsl(var(--primary));
  background: hsl(var(--primary) / 10%);
  border: 4px solid hsl(var(--background));
  box-shadow: 0 10px 26px hsl(var(--primary) / 16%);
}

.profile-summary {
  text-align: center;
}

.profile-summary h2,
.profile-card__header h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 650;
  line-height: 1.35;
  color: hsl(var(--foreground));
  letter-spacing: 0;
}

.profile-summary p,
.profile-card__header p {
  margin: 8px 0 0;
  font-size: 13px;
  line-height: 1.6;
  color: hsl(var(--muted-foreground));
}

.profile-upload {
  margin-top: 20px;
}

.profile-meta {
  display: grid;
  gap: 12px;
  padding-top: 20px;
  margin: 20px 0 0;
  border-top: 1px solid hsl(var(--border));
}

.profile-meta div {
  min-width: 0;
}

.profile-meta dt {
  margin-bottom: 4px;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}

.profile-meta dd {
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 14px;
  color: hsl(var(--foreground));
  white-space: nowrap;
}

.profile-card--form {
  padding: 24px;
}

.profile-card__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding-bottom: 20px;
  margin-bottom: 20px;
  border-bottom: 1px solid hsl(var(--border));
}

.profile-form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  column-gap: 18px;
}

.profile-actions {
  display: flex;
  justify-content: flex-end;
  padding-top: 8px;
}

@media (max-width: 960px) {
  .profile-settings {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 640px) {
  .profile-card--summary,
  .profile-card--form {
    padding: 18px;
  }

  .profile-form-grid {
    grid-template-columns: 1fr;
  }

  .profile-actions {
    justify-content: stretch;
  }

  .profile-actions :deep(.ant-btn) {
    width: 100%;
  }
}
</style>
