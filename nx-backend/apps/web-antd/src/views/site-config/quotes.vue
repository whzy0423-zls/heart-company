<script setup lang="ts">
import { Col, Form, Input, Row, Textarea } from 'ant-design-vue';

import EditorShell from './components/editor-shell.vue';
import { useSiteConfigEditor } from './use-site-config-editor';

const { config, linesToArray, loading, saveConfig, saving } =
  useSiteConfigEditor();
</script>

<template>
  <EditorShell
    description="配置老韩语录互动区。"
    :loading="loading"
    :saving="saving"
    title="语录互动"
    @save="saveConfig"
  >
    <Form v-if="config" layout="vertical">
      <Row :gutter="16">
        <Col :md="8" :xs="24">
          <Form.Item label="Eyebrow">
            <Input v-model:value="config.home.quotes.eyebrow" />
          </Form.Item>
        </Col>
        <Col :md="16" :xs="24">
          <Form.Item label="标题">
            <Input v-model:value="config.home.quotes.title" />
          </Form.Item>
        </Col>
        <Col :xs="24">
          <Form.Item label="说明">
            <Textarea v-model:value="config.home.quotes.lead" :rows="3" />
          </Form.Item>
        </Col>
        <Col :xs="24">
          <Form.Item label="语录，每行一条">
            <Textarea
              :rows="8"
              :value="config.home.quotes.items.join('\n')"
              @update:value="config.home.quotes.items = linesToArray($event)"
            />
          </Form.Item>
        </Col>
      </Row>
    </Form>
  </EditorShell>
</template>
