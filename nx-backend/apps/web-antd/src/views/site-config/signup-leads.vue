<script setup lang="ts">
import type { Dayjs } from 'dayjs';

import type {
  SignupDetail,
  SignupFollowInput,
  SignupLead,
  SignupTimelineItem,
  SystemUser,
} from '#/api';

import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useRoute } from 'vue-router';

import { IconifyIcon } from '@vben/icons';

import {
  Alert,
  Button,
  Card,
  DatePicker,
  Descriptions,
  Drawer,
  Empty,
  Form,
  Input,
  message,
  Select,
  Space,
  Table,
  Tag,
  Timeline,
} from 'ant-design-vue';
import dayjs from 'dayjs';

import {
  getSignupDetailApi,
  getSignupLeadListApi,
  getSystemUserListApi,
  saveSignupFollowApi,
} from '#/api';

import PageShell from '../system/components/page-shell.vue';

const followStatusOptions = [
  { color: 'default', label: '待跟进', value: 'pending' },
  { color: 'processing', label: '已联系', value: 'contacted' },
  { color: 'warning', label: '有意向', value: 'interested' },
  { color: 'success', label: '已成交', value: 'deal' },
  { color: 'error', label: '无效线索', value: 'invalid' },
];
const typeNames: Record<number, string> = {
  1: '完美型',
  2: '助人型',
  3: '成就型',
  4: '自我型',
  5: '理智型',
  6: '忠诚型',
  7: '活跃型',
  8: '领袖型',
  9: '和平型',
};

const activeFollowStatusOptions = followStatusOptions.filter(
  (item) => item.value !== 'deal',
);
const route = useRoute();

const loading = ref(false);
const detailLoading = ref(false);
const saving = ref(false);
const reopening = ref(false);
const leads = ref<SignupLead[]>([]);
const users = ref<SystemUser[]>([]);
const total = ref(0);
const detailOpen = ref(false);
const detail = ref<SignupDetail>();
const nextFollowDate = ref<Dayjs>();
const query = reactive({
  keyword: '',
  page: 1,
  pageSize: 20,
  status: '',
});
const followForm = reactive<SignupFollowInput>({
  content: '',
  followNote: '',
  nextFollowTime: '',
  owner: '',
  status: 'pending',
});
let refreshTimer: number | undefined;
let requestId = 0;

const current = computed(() => detail.value?.lead);
const isDeal = computed(() => current.value?.followStatus === 'deal');
const ownerOptions = computed(() =>
  users.value
    .filter((item) => item.status === 1)
    .map((item) => ({
      label: item.nickname
        ? `${item.nickname}（${item.username}）`
        : item.username,
      value: item.nickname || item.username,
    })),
);
const summary = computed(() => {
  const map = new Map<string, number>();
  for (const item of followStatusOptions) {
    map.set(item.value, 0);
  }
  leads.value.forEach((item) => {
    const key = item.followStatus || 'pending';
    map.set(key, (map.get(key) || 0) + 1);
  });
  return followStatusOptions.map((item) => ({
    ...item,
    count: map.get(item.value) || 0,
  }));
});

const columns = [
  { dataIndex: 'name', fixed: 'left' as const, title: '客户', width: 170 },
  { dataIndex: 'contact', title: '联系方式', width: 190 },
  { dataIndex: 'followStatus', title: '跟进状态', width: 120 },
  { dataIndex: 'owner', title: '负责人', width: 140 },
  { dataIndex: 'nextFollowTime', title: '下次跟进', width: 170 },
  { dataIndex: 'interest', title: '兴趣方向', width: 150 },
  { dataIndex: 'message', ellipsis: true, title: '咨询需求' },
  { dataIndex: 'createTime', title: '提交时间', width: 180 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 112 },
];

async function load(options: { silent?: boolean } = {}) {
  const currentRequestId = ++requestId;
  if (!options.silent) {
    loading.value = true;
  }
  try {
    const result = await getSignupLeadListApi({
      keyword: query.keyword,
      page: query.page,
      pageSize: query.pageSize,
      status: query.status || undefined,
    });
    if (currentRequestId !== requestId) return;
    leads.value = result.items;
    total.value = result.total;
  } finally {
    if (!options.silent && currentRequestId === requestId) {
      loading.value = false;
    }
  }
}

async function loadUsers() {
  try {
    const result = await getSystemUserListApi({ page: 1, pageSize: 200 });
    users.value = result.items;
  } catch {
    users.value = [];
  }
}

async function refreshLatest() {
  query.keyword = '';
  query.status = '';
  query.page = 1;
  await load();
}

async function refreshSilently() {
  await load({ silent: true });
}

async function openDetail(record: SignupLead) {
  detailOpen.value = true;
  detailLoading.value = true;
  try {
    detail.value = await getSignupDetailApi(record.id);
    hydrateFollowForm(detail.value.lead);
  } finally {
    detailLoading.value = false;
  }
}

function hydrateFollowForm(lead: SignupLead) {
  followForm.content = '';
  followForm.followNote = lead.followNote || '';
  followForm.owner = lead.owner || '';
  followForm.status =
    lead.followStatus === 'deal' ? 'contacted' : lead.followStatus || 'pending';
  followForm.nextFollowTime = lead.nextFollowTime || '';
  nextFollowDate.value = lead.nextFollowTime
    ? dayjs(lead.nextFollowTime)
    : undefined;
}

async function saveFollow() {
  if (!current.value || isDeal.value) return;
  saving.value = true;
  try {
    const nextFollowTime = nextFollowDate.value
      ? nextFollowDate.value.format('YYYY-MM-DD HH:mm:ss')
      : '';
    await saveSignupFollowApi(current.value.id, {
      ...followForm,
      nextFollowTime,
    });
    detail.value = await getSignupDetailApi(current.value.id);
    hydrateFollowForm(detail.value.lead);
    await load({ silent: true });
    message.success('跟进信息已保存');
  } finally {
    saving.value = false;
  }
}

async function reopenLead() {
  if (!current.value) return;
  reopening.value = true;
  try {
    await saveSignupFollowApi(current.value.id, {
      content: '重新打开已成交线索',
      followNote: current.value.followNote,
      nextFollowTime: '',
      owner: current.value.owner,
      status: 'contacted',
    });
    detail.value = await getSignupDetailApi(current.value.id);
    hydrateFollowForm(detail.value.lead);
    await load({ silent: true });
    message.success('线索已重新打开');
  } finally {
    reopening.value = false;
  }
}

function contactTypeLabel(type?: string) {
  return type === 'wechat' ? '微信号' : '手机号';
}

function sourceLabel(lead?: SignupLead) {
  if (!lead) return '-';
  if (lead.utmSource) {
    return `${lead.utmSource}${lead.utmCampaign ? ` / ${lead.utmCampaign}` : ''}`;
  }
  if (lead.referrer) return '外部来源';
  return '自然访问';
}

function genderLabel(value?: string) {
  if (value === 'male') return '男生';
  if (value === 'female') return '女生';
  return '未知';
}

function typeLabel(value?: number) {
  if (!value) return '-';
  return `${value}号 ${typeNames[value] || ''}`.trim();
}

function statusMeta(status?: string) {
  return (
    followStatusOptions.find((item) => item.value === status) ?? {
      color: 'default',
      label: '-',
      value: '',
    }
  );
}

function leadRecord(record: Record<string, any>): SignupLead {
  return record as SignupLead;
}

function timelineTitle(item: SignupTimelineItem) {
  if (item.type === 'created') return '提交报名';
  return `跟进记录：${statusMeta(item.status).label}`;
}

function handleTableChange(pagination: {
  current?: number;
  pageSize?: number;
}) {
  query.page = pagination.current ?? 1;
  query.pageSize = pagination.pageSize ?? 20;
  load();
}

function search() {
  query.page = 1;
  load();
}

onMounted(() => {
  const status = String(route.query.status || '');
  if (status) {
    query.status = status;
  }
  load();
  loadUsers();
  refreshTimer = window.setInterval(() => {
    refreshSilently();
  }, 5000);
  window.addEventListener('focus', refreshSilently);
});

onUnmounted(() => {
  if (refreshTimer) {
    window.clearInterval(refreshTimer);
  }
  window.removeEventListener('focus', refreshSilently);
});

watch(
  () => route.query.status,
  (value) => {
    const status = String(value || '');
    if (query.status === status) return;
    query.status = status;
    query.page = 1;
    load();
  },
);
</script>

<template>
  <PageShell
    description="查看官网报名线索，并记录负责人、跟进状态、下次跟进时间与沟通时间线。"
    :loading="loading"
    title="客户跟进管理"
    @refresh="load"
  >
    <div class="lead-page">
      <div class="summary-grid">
        <button
          v-for="item in summary"
          :key="item.value"
          class="summary-item"
          :class="{ active: query.status === item.value }"
          type="button"
          @click="
            query.status = query.status === item.value ? '' : item.value;
            search();
          "
        >
          <span class="summary-label">{{ item.label }}</span>
          <strong>{{ item.count }}</strong>
          <Tag :color="item.color">{{ item.label }}</Tag>
        </button>
      </div>

      <Card :bordered="false" class="filter-card">
        <div class="filter-bar">
          <Input
            v-model:value="query.keyword"
            allow-clear
            class="keyword-input"
            placeholder="搜索称呼 / 联系方式 / 兴趣 / 需求"
            @press-enter="search"
          />
          <Select
            v-model:value="query.status"
            allow-clear
            class="status-select"
            :options="followStatusOptions"
            placeholder="跟进状态"
          />
          <Space>
            <Button type="primary" @click="search">查询</Button>
            <Button @click="refreshLatest">最新报名</Button>
          </Space>
        </div>
      </Card>

      <Card :bordered="false" class="table-card">
        <Table
          :columns="columns"
          :data-source="leads"
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
            <template v-if="column.dataIndex === 'name'">
              <div class="customer-cell">
                <div class="customer-avatar">
                  {{ record.name?.slice(0, 1) || '客' }}
                </div>
                <div class="customer-main">
                  <div class="customer-name">{{ record.name }}</div>
                  <div class="customer-sub">{{ record.createTime }}</div>
                </div>
              </div>
            </template>
            <template v-if="column.dataIndex === 'contact'">
              <div class="contact-cell">
                <Tag
                  :color="record.contactType === 'wechat' ? 'green' : 'blue'"
                >
                  {{ contactTypeLabel(record.contactType) }}
                </Tag>
                <span>{{ record.contact }}</span>
              </div>
            </template>
            <template v-if="column.dataIndex === 'interest'">
              <Tag v-if="record.interest">{{ record.interest }}</Tag>
              <span v-else>-</span>
            </template>
            <template v-if="column.dataIndex === 'followStatus'">
              <Tag :color="statusMeta(leadRecord(record).followStatus).color">
                {{ statusMeta(leadRecord(record).followStatus).label }}
              </Tag>
            </template>
            <template v-if="column.dataIndex === 'owner'">
              {{ record.owner || '-' }}
            </template>
            <template v-if="column.dataIndex === 'nextFollowTime'">
              {{ record.nextFollowTime || '-' }}
            </template>
            <template v-if="column.dataIndex === 'message'">
              <span>{{ record.message || '-' }}</span>
            </template>
            <template v-if="column.key === 'action'">
              <Button
                size="small"
                type="link"
                @click="openDetail(leadRecord(record))"
              >
                查看详情
              </Button>
            </template>
          </template>
        </Table>
      </Card>
    </div>

    <Drawer
      v-model:open="detailOpen"
      :loading="detailLoading"
      title="线索详情"
      width="760px"
    >
      <div v-if="current" class="detail-layout">
        <div class="lead-profile">
          <div class="profile-avatar">
            {{ current.name?.slice(0, 1) || '客' }}
          </div>
          <div class="profile-main">
            <div class="profile-title-row">
              <h3>{{ current.name }}</h3>
              <Tag :color="statusMeta(current.followStatus).color">
                {{ statusMeta(current.followStatus).label }}
              </Tag>
            </div>
            <div class="profile-meta">
              {{ contactTypeLabel(current.contactType) }}：{{ current.contact }}
              <span v-if="current.interest"> · {{ current.interest }}</span>
            </div>
          </div>
        </div>

        <Alert
          v-if="isDeal"
          banner
          message="该线索已成交，默认关闭继续跟进。需要再次沟通时可先重新打开线索。"
          type="success"
        />

        <Descriptions :column="2" bordered size="small">
          <Descriptions.Item label="负责人">
            {{ current.owner || '-' }}
          </Descriptions.Item>
          <Descriptions.Item label="下次跟进">
            {{ current.nextFollowTime || '-' }}
          </Descriptions.Item>
          <Descriptions.Item label="提交时间">
            {{ current.createTime }}
          </Descriptions.Item>
          <Descriptions.Item label="联系方式">
            {{ current.contact }}
          </Descriptions.Item>
          <Descriptions.Item label="客户来源">
            {{ sourceLabel(current) }}
          </Descriptions.Item>
          <Descriptions.Item label="来源页面">
            {{ current.sourcePath || '-' }}
          </Descriptions.Item>
          <Descriptions.Item label="咨询需求" :span="2">
            <div class="message-text">{{ current.message || '-' }}</div>
          </Descriptions.Item>
          <Descriptions.Item label="跟进备注" :span="2">
            <div class="message-text">{{ current.followNote || '-' }}</div>
          </Descriptions.Item>
        </Descriptions>

        <div class="insight-grid">
          <Card :bordered="false" class="insight-card">
            <template #title>
              <span class="card-title">
                <IconifyIcon icon="lucide:radar" />
                来源追踪
              </span>
            </template>
            <Descriptions :column="1" size="small">
              <Descriptions.Item label="访客 ID">
                {{ current.visitorId || '-' }}
              </Descriptions.Item>
              <Descriptions.Item label="落地页">
                <span class="break-text">{{ current.landingPage || '-' }}</span>
              </Descriptions.Item>
              <Descriptions.Item label="来源引用">
                <span class="break-text">{{ current.referrer || '-' }}</span>
              </Descriptions.Item>
              <Descriptions.Item label="UTM">
                <span class="break-text">
                  {{ current.utmSource || '-' }}
                  <template v-if="current.utmMedium">
                    / {{ current.utmMedium }}</template
                  >
                  <template v-if="current.utmCampaign">
                    / {{ current.utmCampaign }}</template
                  >
                </span>
              </Descriptions.Item>
            </Descriptions>
          </Card>

          <Card :bordered="false" class="insight-card">
            <template #title>
              <span class="card-title">
                <IconifyIcon icon="lucide:gamepad-2" />
                小游戏画像
              </span>
            </template>
            <div v-if="detail?.gameResult" class="game-profile">
              <div class="game-type">
                <strong>{{ typeLabel(detail.gameResult.resultType) }}</strong>
                <span>{{ genderLabel(detail.gameResult.gender) }}</span>
              </div>
              <div class="game-meta">
                副型：{{ typeLabel(detail.gameResult.secondType) }}
                <br />
                测试时间：{{ detail.gameResult.createTime }}
              </div>
            </div>
            <Empty
              v-else
              description="暂未绑定小游戏结果"
              :image="Empty.PRESENTED_IMAGE_SIMPLE"
            />
          </Card>
        </div>

        <div class="content-grid">
          <Card :bordered="false" class="follow-panel">
            <template #title>
              <span class="card-title">
                <IconifyIcon icon="lucide:clipboard-pen-line" />
                跟进操作
              </span>
            </template>
            <div v-if="isDeal" class="closed-panel">
              <Empty
                description="已成交线索暂不继续跟进"
                :image="Empty.PRESENTED_IMAGE_SIMPLE"
              />
              <Button :loading="reopening" type="primary" @click="reopenLead">
                重新打开线索
              </Button>
            </div>
            <Form v-else layout="vertical">
              <Form.Item label="跟进状态">
                <Select
                  v-model:value="followForm.status"
                  :options="activeFollowStatusOptions"
                />
              </Form.Item>
              <Form.Item label="负责人">
                <Select
                  v-model:value="followForm.owner"
                  allow-clear
                  show-search
                  :filter-option="
                    (input, option) =>
                      String(option?.label ?? '')
                        .toLowerCase()
                        .includes(input.toLowerCase())
                  "
                  :options="ownerOptions"
                  placeholder="选择负责人"
                />
              </Form.Item>
              <Form.Item label="下次跟进时间">
                <DatePicker
                  v-model:value="nextFollowDate"
                  show-time
                  class="full-control"
                  placeholder="选择时间"
                />
              </Form.Item>
              <Form.Item label="跟进内容">
                <Input.TextArea
                  v-model:value="followForm.content"
                  :rows="3"
                  placeholder="记录本次沟通情况"
                />
              </Form.Item>
              <Form.Item label="线索备注">
                <Input.TextArea
                  v-model:value="followForm.followNote"
                  :rows="3"
                  placeholder="长期备注，会展示在列表详情里"
                />
              </Form.Item>
              <Button
                :loading="saving"
                type="primary"
                block
                @click="saveFollow"
              >
                保存跟进
              </Button>
            </Form>
          </Card>

          <Card :bordered="false" class="timeline-panel">
            <template #title>
              <span class="card-title">
                <IconifyIcon icon="lucide:history" />
                线索时间线
              </span>
            </template>
            <Timeline>
              <Timeline.Item
                v-for="item in detail?.timeline ?? []"
                :key="`${item.type}-${item.createTime}-${item.content}`"
              >
                <div class="timeline-title">{{ timelineTitle(item) }}</div>
                <div class="timeline-meta">
                  {{ item.createTime }}
                  <span v-if="item.operator"> · {{ item.operator }}</span>
                  <span v-if="item.owner"> · 负责人：{{ item.owner }}</span>
                  <span v-if="item.nextFollowTime">
                    · 下次跟进：{{ item.nextFollowTime }}
                  </span>
                </div>
                <div class="timeline-content">{{ item.content || '-' }}</div>
              </Timeline.Item>
            </Timeline>
          </Card>
        </div>

        <Card :bordered="false" class="visit-panel">
          <template #title>
            <span class="card-title">
              <IconifyIcon icon="lucide:route" />
              访问轨迹
            </span>
          </template>
          <Timeline v-if="detail?.visitTraces?.length">
            <Timeline.Item
              v-for="item in detail.visitTraces"
              :key="`${item.path}-${item.createTime}`"
            >
              <div class="timeline-title">{{ item.path }}</div>
              <div class="timeline-meta">
                {{ item.createTime }}
                <span v-if="item.title"> · {{ item.title }}</span>
              </div>
              <div v-if="item.referrer" class="timeline-content">
                来源：{{ item.referrer }}
              </div>
            </Timeline.Item>
          </Timeline>
          <Empty
            v-else
            description="暂无访问轨迹"
            :image="Empty.PRESENTED_IMAGE_SIMPLE"
          />
        </Card>
      </div>
    </Drawer>
  </PageShell>
</template>

<style scoped>
.lead-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 12px;
}

.summary-item {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: flex-start;
  min-width: 0;
  padding: 14px 16px;
  text-align: left;
  cursor: pointer;
  background: hsl(var(--card));
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
  transition:
    border-color 0.2s ease,
    box-shadow 0.2s ease,
    transform 0.2s ease;
}

.summary-item:hover,
.summary-item.active {
  border-color: hsl(var(--primary) / 55%);
  box-shadow: 0 8px 22px hsl(var(--foreground) / 8%);
  transform: translateY(-1px);
}

.summary-label {
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}

.summary-item strong {
  font-size: 24px;
  line-height: 1;
}

.filter-card :deep(.ant-card-body),
.table-card :deep(.ant-card-body) {
  padding: 16px;
}

.filter-bar {
  display: grid;
  grid-template-columns: minmax(260px, 420px) 180px auto;
  gap: 10px;
  justify-content: start;
}

.keyword-input,
.status-select {
  width: 100%;
}

.customer-cell,
.contact-cell,
.lead-profile,
.profile-title-row,
.card-title {
  display: flex;
  align-items: center;
}

.customer-cell {
  gap: 10px;
}

.customer-avatar,
.profile-avatar {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  color: hsl(var(--primary));
  background: hsl(var(--primary) / 12%);
  border: 1px solid hsl(var(--primary) / 20%);
  border-radius: 8px;
}

.customer-avatar {
  width: 34px;
  height: 34px;
}

.customer-main {
  min-width: 0;
}

.customer-name {
  overflow: hidden;
  text-overflow: ellipsis;
  font-weight: 600;
  white-space: nowrap;
}

.customer-sub {
  margin-top: 2px;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
  white-space: nowrap;
}

.contact-cell {
  gap: 6px;
  min-width: 0;
}

.detail-layout {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.lead-profile {
  gap: 14px;
  padding: 16px;
  background: hsl(var(--accent) / 32%);
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
}

.profile-avatar {
  width: 48px;
  height: 48px;
  font-size: 20px;
}

.profile-main {
  min-width: 0;
}

.profile-title-row {
  flex-wrap: wrap;
  gap: 8px;
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

.content-grid {
  display: grid;
  grid-template-columns: minmax(280px, 320px) minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}

.insight-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(260px, 320px);
  gap: 16px;
}

.follow-panel,
.insight-card,
.timeline-panel,
.visit-panel {
  border: 1px solid hsl(var(--border));
}

.card-title {
  gap: 8px;
}

.closed-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
  align-items: stretch;
}

.full-control {
  width: 100%;
}

.message-text,
.break-text,
.timeline-content {
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}

.game-profile {
  display: grid;
  gap: 12px;
}

.game-type {
  display: flex;
  gap: 10px;
  align-items: center;
}

.game-type strong {
  font-size: 20px;
}

.game-type span {
  padding: 4px 8px;
  color: hsl(var(--primary));
  background: hsl(var(--primary) / 10%);
  border-radius: 8px;
}

.game-meta {
  line-height: 1.7;
  color: hsl(var(--muted-foreground));
}

.timeline-title {
  font-weight: 600;
}

.timeline-meta {
  margin: 4px 0 6px;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
}

@media (max-width: 1100px) {
  .summary-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .content-grid,
  .insight-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 768px) {
  .summary-grid,
  .filter-bar {
    grid-template-columns: 1fr;
  }
}
</style>
