<script setup lang="ts">
import type { AppPlan } from '#/api';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { IconifyIcon } from '@vben/icons';
import { useAccessStore } from '@vben/stores';
import {
  Alert,
  Button,
  Drawer,
  Form,
  Input,
  InputNumber,
  Modal,
  Space,
  Switch,
  Table,
  Tag,
  message,
} from 'ant-design-vue';

import {
  getAccessCodesApi,
  getAppPlansApi,
  updateAppPlanApi,
} from '#/api';

const access = useAccessStore();
const canWrite = computed(() =>
  access.accessCodes.includes('App:PlanManagement:Write'),
);
const loading = ref(false);
const saving = ref(false);
const permissionsReady = ref(false);
const actionLoadingCode = ref('');
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
  planLevel: 'free' as AppPlan['planLevel'],
  billingCycle: 'none' as AppPlan['billingCycle'],
  featuresJson: '{}',
  limitsJson: '{}',
});

const baseColumns = [
  { dataIndex: 'name', fixed: 'left' as const, title: '套餐', width: 140 },
  { dataIndex: 'planLevel', title: '会员等级', width: 100 },
  { dataIndex: 'billingCycle', title: '购买周期', width: 100 },
  { dataIndex: 'priceCents', title: '套餐价格', width: 120 },
  { dataIndex: 'durationDays', title: '有效期', width: 100 },
  { dataIndex: 'dailyChatLimit', title: '每日聊天', width: 110 },
  { dataIndex: 'storyMonthlyLimit', title: '每月故事', width: 110 },
  { dataIndex: 'cardLimit', title: '人物卡', width: 90 },
  { dataIndex: 'enabled', title: '上架状态', width: 100 },
  { dataIndex: 'sortOrder', title: '排序', width: 80 },
];
const columns = computed(() => [
  ...baseColumns,
  ...(canWrite.value
    ? [{ fixed: 'right' as const, key: 'action', title: '操作', width: 180 }]
    : []),
]);

function recordOf(record: Record<string, any>) {
  return record as AppPlan;
}

function money(cents: number) {
  return cents > 0 ? `¥${(cents / 100).toFixed(2)}` : '免费';
}

function limitText(value: number) {
  return value < 0 ? '不限' : `${value} 次`;
}

function levelText(value: AppPlan['planLevel']) {
  return value === 'svip' ? 'SVIP' : value === 'vip' ? 'VIP' : '免费';
}

function cycleText(value: AppPlan['billingCycle']) {
  return value === 'month'
    ? '月'
    : value === 'quarter'
      ? '季'
      : value === 'year'
        ? '年'
        : '无';
}

function parseJsonObject(value: string, label: string) {
  try {
    const parsed = JSON.parse(value || '{}');
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      throw new Error('object required');
    }
    return parsed as Record<string, boolean | number>;
  } catch {
    message.warning(`${label}必须是 JSON 对象`);
    return null;
  }
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
    // Legacy annual SKUs remain VIP unless the API explicitly says SVIP.
    planLevel: plan.planLevel ?? (plan.code === 'free' ? 'free' : 'vip'),
    billingCycle: plan.billingCycle ?? (plan.code === 'vip_quarter' ? 'quarter' : plan.code === 'vip_year' ? 'year' : plan.code === 'free' ? 'none' : 'month'),
    featuresJson: JSON.stringify(plan.featureFlags ?? {}, null, 2),
    limitsJson: JSON.stringify(plan.limits ?? {}, null, 2),
  });
  drawerOpen.value = true;
}

async function save() {
  const current = plans.value.find((item) => item.code === form.code);
  if (!current || !form.name.trim()) {
    message.warning('请填写套餐名称');
    return;
  }
  if (form.planLevel === 'free' && form.billingCycle !== 'none') {
    message.warning('免费版只能使用 none 周期');
    return;
  }
  if (form.planLevel !== 'free' && form.billingCycle === 'none') {
    message.warning('付费套餐必须选择 VIP 或 SVIP');
    return;
  }
  const featureFlags = parseJsonObject(form.featuresJson, '功能开关');
  const limits = parseJsonObject(form.limitsJson, '扩展额度');
  if (!featureFlags || !limits) return;
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
      planLevel: form.planLevel,
      billingCycle: form.billingCycle,
      featureFlags: featureFlags as Record<string, boolean>,
      limits: limits as Record<string, number>,
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

function toggleAvailability(plan: AppPlan) {
  const nextEnabled = !plan.enabled;
  Modal.confirm({
    cancelText: '取消',
    content: nextEnabled
      ? '上架后，用户可以在 App 中看到并选择该套餐。'
      : '下架后，新用户将无法在 App 中选择该套餐，已有会员权益不受影响。',
    okButtonProps: { danger: !nextEnabled },
    okText: nextEnabled ? '确认上架' : '确认下架',
    title: `${nextEnabled ? '上架' : '下架'}“${plan.name}”？`,
    async onOk() {
      actionLoadingCode.value = plan.code;
      try {
        await updateAppPlanApi(plan.code, {
          ...plan,
          enabled: nextEnabled,
        });
        message.success(`套餐已${nextEnabled ? '上架' : '下架'}`);
        await load();
      } catch {
        message.error(`${nextEnabled ? '上架' : '下架'}套餐失败`);
      } finally {
        actionLoadingCode.value = '';
      }
    },
  });
}

onMounted(async () => {
  try {
    access.setAccessCodes(await getAccessCodesApi());
  } catch {
    // Keep the cached permissions when the refresh endpoint is temporarily unavailable.
  } finally {
    permissionsReady.value = true;
    await load();
  }
});
</script>

<template>
  <Page title="套餐管理">
    <Alert
      v-if="permissionsReady && !canWrite"
      class="read-only-alert"
      message="当前为只读模式，如需编辑或上下架套餐，请联系管理员开通套餐写入权限。"
      show-icon
      type="info"
    />
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
        <template v-else-if="column.dataIndex === 'planLevel'">
          {{ levelText(recordOf(record).planLevel) }}
        </template>
        <template v-else-if="column.dataIndex === 'billingCycle'">
          {{ cycleText(recordOf(record).billingCycle) }}
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
          <Space v-if="canWrite" :size="4">
            <Button type="link" title="编辑套餐" @click="edit(recordOf(record))">
              <IconifyIcon icon="lucide:pencil" />
              <span>编辑</span>
            </Button>
            <Button
              :danger="recordOf(record).enabled"
              :loading="actionLoadingCode === recordOf(record).code"
              type="link"
              @click="toggleAvailability(recordOf(record))"
            >
              <IconifyIcon
                :icon="recordOf(record).enabled ? 'lucide:archive' : 'lucide:upload'"
              />
              {{ recordOf(record).enabled ? '下架' : '上架' }}
            </Button>
          </Space>
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
            <Input
              v-model:value="form.name"
              :maxlength="40"
              placeholder="请输入套餐名称"
            />
          </Form.Item>
          <Form.Item label="会员等级">
            <select v-model="form.planLevel" class="native-select">
              <option value="free">免费</option>
              <option value="vip">VIP</option>
              <option value="svip">SVIP</option>
            </select>
          </Form.Item>
          <Form.Item label="购买周期">
            <select v-model="form.billingCycle" class="native-select">
              <option value="none">无</option>
              <option value="month">月</option>
              <option value="quarter">季</option>
              <option value="year">年</option>
            </select>
          </Form.Item>
          <Form.Item label="套餐价格（元）">
            <InputNumber
              v-model:value="form.priceYuan"
              :disabled="form.code === 'free'"
              :min="0"
              :precision="2"
              class="full-width"
              placeholder="请输入套餐价格"
            />
          </Form.Item>
          <Form.Item label="划线价格（元）">
            <InputNumber
              v-model:value="form.originalPriceYuan"
              :disabled="form.code === 'free'"
              :min="0"
              :precision="2"
              class="full-width"
              placeholder="请输入划线价格"
            />
          </Form.Item>
          <Form.Item label="有效天数">
            <InputNumber
              v-model:value="form.durationDays"
              :disabled="form.code === 'free'"
              :min="0"
              :max="3660"
              class="full-width"
              placeholder="请输入有效天数"
            />
          </Form.Item>
          <Form.Item label="排序">
            <InputNumber
              v-model:value="form.sortOrder"
              :min="0"
              class="full-width"
              placeholder="请输入排序值"
            />
          </Form.Item>
          <Form.Item label="每日聊天额度（-1 为不限）">
            <InputNumber
              v-model:value="form.dailyChatLimit"
              :min="-1"
              class="full-width"
              placeholder="请输入每日聊天额度"
            />
          </Form.Item>
          <Form.Item label="每月故事额度">
            <InputNumber
              v-model:value="form.storyMonthlyLimit"
              :min="0"
              class="full-width"
              placeholder="请输入每月故事额度"
            />
          </Form.Item>
          <Form.Item label="人物卡上限">
            <InputNumber
              v-model:value="form.cardLimit"
              :min="0"
              class="full-width"
              placeholder="请输入人物卡上限"
            />
          </Form.Item>
          <Form.Item label="上架状态">
            <Switch v-model:checked="form.enabled" />
          </Form.Item>
        </div>
        <Form.Item label="副标题">
          <Input
            v-model:value="form.subtitle"
            :maxlength="100"
            placeholder="请输入套餐副标题"
          />
        </Form.Item>
        <Form.Item label="角标">
          <Input
            v-model:value="form.badge"
            :maxlength="20"
            placeholder="例如：推荐、限时"
          />
        </Form.Item>
        <Form.Item label="权益文案（每行一项）">
          <Input.TextArea
            v-model:value="form.featuresText"
            :auto-size="{ minRows: 4, maxRows: 8 }"
            placeholder="每行填写一项会员权益"
          />
        </Form.Item>
        <Form.Item label="功能开关 JSON">
          <Input.TextArea
            v-model:value="form.featuresJson"
            :auto-size="{ minRows: 3, maxRows: 6 }"
            placeholder='例如：{"deepChat":true,"xinzhili":false}'
          />
        </Form.Item>
        <Form.Item label="扩展额度 JSON">
          <Input.TextArea
            v-model:value="form.limitsJson"
            :auto-size="{ minRows: 3, maxRows: 6 }"
            placeholder='例如：{"deepReport":10,"voiceMinutes":60}'
          />
        </Form.Item>
        <Alert
          message="降级保留规则"
          description="降级后超出额度的历史资源会进入 read_only_over_limit，保留查看和删除入口，不会静默删除。"
          show-icon
          type="info"
        />
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
.native-select {
  width: 100%;
  height: 32px;
  border: 1px solid var(--ant-color-border);
  border-radius: 6px;
  padding: 0 8px;
  background: var(--ant-color-bg-container);
}
.read-only-alert {
  margin-bottom: 16px;
}
@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
