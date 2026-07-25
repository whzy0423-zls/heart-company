<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import { Avatar } from 'ant-design-vue';

const props = withDefaults(
  defineProps<{
    appName?: string;
    packageName?: string;
    size?: number;
    src?: string;
  }>(),
  {
    appName: '',
    packageName: '',
    size: 40,
    src: '',
  },
);

const imageLoadError = ref(false);
const displayName = computed(
  () => props.appName.trim() || props.packageName.trim() || 'Android 应用',
);
const showImage = computed(() => Boolean(props.src) && !imageLoadError.value);
const accessibleLabel = computed(() =>
  showImage.value
    ? `${displayName.value}应用图标`
    : `${displayName.value}应用图标占位符`,
);
const placeholderText = computed(() =>
  displayName.value.slice(0, 1).toUpperCase(),
);

watch(
  () => props.src,
  () => {
    imageLoadError.value = false;
  },
);

function handleImageLoadError() {
  imageLoadError.value = true;
  return true;
}
</script>

<template>
  <Avatar
    class="app-release-icon"
    :alt="`${displayName}应用图标`"
    :aria-label="accessibleLabel"
    role="img"
    shape="square"
    :size="size"
    :src="showImage ? src : undefined"
    :load-error="handleImageLoadError"
  >
    <span aria-hidden="true">{{ placeholderText }}</span>
  </Avatar>
</template>

<style scoped>
.app-release-icon {
  flex: none;
  overflow: hidden;
  background: hsl(var(--accent));
  color: hsl(var(--accent-foreground));
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
}

.app-release-icon :deep(img) {
  object-fit: cover;
}
</style>
