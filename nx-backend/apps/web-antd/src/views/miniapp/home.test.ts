import { beforeEach, describe, expect, it, vi } from 'vitest';

import ts from 'typescript';
import { defineComponent, h } from 'vue';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

vi.mock('ant-design-vue', async () => {
  const stubs = await import('#/test-utils/antd-stubs');
  return {
    ...stubs,
    Collapse: Object.assign(
      defineComponent({
        setup(_, { slots }) {
          return () => h('div', { class: 'collapse-stub' }, slots.default?.());
        },
      }),
      {
        Panel: defineComponent({
          props: { header: String },
          setup(props, { slots }) {
            return () =>
              h('section', { class: 'collapse-panel-stub' }, [
                h('h2', props.header),
                slots.default?.(),
              ]);
          },
        }),
      },
    ),
    Input: defineComponent({
      inheritAttrs: false,
      props: { value: { default: '', type: String } },
      emits: ['update:value'],
      setup(props, { attrs, emit }) {
        return () =>
          h('input', {
            ...attrs,
            value: props.value,
            onInput: (event: Event) =>
              emit('update:value', (event.target as HTMLInputElement).value),
          });
      },
    }),
    InputNumber: defineComponent({
      props: { value: { default: 0, type: Number } },
      emits: ['update:value'],
      setup(props, { attrs, emit }) {
        return () =>
          h('input', {
            ...attrs,
            value: props.value,
            onInput: (event: Event) =>
              emit(
                'update:value',
                Number((event.target as HTMLInputElement).value),
              ),
          });
      },
    }),
    Switch: defineComponent({
      props: { checked: { default: false, type: Boolean } },
      emits: ['update:checked'],
      setup(props, { attrs, emit }) {
        return () =>
          h(
            'button',
            { ...attrs, onClick: () => emit('update:checked', !props.checked) },
            [props.checked ? '启用' : '停用'],
          );
      },
    }),
    Select: defineComponent({
      inheritAttrs: false,
      props: {
        options: { default: () => [], type: Array },
        value: { default: '', type: String },
      },
      emits: ['update:value'],
      setup(props, { attrs, emit }) {
        return () =>
          h(
            'select',
            {
              ...attrs,
              value: props.value,
              onChange: (event: Event) =>
                emit('update:value', (event.target as HTMLSelectElement).value),
            },
            (props.options as Array<{ label: string; value: string }>).map(
              (option) => h('option', { value: option.value }, option.label),
            ),
          );
      },
    }),
  };
});

vi.mock('#/views/site-config/components/editor-shell.vue', () => ({
  default: {
    name: 'EditorShell',
    props: ['description', 'loading', 'saving', 'title'],
    template:
      '<section><h1>{{ title }}</h1><p>{{ description }}</p><slot /><button @click="$emit(\'save\')">保存配置</button></section>',
  },
}));

vi.mock('#/views/site-config/components/image-path-input.vue', () => ({
  default: {
    name: 'ImagePathInput',
    props: {
      dir: String,
      storeObjectUrl: Boolean,
      value: String,
    },
    emits: ['update:value'],
    template:
      '<button class="image-input" :data-dir="dir" :data-store-object-url="String(storeObjectUrl)" :data-value="value" @click="$emit(\'update:value\', \'https://bucket.example.com/miniapp-home/new-banner.png\')">上传轮播图</button>',
  },
}));

vi.mock('#/api', () => ({
  getSiteConfigApi: vi.fn(),
  updateSiteConfigApi: vi.fn(),
}));

vi.mock('#/views/site-config/use-site-config-editor', async () => {
  const { onMounted, ref } = await import('vue');
  const { getSiteConfigApi, updateSiteConfigApi } = await import('#/api');

  return {
    useSiteConfigEditor() {
      const config = ref();
      const loading = ref(false);
      const saving = ref(false);

      onMounted(async () => {
        loading.value = true;
        try {
          config.value = await getSiteConfigApi();
        } finally {
          loading.value = false;
        }
      });

      async function saveConfig() {
        if (!config.value) return;
        saving.value = true;
        try {
          config.value = await updateSiteConfigApi(config.value);
        } finally {
          saving.value = false;
        }
      }

      return { config, loading, saveConfig, saving };
    },
  };
});

import { getSiteConfigApi, updateSiteConfigApi } from '#/api';

import * as miniappHomeModule from './home.vue';
import homeSource from './home.vue?raw';
import apiSource from '../../api/core/site-config.ts?raw';

const MiniappHome = miniappHomeModule.default;
const ensureCarousel = (miniappHomeModule as any).ensureCarousel as
  | ((config: Record<string, unknown>) => unknown)
  | undefined;
const ensureMiniappHome = (miniappHomeModule as any).ensureMiniappHome as
  | ((config: Record<string, unknown>) => any)
  | undefined;
const apiAst = ts.createSourceFile(
  'site-config.ts',
  apiSource,
  ts.ScriptTarget.Latest,
  true,
  ts.ScriptKind.TS,
);

function declaration(name: string) {
  return apiAst.statements.find(
    (node) =>
      (ts.isInterfaceDeclaration(node) || ts.isTypeAliasDeclaration(node)) &&
      node.name.text === name,
  );
}

function literalUnion(name: string) {
  const node = declaration(name);
  if (
    !node ||
    !ts.isTypeAliasDeclaration(node) ||
    !ts.isUnionTypeNode(node.type)
  ) {
    return undefined;
  }
  if (
    !node.modifiers?.some(
      (modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword,
    ) ||
    node.type.types.some(
      (type) =>
        !ts.isLiteralTypeNode(type) || !ts.isStringLiteral(type.literal),
    )
  ) {
    return undefined;
  }
  return node.type.types
    .map((type) =>
      (type as ts.LiteralTypeNode).literal.getText(apiAst).slice(1, -1),
    )
    .sort();
}

function interfaceContract(name: string) {
  const node = declaration(name);
  if (!node || !ts.isInterfaceDeclaration(node)) return undefined;
  return {
    exported:
      node.modifiers?.some(
        (modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword,
      ) ?? false,
    extends:
      node.heritageClauses?.flatMap((clause) =>
        clause.types.map((type) => type.expression.getText(apiAst)),
      ) ?? [],
    fields: Object.fromEntries(
      node.members.filter(ts.isPropertySignature).map((member) => [
        member.name.getText(apiAst),
        {
          optional: Boolean(member.questionToken),
          type: member.type?.getText(apiAst),
        },
      ]),
    ),
  };
}

function createConfig(home: Record<string, unknown> = {}) {
  return {
    home,
    navigation: { drawer: [], main: [], tabs: [] },
    site: {
      brandName: '九型芯',
      copyright: '',
      customerServiceQr: '',
      footerTagline: '',
      logo: '/api/upload-assets/logo',
    },
    types: [],
  };
}

describe('miniapp home carousel management', () => {
  beforeEach(() => {
    vi.mocked(getSiteConfigApi).mockReset();
    vi.mocked(updateSiteConfigApi).mockReset();
  });

  it('declares the configurable miniapp home contract', () => {
    expect(literalUnion('MiniappHomeEntryKey')).toEqual([
      'learn',
      'profile',
      'relation',
      'test',
    ]);
    expect(literalUnion('MiniappHomeIconKey')).toEqual([
      'book',
      'compass',
      'growth',
      'heart',
      'relation',
      'spark',
    ]);
    expect(literalUnion('MiniappHomeThemeKey')).toEqual([
      'blue',
      'cyan',
      'orange',
      'pink',
      'purple',
    ]);
    const required = (type: string) => ({ optional: false, type });
    expect(interfaceContract('MiniappHomeSectionBase')).toEqual({
      exported: true,
      extends: [],
      fields: { enabled: required('boolean') },
    });
    expect(interfaceContract('MiniappHomeBrand')).toEqual({
      exported: true,
      extends: ['MiniappHomeSectionBase'],
      fields: { name: required('string'), tagline: required('string') },
    });
    expect(interfaceContract('MiniappHomeHero')).toEqual({
      exported: true,
      extends: ['MiniappHomeSectionBase'],
      fields: {
        buttonText: required('string'),
        description: required('string'),
        kicker: required('string'),
        title: required('string'),
      },
    });
    expect(interfaceContract('MiniappHomeEntry')).toEqual({
      exported: true,
      extends: ['MiniappHomeSectionBase'],
      fields: {
        description: required('string'),
        icon: required('MiniappHomeIconKey'),
        key: required('MiniappHomeEntryKey'),
        theme: required('MiniappHomeThemeKey'),
        title: required('string'),
      },
    });
    expect(interfaceContract('MiniappHomeEntriesSection')).toEqual({
      exported: true,
      extends: ['MiniappHomeSectionBase'],
      fields: {
        description: required('string'),
        items: required('MiniappHomeEntry[]'),
        title: required('string'),
      },
    });
    expect(interfaceContract('MiniappHomeGrowth')).toEqual({
      exported: true,
      extends: ['MiniappHomeSectionBase'],
      fields: {
        description: required('string'),
        eyebrow: required('string'),
        title: required('string'),
      },
    });
    expect(interfaceContract('MiniappHomeConfig')).toEqual({
      exported: true,
      extends: [],
      fields: {
        brand: required('MiniappHomeBrand'),
        entriesSection: required('MiniappHomeEntriesSection'),
        growth: required('MiniappHomeGrowth'),
        hero: required('MiniappHomeHero'),
      },
    });

    const siteConfig = declaration('SiteConfig');
    expect(siteConfig && ts.isInterfaceDeclaration(siteConfig)).toBe(true);
    const homeProperty =
      siteConfig && ts.isInterfaceDeclaration(siteConfig)
        ? siteConfig.members
            .filter(ts.isPropertySignature)
            .find((member) => member.name.getText(apiAst) === 'home')
        : undefined;
    const homeLiteral =
      homeProperty?.type && ts.isIntersectionTypeNode(homeProperty.type)
        ? homeProperty.type.types.find(ts.isTypeLiteralNode)
        : undefined;
    const miniappHome = homeLiteral?.members
      .filter(ts.isPropertySignature)
      .find((member) => member.name.getText(apiAst) === 'miniappHome');
    expect({
      optional: Boolean(miniappHome?.questionToken),
      type: miniappHome?.type?.getText(apiAst),
    }).toEqual({ optional: true, type: 'MiniappHomeConfig' });
  });

  it('normalizes missing and malformed home carousel config without changing other home fields', () => {
    const missingHome: Record<string, any> = {};
    const nullHome: Record<string, any> = { home: null };
    const nonObjectCarousel: Record<string, any> = {
      home: { miniappCarousel: 'not-a-config' },
    };
    const malformedHome: Record<string, any> = {
      home: {
        keep: { value: 'preserved' },
        miniappCarousel: {
          autoplay: 'yes',
          interval: Number.POSITIVE_INFINITY,
          items: null,
        },
      },
    };
    const invalidItems: Record<string, any> = {
      home: {
        miniappCarousel: {
          autoplay: false,
          interval: 1500,
          items: [
            null,
            { enabled: 'yes', image: 42 },
            { enabled: false, image: '/banner.png' },
          ],
        },
      },
    };
    const tooSlow: Record<string, any> = {
      home: { miniappCarousel: { interval: 12_000 } },
    };

    expect(ensureCarousel?.(missingHome)).toEqual({
      autoplay: true,
      interval: 4000,
      items: [],
    });
    expect(ensureCarousel?.(nullHome)).toEqual({
      autoplay: true,
      interval: 4000,
      items: [],
    });
    expect(ensureCarousel?.(nonObjectCarousel)).toEqual({
      autoplay: true,
      interval: 4000,
      items: [],
    });
    expect(ensureCarousel?.(malformedHome)).toEqual({
      autoplay: true,
      interval: 4000,
      items: [],
    });
    expect(malformedHome.home.keep).toEqual({ value: 'preserved' });
    expect(ensureCarousel?.(invalidItems)).toEqual({
      autoplay: false,
      interval: 2000,
      items: [
        { enabled: true, image: '' },
        { enabled: true, image: '' },
        { enabled: false, image: '/banner.png' },
      ],
    });
    expect(ensureCarousel?.(tooSlow)).toMatchObject({ interval: 10_000 });
  });

  it('initializes complete purple home defaults without replacing sibling home data', () => {
    const carousel = { autoplay: false, interval: 3500, items: [] };
    const keep = { value: 'preserved' };
    const config: Record<string, any> = {
      home: { keep, miniappCarousel: carousel },
    };

    expect(ensureMiniappHome).toBeTypeOf('function');
    expect(ensureMiniappHome?.(config)).toEqual({
      brand: {
        enabled: true,
        name: '九型芯之力',
        tagline: '看见动机，找到成长方向',
      },
      hero: {
        buttonText: '开始人格测试',
        description:
          '从核心动机出发，在老师课程中理解自己，也更从容地走进关系与成长。',
        enabled: true,
        kicker: '老师导学 · 课程配套 · 18 题自测',
        title: '读懂自己内在的能量地图',
      },
      entriesSection: {
        description: '从测试、关系、课程到成长档案，选择此刻最需要的一步。',
        enabled: true,
        items: [
          {
            description: '找到你的核心动机',
            enabled: true,
            icon: 'compass',
            key: 'test',
            theme: 'blue',
            title: '人格测试',
          },
          {
            description: '看见彼此的互动模式',
            enabled: true,
            icon: 'relation',
            key: 'relation',
            theme: 'purple',
            title: '关系合盘',
          },
          {
            description: '跟着课件系统学习',
            enabled: true,
            icon: 'book',
            key: 'learn',
            theme: 'orange',
            title: '老师课程',
          },
          {
            description: '记录你的探索轨迹',
            enabled: true,
            icon: 'growth',
            key: 'profile',
            theme: 'pink',
            title: '成长档案',
          },
        ],
        title: '探索你的九型能量',
      },
      growth: {
        description: '跟随老师的课程与课件，让理解沉淀为真实的成长行动。',
        enabled: true,
        eyebrow: '老师陪伴 · 持续成长',
        title: '把测试发现带进课程练习',
      },
    });
    expect(config.home.keep).toBe(keep);
    expect(config.home.miniappCarousel).toBe(carousel);
  });

  it('repairs malformed sections and preserves configured fixed-entry order once', () => {
    const config: Record<string, any> = {
      home: {
        miniappHome: {
          brand: { enabled: false, name: ' 自定义品牌 ', tagline: '' },
          entriesSection: {
            items: [
              {
                description: ' 自定义档案说明 ',
                enabled: false,
                icon: 'heart',
                key: 'profile',
                theme: 'cyan',
                title: ' 自定义档案 ',
              },
              { key: 'unknown', title: '未知入口' },
              { key: 'profile', title: '重复入口' },
              { icon: 'bad', key: 'test', theme: 'bad' },
            ],
            title: ' ',
          },
          growth: [],
          hero: null,
        },
      },
    };

    const home = ensureMiniappHome?.(config);
    expect(home.brand).toEqual({
      enabled: false,
      name: '自定义品牌',
      tagline: '看见动机，找到成长方向',
    });
    expect(home.hero.title).toBe('读懂自己内在的能量地图');
    expect(home.growth.title).toBe('把测试发现带进课程练习');
    expect(home.entriesSection.title).toBe('探索你的九型能量');
    expect(home.entriesSection.items.map((item: any) => item.key)).toEqual([
      'profile',
      'test',
      'relation',
      'learn',
    ]);
    expect(home.entriesSection.items[0]).toMatchObject({
      description: '自定义档案说明',
      enabled: false,
      icon: 'heart',
      theme: 'cyan',
      title: '自定义档案',
    });
    expect(home.entriesSection.items[1]).toMatchObject({
      icon: 'compass',
      theme: 'blue',
    });

    const allDisabled = ensureMiniappHome?.({
      home: {
        miniappHome: {
          entriesSection: {
            enabled: true,
            items: ['test', 'relation', 'learn', 'profile'].map((key) => ({
              enabled: false,
              key,
            })),
          },
        },
      },
    });
    expect(allDisabled.entriesSection.enabled).toBe(false);
  });

  it('uses a stable item identity instead of index as the carousel row key', () => {
    expect(homeSource).not.toContain(':key="index"');
    expect(homeSource).toContain(':key="itemKey(item)"');
  });

  it('initializes and saves the carousel through image upload and item controls', async () => {
    const config = createConfig({
      existingHomeSetting: { keep: true },
      untouched: { value: 'preserved' },
    });
    vi.mocked(getSiteConfigApi).mockResolvedValue(config as any);
    vi.mocked(updateSiteConfigApi).mockImplementation(async (value) => value);

    const wrapper = mountVueComponent(MiniappHome);
    await flushVuePromises();

    expect(wrapper.text()).toContain('配置小程序首页轮播');
    expect(config.home).toMatchObject({
      existingHomeSetting: { keep: true },
      miniappCarousel: { autoplay: true, interval: 4000, items: [] },
      miniappHome: {
        brand: { enabled: true, name: '九型芯之力' },
        entriesSection: {
          items: [
            { key: 'test' },
            { key: 'relation' },
            { key: 'learn' },
            { key: 'profile' },
          ],
        },
        growth: { enabled: true },
        hero: { enabled: true },
      },
    });
    expect(
      [...document.body.querySelectorAll('.collapse-panel-stub > h2')].map(
        (heading) => heading.textContent,
      ),
    ).toEqual(['轮播图', '顶部品牌', '主视觉', '功能入口', '成长内容']);
    const switchLabels = [
      ...document.body.querySelectorAll('button[aria-label]'),
    ].map((button) => button.getAttribute('aria-label'));
    expect(switchLabels).toEqual(
      expect.arrayContaining([
        '自动轮播',
        '顶部品牌显示状态',
        '主视觉显示状态',
        '功能入口区显示状态',
        '人格测试入口显示状态',
        '关系合盘入口显示状态',
        '老师课程入口显示状态',
        '成长档案入口显示状态',
        '成长内容显示状态',
      ]),
    );
    expect(
      document.body.querySelector('[data-testid="brand-destination"]')
        ?.textContent,
    ).toContain('/pages/profile/profile（底部标签）');
    for (const [key, destination] of Object.entries({
      test: '/pages/test/test（页面跳转）',
      relation: '/pages/relation/relation（页面跳转）',
      learn: '/pages/learn/learn（底部标签）',
      profile: '/pages/profile/profile（底部标签）',
    })) {
      expect(
        document.body.querySelector(`[data-testid="entry-key-${key}"]`)
          ?.textContent,
      ).toBe(key);
      expect(
        document.body.querySelector(`[data-testid="entry-destination-${key}"]`)
          ?.textContent,
      ).toContain(destination);
    }

    const editInput = (testId: string, value: string) => {
      const input = document.body.querySelector(
        `[data-testid="${testId}"]`,
      ) as HTMLInputElement;
      input.value = value;
      input.dispatchEvent(new Event('input', { bubbles: true }));
    };
    editInput('brand-name', '新的品牌名称');
    editInput('hero-title', '新的主视觉标题');
    editInput('entries-title', '新的入口标题');
    editInput('growth-title', '新的成长标题');
    for (const testId of [
      'brand-enabled',
      'hero-enabled',
      'entries-enabled',
      'growth-enabled',
    ]) {
      document.body
        .querySelector(`[data-testid="${testId}"]`)
        ?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    }
    const relationIcon = document.body.querySelector(
      '[data-testid="entry-icon-relation"]',
    ) as HTMLSelectElement;
    relationIcon.value = 'heart';
    relationIcon.dispatchEvent(new Event('change', { bubbles: true }));
    const relationTheme = document.body.querySelector(
      '[data-testid="entry-theme-relation"]',
    ) as HTMLSelectElement;
    relationTheme.value = 'cyan';
    relationTheme.dispatchEvent(new Event('change', { bubbles: true }));
    expect([...relationIcon.options].map((option) => option.value)).toEqual([
      'compass',
      'relation',
      'book',
      'growth',
      'spark',
      'heart',
    ]);
    expect([...relationTheme.options].map((option) => option.value)).toEqual([
      'blue',
      'purple',
      'orange',
      'pink',
      'cyan',
    ]);
    document.body
      .querySelector('[data-testid="entry-enabled-test"]')
      ?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    document.body
      .querySelector('[data-testid="entry-move-down-test"]')
      ?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    await flushVuePromises();

    wrapper.button('新增轮播图')?.click();
    wrapper.button('新增轮播图')?.click();
    wrapper.button('新增轮播图')?.click();
    await flushVuePromises();
    const uploadedItem = (config.home.miniappCarousel as any).items[0];
    expect(document.body.querySelectorAll('.image-input')).toHaveLength(3);
    expect(
      document.body.querySelector('.image-input')?.getAttribute('data-dir'),
    ).toBe('miniapp-home');
    expect(
      document.body
        .querySelector('.image-input')
        ?.getAttribute('data-store-object-url'),
    ).toBe('true');
    expect(
      document.body
        .querySelector('[data-testid="carousel-interval"]')
        ?.getAttribute('min'),
    ).toBe('2000');
    expect(
      document.body
        .querySelector('[data-testid="carousel-interval"]')
        ?.getAttribute('max'),
    ).toBe('10000');

    document.body
      .querySelector('.image-input')
      ?.dispatchEvent(new MouseEvent('click'));
    document.body
      .querySelector('[data-testid="carousel-enabled-0"]')
      ?.dispatchEvent(new MouseEvent('click'));
    wrapper.button('下移')?.click();
    await flushVuePromises();

    expect((config.home.miniappCarousel as any).items).toEqual([
      { enabled: true, image: '' },
      {
        enabled: false,
        image: 'https://bucket.example.com/miniapp-home/new-banner.png',
      },
      { enabled: true, image: '' },
    ]);
    expect((config.home.miniappCarousel as any).items[1]).toBe(uploadedItem);
    expect(
      [...document.body.querySelectorAll('.image-input')].map((input) =>
        input.getAttribute('data-value'),
      ),
    ).toEqual([
      '',
      'https://bucket.example.com/miniapp-home/new-banner.png',
      '',
    ]);

    [...document.body.querySelectorAll('button')]
      .filter((button) => button.textContent?.trim() === '上移')[1]
      ?.click();
    await flushVuePromises();

    expect((config.home.miniappCarousel as any).items).toEqual([
      {
        enabled: false,
        image: 'https://bucket.example.com/miniapp-home/new-banner.png',
      },
      { enabled: true, image: '' },
      { enabled: true, image: '' },
    ]);
    expect((config.home.miniappCarousel as any).items[0]).toBe(uploadedItem);
    expect(
      [...document.body.querySelectorAll('.image-input')].map((input) =>
        input.getAttribute('data-value'),
      ),
    ).toEqual([
      'https://bucket.example.com/miniapp-home/new-banner.png',
      '',
      '',
    ]);

    wrapper.button('删除')?.click();
    wrapper.button('保存配置')?.click();
    await flushVuePromises();

    expect((config.home.miniappCarousel as any).items).toEqual([
      { enabled: true, image: '' },
      { enabled: true, image: '' },
    ]);
    expect(config.home.miniappHome).toMatchObject({
      brand: { enabled: false, name: '新的品牌名称' },
      hero: { enabled: false, title: '新的主视觉标题' },
      entriesSection: {
        enabled: false,
        title: '新的入口标题',
        items: [
          { icon: 'heart', key: 'relation', theme: 'cyan' },
          { enabled: false, key: 'test' },
          { key: 'learn' },
          { key: 'profile' },
        ],
      },
      growth: { enabled: false, title: '新的成长标题' },
    });
    expect(config.home.untouched).toEqual({ value: 'preserved' });
    expect(updateSiteConfigApi).toHaveBeenCalledTimes(1);
    expect(updateSiteConfigApi).toHaveBeenCalledWith(config);
    wrapper.unmount();
  });
});
