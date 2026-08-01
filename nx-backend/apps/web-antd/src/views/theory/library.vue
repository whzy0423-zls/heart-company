<script setup lang="ts">
import type { TheoryLibraryCard, TheoryLibrarySummary } from '#/api';

import { computed, onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import { Button, Card, Col, message, Modal, Row, Space, Statistic, Table, Tag } from 'ant-design-vue';

import {
  getTheoryLibrariesApi,
  getTheoryLibraryCardsApi,
  publishTheoryLibraryApi,
} from '#/api';

const loading = ref(false);
const publishing = ref(false);
const libraries = ref<TheoryLibrarySummary[]>([]);
const cards = ref<TheoryLibraryCard[]>([]);
const selectedId = ref(0);

const current = computed(() => libraries.value.find((item) => item.id === selectedId.value));

const columns = [
  { dataIndex: 'canonicalName', title: '理论卡片', width: 200 },
  { dataIndex: 'domain', title: '领域', width: 150 },
  { dataIndex: 'cardKind', title: '类型', width: 110 },
  { dataIndex: 'summary', title: '内容摘要' },
  { dataIndex: 'status', title: '状态', width: 100 },
  { dataIndex: 'version', title: '版本', width: 80 },
];

function statusText(status?: string) {
  return { active: '已激活', draft: '草稿', enabled: '已启用', published: '已发布' }[status || ''] || status || '-';
}

async function loadDashboard() {
  loading.value = true;
  try {
    const result = await getTheoryLibrariesApi();
    libraries.value = result.libraries;
    if (!selectedId.value && result.libraries.length) selectedId.value = result.libraries[0]!.id;
    if (selectedId.value) cards.value = await getTheoryLibraryCardsApi(selectedId.value);
  } finally {
    loading.value = false;
  }
}

function publish() {
  if (!current.value) return;
  Modal.confirm({
    content: '系统会根据当前理论卡片生成可检索内容，并立即激活新版本。',
    okText: '生成并发布',
    onOk: async () => {
      publishing.value = true;
      try {
        const result = await publishTheoryLibraryApi(current.value!.id);
        message.success(`理论库 v${result.releaseVersion} 已激活，共 ${result.chunkCount} 条可检索内容`);
        await loadDashboard();
      } finally {
        publishing.value = false;
      }
    },
    title: `发布「${current.value.name}」`,
  });
}

onMounted(loadDashboard);
</script>

<template>
  <Page description="管理芯之力使用的正式理论卡片与检索版本。" title="理论库管理">
    <Card :loading="loading">
      <template #title>
        <Space>
          <span>{{ current?.name || '理论库' }}</span>
          <Tag :color="current?.activeReleaseId ? 'green' : 'orange'">
            {{ current?.activeReleaseId ? '检索已生效' : '尚未发布' }}
          </Tag>
        </Space>
      </template>
      <template #extra>
        <Button :loading="publishing" type="primary" @click="publish">生成并发布</Button>
      </template>

      <Row :gutter="16">
        <Col :md="6" :xs="12"><Statistic title="理论卡片" :value="current?.cardCount || 0" /></Col>
        <Col :md="6" :xs="12"><Statistic title="已发布卡片" :value="current?.publishedCards || 0" /></Col>
        <Col :md="6" :xs="12"><Statistic title="可检索内容" :value="current?.chunkCount || 0" /></Col>
        <Col :md="6" :xs="12"><Statistic title="当前版本" :value="current?.currentVersion || 0" /></Col>
      </Row>

      <div class="mt-5 mb-3 text-sm text-gray-500">
        状态：{{ statusText(current?.status) }}；生成并发布后，芯之力会从当前激活版本检索理论内容。
      </div>

      <Table :columns="columns" :data-source="cards" :loading="loading" :pagination="{ pageSize: 20 }" row-key="id">
        <template #bodyCell="{ column, record }">
          <Tag v-if="column.dataIndex === 'status'" :color="record.status === 'published' ? 'green' : 'orange'">
            {{ statusText(record.status) }}
          </Tag>
        </template>
      </Table>
    </Card>
  </Page>
</template>
