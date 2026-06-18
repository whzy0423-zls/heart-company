<script setup lang="ts">
import { Page } from '@vben/common-ui';

import { Button, Card } from 'ant-design-vue';

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
    <Card :bordered="false" :loading="loading">
      <template #extra>
        <div class="actions">
          <Button @click="emit('refresh')">刷新</Button>
          <Button v-if="$slots.create !== null" type="primary" @click="emit('create')">新增</Button>
        </div>
      </template>
      <slot />
    </Card>
  </Page>
</template>

<style scoped>
.actions {
  display: flex;
  gap: 8px;
}
</style>
