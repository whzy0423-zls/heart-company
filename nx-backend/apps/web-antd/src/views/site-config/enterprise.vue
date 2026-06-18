<script setup lang="ts">
import { Col, Form, Input, Row, Textarea } from 'ant-design-vue';

import EditorShell from './components/editor-shell.vue';
import { useSiteConfigEditor } from './use-site-config-editor';

const { config, linesToArray, loading, saveConfig, saving } = useSiteConfigEditor();
</script>

<template>
  <EditorShell description="配置首页企业课程和工作坊模块。" :loading="loading" :saving="saving" title="企业课程" @save="saveConfig">
    <Form v-if="config" layout="vertical">
      <Row :gutter="16">
        <Col :md="8" :xs="24"><Form.Item label="Eyebrow"><Input v-model:value="config.home.enterprise.eyebrow" /></Form.Item></Col>
        <Col :md="16" :xs="24"><Form.Item label="标题"><Input v-model:value="config.home.enterprise.title" /></Form.Item></Col>
        <Col :xs="24"><Form.Item label="描述"><Textarea v-model:value="config.home.enterprise.lead" :rows="4" /></Form.Item></Col>
        <Col :md="12" :xs="24"><Form.Item label="按钮文字"><Input v-model:value="config.home.enterprise.buttonText" /></Form.Item></Col>
        <Col :md="12" :xs="24"><Form.Item label="按钮链接"><Input v-model:value="config.home.enterprise.buttonHref" /></Form.Item></Col>
        <Col :md="12" :xs="24"><Form.Item label="模块标题"><Input v-model:value="config.home.enterprise.moduleTitle" /></Form.Item></Col>
        <Col :xs="24">
          <Form.Item label="工作坊模块，每行一条">
            <Textarea :rows="5" :value="config.home.enterprise.modules.join('\n')" @update:value="config.home.enterprise.modules = linesToArray($event)" />
          </Form.Item>
        </Col>
      </Row>
    </Form>
  </EditorShell>
</template>
