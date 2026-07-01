<script setup lang="ts">
import { Page } from '@vben/common-ui';

import { Button, Card, Spin } from 'ant-design-vue';

defineProps<{
  description?: string;
  loading?: boolean;
  title: string;
}>();

const emit = defineEmits<{
  create: [];
  refresh: [];
}>();
</script>

<template>
  <Page :description="description" :title="title">
    <Card :bordered="false" class="page-shell-card">
      <template #extra>
        <div class="actions">
          <Button :loading="loading" @click="emit('refresh')">刷新</Button>
          <Button
            v-if="$slots.create !== null"
            type="primary"
            @click="emit('create')"
          >
            新增
          </Button>
        </div>
      </template>
      <Spin :spinning="!!loading">
        <slot></slot>
      </Spin>
    </Card>
  </Page>
</template>

<style scoped>
.actions {
  display: flex;
  gap: 8px;
}

.page-shell-card {
  min-height: 320px;
}

.page-shell-card :deep(.ant-spin-nested-loading),
.page-shell-card :deep(.ant-spin-container) {
  min-height: 240px;
}
</style>
