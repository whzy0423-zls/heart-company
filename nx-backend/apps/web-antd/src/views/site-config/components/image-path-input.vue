<script setup lang="ts">
import type { UploadRequestOption } from 'ant-design-vue/es/vc-upload/interface';

import { computed, ref, watch } from 'vue';

import { useAccessStore } from '@vben/stores';

import { Button, Image, Input, message, Upload } from 'ant-design-vue';

import { uploadFileApi } from '#/api';
import { useUploadAssetPreviewUrl } from '#/utils/upload-asset-preview';

import {
  getImagePreviewState,
  selectStoredImagePath,
} from './image-path-input';

const props = withDefaults(
  defineProps<{
    dir?: string;
    emptyText?: string;
    placeholder?: string;
    showPath?: boolean;
    storeObjectUrl?: boolean;
    uploadText?: string;
    variant?: 'image' | 'input';
  }>(),
  {
    dir: 'site',
    emptyText: '未设置图片',
    placeholder: '图片路径或 URL',
    showPath: false,
    storeObjectUrl: false,
    uploadText: '上传',
    variant: 'image',
  },
);
const value = defineModel<string>('value', { default: '' });
const accessStore = useAccessStore();
const uploading = ref(false);
const imageLoadError = ref(false);
const previewSrc = useUploadAssetPreviewUrl(
  () => value.value,
  () => accessStore.accessToken,
);
const previewState = computed(() =>
  getImagePreviewState({
    emptyText: props.emptyText,
    imageError: imageLoadError.value,
    previewSrc: previewSrc.value,
    uploading: uploading.value,
    value: value.value,
  }),
);

watch(
  () => [value.value, previewSrc.value],
  () => {
    imageLoadError.value = false;
  },
);

function markImageLoadError() {
  imageLoadError.value = true;
}

async function customRequest(options: UploadRequestOption) {
  const file = options.file as File;
  uploading.value = true;
  try {
    const result = await uploadFileApi(file, props.dir);
    value.value = selectStoredImagePath(result, props.storeObjectUrl);
    options.onSuccess?.(result, file as any);
    message.success('上传成功');
  } catch (error) {
    options.onError?.(error as Error);
  } finally {
    uploading.value = false;
  }
}
</script>

<template>
  <div
    class="image-uploader"
    :class="{ 'image-uploader--compact': props.variant === 'input' }"
  >
    <Upload
      accept="image/*"
      :custom-request="customRequest"
      :disabled="uploading"
      :max-count="1"
      :show-upload-list="false"
    >
      <div
        class="image-uploader__preview"
        :class="{ 'image-uploader__preview--uploading': uploading }"
        role="button"
        tabindex="0"
      >
        <Image
          v-if="previewState.showImage"
          :height="props.variant === 'input' ? 64 : 88"
          :preview="false"
          :src="previewSrc"
          :width="props.variant === 'input' ? 64 : 88"
          @error="markImageLoadError"
        />
        <span v-else class="image-uploader__empty">
          {{ previewState.text }}
        </span>
      </div>
    </Upload>
    <div class="image-uploader__actions">
      <Upload
        accept="image/*"
        :custom-request="customRequest"
        :disabled="uploading"
        :max-count="1"
        :show-upload-list="false"
      >
        <Button :loading="uploading" type="primary">
          {{ props.uploadText }}
        </Button>
      </Upload>
      <Button :disabled="uploading || !value" @click="value = ''">清除</Button>
      <Input
        v-if="props.showPath"
        v-model:value="value"
        class="image-uploader__path"
        :placeholder="placeholder"
      />
    </div>
  </div>
</template>

<style scoped>
.image-uploader {
  display: flex;
  gap: 16px;
  align-items: center;
}

.image-uploader__preview {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 112px;
  height: 112px;
  overflow: hidden;
  cursor: pointer;
  background: hsl(var(--accent) / 38%);
  border: 1px dashed hsl(var(--border));
  border-radius: 8px;
  transition:
    border-color 0.2s ease,
    background-color 0.2s ease,
    opacity 0.2s ease;
}

.image-uploader__preview:hover {
  background: hsl(var(--accent) / 56%);
  border-color: hsl(var(--primary) / 64%);
}

.image-uploader__preview--uploading {
  cursor: wait;
  opacity: 0.72;
}

.image-uploader__preview :deep(.ant-image-img) {
  object-fit: contain;
}

.image-uploader__empty {
  font-size: 13px;
  color: hsl(var(--muted-foreground));
}

.image-uploader__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.image-uploader--compact .image-uploader__preview {
  width: 80px;
  height: 80px;
}

.image-uploader__path {
  width: min(100%, 420px);
}

@media (max-width: 640px) {
  .image-uploader {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
