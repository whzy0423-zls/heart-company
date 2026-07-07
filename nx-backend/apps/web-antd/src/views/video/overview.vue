<script setup lang="ts">
import type { VideoGeneration, VideoGenerationsOverview } from '#/api';

import { computed, onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Alert,
  Button,
  Card,
  Col,
  Row,
  Statistic,
  Table,
  Tag,
} from 'ant-design-vue';

import { getVideoGenerationsOverviewApi } from '#/api';

const loading = ref(false);
const loadError = ref('');
const overview = ref<VideoGenerationsOverview>({
  recent: [],
  statusCounts: { completed: 0, failed: 0, inProgress: 0, queued: 0 },
  total: 0,
});

const statCards = computed(() => [
  {
    color: '#6366f1',
    label: '总生成次数',
    value: overview.value.total,
  },
  {
    color: '#22c55e',
    label: '已完成',
    value: overview.value.statusCounts.completed,
  },
  {
    color: '#f59e0b',
    label: '生成中',
    value: overview.value.statusCounts.inProgress,
  },
  {
    color: '#ef4444',
    label: '失败',
    value: overview.value.statusCounts.failed,
  },
]);

const recentColumns = [
  { dataIndex: 'id', title: 'ID', width: 80 },
  { dataIndex: 'prompt', ellipsis: true, title: '提示词' },
  { dataIndex: 'model', title: '模型', width: 180 },
  { dataIndex: 'status', title: '状态', width: 120 },
  { dataIndex: 'createTime', title: '创建时间', width: 180 },
];

const STATUS_COLOR: Record<string, string> = {
  completed: 'success',
  failed: 'error',
  in_progress: 'processing',
  queued: 'default',
  succeeded: 'success',
};

const STATUS_LABEL: Record<string, string> = {
  completed: '已完成',
  failed: '失败',
  in_progress: '生成中',
  queued: '队列中',
  succeeded: '已完成',
};

function statusColor(status: string) {
  return STATUS_COLOR[status] ?? 'default';
}

function statusLabel(status: string) {
  return STATUS_LABEL[status] ?? status;
}

function asGeneration(record: Record<string, any>): VideoGeneration {
  return record as VideoGeneration;
}

async function load() {
  loading.value = true;
  loadError.value = '';
  try {
    const result = await getVideoGenerationsOverviewApi();
    overview.value = result;
  } catch (error: any) {
    loadError.value =
      error?.response?.data?.error ||
      error?.response?.data?.message ||
      error?.message ||
      '加载失败，请稍后重试';
  } finally {
    loading.value = false;
  }
}

onMounted(load);
</script>

<template>
  <Page description="汇总视频生成任务的整体状态与最近记录。" title="生成概览">
    <div class="video-overview-page">
      <Alert v-if="loadError" :message="loadError" show-icon type="error">
        <template #action>
          <Button size="small" type="link" @click="load">重试</Button>
        </template>
      </Alert>

      <div class="overview-toolbar">
        <Button :loading="loading" type="primary" @click="load">刷新</Button>
      </div>

      <Row :gutter="[16, 16]">
        <Col
          v-for="card in statCards"
          :key="card.label"
          :lg="6"
          :md="12"
          :xs="24"
        >
          <Card :bordered="false" :loading="loading">
            <Statistic :title="card.label" :value="card.value" />
            <div
              class="stat-accent"
              :style="{ background: card.color }"
            ></div>
          </Card>
        </Col>
      </Row>

      <Card :bordered="false" title="最近 10 条生成记录">
        <Table
          :columns="recentColumns"
          :data-source="overview.recent"
          :loading="loading"
          :pagination="false"
          :scroll="{ x: 860 }"
          row-key="id"
          size="middle"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'status'">
              <Tag :color="statusColor(asGeneration(record).status)">
                {{ statusLabel(asGeneration(record).status) }}
              </Tag>
            </template>
          </template>
        </Table>
      </Card>
    </div>
  </Page>
</template>

<style scoped>
.video-overview-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.overview-toolbar {
  display: flex;
  justify-content: flex-end;
}

.stat-accent {
  height: 4px;
  margin-top: 14px;
  width: 42px;
  border-radius: 999px;
}
</style>
