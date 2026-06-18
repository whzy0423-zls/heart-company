<script setup lang="ts">
import { Page } from '@vben/common-ui';

import { Button, Card, Spin } from 'ant-design-vue';

defineProps<{
  description?: string;
  loading: boolean;
  saving: boolean;
  title: string;
}>();

const emit = defineEmits<{
  save: [];
}>();
</script>

<template>
  <Page :description="description" :title="title">
    <Spin :spinning="loading">
      <Card :bordered="false">
        <template #extra>
          <Button :loading="saving" type="primary" @click="emit('save')">保存配置</Button>
        </template>
        <slot />
      </Card>
    </Spin>
  </Page>
</template>
