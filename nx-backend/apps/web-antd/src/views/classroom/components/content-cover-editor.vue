<script setup lang="ts">
import type {
  ClassroomContent,
  ClassroomCoverAspectRatio,
} from '#/api/core/classroom';

import { ref, watch } from 'vue';
import {
  Alert,
  Button,
  Form,
  Modal,
  Progress,
  Space,
  message,
} from 'ant-design-vue';
import {
  deleteClassroomContentCoverApi,
  setClassroomContentCoverSettingsApi,
  uploadClassroomContentCoverApi,
} from '#/api/core/classroom';

const props = defineProps<{
  content: ClassroomContent;
}>();

const emit = defineEmits<{
  saved: [value: ClassroomContent];
  cancel: [];
}>();

const current = ref(props.content);
const selectedFile = ref<File>();
const selectedRatio = ref<ClassroomCoverAspectRatio>('16:9');
const busyAction = ref<'delete' | 'ratio' | 'upload' | ''>('');
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
  () => props.content,
  (content) => {
    current.value = content;
    selectedRatio.value = content.coverAspectRatio || '16:9';
    selectedFile.value = undefined;
    uploadProgress.value = 0;
  },
  { immediate: true },
);

function coverSourceLabel(content: ClassroomContent) {
  if (content.manualCoverObjectKey) return '手动上传';
  if (content.coverSource === 'audio-default') return '音频默认封面';
  if (content.coverSource === 'generated') return '视频首帧';
  if (content.coverSource === 'legacy') return '历史封面';
  return '暂无封面';
}

function ratioStyle(value: ClassroomCoverAspectRatio) {
  return value.replace(':', ' / ');
}

function replaceContent(value: ClassroomContent) {
  current.value = value;
  selectedRatio.value = value.coverAspectRatio || '16:9';
  selectedFile.value = undefined;
  emit('saved', value);
}

async function upload() {
  if (!selectedFile.value) {
    message.warning('请选择封面图片');
    return;
  }
  busyAction.value = 'upload';
  uploadProgress.value = 35;
  try {
    const updated = await uploadClassroomContentCoverApi(
      current.value.id,
      selectedFile.value,
      current.value.updatedAt,
    );
    uploadProgress.value = 100;
    replaceContent(updated);
    message.success('封面已上传');
  } catch (cause) {
    message.error(cause instanceof Error ? cause.message : '封面上传失败');
  } finally {
    busyAction.value = '';
    uploadProgress.value = 0;
  }
}

async function saveRatio() {
  if (selectedRatio.value === current.value.coverAspectRatio) return;
  busyAction.value = 'ratio';
  try {
    const updated = await setClassroomContentCoverSettingsApi(
      current.value.id,
      selectedRatio.value,
      current.value.updatedAt,
    );
    replaceContent(updated);
    message.success('封面比例已保存');
  } catch (cause) {
    message.error(cause instanceof Error ? cause.message : '封面比例保存失败');
  } finally {
    busyAction.value = '';
  }
}

function removeCover() {
  Modal.confirm({
    title: '删除手动封面？',
    content: '删除后会回退到视频首帧或默认音频封面。',
    async onOk() {
      busyAction.value = 'delete';
      try {
        const updated = await deleteClassroomContentCoverApi(
          current.value.id,
          current.value.updatedAt,
        );
        replaceContent(updated);
        message.success('封面已删除');
      } catch (cause) {
        message.error(cause instanceof Error ? cause.message : '封面删除失败');
      } finally {
        busyAction.value = '';
      }
    },
  });
}

function onPickFile(event: Event) {
  const target = event.target as HTMLInputElement;
  const file = target.files?.[0];
  selectedFile.value = file;
}
</script>

<template>
  <Form layout="vertical" class="content-cover-editor">
    <Alert
      type="info"
      show-icon
      message="封面管理"
      description="手动封面优先；删除后回退到视频首帧或默认音频封面。"
    />
    <div class="cover-preview" :style="{ aspectRatio: ratioStyle(selectedRatio) }">
      <img v-if="current.coverUrl" :src="current.coverUrl" alt="" />
      <div v-else class="cover-preview__empty">暂无封面</div>
      <div class="cover-preview__meta">
        <span>来源：{{ coverSourceLabel(current) }}</span>
        <span>比例：{{ current.coverAspectRatio || selectedRatio }}</span>
      </div>
    </div>
    <Form.Item label="封面比例">
      <Space wrap>
        <Button
          v-for="option in ratioOptions"
          :key="option.value"
          :type="selectedRatio === option.value ? 'primary' : 'default'"
          @click="selectedRatio = option.value"
          >{{ option.label }}</Button
        >
      </Space>
      <span class="field-hint">默认 16:9；保存后会刷新当前封面版本。</span>
    </Form.Item>
    <Form.Item label="上传封面">
      <input
        accept="image/jpeg,image/png,image/webp"
        class="cover-input"
        type="file"
        :disabled="busyAction !== ''"
        @change="onPickFile"
      />
      <span v-if="selectedFile" class="field-hint">已选择：{{ selectedFile.name }}</span>
      <span v-else class="field-hint">支持 JPEG、PNG、WebP。</span>
      <div v-if="busyAction === 'upload'" class="upload-progress">
        <span>上传进度</span>
        <Progress :percent="uploadProgress" status="active" />
      </div>
    </Form.Item>
    <Space wrap>
      <Button
        type="primary"
        :disabled="busyAction !== '' || !selectedFile"
        :loading="busyAction === 'upload'"
        @click="upload"
        >上传封面</Button
      >
      <Button
        :disabled="busyAction !== '' || selectedRatio === current.coverAspectRatio"
        :loading="busyAction === 'ratio'"
        @click="saveRatio"
        >保存比例</Button
      >
      <Button
        danger
        :disabled="busyAction !== '' || !current.manualCoverObjectKey"
        :loading="busyAction === 'delete'"
        @click="removeCover"
        >删除封面</Button
      >
      <Button :disabled="busyAction !== ''" @click="emit('cancel')">关闭</Button>
    </Space>
  </Form>
</template>

<style scoped>
.content-cover-editor {
  display: grid;
  gap: 16px;
}
.cover-preview {
  position: relative;
  overflow: hidden;
  border-radius: 12px;
  background: hsl(var(--muted));
  border: 1px solid hsl(var(--border));
}
.cover-preview img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.cover-preview__empty {
  min-height: 220px;
  display: grid;
  place-items: center;
  color: hsl(var(--muted-foreground));
}
.cover-preview__meta {
  position: absolute;
  left: 12px;
  right: 12px;
  bottom: 12px;
  display: flex;
  justify-content: space-between;
  gap: 12px;
  color: #fff;
  text-shadow: 0 1px 2px rgba(0, 0, 0, 0.5);
}
.field-hint {
  display: block;
  margin-top: 6px;
  color: hsl(var(--muted-foreground));
  line-height: 1.5;
}
.cover-input {
  width: 100%;
}
</style>
