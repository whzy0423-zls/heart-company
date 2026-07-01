<script setup lang="ts">
import { Col, Form, Input, Row, Textarea } from 'ant-design-vue';

import EditorShell from './components/editor-shell.vue';
import ImagePathInput from './components/image-path-input.vue';
import { useSiteConfigEditor } from './use-site-config-editor';

const { config, loading, saveConfig, saving } = useSiteConfigEditor();
</script>

<template>
  <EditorShell
    description="当前先管理首页老师简介 teaser；老师详情页字段后续可继续抽取。"
    :loading="loading"
    :saving="saving"
    title="老师管理"
    @save="saveConfig"
  >
    <Form v-if="config" layout="vertical">
      <Row :gutter="16">
        <Col :md="8" :xs="24">
          <Form.Item label="Eyebrow">
            <Input v-model:value="config.home.teacherTeaser.eyebrow" />
          </Form.Item>
        </Col>
        <Col :md="16" :xs="24">
          <Form.Item label="标题">
            <Input v-model:value="config.home.teacherTeaser.title" />
          </Form.Item>
        </Col>
        <Col :md="12" :xs="24">
          <Form.Item label="主图">
            <ImagePathInput
              v-model:value="config.home.teacherTeaser.image"
              dir="teacher"
              empty-text="未设置主图"
              upload-text="上传主图"
            />
          </Form.Item>
        </Col>
        <Col :md="12" :xs="24">
          <Form.Item label="兜底图">
            <ImagePathInput
              v-model:value="config.home.teacherTeaser.fallbackImage"
              dir="teacher"
              empty-text="未设置兜底图"
              upload-text="上传兜底图"
            />
          </Form.Item>
        </Col>
        <Col :xs="24">
          <Form.Item label="简介摘要">
            <Textarea
              v-model:value="config.home.teacherTeaser.lead"
              :rows="5"
            />
          </Form.Item>
        </Col>
        <Col :md="12" :xs="24">
          <Form.Item label="按钮文字">
            <Input v-model:value="config.home.teacherTeaser.buttonText" />
          </Form.Item>
        </Col>
        <Col :md="12" :xs="24">
          <Form.Item label="按钮链接">
            <Input v-model:value="config.home.teacherTeaser.buttonTo" />
          </Form.Item>
        </Col>
      </Row>
    </Form>
  </EditorShell>
</template>
