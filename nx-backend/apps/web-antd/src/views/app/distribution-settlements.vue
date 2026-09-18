<script setup lang="ts">
import type { DistributionAgent, DistributionSettlement } from '#/api/core/distribution';
import type { SettlementAction } from './distribution-format';

import { computed, onMounted, reactive, ref } from 'vue';
import { Page } from '@vben/common-ui';
import { useAccess } from '@vben/access';
import { Alert, Button, Card, Input, Modal, Select, Space, Table, Tag, message } from 'ant-design-vue';
import {
  createDistributionSettlementApi,
  getDistributionAgentsApi,
  getDistributionSettlementsApi,
  previewDistributionSettlementApi,
  settleDistributionActionApi,
} from '#/api/core/distribution';
import { formatYuan, settlementActionPayload } from './distribution-format';

const { hasAccessByCodes } = useAccess();
const canWrite = computed(() => hasAccessByCodes(['Customer:App:Write']));
const items = ref<DistributionSettlement[]>([]);
const agents = ref<DistributionAgent[]>([]);
const loading = ref(false);
const submitting = ref(false);
const form = reactive({ agentId: undefined as number | undefined, start: '', end: '' });
const preview = ref<{ agentId: number; amount: number; count: number; start: string; end: string }>();
const previewMatches = computed(() => preview.value && preview.value.agentId === form.agentId && preview.value.start === form.start && preview.value.end === form.end);
const activeAction = ref<{ id: number; action: SettlementAction }>();
const actionInput = ref('');
const labels: Record<SettlementAction, string> = { approve: '审核通过', reject: '驳回', cancel: '取消结算', paid: '登记打款' };
const statuses: Record<string, string> = { draft: '待审核', approved: '已审核', paid: '已打款', rejected: '已驳回', cancelled: '已取消' };
const columns = [
  { dataIndex: 'id', title: '批次' },
  { dataIndex: 'agentId', title: '代理' },
  { key: 'period', title: '周期' },
  { dataIndex: 'amount', title: '金额(元)' },
  { dataIndex: 'status', title: '状态' },
  { key: 'action', title: '操作' },
];

async function load() {
  loading.value = true;
  try {
    const [settlements, agentList] = await Promise.all([getDistributionSettlementsApi(), getDistributionAgentsApi()]);
    items.value = settlements.items;
    agents.value = agentList.items;
  } catch { message.error('结算数据加载失败'); }
  finally { loading.value = false; }
}

function period() {
  if (!form.agentId || !form.start || !form.end || form.start > form.end) throw new Error('请选择代理和正确的起止日期');
  return { agentId: form.agentId, start: form.start, end: form.end };
}

async function previewSettlement() {
  submitting.value = true;
  try { preview.value = await previewDistributionSettlementApi(period()); }
  catch (error) { preview.value = undefined; message.error(error instanceof Error ? error.message : '结算预览失败'); }
  finally { submitting.value = false; }
}

async function createSettlement() {
  if (!previewMatches.value || !preview.value?.count) return;
  submitting.value = true;
  try {
    await createDistributionSettlementApi(period());
    preview.value = undefined;
    message.success('结算批次已创建，请审核后登记实际打款凭证');
    await load();
  } catch { message.error('创建失败，佣金可能已被其他批次占用，请重新预览'); }
  finally { submitting.value = false; }
}

function openAction(id: number, action: SettlementAction) {
  activeAction.value = { id, action };
  actionInput.value = '';
}

async function confirmAction() {
  if (!activeAction.value) return;
  submitting.value = true;
  try {
    const { id, action } = activeAction.value;
    const payload = settlementActionPayload(action, actionInput.value);
    await settleDistributionActionApi(id, action, payload);
    activeAction.value = undefined;
    message.success('操作成功');
    await load();
  } catch (error) { message.error(error instanceof Error ? error.message : '操作失败，请检查结算状态'); }
  finally { submitting.value = false; }
}

onMounted(load);
</script>

<template>
  <Page title="分销结算">
    <Card v-if="canWrite" title="生成结算批次" class="mb-4">
      <Space wrap>
        <Select v-model:value="form.agentId" placeholder="选择代理" style="min-width: 220px" :options="agents.map(agent => ({ value: agent.id, label: `${agent.agentCode} · ${agent.level}级代理` }))" />
        <Input v-model:value="form.start" type="date" aria-label="结算开始日期" />
        <span>至</span>
        <Input v-model:value="form.end" type="date" aria-label="结算结束日期" />
        <Button :loading="submitting" @click="previewSettlement">预览结算</Button>
        <Button type="primary" :loading="submitting" :disabled="!previewMatches || !preview?.count" @click="createSettlement">创建批次</Button>
      </Space>
      <Alert v-if="previewMatches && preview" class="mt-3" type="info" :message="`可结算 ${preview.count} 笔，合计 ${formatYuan(preview.amount)}；已被其他批次占用的佣金不重复计入。`" />
    </Card>
    <Table :columns="columns" :data-source="items" :loading="loading" :pagination="{ pageSize: 20 }" row-key="id" :scroll="{ x: 800 }">
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'period'">{{ record.periodStart }} 至 {{ record.periodEnd }}</template>
        <template v-else-if="column.dataIndex === 'amount'">{{ formatYuan(record.amount) }}</template>
        <template v-else-if="column.dataIndex === 'status'"><Tag>{{ statuses[record.status] ?? record.status }}</Tag></template>
        <template v-else-if="column.key === 'action'">
          <Space v-if="canWrite && record.status === 'draft'">
            <a @click="openAction(record.id, 'approve')">审核通过</a>
            <a @click="openAction(record.id, 'reject')">驳回</a>
          </Space>
          <Space v-else-if="canWrite && record.status === 'approved'">
            <a @click="openAction(record.id, 'paid')">登记打款</a>
            <a @click="openAction(record.id, 'cancel')">取消</a>
          </Space>
          <span v-else>—</span>
        </template>
      </template>
    </Table>
    <Modal :open="!!activeAction" :title="activeAction ? labels[activeAction.action] : ''" :confirm-loading="submitting" @ok="confirmAction" @cancel="activeAction = undefined">
      <template v-if="activeAction?.action === 'paid'">
        <p class="mb-3">请完成实际打款后填写银行流水号或打款凭证号。此操作只登记结果，不会发起转账。</p>
        <Input v-model:value="actionInput" placeholder="银行流水号 / 打款凭证号" :maxlength="200" />
      </template>
      <Input.TextArea v-else-if="activeAction?.action === 'reject' || activeAction?.action === 'cancel'" v-model:value="actionInput" placeholder="填写驳回或取消原因" :rows="3" />
      <p v-else>确认审核本批次？审核后仍需完成打款并登记凭证。</p>
    </Modal>
  </Page>
</template>
