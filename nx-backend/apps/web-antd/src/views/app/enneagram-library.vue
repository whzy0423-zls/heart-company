<script setup lang="ts">
import type {
  EnneagramTypeDetail,
  EnneagramTypeItem,
  EnneagramTypeSummary,
  EnneagramVersion,
} from '#/api';

import { computed, onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useAccessStore } from '@vben/stores';
import {
  Alert,
  Button,
  Card,
  Col,
  Drawer,
  Empty,
  Input,
  Modal,
  Row,
  Space,
  Table,
  Tabs,
  Tag,
  message,
} from 'ant-design-vue';

import {
  approveEnneagramTypeApi,
  getEnneagramTypeDetailApi,
  getEnneagramTypesApi,
  getEnneagramVersionsApi,
  previewEnneagramTypeApi,
  publishEnneagramTypeApi,
  rollbackEnneagramTypeApi,
  saveEnneagramDraftApi,
  submitEnneagramReviewApi,
} from '#/api';

const access = useAccessStore();
const canEdit = computed(() => access.accessCodes.includes('App:EnneagramLibrary:Edit'));
const canReview = computed(() => access.accessCodes.includes('App:EnneagramLibrary:Review'));
const canPublish = computed(() => access.accessCodes.includes('App:EnneagramLibrary:Publish'));
const loading = ref(false);
const actionLoading = ref(false);
const error = ref('');
const summaries = ref<EnneagramTypeSummary[]>([]);
const selectedType = ref(1);
const detail = ref<EnneagramTypeDetail>();
const drawerOpen = ref(false);
const draftTitle = ref('');
const draftChapter = ref('');
const draftItems = ref<Array<{ contentKey: string; text: string }>>([]);
const query = ref('');
const preview = ref<{ contentKey: string; dimension: string; score: number; text: string }[]>([]);
const versions = ref<EnneagramVersion[]>([]);

const dimensions: Record<string, string> = {
  core_motivation_and_fear: '核心动机与恐惧',
  strengths: '优势',
  risks: '风险',
  formation_factors: '形成因素',
  relationships: '关系',
  workplace: '工作场景',
  stress_and_defenses: '压力与防御',
  growth_practices: '成长练习',
};
const itemColumns = [
  { dataIndex: 'dimension', title: '内容维度', width: 150 },
  { dataIndex: 'contentKey', title: '条目标识', width: 220 },
  { dataIndex: 'text', title: '内容' },
  { dataIndex: 'sourcePages', title: '来源页', width: 120 },
];
const versionColumns = [
  { dataIndex: 'version', title: '版本', width: 80 },
  { dataIndex: 'status', title: '状态', width: 100 },
  { dataIndex: 'cardCount', title: '卡片', width: 80 },
  { dataIndex: 'activatedAt', title: '发布时间' },
  { key: 'action', title: '操作', width: 100 },
];
const currentSummary = computed(() => summaries.value.find((item) => item.type === selectedType.value));
const groupedItems = computed(() => detail.value?.items ?? []);
const statusText: Record<string, string> = {
  missing: '未导入', draft: '草稿', in_review: '审核中', approved: '已通过', published: '已发布',
};
const statusColor: Record<string, string> = { missing: 'default', draft: 'orange', in_review: 'blue', approved: 'cyan', published: 'green' };

function formatStatus(status?: string) { return statusText[status || ''] || status || '-'; }
function loadDetail() {
  return getEnneagramTypeDetailApi(selectedType.value).then((result) => {
    detail.value = result;
    draftTitle.value = result.summary.title;
    draftChapter.value = result.sourceChapter;
    draftItems.value = result.items.map((item) => ({ contentKey: item.contentKey, text: item.text }));
  });
}
async function load() {
  loading.value = true;
  error.value = '';
  try {
    summaries.value = await getEnneagramTypesApi();
    try {
      await loadDetail();
    } catch {
      detail.value = undefined;
    }
  } catch {
    error.value = '九型人格库加载失败，请重试。';
  } finally { loading.value = false; }
}
async function selectType(type: number) {
  selectedType.value = type;
  loading.value = true;
  try { await loadDetail(); } catch { detail.value = undefined; message.error('型号内容加载失败'); } finally { loading.value = false; }
}
function openEditor() { drawerOpen.value = true; }
async function saveDraft() {
  if (!detail.value) return;
  actionLoading.value = true;
  try {
    detail.value = await saveEnneagramDraftApi(selectedType.value, {
      contentDigest: detail.value.contentDigest,
      items: draftItems.value,
      sourceChapter: draftChapter.value,
      title: draftTitle.value,
    });
    drawerOpen.value = false;
    message.success('草稿已保存');
    await load();
  } catch { message.error('草稿保存失败'); } finally { actionLoading.value = false; }
}
function confirmAction(title: string, action: () => Promise<unknown>) {
  Modal.confirm({ title, async onOk() { actionLoading.value = true; try { await action(); message.success('操作已完成'); await load(); } catch { message.error('操作失败'); } finally { actionLoading.value = false; } } });
}
function submitReview() { confirmAction('提交当前型号审核？', () => submitEnneagramReviewApi(selectedType.value)); }
function approve() { confirmAction('审核通过当前型号？', () => approveEnneagramTypeApi(selectedType.value)); }
function publish() { confirmAction('发布当前型号新版本？', () => publishEnneagramTypeApi(selectedType.value)); }
async function runPreview() {
  try { const result = await previewEnneagramTypeApi(selectedType.value, query.value); preview.value = result.hits; } catch { message.error('预览失败'); }
}
async function loadVersions() {
  try { versions.value = await getEnneagramVersionsApi(selectedType.value); } catch { message.error('历史版本加载失败'); }
}
function rollback(version: number) {
  confirmAction(`回滚到 v${version} 并生成新版本？`, () => rollbackEnneagramTypeApi(selectedType.value, version));
}
function sourcePages(item: any) { return (item.sourcePages || []).map((page: EnneagramTypeItem['sourcePages'][number]) => `${page.sourceId} p${page.pageNumber}`).join(', '); }

onMounted(load);
</script>

<template>
  <Page description="按型号管理九型人格理论内容、来源和发布版本。" title="九型人格库">
    <Alert v-if="error" :message="error" closable type="error" @close="error = ''" />
    <Card :loading="loading" class="mb-4">
      <Tabs :active-key="String(selectedType)" type="card" @change="(key) => selectType(Number(key))">
        <Tabs.TabPane v-for="type in 9" :key="String(type)" :tab="`${type}号`">
          <Space>
            <Tag :color="statusColor[currentSummary?.status || 'missing']">{{ formatStatus(currentSummary?.status) }}</Tag>
            <span>v{{ currentSummary?.currentVersion || 0 }}</span>
            <span>{{ currentSummary?.itemCount || 0 }} 条内容</span>
          </Space>
        </Tabs.TabPane>
      </Tabs>
    </Card>

    <Card v-if="detail" :loading="loading" class="mb-4">
      <template #title>{{ detail.summary.title }}</template>
      <template #extra>
        <Space>
          <Button v-if="canEdit && detail.summary.status === 'draft'" :loading="actionLoading" @click="openEditor">编辑草稿</Button>
          <Button v-if="canEdit && detail.summary.status === 'draft'" :loading="actionLoading" @click="submitReview">提交审核</Button>
          <Button v-if="canReview && detail.summary.status === 'in_review'" :loading="actionLoading" @click="approve">审核通过</Button>
          <Button v-if="canPublish && detail.summary.status === 'approved'" :loading="actionLoading" type="primary" @click="publish">发布</Button>
        </Space>
      </template>
      <Row :gutter="16" class="mb-4">
        <Col :md="6" :xs="12"><b>状态</b><div><Tag :color="statusColor[detail.summary.status]">{{ formatStatus(detail.summary.status) }}</Tag></div></Col>
        <Col :md="6" :xs="12"><b>当前版本</b><div>v{{ detail.summary.currentVersion }}</div></Col>
        <Col :md="6" :xs="12"><b>来源章节</b><div>{{ detail.sourceChapter }}</div></Col>
        <Col :md="6" :xs="12"><b>内容摘要</b><div>{{ detail.contentDigest.slice(0, 12) }}...</div></Col>
      </Row>
      <Table :columns="itemColumns" :data-source="groupedItems" :pagination="{ pageSize: 12 }" :scroll="{ x: 850 }" row-key="contentKey">
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'dimension'">{{ dimensions[record.dimension] || record.dimension }}</template>
          <template v-else-if="column.dataIndex === 'sourcePages'">{{ sourcePages(record) || '项目规则' }}</template>
        </template>
      </Table>
    </Card>
    <Card v-else :loading="loading"><Empty description="该型号尚未导入" /></Card>

    <Card class="mb-4" title="型号检索预览">
      <Space.Compact block><Input v-model:value="query" placeholder="输入一个模拟问题" @press-enter="runPreview" /><Button type="primary" @click="runPreview">预览</Button></Space.Compact>
      <Table v-if="preview.length" :columns="itemColumns.slice(0, 3)" :data-source="preview" :pagination="false" class="mt-4" row-key="contentKey" />
      <Empty v-else class="mt-4" description="输入问题查看当前型号命中内容" />
    </Card>

    <Card title="历史版本">
      <template #extra><Button @click="loadVersions">刷新版本</Button></template>
      <Table :columns="versionColumns" :data-source="versions" :pagination="false" row-key="releaseId">
        <template #bodyCell="{ column, record }">
          <Tag v-if="column.dataIndex === 'status'" :color="record.status === 'active' ? 'green' : 'default'">{{ record.status }}</Tag>
          <Button v-if="column.key === 'action' && canPublish && record.status !== 'active'" size="small" @click="rollback(record.version)">回滚</Button>
        </template>
      </Table>
    </Card>

    <Drawer v-model:open="drawerOpen" :width="680" title="编辑九型人格库草稿">
      <Input v-model:value="draftTitle" class="mb-3" placeholder="标题" />
      <Input v-model:value="draftChapter" class="mb-3" placeholder="来源章节" />
      <div v-for="item in draftItems" :key="item.contentKey" class="mb-4">
        <div class="mb-1 text-sm text-gray-500">{{ item.contentKey }}</div>
        <Input.TextArea v-model:value="item.text" :auto-size="{ minRows: 3, maxRows: 8 }" />
      </div>
      <template #footer><Button :loading="actionLoading" type="primary" @click="saveDraft">保存草稿</Button></template>
    </Drawer>
  </Page>
</template>
