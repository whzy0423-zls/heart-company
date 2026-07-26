<script setup lang="ts">
import type {
  ClassroomContent,
  ClassroomUploadTask,
} from '#/api/core/classroom';
import { resolveUploadRetryContext } from './upload-flow';
import { onMounted, ref } from 'vue';
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

const loading = ref(false);
const error = ref('');
const tasks = ref<ClassroomUploadTask[]>([]);
const selectedContentId = ref<number>();
const selectedFile = ref<File>();
const uploading = ref(false);
const uploadPercent = ref(0);
const activeTaskId = ref<number>();
const uploadContexts = new Map<number, { contentId: number; file: File }>();
const columns = [
  { dataIndex: 'id', title: '任务' },
  { dataIndex: 'contentId', title: '课件' },
  { key: 'progress', title: '上传进度' },
  { dataIndex: 'status', title: '状态' },
  { key: 'action', title: '操作' },
];
const statusText: Record<string, string> = {
  initiated: '等待上传',
  uploading: '上传中',
  completing: '正在合并',
  completed: '上传完成',
  processing: '媒体处理中',
  ready: '可发布',
  failed: '失败',
  aborted: '已终止',
  expired: '已过期',
};

function progress(task: ClassroomUploadTask) {
  return task.status === 'completed'
    ? 100
    : task.status === 'uploading'
      ? 60
      : 10;
}
async function load() {
  loading.value = true;
  error.value = '';
  try {
    tasks.value = (
      await getClassroomUploadTasksApi({ page: 1, pageSize: 50 })
    ).items;
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
      activeTaskId.value = task.id;
      try {
        await abortClassroomUploadApi(task.id);
        uploadContexts.delete(task.id);
        message.success('上传任务已终止');
        await load();
      } catch (cause) {
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
}
function crc64Chunk(
  bytes: Uint8Array,
  seed: [number, number] = [0, 0],
): [number, number] {
  let [high, low] = seed;
  high = ~high >>> 0;
  low = ~low >>> 0;
  for (const byte of bytes) {
    low ^= byte;
    for (let bit = 0; bit < 8; bit++) {
      const leastSignificantBit = low & 1;
      low = ((low >>> 1) | (high << 31)) >>> 0;
      high >>>= 1;
      if (leastSignificantBit) {
        high = (high ^ 0xc96c5795) >>> 0;
        low = (low ^ 0xd7870f42) >>> 0;
      }
    }
  }
  return [~high >>> 0, ~low >>> 0];
}
function crc64Decimal([high, low]: [number, number]) {
  if (high === 0 && low === 0) return '0';
  const digits: number[] = [];
  while (high || low) {
    const highQuotient = Math.floor(high / 10);
    const remainder = high - highQuotient * 10;
    const combined = remainder * 0x100000000 + low;
    high = highQuotient;
    low = Math.floor(combined / 10);
    digits.push(combined % 10);
  }
  return digits.reverse().join('');
}
async function fileCRC64(file: File) {
  const chunkSize = 8 << 20;
  let state: [number, number] = [0, 0];
  for (let offset = 0; offset < file.size; offset += chunkSize)
    state = crc64Chunk(
      new Uint8Array(
        await file.slice(offset, offset + chunkSize).arrayBuffer(),
      ),
      state,
    );
  return `crc64:${crc64Decimal(state)}`;
}
async function performUpload(file: File, contentId: number) {
  uploading.value = true;
  uploadPercent.value = 0;
  try {
    const mime = file.type as
      | 'audio/mp4'
      | 'audio/mpeg'
      | 'audio/x-m4a'
      | 'video/mp4';
    const checksum = await fileCRC64(file);
    const { task } = await initiateClassroomUploadApi({
      checksum,
      contentId,
      contentType: mime,
      filename: file.name,
      sizeBytes: file.size,
    });
    activeTaskId.value = task.id;
    uploadContexts.set(task.id, { contentId, file });
    const parts: Array<{ etag: string; partNumber: number }> = [];
    for (
      let partNumber = 1, offset = 0;
      offset < file.size;
      partNumber++, offset += task.partSize
    ) {
      const signed = await signClassroomUploadPartApi(task.id, partNumber);
      const response = await fetch(signed.url, {
        body: file.slice(offset, offset + task.partSize),
        method: 'PUT',
      });
      if (!response.ok) throw new Error(`第 ${partNumber} 片上传失败`);
      parts.push({
        etag: (response.headers.get('ETag') || '').replaceAll('"', ''),
        partNumber,
      });
      uploadPercent.value = Math.min(
        100,
        Math.round(((offset + task.partSize) / file.size) * 100),
      );
    }
    await completeClassroomUploadApi(task.id, parts);
    uploadContexts.delete(task.id);
    message.success('媒体上传完成，正在处理');
    await load();
  } catch (cause) {
    message.error(cause instanceof Error ? cause.message : '媒体上传失败');
  } finally {
    uploading.value = false;
    activeTaskId.value = undefined;
  }
}
async function uploadFile() {
  if (!props.canUpload || !selectedContentId.value || !selectedFile.value)
    return message.warning('请选择草稿课件和媒体文件');
  await performUpload(selectedFile.value, selectedContentId.value);
}
async function retry(task: ClassroomUploadTask) {
  const context = resolveUploadRetryContext(
    task,
    uploadContexts,
    selectedFile.value,
  );
  if (!context) {
    selectedContentId.value = task.contentId;
    return message.warning('请选择原媒体文件后重试，系统会重新发起分片上传');
  }
  selectedContentId.value = context.contentId;
  await performUpload(context.file, context.contentId);
}
onMounted(load);
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
          ><Tag>{{ statusText[record.status] || record.status }}</Tag></template
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
