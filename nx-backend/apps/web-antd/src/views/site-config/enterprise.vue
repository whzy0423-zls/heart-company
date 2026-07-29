<script lang="ts">
import type {
  EnterpriseConfig,
  EnterpriseProcessStep,
  EnterpriseServiceItem,
} from '#/api';

const DEFAULT_SERVICE_MODES: EnterpriseServiceItem[] = [
  { title: '企业内训', description: '围绕企业当下议题设计半天或全天共学。' },
  {
    title: '团队工作坊',
    description: '用互动练习帮助团队建立沟通和协作共识。',
  },
  {
    title: '管理者培训',
    description: '支持管理者识别不同类型成员的动机与压力反应。',
  },
];
const DEFAULT_PROCESS_STEPS: EnterpriseProcessStep[] = [
  {
    title: '需求沟通',
    description: '先了解团队背景、参与对象和希望解决的问题。',
  },
  {
    title: '方案共创',
    description: '结合九型主题、课件内容和企业节奏设计服务方式。',
  },
  {
    title: '落地交付',
    description: '完成课程或工作坊后，沉淀可复盘的团队语言。',
  },
];

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function text(value: unknown, fallback: string) {
  return typeof value === 'string' && value.trim() ? value.trim() : fallback;
}

function normalizeBookingItems<T extends EnterpriseServiceItem>(
  value: unknown,
  defaults: T[],
): T[] {
  if (!Array.isArray(value)) return defaults.map((item) => ({ ...item }));
  return value
    .filter(isRecord)
    .map((item) => ({
      ...item,
      description: text(item.description, ''),
      title: text(item.title, ''),
    }))
    .filter((item) => item.title || item.description) as T[];
}

export function ensureEnterpriseBookingFields(config: { home?: unknown }) {
  const home = isRecord(config.home) ? config.home : {};
  if (config.home !== home) config.home = home;
  const source = isRecord(home.enterprise) ? home.enterprise : {};
  const enterprise: EnterpriseConfig = {
    ...source,
    buttonHref: text(source.buttonHref, '#signup'),
    buttonText: text(source.buttonText, '了解企业课程'),
    eyebrow: text(source.eyebrow, '企业课程'),
    items: normalizeBookingItems(source.items, DEFAULT_SERVICE_MODES),
    lead: text(
      source.lead,
      '面向企业组织提供以九型为底层的团队建设、团队疏导、组织文化建设与管理沟通，帮助团队提升协作与积极性。',
    ),
    moduleTitle: text(source.moduleTitle, '工作坊模块'),
    modules: Array.isArray(source.modules)
      ? source.modules
          .filter((item): item is string => typeof item === 'string')
          .map((item) => item.trim())
          .filter(Boolean)
      : [],
    processSteps: normalizeBookingItems(
      source.processSteps,
      DEFAULT_PROCESS_STEPS,
    ),
    title: text(source.title, '团队士气及凝聚力教练工作坊'),
  } as EnterpriseConfig;
  home.enterprise = enterprise;
  return enterprise;
}

function defaultServiceModes() {
  return DEFAULT_SERVICE_MODES.map((item) => ({ ...item }));
}

function defaultProcessSteps() {
  return DEFAULT_PROCESS_STEPS.map((item) => ({ ...item }));
}
</script>

<script setup lang="ts">
import { computed, watch } from 'vue';

import { Button, Card, Col, Form, Input, Row, Textarea } from 'ant-design-vue';

import EditorShell from './components/editor-shell.vue';
import { useSiteConfigEditor } from './use-site-config-editor';

const { config, linesToArray, loading, saveConfig, saving } =
  useSiteConfigEditor();
const enterprise = computed<EnterpriseConfig | undefined>(
  () => config.value?.home.enterprise,
);

watch(
  config,
  (current) => {
    if (current) ensureEnterpriseBookingFields(current);
  },
  { immediate: true },
);

function moveItem(
  items: EnterpriseProcessStep[] | EnterpriseServiceItem[],
  index: number,
  direction: -1 | 1,
) {
  const nextIndex = index + direction;
  if (nextIndex < 0 || nextIndex >= items.length) return;
  [items[index], items[nextIndex]] = [items[nextIndex]!, items[index]!];
}

function addService() {
  enterprise.value?.items.push({ title: '', description: '' });
}

function addProcessStep() {
  enterprise.value?.processSteps.push({ title: '', description: '' });
}

function restoreServiceDefaults() {
  if (enterprise.value) enterprise.value.items = defaultServiceModes();
}

function restoreProcessDefaults() {
  if (enterprise.value) enterprise.value.processSteps = defaultProcessSteps();
}

async function saveEnterpriseConfig() {
  if (config.value) ensureEnterpriseBookingFields(config.value);
  await saveConfig();
}
</script>

<template>
  <EditorShell
    description="配置首页企业课程、预约服务方式和合作流程。"
    :loading="loading"
    :saving="saving"
    title="企业课程"
    @save="saveEnterpriseConfig"
  >
    <Form v-if="enterprise" layout="vertical">
      <Row :gutter="16">
        <Col :md="8" :xs="24"
          ><Form.Item label="Eyebrow"
            ><Input
              v-model:value="enterprise.eyebrow"
              placeholder="请输入Eyebrow" /></Form.Item
        ></Col>
        <Col :md="16" :xs="24"
          ><Form.Item label="标题"
            ><Input
              v-model:value="enterprise.title"
              placeholder="请输入标题" /></Form.Item
        ></Col>
        <Col :xs="24"
          ><Form.Item label="描述"
            ><Textarea
              v-model:value="enterprise.lead"
              :rows="4"
              placeholder="请输入描述" /></Form.Item
        ></Col>
        <Col :md="12" :xs="24"
          ><Form.Item label="按钮文字"
            ><Input
              v-model:value="enterprise.buttonText"
              placeholder="请输入按钮文字" /></Form.Item
        ></Col>
        <Col :md="12" :xs="24"
          ><Form.Item label="按钮链接"
            ><Input
              v-model:value="enterprise.buttonHref"
              placeholder="请输入按钮链接" /></Form.Item
        ></Col>
        <Col :md="12" :xs="24"
          ><Form.Item label="模块标题"
            ><Input
              v-model:value="enterprise.moduleTitle"
              placeholder="请输入模块标题" /></Form.Item
        ></Col>
        <Col :xs="24"
          ><Form.Item label="工作坊模块，每行一条"
            ><Textarea
              :rows="5"
              :value="enterprise.modules.join('\n')"
              @update:value="enterprise.modules = linesToArray($event)"
              placeholder="请输入工作坊模块，每行一条" /></Form.Item
        ></Col>
      </Row>

      <Card class="booking-card" title="预约服务方式">
        <template #extra
          ><Button data-testid="add-service" @click="addService"
            >新增服务方式</Button
          ></template
        >
        <p v-if="enterprise.items.length === 0" class="empty-text">
          暂无服务方式，可恢复默认配置。
        </p>
        <Button
          v-if="enterprise.items.length === 0"
          data-testid="restore-service-defaults"
          @click="restoreServiceDefaults"
          >恢复默认服务方式</Button
        >
        <Card
          v-for="(item, index) in enterprise.items"
          :key="`service-${index}`"
          class="repeater-card"
          size="small"
        >
          <Row :gutter="16">
            <Col :md="10" :xs="24"
              ><Form.Item label="服务名称"
                ><Input
                  v-model:value="item.title"
                  :data-testid="`service-title-${index}`"
                  placeholder="请输入服务名称" /></Form.Item
            ></Col>
            <Col :md="14" :xs="24"
              ><Form.Item label="服务说明"
                ><Textarea
                  v-model:value="item.description"
                  :data-testid="`service-description-${index}`"
                  :rows="2"
                  placeholder="请输入服务说明" /></Form.Item
            ></Col>
          </Row>
          <div class="repeater-actions">
            <Button
              :data-testid="`service-move-up-${index}`"
              :disabled="index === 0"
              @click="moveItem(enterprise.items, index, -1)"
              >上移</Button
            ><Button
              :data-testid="`service-move-down-${index}`"
              :disabled="index === enterprise.items.length - 1"
              @click="moveItem(enterprise.items, index, 1)"
              >下移</Button
            ><Button
              :data-testid="`service-remove-${index}`"
              danger
              @click="enterprise.items.splice(index, 1)"
              >删除</Button
            >
          </div>
        </Card>
      </Card>

      <Card class="booking-card" title="合作流程">
        <template #extra
          ><Button data-testid="add-process-step" @click="addProcessStep"
            >新增流程</Button
          ></template
        >
        <p v-if="enterprise.processSteps.length === 0" class="empty-text">
          暂无合作流程，可恢复默认配置。
        </p>
        <Button
          v-if="enterprise.processSteps.length === 0"
          data-testid="restore-process-defaults"
          @click="restoreProcessDefaults"
          >恢复默认合作流程</Button
        >
        <Card
          v-for="(item, index) in enterprise.processSteps"
          :key="`process-${index}`"
          class="repeater-card"
          size="small"
        >
          <Row :gutter="16">
            <Col :md="10" :xs="24"
              ><Form.Item label="流程名称"
                ><Input
                  v-model:value="item.title"
                  :data-testid="`process-title-${index}`"
                  placeholder="请输入流程名称" /></Form.Item
            ></Col>
            <Col :md="14" :xs="24"
              ><Form.Item label="流程说明"
                ><Textarea
                  v-model:value="item.description"
                  :data-testid="`process-description-${index}`"
                  :rows="2"
                  placeholder="请输入流程说明" /></Form.Item
            ></Col>
          </Row>
          <div class="repeater-actions">
            <Button
              :data-testid="`process-move-up-${index}`"
              :disabled="index === 0"
              @click="moveItem(enterprise.processSteps, index, -1)"
              >上移</Button
            ><Button
              :data-testid="`process-move-down-${index}`"
              :disabled="index === enterprise.processSteps.length - 1"
              @click="moveItem(enterprise.processSteps, index, 1)"
              >下移</Button
            ><Button
              :data-testid="`process-remove-${index}`"
              danger
              @click="enterprise.processSteps.splice(index, 1)"
              >删除</Button
            >
          </div>
        </Card>
      </Card>
    </Form>
  </EditorShell>
</template>

<style scoped>
.booking-card + .booking-card {
  margin-top: 16px;
}
.repeater-card + .repeater-card {
  margin-top: 12px;
}
.repeater-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.empty-text {
  color: hsl(var(--muted-foreground));
}
</style>
