<script setup lang="ts">
import type { ClassroomContent, ClassroomSeries } from '#/api/core/classroom';
import { computed, onMounted, ref, watch } from 'vue';
import { Page } from '@vben/common-ui';
import {
  Alert,
  Button,
  Card,
  Empty,
  Modal,
  Space,
  Table,
  Tabs,
  Tag,
  message,
} from 'ant-design-vue';
import { useAccessStore } from '@vben/stores';
import {
  getClassroomContentsApi,
  getClassroomSeriesApi,
  offlineClassroomContentApi,
  publishClassroomContentApi,
  setClassroomContentPlaybackBlockedApi,
} from '#/api/core/classroom';
import ContentEditor from './components/content-editor.vue';
import SeriesView from './series.vue';
import UploadTasks from './upload-tasks.vue';
import {
  classroomOperationError,
  classroomPermissions,
  visibleClassroomTabs,
} from './classroom-view-model';

const accessStore = useAccessStore();
const permissions = computed(() =>
  classroomPermissions(accessStore.accessCodes),
);
const canUpload = computed(() => permissions.value.canUpload);
const canPublish = computed(() => permissions.value.canPublish);
const canPrice = computed(() => permissions.value.canPrice);
const canWrite = computed(() => permissions.value.canWrite);
const activeTab = ref('contents');
const loading = ref(false);
const error = ref('');
const contents = ref<ClassroomContent[]>([]);
const series = ref<ClassroomSeries[]>([]);
const editorOpen = ref(false);
const editing = ref<ClassroomContent>();
const actionLoadingId = ref<number>();
const columns = [
  { dataIndex: 'title', title: '课件' },
  { dataIndex: 'contentType', title: '类型' },
  { dataIndex: 'effectiveAccessLevel', title: '权限' },
  { dataIndex: 'status', title: '状态' },
  { key: 'action', title: '操作' },
];
const allTabs = [
  { key: 'contents', label: '课件管理' },
  { key: 'series', label: '课程系列' },
  { key: 'uploads', label: '上传任务' },
];
const tabs = computed(() => {
  const visible = visibleClassroomTabs(canUpload.value);
  return allTabs.filter((item) => visible.includes(item.key));
});
watch(canUpload, (allowed) => {
  if (!allowed && activeTab.value === 'uploads') activeTab.value = 'contents';
});
const statusText: Record<string, string> = {
  draft: '草稿',
  published: '已发布',
  offline: '已下线',
  failed: '失败',
  ready: '待发布',
  processing: '处理中',
};
async function load() {
  loading.value = true;
  error.value = '';
  try {
    const [c, s] = await Promise.all([
      getClassroomContentsApi({ page: 1, pageSize: 50 }),
      getClassroomSeriesApi({ page: 1, pageSize: 50 }),
    ]);
    contents.value = c.items;
    series.value = s.items;
  } catch {
    error.value = '课件加载失败，请重试。';
  } finally {
    loading.value = false;
  }
}
function confirmLifecycle(
  record: ClassroomContent,
  action: 'publish' | 'offline' | 'block' | 'unblock',
) {
  Modal.confirm({
    title:
      action === 'publish'
        ? '发布课件？'
        : action === 'offline'
          ? '下线课件？'
          : action === 'block'
            ? '阻断播放？'
            : '恢复播放？',
    content: '请确认该操作及其对用户的影响。',
    async onOk() {
      actionLoadingId.value = record.id;
      try {
        if (action === 'publish')
          await publishClassroomContentApi(record.id, {
            expectedUpdatedAt: record.updatedAt,
          });
        else if (action === 'offline')
          await offlineClassroomContentApi(record.id, {
            expectedUpdatedAt: record.updatedAt,
            reason: '后台操作',
          });
        else
          await setClassroomContentPlaybackBlockedApi(
            record.id,
            action === 'block',
            record.updatedAt,
            '后台操作',
          );
        message.success('操作成功');
        await load();
      } catch (cause) {
        message.error(classroomOperationError(cause, '操作失败'));
        throw cause;
      } finally {
        actionLoadingId.value = undefined;
      }
    },
  });
}
function openCreate() {
  editing.value = undefined;
  editorOpen.value = true;
}
function openUploads() {
  activeTab.value = 'uploads';
}
onMounted(load);
</script>

<template>
  <Page title="老师课堂" content-class="classroom-page">
    <Card class="classroom-shell">
      <Tabs v-model:activeKey="activeTab" :items="tabs" />
      <Alert v-if="error" type="error" :message="error" show-icon
        ><template #action><Button @click="load">重试</Button></template></Alert
      >
      <template v-if="activeTab === 'contents'">
        <div class="toolbar">
          <span class="section-help"
            >管理视频课件、音频课件，可按系列内容或独立内容展示；发布前请确认媒体已就绪。</span
          ><Space
            ><Button v-if="canWrite" type="primary" @click="openCreate"
              >新建课件</Button
            ><Button v-if="canUpload" @click="openUploads">上传媒体</Button
            ><Button @click="load">刷新</Button></Space
          >
        </div>
        <Empty
          v-if="!loading && !contents.length"
          description="暂无课件，先创建一条视频或音频课件"
        />
        <Table
          v-else
          :loading="loading"
          :columns="columns"
          :data-source="contents"
          row-key="id"
          :scroll="{ x: 900 }"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.dataIndex === 'contentType'">{{
              record.contentType === 'video' ? '视频课件' : '音频课件'
            }}</template>
            <template v-else-if="column.dataIndex === 'effectiveAccessLevel'">{{
              record.effectiveAccessLevel === 'paid'
                ? '付费'
                : record.effectiveAccessLevel === 'member'
                  ? '会员'
                  : record.effectiveAccessLevel === 'login'
                    ? '登录'
                    : '公开'
            }}</template>
            <Tag
              v-else-if="column.dataIndex === 'status'"
              :color="record.status === 'published' ? 'success' : 'default'"
              >{{ statusText[record.status] || record.status
              }}<span v-if="record.playbackBlocked"> · 播放已阻断</span></Tag
            >
            <Space v-else-if="column.key === 'action'">
              <Button
                v-if="canPublish && record.status !== 'published'"
                :loading="actionLoadingId === record.id"
                @click="confirmLifecycle(record as ClassroomContent, 'publish')"
                >发布</Button
              >
              <Button
                v-if="canPublish && record.status === 'published'"
                :loading="actionLoadingId === record.id"
                @click="confirmLifecycle(record as ClassroomContent, 'offline')"
                >下线</Button
              >
              <Button
                v-if="canPrice"
                @click="
                  editing = record as ClassroomContent;
                  editorOpen = true;
                "
                >定价</Button
              >
              <Button
                v-if="canWrite"
                @click="
                  editing = record as ClassroomContent;
                  editorOpen = true;
                "
                >编辑</Button
              >
              <Button
                v-if="canPublish"
                danger
                :loading="actionLoadingId === record.id"
                @click="
                  confirmLifecycle(
                    record as ClassroomContent,
                    record.playbackBlocked ? 'unblock' : 'block',
                  )
                "
                >{{ record.playbackBlocked ? '恢复播放' : '阻断播放' }}</Button
              >
            </Space>
          </template>
        </Table>
      </template>
      <SeriesView
        v-else-if="activeTab === 'series'"
        :can-price="canPrice"
        :can-publish="canPublish"
        :can-write="canWrite"
      />
      <UploadTasks
        v-else-if="activeTab === 'uploads' && canUpload"
        :can-upload="canUpload"
        :contents="contents"
      />
    </Card>
    <Modal
      v-model:open="editorOpen"
      title="编辑课件"
      :width="760"
      :footer="null"
      ><ContentEditor
        :content="editing"
        :series="series"
        :can-price="canPrice"
        :can-write="canWrite"
        @cancel="editorOpen = false"
        @saved="
          editorOpen = false;
          load();
        "
    /></Modal>
  </Page>
</template>

<style scoped>
.classroom-page {
  min-height: 100%;
}
.classroom-shell {
  min-height: 560px;
}
.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin: 8px 0 16px;
}
.section-help {
  color: hsl(var(--muted-foreground));
  line-height: 1.6;
}
:deep(button:focus-visible),
:deep(.ant-tabs-tab:focus-visible) {
  outline: 2px solid hsl(var(--primary));
  outline-offset: 2px;
}
@media (max-width: 768px) {
  .toolbar {
    align-items: flex-start;
    flex-direction: column;
  }
  .classroom-shell {
    border-radius: 0;
  }
}
@media (prefers-reduced-motion: reduce) {
  * {
    transition-duration: 0.01ms !important;
    animation-duration: 0.01ms !important;
  }
}
</style>
