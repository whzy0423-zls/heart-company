<script setup lang="ts">
import type {
  AppCustomer,
  AppTrialCreditGrant,
  AppTrialCreditList,
} from '#/api';

import { computed, onMounted, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';

import { useAccessStore } from '@vben/stores';

import {
  Alert,
  Button,
  Card,
  Descriptions,
  Drawer,
  Form,
  Input,
  message,
  Modal,
  Pagination,
  Select,
  Space,
  Table,
  Tag,
  Textarea,
} from 'ant-design-vue';

import {
  grantAppTrialCreditsApi,
  getAppCustomerDetailApi,
  getAppCustomerListApi,
  getAppTrialCreditsApi,
  revokeAppTrialCreditsApi,
  updateAppCustomerApi,
} from '#/api';

import { canViewUserInsights } from './app-user-access';
import PageShell from '../system/components/page-shell.vue';
import {
  buildAppCustomerUpdatePayload,
  canEditAppCustomer,
  createAppCustomerEditForm,
} from './app-user-edit';
import { memberPlanLabel } from './app-membership';
import {
  appTrialCreditStatusLabel,
  buildAppTrialCreditPayload,
  createAppTrialCreditForm,
  createAppTrialCreditIdempotencyKey,
  validateAppTrialCreditForm,
} from './app-trial-credit';

const statusOptions = [
  { color: 'success', label: '正常', value: 'active' },
  { color: 'error', label: '禁用', value: 'disabled' },
] satisfies StatusMeta[];

const memberLevelOptions = [
  { label: '普通用户', value: 'free' },
  { label: 'VIP 会员', value: 'vip' },
  { label: 'SVIP', value: 'svip' },
];

const defaultStatusMeta: StatusMeta = {
  color: 'default',
  label: '-',
  value: '',
};

interface StatusMeta {
  color: string;
  label: string;
  value: string;
}

const memberLevelLabels: Record<string, string> = {
  free: '普通用户',
  vip: 'VIP 会员',
  svip: 'SVIP',
  vip_month: 'VIP 月卡',
  vip_quarter: 'VIP 季卡',
  vip_year: 'VIP 年卡',
  svip_month: 'SVIP 月卡',
  svip_quarter: 'SVIP 季卡',
  svip_year: 'SVIP 年卡',
};

const router = useRouter();
const loading = ref(false);
const loadError = ref('');
const accessStore = useAccessStore();
const detailLoading = ref(false);
const customers = ref<AppCustomer[]>([]);
const total = ref(0);
const detailOpen = ref(false);
const detail = ref<AppCustomer>();
const editOpen = ref(false);
const editSaving = ref(false);
const editingCustomer = ref<AppCustomer>();
const editForm = reactive(createAppCustomerEditForm());
const canEdit = computed(() => canEditAppCustomer(accessStore.accessCodes));
const canGrantTrialCredit = computed(() =>
  accessStore.accessCodes.includes('Customer:AppTrialCredit:Grant'),
);
const canOpenUserInsights = computed(() =>
  canViewUserInsights(accessStore.accessCodes),
);
const query = reactive({
  careLevel: undefined as number | undefined,
  keyword: '',
  memberLevel: '',
  page: 1,
  pageSize: 20,
  status: '',
});
let requestId = 0;
let detailRequestId = 0;
const trialCredits = ref<AppTrialCreditList>({
  items: [],
  trialChatRemaining: 0,
});
const grantOpen = ref(false);
const grantSaving = ref(false);
const grantingCustomer = ref<AppCustomer>();
const grantForm = reactive(createAppTrialCreditForm());
let grantIdempotencyKey = '';

const columns = [
  { dataIndex: 'phone', fixed: 'left' as const, title: '手机号', width: 160 },
  { dataIndex: 'nickname', title: '昵称', width: 160 },
  { dataIndex: 'memberLevel', title: '会员等级', width: 130 },
  { dataIndex: 'careLevel', title: '关怀等级', width: 130 },
  { dataIndex: 'memberExpiresAt', title: '会员到期', width: 180 },
  { dataIndex: 'remainingDays', title: '剩余天数', width: 100 },
  { dataIndex: 'status', title: '状态', width: 100 },
  { dataIndex: 'registerSource', title: '注册来源', width: 130 },
  { dataIndex: 'lastLoginAt', title: '最后登录', width: 180 },
  { dataIndex: 'createTime', title: '注册时间', width: 180 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 300 },
];

const trialCreditColumns = [
  { dataIndex: 'amount', title: '赠送', width: 72 },
  { dataIndex: 'remaining', title: '剩余', width: 72 },
  { dataIndex: 'expiresAt', title: '到期时间', width: 168 },
  { dataIndex: 'reason', title: '原因', width: 180 },
  { dataIndex: 'status', title: '状态', width: 80 },
  { key: 'action', title: '操作', width: 72 },
];

function statusMeta(status?: string): StatusMeta {
  return (
    statusOptions.find((item) => item.value === status) ?? defaultStatusMeta
  );
}

function memberLevelLabel(value?: string) {
  if (!value) return '-';
  return memberLevelLabels[value] || memberPlanLabel(value);
}

function sourceLabel(value?: string) {
  if (!value) return '-';
  if (value === 'app_sms') return 'App 短信登录';
  return value;
}

async function load(options: { rethrow?: boolean } = {}) {
  const currentRequestId = ++requestId;
  loading.value = true;
  loadError.value = '';
  try {
    const result = await getAppCustomerListApi({
      keyword: query.keyword || undefined,
      memberLevel: query.memberLevel || undefined,
      page: query.page,
      pageSize: query.pageSize,
      status: query.status || undefined,
      careLevel: query.careLevel,
    });
    if (currentRequestId !== requestId) return;
    customers.value = result.items;
    total.value = result.total;
  } catch (error) {
    if (currentRequestId === requestId) {
      loadError.value = 'App 客户列表加载失败，请稍后重试';
    }
    if (options.rethrow) throw error;
  } finally {
    if (currentRequestId === requestId) {
      loading.value = false;
    }
  }
}

function retryLoad() {
  void load();
}

async function openDetail(record: AppCustomer) {
  const currentDetailRequestId = ++detailRequestId;
  detail.value = undefined;
  detailOpen.value = true;
  detailLoading.value = true;
  try {
    const [result, credits] = await Promise.all([
      getAppCustomerDetailApi(record.id),
      getAppTrialCreditsApi(record.id),
    ]);
    if (currentDetailRequestId !== detailRequestId) return;
    detail.value = result;
    trialCredits.value = credits;
  } catch {
    if (currentDetailRequestId === detailRequestId) {
      detailOpen.value = false;
      message.error('客户详情加载失败，请稍后重试');
    }
  } finally {
    if (currentDetailRequestId === detailRequestId) {
      detailLoading.value = false;
    }
  }
}

function openGrant(record: AppCustomer) {
  grantingCustomer.value = record;
  Object.assign(grantForm, createAppTrialCreditForm());
  grantIdempotencyKey = createAppTrialCreditIdempotencyKey(record.id);
  grantOpen.value = true;
}

async function reloadTrialCredits(userId: number) {
  const credits = await getAppTrialCreditsApi(userId);
  if (detail.value?.id === userId) {
    trialCredits.value = credits;
  }
}

async function saveTrialCredit() {
  if (!grantingCustomer.value) return;
  const validation = validateAppTrialCreditForm(grantForm);
  if (validation) {
    message.error(validation);
    return;
  }
  grantSaving.value = true;
  try {
    await grantAppTrialCreditsApi(
      grantingCustomer.value.id,
      buildAppTrialCreditPayload(grantForm, grantIdempotencyKey),
    );
    await reloadTrialCredits(grantingCustomer.value.id);
    grantOpen.value = false;
    message.success('试用额度已赠送');
  } catch (error) {
    message.error(error instanceof Error ? error.message : '赠送试用额度失败');
  } finally {
    grantSaving.value = false;
  }
}

function canRevokeTrialCredit(item: AppTrialCreditGrant) {
  return (
    canGrantTrialCredit.value &&
    item.status === 'active' &&
    item.remaining > item.reserved &&
    new Date(item.expiresAt).getTime() > Date.now()
  );
}

function revokeTrialCredit(item: AppTrialCreditGrant) {
  if (!detail.value || !canRevokeTrialCredit(item)) return;
  const userId = detail.value.id;
  Modal.confirm({
    content: `将撤销该笔尚未使用的 ${item.remaining - item.reserved} 次额度，已使用记录会保留。`,
    okText: '确认撤销',
    title: '撤销试用额度',
    async onOk() {
      try {
        await revokeAppTrialCreditsApi(userId, item.id);
        await reloadTrialCredits(userId);
        message.success('剩余试用额度已撤销');
      } catch (error) {
        message.error(error instanceof Error ? error.message : '撤销试用额度失败');
        throw error;
      }
    },
  });
}

function formatTrialCreditTime(value?: string) {
  if (!value) return '-';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleString('zh-CN', { hour12: false });
}

function mergeCustomer(updated: AppCustomer) {
  const index = customers.value.findIndex((item) => item.id === updated.id);
  if (index >= 0) {
    customers.value.splice(index, 1, updated);
  }
  if (detail.value?.id === updated.id) {
    detail.value = updated;
  }
}

function goUser360(record: AppCustomer) {
  const query: Record<string, string> = { open: '1' };
  const userId = String(record.id ?? '').trim();
  const keyword = record.phone?.trim();
  if (userId) query.userId = userId;
  if (keyword) query.keyword = keyword;
  router.push({
    path: '/customer/user-insights',
    query,
  });
}

function openEdit(record: AppCustomer) {
  editingCustomer.value = record;
  Object.assign(editForm, createAppCustomerEditForm(record));
  editOpen.value = true;
}

async function saveEdit() {
  if (!editingCustomer.value) return;
  editSaving.value = true;
  try {
    const updated = await updateAppCustomerApi(
      editingCustomer.value.id,
      buildAppCustomerUpdatePayload(editForm),
    );
    mergeCustomer(updated);
    editOpen.value = false;
    message.success('客户信息已更新');
  } catch (error) {
    message.error(error instanceof Error ? error.message : '客户信息更新失败');
    return;
  } finally {
    editSaving.value = false;
  }
  load({ rethrow: true }).catch(() => {
    message.warning('客户信息已更新，列表刷新失败');
  });
}

function customerRecord(record: Record<string, any>): AppCustomer {
  return record as AppCustomer;
}

function trialCreditRecord(record: Record<string, any>): AppTrialCreditGrant {
  return record as AppTrialCreditGrant;
}

function handleTableChange(pagination: {
  current?: number;
  pageSize?: number;
}) {
  query.page = pagination.current ?? 1;
  query.pageSize = pagination.pageSize ?? 20;
  load();
}

function handleMobilePageChange(page: number, pageSize: number) {
  query.page = page;
  query.pageSize = pageSize;
  load();
}

function search() {
  query.page = 1;
  load();
}

onMounted(() => {
  retryLoad();
});
</script>

<template>
  <PageShell
    description="查看通过手机号登录 App 的客户，维护其会员等级与基础资料。"
    :loading="loading"
    title="App 客户"
    @refresh="retryLoad"
  >
    <div class="app-user-page">
      <Alert
        v-if="loadError"
        :message="loadError"
        show-icon
        type="error"
      >
        <template #action>
          <Button size="small" type="link" @click="retryLoad">重试</Button>
        </template>
      </Alert>

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
          <Select
            v-model:value="query.careLevel"
            allow-clear
            class="filter-select"
            :options="Array.from({ length: 10 }, (_, i) => ({ label: `关怀 ${i + 1} 级`, value: i + 1 }))"
            placeholder="关怀等级"
          />
          <Space class="filter-actions">
            <Button type="primary" @click="search">查询</Button>
          </Space>
        </div>
      </Card>

      <Card :bordered="false" class="table-card">
        <div v-if="customers.length" class="mobile-customer-list">
          <article
            v-for="record in customers"
            :key="`mobile-${record.id}`"
            class="mobile-customer-card"
          >
            <div class="mobile-customer-heading">
              <div class="mobile-customer-identity">
                <strong>{{ record.nickname || '未设置昵称' }}</strong>
                <span>{{ record.phone || '-' }}</span>
              </div>
              <Tag :color="statusMeta(record.status).color">
                {{ statusMeta(record.status).label }}
              </Tag>
            </div>
            <div class="mobile-customer-grid">
              <div>
                <span>会员等级</span>
                <Tag>{{ memberLevelLabel(record.memberLevel) }}</Tag>
              </div>
              <div>
                <span>剩余天数</span>
                <b>{{ record.remainingDays ?? 0 }} 天</b>
              </div>
              <div>
                <span>会员到期</span>
                <b>{{ record.memberExpiresAt || '-' }}</b>
              </div>
              <div>
                <span>最后登录</span>
                <b>{{ record.lastLoginAt || '-' }}</b>
              </div>
            </div>
            <div class="mobile-customer-source">
              注册来源：{{ sourceLabel(record.registerSource) }}
            </div>
            <div class="mobile-customer-actions">
              <Button
                v-if="canOpenUserInsights"
                size="small"
                type="link"
                @click="goUser360(customerRecord(record))"
              >
                360
              </Button>
              <Button
                size="small"
                type="link"
                @click="openDetail(customerRecord(record))"
              >
                查看详情
              </Button>
              <Button
                v-if="canEdit"
                size="small"
                type="link"
                @click="openEdit(customerRecord(record))"
              >
                编辑
              </Button>
              <Button
                v-if="canGrantTrialCredit"
                size="small"
                type="link"
                @click="openGrant(customerRecord(record))"
              >
                赠送额度
              </Button>
            </div>
          </article>
        </div>
        <div v-else-if="!loading && !loadError" class="mobile-empty-state">
          暂无客户
        </div>
        <Pagination
          v-if="customers.length || total"
          class="mobile-customer-pagination"
          :current="query.page"
          :page-size="query.pageSize"
          :show-size-changer="true"
          :total="total"
          @change="handleMobilePageChange"
        />
        <Table
          class="desktop-customer-table"
          :columns="columns"
          :data-source="customers"
          :loading="loading"
          :pagination="{
            current: query.page,
            pageSize: query.pageSize,
            showSizeChanger: true,
            total,
          }"
          :scroll="{ x: 1040 }"
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
            <template v-if="column.dataIndex === 'careLevel'">
              <Tag v-if="record.careLevel" :color="record.careLevel >= 8 ? 'error' : record.careLevel >= 4 ? 'warning' : 'success'">
                {{ record.careLevel }} 级{{ record.careLabel ? ` · ${record.careLabel}` : '' }}
              </Tag>
              <span v-else>-</span>
            </template>
            <template v-if="column.dataIndex === 'status'">
              <Tag :color="statusMeta(customerRecord(record).status).color">
                {{ statusMeta(customerRecord(record).status).label }}
              </Tag>
            </template>
            <template v-if="column.dataIndex === 'registerSource'">
              {{ sourceLabel(record.registerSource) }}
            </template>
            <template v-if="column.dataIndex === 'lastLoginAt'">
              {{ record.lastLoginAt || '-' }}
            </template>
            <template v-if="column.key === 'action'">
              <Space>
                <Button
                  v-if="canOpenUserInsights"
                  size="small"
                  type="link"
                  @click="goUser360(customerRecord(record))"
                >
                  360
                </Button>
                <Button
                  size="small"
                  type="link"
                  @click="openDetail(customerRecord(record))"
                >
                  查看详情
                </Button>
                <Button
                  v-if="canEdit"
                  size="small"
                  type="link"
                  @click="openEdit(customerRecord(record))"
                >
                  编辑
                </Button>
                <Button
                  v-if="canGrantTrialCredit"
                  size="small"
                  type="link"
                  @click="openGrant(customerRecord(record))"
                >
                  赠送额度
                </Button>
              </Space>
            </template>
          </template>
        </Table>
      </Card>
    </div>

    <Drawer
      v-model:open="detailOpen"
      :loading="detailLoading"
      title="客户详情"
      width="min(520px, calc(100vw - 32px))"
    >
      <div v-if="detail" class="detail-layout">
        <div class="user-profile">
          <div class="profile-avatar">
            {{ (detail.nickname || detail.phone)?.slice(0, 1) || '客' }}
          </div>
          <div class="profile-main">
            <div class="profile-title-row">
              <h3>{{ detail.nickname || detail.phone }}</h3>
              <Tag :color="statusMeta(detail.status).color">
                {{ statusMeta(detail.status).label }}
              </Tag>
            </div>
            <div class="profile-meta">手机号：{{ detail.phone }}</div>
          </div>
        </div>

        <Descriptions :column="1" bordered size="small">
          <Descriptions.Item label="客户 ID">
            {{ detail.id }}
          </Descriptions.Item>
          <Descriptions.Item label="手机号">
            {{ detail.phone }}
          </Descriptions.Item>
          <Descriptions.Item label="昵称">
            {{ detail.nickname || '-' }}
          </Descriptions.Item>
          <Descriptions.Item label="会员等级">
            {{ memberLevelLabel(detail.memberLevel) }}
          </Descriptions.Item>
          <Descriptions.Item label="会员开始时间">
            {{ detail.memberStartedAt || '-' }}
          </Descriptions.Item>
          <Descriptions.Item label="会员到期时间">
            {{ detail.memberExpiresAt || '-' }}
          </Descriptions.Item>
          <Descriptions.Item label="剩余天数">
            {{ detail.remainingDays ?? 0 }} 天
          </Descriptions.Item>
          <Descriptions.Item label="状态">
            {{ statusMeta(detail.status).label }}
          </Descriptions.Item>
          <Descriptions.Item label="注册来源">
            {{ sourceLabel(detail.registerSource) }}
          </Descriptions.Item>
          <Descriptions.Item label="最后登录">
            {{ detail.lastLoginAt || '-' }}
          </Descriptions.Item>
          <Descriptions.Item label="注册时间">
            {{ detail.createTime }}
          </Descriptions.Item>
          <Descriptions.Item label="更新时间">
            {{ detail.updateTime }}
          </Descriptions.Item>
          <Descriptions.Item label="关怀等级">
            <Tag v-if="detail.careLevel" :color="detail.careLevel >= 8 ? 'error' : detail.careLevel >= 4 ? 'warning' : 'success'">
              {{ detail.careLevel }} 级 · {{ detail.careLabel || '阶段性标注' }}
            </Tag>
            <span v-else>数据积累中</span>
          </Descriptions.Item>
          <Descriptions.Item label="关怀摘要">
            {{ detail.careSummary || '暂未形成足够数据' }}
          </Descriptions.Item>
          <Descriptions.Item label="趋势 / 更新时间">
            {{ detail.careTrend || '-' }} / {{ detail.careEvaluatedAt || '-' }}
          </Descriptions.Item>
        </Descriptions>

        <section class="trial-credit-section">
          <div class="section-heading">
            <div>
              <h4>推广试用对话</h4>
              <div class="section-subtitle">
                当前可用 {{ trialCredits.trialChatRemaining }} 次
                <template v-if="trialCredits.trialChatNearestExpiresAt">
                  · 最近到期 {{ formatTrialCreditTime(trialCredits.trialChatNearestExpiresAt) }}
                </template>
              </div>
            </div>
            <Button
              v-if="canGrantTrialCredit"
              size="small"
              type="primary"
              @click="openGrant(detail)"
            >
              赠送额度
            </Button>
          </div>
          <Table
            :columns="trialCreditColumns"
            :data-source="trialCredits.items"
            :pagination="false"
            :scroll="{ x: 640 }"
            row-key="id"
            size="small"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'amount'">
                {{ record.amount }} 次
              </template>
              <template v-if="column.dataIndex === 'remaining'">
                {{ Math.max(record.remaining - record.reserved, 0) }} 次
              </template>
              <template v-if="column.dataIndex === 'expiresAt'">
                {{ formatTrialCreditTime(record.expiresAt) }}
              </template>
              <template v-if="column.dataIndex === 'status'">
                <Tag>{{ appTrialCreditStatusLabel(record.status) }}</Tag>
              </template>
              <template v-if="column.key === 'action'">
                <Button
                  v-if="canRevokeTrialCredit(trialCreditRecord(record))"
                  danger
                  size="small"
                  type="link"
                  @click="revokeTrialCredit(trialCreditRecord(record))"
                >
                  撤销
                </Button>
              </template>
            </template>
          </Table>
        </section>
      </div>
    </Drawer>

    <Modal
      v-model:open="editOpen"
      :confirm-loading="editSaving"
      ok-text="保存"
      title="编辑客户"
      width="min(520px, calc(100vw - 32px))"
      @ok="saveEdit"
    >
      <Form :model="editForm" layout="vertical">
        <Form.Item label="手机号">
          <Input :value="editingCustomer?.phone || '-'" disabled />
        </Form.Item>
        <Form.Item label="会员等级" name="memberLevel" required>
          <Select
            v-model:value="editForm.memberLevel"
            :options="memberLevelOptions"
           placeholder="请选择会员等级"/>
        </Form.Item>
        <Form.Item label="状态" name="status" required>
          <Select v-model:value="editForm.status" :options="statusOptions"  placeholder="请选择状态"/>
        </Form.Item>
      </Form>
    </Modal>

    <Modal
      v-model:open="grantOpen"
      :confirm-loading="grantSaving"
      ok-text="确认赠送"
      title="赠送试用额度"
      width="min(520px, calc(100vw - 32px))"
      @ok="saveTrialCredit"
    >
      <Form :model="grantForm" layout="vertical">
        <Form.Item label="用户">
          <Input
            :value="grantingCustomer ? `${grantingCustomer.nickname || '-'} / ${grantingCustomer.phone}` : '-'"
            disabled
          />
        </Form.Item>
        <Form.Item label="赠送次数" name="amount" required>
          <Input
            v-model:value.number="grantForm.amount"
            max="1000"
            min="1"
            placeholder="请输入赠送次数"
            type="number"
          />
        </Form.Item>
        <Form.Item label="到期时间" name="expiresAt" required>
          <Input
            v-model:value="grantForm.expiresAt"
            placeholder="请选择到期时间"
            type="datetime-local"
          />
        </Form.Item>
        <Form.Item label="赠送原因" name="reason" required>
          <Textarea
            v-model:value="grantForm.reason"
            :maxlength="200"
            :rows="3"
            placeholder="例如：分享活动奖励"
            show-count
          />
        </Form.Item>
      </Form>
    </Modal>
  </PageShell>
</template>

<style scoped>
.app-user-page {
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
  grid-template-columns: minmax(220px, 360px) 160px 140px 160px auto;
  gap: 10px;
  justify-content: start;
}

.keyword-input,
.filter-select {
  width: 100%;
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

  .table-card :deep(.ant-card-body) {
    padding: 12px;
  }
}

.mobile-customer-list {
  display: none;
}

.mobile-customer-card {
  padding: 14px;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
  background: hsl(var(--background));
}

.mobile-customer-card + .mobile-customer-card {
  margin-top: 10px;
}

.mobile-customer-heading {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  justify-content: space-between;
}

.mobile-customer-identity {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;
}

.mobile-customer-identity strong {
  overflow: hidden;
  font-size: 15px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-customer-identity span,
.mobile-customer-source {
  color: hsl(var(--muted-foreground));
  font-size: 12px;
}

.mobile-customer-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 16px;
  margin-top: 14px;
}

.mobile-customer-grid > div {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 4px;
}

.mobile-customer-grid span {
  color: hsl(var(--muted-foreground));
  font-size: 12px;
}

.mobile-customer-grid b {
  overflow: hidden;
  font-size: 13px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-customer-source {
  margin-top: 12px;
}

.mobile-customer-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 2px 4px;
  margin-top: 10px;
  padding-top: 8px;
  border-top: 1px solid hsl(var(--border));
}

.mobile-empty-state,
.mobile-customer-pagination {
  display: none;
}

@media (max-width: 640px) {
  .desktop-customer-table {
    display: none;
  }

  .mobile-customer-list {
    display: block;
  }

  .mobile-empty-state {
    display: block;
    padding: 32px 0;
    color: hsl(var(--muted-foreground));
    text-align: center;
  }

  .mobile-customer-pagination {
    display: flex;
    justify-content: center;
    margin-top: 14px;
  }
}

.detail-layout {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.user-profile {
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

.profile-meta {
  margin-top: 4px;
  color: hsl(var(--muted-foreground));
}

.trial-credit-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.section-heading {
  display: flex;
  gap: 12px;
  align-items: center;
  justify-content: space-between;
}

.section-heading h4 {
  margin: 0;
  font-size: 15px;
  line-height: 22px;
}

.section-subtitle {
  margin-top: 2px;
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}
</style>
