<script setup lang="ts">
import { Button, Card, Col, Form, Input, Row, Textarea } from 'ant-design-vue';

import EditorShell from './components/editor-shell.vue';
import ImagePathInput from './components/image-path-input.vue';
import { useSiteConfigEditor } from './use-site-config-editor';

const { config, loading, saveConfig, saving } = useSiteConfigEditor();

function addType() {
  const next = String((config.value?.types.length ?? 0) + 1);
  config.value?.types.push({
    avatar: `/assets/avatars/${next}.png`,
    description: '',
    id: next,
    keywords: '',
    name: '新类型',
  });
}

function removeAt<T>(list: T[], index: number) {
  list.splice(index, 1);
}
</script>

<template>
  <EditorShell
    description="配置九种芯片模式的名称、关键词、描述和头像。"
    :loading="loading"
    :saving="saving"
    title="九型数据"
    @save="saveConfig"
  >
    <div v-if="config" class="stack">
      <div class="section-head">
        <h3>类型条目</h3>
        <Button @click="addType">新增类型</Button>
      </div>
      <Card v-for="(item, index) in config.types" :key="item.id" size="small">
        <Form layout="vertical">
          <Row :gutter="12">
            <Col :md="3" :xs="24">
              <Form.Item label="编号">
                <Input v-model:value="item.id" />
              </Form.Item>
            </Col>
            <Col :md="5" :xs="24">
              <Form.Item label="名称">
                <Input v-model:value="item.name" />
              </Form.Item>
            </Col>
            <Col :md="8" :xs="24">
              <Form.Item label="关键词">
                <Input v-model:value="item.keywords" />
              </Form.Item>
            </Col>
            <Col :md="8" :xs="24">
              <Form.Item label="头像">
                <ImagePathInput
                  v-model:value="item.avatar"
                  dir="avatars"
                  empty-text="未设置头像"
                  upload-text="上传头像"
                />
              </Form.Item>
            </Col>
            <Col :xs="24">
              <Form.Item label="描述">
                <Textarea v-model:value="item.description" :rows="2" />
              </Form.Item>
            </Col>
            <Col :xs="24">
              <Button danger @click="removeAt(config.types, index)">
                删除类型
              </Button>
            </Col>
          </Row>
        </Form>
      </Card>
    </div>
  </EditorShell>
</template>

<style scoped>
.stack {
  display: grid;
  gap: 12px;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.section-head h3 {
  margin: 0;
}
</style>
