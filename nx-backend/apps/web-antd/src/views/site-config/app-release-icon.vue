<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import { Avatar } from 'ant-design-vue';

import { useUploadAssetPreviewUrl } from '#/utils/upload-asset-preview';

const props = withDefaults(
  defineProps<{
    accessToken?: null | string;
    appName?: string;
    iconUrl?: string;
    packageName?: string;
    size?: number;
  }>(),
  {
    accessToken: '',
    appName: '',
    iconUrl: '',
    packageName: '',
    size: 40,
  },
);

const imageLoadError = ref(false);
const previewUrl = useUploadAssetPreviewUrl(
  () => props.iconUrl,
  () => props.accessToken,
);
const displayName = computed(
  () => props.appName.trim() || props.packageName.trim() || 'Android 应用',
);
const showImage = computed(
  () => Boolean(previewUrl.value) && !imageLoadError.value,
);
const accessibleLabel = computed(() =>
  showImage.value
    ? `${displayName.value}应用图标`
    : `${displayName.value}应用图标占位符`,
);
const placeholderText = computed(() =>
  displayName.value.slice(0, 1).toUpperCase(),
);

watch(
  () => [props.iconUrl, previewUrl.value],
  () => {
    imageLoadError.value = false;
  },
);
</script>

<template>
  <Avatar
    class="app-release-icon"
    :alt="`${displayName}应用图标`"
    :aria-label="accessibleLabel"
    role="img"
    shape="square"
    :size="size"
    :src="showImage ? previewUrl : undefined"
    @error="imageLoadError = true"
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
