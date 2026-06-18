<script setup lang="ts">
import { Button, Card, Col, Form, Input, Row, Select, Textarea } from 'ant-design-vue';

import EditorShell from './components/editor-shell.vue';
import ImagePathInput from './components/image-path-input.vue';
import { useSiteConfigEditor } from './use-site-config-editor';

const { config, loading, saveConfig, saving } = useSiteConfigEditor();
const variantOptions = [{ value: 'red' }, { value: 'ghost' }, { value: 'blue' }];
const actionTypeOptions = [{ value: 'route' }, { value: 'anchor' }];

function addAction() {
  config.value?.home.hero.actions.push({ label: '新按钮', to: '/', type: 'route', variant: 'ghost' });
}

function addStat() {
  config.value?.home.hero.stats.push({ label: '新统计', value: '1' });
}

function removeAt<T>(list: T[], index: number) {
  list.splice(index, 1);
}
</script>

<template>
  <EditorShell description="配置首页 Hero、老师简介 teaser、小游戏入口和九型概览标题。" :loading="loading" :saving="saving" title="首页管理" @save="saveConfig">
    <Form v-if="config" layout="vertical">
      <Row :gutter="16">
        <Col :md="8" :xs="24"><Form.Item label="Hero Eyebrow"><Input v-model:value="config.home.hero.eyebrow" /></Form.Item></Col>
        <Col :md="16" :xs="24"><Form.Item label="Hero 标题"><Input v-model:value="config.home.hero.title" /></Form.Item></Col>
        <Col :xs="24"><Form.Item label="Hero 文案"><Textarea v-model:value="config.home.hero.lead" :rows="3" /></Form.Item></Col>
      </Row>

      <div class="section-head">
        <h3>Hero 按钮</h3>
        <Button @click="addAction">新增按钮</Button>
      </div>
      <Card v-for="(item, index) in config.home.hero.actions" :key="index" size="small">
        <Row :gutter="12">
          <Col :md="6" :xs="24"><Input v-model:value="item.label" placeholder="文字" /></Col>
          <Col :md="7" :xs="24"><Input v-model:value="item.to" placeholder="链接" /></Col>
          <Col :md="4" :xs="24"><Select v-model:value="item.variant" :options="variantOptions" /></Col>
          <Col :md="4" :xs="24"><Select v-model:value="item.type" :options="actionTypeOptions" /></Col>
          <Col :md="3" :xs="24"><Button danger block @click="removeAt(config.home.hero.actions, index)">删除</Button></Col>
        </Row>
      </Card>

      <div class="section-head">
        <h3>Hero 统计</h3>
        <Button @click="addStat">新增统计</Button>
      </div>
      <Card v-for="(item, index) in config.home.hero.stats" :key="index" size="small">
        <Row :gutter="12">
          <Col :md="6" :xs="24"><Input v-model:value="item.value" placeholder="数值" /></Col>
          <Col :md="6" :xs="24"><Input v-model:value="item.suffix" placeholder="后缀" /></Col>
          <Col :md="9" :xs="24"><Input v-model:value="item.label" placeholder="说明" /></Col>
          <Col :md="3" :xs="24"><Button danger block @click="removeAt(config.home.hero.stats, index)">删除</Button></Col>
        </Row>
      </Card>

      <Row :gutter="16" class="form-block">
        <Col :md="12" :xs="24"><Form.Item label="老师简介标题"><Input v-model:value="config.home.teacherTeaser.title" /></Form.Item></Col>
        <Col :md="12" :xs="24"><Form.Item label="老师图片"><ImagePathInput v-model:value="config.home.teacherTeaser.image" dir="teacher" empty-text="未设置老师图片" upload-text="上传图片" /></Form.Item></Col>
        <Col :xs="24"><Form.Item label="老师简介摘要"><Textarea v-model:value="config.home.teacherTeaser.lead" :rows="3" /></Form.Item></Col>
        <Col :md="12" :xs="24"><Form.Item label="小游戏标题"><Input v-model:value="config.home.game.title" /></Form.Item></Col>
        <Col :md="12" :xs="24"><Form.Item label="九型概览标题"><Input v-model:value="config.home.typesSection.title" /></Form.Item></Col>
      </Row>
    </Form>
  </EditorShell>
</template>

<style scoped>
.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: 18px 0 12px;
}

.section-head h3 {
  margin: 0;
}

.form-block {
  margin-top: 18px;
}
</style>
