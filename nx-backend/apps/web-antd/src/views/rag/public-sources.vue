<script setup lang="ts">
import type { PublicKnowledgeSource } from '#/api';

import { computed, onMounted } from 'vue';

import {
  EyeOutlined,
  ReloadOutlined,
  SearchOutlined,
} from '@ant-design/icons-vue';
import {
  Alert,
  Button,
  Drawer,
  Empty,
  Input,
  message,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  Tooltip,
} from 'ant-design-vue';

import {
  getPublicKnowledgeSourceChunksApi,
  getPublicKnowledgeSourcesApi,
  updatePublicKnowledgeSourcesStatusApi,
} from '#/api';
import EllipsisTooltip from '#/components/ellipsis-tooltip/ellipsis-tooltip.vue';
import { ellipsisColumn } from '#/components/ellipsis-tooltip/table';

import { createPublicSourcesState, previewLocator } from './public-sources';

const {
  categories,
  changePage,
  error,
  items,
  load,
  loading,
  pendingIds,
  preview,
  previewError,
  previewItems,
  previewLoading,
  previewOpen,
  previewSource,
  previewTotal,
  query,
  saving,
  search,
  selectedIds,
  setEnabled,
  total,
} = createPublicSourcesState({
  chunks: getPublicKnowledgeSourceChunksApi,
  list: getPublicKnowledgeSourcesApi,
  status: updatePublicKnowledgeSourcesStatusApi,
});

const enabledOptions = [
  { label: '全部启用状态', value: '' },
  { label: '已启用', value: 'enabled' },
  { label: '已停用', value: 'disabled' },
];
const qualityOptions = [
  { label: '全部质量状态', value: '' },
  { label: '待清洗', value: 'pending' },
  { label: '已清洗', value: 'ready' },
  { label: '待复核', value: 'needs_review' },
];
const categoryOptions = computed(() => [
  { label: '全部分类', value: '' },
  ...categories.value.map((category) => ({
    label: `${category.name} (${category.count})`,
    value: category.name,
  })),
]);
const columns = [
  ellipsisColumn('title', '书籍 / 素材', { width: 240 }),
  ellipsisColumn('category', '分类', { width: 160 }),
  { dataIndex: 'sourceKind', title: '类型', width: 80 },
  { dataIndex: 'enabled', title: '启用状态', width: 110 },
  { dataIndex: 'qualityStatus', title: '质量', width: 100 },
  { dataIndex: 'importedChunks', title: '导入 / 原片段', width: 140 },
  ellipsisColumn('extractStatus', '提取状态', { width: 120 }),
  ellipsisColumn('datasetId', '数据集', { width: 140 }),
  { fixed: 'right' as const, key: 'action', title: '正文', width: 72 },
];
const rowSelection = computed(() => ({
  getCheckboxProps: () => ({ disabled: loading.value || saving.value }),
  onChange: (keys: (number | string)[]) => {
    selectedIds.value = keys.map(String);
  },
  selectedRowKeys: selectedIds.value,
}));

function qualityLabel(status: string) {
  return (
    qualityOptions.find((option) => option.value === status)?.label ?? status
  );
}

function qualityColor(status: string) {
  return status === 'ready'
    ? 'success'
    : status === 'needs_review'
      ? 'warning'
      : 'default';
}

function asSource(record: Record<string, any>) {
  return record as PublicKnowledgeSource;
}

async function updateStatus(ids: string[], enabled: boolean) {
  if (await setEnabled(ids, enabled))
    message.success(enabled ? '已启用所选资料' : '已停用所选资料');
}

function retryPreview() {
  if (previewSource.value) void preview(previewSource.value);
}

onMounted(load);
</script>

<template>
  <section class="public-sources">
    <div class="directory-toolbar">
      <Input
        v-model:value="query.keyword"
        allow-clear
        class="keyword-filter"
        placeholder="搜索书名 / 素材名称"
        aria-label="书名或素材名称"
        @press-enter="search"
      />
      <Select
        v-model:value="query.category"
        :options="categoryOptions"
        class="category-filter"
        show-search
        option-filter-prop="label"
        placeholder="请选择分类"
        aria-label="分类"
        @change="search"
      />
      <Select
        v-model:value="query.enabled"
        :options="enabledOptions"
        class="state-filter"
        placeholder="请选择启用状态"
        aria-label="启用状态"
        @change="search"
      />
      <Select
        v-model:value="query.qualityStatus"
        :options="qualityOptions"
        class="state-filter"
        placeholder="请选择质量状态"
        aria-label="质量状态"
        @change="search"
      />
      <Input
        v-model:value="query.datasetId"
        allow-clear
        class="dataset-filter"
        placeholder="数据集"
        aria-label="数据集"
        @press-enter="search"
      />
      <Button type="primary" :loading="loading" @click="search"
        ><template #icon><SearchOutlined /></template>查询</Button
      >
      <Tooltip title="刷新目录"
        ><Button
          :disabled="loading || saving"
          aria-label="刷新目录"
          @click="load"
          ><template #icon><ReloadOutlined /></template></Button
      ></Tooltip>
    </div>

    <Alert
      v-if="error"
      :message="error"
      type="error"
      show-icon
      class="directory-alert"
    >
      <template #action
        ><Button size="small" :disabled="loading || saving" @click="load"
          >重试</Button
        ></template
      >
    </Alert>

    <div class="selection-toolbar">
      <span class="directory-count"
        >共 {{ total.toLocaleString() }} 个来源<span v-if="selectedIds.length"
          >，已选 {{ selectedIds.length }} 个</span
        ></span
      >
      <Space wrap>
        <Button
          :disabled="!selectedIds.length || loading || saving"
          :loading="saving && pendingIds.length > 1"
          @click="updateStatus(selectedIds, true)"
          >批量启用</Button
        >
        <Button
          :disabled="!selectedIds.length || loading || saving"
          @click="updateStatus(selectedIds, false)"
          >批量停用</Button
        >
      </Space>
    </div>

    <Table
      :columns="columns"
      :data-source="items"
      :loading="loading"
      :row-selection="rowSelection"
      :pagination="{
        current: query.page,
        pageSize: query.pageSize,
        showSizeChanger: true,
        total,
      }"
      :scroll="{ x: 1280 }"
      row-key="id"
      table-layout="fixed"
      @change="changePage"
    >
      <template #emptyText
        ><Empty :description="error ? '目录加载失败' : '暂无匹配来源'"
      /></template>
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'sourceKind'">{{
          record.sourceKind === 'story' ? '故事' : '书籍'
        }}</template>
        <template v-else-if="column.dataIndex === 'enabled'">
          <Switch
            :checked="record.enabled"
            checked-children="启用"
            un-checked-children="停用"
            :loading="pendingIds.includes(record.id)"
            :disabled="saving || loading"
            :aria-label="`${record.title}启用状态`"
            @change="updateStatus([record.id], Boolean($event))"
          />
        </template>
        <template v-else-if="column.dataIndex === 'qualityStatus'"
          ><Tag :color="qualityColor(record.qualityStatus)">{{
            qualityLabel(record.qualityStatus)
          }}</Tag></template
        >
        <template v-else-if="column.dataIndex === 'importedChunks'"
          >{{ record.importedChunks.toLocaleString() }} /
          {{ record.sourceChunks.toLocaleString() }}</template
        >
        <template v-else-if="column.key === 'action'">
          <Tooltip title="正文预览"
            ><Button
              type="text"
              :aria-label="`预览${record.title}`"
              @click="preview(asSource(record))"
              ><template #icon><EyeOutlined /></template></Button
          ></Tooltip>
        </template>
      </template>
    </Table>

    <Drawer
      v-model:open="previewOpen"
      :title="previewSource?.title ?? '正文预览'"
      width="min(760px, calc(100vw - 24px))"
    >
      <div v-if="previewSource" class="preview-meta">
        <Tag>{{ previewSource.category || '未分类' }}</Tag>
        <Tag :color="qualityColor(previewSource.qualityStatus)">{{
          qualityLabel(previewSource.qualityStatus)
        }}</Tag>
        <span
          >{{ previewSource.enabled ? '已启用' : '已停用' }} · 已导入
          {{ previewSource.importedChunks.toLocaleString() }} 个片段</span
        >
      </div>
      <Alert v-if="previewError" :message="previewError" type="error" show-icon>
        <template #action
          ><Button size="small" @click="retryPreview">重试</Button></template
        >
      </Alert>
      <Spin :spinning="previewLoading">
        <div class="preview-body">
          <div v-if="previewItems.length" class="preview-count">
            片段预览 {{ previewItems.length }} /
            {{ previewTotal.toLocaleString() }}
          </div>
          <article
            v-for="chunk in previewItems"
            :key="chunk.id"
            class="preview-chunk"
          >
            <h3><EllipsisTooltip :text="chunk.title" :lines="2" /></h3>
            <div v-if="previewLocator(chunk.locator)" class="chunk-locator">
              {{ previewLocator(chunk.locator) }}
            </div>
            <div class="chunk-content">{{ chunk.content }}</div>
            <div class="chunk-batch">批次：{{ chunk.importBatchId }}</div>
          </article>
          <Empty
            v-if="!previewLoading && !previewError && !previewItems.length"
            description="尚无已导入正文"
          />
        </div>
      </Spin>
    </Drawer>
  </section>
</template>

<style scoped>
.public-sources {
  min-width: 0;
}
.directory-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 16px;
}
.keyword-filter {
  width: 220px;
}
.category-filter {
  width: 190px;
}
.state-filter {
  width: 145px;
}
.dataset-filter {
  width: 160px;
}
.selection-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.directory-count {
  font-size: 13px;
  color: var(--ant-color-text-secondary, #667085);
}
.directory-alert {
  margin-bottom: 12px;
}
.preview-meta {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
  margin-bottom: 16px;
  font-size: 13px;
}
.preview-body {
  min-height: 160px;
}
.preview-count,
.chunk-locator,
.chunk-batch {
  font-size: 12px;
  color: var(--ant-color-text-secondary, #667085);
  overflow-wrap: anywhere;
}
.preview-chunk {
  padding: 20px 0;
  border-bottom: 1px solid var(--ant-color-border-secondary, #eaecf0);
}
.preview-chunk h3 {
  font-size: 15px;
  font-weight: 600;
  margin-bottom: 8px;
}
.chunk-content {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-size: 14px;
  line-height: 1.8;
  margin: 12px 0;
}
@media (max-width: 768px) {
  .keyword-filter,
  .category-filter,
  .dataset-filter {
    width: 100%;
  }
  .state-filter {
    flex: 1;
    min-width: 135px;
  }
}
</style>
