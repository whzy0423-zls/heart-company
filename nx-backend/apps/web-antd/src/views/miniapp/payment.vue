<script lang="ts">
import type { SiteConfig } from '#/api';

function isRecord(value: unknown): value is Record<string, any> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

export interface MiniappPaymentConfig {
  enabled: boolean;
  [key: string]: unknown;
}

/** Normalizes the shared public config while preserving future payment fields. */
export function normalizeMiniappPayment(
  config: Pick<SiteConfig, 'home'> | { home?: unknown },
): MiniappPaymentConfig {
  const home = isRecord(config.home) ? config.home : {};
  if (config.home !== home) config.home = home;
  const source = isRecord(home.miniappPayment) ? home.miniappPayment : {};
  const payment: MiniappPaymentConfig = {
    ...source,
    enabled: typeof source.enabled === 'boolean' ? source.enabled : true,
  };
  home.miniappPayment = payment;
  return payment;
}
</script>

<script setup lang="ts">
import { computed, watch } from 'vue';

import { Alert, Switch } from 'ant-design-vue';

import EditorShell from '#/views/site-config/components/editor-shell.vue';
import { useSiteConfigEditor } from '#/views/site-config/use-site-config-editor';

const { config, loading, saveConfig, saving } = useSiteConfigEditor();
const miniappPayment = computed<MiniappPaymentConfig | undefined>(() =>
  config.value?.home.miniappPayment as MiniappPaymentConfig | undefined,
);

watch(
  config,
  (current) => {
    if (current) normalizeMiniappPayment(current);
  },
  { immediate: true },
);

async function savePaymentConfig() {
  if (config.value) normalizeMiniappPayment(config.value);
  await saveConfig();
}
</script>

<template>
  <EditorShell
    description="统一控制小程序内的报告解锁、课堂购买和微信支付测试入口。"
    :loading="loading"
    :saving="saving"
    title="小程序支付"
    @save="savePaymentConfig"
  >
    <div v-if="miniappPayment" class="payment-editor">
      <div class="payment-visibility">
        <div>
          <strong>启用小程序支付</strong>
          <p>下架后隐藏所有新支付入口，并由服务端拒绝新的支付下单；已购买权益和报告查看不受影响。</p>
        </div>
        <Switch
          v-model:checked="miniappPayment.enabled"
          aria-label="小程序支付上下架状态"
          data-testid="miniapp-payment-enabled"
        />
      </div>
      <Alert
        message="开关保存后，小程序下次刷新配置时生效。微信支付回调仍会继续处理，确保已完成订单正常落账。"
        show-icon
        type="info"
      />
    </div>
  </EditorShell>
</template>

<style scoped>
.payment-editor {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.payment-visibility {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 18px 20px;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
  background: hsl(var(--muted) / 30%);
}

.payment-visibility p {
  margin: 6px 0 0;
  color: hsl(var(--muted-foreground));
  line-height: 1.6;
}

@media (max-width: 640px) {
  .payment-visibility {
    align-items: flex-start;
  }
}
</style>
