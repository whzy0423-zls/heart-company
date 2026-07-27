<script setup lang="ts">
import type {
  MiniappBooking,
  MiniappCustomer,
  MiniappCustomerDetail,
  MiniappTestRecord,
} from '#/api';

import { onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Drawer,
  Input,
  Select,
  Table,
  Tag,
} from 'ant-design-vue';
import { getMiniappCustomerDetailApi, getMiniappCustomerListApi } from '#/api';
import PageShell from '../system/components/page-shell.vue';
import { bookingSignupTarget, miniappOpenIntent } from './miniapp-user';

const route = useRoute();
const router = useRouter();
const users = ref<MiniappCustomer[]>([]);
const total = ref(0);
const loading = ref(false);
const errorText = ref('');
const detailOpen = ref(false);
const detailLoading = ref(false);
const detailError = ref('');
const detail = ref<MiniappCustomerDetail>();
const selectedTest = ref<MiniappTestRecord>();
const testOpen = ref(false);
const query = reactive({ page: 1, pageSize: 20, keyword: '', channel: '' });
const detailPage = reactive({
  testPage: 1,
  testPageSize: 20,
  bookingPage: 1,
  bookingPageSize: 20,
});
let listRequestId = 0;
let detailRequestId = 0;

const columns = [
  { dataIndex: 'nickname', title: '昵称', width: 150 },
  { dataIndex: 'phone', title: '手机号', width: 150 },
  { dataIndex: 'mainType', title: '主类型', width: 90 },
  { dataIndex: 'memberLevel', title: '会员等级', width: 100 },
  { dataIndex: 'channel', title: '渠道', width: 120 },
  { dataIndex: 'scene', title: '场景', width: 120 },
  { dataIndex: 'lastLoginAt', title: '最后登录', width: 180 },
  { key: 'action', title: '操作', width: 100 },
];
const testColumns = [
  { dataIndex: 'resultType', title: '主类型' },
  { dataIndex: 'secondType', title: '次类型' },
  { dataIndex: 'createTime', title: '提交时间' },
  { key: 'action', title: '操作' },
];
const bookingColumns = [
  { dataIndex: 'contactName', title: '联系人' },
  { dataIndex: 'phone', title: '手机号' },
  { dataIndex: 'status', title: '状态' },
  { dataIndex: 'createTime', title: '预约时间' },
  { key: 'action', title: '操作' },
];

async function load() {
  const current = ++listRequestId;
  loading.value = true;
  errorText.value = '';
  try {
    const result = await getMiniappCustomerListApi({
      page: query.page,
      pageSize: query.pageSize,
      keyword: query.keyword || undefined,
      channel: query.channel || undefined,
    });
    if (current !== listRequestId) return;
    users.value = result.items;
    total.value = result.total;
  } catch {
    if (current === listRequestId)
      errorText.value = '小程序客户列表加载失败，请稍后重试';
  } finally {
    if (current === listRequestId) loading.value = false;
  }
}

async function openDetailByID(userId: string, testRecordId?: string) {
  const current = ++detailRequestId;
  detailOpen.value = true;
  detailLoading.value = true;
  detailError.value = '';
  detail.value = undefined;
  try {
    const result = await getMiniappCustomerDetailApi(userId, { ...detailPage });
    if (current !== detailRequestId) return;
    detail.value = result;
    if (testRecordId) {
      selectedTest.value = result.testRecords.items.find(
        (item) => item.id === testRecordId,
      );
      testOpen.value = Boolean(selectedTest.value);
    }
  } catch (error: any) {
    if (current !== detailRequestId) return;
    detailError.value =
      error?.response?.status === 404
        ? '小程序客户不存在'
        : '小程序客户详情加载失败，请稍后重试';
  } finally {
    if (current === detailRequestId) detailLoading.value = false;
  }
}

function openUser(record: Record<string, any>) {
  Object.assign(detailPage, { testPage: 1, bookingPage: 1 });
  void openDetailByID((record as MiniappCustomer).id);
}
function openTest(record: Record<string, any>) {
  selectedTest.value = record as MiniappTestRecord;
  testOpen.value = true;
}
function openSignup(record: Record<string, any>) {
  const target = bookingSignupTarget((record as MiniappBooking).signupId);
  if (target) void router.push(target);
}
function search() {
  query.page = 1;
  void load();
}
function listChange(p: { current?: number; pageSize?: number }) {
  query.page = p.current ?? 1;
  query.pageSize = p.pageSize ?? 20;
  void load();
}
function testChange(p: { current?: number; pageSize?: number }) {
  detailPage.testPage = p.current ?? 1;
  detailPage.testPageSize = p.pageSize ?? 20;
  if (detail.value) void openDetailByID(detail.value.user.id);
}
function bookingChange(p: { current?: number; pageSize?: number }) {
  detailPage.bookingPage = p.current ?? 1;
  detailPage.bookingPageSize = p.pageSize ?? 20;
  if (detail.value) void openDetailByID(detail.value.user.id);
}
function applyBusinessOpen(event?: Event) {
  const query =
    event instanceof CustomEvent && event.detail ? event.detail : route.query;
  const intent = miniappOpenIntent(query);
  if (intent)
    void openDetailByID(
      intent.userId,
      intent.mode === 'test' ? intent.testRecordId : undefined,
    );
}
onMounted(() => {
  void load();
  applyBusinessOpen();
  window.addEventListener('_businessOpen', applyBusinessOpen);
});
onBeforeUnmount(() =>
  window.removeEventListener('_businessOpen', applyBusinessOpen),
);
</script>

<template>
  <PageShell
    title="小程序客户"
    description="查看小程序用户、测评记录与预约信息。"
    :loading="loading"
    @refresh="load"
  >
    <div class="page-stack">
      <Alert v-if="errorText" :message="errorText" type="error" show-icon
        ><template #action
          ><Button size="small" type="link" @click="load"
            >重试</Button
          ></template
        ></Alert
      >
      <Card :bordered="false"
        ><div class="filters">
          <Input
            v-model:value="query.keyword"
            allow-clear
            placeholder="搜索昵称 / 手机号 / 渠道 / 场景"
            @press-enter="search"
          /><Select
            v-model:value="query.channel"
            allow-clear
            :options="[
              { label: '微信', value: 'wechat' },
              { label: '活动', value: 'campaign' },
            ]"
            placeholder="渠道"
          /><Button type="primary" @click="search">查询</Button>
        </div></Card
      >
      <Card :bordered="false"
        ><Table
          row-key="id"
          :columns="columns"
          :data-source="users"
          :pagination="{
            current: query.page,
            pageSize: query.pageSize,
            total,
            showSizeChanger: true,
          }"
          :scroll="{ x: 1010 }"
          @change="listChange"
          ><template #bodyCell="{ column, record }"
            ><template v-if="column.dataIndex === 'nickname'">{{
              record.nickname || '-'
            }}</template
            ><template v-if="column.dataIndex === 'mainType'">{{
              record.mainType || '-'
            }}</template
            ><template v-if="column.dataIndex === 'memberLevel'"
              ><Tag>{{ record.memberLevel || 0 }}</Tag></template
            ><template v-if="column.key === 'action'"
              ><Button type="link" size="small" @click="openUser(record)"
                >查看详情</Button
              ></template
            ></template
          ></Table
        ></Card
      >
    </div>
    <Drawer
      v-model:open="detailOpen"
      title="小程序客户详情"
      width="min(900px, calc(100vw - 32px))"
      :loading="detailLoading"
    >
      <Alert v-if="detailError" :message="detailError" type="error" show-icon />
      <div v-if="detail" class="detail-stack">
        <Descriptions bordered :column="2" size="small"
          ><Descriptions.Item label="客户 ID">{{
            detail.user.id
          }}</Descriptions.Item
          ><Descriptions.Item label="昵称">{{
            detail.user.nickname || '-'
          }}</Descriptions.Item
          ><Descriptions.Item label="手机号">{{
            detail.user.phone || '-'
          }}</Descriptions.Item
          ><Descriptions.Item label="渠道 / 场景"
            >{{ detail.user.channel || '-' }} /
            {{ detail.user.scene || '-' }}</Descriptions.Item
          ><Descriptions.Item label="九型类型">{{
            detail.user.mainType || '-'
          }}</Descriptions.Item
          ><Descriptions.Item label="最后登录">{{
            detail.user.lastLoginAt || '-'
          }}</Descriptions.Item></Descriptions
        >
        <Card size="small"
          ><h3>测评记录</h3>
          <Table
            row-key="id"
            :columns="testColumns"
            :data-source="detail.testRecords.items"
            :pagination="{
              current: detailPage.testPage,
              pageSize: detailPage.testPageSize,
              total: detail.testRecords.total,
            }"
            @change="testChange"
            ><template #bodyCell="{ column, record }"
              ><template v-if="column.key === 'action'"
                ><Button type="link" size="small" @click="openTest(record)"
                  >查看测评</Button
                ></template
              ></template
            ></Table
          ></Card
        >
        <Card size="small"
          ><h3>预约记录</h3>
          <Table
            row-key="id"
            :columns="bookingColumns"
            :data-source="detail.bookings.items"
            :pagination="{
              current: detailPage.bookingPage,
              pageSize: detailPage.bookingPageSize,
              total: detail.bookings.total,
            }"
            @change="bookingChange"
            ><template #bodyCell="{ column, record }"
              ><template v-if="column.key === 'action'"
                ><Button
                  v-if="record.signupId"
                  type="link"
                  size="small"
                  @click="openSignup(record)"
                  >关联报名</Button
                ></template
              ></template
            ></Table
          ></Card
        >
      </div>
    </Drawer>
    <Drawer
      v-model:open="testOpen"
      :title="`测评记录 #${selectedTest?.id || ''}`"
      width="min(620px, calc(100vw - 32px))"
      ><h3 v-if="selectedTest">测评记录 #{{ selectedTest.id }}</h3>
      <Descriptions v-if="selectedTest" bordered :column="1" size="small"
        ><Descriptions.Item label="主类型">{{
          selectedTest.resultType
        }}</Descriptions.Item
        ><Descriptions.Item label="次类型">{{
          selectedTest.secondType || '-'
        }}</Descriptions.Item
        ><Descriptions.Item label="提交时间">{{
          selectedTest.createTime
        }}</Descriptions.Item
        ><Descriptions.Item label="分数">
          <pre>{{ JSON.stringify(selectedTest.scores, null, 2) }}</pre>
        </Descriptions.Item></Descriptions
      ></Drawer
    >
  </PageShell>
</template>

<style scoped>
.page-stack,
.detail-stack {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.filters {
  display: grid;
  grid-template-columns: minmax(240px, 420px) 160px auto;
  gap: 10px;
  justify-content: start;
}
@media (max-width: 640px) {
  .filters {
    grid-template-columns: 1fr;
  }
}
pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
