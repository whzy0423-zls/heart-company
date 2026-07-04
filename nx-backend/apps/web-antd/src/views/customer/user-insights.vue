<script setup lang="ts">
import type { AppUserInsight } from '#/api';

import { computed, onMounted, reactive, ref, watch } from 'vue';

import {
  Alert,
  Button,
  Card,
  Descriptions,
  Drawer,
  Empty,
  Input,
  Select,
  Space,
  Table,
  Tag,
} from 'ant-design-vue';
import { useRoute } from 'vue-router';

import { getAppUserInsightsApi } from '#/api';

import PageShell from '../system/components/page-shell.vue';
import {
  enneagramLabel,
  getCenterSummary,
  getProfileList,
  getProfileSummary,
  getScoreTags,
  getUserInsightStatus,
} from './user-insights';

const memberLevelOptions = [
  { label: '普通用户', value: 'free' },
  { label: 'VIP 会员', value: 'vip' },
  { label: '超级会员', value: 'svip' },
];

const statusOptions = [
  { label: '正常', value: 'active' },
  { label: '禁用', value: 'disabled' },
];

const memberLevelLabels: Record<string, string> = {
  free: '普通用户',
  svip: '超级会员',
  vip: 'VIP 会员',
};

const statusLabels: Record<string, string> = {
  active: '正常',
  disabled: '禁用',
};

const statusColors: Record<string, string> = {
  active: 'success',
  disabled: 'error',
};

const insightStatusColors: Record<string, string> = {
  已有沉淀: 'success',
  已有画像: 'processing',
  待沉淀: 'default',
};

const route = useRoute();
const loading = ref(false);
const loadError = ref('');
const insights = ref<AppUserInsight[]>([]);
const total = ref(0);
const detailOpen = ref(false);
const detail = ref<AppUserInsight>();
const query = reactive({
  keyword: '',
  memberLevel: '',
  page: 1,
  pageSize: 20,
  status: '',
});
let requestId = 0;

const columns = [
  { dataIndex: 'phone', fixed: 'left' as const, title: '手机号', width: 150 },
  { dataIndex: 'nickname', title: '昵称', width: 150 },
  { dataIndex: 'memberLevel', title: '会员', width: 110 },
  { dataIndex: 'primaryType', title: '主型', width: 90 },
  { dataIndex: 'profile', title: '画像摘要', width: 300 },
  { dataIndex: 'memoryCount', title: '沉淀', width: 110 },
  { dataIndex: 'messageCount', title: '对话', width: 110 },
  { dataIndex: 'latestQuizTime', title: '最近测评', width: 170 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 100 },
];

const selectedStrengths = computed(() =>
  detail.value ? getProfileList(detail.value.profile, 'strengths') : [],
);
const selectedChallenges = computed(() =>
  detail.value ? getProfileList(detail.value.profile, 'challenges') : [],
);
const selectedScoreTags = computed(() =>
  detail.value ? getScoreTags(detail.value.score) : [],
);


function routeKeyword() {
  const value = route.query.keyword;
  return Array.isArray(value) ? value[0] || '' : value || '';
}

function applyRouteKeyword() {
  const keyword = routeKeyword().trim();
  if (!keyword || query.keyword === keyword) return false;
  query.keyword = keyword;
  query.page = 1;
  return true;
}

function memberLevelLabel(value?: string) {
  return value ? memberLevelLabels[value] || value : '-';
}

function statusLabel(value?: string) {
  return value ? statusLabels[value] || value : '-';
}

function insightStatus(record: AppUserInsight) {
  return getUserInsightStatus(record);
}

async function load() {
  const currentRequestId = ++requestId;
  loading.value = true;
  try {
    loadError.value = '';
    const result = await getAppUserInsightsApi({
      keyword: query.keyword || undefined,
      memberLevel: query.memberLevel || undefined,
      page: query.page,
      pageSize: query.pageSize,
      status: query.status || undefined,
    });
    if (currentRequestId !== requestId) return;
    insights.value = result.items;
    total.value = result.total;
  } catch {
    if (currentRequestId === requestId) {
      insights.value = [];
      total.value = 0;
      loadError.value = '用户提炼数据加载失败，请稍后重试';
    }
  } finally {
    if (currentRequestId === requestId) {
      loading.value = false;
    }
  }
}

function search() {
  query.page = 1;
  load();
}

function handleTableChange(pagination: {
  current?: number;
  pageSize?: number;
}) {
  query.page = pagination.current ?? 1;
  query.pageSize = pagination.pageSize ?? 20;
  load();
}

function openDetail(record: AppUserInsight) {
  detail.value = record;
  detailOpen.value = true;
}

function insightRecord(record: Record<string, any>): AppUserInsight {
  return record as AppUserInsight;
}

watch(
  () => route.query.keyword,
  () => {
    if (applyRouteKeyword()) {
      load();
    }
  },
);

onMounted(() => {
  applyRouteKeyword();
  load();
});
</script>

<template>
  <PageShell
    description="查看 App 用户已沉淀的画像、记忆、对话与合盘摘要。"
    :loading="loading"
    title="用户提炼数据"
    @refresh="load"
  >
    <div class="insights-page">
      <Card :bordered="false" class="filter-card">
        <div class="filter-bar">
          <Input
            v-model:value="query.keyword"
            allow-clear
            class="keyword-input"
            placeholder="搜索手机号 / 昵称"
            @press-enter="search"
          />
          <Select
            v-model:value="query.memberLevel"
            allow-clear
            class="filter-select"
            :options="memberLevelOptions"
            placeholder="会员等级"
          />
          <Select
            v-model:value="query.status"
            allow-clear
            class="filter-select"
            :options="statusOptions"
            placeholder="状态"
          />
          <Space class="filter-actions">
            <Button type="primary" @click="search">查询</Button>
          </Space>
        </div>
      </Card>

      <Card :bordered="false" class="table-card">
        <Alert
          v-if="loadError"
          class="load-error"
          :message="loadError"
          show-icon
          type="error"
        />
        <Table
          :columns="columns"
          :data-source="insights"
          :loading="loading"
          :pagination="{
            current: query.page,
            pageSize: query.pageSize,
            showSizeChanger: true,
            total,
          }"
          :scroll="{ x: 1190 }"
          row-key="id"
          table-layout="fixed"
          @change="handleTableChange"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'nickname'">
              {{ record.nickname || '-' }}
            </template>
            <template v-if="column.dataIndex === 'memberLevel'">
              <Tag>{{ memberLevelLabel(record.memberLevel) }}</Tag>
            </template>
            <template v-if="column.dataIndex === 'primaryType'">
              <Tag :color="record.primaryType ? 'processing' : 'default'">
                {{ enneagramLabel(record.primaryType) }}
              </Tag>
            </template>
            <template v-if="column.dataIndex === 'profile'">
              <div class="summary-cell">
                <span>{{ getProfileSummary(record.profile) }}</span>
                <Tag :color="insightStatusColors[insightStatus(insightRecord(record))]">
                  {{ insightStatus(insightRecord(record)) }}
                </Tag>
              </div>
            </template>
            <template v-if="column.dataIndex === 'memoryCount'">
              {{ record.memoryCount }} 条记忆 / {{ record.cardCount }} 张卡
            </template>
            <template v-if="column.dataIndex === 'messageCount'">
              {{ record.sessionCount }} 会话 / {{ record.messageCount }} 消息
            </template>
            <template v-if="column.dataIndex === 'latestQuizTime'">
              {{ record.latestQuizTime || '-' }}
            </template>
            <template v-if="column.key === 'action'">
              <Button size="small" type="link" @click="openDetail(insightRecord(record))">
                详情
              </Button>
            </template>
          </template>
        </Table>
      </Card>
    </div>

    <Drawer
      v-model:open="detailOpen"
      title="提炼详情"
      width="min(720px, calc(100vw - 32px))"
    >
      <div v-if="detail" class="detail-layout">
        <div class="profile-head">
          <div class="profile-avatar">
            {{ (detail.nickname || detail.phone)?.slice(0, 1) || '客' }}
          </div>
          <div class="profile-main">
            <div class="profile-title-row">
              <h3>{{ detail.nickname || detail.phone }}</h3>
              <Tag :color="statusColors[detail.status] || 'default'">
                {{ statusLabel(detail.status) }}
              </Tag>
              <Tag>{{ memberLevelLabel(detail.memberLevel) }}</Tag>
            </div>
            <p>{{ getProfileSummary(detail.profile) }}</p>
          </div>
        </div>

        <Descriptions :column="1" bordered size="small">
          <Descriptions.Item label="客户 ID">{{ detail.id }}</Descriptions.Item>
          <Descriptions.Item label="手机号">{{ detail.phone }}</Descriptions.Item>
          <Descriptions.Item label="昵称 / 性别">
            {{ detail.nickname || '-' }} / {{ detail.gender || '-' }}
          </Descriptions.Item>
          <Descriptions.Item label="来源 / 会员">
            {{ detail.registerSource || '-' }} / {{ memberLevelLabel(detail.memberLevel) }}
          </Descriptions.Item>
          <Descriptions.Item label="注册 / 最近登录">
            {{ detail.createTime || '-' }} / {{ detail.lastLoginAt || '-' }}
          </Descriptions.Item>
          <Descriptions.Item label="主型 / 副型 / 侧翼">
            {{ enneagramLabel(detail.primaryType) }} /
            {{ enneagramLabel(detail.secondType) }} /
            {{ enneagramLabel(detail.wingType) }}
          </Descriptions.Item>
          <Descriptions.Item label="中心占比">
            {{ getCenterSummary(detail.centers) }}
          </Descriptions.Item>
          <Descriptions.Item label="最近测评">
            {{ detail.latestQuizTime || '-' }}
          </Descriptions.Item>
          <Descriptions.Item label="记忆沉淀">
            {{ detail.memoryCount }} 条，最新：{{ detail.latestMemory || '-' }}
          </Descriptions.Item>
          <Descriptions.Item label="对话沉淀">
            {{ detail.sessionCount }} 个会话，{{ detail.messageCount }} 条消息，最近：
            {{ detail.latestChatTime || '-' }}
          </Descriptions.Item>
          <Descriptions.Item label="合盘摘要">
            {{ detail.compatibilityCount }} 份，最新：
            {{ detail.latestCompatibilitySummary || '-' }}
          </Descriptions.Item>
        </Descriptions>

        <div class="insight-section">
          <h4>优势</h4>
          <div v-if="selectedStrengths.length > 0" class="tag-list">
            <Tag v-for="item in selectedStrengths" :key="item" color="success">
              {{ item }}
            </Tag>
          </div>
          <Empty v-else :image="Empty.PRESENTED_IMAGE_SIMPLE" />
        </div>

        <div class="insight-section">
          <h4>九型分数</h4>
          <div v-if="selectedScoreTags.length > 0" class="tag-list">
            <Tag v-for="item in selectedScoreTags" :key="item">
              {{ item }}
            </Tag>
          </div>
          <Empty v-else :image="Empty.PRESENTED_IMAGE_SIMPLE" />
        </div>

        <div class="insight-section">
          <h4>成长提醒</h4>
          <div v-if="selectedChallenges.length > 0" class="tag-list">
            <Tag v-for="item in selectedChallenges" :key="item" color="warning">
              {{ item }}
            </Tag>
          </div>
          <Empty v-else :image="Empty.PRESENTED_IMAGE_SIMPLE" />
        </div>
      </div>
    </Drawer>
  </PageShell>
</template>

<style scoped>
.insights-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.filter-card :deep(.ant-card-body),
.table-card :deep(.ant-card-body) {
  padding: 16px;
}

.filter-bar {
  display: grid;
  grid-template-columns: minmax(220px, 360px) 160px 140px auto;
  gap: 10px;
  justify-content: start;
}

.keyword-input,
.filter-select {
  width: 100%;
}

.summary-cell {
  display: flex;
  gap: 8px;
  align-items: center;
  min-width: 0;
}

.summary-cell span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.load-error {
  margin-bottom: 12px;
}

.detail-layout {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.profile-head {
  display: flex;
  gap: 14px;
  align-items: center;
  padding: 16px;
  background: hsl(var(--accent) / 32%);
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
}

.profile-avatar {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  font-size: 20px;
  font-weight: 700;
  color: hsl(var(--primary));
  background: hsl(var(--primary) / 12%);
  border: 1px solid hsl(var(--primary) / 20%);
  border-radius: 8px;
}

.profile-main {
  min-width: 0;
}

.profile-title-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.profile-title-row h3 {
  margin: 0;
  font-size: 18px;
  line-height: 26px;
}

.profile-main p {
  margin: 6px 0 0;
  color: hsl(var(--muted-foreground));
}

.insight-section {
  padding: 16px;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
}

.insight-section h4 {
  margin: 0 0 12px;
  font-size: 15px;
  font-weight: 700;
}

.tag-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

@media (max-width: 640px) {
  .filter-bar {
    grid-template-columns: 1fr;
  }

  .filter-actions,
  .filter-actions :deep(.ant-space-item),
  .filter-actions :deep(.ant-btn) {
    width: 100%;
  }

  .summary-cell {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
