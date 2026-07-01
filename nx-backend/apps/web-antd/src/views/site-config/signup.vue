<script setup lang="ts">
import { Col, Form, Input, Row, Textarea } from 'ant-design-vue';

import EditorShell from './components/editor-shell.vue';
import { useSiteConfigEditor } from './use-site-config-editor';

const { config, linesToArray, loading, saveConfig, saving } =
  useSiteConfigEditor();
</script>

<template>
  <EditorShell
    description="配置报名咨询区文案、卖点和兴趣方向选项。"
    :loading="loading"
    :saving="saving"
    title="报名表单"
    @save="saveConfig"
  >
    <Form v-if="config" layout="vertical">
      <Row :gutter="16">
        <Col :md="8" :xs="24">
          <Form.Item label="Eyebrow">
            <Input v-model:value="config.home.signup.eyebrow" />
          </Form.Item>
        </Col>
        <Col :md="16" :xs="24">
          <Form.Item label="标题">
            <Input v-model:value="config.home.signup.title" />
          </Form.Item>
        </Col>
        <Col :xs="24">
          <Form.Item label="说明">
            <Textarea v-model:value="config.home.signup.lead" :rows="3" />
          </Form.Item>
        </Col>
        <Col :xs="24">
          <Form.Item label="卖点，每行一条">
            <Textarea
              :rows="4"
              :value="config.home.signup.bullets.join('\n')"
              @update:value="config.home.signup.bullets = linesToArray($event)"
            />
          </Form.Item>
        </Col>
        <Col :xs="24">
          <Form.Item label="兴趣方向，每行一条">
            <Textarea
              :rows="4"
              :value="config.home.signup.interestOptions.join('\n')"
              @update:value="
                config.home.signup.interestOptions = linesToArray($event)
              "
            />
          </Form.Item>
        </Col>
        <Col :xs="24">
          <Form.Item label="提交成功文案">
            <Input v-model:value="config.home.signup.successText" />
          </Form.Item>
        </Col>
      </Row>
    </Form>
  </EditorShell>
</template>
