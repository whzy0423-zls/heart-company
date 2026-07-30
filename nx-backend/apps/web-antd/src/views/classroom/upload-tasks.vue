<script setup lang="ts">
import type {
  ClassroomContent,
  ClassroomUploadTask,
} from '#/api/core/classroom';
import {
  canAbortUploadTask,
  classroomUploadMime,
  completeUploadWithStatusReconciliation,
  matchesClassroomContentType,
  matchesUploadIdentity,
  mergeUploadProgress,
  putSignedUploadPart,
  resolveUploadRetryContext,
  shouldAbortController,
} from './upload-flow';
import { uploadStatusLabel } from './classroom-view-model';
import { crc64File } from './upload-checksum';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
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
  reportClassroomUploadProgressApi,
  signClassroomUploadPartApi,
} from '#/api/core/classroom';

const props = defineProps<{
  canUpload?: boolean;
  contents?: ClassroomContent[];
}>();
const emit = defineEmits<{ uploaded: [] }>();

const initialLoading = ref(true);
const error = ref('');
const tasks = ref<ClassroomUploadTask[]>([]);
const selectedContentId = ref<number>();
const selectedFile = ref<File>();
const fileInput = ref<HTMLInputElement>();
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
let latestRequestTicket = 0;
let pollTimer: ReturnType<typeof setTimeout> | undefined;
let stopped = false;
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
const completingActiveUpload = computed(() => {
  if (!activeTaskId.value) return false;
  return tasks.value.some(
    (task) =>
      task.id === activeTaskId.value && task.status === 'completing',
  );
});

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
async function load(options: { silent?: boolean } = {}) {
  const requestTicket = ++latestRequestTicket;
  if (!options.silent && tasks.value.length === 0) initialLoading.value = true;
  try {
    const nextTasks = (
      await getClassroomUploadTasksApi({ page: 1, pageSize: 50 })
    ).items;
    if (requestTicket !== latestRequestTicket) return;
    tasks.value = nextTasks;
    error.value = '';
    for (const task of tasks.value)
      localProgress.set(
        task.id,
        mergeUploadProgress(
          localProgress.get(task.id) ?? {
            completedBytes: 0,
            totalBytes: task.totalBytes || task.expectedSize,
          },
          task,
        ),
      );
  } catch {
    if (requestTicket !== latestRequestTicket) return;
    if (!options.silent || tasks.value.length === 0)
      error.value = '上传任务加载失败，请重试。';
  } finally {
    if (requestTicket === latestRequestTicket) {
      initialLoading.value = false;
    }
  }
}
function pollDelay() {
  return tasks.value.some((task) =>
    ['initiating', 'initiated', 'uploading', 'completing'].includes(task.status),
  )
    ? 5000
    : 30_000;
}
function schedulePoll() {
  if (pollTimer) clearTimeout(pollTimer);
  if (stopped || document.hidden) return;
  pollTimer = setTimeout(async () => {
    await load({ silent: true });
    schedulePoll();
  }, pollDelay());
}
function handleVisibilityChange() {
  if (document.hidden) {
    if (pollTimer) clearTimeout(pollTimer);
    pollTimer = undefined;
    return;
  }
  void load({ silent: true }).finally(schedulePoll);
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
function clearSelectedFile() {
  selectedFile.value = undefined;
  if (fileInput.value) fileInput.value.value = '';
}
async function cancelCurrentUpload() {
  const taskId = activeTaskId.value;
  uploadController.value?.abort();
  if (!taskId) return;
  try {
    await abortClassroomUploadApi(taskId);
    uploadContexts.delete(taskId);
    message.info('上传已取消');
    await load();
  } catch (cause) {
    if ((cause as { status?: number })?.status !== 409)
      message.error(cause instanceof Error ? cause.message : '取消上传失败');
  }
}
async function performUpload(
  file: File,
  contentId: number,
  retryTask?: ClassroomUploadTask,
) {
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
    if (
      retryTask &&
      !matchesUploadIdentity(
        {
          checksum: retryTask.expectedChecksum,
          contentId: retryTask.contentId,
          filename: retryTask.originalFilename,
          size: retryTask.expectedSize,
        },
        file,
        contentId,
        checksum,
      )
    )
      return message.error('重试文件与原任务不一致，请选择原文件');
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
      void reportClassroomUploadProgressApi(task.id, {
        completedBytes: Math.min(file.size, offset + task.partSize),
        completedParts: parts.length,
      }).catch(() => undefined);
      uploadPercent.value = Math.min(
        100,
        20 + Math.round(((offset + task.partSize) / file.size) * 80),
      );
    }
    tasks.value = tasks.value.map((item) =>
      item.id === task.id ? { ...item, status: 'completing' } : item,
    );
    const completionState = await completeUploadWithStatusReconciliation({
      complete: () => completeClassroomUploadApi(task.id, parts),
      readTask: async () => {
        const page = await getClassroomUploadTasksApi({ page: 1, pageSize: 50 });
        const latest = page.items.find((item) => item.id === task.id);
        if (latest)
          tasks.value = tasks.value.map((item) =>
            item.id === latest.id ? latest : item,
          );
        return latest;
      },
    });
    uploadContexts.delete(task.id);
    localProgress.set(task.id, {
      completedBytes: file.size,
      totalBytes: file.size,
    });
    if (completionState === 'processing')
      message.info('文件已上传，服务器正在处理媒体，请稍后查看状态');
    else message.success('媒体上传完成，正在处理');
    clearSelectedFile();
    selectedContentId.value = undefined;
    pendingRetryTask.value = undefined;
    retryConfirmed.value = false;
    emit('uploaded');
    await load();
  } catch (cause) {
    const failedTaskId = activeTaskId.value;
    if (failedTaskId) {
      uploadContexts.delete(failedTaskId);
      const failedTask = tasks.value.find((item) => item.id === failedTaskId);
      if (!failedTask || canAbortUploadTask(failedTask.status)) {
        try {
          await abortClassroomUploadApi(failedTaskId);
        } catch (abortCause) {
          if ((abortCause as { status?: number })?.status !== 409)
            message.warning('上传任务清理失败，请在任务列表中手动终止');
        }
      } else if (failedTask.status === 'completing') {
        message.info('文件已上传，服务器仍在处理媒体，请稍后刷新查看');
      }
      await load();
    }
    const detail = cause instanceof Error ? cause.message : '';
    message.error(
      detail === 'upload conflict'
        ? '该课件存在未结束的上传任务，请先终止后重试'
        : detail || '媒体上传失败',
    );
  } finally {
    uploading.value = false;
    activeTaskId.value = undefined;
    uploadController.value = undefined;
  }
}
async function uploadFile() {
  if (!props.canUpload) return message.warning('当前账号没有媒体上传权限');
  if (!selectedContentId.value) return message.warning('请选择草稿课件');
  if (!selectedFile.value) return message.warning('请选择媒体文件');
  const selectedContent = props.contents?.find(
    (item) => item.id === selectedContentId.value,
  );
  const selectedMime = classroomUploadMime(selectedFile.value) as
    | 'audio/mp4'
    | 'audio/mpeg'
    | 'audio/x-m4a'
    | 'video/mp4'
    | '';
  if (
    selectedContent &&
    selectedMime &&
    !matchesClassroomContentType(selectedContent.contentType, selectedMime)
  )
    return message.error(
      selectedContent.contentType === 'video'
        ? '当前课件类型为视频，请选择 MP4 视频文件'
        : '当前课件类型为音频，请选择 MP3 或 M4A 音频文件',
    );
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
  await performUpload(
    selectedFile.value,
    selectedContentId.value,
    pendingRetryTask.value,
  );
}
async function retry(task: ClassroomUploadTask) {
  const context = resolveUploadRetryContext(task, uploadContexts);
  if (!context) {
    pendingRetryTask.value = task;
    clearSelectedFile();
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
    clearSelectedFile();
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
  stopped = true;
  if (uploading.value && !completingActiveUpload.value)
    message.warning('离开上传页面，当前上传已取消');
  uploadController.value?.abort();
  if (activeTaskId.value) {
    const task = tasks.value.find((item) => item.id === activeTaskId.value);
    if (!task || canAbortUploadTask(task.status))
      void abortClassroomUploadApi(activeTaskId.value).catch(() => undefined);
  }
  window.removeEventListener('beforeunload', beforeUnload);
  document.removeEventListener('visibilitychange', handleVisibilityChange);
  if (pollTimer) clearTimeout(pollTimer);
});
onMounted(() => {
  window.addEventListener('beforeunload', beforeUnload);
  document.addEventListener('visibilitychange', handleVisibilityChange);
  void load().finally(schedulePoll);
});
</script>

<template>
  <Card :loading="initialLoading && tasks.length === 0" title="上传任务">
    <div class="upload-panel" v-if="canUpload">
      <div class="upload-field">
        <span class="upload-label">1. 选择草稿课件</span>
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
      </div>
      <div class="upload-field">
        <span class="upload-label">2. 选择媒体文件</span>
        <input
          ref="fileInput"
          class="native-file-input"
          type="file"
          accept="video/mp4,audio/mpeg,audio/mp4,audio/x-m4a"
          aria-label="选择视频或音频文件"
          @change="chooseFile"
        />
        <div class="file-picker">
          <Button @click="fileInput?.click()">选择文件</Button>
          <span class="selected-file-name" :title="selectedFile?.name">
            {{ selectedFile?.name || '尚未选择文件' }}
          </span>
        </div>
      </div>
      <Button
        type="primary"
        :loading="uploading"
        :disabled="!selectedContentId || !selectedFile || uploading"
        @click="uploadFile"
        >开始上传</Button
      >
      <Progress v-if="uploading" :percent="uploadPercent" />
      <Button
        v-if="uploading && !completingActiveUpload"
        danger
        @click="cancelCurrentUpload"
        >取消当前上传</Button
      >
    </div>
    <Alert v-if="error" type="error" show-icon :message="error"
      ><template #action><Button @click="load()">重试</Button></template></Alert
    >
    <Empty
      v-else-if="!initialLoading && tasks.length === 0"
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
              v-if="canAbortUploadTask(record.status)"
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
.upload-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.upload-label {
  color: hsl(var(--muted-foreground));
  font-size: 13px;
}
.native-file-input {
  height: 1px;
  overflow: hidden;
  position: absolute;
  width: 1px;
  clip-path: inset(50%);
}
.file-picker {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}
.selected-file-name {
  overflow: hidden;
  color: hsl(var(--foreground));
  text-overflow: ellipsis;
  white-space: nowrap;
}
.native-file-input:focus-visible + .file-picker {
  outline: 2px solid hsl(var(--primary));
  outline-offset: 2px;
}
@media (max-width: 768px) {
  .upload-panel {
    grid-template-columns: 1fr;
  }
}
</style>
