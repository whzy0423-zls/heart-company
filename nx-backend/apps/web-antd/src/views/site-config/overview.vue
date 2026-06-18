<script setup lang="ts">
import { Card, Col, Row } from 'ant-design-vue';

import EditorShell from './components/editor-shell.vue';
import { useSiteConfigEditor } from './use-site-config-editor';

const { config, loading, metrics, saveConfig, saving } = useSiteConfigEditor();

const cards = [
  { key: 'mainNavCount', label: '顶部导航' },
  { key: 'drawerNavCount', label: '抽屉导航' },
  { key: 'tabCount', label: '底部 Tab' },
  { key: 'homeSectionCount', label: '首页区块' },
  { key: 'courseCount', label: '课程卡片' },
  { key: 'stageCount', label: '三阶段' },
  { key: 'quoteCount', label: '老韩语录' },
  { key: 'typeCount', label: '九型条目' },
] as const;
</script>

<template>
  <EditorShell
    description="当前后台已按官网页面拆分配置入口；此阶段只调整后台，不改官网展示代码。"
    :loading="loading"
    :saving="saving"
    title="官网管理概览"
    @save="saveConfig"
  >
    <Row v-if="config" :gutter="[16, 16]">
      <Col v-for="item in cards" :key="item.key" :lg="6" :md="12" :xs="24">
        <Card class="metric-card" size="small">
          <span>{{ item.label }}</span>
          <strong>{{ metrics[item.key] }}</strong>
        </Card>
      </Col>
    </Row>
  </EditorShell>
</template>

<style scoped>
.metric-card span,
.metric-card strong {
  display: block;
}

.metric-card span {
  color: hsl(var(--muted-foreground));
}

.metric-card strong {
  margin-top: 8px;
  font-size: 30px;
  line-height: 1;
}
</style>
