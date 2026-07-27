<script setup lang="ts">
import { Col, Form, Input, Row, Textarea } from 'ant-design-vue';

import EditorShell from './components/editor-shell.vue';
import ImagePathInput from './components/image-path-input.vue';
import { useSiteConfigEditor } from './use-site-config-editor';

const { config, loading, saveConfig, saving } = useSiteConfigEditor();
</script>

<template>
  <EditorShell
    description="配置官网与小程序展示的老师简介；课堂音视频内容及其老师快照请前往「老师课堂」管理。"
    :loading="loading"
    :saving="saving"
    title="老师资料管理"
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
