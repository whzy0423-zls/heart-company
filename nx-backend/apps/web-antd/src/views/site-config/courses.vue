<script setup lang="ts">
import { Button, Card, Col, Form, Input, Row, Textarea } from 'ant-design-vue';

import EditorShell from './components/editor-shell.vue';
import { useSiteConfigEditor } from './use-site-config-editor';

const { config, linesToArray, loading, saveConfig, saving } =
  useSiteConfigEditor();

function addCourse() {
  config.value?.home.courses.items.push({
    badge: 'N',
    bullets: ['课程要点'],
    description: '',
    title: '新课程',
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
    description="配置首页课程方向卡片，后续可扩展为课程产品库。"
    :loading="loading"
    :saving="saving"
    title="课程管理"
    @save="saveConfig"
  >
    <Form v-if="config" layout="vertical">
      <Row :gutter="16">
        <Col :md="8" :xs="24">
          <Form.Item label="Eyebrow">
            <Input v-model:value="config.home.courses.eyebrow" />
          </Form.Item>
        </Col>
        <Col :md="16" :xs="24">
          <Form.Item label="区块标题">
            <Input v-model:value="config.home.courses.title" />
          </Form.Item>
        </Col>
      </Row>
      <div class="section-head">
        <h3>课程卡片</h3>
        <Button @click="addCourse">新增课程</Button>
      </div>
      <Card
        v-for="(item, index) in config.home.courses.items"
        :key="index"
        size="small"
      >
        <Row :gutter="12">
          <Col :md="4" :xs="24">
            <Form.Item label="徽标">
              <Input v-model:value="item.badge" />
            </Form.Item>
          </Col>
          <Col :md="20" :xs="24">
            <Form.Item label="标题">
              <Input v-model:value="item.title" />
            </Form.Item>
          </Col>
          <Col :xs="24">
            <Form.Item label="描述">
              <Textarea v-model:value="item.description" :rows="2" />
            </Form.Item>
          </Col>
          <Col :xs="24">
            <Form.Item label="要点，每行一条">
              <Textarea
                :rows="4"
                :value="item.bullets.join('\n')"
                @update:value="item.bullets = linesToArray($event)"
              />
            </Form.Item>
          </Col>
          <Col :xs="24">
            <Button danger @click="removeAt(config.home.courses.items, index)">
              删除课程
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
