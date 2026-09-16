<script setup lang="ts">
import type { AppCustomer } from '#/api';
import type { DistributionAgent } from '#/api/core/distribution';

import { computed, onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useAccessStore } from '@vben/stores';
import {
  Alert,
  Button,
  Card,
  Col,
  Form,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Statistic,
  Table,
  Tag,
  Typography,
  message,
} from 'ant-design-vue';

import { getAppCustomerListApi } from '#/api';
import {
  createDistributionAgentApi,
  getDistributionAgentsApi,
  updateDistributionAgentStatusApi,
} from '#/api/core/distribution';

const access = useAccessStore();
const canWrite = computed(() => access.accessCodes.includes('Customer:App:Write'));

const agents = ref<DistributionAgent[]>([]);
const loading = ref(false);
const creating = ref(false);
const actionLoadingId = ref<number | null>(null);
const createAgentModalOpen = ref(false);
const selectedCustomerId = ref<number>();
const customerOptions = ref<AppCustomer[]>([]);
const customerSearching = ref(false);
let customerSearchRequestId = 0;

const columns = [
  { dataIndex: 'id', fixed: 'left' as const, title: '代理 ID', width: 100 },
  { dataIndex: 'appUserId', title: 'App 用户 ID', width: 120 },
  { dataIndex: 'agentCode', title: '代理号', width: 180 },
  { dataIndex: 'level', title: '代理等级', width: 120 },
  { dataIndex: 'status', title: '状态', width: 110 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 140 },
];

const activeAgentCount = computed(
  () => agents.value.filter((item) => item.status === 'active').length,
);
const pausedAgentCount = computed(
  () => agents.value.filter((item) => item.status === 'paused').length,
);
const existingAgentCustomerIds = computed(
  () => new Set(agents.value.map((item) => item.appUserId)),
);
const selectedCustomer = computed(() =>
  customerOptions.value.find((item) => item.id === selectedCustomerId.value),
);
const generatedAgentCodePreview = computed(() =>
  selectedCustomerId.value ? `A${selectedCustomerId.value}` : '选择客户后由后台自动生成',
);

function agentOf(record: Record<string, any>) {
  return record as DistributionAgent;
}

function levelText(level: number) {
  return level === 1 ? '一级代理' : `${level} 级代理`;
}

function statusText(status: string) {
  switch (status) {
    case 'active': {
      return '正常';
    }
    case 'paused': {
      return '已暂停';
    }
    default: {
      return status || '-';
    }
  }
}

function statusColor(status: string) {
  return status === 'active' ? 'green' : 'default';
}

function customerOptionLabel(customer: AppCustomer) {
  const nickname = customer.nickname || '未填写昵称';
  const phone = customer.phone || '未绑定手机号';
  const account = customer.account ? ` / ${customer.account}` : '';
  const existingAgentText = customerIsExistingAgent(customer.id)
    ? '｜已是代理'
    : '';
  return `#${customer.id}｜${nickname}｜${phone}${account}${existingAgentText}`;
}

function customerIsExistingAgent(customerId: number) {
  return existingAgentCustomerIds.value.has(customerId);
}

function resetCreateForm() {
  selectedCustomerId.value = undefined;
  customerOptions.value = [];
}

async function searchAppCustomers(keyword = '') {
  const currentRequestId = ++customerSearchRequestId;
  customerSearching.value = true;
  try {
    const result = await getAppCustomerListApi({
      keyword: keyword || undefined,
      page: 1,
      pageSize: 20,
      status: 'active',
    });
    if (currentRequestId !== customerSearchRequestId) return;
    customerOptions.value = result.items;
  } catch {
    message.error('App 客户查询失败');
  } finally {
    if (currentRequestId === customerSearchRequestId) {
      customerSearching.value = false;
    }
  }
}

async function openCreateModal() {
  resetCreateForm();
  createAgentModalOpen.value = true;
  await searchAppCustomers();
}

async function load() {
  loading.value = true;
  try {
    agents.value = (await getDistributionAgentsApi()).items;
  } catch {
    message.error('代理列表加载失败');
  } finally {
    loading.value = false;
  }
}

async function create() {
  if (!selectedCustomerId.value) {
    message.warning('请先选择 App 客户');
    return;
  }
  creating.value = true;
  try {
    await createDistributionAgentApi({ appUserId: selectedCustomerId.value });
    message.success('代理已创建，代理号由后台自动生成');
    createAgentModalOpen.value = false;
    resetCreateForm();
    await load();
  } catch {
    message.error('创建失败，请检查用户是否存在、是否已经是代理，或自动代理号是否重复');
  } finally {
    creating.value = false;
  }
}

async function toggle(agent: DistributionAgent) {
  if (!canWrite.value) {
    message.warning('当前账号没有代理编辑权限');
    return;
  }
  const status = agent.status === 'active' ? 'paused' : 'active';
  actionLoadingId.value = agent.id;
  try {
    await updateDistributionAgentStatusApi(agent.id, status);
    agent.status = status;
    message.success(status === 'active' ? '代理已恢复' : '代理已暂停');
  } catch {
    message.error('状态更新失败');
  } finally {
    actionLoadingId.value = null;
  }
}

onMounted(load);
</script>

<template>
  <Page title="分销代理管理">
    <div class="distribution-page">
      <Alert
        show-icon
        type="info"
        message="代理管理说明"
        description="这里用于把已有 App 用户开通为一级代理。创建后用户会立即获得一级代理身份，可使用后台自动生成的代理号邀请客户；暂停后该代理不能继续作为有效邀请人，但历史数据仍保留。"
      />

      <Row :gutter="16">
        <Col :lg="8" :xs="24">
          <Card>
            <Statistic title="代理总数" :value="agents.length" />
          </Card>
        </Col>
        <Col :lg="8" :xs="24">
          <Card>
            <Statistic title="正常代理" :value="activeAgentCount" />
          </Card>
        </Col>
        <Col :lg="8" :xs="24">
          <Card>
            <Statistic title="已暂停" :value="pausedAgentCount" />
          </Card>
        </Col>
      </Row>

      <Card>
        <template #title>代理列表</template>
        <template #extra>
          <Button type="primary" :disabled="!canWrite" @click="openCreateModal">
            新增代理
          </Button>
        </template>

        <Table
          :columns="columns"
          :data-source="agents"
          :loading="loading"
          :pagination="false"
          :scroll="{ x: 820 }"
          row-key="id"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'appUserId'">
              #{{ agentOf(record).appUserId }}
            </template>
            <template v-else-if="column.dataIndex === 'agentCode'">
              <Space>
                <Tag color="blue">{{ agentOf(record).agentCode }}</Tag>
                <Typography.Text type="secondary">后台自动生成</Typography.Text>
              </Space>
            </template>
            <template v-else-if="column.dataIndex === 'level'">
              <Tag color="purple">{{ levelText(agentOf(record).level) }}</Tag>
            </template>
            <template v-else-if="column.dataIndex === 'status'">
              <Tag :color="statusColor(agentOf(record).status)">
                {{ statusText(agentOf(record).status) }}
              </Tag>
            </template>
            <template v-else-if="column.key === 'action'">
              <Popconfirm
                :title="agentOf(record).status === 'active' ? '确认暂停该代理？' : '确认恢复该代理？'"
                ok-text="确认"
                cancel-text="取消"
                @confirm="toggle(agentOf(record))"
              >
                <Button
                  type="link"
                  :disabled="!canWrite"
                  :loading="actionLoadingId === agentOf(record).id"
                >
                  {{ agentOf(record).status === 'active' ? '暂停' : '恢复' }}
                </Button>
              </Popconfirm>
            </template>
          </template>
        </Table>
      </Card>

      <Modal
        v-model:open="createAgentModalOpen"
        title="新增代理"
        ok-text="确认开通"
        cancel-text="取消"
        :confirm-loading="creating"
        @cancel="resetCreateForm"
        @ok="create"
      >
        <Space direction="vertical" size="middle" class="full-width">
          <Alert
            show-icon
            type="warning"
            message="开通前请确认"
            description="请先搜索并选择当前 App 的客户。创建后用户会立即获得一级代理身份；代理号由后台自动生成，前端不需要填写。"
          />
          <Form layout="vertical">
            <Form.Item label="搜索并选择 App 客户" required>
              <Select
                v-model:value="selectedCustomerId"
                show-search
                allow-clear
                :filter-option="false"
                :loading="customerSearching"
                placeholder="输入手机号、昵称或账号搜索"
                class="full-width"
                @focus="searchAppCustomers()"
                @search="searchAppCustomers"
              >
                <Select.Option
                  v-for="customer in customerOptions"
                  :key="customer.id"
                  :value="customer.id"
                  :disabled="customerIsExistingAgent(customer.id)"
                >
                  {{ customerOptionLabel(customer) }}
                </Select.Option>
              </Select>
              <Typography.Text type="secondary">
                只能从当前 App 客户中选择，避免手填 ID 出错。
              </Typography.Text>
            </Form.Item>

            <Form.Item label="已选客户">
              <template v-if="selectedCustomer">
                <Space wrap>
                  <Tag color="blue">#{{ selectedCustomer.id }}</Tag>
                  <span>{{ selectedCustomer.nickname || '未填写昵称' }}</span>
                  <Typography.Text type="secondary">
                    {{ selectedCustomer.phone || '未绑定手机号' }}
                  </Typography.Text>
                </Space>
              </template>
              <Typography.Text v-else type="secondary">
                请先在上方搜索框里选择一个 App 客户。
              </Typography.Text>
            </Form.Item>

            <Form.Item label="代理号由后台自动生成">
              <Space>
                <Tag color="green">{{ generatedAgentCodePreview }}</Tag>
                <Typography.Text type="secondary">
                  开通成功后以列表返回的代理号为准。
                </Typography.Text>
              </Space>
            </Form.Item>
          </Form>
        </Space>
      </Modal>
    </div>
  </Page>
</template>

<style scoped>
.distribution-page {
  display: grid;
  gap: 16px;
}
.full-width {
  width: 100%;
}
</style>
