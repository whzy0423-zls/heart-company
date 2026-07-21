<script setup lang="ts">
import type { AppRelease } from '#/api';

import { computed, onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { useAccessStore } from '@vben/stores';

import {
  Alert,
  Button,
  Card,
  Input,
  message,
  Modal,
  Progress,
  Space,
  Table,
  Tag,
  Tooltip,
  Upload,
} from 'ant-design-vue';

import {
  archiveAppReleaseApi,
  getAppReleaseListApi,
  publishAppReleaseApi,
  uploadAppReleaseApi,
} from '#/api';

import AppReleaseIcon from './app-release-icon.vue';
import {
  canArchiveRelease,
  canPublishRelease,
  formatReleaseFileSize,
  releaseStatusLabel,
  validateAPKFile,
} from './app-release-view';

const access = useAccessStore();
const canWrite = computed(() =>
  access.accessCodes.includes('Website:AppReleases:Write'),
);
const loading = ref(false);
const uploading = ref(false);
const error = ref('');
const items = ref<AppRelease[]>([]);
const current = ref<AppRelease | null>(null);
const total = ref(0);
const totalFileSize = ref(0);
const page = ref(1);
const pageSize = 20;
const selectedFile = ref<File | null>(null);
const releaseNotes = ref('');
const uploadProgress = ref(0);
const columns = [
  { title: '应用', key: 'app', width: 240 },
  { title: '版本', key: 'version', width: 130 },
  { title: '安装包', key: 'file', width: 190 },
  { title: '状态', key: 'status', width: 100 },
  { title: '更新时间', dataIndex: 'createdAt', width: 180 },
  { title: '更新说明', dataIndex: 'releaseNotes' },
  { title: '操作', key: 'action', width: 150 },
];
async function load() {
  loading.value = true;
  error.value = '';
  try {
    const data = await getAppReleaseListApi({ page: page.value, pageSize });
    items.value = data.items;
    current.value = data.current;
    total.value = data.total;
    totalFileSize.value = data.totalFileSize;
  } catch {
    error.value = '版本列表加载失败，请重试';
  } finally {
    loading.value = false;
  }
}
function beforeUpload(file: File) {
  const issue = validateAPKFile(file);
  if (issue) {
    message.warning(issue);
    return Upload.LIST_IGNORE;
  }
  selectedFile.value = file;
  return false;
}
function asRelease(record: Record<string, any>) {
  return record as AppRelease;
}
function appDisplayName(record: AppRelease) {
  return record.appName?.trim() || record.packageName?.trim() || 'Android 应用';
}
async function upload() {
  if (!selectedFile.value) {
    message.warning('请选择 APK 文件');
    return;
  }
  uploading.value = true;
  uploadProgress.value = 0;
  try {
    const created = await uploadAppReleaseApi(
      selectedFile.value,
      releaseNotes.value,
      (e) => {
        if (e.total)
          uploadProgress.value = Math.round((e.loaded / e.total) * 100);
      },
    );
    message.success(`已上传 ${created.versionName} (${created.versionCode})`);
    selectedFile.value = null;
    releaseNotes.value = '';
    await load();
  } catch {
    message.error('上传失败，请检查安装包或网络后重试');
  } finally {
    uploading.value = false;
  }
}
function publishRelease(record: AppRelease) {
  Modal.confirm({
    title: `发布 ${record.versionName}？`,
    content: '发布后官网会立即展示此版本，当前正式版将自动归档。',
    async onOk() {
      await publishAppReleaseApi(record.id);
      message.success('已发布');
      await load();
    },
  });
}
function archiveRelease(record: AppRelease) {
  Modal.confirm({
    title: `下架 ${record.versionName}？`,
    content: '下架后官网将暂时没有可下载版本。',
    okType: 'danger',
    async onOk() {
      await archiveAppReleaseApi(record.id);
      message.success('已下架');
      await load();
    },
  });
}
onMounted(load);
</script>

<template>
  <Page
    title="App 版本"
    description="管理官网 Android 安装包的上传、发布与下架"
  >
    <Alert v-if="error" type="error" show-icon :message="error">
      <template #action>
        <Button size="small" @click="load">重试</Button>
      </template>
    </Alert>
    <Card title="当前正式版本" class="release-card">
      <template v-if="current">
        <div class="current-release">
          <AppReleaseIcon
            :access-token="access.accessToken"
            :app-name="appDisplayName(current)"
            :icon-url="current.iconUrl"
            :package-name="current.packageName"
            :size="48"
          />
          <div class="current-release__content">
            <div class="current-release__heading">
              <strong>{{ appDisplayName(current) }}</strong>
              <Tag color="success">已发布</Tag>
            </div>
            <Tooltip :title="current.packageName">
              <div class="release-package" :title="current.packageName">
                {{ current.packageName }}
              </div>
            </Tooltip>
            <div class="current-release__details">
              <strong>
                {{ current.versionName }} ({{ current.versionCode }})
              </strong>
              <span>
                {{ current.fileName }} ·
                {{ formatReleaseFileSize(current.fileSize) }}
              </span>
              <Tag v-if="!current.fileAvailable" color="error">文件缺失</Tag>
            </div>
          </div>
        </div>
      </template>
      <span v-else>暂未发布 Android 版本</span>
    </Card>
    <Card v-if="canWrite" title="上传新版本" class="release-card">
      <Space direction="vertical" style="width: 100%">
        <Alert
          type="info"
          show-icon
          message="仅上传正式签名 APK，最大 300 MiB；版本号将从安装包 Manifest 自动读取。"
        />
        <Upload
          accept=".apk,application/vnd.android.package-archive"
          :max-count="1"
          :before-upload="beforeUpload"
          :show-upload-list="false"
        >
          <Button :disabled="uploading">选择 APK</Button>
        </Upload>
        <span v-if="selectedFile">
          {{ selectedFile.name }} ·
          {{ formatReleaseFileSize(selectedFile.size) }}
        </span>
        <Input.TextArea
          v-model:value="releaseNotes"
          :rows="3"
          placeholder="更新说明"
        />
        <Progress v-if="uploading" :percent="uploadProgress" />
        <Button
          type="primary"
          :loading="uploading"
          :disabled="!selectedFile"
          @click="upload"
        >
          提交上传
        </Button>
      </Space>
    </Card>
    <Card
      :title="`历史版本（${total} 个，${formatReleaseFileSize(totalFileSize)}）`"
    >
      <Table
        :columns="columns"
        :data-source="items"
        :loading="loading"
        row-key="id"
        :pagination="{ current: page, pageSize, total }"
        @change="
          (p: any) => {
            page = p.current;
            load();
          }
        "
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'app'">
            <div class="release-app-cell">
              <AppReleaseIcon
                :access-token="access.accessToken"
                :app-name="appDisplayName(asRelease(record))"
                :icon-url="record.iconUrl"
                :package-name="record.packageName"
                :size="40"
              />
              <div class="release-app-cell__text">
                <strong>{{ appDisplayName(asRelease(record)) }}</strong>
                <Tooltip :title="record.packageName">
                  <div
                    class="release-package release-package--compact"
                    :title="record.packageName"
                  >
                    {{ record.packageName }}
                  </div>
                </Tooltip>
              </div>
            </div>
          </template>
          <template v-else-if="column.key === 'version'">
            <b>{{ record.versionName }}</b>
            <div>#{{ record.versionCode }}</div>
          </template>
          <template v-else-if="column.key === 'file'">
            <div>{{ record.fileName }}</div>
            <small>{{ formatReleaseFileSize(record.fileSize) }}</small>
            <Tag v-if="!record.fileAvailable" color="error">
              文件缺失
            </Tag>
          </template>
          <template v-else-if="column.key === 'status'">
            <Tag
              :color="
                record.status === 'published'
                  ? 'success'
                  : record.status === 'archived'
                    ? 'default'
                    : 'processing'
              "
            >
              {{ releaseStatusLabel(record.status) }}
            </Tag>
          </template>
          <template v-else-if="column.key === 'action'">
            <Space v-if="canWrite">
              <Button
                v-if="canPublishRelease(asRelease(record))"
                type="primary"
                size="small"
                @click="publishRelease(asRelease(record))"
              >
                发布
              </Button>
              <Button
                v-if="canArchiveRelease(asRelease(record))"
                danger
                size="small"
                @click="archiveRelease(asRelease(record))"
              >
                下架
              </Button>
            </Space>
          </template>
        </template>
      </Table>
    </Card>
  </Page>
</template>

<style scoped>
.release-card {
  margin-bottom: 16px;
}

.current-release,
.release-app-cell {
  display: flex;
  gap: 12px;
  align-items: center;
  min-width: 0;
}

.current-release__content,
.release-app-cell__text {
  min-width: 0;
}

.current-release__content {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 8px;
}

.current-release__heading,
.current-release__details {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}

.release-app-cell__text {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 4px;
}

.release-package {
  max-width: 100%;
  overflow-wrap: anywhere;
  color: hsl(var(--muted-foreground));
}

.release-package--compact {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 640px) {
  .current-release {
    align-items: flex-start;
  }
}
</style>
