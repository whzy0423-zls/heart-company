<script setup lang="ts">
import type { AppAnalyticsOverview } from '#/api/core/app-analytics';

import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';

import { Page } from '@vben/common-ui';

import {
  Alert,
  Button,
  Card,
  Col,
  Row,
  Space,
  Statistic,
  Table,
  Tag,
} from 'ant-design-vue';

import { getAppAnalyticsOverviewApi } from '#/api';

import {
  appAnalyticsStatCards,
  distributionRows,
  emptyAppAnalyticsOverview,
  normalizeRecentRows,
} from './app-analytics';

const router = useRouter();
const loading = ref(false);
const loadError = ref('');
const overview = ref<AppAnalyticsOverview>(emptyAppAnalyticsOverview());

const memberLabels: Record<string, string> = {
  free: '普通用户',
  svip: '超级会员',
  vip: 'VIP 会员',
};

const statusLabels: Record<string, string> = {
  active: '正常',
  disabled: '禁用',
};

const statCards = computed(() => appAnalyticsStatCards(overview.value));
const memberRows = computed(() =>
  distributionRows(
    overview.value.memberLevelDistribution ??
      overview.value.memberDistribution ??
      {},
    memberLabels,
  ),
);
const statusRows = computed(() =>
  distributionRows(overview.value.statusDistribution ?? {}, statusLabels),
);
const recentUsers = computed(() => normalizeRecentRows(overview.value.recentUsers));
const recentExtractedUsers = computed(() =>
  normalizeRecentRows(
    overview.value.recentExtractedUsers ?? overview.value.recentMemoryUsers,
  ),
);

const distributionColumns = [
  { dataIndex: 'label', title: '分类' },
  { dataIndex: 'count', title: '人数', width: 90 },
  { dataIndex: 'percent', title: '占比', width: 90 },
];

const recentUserColumns = [
  { dataIndex: 'phone', title: '手机号', width: 150 },
  { dataIndex: 'nickname', title: '昵称', width: 130 },
  { dataIndex: 'memberLevel', title: '会员', width: 100 },
  { dataIndex: 'status', title: '状态', width: 90 },
  { dataIndex: 'createTime', title: '注册时间', width: 170 },
  { key: 'action', title: '操作', width: 90 },
];

const recentInsightColumns = [
  { dataIndex: 'phone', title: '手机号', width: 150 },
  { dataIndex: 'nickname', title: '昵称', width: 130 },
  { dataIndex: 'primaryType', title: '主型', width: 90 },
  { dataIndex: 'memoryCount', title: '记忆', width: 90 },
  { dataIndex: 'lastMemoryAt', title: '最近沉淀时间', width: 170 },
  { key: 'action', title: '操作', width: 90 },
];

function memberLabel(value?: string) {
  return value ? memberLabels[value] || value : '-';
}

function statusLabel(value?: string) {
  return value ? statusLabels[value] || value : '-';
}

function enneagramLabel(value?: number) {
  return value && value > 0 ? `${value}号` : '-';
}

function goInsights(phone?: string) {
  router.push({
    path: '/customer/user-insights',
    query: phone ? { keyword: phone } : undefined,
  });
}

async function loadOverview() {
  loading.value = true;
  loadError.value = '';
  try {
    overview.value = {
      ...emptyAppAnalyticsOverview(),
      ...(await getAppAnalyticsOverviewApi()),
    };
  } catch {
    loadError.value = 'App 数据概览加载失败，请稍后重试';
  } finally {
    loading.value = false;
  }
}

onMounted(loadOverview);
</script>

<template>
  <Page
    title="App 数据看板"
    description="汇总 App 客户、会员状态与用户提炼沉淀情况。"
  >
    <div class="app-dashboard-page">
      <Alert
        v-if="loadError"
        :message="loadError"
        show-icon
        type="error"
      >
        <template #action>
          <Button size="small" type="link" @click="loadOverview">重试</Button>
        </template>
      </Alert>

      <div class="toolbar">
        <Space>
          <Button :loading="loading" type="primary" @click="loadOverview">刷新</Button>
          <Button @click="router.push('/customer/app-users')">查看 App 客户</Button>
        </Space>
      </div>

      <Row :gutter="[16, 16]">
        <Col v-for="item in statCards" :key="item.label" :lg="6" :md="12" :xs="24">
          <Card :loading="loading" :bordered="false">
            <Statistic :title="item.label" :value="item.value" />
            <div class="stat-accent" :style="{ background: item.color }"></div>
          </Card>
        </Col>
      </Row>

      <Row :gutter="[16, 16]">
        <Col :lg="12" :xs="24">
          <Card :bordered="false" title="会员分布">
            <Table
              :columns="distributionColumns"
              :data-source="memberRows"
              :loading="loading"
              :pagination="false"
              row-key="value"
              size="small"
            />
          </Card>
        </Col>
        <Col :lg="12" :xs="24">
          <Card :bordered="false" title="状态分布">
            <Table
              :columns="distributionColumns"
              :data-source="statusRows"
              :loading="loading"
              :pagination="false"
              row-key="value"
              size="small"
            />
          </Card>
        </Col>
      </Row>

      <Card :bordered="false" title="最近用户">
        <Table
          :columns="recentUserColumns"
          :data-source="recentUsers"
          :loading="loading"
          :pagination="false"
          :scroll="{ x: 830 }"
          row-key="id"
          size="middle"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'phone'">
              {{ record.phone || '-' }}
            </template>
            <template v-if="column.dataIndex === 'nickname'">
              {{ record.title }}
            </template>
            <template v-if="column.dataIndex === 'memberLevel'">
              <Tag>{{ memberLabel(record.memberLevel) }}</Tag>
            </template>
            <template v-if="column.dataIndex === 'status'">
              <Tag :color="record.status === 'active' ? 'success' : 'default'">
                {{ statusLabel(record.status) }}
              </Tag>
            </template>
            <template v-if="column.dataIndex === 'createTime'">
              {{ record.createTime || '-' }}
            </template>
            <template v-if="column.key === 'action'">
              <Button size="small" type="link" @click="goInsights(record.phone)">360</Button>
            </template>
          </template>
        </Table>
      </Card>

      <Card :bordered="false" title="最近提炼用户">
        <Table
          :columns="recentInsightColumns"
          :data-source="recentExtractedUsers"
          :loading="loading"
          :pagination="false"
          :scroll="{ x: 820 }"
          row-key="id"
          size="middle"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'phone'">
              {{ record.phone || '-' }}
            </template>
            <template v-if="column.dataIndex === 'nickname'">
              {{ record.title }}
            </template>
            <template v-if="column.dataIndex === 'primaryType'">
              <Tag :color="record.primaryType ? 'processing' : 'default'">
                {{ enneagramLabel(record.primaryType) }}
              </Tag>
            </template>
            <template v-if="column.dataIndex === 'memoryCount'">
              {{ record.memoryCount ?? 0 }} 条
            </template>
            <template v-if="column.dataIndex === 'lastMemoryAt'">
              {{ record.lastMemoryAt || record.latestMemory || '-' }}
            </template>
            <template v-if="column.key === 'action'">
              <Button size="small" type="link" @click="goInsights(record.phone)">360</Button>
            </template>
          </template>
        </Table>
      </Card>
    </div>
  </Page>
</template>

<style scoped>
.app-dashboard-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.toolbar {
  display: flex;
  justify-content: flex-end;
}

.stat-accent {
  width: 42px;
  height: 4px;
  margin-top: 14px;
  border-radius: 999px;
}
</style>
