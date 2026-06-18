<script setup lang="ts">
import type { SystemMenu } from '#/api';

import { computed, onMounted, ref } from 'vue';

import {
  Button,
  Form,
  Input,
  InputNumber,
  message,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
} from 'ant-design-vue';

import {
  deleteSystemMenuApi,
  getSystemMenuListApi,
  saveSystemMenuApi,
} from '#/api';

import PageShell from '../components/page-shell.vue';

interface MenuForm {
  authCode?: string;
  component?: string;
  icon?: string;
  id?: number;
  name: string;
  path: string;
  pid: number;
  sort: number;
  status: number;
  title: string;
  type: string;
}

const loading = ref(false);
const saving = ref(false);
const modalOpen = ref(false);
const menus = ref<SystemMenu[]>([]);
const form = ref<MenuForm>(emptyForm());

const typeOptions = [
  { label: '目录', value: 'catalog' },
  { label: '菜单', value: 'menu' },
  { label: '按钮', value: 'button' },
];

const columns = [
  { dataIndex: ['meta', 'title'], title: '菜单名称' },
  { dataIndex: 'name', title: '菜单标识' },
  { dataIndex: 'path', title: '路由路径' },
  { dataIndex: 'component', title: '组件' },
  { dataIndex: 'type', title: '类型' },
  { dataIndex: 'authCode', title: '权限码' },
  { dataIndex: 'status', title: '状态' },
  { key: 'action', title: '操作', width: 160 },
];

// 父级可选项：目录/菜单都可作父级，扁平化展开。
const parentOptions = computed(() => {
  const options: { label: string; value: number }[] = [
    { label: '顶级菜单', value: 0 },
  ];
  const walk = (items: SystemMenu[], prefix: string) => {
    for (const item of items) {
      options.push({
        label: prefix + (item.meta?.title ?? item.name),
        value: item.id,
      });
      if (item.children?.length) walk(item.children, `${prefix}— `);
    }
  };
  walk(menus.value, '');
  return options;
});

function emptyForm(): MenuForm {
  return {
    authCode: '',
    component: '',
    icon: '',
    name: '',
    path: '',
    pid: 0,
    sort: 0,
    status: 1,
    title: '',
    type: 'menu',
  };
}

async function load() {
  loading.value = true;
  try {
    menus.value = await getSystemMenuListApi();
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  form.value = emptyForm();
  modalOpen.value = true;
}

function openEdit(record: SystemMenu) {
  form.value = {
    authCode: record.authCode ?? '',
    component: record.component ?? '',
    icon: record.meta?.icon ?? '',
    id: record.id,
    name: record.name,
    path: record.path,
    pid: record.pid ?? 0,
    sort: record.sort ?? 0,
    status: record.status,
    title: record.meta?.title ?? '',
    type: record.type,
  };
  modalOpen.value = true;
}

async function save() {
  if (!form.value.name || !form.value.title) {
    message.warning('请填写菜单标识和菜单名称');
    return;
  }
  saving.value = true;
  try {
    const payload: SystemMenu = {
      authCode: form.value.authCode,
      component: form.value.component,
      id: form.value.id ?? 0,
      meta: { icon: form.value.icon, title: form.value.title },
      name: form.value.name,
      path: form.value.path,
      pid: form.value.pid,
      sort: form.value.sort,
      status: form.value.status,
      type: form.value.type,
    };
    await saveSystemMenuApi(payload);
    message.success('已保存菜单');
    modalOpen.value = false;
    await load();
  } finally {
    saving.value = false;
  }
}

async function remove(record: SystemMenu) {
  await deleteSystemMenuApi(record.id);
  message.success('已删除菜单');
  await load();
}

onMounted(load);
</script>

<template>
  <PageShell
    description="维护后台侧边栏菜单、目录与按钮权限码。修改后对应角色刷新即生效。"
    :loading="loading"
    title="菜单权限"
    @create="openCreate"
    @refresh="load"
  >
    <Table
      :columns="columns"
      :data-source="menus"
      :pagination="false"
      row-key="id"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'type'">
          <Tag>{{ record.type }}</Tag>
        </template>
        <template v-if="column.dataIndex === 'status'">
          <Tag :color="record.status === 1 ? 'green' : 'red'">
            {{ record.status === 1 ? '启用' : '停用' }}
          </Tag>
        </template>
        <template v-if="column.key === 'action'">
          <Space>
            <Button size="small" type="link" @click="openEdit(record)">
              编辑
            </Button>
            <Popconfirm title="确认删除该菜单及其子菜单？" @confirm="remove(record)">
              <Button danger size="small" type="link">删除</Button>
            </Popconfirm>
          </Space>
        </template>
      </template>
    </Table>

    <Modal
      v-model:open="modalOpen"
      :confirm-loading="saving"
      title="菜单信息"
      width="640px"
      @ok="save"
    >
      <Form layout="vertical">
        <Form.Item label="上级菜单">
          <Select v-model:value="form.pid" :options="parentOptions" />
        </Form.Item>
        <Form.Item label="类型">
          <Select v-model:value="form.type" :options="typeOptions" />
        </Form.Item>
        <Form.Item label="菜单名称（显示）">
          <Input v-model:value="form.title" placeholder="如：用户管理" />
        </Form.Item>
        <Form.Item label="菜单标识（唯一英文名）">
          <Input v-model:value="form.name" placeholder="如：SystemUser" />
        </Form.Item>
        <Form.Item label="路由路径">
          <Input v-model:value="form.path" placeholder="如：/system/user" />
        </Form.Item>
        <Form.Item label="组件路径">
          <Input
            v-model:value="form.component"
            placeholder="如：/system/user/list（目录可留空）"
          />
        </Form.Item>
        <Form.Item label="图标">
          <Input v-model:value="form.icon" placeholder="如：lucide:users" />
        </Form.Item>
        <Form.Item label="权限码">
          <Input
            v-model:value="form.authCode"
            placeholder="如：System:User:List"
          />
        </Form.Item>
        <Form.Item label="排序">
          <InputNumber v-model:value="form.sort" :min="0" />
        </Form.Item>
        <Form.Item label="启用">
          <Switch
            v-model:checked="form.status"
            :checked-value="1"
            :un-checked-value="0"
          />
        </Form.Item>
      </Form>
    </Modal>
  </PageShell>
</template>
