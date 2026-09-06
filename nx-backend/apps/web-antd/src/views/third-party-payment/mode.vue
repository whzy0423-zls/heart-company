<script setup lang="ts">
import { Page } from '@vben/common-ui';
import { Alert, Button, Card, Modal, Radio, Space, Tag, message } from 'ant-design-vue';
import { computed, onMounted, ref } from 'vue';

import { requestClient } from '#/api/request';

type PaymentMode = 'customer_service' | 'xzn';

interface PaymentModeState {
  customerServiceConfigured: boolean;
  mode: PaymentMode;
  xznChannels: string[];
  xznConfigured: boolean;
  xznEnabled: boolean;
}

const loading = ref(false);
const saving = ref(false);
const state = ref<PaymentModeState>({
  customerServiceConfigured: false,
  mode: 'customer_service',
  xznChannels: [],
  xznConfigured: false,
  xznEnabled: false,
});
const selectedMode = ref<PaymentMode>('customer_service');

const xznReady = computed(
  () =>
    state.value.xznConfigured &&
    state.value.xznEnabled &&
    state.value.xznChannels.length > 0,
);

async function loadMode() {
  loading.value = true;
  try {
    const data = await requestClient.get<PaymentModeState>(
      '/admin/app-payment-mode',
    );
    state.value = data;
    selectedMode.value = data.mode;
  } catch (error) {
    message.error(error instanceof Error ? error.message : '读取支付模式失败');
  } finally {
    loading.value = false;
  }
}

function requestModeChange() {
  if (selectedMode.value === state.value.mode) return;
  if (selectedMode.value === 'xzn' && !state.value.xznConfigured) {
    message.warning('请先完成星之柠商户配置');
    selectedMode.value = state.value.mode;
    return;
  }
  const nextMode = selectedMode.value;
  Modal.confirm({
    title: '确认切换支付模式？',
    content: '切换仅影响新订单，已有未完成订单仍按创建时的支付模式继续。',
    okText: '确认切换',
    cancelText: '取消',
    async onOk() {
      saving.value = true;
      try {
        const data = await requestClient.put<PaymentModeState>(
          '/admin/app-payment-mode',
          { mode: nextMode },
        );
        state.value = data;
        selectedMode.value = data.mode;
        message.success('支付模式已更新，App 刷新权益页后生效');
      } catch (error) {
        selectedMode.value = state.value.mode;
        message.error(error instanceof Error ? error.message : '保存支付模式失败');
        throw error;
      } finally {
        saving.value = false;
      }
    },
    onCancel() {
      selectedMode.value = state.value.mode;
    },
  });
}

onMounted(loadMode);
</script>

<template>
  <Page description="控制 App 新订单使用客服微信或星之柠收银台" title="支付模式">
    <div class="page-content">
      <Alert
        message="切换仅影响新订单，已有未完成订单会继续使用创建时固化的支付模式。"
        show-icon
        type="info"
      />

      <Card :bordered="false" :loading="loading" title="当前模式">
        <Radio.Group v-model:value="selectedMode" class="mode-grid">
          <Radio.Button value="customer_service">
            <div class="mode-option">
              <strong>客服微信</strong>
              <span>创建待确认订单，用户扫码添加客服并提交订单号。</span>
            </div>
          </Radio.Button>
          <Radio.Button value="xzn" :disabled="!state.xznConfigured">
            <div class="mode-option">
              <strong>星之柠 XZN</strong>
              <span>新订单进入星之柠收银台，支付回调后自动开通会员。</span>
            </div>
          </Radio.Button>
        </Radio.Group>
        <Button
          class="save-button"
          type="primary"
          :disabled="selectedMode === state.mode"
          :loading="saving"
          @click="requestModeChange"
        >
          保存并切换
        </Button>
      </Card>

      <Card :bordered="false" title="配置状态">
        <Space direction="vertical" size="middle">
          <div>
            客服二维码：
            <Tag :color="state.customerServiceConfigured ? 'green' : 'orange'">
              {{ state.customerServiceConfigured ? '已配置' : '未配置' }}
            </Tag>
          </div>
          <div>
            星之柠商户：
            <Tag :color="state.xznConfigured ? 'green' : 'red'">
              {{ state.xznConfigured ? '已配置' : '未配置' }}
            </Tag>
          </div>
          <div>
            星之柠 App 通道：
            <Tag :color="xznReady ? 'green' : 'orange'">
              {{ xznReady ? state.xznChannels.join('、') : '未启用可用通道' }}
            </Tag>
          </div>
        </Space>
      </Card>
    </div>
  </Page>
</template>

<style scoped>
.page-content { display: flex; flex-direction: column; gap: 16px; }
.mode-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.mode-grid :deep(.ant-radio-button-wrapper) { height: auto; padding: 18px; border: 1px solid #d9d9d9; border-radius: 8px; }
.mode-grid :deep(.ant-radio-button-wrapper::before) { display: none; }
.mode-option { display: flex; flex-direction: column; gap: 8px; white-space: normal; }
.mode-option span { color: #667085; line-height: 1.6; }
.save-button { margin-top: 20px; }
@media (max-width: 760px) { .mode-grid { grid-template-columns: 1fr; } }
</style>
