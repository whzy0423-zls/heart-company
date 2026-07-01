<script setup lang="ts">
import { Button, Card, Col, Input, Row, Select } from 'ant-design-vue';

import EditorShell from './components/editor-shell.vue';
import { useSiteConfigEditor } from './use-site-config-editor';

const { config, loading, saveConfig, saving } = useSiteConfigEditor();
const typeOptions = [
  { value: 'route' },
  { value: 'hash' },
  { value: 'anchor' },
];

function addMainNav() {
  config.value?.navigation.main.push({
    label: '新导航',
    to: '/',
    type: 'route',
  });
}

function addDrawerNav() {
  config.value?.navigation.drawer.push({
    label: '新导航',
    to: '/',
    type: 'route',
  });
}

function removeAt<T>(list: T[], index: number) {
  list.splice(index, 1);
}
</script>

<template>
  <EditorShell
    description="配置官网顶部导航、移动端抽屉导航和底部 Tab。"
    :loading="loading"
    :saving="saving"
    title="导航管理"
    @save="saveConfig"
  >
    <div v-if="config" class="stack">
      <section>
        <div class="section-head">
          <h3>顶部导航</h3>
          <Button @click="addMainNav">新增</Button>
        </div>
        <Card
          v-for="(item, index) in config.navigation.main"
          :key="index"
          size="small"
        >
          <Row :gutter="12">
            <Col :md="7" :xs="24">
              <Input v-model:value="item.label" placeholder="名称" />
            </Col>
            <Col :md="9" :xs="24">
              <Input v-model:value="item.to" placeholder="链接" />
            </Col>
            <Col :md="5" :xs="24">
              <Select v-model:value="item.type" :options="typeOptions" />
            </Col>
            <Col :md="3" :xs="24">
              <Button
                danger
                block
                @click="removeAt(config.navigation.main, index)"
              >
                删除
              </Button>
            </Col>
          </Row>
        </Card>
      </section>

      <section>
        <div class="section-head">
          <h3>抽屉导航</h3>
          <Button @click="addDrawerNav">新增</Button>
        </div>
        <Card
          v-for="(item, index) in config.navigation.drawer"
          :key="index"
          size="small"
        >
          <Row :gutter="12">
            <Col :md="7" :xs="24">
              <Input v-model:value="item.label" placeholder="名称" />
            </Col>
            <Col :md="9" :xs="24">
              <Input v-model:value="item.to" placeholder="链接" />
            </Col>
            <Col :md="5" :xs="24">
              <Select v-model:value="item.type" :options="typeOptions" />
            </Col>
            <Col :md="3" :xs="24">
              <Button
                danger
                block
                @click="removeAt(config.navigation.drawer, index)"
              >
                删除
              </Button>
            </Col>
          </Row>
        </Card>
      </section>
    </div>
  </EditorShell>
</template>

<style scoped>
.stack {
  display: grid;
  gap: 24px;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.section-head h3 {
  margin: 0;
}
</style>
