<script setup lang="ts">
import { computed } from 'vue';

import { Col, Form, Input, Row } from 'ant-design-vue';

import EditorShell from './components/editor-shell.vue';
import ImagePathInput from './components/image-path-input.vue';
import { useSiteConfigEditor } from './use-site-config-editor';

const { config, loading, saveConfig, saving } = useSiteConfigEditor();

const logoHelp = computed(() => {
  return config.value?.site.logo
    ? '图片上传成功后，保存配置会把 Logo 地址写入数据库。'
    : '请上传站点 Logo。';
});
</script>

<template>
  <EditorShell
    description="维护官网品牌、Logo 与页脚展示内容。"
    :loading="loading"
    :saving="saving"
    title="站点设置"
    @save="saveConfig"
  >
    <Form v-if="config" layout="vertical">
      <Row :gutter="16">
        <Col :md="12" :xs="24">
          <Form.Item label="品牌名称">
            <Input v-model:value="config.site.brandName" />
          </Form.Item>
        </Col>
        <Col :md="12" :xs="24">
          <Form.Item label="站点 Logo" :help="logoHelp" required>
            <ImagePathInput
              v-model:value="config.site.logo"
              dir="site-logo"
              empty-text="未设置 Logo"
              upload-text="上传 Logo"
              variant="image"
            />
          </Form.Item>
        </Col>
        <Col :md="12" :xs="24">
          <Form.Item label="页脚短语">
            <Input v-model:value="config.site.footerTagline" />
          </Form.Item>
        </Col>
        <Col :md="12" :xs="24">
          <Form.Item label="版权信息">
            <Input v-model:value="config.site.copyright" />
          </Form.Item>
        </Col>
      </Row>
    </Form>
  </EditorShell>
</template>
