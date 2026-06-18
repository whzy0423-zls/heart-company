<script setup lang="ts">
import type { SystemMenu, SystemRole } from '#/api';

import { computed, onMounted, ref } from 'vue';

import { Button, Form, Input, message, Modal, Space, Switch, Table, Tree } from 'ant-design-vue';

import { deleteSystemRoleApi, getSystemMenuListApi, getSystemRoleListApi, saveSystemRoleApi } from '#/api';

import PageShell from '../components/page-shell.vue';
import { toMenuTreeNodes } from './menu-tree';

const loading = ref(false);
const saving = ref(false);
const modalOpen = ref(false);
const roles = ref<SystemRole[]>([]);
const menus = ref<SystemMenu[]>([]);
const total = ref(0);
const query = ref({ name: '' });
const form = ref<SystemRole>({ code: '', menuIds: [], name: '', remark: '', status: 1 });
const menuTreeNodes = computed(() => toMenuTreeNodes(menus.value));

const columns = [
  { dataIndex: 'name', title: '角色名称' },
  { dataIndex: 'code', title: '角色编码' },
  { dataIndex: 'status', title: '状态' },
  { dataIndex: 'createTime', title: '创建时间' },
  { key: 'action', title: '操作', width: 160 },
];

async function load() {
  loading.value = true;
  try {
    const [rolePage, menuTree] = await Promise.all([
      getSystemRoleListApi(query.value),
      getSystemMenuListApi(),
    ]);
    roles.value = rolePage.items;
    total.value = rolePage.total;
    menus.value = menuTree;
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  form.value = { code: '', menuIds: [], name: '', remark: '', status: 1 };
  modalOpen.value = true;
}

function openEdit(record: SystemRole) {
  form.value = { ...record, menuIds: [...record.menuIds] };
  modalOpen.value = true;
}

async function save() {
  saving.value = true;
  try {
    await saveSystemRoleApi(form.value);
    message.success('已保存角色');
    modalOpen.value = false;
    await load();
  } finally {
    saving.value = false;
  }
}

async function remove(record: SystemRole) {
  if (!record.id) return;
  await deleteSystemRoleApi(record.id);
  message.success('已删除角色');
  await load();
}

onMounted(load);
</script>

<template>
  <PageShell description="维护角色以及可访问菜单权限。" :loading="loading" title="角色管理" @create="openCreate" @refresh="load">
    <Space class="toolbar">
      <Input v-model:value="query.name" allow-clear placeholder="搜索角色名称/编码" />
      <Button type="primary" @click="load">查询</Button>
    </Space>
    <Table :columns="columns" :data-source="roles" :pagination="{ total }" row-key="id">
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'status'">{{ record.status === 1 ? '启用' : '停用' }}</template>
        <template v-if="column.key === 'action'">
          <Space>
            <Button size="small" type="link" @click="openEdit(record)">编辑</Button>
            <Button danger size="small" type="link" @click="remove(record)">删除</Button>
          </Space>
        </template>
      </template>
    </Table>

    <Modal v-model:open="modalOpen" :confirm-loading="saving" title="角色信息" @ok="save">
      <Form layout="vertical">
        <Form.Item label="角色名称"><Input v-model:value="form.name" /></Form.Item>
        <Form.Item label="角色编码"><Input v-model:value="form.code" /></Form.Item>
        <Form.Item label="启用"><Switch v-model:checked="form.status" :checked-value="1" :un-checked-value="0" /></Form.Item>
        <Form.Item label="菜单权限">
          <Tree
            v-model:checked-keys="form.menuIds"
            checkable
            :tree-data="menuTreeNodes"
          />
        </Form.Item>
        <Form.Item label="备注"><Input v-model:value="form.remark" /></Form.Item>
      </Form>
    </Modal>
  </PageShell>
</template>

<style scoped>
.toolbar {
  margin-bottom: 16px;
}
</style>
