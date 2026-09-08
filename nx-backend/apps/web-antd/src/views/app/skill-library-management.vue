<script setup lang="ts">
import type {
  SkillLibraryAdminCategory,
  SkillLibraryAdminLibrary,
  SkillLibraryAdminSkill,
  SkillLibraryAdminStatus,
} from '#/api';

import { computed, onMounted, reactive, ref } from 'vue';

import { Page } from '@vben/common-ui';
import { IconifyIcon } from '@vben/icons';
import { useAccessStore } from '@vben/stores';
import {
  Alert,
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  message,
} from 'ant-design-vue';

import {
  getSkillLibraryManagementApi,
  updateSkillLibraryApi,
  updateSkillLibraryCategoryApi,
  updateSkillLibrarySkillApi,
} from '#/api';

type EditorKind = 'category' | 'library' | 'skill';

const access = useAccessStore();
const canEdit = computed(() => access.accessCodes.includes('App:SkillLibrary:Edit'));
const loading = ref(false);
const saving = ref(false);
const error = ref('');
const activeTab = ref('skills');
const query = ref('');
const categoryFilter = ref<number>();
const libraries = ref<SkillLibraryAdminLibrary[]>([]);
const categories = ref<SkillLibraryAdminCategory[]>([]);
const skills = ref<SkillLibraryAdminSkill[]>([]);
const editorOpen = ref(false);
const editorKind = ref<EditorKind>('skill');
const activeId = ref(0);

const form = reactive({
  categoryId: 0,
  colorToken: 'green',
  description: '',
  iconKey: 'menu_book',
  key: '',
  name: '',
  sortOrder: 0,
  status: 'enabled' as SkillLibraryAdminStatus,
  summary: '',
});

const statusOptions = [
  { label: '启用', value: 'enabled' },
  { label: '停用', value: 'disabled' },
];
const colorOptions = [
  { label: '绿色', value: 'green' },
  { label: '蓝色', value: 'blue' },
  { label: '紫色', value: 'purple' },
  { label: '粉色', value: 'pink' },
  { label: '暖金', value: 'sand' },
  { label: '提醒色', value: 'warning' },
  { label: '信息色', value: 'info' },
];
const categoryOptions = computed(() => categories.value.map((item) => ({
  label: item.name,
  value: item.id,
})));
const filteredSkills = computed(() => {
  const keyword = query.value.trim().toLowerCase();
  return skills.value.filter((item) => {
    if (categoryFilter.value && item.categoryId !== categoryFilter.value) return false;
    if (!keyword) return true;
    return [item.name, item.key, item.summary, item.categoryName]
      .join('\n')
      .toLowerCase()
      .includes(keyword);
  });
});
const enabledSkillCount = computed(() => skills.value.filter((item) => item.status === 'enabled').length);
const editorTitle = computed(() => ({
  category: '编辑分类',
  library: '编辑技能库',
  skill: '编辑技能',
})[editorKind.value]);

const skillColumns = [
  { dataIndex: 'name', title: '技能名称', width: 200 },
  { dataIndex: 'categoryName', title: '所属分类', width: 130 },
  { dataIndex: 'summary', title: '技能简介' },
  { dataIndex: 'publishedVersion', title: '版本', width: 90 },
  { dataIndex: 'status', title: '启用状态', width: 90 },
  { key: 'action', title: '操作', width: 92 },
];
const categoryColumns = [
  { dataIndex: 'name', title: '分类名称', width: 180 },
  { dataIndex: 'key', title: '分类标识' },
  { dataIndex: 'skillCount', title: '技能数', width: 90 },
  { dataIndex: 'sortOrder', title: '排序', width: 90 },
  { dataIndex: 'status', title: '启用状态', width: 90 },
  { key: 'action', title: '操作', width: 92 },
];

async function load() {
  loading.value = true;
  error.value = '';
  try {
    const catalog = await getSkillLibraryManagementApi();
    libraries.value = catalog?.libraries ?? [];
    categories.value = catalog?.categories ?? [];
    skills.value = catalog?.skills ?? [];
  } catch {
    error.value = '成长技能库加载失败，请稍后重试。';
  } finally {
    loading.value = false;
  }
}

function fillBase(item: SkillLibraryAdminLibrary | SkillLibraryAdminCategory) {
  Object.assign(form, {
    categoryId: 0,
    colorToken: 'colorToken' in item ? item.colorToken : 'green',
    description: 'description' in item ? item.description : '',
    iconKey: item.iconKey,
    key: item.key,
    name: item.name,
    sortOrder: item.sortOrder,
    status: item.status,
    summary: '',
  });
}

function editLibrary(item: SkillLibraryAdminLibrary) {
  editorKind.value = 'library';
  activeId.value = item.id;
  fillBase(item);
  editorOpen.value = true;
}

function editCategory(item: SkillLibraryAdminCategory) {
  editorKind.value = 'category';
  activeId.value = item.id;
  fillBase(item);
  editorOpen.value = true;
}

function editSkill(item: SkillLibraryAdminSkill) {
  editorKind.value = 'skill';
  activeId.value = item.id;
  Object.assign(form, {
    categoryId: item.categoryId,
    colorToken: item.colorToken,
    description: item.description,
    iconKey: item.iconKey,
    key: item.key,
    name: item.name,
    sortOrder: item.sortOrder,
    status: item.status,
    summary: item.summary,
  });
  editorOpen.value = true;
}

function editSkillRecord(record: Record<string, any>) {
  editSkill(record as unknown as SkillLibraryAdminSkill);
}

function editCategoryRecord(record: Record<string, any>) {
  editCategory(record as unknown as SkillLibraryAdminCategory);
}

async function save() {
  if (!form.name.trim()) {
    message.warning('请输入名称');
    return;
  }
  if (editorKind.value === 'skill' && (!form.summary.trim() || !form.categoryId)) {
    message.warning('请填写技能简介并选择所属分类');
    return;
  }
  saving.value = true;
  const metadata = {
    colorToken: form.colorToken,
    description: form.description.trim(),
    iconKey: form.iconKey.trim(),
    name: form.name.trim(),
    sortOrder: form.sortOrder,
    status: form.status,
  };
  try {
    if (editorKind.value === 'library') {
      await updateSkillLibraryApi(activeId.value, metadata);
    } else if (editorKind.value === 'category') {
      await updateSkillLibraryCategoryApi(activeId.value, metadata);
    } else {
      await updateSkillLibrarySkillApi(activeId.value, {
        ...metadata,
        categoryId: form.categoryId,
        summary: form.summary.trim(),
      });
    }
    editorOpen.value = false;
    message.success('保存成功，App 刷新技能库后生效');
    await load();
  } catch {
    message.error('保存失败，请检查填写内容');
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <Page title="成长技能库管理">
    <Alert
      v-if="error"
      class="mb-4"
      :message="error"
      show-icon
      type="error"
    >
      <template #action><Button size="small" @click="load">重试</Button></template>
    </Alert>

    <div class="mb-4 grid gap-3 md:grid-cols-3">
      <div class="rounded-md border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-900">
        <div class="text-sm text-gray-500">当前技能库</div>
        <div class="mt-1 text-lg font-semibold">{{ libraries[0]?.name || '学习成长类书籍' }}</div>
      </div>
      <div class="rounded-md border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-900">
        <div class="text-sm text-gray-500">App 可用技能</div>
        <div class="mt-1 text-lg font-semibold text-emerald-600">{{ enabledSkillCount }}</div>
      </div>
      <div class="rounded-md border border-gray-200 bg-white p-4 dark:border-gray-700 dark:bg-gray-900">
        <div class="text-sm text-gray-500">技能分类</div>
        <div class="mt-1 text-lg font-semibold text-blue-600">{{ categories.length }}</div>
      </div>
    </div>

    <Card :bordered="false">
      <Tabs v-model:active-key="activeTab">
        <Tabs.TabPane key="skills" tab="技能管理">
          <div class="mb-4 flex flex-wrap items-center gap-3">
            <Input
              v-model:value="query"
              allow-clear
              class="max-w-sm"
              placeholder="搜索技能名称、标识或简介"
            >
              <template #prefix><IconifyIcon icon="lucide:search" /></template>
            </Input>
            <Select
              v-model:value="categoryFilter"
              allow-clear
              class="w-44"
              :options="categoryOptions"
              placeholder="全部分类"
            />
            <Button @click="load"><IconifyIcon icon="lucide:refresh-cw" />刷新</Button>
          </div>
          <Table
            :columns="skillColumns"
            :data-source="filteredSkills"
            :loading="loading"
            :pagination="{ pageSize: 15, showSizeChanger: true }"
            row-key="id"
            :scroll="{ x: 980 }"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'name'">
                <div class="font-medium">{{ record.name }}</div>
                <div class="text-xs text-gray-400">{{ record.key }}</div>
              </template>
              <template v-else-if="column.dataIndex === 'publishedVersion'">
                <Tag v-if="record.hasPublishedVersion" color="blue">v{{ record.publishedVersion }}</Tag>
                <Tag v-else>未发布</Tag>
              </template>
              <template v-else-if="column.dataIndex === 'status'">
                <Tag :color="record.status === 'enabled' ? 'green' : 'default'">
                  {{ record.status === 'enabled' ? '已启用' : '已停用' }}
                </Tag>
              </template>
              <template v-else-if="column.key === 'action'">
                <Button v-if="canEdit" type="link" @click="editSkillRecord(record)">编辑技能</Button>
              </template>
            </template>
          </Table>
        </Tabs.TabPane>

        <Tabs.TabPane key="categories" tab="分类管理">
          <Table
            :columns="categoryColumns"
            :data-source="categories"
            :loading="loading"
            :pagination="false"
            row-key="id"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'status'">
                <Tag :color="record.status === 'enabled' ? 'green' : 'default'">
                  {{ record.status === 'enabled' ? '已启用' : '已停用' }}
                </Tag>
              </template>
              <template v-else-if="column.key === 'action'">
                <Button v-if="canEdit" type="link" @click="editCategoryRecord(record)">编辑</Button>
              </template>
            </template>
          </Table>
        </Tabs.TabPane>

        <Tabs.TabPane key="library" tab="技能库设置">
          <div v-if="libraries[0]" class="max-w-3xl py-2">
            <div class="mb-2 text-base font-semibold">{{ libraries[0].name }}</div>
            <div class="mb-4 text-gray-500">{{ libraries[0].description }}</div>
            <Space>
              <Tag :color="libraries[0].status === 'enabled' ? 'green' : 'default'">
                {{ libraries[0].status === 'enabled' ? '已启用' : '已停用' }}
              </Tag>
              <Button v-if="canEdit" type="primary" @click="editLibrary(libraries[0])">编辑技能库</Button>
            </Space>
          </div>
        </Tabs.TabPane>
      </Tabs>
    </Card>

    <Modal
      v-model:open="editorOpen"
      :confirm-loading="saving"
      :title="editorTitle"
      width="640px"
      @ok="save"
    >
      <Form layout="vertical">
        <Form.Item :label="editorKind === 'skill' ? '技能标识' : '数据标识'">
          <Input v-model:value="form.key" disabled />
        </Form.Item>
        <Form.Item :label="editorKind === 'skill' ? '技能名称' : '名称'" required>
          <Input v-model:value="form.name" :maxlength="100" />
        </Form.Item>
        <Form.Item v-if="editorKind === 'skill'" label="所属分类" required>
          <Select v-model:value="form.categoryId" :options="categoryOptions" />
        </Form.Item>
        <Form.Item v-if="editorKind === 'skill'" label="技能简介" required>
          <Input.TextArea v-model:value="form.summary" :maxlength="500" :rows="3" show-count />
        </Form.Item>
        <Form.Item v-if="editorKind !== 'category'" label="详细说明">
          <Input.TextArea v-model:value="form.description" :maxlength="2000" :rows="4" show-count />
        </Form.Item>
        <div class="grid grid-cols-1 gap-x-4 md:grid-cols-2">
          <Form.Item label="图标">
            <Input v-model:value="form.iconKey" placeholder="例如 menu_book" />
          </Form.Item>
          <Form.Item v-if="editorKind !== 'library'" label="颜色">
            <Select v-model:value="form.colorToken" :options="colorOptions" />
          </Form.Item>
          <Form.Item label="排序">
            <InputNumber v-model:value="form.sortOrder" class="w-full" :min="0" :precision="0" />
          </Form.Item>
          <Form.Item label="启用状态">
            <Select v-model:value="form.status" :options="statusOptions" />
          </Form.Item>
        </div>
        <Alert
          v-if="editorKind === 'skill'"
          message="技能标识用于关联历史会话，不支持修改；停用后不会继续出现在 App 技能列表中。"
          show-icon
          type="info"
        />
      </Form>
    </Modal>
  </Page>
</template>
