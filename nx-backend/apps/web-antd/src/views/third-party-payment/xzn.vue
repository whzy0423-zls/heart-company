<script setup lang="ts">
import { Page } from '@vben/common-ui';
import { Alert, Button, Card, Descriptions, Form, Input, Select, Tag, message } from 'ant-design-vue';
import { reactive, ref } from 'vue';

import { requestClient } from '#/api/request';

const docsUrl = 'https://pay.xzncraft.cn/docs#sdk-debug';
const apiBase = 'https://pay.xzncraft.cn/openapi';
const channels = [
  { name: 'H5 支付', detail: '网页端跳转支付，需配置回跳地址' },
  { name: 'App 支付', detail: 'App 内发起统一下单并拉起支付' },
  { name: '抖音支付', detail: '抖音端使用 douyinpay 支付方式' },
];
const form = reactive({ outTradeNo: `XZN${Date.now()}`, totalAmount: '0.01', subject: '星之柠测试订单', paytypeCode: 'wxpay' });
const result = ref('');
const loading = ref(false);
async function createOrder() {
  loading.value = true;
  try {
    const data = await requestClient.post<Record<string, unknown>>(
      '/admin/xzn-pay/create',
      form,
    );
    result.value = JSON.stringify(data, null, 2);
    message.success('下单请求成功');
  } catch (error) { message.error(error instanceof Error ? error.message : '下单失败'); }
  finally { loading.value = false; }
}
</script>

<template>
  <Page description="统一管理星之柠聚合支付的接入准备信息" title="星之柠支付">
    <div class="page-content">
      <Alert message="当前仅完成接入工作台，真实商户密钥需由后端安全配置接口保存。" show-icon type="info" />
      <Card :bordered="false" class="section-card" title="通道状态">
        <div class="channel-grid">
          <Card v-for="channel in channels" :key="channel.name" size="small">
            <div class="channel-title"><strong>{{ channel.name }}</strong><Tag color="orange">待配置</Tag></div>
            <div class="channel-detail">{{ channel.detail }}</div>
          </Card>
        </div>
      </Card>
      <Card :bordered="false" class="section-card" title="接口信息">
        <Descriptions :column="1" bordered size="small">
          <Descriptions.Item label="接口地址">{{ apiBase }}</Descriptions.Item>
          <Descriptions.Item label="统一下单">POST /pay/create</Descriptions.Item>
          <Descriptions.Item label="订单查询">POST /pay/query</Descriptions.Item>
          <Descriptions.Item label="订单退款">POST /pay/refund</Descriptions.Item>
          <Descriptions.Item label="异步回调">服务端接收 notify_url，并返回 success</Descriptions.Item>
          <Descriptions.Item label="签名方式">MD5 或 SHA256WithRSA</Descriptions.Item>
        </Descriptions>
      </Card>
      <Card :bordered="false" class="section-card" title="测试下单">
        <Form class="order-form" layout="vertical"><div class="form-grid">
          <Form.Item label="商户订单号"><Input v-model:value="form.outTradeNo" /></Form.Item>
          <Form.Item label="金额（元）"><Input v-model:value="form.totalAmount" /></Form.Item>
          <Form.Item label="支付方式"><Select v-model:value="form.paytypeCode" :options="[{label:'微信',value:'wxpay'},{label:'支付宝',value:'alipay'},{label:'抖音',value:'douyinpay'}]" /></Form.Item>
          <Form.Item label="订单标题"><Input v-model:value="form.subject" /></Form.Item>
        </div><Button type="primary" :loading="loading" @click="createOrder">发起测试下单</Button></Form>
        <pre v-if="result" class="result">{{ result }}</pre>
      </Card>
      <Card :bordered="false" class="section-card" title="接入工具">
        <p class="tool-description">先在星之柠后台创建商户并获取商户号、商户密钥，再按通道配置回调地址。</p>
        <Button type="primary" :href="docsUrl" target="_blank">打开星之柠 SDK 调试</Button>
      </Card>
    </div>
  </Page>
</template>

<style scoped>
.page-content { display: flex; flex-direction: column; gap: 16px; }
:deep(.section-card > .ant-card-head) { min-height: 48px; padding: 0 20px; }
:deep(.section-card > .ant-card-head .ant-card-head-title) { padding: 12px 0; }
:deep(.section-card > .ant-card-body) { padding: 20px; }
.channel-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.channel-title { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.channel-detail { color: #667085; font-size: 13px; margin-top: 8px; }
.form-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
.order-form :deep(.ant-form-item) { margin-bottom: 16px; }
.tool-description { margin: 0 0 12px; }
.result { background: #f6f8fa; padding: 12px; margin-top: 16px; white-space: pre-wrap; }
@media (max-width: 900px) { .channel-grid { grid-template-columns: 1fr; } }
@media (max-width: 900px) { .form-grid { grid-template-columns: 1fr; } }
</style>
