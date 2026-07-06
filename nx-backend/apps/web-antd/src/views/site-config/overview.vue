<script setup lang="ts">
import type { SiteBuildStatus } from '#/api';

import { onMounted, ref } from 'vue';

import { Button, Card, Col, message, Row, Space, Tag } from 'ant-design-vue';

import { getSiteBuildStatusApi } from '#/api';

import EditorShell from './components/editor-shell.vue';
import { useSiteConfigEditor } from './use-site-config-editor';

const { config, loading, metrics, saveConfig, saving } = useSiteConfigEditor();
const buildStatus = ref<SiteBuildStatus>();
const buildStatusLoading = ref(false);

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

function stateColor(state?: string) {
  if (state === 'success' || state === 'completed') return 'success';
  if (state === 'failed' || state === 'error') return 'error';
  if (state === 'running' || state === 'queued' || state === 'pending' || state === 'building') return 'processing';
  return 'default';
}

async function refreshBuildStatus() {
  buildStatusLoading.value = true;
  try {
    buildStatus.value = await getSiteBuildStatusApi();
  } catch (error) {
    message.error(error instanceof Error ? error.message : '构建状态加载失败');
  } finally {
    buildStatusLoading.value = false;
  }
}

async function saveAndRefreshBuildStatus() {
  await saveConfig();
  await refreshBuildStatus();
}

onMounted(refreshBuildStatus);
</script>

<template>
  <EditorShell
    description="当前后台已按官网页面拆分配置入口；保存后可在这里查看官网构建/发布状态。"
    :loading="loading"
    :saving="saving"
    title="官网管理概览"
    @save="saveAndRefreshBuildStatus"
  >
    <Row v-if="config" :gutter="[16, 16]">
      <Col v-for="item in cards" :key="item.key" :lg="6" :md="12" :xs="24">
        <Card class="metric-card" size="small">
          <span>{{ item.label }}</span>
          <strong>{{ metrics[item.key] }}</strong>
        </Card>
      </Col>
      <Col :xs="24">
        <Card class="build-status-card" size="small" title="构建状态">
          <template #extra>
            <Button :loading="buildStatusLoading" size="small" @click="refreshBuildStatus">
              刷新状态
            </Button>
          </template>
          <div v-if="buildStatus" class="build-status-content">
            <Space wrap>
              <Tag :color="stateColor(buildStatus.state)">{{ buildStatus.state || 'unknown' }}</Tag>
              <Tag v-if="buildStatus.queuedNext" color="processing">已排队下一次构建</Tag>
            </Space>
            <p>{{ buildStatus.message || '暂无构建消息' }}</p>
            <div class="build-meta">
              <span>开始：{{ buildStatus.startedAt || '-' }}</span>
              <span>结束：{{ buildStatus.finishedAt || '-' }}</span>
              <span>耗时：{{ buildStatus.durationMs || 0 }}ms</span>
            </div>
            <pre v-if="buildStatus.log" class="build-log">{{ buildStatus.log }}</pre>
          </div>
          <div v-else class="build-status-empty">暂无构建状态</div>
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

.build-status-content {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.build-status-content p {
  margin: 0;
  color: hsl(var(--foreground));
}

.build-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  color: hsl(var(--muted-foreground));
}

.build-log {
  max-height: 180px;
  padding: 12px;
  overflow: auto;
  font-size: 12px;
  white-space: pre-wrap;
  background: hsl(var(--muted) / 50%);
  border-radius: 6px;
}

.build-status-empty {
  color: hsl(var(--muted-foreground));
}
</style>
