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
    description="配置课程方向与报名产品，不管理课堂音视频内容；视频、音频课件请前往「老师课堂」。"
    :loading="loading"
    :saving="saving"
    title="课程产品管理"
    @save="saveConfig"
  >
    <Form v-if="config" layout="vertical">
      <Row :gutter="16">
        <Col :md="8" :xs="24">
          <Form.Item label="Eyebrow">
            <Input v-model:value="config.home.courses.eyebrow"  placeholder="请输入Eyebrow"/>
          </Form.Item>
        </Col>
        <Col :md="16" :xs="24">
          <Form.Item label="区块标题">
            <Input v-model:value="config.home.courses.title"  placeholder="请输入区块标题"/>
          </Form.Item>
        </Col>
      </Row>
      <div class="section-head">
        <h3>课程产品卡片</h3>
        <Button @click="addCourse">新增课程产品</Button>
      </div>
      <Card
        v-for="(item, index) in config.home.courses.items"
        :key="index"
        size="small"
      >
        <Row :gutter="12">
          <Col :md="4" :xs="24">
            <Form.Item label="徽标">
              <Input v-model:value="item.badge"  placeholder="请输入徽标"/>
            </Form.Item>
          </Col>
          <Col :md="20" :xs="24">
            <Form.Item label="标题">
              <Input v-model:value="item.title"  placeholder="请输入标题"/>
            </Form.Item>
          </Col>
          <Col :xs="24">
            <Form.Item label="描述">
              <Textarea v-model:value="item.description" :rows="2"  placeholder="请输入描述"/>
            </Form.Item>
          </Col>
          <Col :xs="24">
            <Form.Item label="要点，每行一条">
              <Textarea
                :rows="4"
                :value="item.bullets.join('\n')"
                @update:value="item.bullets = linesToArray($event)"
               placeholder="请输入要点，每行一条"/>
            </Form.Item>
          </Col>
          <Col :xs="24">
            <Button danger @click="removeAt(config.home.courses.items, index)">
              删除课程产品
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
