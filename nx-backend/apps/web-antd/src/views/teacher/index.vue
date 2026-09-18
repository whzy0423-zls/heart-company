<script setup lang="ts">
import type {
  TeacherContactConfig,
  TeacherFeedType,
  TeacherOfflineConfig,
  TeacherProfile,
  TeacherReviewItem,
  TeacherReviewStatus,
} from '#/api/core/teacher';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useAccessStore } from '@vben/stores';
import {
  Alert,
  Button,
  Card,
  Col,
  Form,
  Input,
  InputNumber,
  Modal,
  Row,
  Select,
  Space,
  Switch,
  Table,
  Tabs,
  Tag,
  Textarea,
  message,
} from 'ant-design-vue';

import {
  approveTeacherReviewApi,
  bindTeacherUserApi,
  createTeacherApi,
  getTeacherReviewQueueApi,
  getTeachersApi,
  offlineTeacherReviewApi,
  rejectTeacherReviewApi,
  setTeacherEnabledApi,
  updateTeacherApi,
} from '#/api/core/teacher';
import ImagePathInput from '../site-config/components/image-path-input.vue';

type TeacherDraft = TeacherProfile & { appUserIdInput?: number };

const accessStore = useAccessStore();
const canWrite = computed(() => accessStore.accessCodes.includes('Teacher:Write'));
const canReview = computed(() => accessStore.accessCodes.includes('Teacher:Review'));
const activeTab = ref('profiles');
const loading = ref(false);
const reviewLoading = ref(false);
const error = ref('');
const reviewError = ref('');
const teachers = ref<TeacherProfile[]>([]);
const reviews = ref<TeacherReviewItem[]>([]);
const teacherTotal = ref(0);
const reviewTotal = ref(0);
const profileModalOpen = ref(false);
const bindingModalOpen = ref(false);
const rejectModalOpen = ref(false);
const saving = ref(false);
const actionLoadingId = ref<number>();
const editingKey = ref<string>();
const bindingKey = ref<string>();
const rejectTarget = ref<TeacherReviewItem>();
const reviewReason = ref('');
const query = reactive({ page: 1, pageSize: 20, keyword: '', enabled: undefined as string | undefined });
const reviewQuery = reactive({ page: 1, pageSize: 20, teacherKey: undefined as string | undefined, feedType: undefined as TeacherFeedType | undefined, reviewStatus: 'pending_review' as TeacherReviewStatus | undefined });

function blankDraft(): TeacherDraft {
  return {
    appUserId: undefined,
    appUserIdInput: undefined,
    avatar: '',
    bio: '',
    cover: '',
    customerServiceConfig: {},
    customerService: {},
    detailIntro: '',
    enabled: true,
    experience: '',
    introVideoUrl: '',
    key: '',
    name: '',
    offlineServiceConfig: { enabled: false, types: [] },
    offlineService: { enabled: false, types: [] },
    shortIntro: '',
    showInDrawer: true,
    showOnHome: true,
    sortOrder: 0,
    tags: [],
    expertise: [],
    title: '',
  };
}

const form = reactive<TeacherDraft>(blankDraft());
const reviewStatusOptions = [
  { label: '待审核', value: 'pending_review' },
  { label: '已退回', value: 'rejected' },
  { label: '已发布', value: 'published' },
  { label: '已下架', value: 'offline' },
  { label: '草稿', value: 'draft' },
];
const statusLabel: Record<string, string> = {
  draft: '草稿',
  offline: '已下架',
  pending_review: '待审核',
  published: '已发布',
  rejected: '已退回',
};
const statusColor: Record<string, string> = {
  draft: 'default',
  offline: 'red',
  pending_review: 'blue',
  published: 'green',
  rejected: 'orange',
};
const profileColumns = [
  { dataIndex: 'name', title: '老师' },
  { dataIndex: 'key', title: '老师标识' },
  { dataIndex: 'title', title: '身份' },
  { dataIndex: 'appUserId', title: '绑定 App 用户' },
  { dataIndex: 'enabled', title: '状态' },
  { dataIndex: 'sortOrder', title: '排序' },
  { key: 'action', title: '操作', width: 220 },
];
const reviewColumns = [
  { dataIndex: 'title', title: '内容' },
  { dataIndex: 'teacherName', title: '老师' },
  { dataIndex: 'feedType', title: '内容类型' },
  { dataIndex: 'reviewStatus', title: '审核状态' },
  { dataIndex: 'reviewReason', title: '退回原因' },
  { key: 'action', title: '操作', width: 220 },
];

function profileRecord(record: Record<string, unknown>) {
  return record as unknown as TeacherProfile;
}
function reviewRecord(record: Record<string, unknown>) {
  return record as unknown as TeacherReviewItem;
}
function configValue<T extends TeacherContactConfig | TeacherOfflineConfig>(config: T | undefined) {
  return config ?? ({} as T);
}
function splitTags(value: string | undefined) {
  return (value ?? '').split(/[,，\n]/).map((item) => item.trim()).filter(Boolean);
}
function teacherPayload() {
  const { appUserIdInput: _unused, ...payload } = form;
  return {
    ...payload,
    customerService: form.customerServiceConfig,
    detailIntro: form.bio,
    expertise: [...form.tags],
    offlineService: form.offlineServiceConfig,
  };
}

async function loadTeachers() {
  loading.value = true;
  error.value = '';
  try {
    const result = await getTeachersApi({
      ...query,
      enabled: query.enabled === undefined ? undefined : query.enabled === 'true',
    });
    teachers.value = result.items;
    teacherTotal.value = result.total;
  } catch {
    error.value = '老师资料加载失败，请重试。';
  } finally {
    loading.value = false;
  }
}
async function loadReviews() {
  reviewLoading.value = true;
  reviewError.value = '';
  try {
    const result = await getTeacherReviewQueueApi(reviewQuery);
    reviews.value = result.items;
    reviewTotal.value = result.total;
  } catch {
    reviewError.value = '审核队列加载失败，请重试。';
  } finally {
    reviewLoading.value = false;
  }
}
async function load() {
  await Promise.all([loadTeachers(), loadReviews()]);
}
function openCreate() {
  Object.assign(form, blankDraft());
  editingKey.value = undefined;
  profileModalOpen.value = true;
}
function openEdit(record: TeacherProfile) {
  Object.assign(form, blankDraft(), record, {
    customerServiceConfig: { ...configValue(record.customerService ?? record.customerServiceConfig) },
    customerService: { ...configValue(record.customerService ?? record.customerServiceConfig) },
    offlineServiceConfig: { ...configValue(record.offlineService ?? record.offlineServiceConfig) },
    offlineService: { ...configValue(record.offlineService ?? record.offlineServiceConfig) },
    tags: [...(record.expertise ?? record.tags ?? [])],
    expertise: [...(record.expertise ?? record.tags ?? [])],
    bio: record.detailIntro ?? record.bio ?? '',
  });
  editingKey.value = record.key;
  profileModalOpen.value = true;
}
async function saveProfile() {
  if (!form.key.trim() || !form.name.trim()) {
    message.warning('请填写老师标识和姓名');
    return;
  }
  saving.value = true;
  try {
    if (editingKey.value) await updateTeacherApi(editingKey.value, teacherPayload());
    else await createTeacherApi(teacherPayload() as any);
    message.success('老师资料已保存，管理员修改优先');
    profileModalOpen.value = false;
    await loadTeachers();
  } finally {
    saving.value = false;
  }
}
function openBinding(record: TeacherProfile) {
  bindingKey.value = record.key;
  form.appUserIdInput = record.appUserId;
  bindingModalOpen.value = true;
}
async function saveBinding() {
  if (!bindingKey.value || !form.appUserIdInput) {
    message.warning('请输入 App 用户 ID');
    return;
  }
  saving.value = true;
  try {
    await bindTeacherUserApi(bindingKey.value, { appUserId: form.appUserIdInput, enabled: true });
    message.success('已绑定 App 用户为老师');
    bindingModalOpen.value = false;
    await loadTeachers();
  } finally {
    saving.value = false;
  }
}
async function toggleEnabled(record: TeacherProfile) {
  await setTeacherEnabledApi(record.key, !record.enabled);
  message.success(record.enabled ? '老师已停用' : '老师已启用');
  await loadTeachers();
}
async function reviewAction(record: TeacherReviewItem, action: 'approve' | 'offline') {
  actionLoadingId.value = record.id;
  try {
    if (action === 'approve') await approveTeacherReviewApi(record.id, { expectedUpdatedAt: record.updatedAt });
    else await offlineTeacherReviewApi(record.id, { reason: '管理员下架' });
    message.success(action === 'approve' ? '审核已通过并发布' : '内容已下架');
    await loadReviews();
  } finally {
    actionLoadingId.value = undefined;
  }
}
function openReject(record: TeacherReviewItem) {
  rejectTarget.value = record;
  reviewReason.value = '';
  rejectModalOpen.value = true;
}
async function rejectReview() {
  if (!rejectTarget.value || !reviewReason.value.trim()) {
    message.warning('退回必须填写退回原因');
    return;
  }
  actionLoadingId.value = rejectTarget.value.id;
  try {
    await rejectTeacherReviewApi(rejectTarget.value.id, { reason: reviewReason.value.trim() });
    message.success('内容已退回老师修改');
    rejectModalOpen.value = false;
    await loadReviews();
  } finally {
    actionLoadingId.value = undefined;
  }
}
function handleProfilePageChange(page: number, pageSize: number) {
  query.page = page;
  query.pageSize = pageSize;
  loadTeachers();
}
function handleReviewPageChange(page: number, pageSize: number) {
  reviewQuery.page = page;
  reviewQuery.pageSize = pageSize;
  loadReviews();
}
onMounted(load);
</script>

<template>
  <Page title="老师管理" content-class="teacher-page">
    <Card class="teacher-shell">
      <Tabs v-model:active-key="activeTab" :items="[
        { key: 'profiles', label: '老师资料' },
        { key: 'reviews', label: '审核队列' },
      ]" />
      <Alert v-if="error && activeTab === 'profiles'" type="error" :message="error" show-icon />
      <Alert v-if="reviewError && activeTab === 'reviews'" type="error" :message="reviewError" show-icon />
      <template v-if="activeTab === 'profiles'">
        <Space class="toolbar" wrap>
          <Input v-model:value="query.keyword" allow-clear placeholder="搜索老师姓名或标识" @press-enter="loadTeachers" />
          <Select v-model:value="query.enabled" allow-clear placeholder="全部状态" :options="[{ label: '启用', value: 'true' }, { label: '停用', value: 'false' }]" @change="loadTeachers" />
          <Button type="primary" @click="loadTeachers">查询</Button>
          <Button v-if="canWrite" type="primary" @click="openCreate">新增老师</Button>
        </Space>
        <Table :columns="profileColumns" :data-source="teachers" :loading="loading" row-key="key" :pagination="{ current: query.page, pageSize: query.pageSize, total: teacherTotal }" @change="(p: any) => handleProfilePageChange(p.current ?? 1, p.pageSize ?? 20)">
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'enabled'"><Tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '停用' }}</Tag></template>
            <template v-if="column.key === 'action'"><Space><Button v-if="canWrite" size="small" type="link" @click="openEdit(profileRecord(record))">编辑</Button><Button v-if="canWrite" size="small" type="link" @click="openBinding(profileRecord(record))">绑定用户</Button><Button v-if="canWrite" size="small" type="link" @click="toggleEnabled(profileRecord(record))">{{ record.enabled ? '停用' : '启用' }}</Button></Space></template>
          </template>
        </Table>
      </template>
      <template v-else>
        <Space class="toolbar" wrap>
          <Select v-model:value="reviewQuery.teacherKey" allow-clear placeholder="老师筛选" :options="teachers.map((item) => ({ label: item.name, value: item.key }))" @change="loadReviews" />
          <Select v-model:value="reviewQuery.feedType" allow-clear placeholder="内容类型" :options="[{ label: '正式课程', value: 'course' }, { label: '日常动态', value: 'daily' }]" @change="loadReviews" />
          <Select v-model:value="reviewQuery.reviewStatus" placeholder="审核状态" :options="reviewStatusOptions" @change="loadReviews" />
          <Button @click="loadReviews">刷新队列</Button>
        </Space>
        <Table :columns="reviewColumns" :data-source="reviews" :loading="reviewLoading" row-key="id" :pagination="{ current: reviewQuery.page, pageSize: reviewQuery.pageSize, total: reviewTotal }" @change="(p: any) => handleReviewPageChange(p.current ?? 1, p.pageSize ?? 20)">
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'feedType'">{{ record.feedType === 'daily' ? '日常动态' : '正式课程' }}</template>
            <template v-if="column.dataIndex === 'reviewStatus'"><Tag :color="statusColor[record.reviewStatus]">{{ statusLabel[record.reviewStatus] ?? record.reviewStatus }}</Tag></template>
            <template v-if="column.dataIndex === 'reviewReason'"><span>{{ record.reviewReason || '—' }}</span></template>
            <template v-if="column.key === 'action'"><Space v-if="canReview"><Button v-if="record.reviewStatus === 'pending_review'" :loading="actionLoadingId === record.id" size="small" type="link" @click="reviewAction(reviewRecord(record), 'approve')">通过</Button><Button v-if="record.reviewStatus === 'pending_review'" size="small" type="link" @click="openReject(reviewRecord(record))">退回</Button><Button v-if="record.reviewStatus === 'published'" :loading="actionLoadingId === record.id" size="small" danger type="link" @click="reviewAction(reviewRecord(record), 'offline')">下架</Button></Space></template>
          </template>
        </Table>
      </template>
    </Card>

    <Modal v-model:open="profileModalOpen" :confirm-loading="saving" :title="editingKey ? '编辑老师资料' : '新增老师'" width="min(860px, calc(100vw - 32px))" @ok="saveProfile">
      <Form layout="vertical"><Row :gutter="16"><Col :md="12" :xs="24"><Form.Item label="老师标识"><Input v-model:value="form.key" :disabled="!!editingKey" placeholder="例如 han" /></Form.Item></Col><Col :md="12" :xs="24"><Form.Item label="姓名"><Input v-model:value="form.name" placeholder="请输入老师姓名" /></Form.Item></Col><Col :md="12" :xs="24"><Form.Item label="身份"><Input v-model:value="form.title" placeholder="例如 九型导师" /></Form.Item></Col><Col :md="12" :xs="24"><Form.Item label="排序"><InputNumber v-model:value="form.sortOrder" :min="0" style="width: 100%" /></Form.Item></Col><Col :md="12" :xs="24"><Form.Item label="头像"><ImagePathInput v-model:value="form.avatar" dir="teacher-avatars" empty-text="未设置头像" upload-text="上传头像" /></Form.Item></Col><Col :md="12" :xs="24"><Form.Item label="封面"><ImagePathInput v-model:value="form.cover" dir="teacher-covers" empty-text="未设置封面" upload-text="上传封面" /></Form.Item></Col><Col :xs="24"><Form.Item label="列表简介"><Input v-model:value="form.shortIntro" /></Form.Item></Col><Col :xs="24"><Form.Item label="详细介绍"><Textarea v-model:value="form.bio" :rows="4" /></Form.Item></Col><Col :xs="24"><Form.Item label="擅长标签（逗号分隔）"><Input :value="form.tags.join('、')" @update:value="(value: string) => { form.tags = splitTags(value); }" /></Form.Item></Col><Col :md="8" :xs="24"><Form.Item label="首页展示"><Switch v-model:checked="form.showOnHome" /></Form.Item></Col><Col :md="8" :xs="24"><Form.Item label="侧边栏展示"><Switch v-model:checked="form.showInDrawer" /></Form.Item></Col><Col :md="8" :xs="24"><Form.Item label="启用老师"><Switch v-model:checked="form.enabled" /></Form.Item></Col><Col :xs="24"><Form.Item label="客服配置"><Input v-model:value="form.customerServiceConfig!.wechat" placeholder="客服微信号" /><Textarea v-model:value="form.customerServiceConfig!.description" :rows="2" placeholder="客服说明" /></Form.Item></Col><Col :xs="24"><Form.Item label="线下服务"><Input v-model:value="form.offlineServiceConfig!.city" placeholder="服务城市" /><Textarea v-model:value="form.offlineServiceConfig!.description" :rows="2" placeholder="线下服务/合作说明" /></Form.Item></Col></Row><Alert type="info" show-icon message="管理员修改优先" description="老师端草稿不会覆盖管理员维护的正式资料，所有修改会记录操作日志。" /></Form>
    </Modal>
    <Modal v-model:open="bindingModalOpen" :confirm-loading="saving" title="绑定 App 用户为老师" @ok="saveBinding"><Form layout="vertical"><Form.Item label="App 用户 ID"><InputNumber v-model:value="form.appUserIdInput" :min="1" style="width: 100%" /></Form.Item><Alert type="info" show-icon message="老师和代理身份独立" description="绑定老师不会修改该用户已有的代理身份或佣金配置。" /></Form></Modal>
    <Modal v-model:open="rejectModalOpen" title="退回内容" ok-text="确认退回" @ok="rejectReview"><Form layout="vertical"><Form.Item label="退回原因" required><Textarea v-model:value="reviewReason" :rows="4" placeholder="请填写退回原因，老师将依据原因修改后重新提交" /></Form.Item></Form></Modal>
  </Page>
</template>

<style scoped>
.teacher-page { min-height: 100%; }
.teacher-shell { min-height: 520px; }
.toolbar { margin: 16px 0; }
</style>
