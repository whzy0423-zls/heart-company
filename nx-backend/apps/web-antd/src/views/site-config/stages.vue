<script setup lang="ts">
import { Button, Card, Col, Form, Input, Row, Textarea } from 'ant-design-vue';

import EditorShell from './components/editor-shell.vue';
import { useSiteConfigEditor } from './use-site-config-editor';

const { config, loading, saveConfig, saving } = useSiteConfigEditor();

function addStage() {
  config.value?.home.stages.items.push({
    description: '',
    kicker: '新阶段',
    subtitle: '',
    title: '阶段标题',
    to: '/stages',
  });
}

function removeAt<T>(list: T[], index: number | string) {
  const position = Number(index);
  if (!Number.isInteger(position)) return;
  list.splice(position, 1);
}
</script>

<template>
  <EditorShell
    description="配置首页三阶段区块和阶段卡片。"
    :loading="loading"
    :saving="saving"
    title="三阶段管理"
    @save="saveConfig"
  >
    <Form v-if="config" layout="vertical">
      <Row :gutter="16">
        <Col :md="8" :xs="24">
          <Form.Item label="Eyebrow">
            <Input v-model:value="config.home.stages.eyebrow" />
          </Form.Item>
        </Col>
        <Col :md="16" :xs="24">
          <Form.Item label="标题">
            <Input v-model:value="config.home.stages.title" />
          </Form.Item>
        </Col>
        <Col :xs="24">
          <Form.Item label="说明">
            <Textarea v-model:value="config.home.stages.lead" :rows="3" />
          </Form.Item>
        </Col>
      </Row>
      <div class="section-head">
        <h3>阶段卡片</h3>
        <Button @click="addStage">新增阶段</Button>
      </div>
      <Card
        v-for="(item, index) in config.home.stages.items"
        :key="index"
        size="small"
      >
        <Row :gutter="12">
          <Col :md="6" :xs="24">
            <Form.Item label="阶段标识">
              <Input v-model:value="item.kicker" />
            </Form.Item>
          </Col>
          <Col :md="10" :xs="24">
            <Form.Item label="标题">
              <Input v-model:value="item.title" />
            </Form.Item>
          </Col>
          <Col :md="8" :xs="24">
            <Form.Item label="链接">
              <Input v-model:value="item.to" />
            </Form.Item>
          </Col>
          <Col :xs="24">
            <Form.Item label="副标题">
              <Input v-model:value="item.subtitle" />
            </Form.Item>
          </Col>
          <Col :xs="24">
            <Form.Item label="描述">
              <Textarea v-model:value="item.description" :rows="3" />
            </Form.Item>
          </Col>
          <Col :xs="24">
            <Button danger @click="removeAt(config.home.stages.items, index)">
              删除阶段
            </Button>
          </Col>
        </Row>
      </Card>
    </Form>
  </EditorShell>
</template>

<style scoped>
.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: 12px 0;
}

.section-head h3 {
  margin: 0;
}
</style>
