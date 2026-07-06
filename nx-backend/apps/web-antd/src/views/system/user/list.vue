<script setup lang="ts">
import type { SystemRole, SystemUser } from '#/api';

import { computed, onMounted, ref } from 'vue';

import { useAccessStore } from '@vben/stores';

import {
  Button,
  Form,
  Input,
  message,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
} from 'ant-design-vue';

import {
  deleteSystemUserApi,
  getSystemRoleListApi,
  getSystemUserListApi,
  saveSystemUserApi,
} from '#/api';

import ImagePathInput from '../../site-config/components/image-path-input.vue';
import PageShell from '../components/page-shell.vue';

const accessStore = useAccessStore();
const loading = ref(false);
const saving = ref(false);
const modalOpen = ref(false);
const users = ref<SystemUser[]>([]);
const roles = ref<SystemRole[]>([]);
const total = ref(0);
const query = ref({ page: 1, pageSize: 20, username: '' });
const form = ref<SystemUser>({
  avatar: '',
  nickname: '',
  roleIds: [],
  status: 1,
  username: '',
});

const roleOptions = computed(() =>
  roles.value.map((item) => ({ label: item.name, value: item.id })),
);
const canCreate = computed(() =>
  accessStore.accessCodes.includes('System:User:Create'),
);
const canUpdate = computed(() =>
  accessStore.accessCodes.includes('System:User:Update'),
);
const canDelete = computed(() =>
  accessStore.accessCodes.includes('System:User:Delete'),
);

const columns = [
  { dataIndex: 'username', title: '账号' },
  { dataIndex: 'nickname', title: '昵称' },
  { dataIndex: 'email', title: '邮箱' },
  { dataIndex: 'status', title: '状态' },
  { dataIndex: 'createTime', title: '创建时间' },
  { key: 'action', title: '操作', width: 160 },
];

function userRecord(record: Record<string, any>): SystemUser {
  return record as SystemUser;
}

async function load() {
  loading.value = true;
  try {
    const userPage = await getSystemUserListApi({
      page: query.value.page,
      pageSize: query.value.pageSize,
      username: query.value.username || undefined,
    });
    users.value = userPage.items;
    total.value = userPage.total;
    try {
      const rolePage = await getSystemRoleListApi({ pageSize: 100 });
      roles.value = rolePage.items;
    } catch {
      roles.value = [];
    }
  } finally {
    loading.value = false;
  }
}

function search() {
  query.value.page = 1;
  load();
}

function handleTableChange(pagination: {
  current?: number;
  pageSize?: number;
}) {
  query.value.page = pagination.current ?? 1;
  query.value.pageSize = pagination.pageSize ?? 20;
  load();
}

function openCreate() {
  form.value = {
    avatar: '',
    email: '',
    nickname: '',
    remark: '',
    roleIds: [],
    status: 1,
    username: '',
  };
  modalOpen.value = true;
}

function openEdit(record: SystemUser) {
  form.value = { ...record, roleIds: [...record.roleIds] };
  modalOpen.value = true;
}

function handleStatusChange(checked: boolean | number | string) {
  if (checked === true || checked === 1) {
    form.value.status = 1;
    return;
  }
  Modal.confirm({
    cancelText: '取消',
    content: '停用后该后台用户将无法继续使用已分配的后台权限，请确认。',
    okText: '确认停用',
    okType: 'danger',
    onOk: () => {
      form.value.status = 0;
    },
    title: '确认停用用户',
  });
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
  <PageShell
    description="维护后台登录账号、状态和角色归属。"
    :loading="loading"
    :show-create="canCreate"
    title="用户管理"
    @create="openCreate"
    @refresh="load"
  >
    <Space class="toolbar">
      <Input
        v-model:value="query.username"
        allow-clear
        placeholder="搜索账号/昵称"
      />
      <Button type="primary" @click="search">查询</Button>
    </Space>
    <Table
      :columns="columns"
      :data-source="users"
      :pagination="{
        current: query.page,
        pageSize: query.pageSize,
        showSizeChanger: true,
        total,
      }"
      row-key="id"
      @change="handleTableChange"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.dataIndex === 'status'">
          <span>{{ record.status === 1 ? '启用' : '停用' }}</span>
        </template>
        <template v-if="column.key === 'action'">
          <Space>
            <Button
              v-if="canUpdate"
              size="small"
              type="link"
              @click="openEdit(userRecord(record))"
            >
              编辑
            </Button>
            <Popconfirm
              v-if="canDelete"
              cancel-text="取消"
              ok-text="确认删除"
              placement="topRight"
              :title="`确认删除用户「${record.username || record.nickname || record.id}」吗？此操作不可恢复。`"
              @confirm="remove(userRecord(record))"
            >
              <Button danger size="small" type="link"> 删除 </Button>
            </Popconfirm>
          </Space>
        </template>
      </template>
    </Table>

    <Modal
      v-model:open="modalOpen"
      :confirm-loading="saving"
      title="用户信息"
      width="min(720px, calc(100vw - 32px))"
      @ok="save"
    >
      <Form layout="vertical">
        <Form.Item label="账号">
          <Input v-model:value="form.username" />
        </Form.Item>
        <Form.Item :label="form.id ? '密码（留空则不修改）' : '密码'">
          <Input.Password
            v-model:value="form.password"
            placeholder="请输入登录密码"
          />
        </Form.Item>
        <Form.Item label="头像">
          <ImagePathInput
            v-model:value="form.avatar"
            dir="user-avatars"
            empty-text="未设置头像"
            :store-object-url="false"
            upload-text="上传头像"
          />
        </Form.Item>
        <Form.Item label="昵称">
          <Input v-model:value="form.nickname" />
        </Form.Item>
        <Form.Item label="邮箱"><Input v-model:value="form.email" /></Form.Item>
        <Form.Item label="角色">
          <Select
            v-model:value="form.roleIds"
            mode="multiple"
            :options="roleOptions"
          />
        </Form.Item>
        <Form.Item label="启用">
          <Switch
            :checked="form.status === 1"
            @change="handleStatusChange"
          />
        </Form.Item>
        <Form.Item label="备注">
          <Input v-model:value="form.remark" />
        </Form.Item>
      </Form>
    </Modal>
  </PageShell>
</template>

<style scoped>
.toolbar {
  margin-bottom: 16px;
}
</style>
