<template>
  <div class="p-6">
    <Card title="视频项目" :bordered="false">
      <template #extra>
        <Button type="primary" @click="showCreateModal">
          <template #icon>
            <PlusOutlined />
          </template>
          创建项目
        </Button>
      </template>

      <!-- 搜索 -->
      <div class="mb-4">
        <Input.Search
          v-model:value="searchKeyword"
          placeholder="搜索项目名称"
          style="width: 300px"
          @search="loadProjects"
        />
      </div>

      <!-- 项目列表 -->
      <Table
        :columns="columns"
        :data-source="projects"
        :loading="loading"
        :pagination="pagination"
        row-key="id"
        @change="handleTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'name'">
            <a @click="goToWorkbench(record.id)">{{ record.name }}</a>
          </template>

          <template v-else-if="column.key === 'status'">
            <Tag :color="getStatusColor(record.status)">
              {{ getStatusText(record.status) }}
            </Tag>
          </template>

          <template v-else-if="column.key === 'progress'">
            <Progress
              :percent="getProgress(record)"
              :status="record.totalShots === record.completedShots ? 'success' : 'active'"
            />
            <span class="ml-2 text-gray-500">
              {{ record.completedShots }}/{{ record.totalShots }}
            </span>
          </template>

          <template v-else-if="column.key === 'action'">
            <Space>
              <Button type="link" size="small" @click="goToWorkbench(record.id)">
                工作台
              </Button>
              <Button type="link" size="small" @click="editProject(record)">
                编辑
              </Button>
              <Popconfirm
                title="确定删除该项目吗？"
                @confirm="deleteProject(record.id)"
              >
                <Button type="link" size="small" danger>删除</Button>
              </Popconfirm>
            </Space>
          </template>
        </template>
      </Table>
    </Card>

    <!-- 创建/编辑项目模态框 -->
    <Modal
      v-model:open="modalVisible"
      :title="modalTitle"
      :confirm-loading="modalLoading"
      @ok="handleSubmit"
    >
      <Form :model="formData" :label-col="{ span: 6 }">
        <Form.Item label="项目名称" required>
          <Input v-model:value="formData.name" placeholder="请输入项目名称" />
        </Form.Item>

        <Form.Item label="主题预设">
          <Select
            placeholder="选择一个主题提示词，可继续编辑"
            allow-clear
            @change="applyThemePreset"
          >
            <Select.Option
              v-for="preset in themePresets"
              :key="preset.label"
              :value="preset.value"
            >
              {{ preset.label }}
            </Select.Option>
          </Select>
          <Space class="preset-tags" wrap>
            <Tag
              v-for="preset in themePresets"
              :key="preset.label"
              class="preset-tag"
              @click="applyThemePreset(preset.value)"
            >
              {{ preset.label }}
            </Tag>
          </Space>
        </Form.Item>

        <Form.Item label="主题提示词">
          <Input.TextArea
            v-model:value="formData.theme"
            :rows="2"
            placeholder="例如：一个普通人在低谷中重新找回内在力量，完成自我和解与成长"
          />
        </Form.Item>

        <Form.Item label="项目描述">
          <Input.TextArea
            v-model:value="formData.description"
            :rows="3"
            placeholder="请输入项目描述"
          />
        </Form.Item>

        <Form.Item label="风格预设">
          <Select
            placeholder="选择一个风格提示词，可继续编辑"
            allow-clear
            @change="applyStylePreset"
          >
            <Select.Option
              v-for="preset in stylePresets"
              :key="preset.label"
              :value="preset.value"
            >
              {{ preset.label }}
            </Select.Option>
          </Select>
          <Space class="preset-tags" wrap>
            <Tag
              v-for="preset in stylePresets"
              :key="preset.label"
              class="preset-tag"
              @click="applyStylePreset(preset.value)"
            >
              {{ preset.label }}
            </Tag>
          </Space>
        </Form.Item>

        <Form.Item label="风格提示词">
          <Input.TextArea
            v-model:value="formData.styleGuide"
            :rows="3"
            placeholder="例如：温暖治愈的写实电影风格，柔和自然光，浅景深，细腻人物表情。"
          />
        </Form.Item>
      </Form>
    </Modal>
  </div>
</template>

<script setup lang="ts">
import type { TableColumnsType } from 'ant-design-vue';

import { onMounted, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { Button, Card, Form, Input, Modal, Popconfirm, Progress, Select, Space, Table, Tag, message } from 'ant-design-vue';
import { PlusOutlined } from '@ant-design/icons-vue';
import {
  createVideoProjectApi,
  deleteVideoProjectApi,
  listVideoProjectsApi,
  updateVideoProjectApi,
  type VideoProject,
} from '#/api/core/videoproject';

const router = useRouter();

// 数据
const projects = ref<VideoProject[]>([]);
const loading = ref(false);
const searchKeyword = ref('');

const pagination = reactive({
  current: 1,
  pageSize: 10,
  total: 0,
  showSizeChanger: true,
  showTotal: (total: number) => `共 ${total} 条`,
});

// 模态框
const modalVisible = ref(false);
const modalLoading = ref(false);
const modalTitle = ref('创建项目');
const editingId = ref<string | number | ''>('');

const formData = reactive({
  name: '',
  theme: '',
  description: '',
  styleGuide: '',
});

const themePresets = [
  {
    label: '治愈成长',
    value: '一个普通人在低谷中重新找回内在力量，完成自我和解与成长',
  },
  {
    label: '亲子陪伴',
    value: '父母与孩子在一次温暖事件中互相理解，表达爱与支持',
  },
  {
    label: '职场突破',
    value: '职场新人面对挑战，从焦虑犹豫到主动突破并获得认可',
  },
  {
    label: '友情重逢',
    value: '多年未见的朋友因一次意外重逢，重新理解彼此的珍贵',
  },
  {
    label: '国风心灵',
    value: '以东方哲思和自然意象讲述内心觉醒、选择与命运转折',
  },
];

const stylePresets = [
  {
    label: '温暖治愈写实',
    value:
      '温暖治愈的写实电影风格，柔和自然光，浅景深，细腻人物表情，情绪克制但有感染力，画面干净高级。',
  },
  {
    label: '3D 卡通亲和',
    value:
      '高质量 3D 卡通动画风格，色彩明亮温暖，角色表情丰富，动作自然，适合亲子与轻松叙事，整体氛围积极治愈。',
  },
  {
    label: '国风水墨诗意',
    value:
      '东方国风水墨美学，留白构图，柔和雾气，山水与传统纹样元素，节奏舒缓，富有诗意和哲思。',
  },
  {
    label: '电影感悬疑',
    value:
      '电影级悬疑叙事风格，低饱和色调，强烈明暗对比，镜头运动克制，氛围紧张但不恐怖，强调细节和反转。',
  },
  {
    label: '赛博未来感',
    value:
      '赛博朋克未来视觉，霓虹光影，高对比城市夜景，科技界面元素，节奏感强，适合科技、AI、未来主题。',
  },
];

function applyThemePreset(value: unknown) {
  if (typeof value !== 'string') return;
  formData.theme = value;
  if (!formData.description.trim()) {
    formData.description = value;
  }
}

function applyStylePreset(value: unknown) {
  if (typeof value !== 'string') return;
  formData.styleGuide = value;
}

// 表格列
const columns: TableColumnsType<VideoProject> = [
  { title: '项目名称', dataIndex: 'name', key: 'name', width: 200 },
  { title: '主题', dataIndex: 'theme', key: 'theme', width: 120 },
  { title: '描述', dataIndex: 'description', key: 'description', ellipsis: true },
  { title: '状态', dataIndex: 'status', key: 'status', width: 100 },
  { title: '分镜进度', key: 'progress', width: 200 },
  { title: '角色数', dataIndex: 'characterCount', key: 'characterCount', width: 100 },
  { title: '场景数', dataIndex: 'sceneCount', key: 'sceneCount', width: 100 },
  { title: '创建时间', dataIndex: 'createTime', key: 'createTime', width: 180 },
  { title: '操作', key: 'action', width: 200, fixed: 'right' as const },
];

// 加载项目列表
async function loadProjects() {
  loading.value = true;
  try {
    const res = await listVideoProjectsApi({
      keyword: searchKeyword.value,
      page: pagination.current,
      pageSize: pagination.pageSize,
    });
    projects.value = res.items || [];
    pagination.total = res.total || 0;
  } catch (error) {
    message.error('加载项目列表失败');
  } finally {
    loading.value = false;
  }
}

// 表格分页变化
function handleTableChange(pag: any) {
  pagination.current = pag.current;
  pagination.pageSize = pag.pageSize;
  loadProjects();
}

// 显示创建模态框
function showCreateModal() {
  modalTitle.value = '创建项目';
  editingId.value = '';
  Object.assign(formData, {
    name: '',
    theme: '',
    description: '',
    styleGuide: '',
  });
  modalVisible.value = true;
}

// 编辑项目
function editProject(record: Record<string, any>) {
  modalTitle.value = '编辑项目';
  editingId.value = record.id;
  Object.assign(formData, {
    name: record.name,
    theme: record.theme || '',
    description: record.description || '',
    styleGuide: record.styleGuide || '',
  });
  modalVisible.value = true;
}

// 提交表单
async function handleSubmit() {
  if (!formData.name) {
    message.warning('请输入项目名称');
    return;
  }

  modalLoading.value = true;
  try {
    if (editingId.value) {
      await updateVideoProjectApi(editingId.value, formData);
      message.success('项目更新成功');
    } else {
      await createVideoProjectApi(formData);
      message.success('项目创建成功');
    }
    modalVisible.value = false;
    loadProjects();
  } catch (error) {
    message.error('操作失败');
  } finally {
    modalLoading.value = false;
  }
}

// 删除项目
async function deleteProject(id: string | number) {
  try {
    await deleteVideoProjectApi(id);
    message.success('删除成功');
    loadProjects();
  } catch (error) {
    message.error('删除失败');
  }
}

// 跳转到工作台
function goToWorkbench(id: string | number) {
  router.push(`/video/projects/${id}/workbench`);
}

// 状态相关
function getStatusText(status: string | undefined) {
  status = status || 'active';
  const map: Record<string, string> = {
    draft: '草稿',
    in_progress: '进行中',
    completed: '已完成',
    failed: '失败',
  };
  return map[status] || status;
}

function getStatusColor(status: string | undefined) {
  status = status || 'active';
  const map: Record<string, string> = {
    draft: 'default',
    in_progress: 'processing',
    completed: 'success',
    failed: 'error',
  };
  return map[status] || 'default';
}

function getProgress(record: Record<string, any>) {
  const total = record.totalShots || 0;
  if (total === 0) return 0;
  return Math.round(((record.completedShots || 0) / total) * 100);
}

onMounted(() => {
  loadProjects();
});
</script>

<style scoped>
.ml-2 {
  margin-left: 8px;
}

.preset-tags {
  margin-top: 8px;
}

.preset-tag {
  cursor: pointer;
  user-select: none;
}
</style>
