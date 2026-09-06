<script setup lang="ts">
import type {
  StoryGenerationConfigPayload,
  StoryGenerationPingResult,
  StorySkillAdminItem,
  StorySkillCategory,
} from '#/api';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { IconifyIcon } from '@vben/icons';
import { useAccessStore } from '@vben/stores';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Empty,
  Form,
  Input,
  InputNumber,
  Modal,
  Segmented,
  Select,
  Spin,
  Switch,
  Table,
  Tag,
  Tooltip,
  Upload,
  message,
} from 'ant-design-vue';

import {
  deleteStorySkillApi,
  getAccessCodesApi,
  getStorySkillApi,
  getStorySkillsApi,
  getStoryGenerationConfigApi,
  publishStorySkillApi,
  updateStorySkillApi,
  updateStoryGenerationConfigApi,
  testStoryGenerationConfigApi,
  uploadStorySkillApi,
} from '#/api';

const categories = [
  { label: '全部', value: 'all' },
  { label: '神话', value: 'myth' },
  { label: '民间', value: 'folk' },
  { label: '童话', value: 'fairy_tale' },
  { label: '小说', value: 'novel' },
  { label: '现实', value: 'realistic' },
];
const categoryOptions = categories.slice(1);
const columns = [
  { dataIndex: 'name', title: '技能名称', width: 180 },
  { dataIndex: 'categoryName', title: '故事类型', width: 100 },
  { dataIndex: 'summary', title: '用途' },
  { dataIndex: 'version', title: '版本', width: 90 },
  { dataIndex: 'status', title: '状态', width: 90 },
  { key: 'action', title: '操作', width: 188 },
];
const access = useAccessStore();
const canEdit = computed(() => access.accessCodes.includes('App:StoryManagement:Edit'));
const canDelete = computed(() => access.accessCodes.includes('App:StoryManagement:Delete'));
const canPublish = computed(() => access.accessCodes.includes('App:StoryManagement:Publish'));
const loading = ref(false);
const saving = ref(false);
const error = ref('');
const selectedCategory = ref('all');
const items = ref<StorySkillAdminItem[]>([]);
const editorOpen = ref(false);
const editorMode = ref<'create' | 'edit'>('create');
const detailOpen = ref(false);
const detailLoading = ref(false);
const activeItem = ref<StorySkillAdminItem>();
const actionLoadingId = ref<number>();
const modelLoading = ref(false);
const modelSaving = ref(false);
const modelTesting = ref(false);
const modelKeySet = ref(false);
const modelForm = reactive<StoryGenerationConfigPayload>({
  enabled: false,
  provider: 'openai-compatible',
  apiBase: '',
  apiKey: '',
  model: '',
  temperature: 0.2,
  maxTokens: 5600,
  timeoutSeconds: 90,
  systemPrompt: '',
});
const selectedFile = ref<File>();
const form = reactive({
  category: 'myth' as StorySkillCategory,
  instructions: '',
  key: '',
  name: '',
  summary: '',
  version: '1.0.0',
});
const filteredItems = computed(() => selectedCategory.value === 'all'
  ? items.value
  : items.value.filter((item) => item.category === selectedCategory.value));
const statusLabel: Record<string, string> = { draft: '草稿', enabled: '已发布', published: '已发布', review: '审核中' };
const statusColor: Record<string, string> = { draft: 'orange', enabled: 'green', published: 'green', review: 'blue' };

async function load() {
  loading.value = true;
  error.value = '';
  try { items.value = await getStorySkillsApi(); }
  catch { error.value = '故事技能加载失败，请重试。'; }
  finally { loading.value = false; }
}
async function loadModelConfig() {
  modelLoading.value = true;
  try {
    const data = await getStoryGenerationConfigApi();
    Object.assign(modelForm, {
      enabled: data?.enabled ?? false,
      provider: data?.provider ?? 'openai-compatible',
      apiBase: data?.apiBase ?? '',
      apiKey: '',
      model: data?.model ?? '',
      temperature: data?.temperature ?? 0.2,
      maxTokens: data?.maxTokens ?? 5600,
      timeoutSeconds: data?.timeoutSeconds ?? 90,
      systemPrompt: data?.systemPrompt ?? '',
    });
    modelKeySet.value = Boolean(data?.apiKeySet);
  } catch { message.error('故事生成模型配置加载失败'); }
  finally { modelLoading.value = false; }
}
async function saveModelConfig() {
  modelSaving.value = true;
  try {
    const data = await updateStoryGenerationConfigApi(modelForm);
    Object.assign(modelForm, { ...data, apiKey: '' });
    modelKeySet.value = Boolean(data.apiKeySet);
    message.success('故事生成模型配置已保存');
  } catch { message.error('保存失败，请检查地址、密钥和模型名'); }
  finally { modelSaving.value = false; }
}
async function testModelConfig() {
  modelTesting.value = true;
  try {
    const result: StoryGenerationPingResult = await testStoryGenerationConfigApi(modelForm);
    if (result?.ok) message.success(result.message || '连接正常');
    else message.error(result?.message || '连接测试失败');
  } catch { message.error('连接测试失败，请检查配置'); }
  finally { modelTesting.value = false; }
}
function openUpload() {
  Object.assign(form, { category: 'myth', instructions: '', key: '', name: '', summary: '', version: '1.0.0' });
  selectedFile.value = undefined;
  editorMode.value = 'create';
  activeItem.value = undefined;
  editorOpen.value = true;
}
function beforeUpload(file: File) {
  selectedFile.value = file;
  return false;
}
function nextVersion(version: string) {
  const parts = version.split('.');
  const last = Number(parts.at(-1));
  if (parts.length > 1 && Number.isInteger(last)) {
    parts[parts.length - 1] = String(last + 1);
    return parts.join('.');
  }
  return `${version || '1.0'}.1`;
}
async function view(item: StorySkillAdminItem) {
  detailOpen.value = true;
  detailLoading.value = true;
  activeItem.value = undefined;
  try { activeItem.value = await getStorySkillApi(item.id); }
  catch { message.error('故事技能详情加载失败'); detailOpen.value = false; }
  finally { detailLoading.value = false; }
}
async function edit(item: StorySkillAdminItem) {
  detailLoading.value = true;
  try {
    const detail = await getStorySkillApi(item.id);
    activeItem.value = detail;
    Object.assign(form, {
      category: detail.category,
      instructions: detail.instructions ?? '',
      key: detail.key,
      name: detail.name,
      summary: detail.summary,
      version: nextVersion(detail.version),
    });
    selectedFile.value = undefined;
    editorMode.value = 'edit';
    editorOpen.value = true;
  } catch { message.error('故事技能详情加载失败'); }
  finally { detailLoading.value = false; }
}
async function save() {
  if (!form.key || !form.name || !form.summary || (!selectedFile.value && !form.instructions.trim())) {
    message.warning('请补全信息并上传 SKILL.md 或填写技能规则');
    return;
  }
  saving.value = true;
  try {
    const input = { ...form, file: selectedFile.value };
    if (editorMode.value === 'edit' && activeItem.value) {
      await updateStorySkillApi(activeItem.value.id, input);
      message.success('已保存为新草稿，发布后 App 生效');
    } else {
      await uploadStorySkillApi(input);
      message.success('故事技能已保存为草稿');
    }
    editorOpen.value = false;
    await load();
  } catch { message.error(editorMode.value === 'edit' ? '故事技能保存失败，请检查版本号或技能标识' : '故事技能上传失败'); }
  finally { saving.value = false; }
}
function publish(item: StorySkillAdminItem) {
  Modal.confirm({
    title: `发布“${item.name}”到 App？`,
    content: `发布后，用户选择“${item.categoryName}”类型时可以选择这个技能。`,
    async onOk() {
      actionLoadingId.value = item.id;
      try {
        await publishStorySkillApi(item.id);
        message.success('已发布到 App');
        await load();
      } finally { actionLoadingId.value = undefined; }
    },
  });
}
function remove(item: StorySkillAdminItem) {
  Modal.confirm({
    title: `删除“${item.name}”？`,
    content: '删除后将从后台列表和 App 选择列表中移除，已产生的历史会话仍会保留。',
    okText: '删除 Skill',
    okButtonProps: { danger: true },
    cancelText: '取消',
    async onOk() {
      actionLoadingId.value = item.id;
      try {
        await deleteStorySkillApi(item.id);
        message.success('故事技能已删除');
        await load();
      } finally { actionLoadingId.value = undefined; }
    },
  });
}
onMounted(async () => {
  try { access.setAccessCodes(await getAccessCodesApi()); }
  catch {}
  finally { await Promise.all([load(), loadModelConfig()]); }
});
</script>

<template>
  <Page title="我的故事管理">
    <Alert v-if="error" :message="error" closable type="error" @close="error = ''" />
    <Card class="story-model-card" title="故事生成模型" :loading="modelLoading">
      <template #extra><Tag :color="modelKeySet ? 'green' : 'default'">{{ modelKeySet ? '密钥已配置' : '未配置密钥' }}</Tag></template>
      <Alert class="model-hint" type="info" show-icon message="仅用于“我的故事”生成，不会改变聊天、语音或其他模型。未启用时沿用聊天模型。" />
      <Form layout="vertical">
        <div class="form-grid model-grid">
          <Form.Item label="启用故事专用模型"><Switch v-model:checked="modelForm.enabled" /></Form.Item>
          <Form.Item label="协议"><Select v-model:value="modelForm.provider" :options="[{ label: 'OpenAI 兼容', value: 'openai-compatible' }, { label: 'Anthropic 兼容', value: 'anthropic-compatible' }]" /></Form.Item>
          <Form.Item label="API Base"><Input v-model:value="modelForm.apiBase" placeholder="https://api.example.com/v1" /></Form.Item>
          <Form.Item label="模型名称"><Input v-model:value="modelForm.model" placeholder="例如 gpt-4o-mini" /></Form.Item>
          <Form.Item label="API Key"><Input.Password v-model:value="modelForm.apiKey" :placeholder="modelKeySet ? '留空沿用已配置密钥' : '请输入 API Key'" /></Form.Item>
          <Form.Item label="Temperature"><InputNumber v-model:value="modelForm.temperature" :max="2" :min="0.1" :step="0.1" style="width: 100%" /></Form.Item>
          <Form.Item label="最大输出 Token"><InputNumber v-model:value="modelForm.maxTokens" :max="16000" :min="1000" style="width: 100%" /></Form.Item>
          <Form.Item label="超时（秒）"><InputNumber v-model:value="modelForm.timeoutSeconds" :max="90" :min="1" style="width: 100%" /></Form.Item>
        </div>
        <Form.Item label="系统提示词"><Input.TextArea v-model:value="modelForm.systemPrompt" :auto-size="{ minRows: 3, maxRows: 8 }" placeholder="可选：补充故事生成规则" /></Form.Item>
        <div class="model-actions">
          <Button v-if="canEdit" :loading="modelTesting" @click="testModelConfig">测试连接</Button>
          <Button v-if="canEdit" type="primary" :loading="modelSaving" @click="saveModelConfig">保存配置</Button>
        </div>
      </Form>
    </Card>
    <div class="story-toolbar">
      <Segmented v-model:value="selectedCategory" :options="categories" />
      <Button v-if="canEdit" type="primary" @click="openUpload">
        <IconifyIcon class="mr-1" icon="lucide:upload" />上传 Skill
      </Button>
    </div>
    <Table class="desktop-skill-table" :columns="columns" :data-source="filteredItems" :loading="loading" :pagination="false" row-key="id">
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'name'">
          <div class="skill-name">{{ record.name }}</div>
          <div class="skill-key">{{ record.key }}</div>
        </template>
        <template v-else-if="column.dataIndex === 'status'">
          <Tag :color="statusColor[record.status] || 'default'">{{ statusLabel[record.status] || record.status }}</Tag>
        </template>
        <template v-else-if="column.key === 'action'">
          <div class="skill-actions">
            <Tooltip title="查看详情"><Button aria-label="查看详情" size="small" type="text" @click="view(record as StorySkillAdminItem)"><IconifyIcon icon="lucide:eye" /></Button></Tooltip>
            <Tooltip v-if="canEdit && !record.publishedVersion" title="编辑 Skill"><Button aria-label="编辑 Skill" size="small" type="text" @click="edit(record as StorySkillAdminItem)"><IconifyIcon icon="lucide:pencil" /></Button></Tooltip>
            <Tooltip v-if="canPublish && record.hasDraft" title="发布到 App"><Button aria-label="发布到 App" :loading="actionLoadingId === record.id" size="small" type="text" @click="publish(record as StorySkillAdminItem)"><IconifyIcon icon="lucide:send" /></Button></Tooltip>
            <Tooltip v-if="canDelete" title="删除 Skill"><Button aria-label="删除 Skill" danger :loading="actionLoadingId === record.id" size="small" type="text" @click="remove(record as StorySkillAdminItem)"><IconifyIcon icon="lucide:trash-2" /></Button></Tooltip>
          </div>
        </template>
      </template>
    </Table>
    <div class="mobile-skill-list">
      <div v-for="item in filteredItems" :key="item.id" class="mobile-skill-item">
        <div class="mobile-skill-heading">
          <div>
            <div class="skill-name">{{ item.name }}</div>
            <div class="skill-key">{{ item.key }}</div>
          </div>
          <Tag :color="statusColor[item.status] || 'default'">{{ statusLabel[item.status] || item.status }}</Tag>
        </div>
        <div class="mobile-skill-summary">{{ item.summary }}</div>
        <div class="mobile-skill-meta">
          <span>{{ item.categoryName }}</span>
          <span>v{{ item.version }}</span>
        </div>
        <div class="mobile-skill-actions skill-actions">
          <Button aria-label="查看详情" size="small" type="text" @click="view(item)"><IconifyIcon icon="lucide:eye" />查看</Button>
          <Button v-if="canEdit && !item.publishedVersion" aria-label="编辑 Skill" size="small" type="text" @click="edit(item)"><IconifyIcon icon="lucide:pencil" />编辑</Button>
          <Button v-if="canPublish && item.hasDraft" aria-label="发布到 App" :loading="actionLoadingId === item.id" size="small" type="text" @click="publish(item)"><IconifyIcon icon="lucide:send" />发布</Button>
          <Button v-if="canDelete" aria-label="删除 Skill" danger :loading="actionLoadingId === item.id" size="small" type="text" @click="remove(item)"><IconifyIcon icon="lucide:trash-2" />删除</Button>
        </div>
      </div>
      <Empty v-if="!loading && filteredItems.length === 0" description="暂无故事 Skill" />
    </div>

    <Modal v-model:open="editorOpen" :confirm-loading="saving" :title="editorMode === 'create' ? '上传故事 Skill' : '编辑故事 Skill'" width="680px" @ok="save">
      <Form layout="vertical">
        <Form.Item label="故事类型" required><Select v-model:value="form.category" :options="categoryOptions" /></Form.Item>
        <div class="form-grid">
          <Form.Item label="技能名称" required><Input v-model:value="form.name" placeholder="例如：英雄旅程" /></Form.Item>
          <Form.Item label="技能标识" required><Input v-model:value="form.key" placeholder="hero-journey" /></Form.Item>
        </div>
        <Form.Item label="用途摘要" required><Input v-model:value="form.summary" placeholder="在 App 选择列表中展示" /></Form.Item>
        <Form.Item label="版本"><Input v-model:value="form.version" placeholder="1.0.0" /></Form.Item>
        <Form.Item label="Skill 文件">
          <Upload :before-upload="beforeUpload" :file-list="selectedFile ? [{ name: selectedFile.name, uid: 'story-skill' }] : []" :max-count="1" accept=".md,.txt" @remove="selectedFile = undefined">
            <Button><IconifyIcon class="mr-1" icon="lucide:file-up" />选择 SKILL.md</Button>
          </Upload>
        </Form.Item>
        <Form.Item label="技能规则">
          <Input.TextArea v-model:value="form.instructions" :auto-size="{ minRows: 6, maxRows: 12 }" placeholder="未上传文件时，可直接粘贴蒸馏后的 Skill 规则" />
        </Form.Item>
      </Form>
    </Modal>

    <Modal v-model:open="detailOpen" :footer="null" title="故事 Skill 详情" width="720px">
      <div class="detail-loading"><Spin v-if="detailLoading" /></div>
      <template v-if="activeItem && !detailLoading">
        <Descriptions bordered :column="1" size="small">
          <Descriptions.Item label="技能名称">{{ activeItem.name }}</Descriptions.Item>
          <Descriptions.Item label="技能标识">{{ activeItem.key }}</Descriptions.Item>
          <Descriptions.Item label="故事类型">{{ activeItem.categoryName }}</Descriptions.Item>
          <Descriptions.Item label="当前版本">v{{ activeItem.version }}</Descriptions.Item>
          <Descriptions.Item label="线上版本">{{ activeItem.publishedVersion ? `v${activeItem.publishedVersion}` : '尚未发布' }}</Descriptions.Item>
          <Descriptions.Item label="用途摘要">{{ activeItem.summary }}</Descriptions.Item>
        </Descriptions>
        <div class="detail-rules-title">技能规则</div>
        <pre class="detail-rules">{{ activeItem.instructions }}</pre>
      </template>
    </Modal>
  </Page>
</template>

<style scoped>
.story-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 16px; }
.story-model-card { margin-bottom: 18px; }
.model-hint { margin-bottom: 16px; }
.model-grid { grid-template-columns: repeat(4, minmax(0, 1fr)); }
.model-actions { display: flex; justify-content: flex-end; gap: 10px; }
.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.skill-name { font-weight: 600; }
.skill-key { color: var(--vben-color-text-secondary); font-size: 12px; }
.skill-actions { align-items: center; display: flex; gap: 2px; }
.skill-actions .ant-btn { align-items: center; display: inline-flex; gap: 4px; justify-content: center; }
.detail-loading { display: flex; justify-content: center; min-height: 24px; }
.detail-rules-title { font-size: 14px; font-weight: 600; margin: 18px 0 8px; }
.detail-rules { background: var(--vben-bg-color-deep); border: 1px solid rgb(128 128 128 / 18%); border-radius: 6px; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 13px; line-height: 1.6; margin: 0; max-height: 420px; overflow: auto; padding: 14px; white-space: pre-wrap; word-break: break-word; }
.mobile-skill-list { display: none; }
@media (max-width: 720px) {
  .story-toolbar { align-items: stretch; flex-direction: column; }
  .story-toolbar :deep(.ant-segmented) { overflow-x: auto; }
  .form-grid { grid-template-columns: 1fr; gap: 0; }
  .model-grid { grid-template-columns: 1fr; }
  .desktop-skill-table { display: none; }
  .mobile-skill-list { display: grid; gap: 10px; }
  .mobile-skill-item { border: 1px solid rgb(128 128 128 / 22%); border-radius: 6px; padding: 14px; }
  .mobile-skill-heading { align-items: flex-start; display: flex; gap: 12px; justify-content: space-between; }
  .mobile-skill-summary { color: var(--vben-color-text-secondary); font-size: 13px; line-height: 1.55; margin-top: 10px; }
  .mobile-skill-meta { align-items: center; color: var(--vben-color-text-secondary); display: flex; font-size: 12px; gap: 14px; margin-top: 12px; }
  .mobile-skill-actions { border-top: 1px solid rgb(128 128 128 / 14%); justify-content: flex-end; margin-top: 10px; padding-top: 8px; }
}
</style>
