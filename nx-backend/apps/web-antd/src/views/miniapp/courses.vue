<script lang="ts">
import type { MiniappCourse, MiniappCoursesConfig } from '#/api';

const COURSE_ID_PATTERN = /^[A-Za-z0-9][A-Za-z0-9_-]{0,79}$/;
const MAX_PRICE_CENTS = 99_999_900;

function isRecord(value: unknown): value is Record<string, any> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function text(value: unknown, fallback = '') {
  return typeof value === 'string' && value.trim() ? value.trim() : fallback;
}

function lines(value: unknown) {
  return Array.isArray(value)
    ? value
        .filter((item): item is string => typeof item === 'string')
        .map((item) => item.trim())
        .filter(Boolean)
    : [];
}

function stableLegacyId(title: string, index: number) {
  let hash = 2166136261;
  for (const character of title) {
    hash ^= character.codePointAt(0) ?? 0;
    hash = Math.imul(hash, 16777619);
  }
  return `course-${(hash >>> 0).toString(16)}-${index + 1}`;
}

export function newCourseId() {
  const randomUuid = globalThis.crypto?.randomUUID?.();
  if (randomUuid) return `course-${randomUuid}`;
  return `course-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

function normalizeCourse(value: unknown, index: number): MiniappCourse {
  const source = isRecord(value) ? value : {};
  const title = text(source.title, '新课程');
  const candidateId = text(source.id);
  const id = COURSE_ID_PATTERN.test(candidateId)
    ? candidateId
    : stableLegacyId(title, index);
  const priceCents =
    Number.isInteger(source.priceCents) && source.priceCents >= 0
      ? Number(source.priceCents)
      : 0;
  const paymentMode = priceCents > 0 ? 'paid' : 'consult';
  return {
    ...source,
    id,
    title,
    subtitle: text(source.subtitle),
    description: text(source.description),
    cover: text(source.cover),
    badge: text(source.badge, '成长课堂'),
    format: text(source.format, '主题共学'),
    duration: text(source.duration),
    schedule: text(source.schedule),
    location: text(source.location),
    bullets: lines(source.bullets),
    outline: lines(source.outline),
    notice: text(source.notice),
    enabled: typeof source.enabled === 'boolean' ? source.enabled : true,
    priceCents,
    paymentMode,
  };
}

/** Normalizes the dedicated mini-app course catalog while preserving all other config keys. */
export function normalizeMiniappCourses(config: {
  home?: unknown;
}): MiniappCoursesConfig {
  const home = isRecord(config.home) ? config.home : {};
  if (config.home !== home) config.home = home;
  const hasDedicatedCatalog = Object.prototype.hasOwnProperty.call(
    home,
    'miniappCourses',
  );
  const source = isRecord(home.miniappCourses)
    ? home.miniappCourses
    : !hasDedicatedCatalog && isRecord(home.courses)
      ? home.courses
      : {};
  const rawItems = Array.isArray(source.items) ? source.items : [];
  const catalog: MiniappCoursesConfig = {
    ...(isRecord(home.miniappCourses) ? home.miniappCourses : {}),
    items: rawItems.map((item, index) => normalizeCourse(item, index)),
  };
  home.miniappCourses = catalog;
  return catalog;
}

export function yuanToCents(value: string | number) {
  const input = String(value ?? '')
    .trim()
    .replace(/^¥/, '');
  if (!input) return 0;
  if (!/^(?:0|[1-9]\d*)(?:\.\d{0,2})?$/.test(input)) return null;
  const cents = Math.round(Number(input) * 100);
  return Number.isSafeInteger(cents) && cents <= MAX_PRICE_CENTS ? cents : null;
}

export function centsToYuan(value: unknown) {
  const cents =
    Number.isInteger(value) && Number(value) >= 0 ? Number(value) : 0;
  return (cents / 100).toFixed(2);
}

export function validateCourse(course: Record<string, any>) {
  const errors: string[] = [];
  if (!text(course.title)) errors.push('请填写课程标题');
  if (!COURSE_ID_PATTERN.test(text(course.id)))
    errors.push('课程编号格式不正确，请重新生成');
  const priceCents = Number(course.priceCents);
  if (
    !Number.isInteger(priceCents) ||
    priceCents < 0 ||
    priceCents > MAX_PRICE_CENTS
  ) {
    errors.push('课程价格必须是 0 至 999999 元之间的金额');
  }
  return errors;
}
</script>

<script setup lang="ts">
import { computed, reactive, watch } from 'vue';

import {
  Alert,
  Button,
  Card,
  Col,
  Form,
  Input,
  message,
  Row,
  Switch,
  Textarea,
} from 'ant-design-vue';

import EditorShell from '#/views/site-config/components/editor-shell.vue';
import ImagePathInput from '#/views/site-config/components/image-path-input.vue';
import { useSiteConfigEditor } from '#/views/site-config/use-site-config-editor';

const { config, linesToArray, loading, saveConfig, saving } =
  useSiteConfigEditor();
const priceDrafts = reactive(new Map<string, string>());
const catalog = computed<MiniappCoursesConfig | undefined>(() => {
  const home = config.value?.home;
  return home && isRecord(home.miniappCourses)
    ? (home.miniappCourses as MiniappCoursesConfig)
    : undefined;
});
const courses = computed(() => catalog.value?.items ?? []);

watch(
  config,
  (current) => {
    if (current) {
      normalizeMiniappCourses(current);
      priceDrafts.clear();
    }
  },
  { immediate: true },
);

function priceText(course: MiniappCourse) {
  return (
    priceDrafts.get(course.id) ??
    (course.priceCents ? centsToYuan(course.priceCents) : '')
  );
}

function setPrice(course: MiniappCourse, value: string | number) {
  const raw = String(value ?? '').trim();
  priceDrafts.set(course.id, raw);
  const cents = yuanToCents(raw);
  if (cents !== null) {
    course.priceCents = cents;
    course.paymentMode = cents > 0 ? 'paid' : 'consult';
  }
}

function priceError(course: MiniappCourse) {
  return yuanToCents(priceText(course)) === null
    ? '请输入 0 至 999999 元之间的金额，最多两位小数'
    : '';
}

function addCourse() {
  catalog.value?.items.push({
    id: newCourseId(),
    title: '新课程',
    subtitle: '',
    description: '',
    cover: '',
    badge: '成长课堂',
    format: '主题共学',
    duration: '',
    schedule: '',
    location: '',
    bullets: [],
    outline: [],
    notice: '',
    enabled: true,
    priceCents: 0,
    paymentMode: 'consult',
  });
}

function removeCourse(index: number) {
  catalog.value?.items.splice(index, 1);
}

function moveCourse(index: number, offset: number) {
  const list = catalog.value?.items;
  if (!list) return;
  const next = index + offset;
  if (next < 0 || next >= list.length) return;
  const [item] = list.splice(index, 1);
  if (item) list.splice(next, 0, item);
}

async function saveCourses() {
  if (!config.value || !catalog.value) return;
  const invalidPrice = catalog.value.items.find((course) => {
    const raw = priceDrafts.get(course.id);
    return raw !== undefined && yuanToCents(raw) === null;
  });
  if (invalidPrice) {
    message.error('课程价格请输入 0 至 999999 元之间的金额，最多两位小数');
    return;
  }
  const ids = new Set<string>();
  const duplicateId = catalog.value.items.find((course) => {
    if (ids.has(course.id)) return true;
    ids.add(course.id);
    return false;
  });
  if (duplicateId) {
    message.error('课程编号不可重复，请重新生成后再保存');
    return;
  }
  const errors = catalog.value.items.flatMap((course, index) =>
    validateCourse(course).map((error) => `第 ${index + 1} 门课程：${error}`),
  );
  if (errors.length) {
    message.error(errors[0]);
    return;
  }
  catalog.value.items.forEach((course) => {
    const cents = yuanToCents(priceText(course));
    if (cents !== null) {
      course.priceCents = cents;
      course.paymentMode = cents > 0 ? 'paid' : 'consult';
    }
  });
  await saveConfig('已保存小程序课程配置');
}
</script>

<template>
  <EditorShell
    description="管理小程序报名页的课程产品、展示内容、上下架和微信支付金额；老师课堂里的视频与音频课件请前往「老师课堂」。"
    :loading="loading"
    :saving="saving"
    title="小程序课程配置"
    @save="saveCourses"
  >
    <div v-if="catalog" class="course-editor">
      <Alert
        message="金额留空或填写 0，小程序显示「咨询老师」；填写大于 0 元的金额，自动启用微信支付。金额按元输入，最多两位小数。"
        show-icon
        type="info"
      />
      <div class="course-toolbar">
        <div>
          <h3>报名课程</h3>
          <p>这些课程会展示在小程序「报名」页，与老师课堂媒体内容独立管理。</p>
        </div>
        <Button type="primary" @click="addCourse">新增课程</Button>
      </div>
      <Card
        v-for="(course, index) in courses"
        :key="course.id"
        class="course-card"
        size="small"
      >
        <template #title>
          <div class="course-card-title">
            <span>课程 {{ index + 1 }}</span>
            <code>{{ course.id }}</code>
          </div>
        </template>
        <template #extra>
          <Button
            size="small"
            :disabled="index === 0"
            @click="moveCourse(index, -1)"
            >上移</Button
          >
          <Button
            size="small"
            :disabled="index === courses.length - 1"
            @click="moveCourse(index, 1)"
            >下移</Button
          >
          <Button danger size="small" @click="removeCourse(index)">删除</Button>
        </template>
        <Form layout="vertical">
          <Row :gutter="16">
            <Col :md="16" :xs="24">
              <Form.Item label="课程标题">
                <Input
                  v-model:value="course.title"
                  placeholder="例如：个人成长基础课"
                />
              </Form.Item>
            </Col>
            <Col :md="8" :xs="24">
              <Form.Item label="报名页徽标">
                <Input
                  v-model:value="course.badge"
                  placeholder="例如：成长入门"
                />
              </Form.Item>
            </Col>
            <Col :xs="24">
              <Form.Item label="课程封面">
                <ImagePathInput
                  v-model:value="course.cover"
                  dir="miniapp-courses"
                  variant="input"
                  show-path
                />
              </Form.Item>
            </Col>
            <Col :md="12" :xs="24">
              <Form.Item label="一句话副标题">
                <Input
                  v-model:value="course.subtitle"
                  placeholder="用于报名页课程卡片"
                />
              </Form.Item>
            </Col>
            <Col :md="12" :xs="24">
              <Form.Item label="课程形式">
                <Input
                  v-model:value="course.format"
                  placeholder="例如：线上直播 / 线下工作坊"
                />
              </Form.Item>
            </Col>
            <Col :xs="24">
              <Form.Item label="课程介绍">
                <Textarea
                  v-model:value="course.description"
                  :rows="3"
                  placeholder="介绍课程适合谁、解决什么问题"
                />
              </Form.Item>
            </Col>
            <Col :md="8" :xs="24">
              <Form.Item label="时长">
                <Input
                  v-model:value="course.duration"
                  placeholder="例如：4 周 / 8 小时"
                />
              </Form.Item>
            </Col>
            <Col :md="8" :xs="24">
              <Form.Item label="排期">
                <Input
                  v-model:value="course.schedule"
                  placeholder="例如：每周六 14:00"
                />
              </Form.Item>
            </Col>
            <Col :md="8" :xs="24">
              <Form.Item label="地点">
                <Input
                  v-model:value="course.location"
                  placeholder="例如：线上腾讯会议"
                />
              </Form.Item>
            </Col>
            <Col :md="8" :xs="24">
              <Form.Item
                label="课程价格（元）"
                :help="priceError(course) || '留空或填 0 为咨询老师；正金额自动启用微信支付'"
                :validate-status="priceError(course) ? 'error' : undefined"
              >
                <Input
                  :value="priceText(course)"
                  aria-label="课程价格（元）"
                  inputmode="decimal"
                  placeholder="留空或填 0：咨询老师"
                  @update:value="setPrice(course, $event)"
                />
              </Form.Item>
            </Col>
            <Col :md="8" :xs="24">
              <Form.Item label="小程序报名方式">
                <span>{{ course.paymentMode === 'paid' ? '微信支付' : '咨询老师' }}</span>
              </Form.Item>
            </Col>
            <Col :md="8" :xs="24">
              <Form.Item label="报名页状态">
                <Switch
                  v-model:checked="course.enabled"
                  checked-children="上架"
                  un-checked-children="下架"
                />
                <span class="switch-note">{{
                  course.enabled ? '用户可见' : '暂不展示'
                }}</span>
              </Form.Item>
            </Col>
            <Col :md="12" :xs="24">
              <Form.Item label="课程要点（每行一条）">
                <Textarea
                  :rows="4"
                  :value="course.bullets.join('\n')"
                  placeholder="例如：建立性格觉察的基础"
                  @update:value="course.bullets = linesToArray($event)"
                />
              </Form.Item>
            </Col>
            <Col :md="12" :xs="24">
              <Form.Item label="课程大纲（每行一条）">
                <Textarea
                  :rows="4"
                  :value="course.outline.join('\n')"
                  placeholder="例如：第一周：认识九型"
                  @update:value="course.outline = linesToArray($event)"
                />
              </Form.Item>
            </Col>
            <Col :xs="24">
              <Form.Item label="报名须知">
                <Textarea
                  v-model:value="course.notice"
                  :rows="2"
                  placeholder="费用、退改或开课前提醒"
                />
              </Form.Item>
            </Col>
          </Row>
        </Form>
      </Card>
      <div v-if="!courses.length" class="empty-state">
        暂时没有课程，请点击「新增课程」开始配置。
      </div>
    </div>
  </EditorShell>
</template>

<style scoped>
.course-editor {
  display: flex;
  flex-direction: column;
  gap: 18px;
}
.course-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
}
.course-toolbar h3 {
  margin: 0;
}
.course-toolbar p {
  margin: 6px 0 0;
  color: hsl(var(--muted-foreground));
}
.course-card-title {
  display: flex;
  align-items: center;
  gap: 12px;
}
.course-card-title code {
  color: hsl(var(--muted-foreground));
  font-size: 12px;
  font-weight: 400;
}
.course-card :deep(.ant-card-extra) {
  display: flex;
  gap: 6px;
}
.switch-note {
  display: inline-block;
  margin-left: 10px;
  color: hsl(var(--muted-foreground));
}
.empty-state {
  padding: 36px;
  color: hsl(var(--muted-foreground));
  text-align: center;
  border: 1px dashed hsl(var(--border));
  border-radius: 8px;
}
@media (max-width: 640px) {
  .course-toolbar {
    align-items: flex-start;
    flex-direction: column;
  }
  .course-card :deep(.ant-card-extra) {
    flex-wrap: wrap;
  }
}
</style>
