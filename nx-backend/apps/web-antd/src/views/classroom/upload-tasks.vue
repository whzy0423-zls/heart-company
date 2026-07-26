<script setup lang="ts">
import type {
  ClassroomContent,
  ClassroomUploadTask,
} from '#/api/core/classroom';
import {
  classroomUploadMime,
  putSignedUploadPart,
  resolveUploadRetryContext,
  shouldAbortController,
} from './upload-flow';
import { uploadStatusLabel } from './classroom-view-model';
import { crc64File } from './upload-checksum';
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import {
  Alert,
  Button,
  Card,
  Empty,
  Modal,
  Progress,
  Select,
  Space,
  Table,
  Tag,
  message,
} from 'ant-design-vue';
import {
  abortClassroomUploadApi,
  completeClassroomUploadApi,
  getClassroomUploadTasksApi,
  initiateClassroomUploadApi,
  signClassroomUploadPartApi,
} from '#/api/core/classroom';

const props = defineProps<{
  canUpload?: boolean;
  contents?: ClassroomContent[];
}>();
const emit = defineEmits<{ uploaded: [] }>();

const loading = ref(false);
const error = ref('');
const tasks = ref<ClassroomUploadTask[]>([]);
const selectedContentId = ref<number>();
const selectedFile = ref<File>();
const uploading = ref(false);
const uploadPercent = ref(0);
const activeTaskId = ref<number>();
const pendingRetryTask = ref<ClassroomUploadTask>();
const retryConfirmed = ref(false);
const uploadController = ref<AbortController>();
const uploadContexts = new Map<
  number,
  {
    checksum: string;
    contentId: number;
    file: File;
    filename: string;
    size: number;
  }
>();
const localProgress = new Map<
  number,
  { completedBytes: number; totalBytes: number }
>();
const columns = [
  { dataIndex: 'id', title: '任务' },
  { dataIndex: 'contentId', title: '课件' },
  { key: 'progress', title: '上传进度' },
  { dataIndex: 'status', title: '状态' },
  { key: 'action', title: '操作' },
];
function contentStatus(task: ClassroomUploadTask) {
  return props.contents?.find((item) => item.id === task.contentId)?.status;
}
function displayStatus(task: ClassroomUploadTask) {
  return uploadStatusLabel(task.status, contentStatus(task));
}

function progress(task: ClassroomUploadTask) {
  if (task.id === activeTaskId.value) return uploadPercent.value;
  const remote = task as ClassroomUploadTask & {
    completedBytes?: number;
    progressPercent?: number;
  };
  if (typeof remote.progressPercent === 'number') return remote.progressPercent;
  const local = localProgress.get(task.id);
  if (local)
    return Math.min(
      100,
      Math.round((local.completedBytes / local.totalBytes) * 100),
    );
  return task.status === 'completed' ? 100 : 0;
}
async function load() {
  loading.value = true;
  error.value = '';
  try {
    tasks.value = (
      await getClassroomUploadTasksApi({ page: 1, pageSize: 50 })
    ).items;
    for (const task of tasks.value) {
      const remote = task as ClassroomUploadTask & { completedBytes?: number };
      if (typeof remote.completedBytes === 'number')
        localProgress.set(task.id, {
          completedBytes: remote.completedBytes,
          totalBytes: task.expectedSize,
        });
    }
  } catch {
    error.value = '上传任务加载失败，请重试。';
  } finally {
    loading.value = false;
  }
}
function confirmAbort(task: ClassroomUploadTask) {
  Modal.confirm({
    title: '终止上传任务？',
    content: '已上传的分片将被清理，之后需要重新选择文件上传。',
    async onOk() {
      const activeAtStart = activeTaskId.value;
      const controllerAtStart = uploadController.value;
      activeTaskId.value = task.id;
      try {
        if (shouldAbortController(activeAtStart, task.id))
          controllerAtStart?.abort();
        await abortClassroomUploadApi(task.id);
        uploadContexts.delete(task.id);
        message.success('上传任务已终止');
        await load();
      } catch (cause) {
        if ((cause as { status?: number })?.status === 409) {
          message.info('上传任务已完成或已终止');
          await load();
          return;
        }
        message.error(cause instanceof Error ? cause.message : '终止上传失败');
        throw cause;
      } finally {
        activeTaskId.value = undefined;
      }
    },
  });
}
function chooseFile(event: Event) {
  selectedFile.value = (event.target as HTMLInputElement).files?.[0];
  retryConfirmed.value = false;
}
async function performUpload(file: File, contentId: number) {
  if (uploading.value)
    return message.warning('已有上传任务正在进行，请等待完成或终止');
  uploading.value = true;
  uploadPercent.value = 0;
  const controller = new AbortController();
  uploadController.value = controller;
  try {
    const mime = classroomUploadMime(file) as
      | 'audio/mp4'
      | 'audio/mpeg'
      | 'audio/x-m4a'
      | 'video/mp4';
    if (!mime) return message.warning('请选择 MP4、MP3 或 M4A 媒体文件');
    const checksum = await crc64File(file, {
      signal: controller.signal,
      onProgress: (value) => {
        uploadPercent.value = Math.round(value * 0.2);
      },
    });
    const { task } = await initiateClassroomUploadApi({
      checksum,
      contentId,
      contentType: mime,
      filename: file.name,
      sizeBytes: file.size,
    });
    activeTaskId.value = task.id;
    uploadContexts.set(task.id, {
      checksum,
      contentId,
      file,
      filename: file.name,
      size: file.size,
    });
    tasks.value = [
      { ...task, status: 'uploading' },
      ...tasks.value.filter((item) => item.id !== task.id),
    ];
    localProgress.set(task.id, { completedBytes: 0, totalBytes: file.size });
    const parts: Array<{ etag: string; partNumber: number }> = [];
    for (
      let partNumber = 1, offset = 0;
      offset < file.size;
      partNumber++, offset += task.partSize
    ) {
      const signed = await signClassroomUploadPartApi(task.id, partNumber);
      const etag = await putSignedUploadPart(
        signed.url,
        file.slice(offset, offset + task.partSize),
        controller.signal,
      );
      parts.push({
        etag: etag.replaceAll('"', ''),
        partNumber,
      });
      localProgress.set(task.id, {
        completedBytes: Math.min(file.size, offset + task.partSize),
        totalBytes: file.size,
      });
      uploadPercent.value = Math.min(
        100,
        20 + Math.round(((offset + task.partSize) / file.size) * 80),
      );
    }
    await completeClassroomUploadApi(task.id, parts);
    uploadContexts.delete(task.id);
    localProgress.set(task.id, {
      completedBytes: file.size,
      totalBytes: file.size,
    });
    message.success('媒体上传完成，正在处理');
    selectedFile.value = undefined;
    selectedContentId.value = undefined;
    pendingRetryTask.value = undefined;
    retryConfirmed.value = false;
    emit('uploaded');
    await load();
  } catch (cause) {
    message.error(cause instanceof Error ? cause.message : '媒体上传失败');
  } finally {
    uploading.value = false;
    activeTaskId.value = undefined;
    uploadController.value = undefined;
  }
}
async function uploadFile() {
  if (!props.canUpload || !selectedContentId.value || !selectedFile.value)
    return message.warning('请选择草稿课件和媒体文件');
  if (
    pendingRetryTask.value &&
    selectedFile.value.size !== pendingRetryTask.value.expectedSize
  )
    return message.error('重试文件大小与原任务不一致，请选择原文件');
  if (pendingRetryTask.value && !retryConfirmed.value) {
    Modal.confirm({
      title: '确认使用此文件重试？',
      content: '系统会为该任务重新校验并发起分片上传。',
      onOk: () => {
        retryConfirmed.value = true;
        void uploadFile();
      },
    });
    return;
  }
  await performUpload(selectedFile.value, selectedContentId.value);
}
async function retry(task: ClassroomUploadTask) {
  const context = resolveUploadRetryContext(task, uploadContexts);
  if (!context) {
    pendingRetryTask.value = task;
    selectedFile.value = undefined;
    selectedContentId.value = task.contentId;
    return message.warning('请选择原媒体文件后重试，系统会重新发起分片上传');
  }
  selectedContentId.value = context.contentId;
  if (
    context.contentId !== task.contentId ||
    context.file.name !== context.filename ||
    context.file.size !== context.size ||
    context.file.size !== task.expectedSize
  )
    return message.error('重试文件大小与原任务不一致');
  await performUpload(context.file, context.contentId);
}
watch(selectedContentId, (value, previous) => {
  if (
    value !== previous &&
    (!pendingRetryTask.value || value !== pendingRetryTask.value.contentId)
  ) {
    pendingRetryTask.value = undefined;
    selectedFile.value = undefined;
    retryConfirmed.value = false;
  }
});
function beforeUnload(event: BeforeUnloadEvent) {
  if (uploading.value) {
    event.preventDefault();
    event.returnValue = '';
  }
}
onBeforeUnmount(() => {
  if (uploading.value) message.warning('离开上传页面，当前上传已取消');
  uploadController.value?.abort();
  if (activeTaskId.value)
    void abortClassroomUploadApi(activeTaskId.value).catch(() => undefined);
  window.removeEventListener('beforeunload', beforeUnload);
});
onMounted(() => {
  window.addEventListener('beforeunload', beforeUnload);
  void load();
});
</script>

<template>
  <Card :loading="loading" title="上传任务">
    <div class="upload-panel" v-if="canUpload">
      <Select
        v-model:value="selectedContentId"
        placeholder="选择草稿课件"
        :options="
          (contents ?? [])
            .filter(
              (item) => item.status === 'draft' || item.status === 'failed',
            )
            .map((item) => ({ label: item.title, value: item.id }))
        "
      />
      <input
        type="file"
        accept="video/mp4,audio/mpeg,audio/mp4,audio/x-m4a"
        aria-label="选择视频或音频文件"
        @change="chooseFile"
      />
      <Button type="primary" :loading="uploading" @click="uploadFile"
        >开始上传</Button
      >
      <Progress v-if="uploading" :percent="uploadPercent" />
      <Button v-if="uploading" danger @click="uploadController?.abort()"
        >取消当前上传</Button
      >
    </div>
    <Alert v-if="error" type="error" show-icon :message="error"
      ><template #action><Button @click="load">重试</Button></template></Alert
    >
    <Empty
      v-else-if="!loading && tasks.length === 0"
      description="暂无上传任务"
    />
    <Table
      v-else
      :columns="columns"
      :data-source="tasks"
      row-key="id"
      :pagination="false"
      :scroll="{ x: 720 }"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'progress'"
          ><Progress
            :percent="progress(record as ClassroomUploadTask)"
            size="small"
        /></template>
        <template v-else-if="column.dataIndex === 'status'"
          ><Tag>{{
            displayStatus(record as ClassroomUploadTask)
          }}</Tag></template
        >
        <template v-else-if="column.key === 'action'"
          ><Space
            ><Button
              v-if="record.status === 'failed' || record.status === 'expired'"
              :disabled="!canUpload"
              :loading="activeTaskId === record.id"
              @click="retry(record as ClassroomUploadTask)"
              >重试</Button
            ><Button
              v-if="!['completed', 'aborted'].includes(record.status)"
              danger
              :disabled="!canUpload"
              :loading="activeTaskId === record.id"
              @click="confirmAbort(record as ClassroomUploadTask)"
              >终止</Button
            ></Space
          ></template
        >
      </template>
    </Table>
  </Card>
</template>
<style scoped>
.upload-panel {
  display: grid;
  grid-template-columns: minmax(180px, 1fr) minmax(220px, 2fr) auto;
  gap: 12px;
  align-items: center;
  margin-bottom: 16px;
}
input[type='file']:focus-visible {
  outline: 2px solid hsl(var(--primary));
  outline-offset: 2px;
}
@media (max-width: 768px) {
  .upload-panel {
    grid-template-columns: 1fr;
  }
}
</style>
