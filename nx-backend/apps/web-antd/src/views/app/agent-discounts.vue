<script setup lang="ts">
import type {
  AppAgentDiscountAudience,
  AppAgentDiscountMode,
  AppAgentDiscountRule,
  AppPlan,
} from '#/api';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { IconifyIcon } from '@vben/icons';
import { useAccessStore } from '@vben/stores';

import {
  Alert,
  Button,
  Drawer,
  Form,
  InputNumber,
  message,
  Segmented,
  Space,
  Switch,
  Table,
  Tag,
} from 'ant-design-vue';

import {
  getAccessCodesApi,
  getAppAgentDiscountsApi,
  getAppPlansApi,
  updateAppAgentDiscountsApi,
} from '#/api';

import {
  discountedPriceCents,
  validateDiscountRule,
} from './agent-discount-pricing';

const paidProductIds = [
  'vip_month',
  'vip_quarter',
  'vip_year',
  'svip_month',
  'svip_quarter',
  'svip_year',
] as const;
const audiences: Array<{ key: AppAgentDiscountAudience; label: string }> = [
  { key: 'agent_self', label: '一级代理本人' },
  { key: 'invited_user', label: '受邀用户' },
];
const discountModes: Array<{ label: string; value: AppAgentDiscountMode }> = [
  { label: '按比例减免', value: 'percent_off' },
  { label: '按金额减免', value: 'amount_off' },
];

interface DiscountDraft {
  enabled: boolean;
  mode: AppAgentDiscountMode;
  value: number;
}

const access = useAccessStore();
const canWrite = computed(() =>
  access.accessCodes.includes('App:PlanManagement:Write'),
);
const permissionsReady = ref(false);
const loading = ref(false);
const saving = ref(false);
const loadFailed = ref(false);
const drawerOpen = ref(false);
const selectedPlan = ref<AppPlan | null>(null);
const plans = ref<AppPlan[]>([]);
const rules = ref<AppAgentDiscountRule[]>([]);
const draftRules = reactive<Record<AppAgentDiscountAudience, DiscountDraft>>({
  agent_self: { enabled: false, mode: 'percent_off', value: 0 },
  invited_user: { enabled: false, mode: 'percent_off', value: 0 },
});

const paidPlans = computed(() =>
  paidProductIds
    .map((id) => plans.value.find((plan) => plan.code === id))
    .filter((plan): plan is AppPlan => plan !== undefined),
);
const columns = computed(() => [
  { dataIndex: 'name', fixed: 'left' as const, title: '套餐', width: 175 },
  { dataIndex: 'priceCents', title: '原价', width: 125 },
  { key: 'agentSelfPrice', title: '一级代理本人优惠价', width: 220 },
  { key: 'invitedUserPrice', title: '受邀用户优惠价', width: 220 },
  ...(canWrite.value
    ? [{ fixed: 'right' as const, key: 'action', title: '操作', width: 105 }]
    : []),
]);

function defaultRule(
  productId: string,
  audience: AppAgentDiscountAudience,
): AppAgentDiscountRule {
  return { audience, enabled: false, mode: 'percent_off', productId, value: 0 };
}

function normalizeRules(incoming: AppAgentDiscountRule[]) {
  return paidProductIds.flatMap((productId) =>
    audiences.map(
      ({ key }) =>
        incoming.find(
          (rule) => rule.productId === productId && rule.audience === key,
        ) ?? defaultRule(productId, key),
    ),
  );
}

function recordOf(record: Record<string, any>) {
  return record as AppPlan;
}

function ruleFor(productId: string, audience: AppAgentDiscountAudience) {
  return (
    rules.value.find(
      (rule) => rule.productId === productId && rule.audience === audience,
    ) ?? defaultRule(productId, audience)
  );
}

function money(cents: number) {
  return `¥${(cents / 100).toFixed(2)}`;
}

function discountLabel(rule: AppAgentDiscountRule) {
  return rule.mode === 'percent_off'
    ? `减 ${(rule.value / 100).toFixed(2)}%`
    : `减 ${money(rule.value)}`;
}

function priceFor(plan: AppPlan, audience: AppAgentDiscountAudience) {
  return discountedPriceCents(plan.priceCents, ruleFor(plan.code, audience));
}

async function load() {
  loading.value = true;
  loadFailed.value = false;
  try {
    const [planList, discountConfig] = await Promise.all([
      getAppPlansApi(),
      getAppAgentDiscountsApi(),
    ]);
    plans.value = planList;
    rules.value = normalizeRules(discountConfig.rules ?? []);
  } catch {
    loadFailed.value = true;
    message.error('代理购卡优惠配置加载失败');
  } finally {
    loading.value = false;
  }
}

function edit(plan: AppPlan) {
  if (!canWrite.value) return;
  selectedPlan.value = plan;
  for (const { key } of audiences) {
    const rule = ruleFor(plan.code, key);
    Object.assign(draftRules[key], {
      enabled: rule.enabled,
      mode: rule.mode,
      value: rule.value / 100,
    });
  }
  drawerOpen.value = true;
}

function setDraftValue(
  audience: AppAgentDiscountAudience,
  value: null | number | string,
) {
  draftRules[audience].value = Number(value ?? 0);
}

function resetDraftValue(audience: AppAgentDiscountAudience) {
  draftRules[audience].value = 0;
}

function draftRule(audience: AppAgentDiscountAudience): AppAgentDiscountRule {
  const draft = draftRules[audience];
  return {
    audience,
    enabled: draft.enabled,
    mode: draft.mode,
    productId: selectedPlan.value?.code ?? '',
    value: Math.round(draft.value * 100),
  };
}

async function save() {
  const plan = selectedPlan.value;
  if (!canWrite.value || !plan || saving.value) return;
  const editedRules = audiences.map(({ key }) => draftRule(key));
  for (const { key, label } of audiences) {
    const rule = editedRules.find((item) => item.audience === key)!;
    const error = validateDiscountRule(plan.priceCents, rule);
    if (error) {
      message.warning(`${label}：${error}`);
      return;
    }
  }
  const nextRules = rules.value.map(
    (rule) =>
      editedRules.find(
        (item) =>
          item.productId === rule.productId && item.audience === rule.audience,
      ) ?? rule,
  );
  saving.value = true;
  try {
    const updated = await updateAppAgentDiscountsApi({ rules: nextRules });
    rules.value = normalizeRules(updated.rules ?? nextRules);
    drawerOpen.value = false;
    message.success('代理购卡优惠已保存');
  } catch {
    message.error('代理购卡优惠保存失败');
  } finally {
    saving.value = false;
  }
}

onMounted(async () => {
  try {
    access.setAccessCodes(await getAccessCodesApi());
  } catch {
    // Keep cached permissions if the refresh endpoint is temporarily unavailable.
  } finally {
    permissionsReady.value = true;
    await load();
  }
});
</script>

<template>
  <Page title="代理购卡优惠">
    <Alert
      v-if="permissionsReady && !canWrite"
      class="status-alert"
      message="当前为只读模式，如需编辑优惠，请联系管理员开通套餐写入权限。"
      show-icon
      type="info"
    />
    <Alert
      v-if="loadFailed"
      class="status-alert"
      message="优惠配置加载失败"
      show-icon
      type="error"
    >
      <template #action>
        <Button size="small" @click="load">重试</Button>
      </template>
    </Alert>

    <Table
      :columns="columns"
      :data-source="paidPlans"
      :loading="loading"
      :pagination="false"
      :row-key="(record: AppPlan) => record.code"
      :scroll="{ x: 850 }"
      size="middle"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'name'">
          <div class="plan-name">{{ recordOf(record).name }}</div>
          <div class="plan-code">{{ recordOf(record).code }}</div>
        </template>
        <template v-else-if="column.dataIndex === 'priceCents'">
          <strong>{{ money(recordOf(record).priceCents) }}</strong>
        </template>
        <template v-else-if="column.key === 'agentSelfPrice'">
          <div
            v-if="ruleFor(recordOf(record).code, 'agent_self').enabled"
            class="price-cell"
          >
            <strong>{{
              money(priceFor(recordOf(record), 'agent_self'))
            }}</strong>
            <Tag color="green">
              {{ discountLabel(ruleFor(recordOf(record).code, 'agent_self')) }}
            </Tag>
          </div>
          <Tag v-else>未启用</Tag>
        </template>
        <template v-else-if="column.key === 'invitedUserPrice'">
          <div
            v-if="ruleFor(recordOf(record).code, 'invited_user').enabled"
            class="price-cell"
          >
            <strong>{{
              money(priceFor(recordOf(record), 'invited_user'))
            }}</strong>
            <Tag color="green">
              {{
                discountLabel(ruleFor(recordOf(record).code, 'invited_user'))
              }}
            </Tag>
          </div>
          <Tag v-else>未启用</Tag>
        </template>
        <template v-else-if="column.key === 'action'">
          <Button v-if="canWrite" type="link" @click="edit(recordOf(record))">
            <IconifyIcon icon="lucide:pencil" />
            配置
          </Button>
        </template>
      </template>
    </Table>

    <Drawer
      v-model:open="drawerOpen"
      :title="selectedPlan ? `配置优惠 · ${selectedPlan.name}` : '配置优惠'"
      width="min(540px, 100vw)"
      destroy-on-close
    >
      <template v-if="selectedPlan">
        <div class="base-price">
          <span>套餐原价</span>
          <strong>{{ money(selectedPlan.priceCents) }}</strong>
        </div>
        <Form layout="vertical">
          <section
            v-for="item in audiences"
            :key="item.key"
            class="discount-section"
          >
            <div class="section-heading">
              <h3>{{ item.label }}</h3>
              <Switch
                v-model:checked="draftRules[item.key].enabled"
                :aria-label="`启用${item.label}优惠`"
              />
            </div>
            <div class="editor-grid">
              <Form.Item label="优惠方式">
                <Segmented
                  v-model:value="draftRules[item.key].mode"
                  :disabled="!draftRules[item.key].enabled"
                  :options="discountModes"
                  block
                  @change="resetDraftValue(item.key)"
                />
              </Form.Item>
              <Form.Item label="减免额度">
                <InputNumber
                  :value="draftRules[item.key].value"
                  :placeholder="
                    draftRules[item.key].mode === 'percent_off'
                      ? '输入减免比例'
                      : '输入减免金额'
                  "
                  :addon-after="
                    draftRules[item.key].mode === 'percent_off' ? '%' : '元'
                  "
                  :disabled="!draftRules[item.key].enabled"
                  :max="
                    draftRules[item.key].mode === 'percent_off'
                      ? 99.99
                      : (selectedPlan.priceCents - 1) / 100
                  "
                  :min="0"
                  :precision="2"
                  class="full-width"
                  @change="(value) => setDraftValue(item.key, value)"
                />
              </Form.Item>
            </div>
            <div class="price-preview">
              <span>原价 {{ money(selectedPlan.priceCents) }}</span>
              <IconifyIcon icon="lucide:arrow-right" />
              <strong
                >优惠价
                {{
                  money(
                    discountedPriceCents(
                      selectedPlan.priceCents,
                      draftRule(item.key),
                    ),
                  )
                }}</strong
              >
            </div>
          </section>
        </Form>
      </template>
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
.status-alert {
  margin-bottom: 16px;
}
.plan-name {
  font-weight: 600;
}
.plan-code {
  color: var(--ant-color-text-tertiary);
  font-size: 12px;
}
.price-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  white-space: nowrap;
}
.base-price,
.section-heading,
.price-preview {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.base-price {
  padding: 0 0 20px;
  color: var(--ant-color-text-secondary);
}
.base-price strong {
  color: var(--ant-color-text);
  font-size: 18px;
}
.discount-section {
  border-top: 1px solid var(--ant-color-border-secondary);
  padding: 20px 0;
}
.section-heading {
  margin-bottom: 16px;
}
.section-heading h3 {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
}
.editor-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr);
  gap: 12px;
}
.full-width {
  width: 100%;
}
.price-preview {
  justify-content: flex-start;
  flex-wrap: wrap;
  color: var(--ant-color-text-secondary);
}
.price-preview strong {
  color: var(--ant-color-text);
}
@media (max-width: 560px) {
  .editor-grid {
    grid-template-columns: 1fr;
    gap: 0;
  }
}
</style>
