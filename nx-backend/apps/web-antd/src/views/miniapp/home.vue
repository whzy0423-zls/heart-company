<script lang="ts">
import type {
  MiniappCarouselConfig,
  MiniappHomeConfig,
  MiniappHomeEntry,
  MiniappHomeEntryKey,
  MiniappHomeIconKey,
  MiniappHomeThemeKey,
} from '#/api';

export const MINIAPP_HOME_ICON_KEYS: readonly MiniappHomeIconKey[] = Object.freeze([
  'compass',
  'relation',
  'book',
  'growth',
  'spark',
  'heart',
]);

export const MINIAPP_HOME_THEME_KEYS: readonly MiniappHomeThemeKey[] = Object.freeze([
  'blue',
  'purple',
  'orange',
  'pink',
  'cyan',
]);

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
    theme: 'orange',
    title: '老师课程',
  },
  profile: {
    description: '记录你的探索轨迹',
    enabled: true,
    icon: 'growth',
    key: 'profile',
    theme: 'pink',
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
      if (!isRecord(rawEntry) || !ENTRY_ORDER.includes(rawEntry.key as MiniappHomeEntryKey)) {
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
      kicker: normalizedText(
        rawHero.kicker,
        '老师导学 · 课程配套 · 18 题自测',
      ),
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

import { Button, Card, Form, InputNumber, Switch } from 'ant-design-vue';

import EditorShell from '#/views/site-config/components/editor-shell.vue';
import ImagePathInput from '#/views/site-config/components/image-path-input.vue';
import { useSiteConfigEditor } from '#/views/site-config/use-site-config-editor';

const { config, loading, saveConfig, saving } = useSiteConfigEditor();

const carousel = computed<MiniappCarouselConfig | undefined>(
  () => config.value?.home.miniappCarousel,
);
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
    description="配置小程序首页顶部轮播图，图片顺序即为展示顺序。"
    :loading="loading"
    :saving="saving"
    title="首页管理"
    @save="saveConfig"
  >
    <Form v-if="carousel" layout="vertical">
      <div class="carousel-settings">
        <Form.Item label="自动轮播">
          <Switch v-model:checked="carousel.autoplay" />
        </Form.Item>
        <Form.Item label="轮播间隔（毫秒）">
          <InputNumber
            v-model:value="carousel.interval"
            data-testid="carousel-interval"
            :max="10_000"
            :min="2000"
            :step="500"
          />
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
    </Form>
  </EditorShell>
</template>

<style scoped>
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
</style>
