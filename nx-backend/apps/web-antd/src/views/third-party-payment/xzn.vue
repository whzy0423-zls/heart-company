<script setup lang="ts">
import { Page } from '@vben/common-ui';
import { Alert, Button, Card, Descriptions, Tag } from 'ant-design-vue';

const docsUrl = 'https://pay.xzncraft.cn/docs#sdk-debug';
const apiBase = 'https://pay.xzncraft.cn/openapi';
const channels = [
  { name: 'H5 支付', detail: '网页端跳转支付，需配置回跳地址' },
  { name: 'App 支付', detail: 'App 内发起统一下单并拉起支付' },
  { name: '抖音支付', detail: '抖音端使用 douyinpay 支付方式' },
];
</script>

<template>
  <Page description="统一管理星之柠聚合支付的接入准备信息" title="星之柠支付">
    <Alert class="mb-4" message="当前仅完成接入工作台，真实商户密钥需由后端安全配置接口保存。" show-icon type="info" />
    <Card :bordered="false" class="mb-4" title="通道状态">
      <div class="channel-grid">
        <Card v-for="channel in channels" :key="channel.name" size="small">
          <div class="channel-title"><strong>{{ channel.name }}</strong><Tag color="orange">待配置</Tag></div>
          <div class="channel-detail">{{ channel.detail }}</div>
        </Card>
      </div>
    </Card>
    <Card :bordered="false" class="mb-4" title="接口信息">
      <Descriptions :column="1" bordered size="small">
        <Descriptions.Item label="接口地址">{{ apiBase }}</Descriptions.Item>
        <Descriptions.Item label="统一下单">POST /pay/create</Descriptions.Item>
        <Descriptions.Item label="订单查询">POST /pay/query</Descriptions.Item>
        <Descriptions.Item label="订单退款">POST /pay/refund</Descriptions.Item>
        <Descriptions.Item label="异步回调">服务端接收 notify_url，并返回 success</Descriptions.Item>
        <Descriptions.Item label="签名方式">MD5 或 SHA256WithRSA</Descriptions.Item>
      </Descriptions>
    </Card>
    <Card :bordered="false" title="接入工具">
      <p>先在星之柠后台创建商户并获取商户号、商户密钥，再按通道配置回调地址。</p>
      <Button type="primary" :href="docsUrl" target="_blank">打开星之柠 SDK 调试</Button>
    </Card>
  </Page>
</template>

<style scoped>
.channel-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.channel-title { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.channel-detail { color: #667085; font-size: 13px; margin-top: 8px; }
@media (max-width: 900px) { .channel-grid { grid-template-columns: 1fr; } }
</style>
