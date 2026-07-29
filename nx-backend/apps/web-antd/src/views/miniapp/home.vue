<script lang="ts">
import type {
  MiniappCarouselConfig,
  MiniappHomeConfig,
  MiniappHomeEntry,
  MiniappHomeEntryKey,
  MiniappHomeIconKey,
  MiniappHomeThemeKey,
} from '#/api';

export const MINIAPP_HOME_ICON_KEYS: readonly MiniappHomeIconKey[] =
  Object.freeze(['compass', 'relation', 'book', 'growth', 'spark', 'heart']);

export const MINIAPP_HOME_THEME_KEYS: readonly MiniappHomeThemeKey[] =
  Object.freeze(['blue', 'purple', 'orange', 'pink', 'cyan']);

const ENTRY_DEFAULTS: Record<MiniappHomeEntryKey, MiniappHomeEntry> = {
  test: {
    description: '找到你的核心动机',
    enabled: true,
    icon: 'compass',
    key: 'test',
    theme: 'blue',
    title: '人格测试',
  },
  relation: {
    description: '看见彼此的互动模式',
    enabled: true,
    icon: 'relation',
    key: 'relation',
    theme: 'purple',
    title: '关系合盘',
  },
  learn: {
    description: '跟着课件系统学习',
    enabled: true,
    icon: 'book',
    key: 'learn',
    theme: 'cyan',
    title: '老师课程',
  },
  profile: {
    description: '记录你的探索轨迹',
    enabled: true,
    icon: 'growth',
    key: 'profile',
    theme: 'orange',
    title: '成长档案',
  },
};

const ENTRY_ORDER: MiniappHomeEntryKey[] = [
  'test',
  'relation',
  'learn',
  'profile',
];

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function normalizeInterval(value: unknown) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return 4000;
  return Math.min(10_000, Math.max(2000, value));
}

function normalizedText(value: unknown, fallback: string) {
  return typeof value === 'string' && value.trim() ? value.trim() : fallback;
}

function normalizedEnabled(value: unknown) {
  return typeof value === 'boolean' ? value : true;
}

function normalizeEntry(
  rawValue: unknown,
  fallback: MiniappHomeEntry,
): MiniappHomeEntry {
  const value = isRecord(rawValue) ? rawValue : {};
  return {
    description: normalizedText(value.description, fallback.description),
    enabled:
      typeof value.enabled === 'boolean' ? value.enabled : fallback.enabled,
    icon: MINIAPP_HOME_ICON_KEYS.includes(value.icon as MiniappHomeIconKey)
      ? (value.icon as MiniappHomeIconKey)
      : fallback.icon,
    key: fallback.key,
    theme: MINIAPP_HOME_THEME_KEYS.includes(value.theme as MiniappHomeThemeKey)
      ? (value.theme as MiniappHomeThemeKey)
      : fallback.theme,
    title: normalizedText(value.title, fallback.title),
  };
}

export function ensureMiniappHome(config: { home?: unknown }) {
  const home = isRecord(config.home) ? config.home : {};
  if (config.home !== home) config.home = home;
  const rawHome = isRecord(home.miniappHome) ? home.miniappHome : {};
  const rawBrand = isRecord(rawHome.brand) ? rawHome.brand : {};
  const rawHero = isRecord(rawHome.hero) ? rawHome.hero : {};
  const rawEntries = isRecord(rawHome.entriesSection)
    ? rawHome.entriesSection
    : {};
  const rawGrowth = isRecord(rawHome.growth) ? rawHome.growth : {};
  const entries: MiniappHomeEntry[] = [];
  const seen = new Set<MiniappHomeEntryKey>();

  if (Array.isArray(rawEntries.items)) {
    for (const rawEntry of rawEntries.items) {
      if (
        !isRecord(rawEntry) ||
        !ENTRY_ORDER.includes(rawEntry.key as MiniappHomeEntryKey)
      ) {
        continue;
      }
      const key = rawEntry.key as MiniappHomeEntryKey;
      if (seen.has(key)) continue;
      seen.add(key);
      entries.push(normalizeEntry(rawEntry, ENTRY_DEFAULTS[key]));
    }
  }
  for (const key of ENTRY_ORDER) {
    if (!seen.has(key)) entries.push({ ...ENTRY_DEFAULTS[key] });
  }

  const miniappHome: MiniappHomeConfig = {
    brand: {
      enabled: normalizedEnabled(rawBrand.enabled),
      name: normalizedText(rawBrand.name, '九型芯之力'),
      tagline: normalizedText(rawBrand.tagline, '看见动机，找到成长方向'),
    },
    hero: {
      buttonText: normalizedText(rawHero.buttonText, '开始人格测试'),
      description: normalizedText(
        rawHero.description,
        '从核心动机出发，在老师课程中理解自己，也更从容地走进关系与成长。',
      ),
      enabled: normalizedEnabled(rawHero.enabled),
      kicker: normalizedText(rawHero.kicker, '老师导学 · 课程配套 · 18 题自测'),
      title: normalizedText(rawHero.title, '读懂自己内在的能量地图'),
    },
    entriesSection: {
      description: normalizedText(
        rawEntries.description,
        '从测试、关系、课程到成长档案，选择此刻最需要的一步。',
      ),
      enabled:
        normalizedEnabled(rawEntries.enabled) &&
        entries.some((entry) => entry.enabled),
      items: entries,
      title: normalizedText(rawEntries.title, '探索你的九型能量'),
    },
    growth: {
      description: normalizedText(
        rawGrowth.description,
        '跟随老师的课程与课件，让理解沉淀为真实的成长行动。',
      ),
      enabled: normalizedEnabled(rawGrowth.enabled),
      eyebrow: normalizedText(rawGrowth.eyebrow, '老师陪伴 · 持续成长'),
      title: normalizedText(rawGrowth.title, '把测试发现带进课程练习'),
    },
  };

  home.miniappHome = miniappHome;
  return miniappHome;
}

export function ensureCarousel(config: { home?: unknown }) {
  const home = isRecord(config.home) ? config.home : {};
  if (config.home !== home) config.home = home;
  const rawCarousel = isRecord(home.miniappCarousel)
    ? home.miniappCarousel
    : {};
  const carousel: MiniappCarouselConfig = {
    autoplay:
      typeof rawCarousel.autoplay === 'boolean' ? rawCarousel.autoplay : true,
    interval: normalizeInterval(rawCarousel.interval),
    items: Array.isArray(rawCarousel.items)
      ? rawCarousel.items.map((rawItem: unknown) => {
          const item = isRecord(rawItem) ? rawItem : {};
          return {
            enabled: typeof item.enabled === 'boolean' ? item.enabled : true,
            image: typeof item.image === 'string' ? item.image : '',
          };
        })
      : [],
  };

  home.miniappCarousel = carousel;
  return carousel;
}
</script>

<script setup lang="ts">
import type { MiniappCarouselItem } from '#/api';

import { computed, watch } from 'vue';

import {
  Button,
  Card,
  Collapse,
  Form,
  Input,
  InputNumber,
  Select,
  Switch,
} from 'ant-design-vue';

import EditorShell from '#/views/site-config/components/editor-shell.vue';
import ImagePathInput from '#/views/site-config/components/image-path-input.vue';
import { useSiteConfigEditor } from '#/views/site-config/use-site-config-editor';

const { config, loading, saveConfig, saving } = useSiteConfigEditor();

const carousel = computed<MiniappCarouselConfig | undefined>(
  () => config.value?.home.miniappCarousel,
);
const miniappHome = computed<MiniappHomeConfig | undefined>(
  () => config.value?.home.miniappHome,
);
const iconLabels: Record<MiniappHomeIconKey, string> = {
  compass: '罗盘',
  relation: '关系',
  book: '书本',
  growth: '成长',
  spark: '星光',
  heart: '爱心',
};
const themeLabels: Record<MiniappHomeThemeKey, string> = {
  blue: '蓝色',
  purple: '紫色',
  orange: '橙色',
  pink: '粉色',
  cyan: '青色',
};
const iconOptions = MINIAPP_HOME_ICON_KEYS.map((value) => ({
  label: iconLabels[value],
  value,
}));
const themeOptions = MINIAPP_HOME_THEME_KEYS.map((value) => ({
  label: themeLabels[value],
  value,
}));
const entryDestinations: Record<MiniappHomeEntryKey, string> = {
  test: '/pages/test/test（页面跳转）',
  relation: '/pages/relation/relation（页面跳转）',
  learn: '/pages/learn/learn（底部标签）',
  profile: '/pages/profile/profile（底部标签）',
};
const itemKeys = new WeakMap<MiniappCarouselItem, string>();
let nextItemKey = 0;

watch(
  config,
  (current) => {
    if (current) {
      ensureCarousel(current);
      ensureMiniappHome(current);
    }
  },
  { immediate: true },
);

function addItem() {
  carousel.value?.items.push({ enabled: true, image: '' });
}

function moveItem(index: number, direction: -1 | 1) {
  const items = carousel.value?.items;
  if (!items) return;

  const nextIndex = index + direction;
  if (nextIndex < 0 || nextIndex >= items.length) return;
  [items[index], items[nextIndex]] = [items[nextIndex]!, items[index]!];
}

function removeItem(index: number) {
  carousel.value?.items.splice(index, 1);
}

function moveHomeEntry(index: number, direction: -1 | 1) {
  const items = miniappHome.value?.entriesSection.items;
  if (!items) return;
  const nextIndex = index + direction;
  if (nextIndex < 0 || nextIndex >= items.length) return;
  [items[index], items[nextIndex]] = [items[nextIndex]!, items[index]!];
}

function itemKey(item: MiniappCarouselItem) {
  const existing = itemKeys.get(item);
  if (existing) return existing;

  const key = `miniapp-carousel-${nextItemKey++}`;
  itemKeys.set(item, key);
  return key;
}
</script>

<template>
  <EditorShell
    description="配置小程序首页轮播、品牌、主视觉、九型测试小游戏独立模块、其他功能入口与成长内容。页面目标、图标和配色均使用固定安全选项。"
    :loading="loading"
    :saving="saving"
    title="首页管理"
    @save="saveConfig"
  >
    <Form v-if="carousel && miniappHome" layout="vertical">
      <Collapse
        :default-active-key="['carousel', 'brand', 'hero', 'entries', 'growth']"
        class="home-editor"
      >
        <Collapse.Panel key="carousel" header="轮播图">
          <div class="carousel-settings">
            <Form.Item label="自动轮播">
              <Switch
                v-model:checked="carousel.autoplay"
                aria-label="自动轮播"
              />
            </Form.Item>
            <Form.Item label="轮播间隔（毫秒）">
              <InputNumber
                v-model:value="carousel.interval"
                data-testid="carousel-interval"
                :max="10_000"
                :min="2000"
                :step="500"
               placeholder="请输入轮播间隔（毫秒）"/>
            </Form.Item>
          </div>

          <div class="section-head">
            <h3>轮播图片</h3>
            <Button @click="addItem">新增轮播图</Button>
          </div>

          <p v-if="carousel.items.length === 0" class="empty-text">
            暂无轮播图，点击“新增轮播图”后上传图片。
          </p>

          <Card
            v-for="(item, index) in carousel.items"
            :key="itemKey(item)"
            class="carousel-item"
            size="small"
          >
            <div class="carousel-item__content">
              <span class="carousel-item__index">{{ index + 1 }}</span>
              <ImagePathInput
                v-model:value="item.image"
                dir="miniapp-home"
                empty-text="未设置轮播图"
                store-object-url
                upload-text="上传图片"
              />
              <Switch
                v-model:checked="item.enabled"
                :aria-label="`轮播图 ${index + 1} 显示状态`"
                :data-testid="`carousel-enabled-${index}`"
                :checked-children="'启用'"
                :un-checked-children="'停用'"
              />
              <div class="carousel-item__actions">
                <Button :disabled="index === 0" @click="moveItem(index, -1)">
                  上移
                </Button>
                <Button
                  :disabled="index === carousel.items.length - 1"
                  @click="moveItem(index, 1)"
                >
                  下移
                </Button>
                <Button danger @click="removeItem(index)">删除</Button>
              </div>
            </div>
          </Card>
        </Collapse.Panel>

        <Collapse.Panel key="brand" header="顶部品牌">
          <div class="section-toggle">
            <span>显示顶部品牌</span>
            <Switch
              v-model:checked="miniappHome.brand.enabled"
              aria-label="顶部品牌显示状态"
              data-testid="brand-enabled"
            />
          </div>
          <div class="field-grid">
            <Form.Item label="品牌名称">
              <Input
                v-model:value="miniappHome.brand.name"
                data-testid="brand-name"
               placeholder="请输入品牌名称"/>
            </Form.Item>
            <Form.Item label="品牌说明">
              <Input v-model:value="miniappHome.brand.tagline"  placeholder="请输入品牌说明"/>
            </Form.Item>
          </div>
          <p class="fixed-destination" data-testid="brand-destination">
            顶部头像固定目标：/pages/profile/profile（底部标签）
          </p>
        </Collapse.Panel>

        <Collapse.Panel key="hero" header="主视觉">
          <div class="section-toggle">
            <span>显示主视觉</span>
            <Switch
              v-model:checked="miniappHome.hero.enabled"
              aria-label="主视觉显示状态"
              data-testid="hero-enabled"
            />
          </div>
          <div class="field-grid">
            <Form.Item label="引导短语">
              <Input v-model:value="miniappHome.hero.kicker"  placeholder="请输入引导短语"/>
            </Form.Item>
            <Form.Item label="标题">
              <Input
                v-model:value="miniappHome.hero.title"
                data-testid="hero-title"
               placeholder="请输入标题"/>
            </Form.Item>
            <Form.Item class="field-grid__wide" label="说明">
              <Input v-model:value="miniappHome.hero.description"  placeholder="请输入说明"/>
            </Form.Item>
            <Form.Item label="按钮文字">
              <Input v-model:value="miniappHome.hero.buttonText"  placeholder="请输入按钮文字"/>
            </Form.Item>
          </div>
          <p class="fixed-destination">
            固定目标：/pages/test/test（页面跳转）
          </p>
        </Collapse.Panel>

        <Collapse.Panel key="entries" header="小游戏与功能入口">
          <p
            class="fixed-destination"
            data-testid="test-game-config-note"
          >
            <strong>九型测试小游戏独立模块：</strong>
            test
            开关控制首页独立小游戏区；test
            配置项所在顺序不影响独立模块位置，其他功能入口仍按下方顺序展示并支持排序。
          </p>
          <div class="section-toggle">
            <span>显示其他功能入口</span>
            <Switch
              v-model:checked="miniappHome.entriesSection.enabled"
              aria-label="其他功能入口区显示状态"
              data-testid="entries-enabled"
            />
          </div>
          <div class="field-grid">
            <Form.Item label="区块标题">
              <Input
                v-model:value="miniappHome.entriesSection.title"
                data-testid="entries-title"
               placeholder="请输入区块标题"/>
            </Form.Item>
            <Form.Item label="区块说明">
              <Input v-model:value="miniappHome.entriesSection.description"  placeholder="请输入区块说明"/>
            </Form.Item>
          </div>

          <Card
            v-for="(entry, index) in miniappHome.entriesSection.items"
            :key="entry.key"
            class="home-entry"
            size="small"
          >
            <div class="home-entry__head">
              <strong :data-testid="`entry-label-${entry.key}`">
                {{
                  entry.key === 'test'
                    ? '九型测试小游戏（首页独立模块）'
                    : entry.title
                }}
              </strong>
              <Switch
                v-model:checked="entry.enabled"
                :aria-label="
                  entry.key === 'test'
                    ? '九型测试小游戏独立模块显示状态'
                    : `${entry.title}入口显示状态`
                "
                :data-testid="`entry-enabled-${entry.key}`"
              />
            </div>
            <p class="fixed-entry-key">
              固定入口键：<code :data-testid="`entry-key-${entry.key}`">{{
                entry.key
              }}</code>
            </p>
            <p
              class="fixed-destination"
              :data-testid="`entry-destination-${entry.key}`"
            >
              固定目标：{{ entryDestinations[entry.key] }}
            </p>
            <div class="field-grid field-grid--entry">
              <Form.Item label="标题">
                <Input v-model:value="entry.title"  placeholder="请输入标题"/>
              </Form.Item>
              <Form.Item label="说明">
                <Input v-model:value="entry.description"  placeholder="请输入说明"/>
              </Form.Item>
              <Form.Item label="预设图标">
                <Select
                  v-model:value="entry.icon"
                  :data-testid="`entry-icon-${entry.key}`"
                  :options="iconOptions"
                 placeholder="请选择预设图标"/>
              </Form.Item>
              <Form.Item label="预设主题色">
                <Select
                  v-model:value="entry.theme"
                  :data-testid="`entry-theme-${entry.key}`"
                  :options="themeOptions"
                 placeholder="请选择预设主题色"/>
              </Form.Item>
            </div>
            <div class="home-entry__actions">
              <Button
                :data-testid="`entry-move-up-${entry.key}`"
                :disabled="index === 0"
                @click="moveHomeEntry(index, -1)"
              >
                上移
              </Button>
              <Button
                :data-testid="`entry-move-down-${entry.key}`"
                :disabled="
                  index === miniappHome.entriesSection.items.length - 1
                "
                @click="moveHomeEntry(index, 1)"
              >
                下移
              </Button>
            </div>
          </Card>
        </Collapse.Panel>

        <Collapse.Panel key="growth" header="成长内容">
          <div class="section-toggle">
            <span>显示成长内容</span>
            <Switch
              v-model:checked="miniappHome.growth.enabled"
              aria-label="成长内容显示状态"
              data-testid="growth-enabled"
            />
          </div>
          <div class="field-grid">
            <Form.Item label="引导短语">
              <Input v-model:value="miniappHome.growth.eyebrow"  placeholder="请输入引导短语"/>
            </Form.Item>
            <Form.Item label="标题">
              <Input
                v-model:value="miniappHome.growth.title"
                data-testid="growth-title"
               placeholder="请输入标题"/>
            </Form.Item>
            <Form.Item class="field-grid__wide" label="说明">
              <Input v-model:value="miniappHome.growth.description"  placeholder="请输入说明"/>
            </Form.Item>
          </div>
          <p class="fixed-destination">
            固定目标：/pages/learn/learn（底部标签）
          </p>
        </Collapse.Panel>
      </Collapse>
    </Form>
  </EditorShell>
</template>

<style scoped>
.home-editor :deep(.ant-collapse-content-box) {
  padding: 16px;
}

.carousel-settings {
  display: flex;
  flex-wrap: wrap;
  gap: 24px;
}

.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: 18px 0 12px;
}

.section-head h3 {
  margin: 0;
}

.empty-text {
  color: hsl(var(--muted-foreground));
}

.carousel-item + .carousel-item {
  margin-top: 12px;
}

.carousel-item__content {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
}

.carousel-item__index {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  color: hsl(var(--primary-foreground));
  background: hsl(var(--primary));
  border-radius: 50%;
}

.carousel-item__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.section-toggle,
.home-entry__head {
  display: flex;
  gap: 16px;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.field-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 16px;
}

.field-grid__wide {
  grid-column: 1 / -1;
}

.fixed-destination {
  margin: 0 0 16px;
  color: hsl(var(--muted-foreground));
  overflow-wrap: anywhere;
}

.fixed-entry-key {
  margin: 0 0 8px;
  color: hsl(var(--foreground));
}

.home-entry + .home-entry {
  margin-top: 12px;
}

.field-grid--entry {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.home-entry__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

@media (max-width: 900px) {
  .field-grid,
  .field-grid--entry {
    grid-template-columns: 1fr;
  }

  .field-grid__wide {
    grid-column: auto;
  }
}
</style>
