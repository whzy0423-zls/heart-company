<script setup lang="ts">
import type { ClassroomSeries } from '#/api/core/classroom';
import { onMounted, reactive, ref } from 'vue';
import {
  Alert,
  Button,
  Card,
  Empty,
  Form,
  Input,
  InputNumber,
  message,
  Modal,
  Space,
  Select,
  Table,
  Tag,
} from 'ant-design-vue';
import {
  deleteClassroomSeriesApi,
  createClassroomSeriesApi,
  getClassroomSeriesApi,
  offlineClassroomSeriesApi,
  publishClassroomSeriesApi,
  setClassroomSeriesPlaybackBlockedApi,
  setClassroomSeriesPriceApi,
  updateClassroomSeriesApi,
} from '#/api/core/classroom';
import { seriesMetadataPayload } from './series-model';

const props = defineProps<{
  canPrice?: boolean;
  canPublish: boolean;
  canWrite: boolean;
}>();
const loading = ref(false);
const error = ref('');
const rows = ref<ClassroomSeries[]>([]);
const editorOpen = ref(false);
const editing = ref<ClassroomSeries>();
const saving = ref(false);
const actionLoadingId = ref<number>();
const form = reactive({
  title: '',
  summary: '',
  teacherName: '',
  teacherKey: '',
  accessLevel: 'public' as 'public' | 'login' | 'member' | 'paid',
  priceCents: 0,
});
const columns = [
  { dataIndex: 'title', title: '系列名称' },
  { dataIndex: 'teacherName', title: '老师' },
  { dataIndex: 'status', title: '状态' },
  { key: 'action', title: '操作' },
];
async function load() {
  loading.value = true;
  error.value = '';
  try {
    rows.value = (await getClassroomSeriesApi({ page: 1, pageSize: 50 })).items;
  } catch {
    error.value = '课程系列加载失败，请重试。';
  } finally {
    loading.value = false;
  }
}
function openEditor(record?: ClassroomSeries) {
  editing.value = record;
  Object.assign(
    form,
    record
      ? {
          title: record.title,
          summary: record.summary,
          teacherName: record.teacherName,
          teacherKey: record.teacherKey,
          accessLevel: record.accessLevel,
          priceCents: record.priceCents,
        }
      : {
          title: '',
          summary: '',
          teacherName: '',
          teacherKey: '',
          accessLevel: 'public',
          priceCents: 0,
        },
  );
  editorOpen.value = true;
}
async function save() {
  if (!form.title.trim()) return message.warning('请填写系列名称');
  if (!props.canWrite && !editing.value) return;
  saving.value = true;
  try {
    let saved = editing.value as ClassroomSeries;
    if (props.canWrite)
      saved = editing.value
        ? await updateClassroomSeriesApi(editing.value.id, {
            ...seriesMetadataPayload(form),
            expectedUpdatedAt: editing.value.updatedAt,
          })
        : await createClassroomSeriesApi(seriesMetadataPayload(form));
    if (
      props.canPrice &&
      (saved.accessLevel !== form.accessLevel ||
        saved.priceCents !== form.priceCents)
    )
      saved = await setClassroomSeriesPriceApi(saved.id, {
        accessLevel: form.accessLevel,
        priceCents: form.accessLevel === 'paid' ? form.priceCents : 0,
        expectedUpdatedAt: saved.updatedAt,
      });
    message.success('系列已保存');
    editorOpen.value = false;
    await load();
  } catch (cause) {
    message.error(cause instanceof Error ? cause.message : '系列保存失败');
  } finally {
    saving.value = false;
  }
}
function confirmAction(
  record: ClassroomSeries,
  action: 'delete' | 'offline' | 'publish' | 'block' | 'unblock',
) {
  Modal.confirm({
    title:
      action === 'delete'
        ? '删除课程系列？'
        : action === 'offline'
          ? '下线课程系列？'
          : action === 'publish'
            ? '发布课程系列？'
            : action === 'block'
              ? '阻断系列播放？'
              : '恢复系列播放？',
    content: '此操作会影响小程序中的可见性，请确认。',
    async onOk() {
      actionLoadingId.value = record.id;
      try {
        if (action === 'delete')
          await deleteClassroomSeriesApi(
            record.id,
            record.updatedAt,
            '后台操作',
          );
        else if (action === 'offline')
          await offlineClassroomSeriesApi(record.id, {
            expectedUpdatedAt: record.updatedAt,
            reason: '后台操作',
          });
        else if (action === 'publish')
          await publishClassroomSeriesApi(record.id, {
            expectedUpdatedAt: record.updatedAt,
          });
        else
          await setClassroomSeriesPlaybackBlockedApi(
            record.id,
            action === 'block',
            record.updatedAt,
            '后台操作',
          );
        message.success('操作成功');
        await load();
      } catch (cause) {
        message.error(cause instanceof Error ? cause.message : '操作失败');
        throw cause;
      } finally {
        actionLoadingId.value = undefined;
      }
    },
  });
}
onMounted(load);
</script>
<template>
  <Card title="课程系列" :loading="loading">
    <template #extra
      ><Button v-if="canWrite" type="primary" @click="openEditor()"
        >新建系列</Button
      ></template
    >
    <Alert v-if="error" type="error" :message="error" show-icon
      ><template #action><Button @click="load">重试</Button></template></Alert
    >
    <Empty v-else-if="!loading && !rows.length" description="暂无课程系列" />
    <Table
      v-else
      :columns="columns"
      :data-source="rows"
      row-key="id"
      :scroll="{ x: 720 }"
    >
      <template #bodyCell="{ column, record }">
        <Tag v-if="column.dataIndex === 'status'">{{ record.status }}</Tag>
        <Space v-else-if="column.key === 'action'">
          <Button v-if="canWrite" @click="openEditor(record as ClassroomSeries)"
            >编辑</Button
          ><Button
            v-if="canPrice"
            @click="openEditor(record as ClassroomSeries)"
            >定价</Button
          ><Button
            v-if="canPublish && record.status !== 'published'"
            :loading="actionLoadingId === record.id"
            @click="confirmAction(record as ClassroomSeries, 'publish')"
            >发布</Button
          >
          <Button
            v-if="canPublish && record.status === 'published'"
            :loading="actionLoadingId === record.id"
            @click="confirmAction(record as ClassroomSeries, 'offline')"
            >下线</Button
          >
          <Button
            v-if="canWrite"
            danger
            :loading="actionLoadingId === record.id"
            @click="confirmAction(record as ClassroomSeries, 'delete')"
            >删除</Button
          >
          <Button
            v-if="canPublish"
            :loading="actionLoadingId === record.id"
            @click="
              confirmAction(
                record as ClassroomSeries,
                record.playbackBlocked ? 'unblock' : 'block',
              )
            "
            >{{ record.playbackBlocked ? '恢复播放' : '阻断播放' }}</Button
          >
        </Space>
      </template>
    </Table>
  </Card>
  <Modal
    v-model:open="editorOpen"
    title="课程系列"
    ok-text="保存系列"
    :confirm-loading="saving"
    :ok-button-props="{ disabled: !canWrite && !canPrice }"
    @ok="save"
  >
    <Form layout="vertical"
      ><Form.Item label="系列名称" required
        ><Input v-model:value="form.title" /></Form.Item
      ><Form.Item label="老师"
        ><Input v-model:value="form.teacherName" /></Form.Item
      ><Form.Item label="权限"
        ><Select
          v-model:value="form.accessLevel"
          :disabled="!canPrice"
          :options="[
            { label: '公开', value: 'public' },
            { label: '登录后', value: 'login' },
            { label: '会员', value: 'member' },
            { label: '付费', value: 'paid' },
          ]" /></Form.Item
      ><Form.Item v-if="form.accessLevel === 'paid'" label="价格（分）"
        ><InputNumber
          v-model:value="form.priceCents"
          :disabled="!canPrice"
          :min="1" /></Form.Item
      ><Form.Item label="简介"
        ><Input.TextArea v-model:value="form.summary" /></Form.Item
    ></Form>
  </Modal>
</template>
<style scoped>
:deep(button:focus-visible) {
  outline: 2px solid hsl(var(--primary));
  outline-offset: 2px;
}
@media (max-width: 768px) {
  :deep(.ant-card-body) {
    padding: 12px;
  }
}
</style>
