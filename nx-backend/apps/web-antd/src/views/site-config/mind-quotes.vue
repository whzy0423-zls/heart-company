<script setup lang="ts">
import type { MindGroup, MindQuote } from '#/api';

import { computed, onMounted, ref } from 'vue';

import { Page } from '@vben/common-ui';

import {
  Button,
  Card,
  Col,
  Empty,
  Form,
  Input,
  InputNumber,
  message,
  Modal,
  Popconfirm,
  Row,
  Select,
  Space,
  Switch,
  Table,
  Tag,
} from 'ant-design-vue';

import {
  createMindQuoteApi,
  deleteMindGroupApi,
  deleteMindQuoteApi,
  getMindGroupsApi,
  getMindQuotesApi,
  saveMindGroupApi,
  updateMindQuoteApi,
} from '#/api';

const groups = ref<MindGroup[]>([]);
const quotes = ref<MindQuote[]>([]);
const total = ref(0);
const loadingGroups = ref(false);
const loadingQuotes = ref(false);
// 当前选中分组筛选：'' 全部, '0' 未分组, 其它为分组 id
const activeGroup = ref<string>('');

// 分组弹窗
const groupModal = ref(false);
const groupSaving = ref(false);
const groupForm = ref<{
  id?: string;
  intro: string;
  name: string;
  sort: number;
  status: string;
}>(emptyGroup());

// 心语弹窗
const quoteModal = ref(false);
const quoteSaving = ref(false);
const quoteForm = ref<{
  content: string;
  groupId: string;
  id?: string;
  prompt: string;
  sort: number;
  status: string;
  title: string;
}>(emptyQuote());

function emptyGroup() {
  return { intro: '', name: '', sort: 0, status: 'enabled' };
}
function emptyQuote() {
  return {
    content: '',
    groupId: '',
    prompt: '',
    sort: 0,
    status: 'enabled',
    title: '',
  };
}

const groupOptions = computed(() => [
  { label: '未分组', value: '' },
  ...groups.value.map((g) => ({ label: g.name, value: g.id })),
]);

const quoteColumns = [
  { dataIndex: 'sort', title: '排序', width: 70 },
  { dataIndex: 'title', title: '简短文案' },
  { dataIndex: 'groupId', title: '所属分组', width: 130 },
  { dataIndex: 'status', title: '状态', width: 80 },
  { key: 'action', title: '操作', width: 140 },
];

function quoteRecord(record: Record<string, any>): MindQuote {
  return record as MindQuote;
}

function groupName(id: string) {
  if (!id) return '未分组';
  return groups.value.find((g) => g.id === id)?.name ?? '未分组';
}

onMounted(async () => {
  await loadGroups();
  await loadQuotes();
});

async function loadGroups() {
  loadingGroups.value = true;
  try {
    const res = await getMindGroupsApi();
    groups.value = res?.items ?? [];
  } finally {
    loadingGroups.value = false;
  }
}

async function loadQuotes() {
  loadingQuotes.value = true;
  try {
    const params: Record<string, any> = { pageSize: 100 };
    if (activeGroup.value === '0') params.groupId = '0';
    else if (activeGroup.value) params.groupId = activeGroup.value;
    const res = await getMindQuotesApi(params);
    quotes.value = res?.items ?? [];
    total.value = res?.total ?? 0;
  } finally {
    loadingQuotes.value = false;
  }
}

function pickGroup(id: string) {
  activeGroup.value = id;
  loadQuotes();
}

// ---- 分组 CRUD ----
function openCreateGroup() {
  groupForm.value = emptyGroup();
  groupModal.value = true;
}
function openEditGroup(g: MindGroup) {
  groupForm.value = {
    id: g.id,
    intro: g.intro,
    name: g.name,
    sort: g.sort,
    status: g.status,
  };
  groupModal.value = true;
}
async function saveGroup() {
  if (!groupForm.value.name.trim()) {
    message.warning('请填写分组名称');
    return;
  }
  groupSaving.value = true;
  try {
    await saveMindGroupApi(groupForm.value);
    message.success('已保存');
    groupModal.value = false;
    await loadGroups();
  } finally {
    groupSaving.value = false;
  }
}
async function removeGroup(g: MindGroup) {
  await deleteMindGroupApi(g.id);
  message.success('已删除分组（其下心语转为未分组）');
  if (activeGroup.value === g.id) activeGroup.value = '';
  await loadGroups();
  await loadQuotes();
}

// ---- 心语 CRUD ----
function openCreateQuote() {
  quoteForm.value = emptyQuote();
  // 默认归到当前选中分组
  if (activeGroup.value && activeGroup.value !== '0') {
    quoteForm.value.groupId = activeGroup.value;
  }
  quoteModal.value = true;
}
function openEditQuote(q: MindQuote) {
  quoteForm.value = {
    content: q.content,
    groupId: q.groupId ?? '',
    id: q.id,
    prompt: q.prompt,
    sort: q.sort,
    status: q.status,
    title: q.title,
  };
  quoteModal.value = true;
}
async function saveQuote() {
  if (!quoteForm.value.title?.trim()) {
    message.warning('请填写简短文案');
    return;
  }
  quoteSaving.value = true;
  try {
    const payload = { ...quoteForm.value };
    await (quoteForm.value.id
      ? updateMindQuoteApi(quoteForm.value.id, payload)
      : createMindQuoteApi(payload));
    message.success('已保存');
    quoteModal.value = false;
    await loadQuotes();
    await loadGroups(); // 刷新各组数量
  } finally {
    quoteSaving.value = false;
  }
}
async function removeQuote(q: MindQuote) {
  await deleteMindQuoteApi(q.id);
  message.success('已删除');
  await loadQuotes();
  await loadGroups();
}
</script>

<template>
  <Page
    title="心语管理"
    description="维护「成长心语」的分组与心语。分组对应脑/心/腹等中心，可新增；每条心语含官网展示的简短文案与详情页的完整原文。"
  >
    <Row :gutter="16">
      <!-- 左：分组 -->
      <Col :md="8" :xs="24">
        <Card title="分组" :bordered="false" :loading="loadingGroups">
          <template #extra>
            <Button size="small" type="primary" @click="openCreateGroup">
              新增分组
            </Button>
          </template>
          <div
            class="group-item"
            :class="{ on: activeGroup === '' }"
            @click="pickGroup('')"
          >
            <span class="group-item__name">全部心语</span>
            <span class="group-item__count">{{ total }}</span>
          </div>
          <div
            class="group-item"
            :class="{ on: activeGroup === '0' }"
            @click="pickGroup('0')"
          >
            <span class="group-item__name">未分组</span>
          </div>
          <div
            v-for="g in groups"
            :key="g.id"
            class="group-item"
            :class="{ on: activeGroup === g.id }"
            @click="pickGroup(g.id)"
          >
            <div class="group-item__main">
              <span class="group-item__name">{{ g.name }}</span>
              <span class="group-item__intro">{{ g.intro }}</span>
            </div>
            <div class="group-item__ops" @click.stop>
              <Tag color="blue">{{ g.quoteCount ?? 0 }}</Tag>
              <a @click="openEditGroup(g)">编辑</a>
              <Popconfirm
                title="删除该分组？其下心语将转为未分组"
                @confirm="removeGroup(g)"
              >
                <a class="danger">删除</a>
              </Popconfirm>
            </div>
          </div>
          <Empty v-if="groups.length === 0" description="还没有分组" />
        </Card>
      </Col>

      <!-- 右：心语 -->
      <Col :md="16" :xs="24">
        <Card
          :title="`心语列表 · ${groupName(activeGroup === '0' ? '' : activeGroup)}`"
          :bordered="false"
        >
          <template #extra>
            <Space>
              <Button size="small" @click="loadQuotes">刷新</Button>
              <Button size="small" type="primary" @click="openCreateQuote">
                新增心语
              </Button>
            </Space>
          </template>
          <Table
            :columns="quoteColumns"
            :data-source="quotes"
            :loading="loadingQuotes"
            :pagination="false"
            row-key="id"
            size="small"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'groupId'">
                <Tag>{{ groupName(record.groupId) }}</Tag>
              </template>
              <template v-else-if="column.dataIndex === 'status'">
                <Tag :color="record.status === 'enabled' ? 'green' : 'default'">
                  {{ record.status === 'enabled' ? '启用' : '停用' }}
                </Tag>
              </template>
              <template v-else-if="column.key === 'action'">
                <Space>
                  <a @click="openEditQuote(quoteRecord(record))">编辑</a>
                  <Popconfirm
                    title="确认删除这条心语？"
                    @confirm="removeQuote(quoteRecord(record))"
                  >
                    <a class="danger">删除</a>
                  </Popconfirm>
                </Space>
              </template>
            </template>
          </Table>
        </Card>
      </Col>
    </Row>

    <!-- 分组弹窗 -->
    <Modal
      v-model:open="groupModal"
      :title="groupForm.id ? '编辑分组' : '新增分组'"
      :confirm-loading="groupSaving"
      @ok="saveGroup"
    >
      <Form layout="vertical">
        <Form.Item label="分组名称" required>
          <Input
            v-model:value="groupForm.name"
            placeholder="如 脑组（1·5·6）"
          />
        </Form.Item>
        <Form.Item label="分组简介">
          <Input.TextArea
            v-model:value="groupForm.intro"
            :rows="3"
            placeholder="一句话说明这个分组"
          />
        </Form.Item>
        <Form.Item label="排序">
          <InputNumber v-model:value="groupForm.sort" :min="0" />
        </Form.Item>
        <Form.Item label="启用">
          <Switch
            :checked="groupForm.status === 'enabled'"
            @change="(v) => (groupForm.status = v ? 'enabled' : 'disabled')"
          />
        </Form.Item>
      </Form>
    </Modal>

    <!-- 心语弹窗 -->
    <Modal
      v-model:open="quoteModal"
      :title="quoteForm.id ? '编辑心语' : '新增心语'"
      :confirm-loading="quoteSaving"
      width="640px"
      @ok="saveQuote"
    >
      <Form layout="vertical">
        <Form.Item label="所属分组">
          <Select v-model:value="quoteForm.groupId" :options="groupOptions" />
        </Form.Item>
        <Form.Item label="简短文案（官网卡片展示）" required>
          <Input.TextArea
            v-model:value="quoteForm.title"
            :rows="2"
            placeholder="一句话提炼，用户在官网列表里看到的就是这句"
          />
        </Form.Item>
        <Form.Item label="完整原文（详情页展示）">
          <Input.TextArea
            v-model:value="quoteForm.content"
            :rows="8"
            placeholder="老韩语录原文"
          />
        </Form.Item>
        <Form.Item label="回应提示">
          <Input
            v-model:value="quoteForm.prompt"
            placeholder="引导用户思考的一句话"
          />
        </Form.Item>
        <Row :gutter="16">
          <Col :span="12">
            <Form.Item label="排序">
              <InputNumber
                v-model:value="quoteForm.sort"
                :min="0"
                style="width: 100%"
              />
            </Form.Item>
          </Col>
          <Col :span="12">
            <Form.Item label="启用">
              <Switch
                :checked="quoteForm.status === 'enabled'"
                @change="(v) => (quoteForm.status = v ? 'enabled' : 'disabled')"
              />
            </Form.Item>
          </Col>
        </Row>
      </Form>
    </Modal>
  </Page>
</template>

<style scoped>
.group-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  margin-bottom: 8px;
  cursor: pointer;
  border: 1px solid hsl(var(--border));
  border-radius: 8px;
  transition: all 0.2s;
}

.group-item:hover {
  border-color: hsl(var(--primary));
}

.group-item.on {
  background: hsl(var(--primary) / 8%);
  border-color: hsl(var(--primary));
}

.group-item__main {
  display: flex;
  flex-direction: column;
  gap: 2px;
  overflow: hidden;
}

.group-item__name {
  font-weight: 600;
}

.group-item__intro {
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 12px;
  color: hsl(var(--muted-foreground));
  white-space: nowrap;
}

.group-item__ops {
  display: flex;
  flex-shrink: 0;
  gap: 8px;
  align-items: center;
  font-size: 12px;
}

.danger {
  color: hsl(var(--destructive, 0 84% 60%));
}
</style>
