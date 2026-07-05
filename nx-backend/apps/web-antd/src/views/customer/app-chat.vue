<script setup lang="ts">
import type { AppChatAuditMessage } from '#/api';

import { onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Alert,
  Button,
  Card,
  Descriptions,
  Drawer,
  Input,
  Select,
  Space,
  Table,
  Tag,
} from 'ant-design-vue';

import { getAppChatAuditMessagesApi } from '#/api';

const loading = ref(false);
const loadError = ref('');
const items = ref<AppChatAuditMessage[]>([]);
const total = ref(0);
const detailOpen = ref(false);
const current = ref<AppChatAuditMessage>();
const query = reactive({ keyword: '', role: '', feedback: '', page: 1, pageSize: 20 });
const roleOptions = [{ label: '全部角色', value: '' }, { label: '用户提问', value: 'user' }, { label: '回答', value: 'assistant' }];
const feedbackOptions = [{ label: '全部反馈', value: '' }, { label: '有帮助', value: 'helpful' }, { label: '不准确', value: 'inaccurate' }, { label: '想继续问', value: 'continue' }];
const columns = [
  { dataIndex: 'createTime', title: '时间', width: 180 }, { dataIndex: 'phone', title: '手机号', width: 150 },
  { dataIndex: 'cardName', title: '卡片', width: 140 }, { dataIndex: 'role', title: '角色', width: 100 },
  { dataIndex: 'content', ellipsis: true, title: '内容' }, { dataIndex: 'feedback', title: '反馈', width: 110 },
  { dataIndex: 'favorite', title: '收藏', width: 90 }, { fixed: 'right' as const, key: 'action', title: '操作', width: 90 },
];
let requestId = 0;

function row(record: Record<string, any>) { return record as AppChatAuditMessage; }
function roleLabel(v?: string) { return v === 'assistant' ? '回答' : v === 'user' ? '用户' : v || '-'; }

async function load() {
  const currentRequestId = ++requestId;
  loading.value = true;
  loadError.value = '';
  try {
    const r = await getAppChatAuditMessagesApi({
      ...query,
      feedback: query.feedback || undefined,
      keyword: query.keyword || undefined,
      role: query.role || undefined,
    });
    if (currentRequestId !== requestId) return;
    items.value = r.items;
    total.value = r.total;
  } catch {
    if (currentRequestId === requestId) {
      loadError.value = '聊天质检列表加载失败，请稍后重试';
    }
  } finally {
    if (currentRequestId === requestId) {
      loading.value = false;
    }
  }
}

function search() { query.page = 1; void load(); }
function change(p: { current?: number; pageSize?: number }) { query.page = p.current ?? 1; query.pageSize = p.pageSize ?? 20; void load(); }
function open(record: AppChatAuditMessage) { current.value = record; detailOpen.value = true; }
onMounted(() => {
  void load();
});
</script>
<template>
  <Page title="聊天质检" description="查看 App 问答明细、反馈和收藏，辅助发现低质量回答。">
    <Card :bordered="false">
      <Alert
        v-if="loadError"
        :message="loadError"
        show-icon
        type="error"
      >
        <template #action>
          <Button size="small" type="link" @click="load">重试</Button>
        </template>
      </Alert>
      <div class="toolbar"><Input v-model:value="query.keyword" allow-clear class="keyword" placeholder="搜索内容 / 手机号 / 昵称" @press-enter="search"/><Select v-model:value="query.role" :options="roleOptions" class="filter" @change="search"/><Select v-model:value="query.feedback" :options="feedbackOptions" class="filter" @change="search"/><Space><Button type="primary" @click="search">查询</Button><Button :loading="loading" @click="load">刷新</Button></Space></div>
      <Table :columns="columns" :data-source="items" :loading="loading" :pagination="{ current: query.page, pageSize: query.pageSize, showSizeChanger: true, total }" row-key="id" :scroll="{ x: 1180 }" @change="change">
        <template #bodyCell="{ column, record }"><template v-if="column.dataIndex === 'role'"><Tag>{{ roleLabel(record.role) }}</Tag></template><template v-if="column.dataIndex === 'favorite'">{{ record.favorite ? '是' : '否' }}</template><template v-if="column.key === 'action'"><Button size="small" type="link" @click="open(row(record))">详情</Button></template></template>
      </Table>
    </Card>
    <Drawer v-model:open="detailOpen" title="聊天质检详情" width="min(720px, calc(100vw - 32px))"><Descriptions v-if="current" bordered :column="1" size="small"><Descriptions.Item label="用户">{{ current.phone }} / {{ current.nickname || '-' }} / #{{ current.appUserId }}</Descriptions.Item><Descriptions.Item label="卡片">{{ current.cardName || '-' }} / #{{ current.cardId }}</Descriptions.Item><Descriptions.Item label="角色">{{ roleLabel(current.role) }}</Descriptions.Item><Descriptions.Item label="反馈">{{ current.feedback || '-' }}</Descriptions.Item><Descriptions.Item label="收藏">{{ current.favorite ? '是' : '否' }}</Descriptions.Item><Descriptions.Item label="时间">{{ current.createTime }}</Descriptions.Item><Descriptions.Item label="内容"><pre>{{ current.content }}</pre></Descriptions.Item></Descriptions></Drawer>
  </Page>
</template>
<style scoped>.toolbar{display:flex;flex-wrap:wrap;gap:12px;margin:16px 0}.keyword{width:min(100%,280px)}.filter{width:min(100%,150px)}pre{white-space:pre-wrap;line-height:1.7}@media (max-width:640px){.toolbar,.toolbar :deep(.ant-space){width:100%}.keyword,.filter{width:100%}}</style>
