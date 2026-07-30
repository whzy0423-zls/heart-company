<script lang="ts">
import type {
  MiniappLearnClassroom,
  MiniappLearnConfig,
  MiniappLearnCoursesSection,
  MiniappLearnHero,
  MiniappLearnQuotesSection,
  MiniappLearnSection,
  MiniappLearnSections,
} from '#/api';

const DEFAULT_LEARN = {
  hero: {
    eyebrow: '老师课堂',
    title: '跟着老师，把九型真正用进工作与生活',
    lead: '从视频与音频课件开始，理解自己、改善关系，也为团队协作建立更清晰的共同语言。',
    meta: ['视频课程', '音频精讲', '九型实践'],
  },
  classroom: {
    eyebrow: '课堂精选',
    title: '视频与音频课件',
    moreText: '查看全部',
    heroEyebrow: '随时回看 · 反复练习',
    heroTitle: '把老师以往开课内容，整理成可以持续学习的专业课件',
    heroLead: '支持视频和音频；先看独立课件，也可以进入系列课程循序学习。',
    ctaText: '进入老师课堂',
    emptyTitle: '老师课堂正在准备中',
    emptyDescription:
      '可以先浏览老师介绍和课程方向，新的视频与音频课件会在这里持续更新。',
    emptyActionText: '进入课堂看看',
  },
  sections: {
    teacher: { eyebrow: '老师简介', title: '认识你的学习向导' },
    courses: {
      eyebrow: '课程方向',
      title: '循序建立九型视角',
      emptyTitle: '课程方向正在整理中',
      emptyDescription:
        '更多面向个人成长、关系沟通与企业团队的学习主题会持续补充。',
    },
    types: { eyebrow: '九型内容', title: '九种性格，九条成长路径' },
    quotes: {
      eyebrow: '课堂一念',
      title: '把觉察带回当下',
      emptyTitle: '课堂语录即将上线',
    },
  },
  bottomCtaText: '先完成测试，建立你的学习地图',
} as const;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function text(value: unknown, fallback: string) {
  return typeof value === 'string' && value.trim() ? value.trim() : fallback;
}

function normalizeTags(value: unknown) {
  const tags = Array.isArray(value)
    ? value
        .filter((item): item is string => typeof item === 'string')
        .map((item) => item.trim())
        .filter(Boolean)
        .slice(0, 3)
    : [];
  return tags.length ? tags : [...DEFAULT_LEARN.hero.meta];
}

function normalizeSection(
  value: unknown,
  fallback: MiniappLearnSection,
): MiniappLearnSection {
  const source = isRecord(value) ? value : {};
  return {
    ...source,
    eyebrow: text(source.eyebrow, fallback.eyebrow),
    title: text(source.title, fallback.title),
  } as MiniappLearnSection;
}

export function ensureMiniappLearn(config: { home?: unknown }) {
  const home = isRecord(config.home) ? config.home : {};
  if (config.home !== home) config.home = home;

  const source = isRecord(home.miniappLearn) ? home.miniappLearn : {};
  const rawHero = isRecord(source.hero) ? source.hero : {};
  const rawClassroom = isRecord(source.classroom) ? source.classroom : {};
  const rawSections = isRecord(source.sections) ? source.sections : {};
  const rawCourses = isRecord(rawSections.courses) ? rawSections.courses : {};
  const rawQuotes = isRecord(rawSections.quotes) ? rawSections.quotes : {};

  const hero: MiniappLearnHero = {
    ...rawHero,
    eyebrow: text(rawHero.eyebrow, DEFAULT_LEARN.hero.eyebrow),
    lead: text(rawHero.lead, DEFAULT_LEARN.hero.lead),
    meta: normalizeTags(rawHero.meta),
    title: text(rawHero.title, DEFAULT_LEARN.hero.title),
  } as MiniappLearnHero;
  const classroom: MiniappLearnClassroom = {
    ...rawClassroom,
    ctaText: text(rawClassroom.ctaText, DEFAULT_LEARN.classroom.ctaText),
    emptyActionText: text(
      rawClassroom.emptyActionText,
      DEFAULT_LEARN.classroom.emptyActionText,
    ),
    emptyDescription: text(
      rawClassroom.emptyDescription,
      DEFAULT_LEARN.classroom.emptyDescription,
    ),
    emptyTitle: text(
      rawClassroom.emptyTitle,
      DEFAULT_LEARN.classroom.emptyTitle,
    ),
    eyebrow: text(rawClassroom.eyebrow, DEFAULT_LEARN.classroom.eyebrow),
    heroEyebrow: text(
      rawClassroom.heroEyebrow,
      DEFAULT_LEARN.classroom.heroEyebrow,
    ),
    heroLead: text(rawClassroom.heroLead, DEFAULT_LEARN.classroom.heroLead),
    heroTitle: text(rawClassroom.heroTitle, DEFAULT_LEARN.classroom.heroTitle),
    moreText: text(rawClassroom.moreText, DEFAULT_LEARN.classroom.moreText),
    title: text(rawClassroom.title, DEFAULT_LEARN.classroom.title),
  } as MiniappLearnClassroom;
  const courses: MiniappLearnCoursesSection = {
    ...normalizeSection(rawCourses, DEFAULT_LEARN.sections.courses),
    emptyDescription: text(
      rawCourses.emptyDescription,
      DEFAULT_LEARN.sections.courses.emptyDescription,
    ),
    emptyTitle: text(
      rawCourses.emptyTitle,
      DEFAULT_LEARN.sections.courses.emptyTitle,
    ),
  } as MiniappLearnCoursesSection;
  const quotes: MiniappLearnQuotesSection = {
    ...normalizeSection(rawQuotes, DEFAULT_LEARN.sections.quotes),
    emptyTitle: text(
      rawQuotes.emptyTitle,
      DEFAULT_LEARN.sections.quotes.emptyTitle,
    ),
  } as MiniappLearnQuotesSection;
  const sections: MiniappLearnSections = {
    ...rawSections,
    courses,
    quotes,
    teacher: normalizeSection(
      rawSections.teacher,
      DEFAULT_LEARN.sections.teacher,
    ),
    types: normalizeSection(rawSections.types, DEFAULT_LEARN.sections.types),
  } as MiniappLearnSections;
  const miniappLearn: MiniappLearnConfig = {
    ...source,
    bottomCtaText: text(source.bottomCtaText, DEFAULT_LEARN.bottomCtaText),
    classroom,
    hero,
    sections,
  } as MiniappLearnConfig;

  home.miniappLearn = miniappLearn;
  return miniappLearn;
}
</script>

<script setup lang="ts">
import { computed, watch } from 'vue';

import { Collapse, Form, Input, Textarea } from 'ant-design-vue';

import EditorShell from '#/views/site-config/components/editor-shell.vue';
import { useSiteConfigEditor } from '#/views/site-config/use-site-config-editor';

const { config, loading, saveConfig, saving } = useSiteConfigEditor();
const miniappLearn = computed<MiniappLearnConfig | undefined>(
  () => config.value?.home.miniappLearn,
);

watch(
  config,
  (current) => {
    if (current) ensureMiniappLearn(current);
  },
  { immediate: true },
);

function updateTags(value: string) {
  if (!miniappLearn.value) return;
  miniappLearn.value.hero.meta = normalizeTags(value.split('\n'));
}

async function saveLearnConfig() {
  if (config.value) ensureMiniappLearn(config.value);
  await saveConfig();
}
</script>

<template>
  <EditorShell
    description="配置学习页文案；视频/音频实际内容请前往“老师课堂”管理。"
    :loading="loading"
    :saving="saving"
    title="学习页管理"
    @save="saveLearnConfig"
  >
    <Form v-if="miniappLearn" layout="vertical">
      <Collapse
        :default-active-key="['hero', 'classroom', 'sections']"
        class="learn-editor"
      >
        <Collapse.Panel key="hero" header="页面主视觉">
          <div class="field-grid">
            <Form.Item label="引导短语">
              <Input
                v-model:value="miniappLearn.hero.eyebrow"
                placeholder="请输入引导短语"
              />
            </Form.Item>
            <Form.Item label="标题">
              <Input
                v-model:value="miniappLearn.hero.title"
                data-testid="learn-hero-title"
                placeholder="请输入标题"
              />
            </Form.Item>
            <Form.Item class="field-grid__wide" label="说明">
              <Textarea
                v-model:value="miniappLearn.hero.lead"
                :rows="3"
                placeholder="请输入说明"
              />
            </Form.Item>
            <Form.Item
              class="field-grid__wide"
              label="标签（每行一条，最多 3 项）"
            >
              <Textarea
                :rows="4"
                :value="miniappLearn.hero.meta.join('\n')"
                data-testid="learn-hero-tags"
                placeholder="每行填写一个标签，例如：关系成长"
                @update:value="updateTags"
              />
            </Form.Item>
          </div>
        </Collapse.Panel>

        <Collapse.Panel key="classroom" header="课堂精选">
          <p class="classroom-note">
            课堂内容由“老师课堂”模块上传和维护，本页只配置展示文案。
          </p>
          <div class="field-grid">
            <Form.Item label="Eyebrow"
              ><Input
                v-model:value="miniappLearn.classroom.eyebrow"
                placeholder="请输入Eyebrow"
            /></Form.Item>
            <Form.Item label="标题"
              ><Input
                v-model:value="miniappLearn.classroom.title"
                placeholder="请输入标题"
            /></Form.Item>
            <Form.Item label="更多按钮"
              ><Input
                v-model:value="miniappLearn.classroom.moreText"
                placeholder="请输入更多按钮文字"
            /></Form.Item>
            <Form.Item label="主视觉引导短语"
              ><Input
                v-model:value="miniappLearn.classroom.heroEyebrow"
                placeholder="请输入引导短语"
            /></Form.Item>
            <Form.Item class="field-grid__wide" label="主视觉标题"
              ><Input
                v-model:value="miniappLearn.classroom.heroTitle"
                placeholder="请输入主视觉标题"
            /></Form.Item>
            <Form.Item class="field-grid__wide" label="主视觉说明"
              ><Textarea
                v-model:value="miniappLearn.classroom.heroLead"
                :rows="3"
                placeholder="请输入主视觉说明"
            /></Form.Item>
            <Form.Item label="行动按钮"
              ><Input
                v-model:value="miniappLearn.classroom.ctaText"
                placeholder="请输入行动按钮文字"
            /></Form.Item>
            <Form.Item label="空态标题"
              ><Input
                v-model:value="miniappLearn.classroom.emptyTitle"
                data-testid="learn-classroom-empty-title"
                placeholder="请输入空态标题"
            /></Form.Item>
            <Form.Item class="field-grid__wide" label="空态说明"
              ><Textarea
                v-model:value="miniappLearn.classroom.emptyDescription"
                :rows="3"
                placeholder="请输入空态说明"
            /></Form.Item>
            <Form.Item label="空态行动按钮"
              ><Input
                v-model:value="miniappLearn.classroom.emptyActionText"
                placeholder="请输入空态行动按钮文字"
            /></Form.Item>
          </div>
        </Collapse.Panel>

        <Collapse.Panel key="sections" header="区块标题与底部 CTA">
          <div class="field-grid">
            <Form.Item label="老师区 Eyebrow"
              ><Input
                v-model:value="miniappLearn.sections.teacher.eyebrow"
                placeholder="请输入老师区 Eyebrow"
            /></Form.Item>
            <Form.Item label="老师区标题"
              ><Input
                v-model:value="miniappLearn.sections.teacher.title"
                placeholder="请输入老师区标题"
            /></Form.Item>
            <Form.Item label="课程区 Eyebrow"
              ><Input
                v-model:value="miniappLearn.sections.courses.eyebrow"
                placeholder="请输入课程区 Eyebrow"
            /></Form.Item>
            <Form.Item label="课程区标题"
              ><Input
                v-model:value="miniappLearn.sections.courses.title"
                placeholder="请输入课程区标题"
            /></Form.Item>
            <Form.Item label="课程空态标题"
              ><Input
                v-model:value="miniappLearn.sections.courses.emptyTitle"
                placeholder="请输入课程空态标题"
            /></Form.Item>
            <Form.Item label="九型区 Eyebrow"
              ><Input
                v-model:value="miniappLearn.sections.types.eyebrow"
                placeholder="请输入九型区 Eyebrow"
            /></Form.Item>
            <Form.Item label="九型区标题"
              ><Input
                v-model:value="miniappLearn.sections.types.title"
                placeholder="请输入九型区标题"
            /></Form.Item>
            <Form.Item label="语录区 Eyebrow"
              ><Input
                v-model:value="miniappLearn.sections.quotes.eyebrow"
                placeholder="请输入语录区 Eyebrow"
            /></Form.Item>
            <Form.Item label="语录区标题"
              ><Input
                v-model:value="miniappLearn.sections.quotes.title"
                placeholder="请输入语录区标题"
            /></Form.Item>
            <Form.Item label="语录空态标题"
              ><Input
                v-model:value="miniappLearn.sections.quotes.emptyTitle"
                placeholder="请输入语录空态标题"
            /></Form.Item>
            <Form.Item class="field-grid__wide" label="课程空态说明"
              ><Textarea
                v-model:value="miniappLearn.sections.courses.emptyDescription"
                :rows="3"
                data-testid="learn-courses-empty-description"
                placeholder="请输入课程空态说明"
            /></Form.Item>
            <Form.Item class="field-grid__wide" label="底部 CTA"
              ><Input
                v-model:value="miniappLearn.bottomCtaText"
                data-testid="learn-bottom-cta"
                placeholder="请输入底部 CTA 文案"
            /></Form.Item>
          </div>
        </Collapse.Panel>
      </Collapse>
    </Form>
  </EditorShell>
</template>

<style scoped>
.learn-editor :deep(.ant-collapse-content-box) {
  padding: 16px;
}
.field-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 16px;
}
.field-grid__wide {
  grid-column: 1 / -1;
}
.classroom-note {
  margin: 0 0 16px;
  color: hsl(var(--muted-foreground));
}
@media (max-width: 900px) {
  .field-grid {
    grid-template-columns: 1fr;
  }
  .field-grid__wide {
    grid-column: auto;
  }
}
</style>
