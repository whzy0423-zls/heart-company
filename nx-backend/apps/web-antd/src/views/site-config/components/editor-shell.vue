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
    <Card :bordered="false" class="editor-shell-card">
      <template #extra>
        <Button
          :disabled="loading"
          :loading="saving"
          type="primary"
          @click="emit('save')"
        >
          保存配置
        </Button>
      </template>
      <Spin :spinning="loading">
        <slot></slot>
      </Spin>
    </Card>
  </Page>
</template>

<style scoped>
.editor-shell-card {
  min-height: 360px;
}

.editor-shell-card :deep(.ant-spin-nested-loading),
.editor-shell-card :deep(.ant-spin-container) {
  min-height: 280px;
}
</style>
