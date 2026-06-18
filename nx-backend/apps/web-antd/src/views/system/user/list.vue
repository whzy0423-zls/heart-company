<script setup lang="ts">
import type { SystemRole, SystemUser } from '#/api';

import { computed, onMounted, ref } from 'vue';

import { Button, Form, Input, message, Modal, Select, Space, Switch, Table } from 'ant-design-vue';

import { deleteSystemUserApi, getSystemRoleListApi, getSystemUserListApi, saveSystemUserApi } from '#/api';

import ImagePathInput from '../../site-config/components/image-path-input.vue';
import PageShell from '../components/page-shell.vue';

const loading = ref(false);
const saving = ref(false);
const modalOpen = ref(false);
const users = ref<SystemUser[]>([]);
const roles = ref<SystemRole[]>([]);
const total = ref(0);
const query = ref({ username: '' });
const form = ref<SystemUser>({ avatar: '', nickname: '', roleIds: [], status: 1, username: '' });

const roleOptions = computed(() => roles.value.map((item) => ({ label: item.name, value: item.id })));

const columns = [
  { dataIndex: 'username', title: '账号' },
  { dataIndex: 'nickname', title: '昵称' },
  { dataIndex: 'email', title: '邮箱' },
  { dataIndex: 'status', title: '状态' },
  { dataIndex: 'createTime', title: '创建时间' },
  { key: 'action', title: '操作', width: 160 },
];

async function load() {
  loading.value = true;
  try {
    const [userPage, rolePage] = await Promise.all([
      getSystemUserListApi(query.value),
      getSystemRoleListApi({ pageSize: 100 }),
    ]);
    users.value = userPage.items;
    total.value = userPage.total;
    roles.value = rolePage.items;
  } finally {
    loading.value = false;
  }
}

function openCreate() {
  form.value = { avatar: '', email: '', nickname: '', remark: '', roleIds: [], status: 1, username: '' };
  modalOpen.value = true;
}

function openEdit(record: SystemUser) {
  form.value = { ...record, roleIds: [...record.roleIds] };
  modalOpen.value = true;
}

async function save() {
  saving.value = true;
  try {
    await saveSystemUserApi(form.value);
    message.success('已保存用户');
    modalOpen.value = false;
    await load();
  } finally {
    saving.value = false;
  }
}

async function remove(record: SystemUser) {
  if (!record.id) return;
  await deleteSystemUserApi(record.id);
  message.success('已删除用户');
  await load();
}

onMounted(load);
</script>

<template>
  <PageShell description="维护后台登录账号、状态和角色归属。" :loading="loading" title="用户管理" @create="openCreate" @refresh="load">
    <Space class="toolbar">
      <Input v-model:value="query.username" allow-clear placeholder="搜索账号/昵称" />
      <Button type="primary" @click="load">查询</Button>
    </Space>
    <Table :columns="columns" :data-source="users" :pagination="{ total }" row-key="id">
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'status'">
          <span>{{ record.status === 1 ? '启用' : '停用' }}</span>
        </template>
        <template v-if="column.key === 'action'">
          <Space>
            <Button size="small" type="link" @click="openEdit(record)">编辑</Button>
            <Button danger size="small" type="link" @click="remove(record)">删除</Button>
          </Space>
        </template>
      </template>
    </Table>

    <Modal v-model:open="modalOpen" :confirm-loading="saving" title="用户信息" @ok="save">
      <Form layout="vertical">
        <Form.Item label="账号"><Input v-model:value="form.username" /></Form.Item>
        <Form.Item :label="form.id ? '密码（留空则不修改）' : '密码'">
          <Input.Password v-model:value="form.password" placeholder="请输入登录密码" />
        </Form.Item>
        <Form.Item label="头像"><ImagePathInput v-model:value="form.avatar" dir="user-avatars" empty-text="未设置头像" upload-text="上传头像" /></Form.Item>
        <Form.Item label="昵称"><Input v-model:value="form.nickname" /></Form.Item>
        <Form.Item label="邮箱"><Input v-model:value="form.email" /></Form.Item>
        <Form.Item label="角色"><Select v-model:value="form.roleIds" mode="multiple" :options="roleOptions" /></Form.Item>
        <Form.Item label="启用"><Switch v-model:checked="form.status" :checked-value="1" :un-checked-value="0" /></Form.Item>
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
