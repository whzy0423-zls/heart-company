<script setup lang="ts">
import { computed } from 'vue';
import { useAccessStore } from '@vben/stores';
import { Page } from '@vben/common-ui';

import DistributionPosterComposer from './distribution-poster-composer.vue';
const access = useAccessStore();
const canEdit = computed(() => access.accessCodes.includes('Customer:App:Write'));
</script>

<template>
  <Page title="海报管理">
    <DistributionPosterComposer :editable="canEdit" />
  </Page>
</template>
