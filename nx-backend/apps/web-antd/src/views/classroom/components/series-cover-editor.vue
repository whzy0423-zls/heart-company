<script setup lang="ts">
import type {
  ClassroomCoverAspectRatio,
  ClassroomSeries,
} from '#/api/core/classroom';

import { ref, watch } from 'vue';

import {
  Alert,
  Button,
  Form,
  message,
  Modal,
  Progress,
  Space,
} from 'ant-design-vue';

import {
  deleteClassroomSeriesCoverApi,
  setClassroomSeriesCoverSettingsApi,
  uploadClassroomSeriesCoverApi,
} from '#/api/core/classroom';

const props = withDefaults(
  defineProps<{
    disabled?: boolean;
    series: ClassroomSeries;
  }>(),
  { disabled: false },
);

const emit = defineEmits<{
  saved: [value: ClassroomSeries];
}>();

const current = ref(props.series);
const selectedFile = ref<File>();
const selectedRatio = ref<ClassroomCoverAspectRatio>('16:9');
const busyAction = ref<'' | 'delete' | 'ratio' | 'upload'>('');
const uploadProgress = ref(0);

const ratioOptions: Array<{
  label: string;
  value: ClassroomCoverAspectRatio;
}> = [
  { label: '16:9', value: '16:9' },
  { label: '9:16', value: '9:16' },
  { label: '1:1', value: '1:1' },
];

watch(
  () => props.series,
  (series) => {
    current.value = series;
    selectedRatio.value = series.coverAspectRatio || '16:9';
    selectedFile.value = undefined;
    uploadProgress.value = 0;
  },
  { immediate: true },
);

function ratioStyle(value: ClassroomCoverAspectRatio) {
  return value.replace(':', ' / ');
}

function replaceSeries(value: ClassroomSeries) {
  current.value = value;
  selectedRatio.value = value.coverAspectRatio || '16:9';
  selectedFile.value = undefined;
  emit('saved', value);
}

function onPickFile(event: Event) {
  const target = event.target as HTMLInputElement;
  selectedFile.value = target.files?.[0];
}

async function upload() {
  if (!selectedFile.value) {
    message.warning('请选择封面图片');
    return;
  }
  busyAction.value = 'upload';
  uploadProgress.value = 35;
  try {
    const updated = await uploadClassroomSeriesCoverApi(
      current.value.id,
      selectedFile.value,
      current.value.updatedAt,
    );
    uploadProgress.value = 100;
    replaceSeries(updated);
    message.success('系列封面已上传');
  } catch (error) {
    message.error(error instanceof Error ? error.message : '系列封面上传失败');
  } finally {
    busyAction.value = '';
    uploadProgress.value = 0;
  }
}

async function saveRatio() {
  if (selectedRatio.value === current.value.coverAspectRatio) return;
  busyAction.value = 'ratio';
  try {
    const updated = await setClassroomSeriesCoverSettingsApi(
      current.value.id,
      selectedRatio.value,
      current.value.updatedAt,
    );
    replaceSeries(updated);
    message.success('系列封面比例已保存');
  } catch (error) {
    message.error(
      error instanceof Error ? error.message : '系列封面比例保存失败',
    );
  } finally {
    busyAction.value = '';
  }
}

function removeCover() {
  Modal.confirm({
    title: '删除系列手动封面？',
    content: '删除后会自动回退到系列第一节课的封面。',
    async onOk() {
      busyAction.value = 'delete';
      try {
        const updated = await deleteClassroomSeriesCoverApi(
          current.value.id,
          current.value.updatedAt,
        );
        replaceSeries(updated);
        message.success('系列封面已删除');
      } catch (error) {
        message.error(
          error instanceof Error ? error.message : '系列封面删除失败',
        );
      } finally {
        busyAction.value = '';
      }
    },
  });
}
</script>

<template>
  <Form layout="vertical" class="series-cover-editor">
    <Alert
      type="info"
      show-icon
      message="系列封面管理"
      description="无手动封面时，自动回退到第一节课封面；默认比例为 16:9。"
    />
    <p class="cover-fallback-hint">
      无手动封面时，自动回退到第一节课封面；默认比例为 16:9。
    </p>
    <div
      class="cover-preview"
      :style="{ aspectRatio: ratioStyle(selectedRatio) }"
    >
      <img v-if="current.coverUrl" :src="current.coverUrl" alt="系列封面预览" />
      <div v-else class="cover-preview__empty">暂无系列封面</div>
      <div class="cover-preview__meta">
        <span>{{
          current.manualCoverObjectKey ? '手动封面' : '自动回退封面'
        }}</span>
        <span>{{ current.coverAspectRatio || selectedRatio }}</span>
      </div>
    </div>
    <Form.Item label="封面比例">
      <Space wrap>
        <Button
          v-for="option in ratioOptions"
          :key="option.value"
          :type="selectedRatio === option.value ? 'primary' : 'default'"
          :disabled="disabled || busyAction !== ''"
          @click="selectedRatio = option.value"
        >
          {{ option.label }}
        </Button>
      </Space>
      <span class="field-hint">可选择 16:9、9:16 或 1:1，默认 16:9。</span>
    </Form.Item>
    <Form.Item label="上传或替换封面">
      <input
        accept="image/jpeg,image/png,image/webp"
        class="cover-input"
        type="file"
        :disabled="disabled || busyAction !== ''"
        @change="onPickFile"
      />
      <span v-if="selectedFile" class="field-hint">
        已选择：{{ selectedFile.name }}
      </span>
      <span v-else class="field-hint">支持 JPEG、PNG、WebP。</span>
      <div v-if="busyAction === 'upload'" class="upload-progress">
        <span>上传进度</span>
        <Progress :percent="uploadProgress" status="active" />
      </div>
    </Form.Item>
    <Space wrap>
      <Button
        type="primary"
        :disabled="disabled || busyAction !== '' || !selectedFile"
        :loading="busyAction === 'upload'"
        @click="upload"
      >
        上传封面
      </Button>
      <Button
        :disabled="
          disabled ||
          busyAction !== '' ||
          selectedRatio === current.coverAspectRatio
        "
        :loading="busyAction === 'ratio'"
        @click="saveRatio"
      >
        保存比例
      </Button>
      <Button
        danger
        :disabled="
          disabled || busyAction !== '' || !current.manualCoverObjectKey
        "
        :loading="busyAction === 'delete'"
        @click="removeCover"
      >
        删除封面
      </Button>
    </Space>
  </Form>
</template>

<style scoped>
.series-cover-editor {
  display: grid;
  gap: 16px;
  margin-top: 20px;
  padding-top: 20px;
  border-top: 1px solid hsl(var(--border));
}
.cover-preview {
  position: relative;
  overflow: hidden;
  max-height: 420px;
  border: 1px solid hsl(var(--border));
  border-radius: 12px;
  background: hsl(var(--muted));
}
.cover-preview img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.cover-preview__empty {
  display: grid;
  min-height: 220px;
  place-items: center;
  color: hsl(var(--muted-foreground));
}
.cover-preview__meta {
  position: absolute;
  right: 12px;
  bottom: 12px;
  left: 12px;
  display: flex;
  justify-content: space-between;
  gap: 12px;
  color: #fff;
  text-shadow: 0 1px 2px rgb(0 0 0 / 50%);
}
.field-hint {
  display: block;
  margin-top: 6px;
  color: hsl(var(--muted-foreground));
  line-height: 1.5;
}
.cover-fallback-hint {
  margin: 0;
  color: hsl(var(--muted-foreground));
  line-height: 1.5;
}
.cover-input {
  width: 100%;
}

:deep(button:focus-visible),
.cover-input:focus-visible {
  outline: 2px solid hsl(var(--primary));
  outline-offset: 2px;
}

@media (max-width: 768px) {
  .cover-preview__empty {
    min-height: 160px;
  }
}
</style>
