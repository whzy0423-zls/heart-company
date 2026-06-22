<script setup lang="ts">
import type { Article, VoiceOption } from '#/api';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { IconifyIcon } from '@vben/icons';

import {
  Button,
  Card,
  Drawer,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  message,
} from 'ant-design-vue';

import {
  createArticleApi,
  deleteArticleApi,
  generateArticleAudioApi,
  getArticlesApi,
  getReadingSettingsApi,
  getVoiceOptionsApi,
  updateArticleApi,
  updateReadingSettingsApi,
} from '#/api';

import ImagePathInput from '#/views/site-config/components/image-path-input.vue';

const loading = ref(false);
const saving = ref(false);
const drawerOpen = ref(false);
const articles = ref<Article[]>([]);
const total = ref(0);

// 听书相关状态。
const voiceOptions = ref<VoiceOption[]>([]);
const defaultVoiceKey = ref('');
const savingDefaultVoice = ref(false);
const generatingId = ref('');

const query = reactive({
  keyword: '',
  page: 1,
  pageSize: 20,
  status: '',
});

const form = reactive({
  author: '',
  category: '',
  content: '',
  cover: '',
  id: '',
  sort: 0,
  status: 'published',
  summary: '',
  tagsText: '',
  title: '',
  voiceKey: '',
});

const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '已发布', value: 'published' },
  { label: '草稿', value: 'draft' },
];

const editStatusOptions = [
  { label: '已发布', value: 'published' },
  { label: '草稿', value: 'draft' },
];

// 文章音色下拉：第一项为“跟随全局默认”。
const voiceSelectOptions = computed(() => [
  { label: '跟随全局默认音色', value: '' },
  ...voiceOptions.value.map((item) => ({ label: item.label, value: item.id })),
]);

// 全局默认音色下拉（必须选一个具体音色）。
const defaultVoiceSelectOptions = computed(() =>
  voiceOptions.value.map((item) => ({ label: item.label, value: item.id })),
);

function voiceLabel(key?: string) {
  if (!key) return '默认';
  return voiceOptions.value.find((item) => item.id === key)?.label ?? key;
}

const columns = [
  { dataIndex: 'title', title: '标题', width: 220 },
  { dataIndex: 'category', title: '分类', width: 100 },
  { dataIndex: 'status', title: '状态', width: 80 },
  { dataIndex: 'audioStatus', title: '听书', width: 150 },
  { dataIndex: 'tags', title: '标签', width: 160 },
  { dataIndex: 'author', title: '作者', width: 100 },
  { dataIndex: 'viewCount', title: '阅读', width: 70 },
  { dataIndex: 'updateTime', title: '更新时间', width: 160 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 250 },
];

const contentChars = computed(() => form.content.length);

const audioStatusMeta: Record<string, { color: string; text: string }> = {
  failed: { color: 'error', text: '生成失败' },
  generating: { color: 'processing', text: '生成中' },
  none: { color: 'default', text: '未生成' },
  ready: { color: 'success', text: '已就绪' },
};

function audioMeta(status?: string) {
  return audioStatusMeta[status || 'none'] ?? { color: 'default', text: '未生成' };
}

async function load() {
  loading.value = true;
  try {
    const result = await getArticlesApi({
      keyword: query.keyword,
      page: query.page,
      pageSize: query.pageSize,
      status: query.status || undefined,
    });
    articles.value = result.items;
    total.value = result.total;
  } finally {
    loading.value = false;
  }
}

async function loadVoiceMeta() {
  try {
    const [options, settings] = await Promise.all([
      getVoiceOptionsApi(),
      getReadingSettingsApi(),
    ]);
    voiceOptions.value = options || [];
    defaultVoiceKey.value = settings?.voiceKey || '';
  } catch {
    // 语音服务未配置时静默降级，不阻断文章管理。
  }
}

async function saveDefaultVoice() {
  savingDefaultVoice.value = true;
  try {
    await updateReadingSettingsApi(defaultVoiceKey.value);
    message.success('全局默认听书音色已保存');
  } finally {
    savingDefaultVoice.value = false;
  }
}

async function generateAudio(record: Article) {
  if (!record.voiceKey && !defaultVoiceKey.value) {
    message.warning('请先为该文章选择音色，或在上方设置全局默认音色');
    return;
  }
  generatingId.value = record.id;
  const hide = message.loading('正在生成听书音频，长文可能需要一会儿…', 0);
  try {
    const updated = await generateArticleAudioApi(record.id);
    const index = articles.value.findIndex((item) => item.id === record.id);
    if (index !== -1) articles.value[index] = updated;
    message.success('听书音频已生成');
  } catch (error: any) {
    message.error(error?.message || '音频生成失败');
  } finally {
    hide();
    generatingId.value = '';
  }
}

function tagsFromText(text: string) {
  return text
    .split(/[,，\n]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function resetForm() {
  form.author = '';
  form.category = '';
  form.content = '';
  form.cover = '';
  form.id = '';
  form.sort = 0;
  form.status = 'published';
  form.summary = '';
  form.tagsText = '';
  form.title = '';
  form.voiceKey = '';
}

function openCreate() {
  resetForm();
  drawerOpen.value = true;
}

function openEdit(record: Article) {
  form.author = record.author || '';
  form.category = record.category || '';
  form.content = record.content || '';
  form.cover = record.cover || '';
  form.id = record.id;
  form.sort = Number(record.sort || 0);
  form.status = record.status || 'published';
  form.summary = record.summary || '';
  form.tagsText = (record.tags || []).join('\n');
  form.title = record.title || '';
  form.voiceKey = record.voiceKey || '';
  drawerOpen.value = true;
}

function asArticle(record: Record<string, any>) {
  return record as Article;
}

async function submit() {
  if (!form.title.trim() || !form.content.trim()) {
    message.warning('请填写标题和正文');
    return;
  }
  saving.value = true;
  try {
    const payload = {
      author: form.author,
      category: form.category,
      content: form.content,
      cover: form.cover,
      sort: form.sort,
      status: form.status,
      summary: form.summary,
      tags: tagsFromText(form.tagsText),
      title: form.title,
      voiceKey: form.voiceKey,
    };
    if (form.id) {
      await updateArticleApi(form.id, payload);
      message.success('文章已更新');
    } else {
      await createArticleApi(payload);
      message.success('文章已发布');
    }
    drawerOpen.value = false;
    await load();
  } finally {
    saving.value = false;
  }
}

async function toggleStatus(record: Article) {
  const nextStatus = record.status === 'published' ? 'draft' : 'published';
  await updateArticleApi(record.id, {
    author: record.author,
    category: record.category,
    content: record.content,
    cover: record.cover,
    sort: record.sort,
    status: nextStatus,
    summary: record.summary,
    tags: record.tags || [],
    title: record.title,
    voiceKey: record.voiceKey,
  });
  message.success(nextStatus === 'published' ? '已发布' : '已转为草稿');
  await load();
}

function removeArticle(record: Article) {
  Modal.confirm({
    content: `确认删除「${record.title}」吗？删除后 H5 阅读页将不再展示这篇文章。`,
    onOk: async () => {
      await deleteArticleApi(record.id);
      message.success('已删除');
      await load();
    },
    title: '删除文章',
  });
}

function statusColor(status: string) {
  return status === 'published' ? 'success' : 'warning';
}

function statusLabel(status: string) {
  return status === 'published' ? '已发布' : '草稿';
}

function search() {
  query.page = 1;
  load();
}

function handleTableChange(pagination: { current?: number; pageSize?: number }) {
  query.page = pagination.current ?? 1;
  query.pageSize = pagination.pageSize ?? 20;
  load();
}

onMounted(() => {
  load();
  loadVoiceMeta();
});
</script>

<template>
  <Page
    description="维护 H5 读书页展示的文章。正文使用 Markdown 编写，发布后会出现在阅读 H5 列表与详情页。"
    title="文章管理"
  >
    <Card :bordered="false" class="article-card">
      <div class="table-head">
        <div>
          <div class="card-title">阅读文章</div>
          <div class="card-desc">
            共 {{ total }} 篇。建议填写摘要与封面，H5 列表展示更美观。
          </div>
        </div>
        <Space wrap>
          <Select
            v-model:value="query.status"
            :options="statusOptions"
            class="status-select"
          />
          <Input
            v-model:value="query.keyword"
            allow-clear
            class="keyword-input"
            placeholder="搜索标题 / 摘要 / 正文"
            @press-enter="search"
          />
          <Button type="primary" @click="search">查询</Button>
          <Button :loading="loading" @click="load">刷新</Button>
          <Button type="primary" @click="openCreate">新增文章</Button>
        </Space>
      </div>

      <div class="voice-panel">
        <div class="voice-panel-main">
          <span class="voice-panel-icon">
            <IconifyIcon icon="lucide:headphones" />
          </span>
          <div class="voice-panel-copy">
            <div class="voice-panel-title">全局默认听书音色</div>
            <div class="voice-panel-desc">
              未单独指定音色的文章会使用该音色生成音频，修改音色或正文后需重新生成。
            </div>
          </div>
        </div>
        <div class="voice-panel-actions">
          <Select
            v-model:value="defaultVoiceKey"
            :options="defaultVoiceSelectOptions"
            class="voice-panel-select"
            placeholder="选择默认音色"
            show-search
            option-filter-prop="label"
          />
          <Button
            :loading="savingDefaultVoice"
            type="primary"
            @click="saveDefaultVoice"
          >
            <IconifyIcon class="mr-1" icon="lucide:save" />
            保存默认音色
          </Button>
        </div>
      </div>

      <Table
        :columns="columns"
        :data-source="articles"
        :loading="loading"
        :pagination="{
          current: query.page,
          pageSize: query.pageSize,
          showSizeChanger: true,
          total,
        }"
        :scroll="{ x: 1320 }"
        row-key="id"
        table-layout="fixed"
        @change="handleTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'status'">
            <Tag :color="statusColor(record.status)">
              {{ statusLabel(record.status) }}
            </Tag>
          </template>
          <template v-else-if="column.dataIndex === 'category'">
            <span>{{ record.category || '-' }}</span>
          </template>
          <template v-else-if="column.dataIndex === 'audioStatus'">
            <Space :size="4" direction="vertical">
              <Tag :color="audioMeta(record.audioStatus).color">
                {{ audioMeta(record.audioStatus).text }}
              </Tag>
              <span class="voice-cell">音色：{{ voiceLabel(record.voiceKey) }}</span>
            </Space>
          </template>
          <template v-else-if="column.dataIndex === 'tags'">
            <Space :size="4" wrap>
              <Tag v-for="tag in record.tags" :key="tag" color="blue">
                {{ tag }}
              </Tag>
              <span v-if="!record.tags?.length">-</span>
            </Space>
          </template>
          <template v-else-if="column.key === 'action'">
            <Space :size="4">
              <Button size="small" type="link" @click="openEdit(asArticle(record))">
                编辑
              </Button>
              <Button
                :loading="generatingId === record.id"
                size="small"
                type="link"
                @click="generateAudio(asArticle(record))"
              >
                {{ record.audioStatus === 'ready' ? '重新生成听书' : '生成听书' }}
              </Button>
              <Button size="small" type="link" @click="toggleStatus(asArticle(record))">
                {{ record.status === 'published' ? '转草稿' : '发布' }}
              </Button>
              <Button danger size="small" type="link" @click="removeArticle(asArticle(record))">
                删除
              </Button>
            </Space>
          </template>
        </template>
      </Table>
    </Card>

    <Drawer
      v-model:open="drawerOpen"
      :title="form.id ? '编辑文章' : '新增文章'"
      width="820px"
    >
      <Form layout="vertical">
        <Form.Item label="标题" required>
          <Input v-model:value="form.title" placeholder="例如：九型人格与亲密关系" />
        </Form.Item>
        <Space align="start" class="form-row">
          <Form.Item label="分类">
            <Input v-model:value="form.category" placeholder="如：成长 / 关系 / 职场" />
          </Form.Item>
          <Form.Item label="作者">
            <Input v-model:value="form.author" placeholder="作者署名" />
          </Form.Item>
        </Space>
        <Form.Item label="封面图">
          <ImagePathInput v-model:value="form.cover" dir="article" />
        </Form.Item>
        <Form.Item label="摘要">
          <Input.TextArea
            v-model:value="form.summary"
            :rows="2"
            placeholder="一句话摘要，用于 H5 列表卡片展示"
          />
        </Form.Item>
        <Form.Item required>
          <template #label>
            正文（Markdown）
            <span class="char-count">{{ contentChars }} 字</span>
          </template>
          <Input.TextArea
            v-model:value="form.content"
            :rows="16"
            class="markdown-area"
            placeholder="支持 Markdown：# 标题、**加粗**、- 列表、> 引用、图片、代码块等"
          />
        </Form.Item>
        <Form.Item label="标签">
          <Input.TextArea
            v-model:value="form.tagsText"
            :rows="2"
            placeholder="一行一个标签，或用逗号分隔"
          />
        </Form.Item>
        <Space align="start">
          <Form.Item label="状态">
            <Select
              v-model:value="form.status"
              :options="editStatusOptions"
              class="drawer-select"
            />
          </Form.Item>
          <Form.Item label="排序">
            <InputNumber v-model:value="form.sort" :min="0" class="sort-input" />
          </Form.Item>
        </Space>
        <Form.Item label="听书音色">
          <Select
            v-model:value="form.voiceKey"
            :options="voiceSelectOptions"
            option-filter-prop="label"
            placeholder="跟随全局默认音色"
            show-search
          />
          <div class="voice-form-hint">
            选择后保存文章，再到列表点「生成听书」即可合成音频；修改音色或正文后需重新生成。
          </div>
        </Form.Item>
        <Space>
          <Button :loading="saving" type="primary" @click="submit">保存</Button>
          <Button @click="drawerOpen = false">取消</Button>
        </Space>
      </Form>
    </Drawer>
  </Page>
</template>

<style scoped>
.article-card {
  border-radius: 8px;
}

.table-head {
  display: flex;
  gap: 16px;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 16px;
}

.card-title {
  font-size: 16px;
  font-weight: 600;
}

.card-desc {
  margin-top: 4px;
  color: #667085;
  font-size: 13px;
}

.keyword-input {
  width: 240px;
}

.voice-panel {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 16px;
  align-items: center;
  padding: 14px 16px;
  margin-bottom: 16px;
  color: hsl(var(--foreground));
  background: hsl(var(--card) / 86%);
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
}

.voice-panel-main {
  display: flex;
  min-width: 0;
  gap: 12px;
  align-items: center;
}

.voice-panel-icon {
  display: inline-flex;
  flex: 0 0 34px;
  width: 34px;
  height: 34px;
  align-items: center;
  justify-content: center;
  color: hsl(var(--primary));
  background: hsl(var(--primary) / 10%);
  border: 1px solid hsl(var(--primary) / 16%);
  border-radius: 8px;
}

.voice-panel-copy {
  min-width: 0;
}

.voice-panel-title {
  font-size: 14px;
  font-weight: 600;
  line-height: 22px;
}

.voice-panel-desc {
  margin-top: 2px;
  overflow: hidden;
  color: hsl(var(--muted-foreground));
  font-size: 12px;
  line-height: 20px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.voice-panel-actions {
  display: flex;
  flex-wrap: nowrap;
  gap: 10px;
  align-items: center;
}

.voice-panel-select {
  width: 280px;
}

.voice-cell {
  color: #98a2b3;
  font-size: 12px;
}

.voice-form-hint {
  margin-top: 6px;
  color: #98a2b3;
  font-size: 12px;
}

.status-select,
.drawer-select {
  width: 130px;
}

.sort-input {
  width: 130px;
}

.form-row :deep(.ant-form-item) {
  width: 240px;
}

.char-count {
  margin-left: 8px;
  color: #98a2b3;
  font-size: 12px;
  font-weight: 400;
}

.markdown-area {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 13px;
}

@media (max-width: 768px) {
  .table-head {
    display: block;
  }

  .table-head :deep(.ant-space) {
    width: 100%;
    margin-top: 12px;
  }

  .keyword-input,
  .status-select {
    width: 100%;
  }

  .voice-panel {
    grid-template-columns: 1fr;
  }

  .voice-panel-actions {
    flex-wrap: wrap;
  }

  .voice-panel-select {
    width: 100%;
  }

  .voice-panel-actions :deep(.ant-btn) {
    width: 100%;
  }

  .voice-panel-desc {
    white-space: normal;
  }
}
</style>
