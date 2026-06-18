<script setup lang="ts">
import type { AdminBranding } from '#/api';

import { onMounted, ref } from 'vue';

import { updatePreferences } from '@vben/preferences';

import { Alert, Col, Form, Input, message, Row } from 'ant-design-vue';

import { getAdminBrandingApi, updateAdminBrandingApi } from '#/api';
import { BRANDING_CACHE_KEY } from '#/branding';

import EditorShell from '../site-config/components/editor-shell.vue';
import ImagePathInput from '../site-config/components/image-path-input.vue';

const loading = ref(true);
const saving = ref(false);
const form = ref<AdminBranding>({ loadingText: '', logo: '', name: '' });

onMounted(load);

async function load() {
  loading.value = true;
  try {
    const data = await getAdminBrandingApi();
    if (data) {
      form.value = {
        loadingText: data.loadingText ?? '',
        logo: data.logo ?? '',
        name: data.name ?? '',
      };
    }
  } finally {
    loading.value = false;
  }
}

async function save() {
  if (!form.value.name?.trim()) {
    message.warning('请填写后台名称');
    return;
  }
  saving.value = true;
  try {
    const saved = await updateAdminBrandingApi(form.value);
    // 即时生效：侧边栏 / 登录页 / 浏览器标题
    updatePreferences({
      app: { name: saved.name },
      logo: { enable: true, source: saved.logo },
    });
    // 写入缓存，供下次启动加载屏渲染
    localStorage.setItem(BRANDING_CACHE_KEY, JSON.stringify(saved));
    message.success('已保存并即时生效；启动加载屏将在下次刷新后更新');
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <EditorShell
    description="配置后台的 Logo、名称与启动加载屏。保存后侧边栏 / 登录页 / 标题即时生效，启动屏在下次刷新后更新。"
    :loading="loading"
    :saving="saving"
    title="后台品牌"
    @save="save"
  >
    <Form v-if="form" layout="vertical">
      <Row :gutter="24">
        <Col :md="12" :xs="24">
          <Form.Item label="后台名称" required>
            <Input
              v-model:value="form.name"
              placeholder="显示在侧边栏 / 登录页 / 浏览器标题"
            />
          </Form.Item>
          <Form.Item label="Logo">
            <ImagePathInput v-model:value="form.logo" dir="branding" empty-text="未设置 Logo" upload-text="上传 Logo" variant="image" />
          </Form.Item>
          <Form.Item label="启动加载屏文案">
            <Input
              v-model:value="form.loadingText"
              placeholder="留空则使用「后台名称」"
            />
          </Form.Item>
        </Col>
        <Col :md="12" :xs="24">
          <Alert
            type="info"
            show-icon
            message="提示"
            description="Logo 支持上传图片或直接填写路径 / 外链（如 /logo.png）。启动加载屏会在浏览器下次打开后台时显示新的 Logo 与文案。"
          />
        </Col>
      </Row>
    </Form>
  </EditorShell>
</template>

<style scoped>
</style>
