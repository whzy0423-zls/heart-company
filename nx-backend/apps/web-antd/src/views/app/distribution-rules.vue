<script setup lang="ts">
import type { DistributionRule } from '#/api/core/distribution';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Alert,
  Button,
  Card,
  Col,
  Divider,
  InputNumber,
  Row,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'ant-design-vue';

import {
  activateDistributionRuleApi,
  createDistributionRuleApi,
  getDistributionRulesApi,
} from '#/api/core/distribution';

const levelLabels: Record<number, string> = {
  1: '一级代理比例',
  2: '二级代理比例',
  3: '三级代理比例',
};

const columns = [
  { dataIndex: 'version', title: '版本', width: 90 },
  { dataIndex: 'status', title: '状态', width: 110 },
  { key: 'rates', title: '三级分佣比例', width: 300 },
  { dataIndex: 'totalRateBps', title: '总分佣比例', width: 120 },
  { dataIndex: 'createdAt', title: '创建时间', width: 180 },
  { dataIndex: 'activatedAt', title: '启用时间', width: 180 },
  { fixed: 'right' as const, key: 'action', title: '操作', width: 110 },
];

const rules = ref<DistributionRule[]>([]);
const loading = ref(false);
const creating = ref(false);
const activatingId = ref<number | null>(null);
const exampleOrderYuan = ref(199);
const draftRates = reactive<Record<number, number>>({ 1: 30, 2: 10, 3: 5 });

const activeRule = computed(() =>
  rules.value.find((item) => item.status === 'active'),
);

const totalDraftRateBps = computed(() => percentToBps(draftTotalPercent.value));
const draftTotalPercent = computed(() =>
  [1, 2, 3].reduce((sum, level) => sum + normalizePercent(draftRates[level]), 0),
);
const draftTotalValid = computed(() => totalDraftRateBps.value <= 10_000);
const draftPreviewRows = computed(() =>
  [1, 2, 3].map((level) => {
    const percent = normalizePercent(draftRates[level]);
    return {
      amount: exampleOrderYuan.value * (percent / 100),
      label: levelLabels[level],
      level,
      percent,
    };
  }),
);
const draftPreviewTotal = computed(() =>
  draftPreviewRows.value.reduce((sum, item) => sum + item.amount, 0),
);
const commissionRateSummary = computed(() =>
  `总分佣比例 ${formatPercentFromBps(totalDraftRateBps.value)}，示例订单 ¥${exampleOrderYuan.value.toFixed(2)} 预计分佣 ¥${draftPreviewTotal.value.toFixed(2)}`,
);

function normalizePercent(value: null | number | undefined) {
  if (!Number.isFinite(Number(value))) return 0;
  return Math.max(0, Math.min(100, Number(value)));
}

function draftRate(level: number) {
  return normalizePercent(draftRates[level]);
}

function percentToBps(value: number) {
  return Math.round(normalizePercent(value) * 100);
}

function ruleOf(record: Record<string, any>) {
  return record as DistributionRule;
}

function rateBps(rule: DistributionRule, level: number) {
  return Number(rule.rates?.[level] ?? rule.rates?.[String(level) as any] ?? 0);
}

function formatPercentFromBps(bps: number) {
  return `${(Number(bps || 0) / 100).toFixed(2)}%`;
}

function formatDate(value: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString('zh-CN', { hour12: false });
}

function statusText(status: string) {
  switch (status) {
    case 'active':
      return '当前启用';
    case 'archived':
      return '历史归档';
    case 'draft':
      return '草稿待启用';
    default:
      return status || '-';
  }
}

function statusColor(status: string) {
  switch (status) {
    case 'active':
      return 'green';
    case 'draft':
      return 'blue';
    case 'archived':
      return 'default';
    default:
      return 'default';
  }
}

async function load() {
  loading.value = true;
  try {
    rules.value = (await getDistributionRulesApi()).items;
  } catch {
    message.error('佣金规则加载失败');
  } finally {
    loading.value = false;
  }
}

async function create() {
  if (!draftTotalValid.value) {
    message.warning('总分佣比例不能超过 100%');
    return;
  }
  creating.value = true;
  try {
    await createDistributionRuleApi({
      name: '分销佣金规则',
      rates: {
        1: percentToBps(draftRate(1)),
        2: percentToBps(draftRate(2)),
        3: percentToBps(draftRate(3)),
      },
    });
    message.success('规则草稿已创建，请确认后手动启用');
    await load();
  } catch {
    message.error('规则创建失败，请检查比例设置');
  } finally {
    creating.value = false;
  }
}

async function activate(rule: DistributionRule) {
  activatingId.value = rule.id;
  try {
    await activateDistributionRuleApi(rule.id);
    message.success(`版本 ${rule.version} 已启用，原启用规则已归档`);
    await load();
  } catch {
    message.error('规则启用失败，请确认该规则仍是草稿');
  } finally {
    activatingId.value = null;
  }
}

onMounted(load);
</script>

<template>
  <Page title="佣金规则">
    <div class="rule-page">
      <Alert
        show-icon
        type="info"
        message="佣金规则怎么生效"
        description="创建规则时填写一级、二级、三级代理比例；比例保存为基点，100 基点 = 1%，10000 基点 = 100%。新规则先进入草稿，点击启用后才用于后续已支付订单的佣金计算；历史佣金记录不追溯重算。"
      />

      <Row :gutter="16">
        <Col :lg="8" :xs="24">
          <Card title="当前启用规则" class="panel-card">
            <template v-if="activeRule">
              <div class="active-version">版本 {{ activeRule.version }}</div>
              <div class="active-summary">
                总分佣比例：{{ formatPercentFromBps(activeRule.totalRateBps) }}
              </div>
              <div class="rate-tags">
                <Tag color="green">
                  一级 {{ formatPercentFromBps(rateBps(activeRule, 1)) }}
                </Tag>
                <Tag color="blue">
                  二级 {{ formatPercentFromBps(rateBps(activeRule, 2)) }}
                </Tag>
                <Tag color="purple">
                  三级 {{ formatPercentFromBps(rateBps(activeRule, 3)) }}
                </Tag>
              </div>
              <div class="muted">启用时间：{{ formatDate(activeRule.activatedAt) }}</div>
            </template>
            <template v-else>
              <Typography.Text type="secondary">
                还没有启用规则，请创建草稿并启用。
              </Typography.Text>
            </template>
          </Card>
        </Col>
        <Col :lg="16" :xs="24">
          <Card title="创建新规则草稿" class="panel-card">
            <div class="form-grid">
              <label v-for="level in [1, 2, 3]" :key="level" class="rate-field">
                <span>{{ levelLabels[level] }}</span>
                <InputNumber
                  v-model:value="draftRates[level]"
                  :min="0"
                  :max="100"
                  :precision="2"
                  :step="0.5"
                  :aria-label="`${levelLabels[level]}百分比`"
                  addon-after="%"
                  class="rate-input"
                />
                <small>{{ percentToBps(draftRate(level)) }} 基点</small>
              </label>
              <label class="rate-field">
                <span>示例订单金额</span>
                <InputNumber
                  v-model:value="exampleOrderYuan"
                  :min="0"
                  :precision="2"
                  addon-before="¥"
                  class="rate-input"
                  placeholder="例如 99.00"
                />
                <small>用于预估各级佣金</small>
              </label>
            </div>

            <Divider />

            <Space direction="vertical" size="small" class="full-width">
              <div class="summary-row">
                <span>{{ commissionRateSummary }}</span>
                <Tag :color="draftTotalValid ? 'green' : 'red'">
                  总分佣比例 {{ formatPercentFromBps(totalDraftRateBps) }}
                </Tag>
              </div>
              <div class="preview-grid">
                <div v-for="item in draftPreviewRows" :key="item.level" class="preview-cell">
                  <span>{{ item.label }}</span>
                  <strong>{{ item.percent.toFixed(2) }}%</strong>
                  <small>预计 ¥{{ item.amount.toFixed(2) }}</small>
                </div>
              </div>
              <Alert
                v-if="!draftTotalValid"
                show-icon
                type="error"
                message="总分佣比例超过 100%，请降低某一级比例后再创建。"
              />
              <Button type="primary" :loading="creating" @click="create">
                创建草稿规则
              </Button>
            </Space>
          </Card>
        </Col>
      </Row>

      <Card title="规则版本记录" class="panel-card">
        <Table
          :columns="columns"
          :data-source="rules"
          :loading="loading"
          :pagination="false"
          :scroll="{ x: 980 }"
          row-key="id"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'version'">
              版本 {{ ruleOf(record).version }}
            </template>
            <template v-else-if="column.dataIndex === 'status'">
              <Tag :color="statusColor(ruleOf(record).status)">
                {{ statusText(ruleOf(record).status) }}
              </Tag>
            </template>
            <template v-else-if="column.key === 'rates'">
              <Space wrap>
                <Tag color="green">一级 {{ formatPercentFromBps(rateBps(ruleOf(record), 1)) }}</Tag>
                <Tag color="blue">二级 {{ formatPercentFromBps(rateBps(ruleOf(record), 2)) }}</Tag>
                <Tag color="purple">三级 {{ formatPercentFromBps(rateBps(ruleOf(record), 3)) }}</Tag>
              </Space>
            </template>
            <template v-else-if="column.dataIndex === 'totalRateBps'">
              {{ formatPercentFromBps(ruleOf(record).totalRateBps) }}
            </template>
            <template v-else-if="column.dataIndex === 'createdAt'">
              {{ formatDate(ruleOf(record).createdAt) }}
            </template>
            <template v-else-if="column.dataIndex === 'activatedAt'">
              {{ formatDate(ruleOf(record).activatedAt) }}
            </template>
            <template v-else-if="column.key === 'action'">
              <Button
                v-if="ruleOf(record).status === 'draft'"
                type="link"
                :loading="activatingId === ruleOf(record).id"
                @click="activate(ruleOf(record))"
              >
                启用
              </Button>
              <span v-else class="muted">—</span>
            </template>
          </template>
        </Table>
      </Card>
    </div>
  </Page>
</template>

<style scoped>
.rule-page {
  display: grid;
  gap: 16px;
}
.panel-card {
  height: 100%;
}
.active-version {
  font-size: 24px;
  font-weight: 700;
}
.active-summary {
  margin: 8px 0 12px;
  color: var(--ant-color-text-secondary);
}
.rate-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
}
.form-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}
.rate-field {
  display: grid;
  gap: 6px;
}
.rate-input,
.full-width {
  width: 100%;
}
.summary-row {
  align-items: center;
  display: flex;
  gap: 12px;
  justify-content: space-between;
}
.preview-grid {
  display: grid;
  gap: 8px;
  grid-template-columns: repeat(3, minmax(0, 1fr));
}
.preview-cell {
  background: var(--ant-color-fill-tertiary);
  border-radius: 10px;
  display: grid;
  gap: 4px;
  padding: 10px 12px;
}
.preview-cell strong {
  font-size: 18px;
}
.muted,
.rate-field small,
.preview-cell small {
  color: var(--ant-color-text-tertiary);
}
@media (max-width: 900px) {
  .form-grid,
  .preview-grid {
    grid-template-columns: 1fr;
  }
  .summary-row {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
