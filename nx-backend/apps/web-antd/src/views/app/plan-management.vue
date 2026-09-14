<script setup lang="ts">
import type { AppPlan } from '#/api';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { IconifyIcon } from '@vben/icons';
import { useAccessStore } from '@vben/stores';
import {
  Button,
  Drawer,
  Form,
  Input,
  InputNumber,
  Space,
  Switch,
  Table,
  Tag,
  message,
} from 'ant-design-vue';

import { getAppPlansApi, updateAppPlanApi } from '#/api';

const access = useAccessStore();
const canWrite = computed(() =>
  access.accessCodes.includes('App:PlanManagement:Write'),
);
const loading = ref(false);
const saving = ref(false);
const drawerOpen = ref(false);
const plans = ref<AppPlan[]>([]);
const form = reactive({
  badge: '',
  cardLimit: 1,
  code: '',
  companionEnabled: false,
  dailyChatLimit: 5,
  deepChatEnabled: false,
  durationDays: 0,
  enabled: true,
  featuresText: '',
  memberPosterEnabled: false,
  name: '',
  originalPriceYuan: 0,
  priceYuan: 0,
  sortOrder: 0,
  storyMonthlyLimit: 1,
  subtitle: '',
});

const columns = [
  { dataIndex: 'name', fixed: 'left' as const, title: '套餐', width: 140 },
  { dataIndex: 'priceCents', title: '套餐价格', width: 120 },
  { dataIndex: 'durationDays', title: '有效期', width: 100 },
  { dataIndex: 'dailyChatLimit', title: '每日聊天', width: 110 },
  { dataIndex: 'storyMonthlyLimit', title: '每月故事', width: 110 },
  { dataIndex: 'cardLimit', title: '人物卡', width: 90 },
  { dataIndex: 'enabled', title: '上架状态', width: 100 },
  { dataIndex: 'sortOrder', title: '排序', width: 80 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 90 },
];

function recordOf(record: Record<string, any>) {
  return record as AppPlan;
}

function money(cents: number) {
  return cents > 0 ? `¥${(cents / 100).toFixed(2)}` : '免费';
}

function limitText(value: number) {
  return value < 0 ? '不限' : `${value} 次`;
}

async function load() {
  loading.value = true;
  try {
    plans.value = await getAppPlansApi();
  } catch {
    message.error('套餐配置加载失败');
  } finally {
    loading.value = false;
  }
}

function edit(plan: AppPlan) {
  Object.assign(form, {
    ...plan,
    featuresText: plan.features.join('\n'),
    originalPriceYuan: plan.originalPriceCents / 100,
    priceYuan: plan.priceCents / 100,
  });
  drawerOpen.value = true;
}

async function save() {
  const current = plans.value.find((item) => item.code === form.code);
  if (!current || !form.name.trim()) {
    message.warning('请填写套餐名称');
    return;
  }
  saving.value = true;
  try {
    const payload: AppPlan = {
      ...current,
      ...form,
      features: form.featuresText
        .split('\n')
        .map((item) => item.trim())
        .filter(Boolean),
      name: form.name.trim(),
      originalPriceCents: Math.round(form.originalPriceYuan * 100),
      priceCents: Math.round(form.priceYuan * 100),
      subtitle: form.subtitle.trim(),
    };
    await updateAppPlanApi(form.code, payload);
    message.success('套餐配置已保存');
    drawerOpen.value = false;
    await load();
  } catch {
    message.error('套餐配置保存失败');
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <Page title="套餐管理">
    <Table
      :columns="columns"
      :data-source="plans"
      :loading="loading"
      :pagination="false"
      :row-key="(record: AppPlan) => record.code"
      :scroll="{ x: 940 }"
      size="middle"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'name'">
          <div class="plan-name">{{ recordOf(record).name }}</div>
          <div class="plan-code">{{ recordOf(record).code }}</div>
        </template>
        <template v-else-if="column.dataIndex === 'priceCents'">
          {{ money(recordOf(record).priceCents) }}
        </template>
        <template v-else-if="column.dataIndex === 'durationDays'">
          {{
            recordOf(record).durationDays > 0
              ? `${recordOf(record).durationDays} 天`
              : '-'
          }}
        </template>
        <template v-else-if="column.dataIndex === 'dailyChatLimit'">
          {{ limitText(recordOf(record).dailyChatLimit) }}
        </template>
        <template v-else-if="column.dataIndex === 'storyMonthlyLimit'">
          {{ recordOf(record).storyMonthlyLimit }} 篇
        </template>
        <template v-else-if="column.dataIndex === 'cardLimit'">
          {{ recordOf(record).cardLimit }} 张
        </template>
        <template v-else-if="column.dataIndex === 'enabled'">
          <Tag :color="recordOf(record).enabled ? 'green' : 'default'">
            {{ recordOf(record).enabled ? '已上架' : '已下架' }}
          </Tag>
        </template>
        <template v-else-if="column.key === 'action'">
          <Button
            v-if="canWrite"
            type="link"
            title="编辑套餐"
            @click="edit(recordOf(record))"
          >
            <IconifyIcon icon="lucide:pencil" />
          </Button>
        </template>
      </template>
    </Table>

    <Drawer
      v-model:open="drawerOpen"
      :width="560"
      destroy-on-close
      title="编辑套餐"
    >
      <Form layout="vertical">
        <div class="form-grid">
          <Form.Item label="套餐代码">
            <Input v-model:value="form.code" disabled />
          </Form.Item>
          <Form.Item label="套餐名称" required>
            <Input v-model:value="form.name" :maxlength="40" />
          </Form.Item>
          <Form.Item label="套餐价格（元）">
            <InputNumber
              v-model:value="form.priceYuan"
              :disabled="form.code === 'free'"
              :min="0"
              :precision="2"
              class="full-width"
            />
          </Form.Item>
          <Form.Item label="划线价格（元）">
            <InputNumber
              v-model:value="form.originalPriceYuan"
              :disabled="form.code === 'free'"
              :min="0"
              :precision="2"
              class="full-width"
            />
          </Form.Item>
          <Form.Item label="有效天数">
            <InputNumber
              v-model:value="form.durationDays"
              :disabled="form.code === 'free'"
              :min="0"
              :max="3660"
              class="full-width"
            />
          </Form.Item>
          <Form.Item label="排序">
            <InputNumber
              v-model:value="form.sortOrder"
              :min="0"
              class="full-width"
            />
          </Form.Item>
          <Form.Item label="每日聊天额度（-1 为不限）">
            <InputNumber
              v-model:value="form.dailyChatLimit"
              :min="-1"
              class="full-width"
            />
          </Form.Item>
          <Form.Item label="每月故事额度">
            <InputNumber
              v-model:value="form.storyMonthlyLimit"
              :min="0"
              class="full-width"
            />
          </Form.Item>
          <Form.Item label="人物卡上限">
            <InputNumber
              v-model:value="form.cardLimit"
              :min="0"
              class="full-width"
            />
          </Form.Item>
          <Form.Item label="上架状态">
            <Switch v-model:checked="form.enabled" />
          </Form.Item>
        </div>
        <Form.Item label="副标题">
          <Input v-model:value="form.subtitle" :maxlength="100" />
        </Form.Item>
        <Form.Item label="角标">
          <Input v-model:value="form.badge" :maxlength="20" />
        </Form.Item>
        <Form.Item label="权益文案（每行一项）">
          <Input.TextArea
            v-model:value="form.featuresText"
            :auto-size="{ minRows: 4, maxRows: 8 }"
          />
        </Form.Item>
        <Space size="large" wrap>
          <label
            ><Switch v-model:checked="form.deepChatEnabled" /> 深度对话</label
          >
          <label
            ><Switch v-model:checked="form.companionEnabled" /> 专业陪伴</label
          >
          <label
            ><Switch v-model:checked="form.memberPosterEnabled" />
            会员海报</label
          >
        </Space>
      </Form>
      <template #footer>
        <Space>
          <Button @click="drawerOpen = false">取消</Button>
          <Button type="primary" :loading="saving" @click="save">保存</Button>
        </Space>
      </template>
    </Drawer>
  </Page>
</template>

<style scoped>
.plan-name {
  font-weight: 600;
}
.plan-code {
  color: var(--ant-color-text-tertiary);
  font-size: 12px;
}
.form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 16px;
}
.full-width {
  width: 100%;
}
@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
