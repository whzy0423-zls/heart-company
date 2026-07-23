import { beforeEach, describe, expect, it, vi } from 'vitest';

import { readFileSync } from 'node:fs';

import { defineComponent, h } from 'vue';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

vi.mock('ant-design-vue', async () => {
  const stubs = await import('#/test-utils/antd-stubs');
  return {
    ...stubs,
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
    props: ['dir', 'value'],
    emits: ['update:value'],
    template:
      '<button class="image-input" :data-dir="dir" :data-value="value" @click="$emit(\'update:value\', \'/api/upload-assets/new-banner\')">上传轮播图</button>',
  },
}));

vi.mock('#/api', () => ({
  getSiteConfigApi: vi.fn(),
  updateSiteConfigApi: vi.fn(),
}));

import { getSiteConfigApi, updateSiteConfigApi } from '#/api';

import * as miniappHomeModule from './home.vue';

const MiniappHome = miniappHomeModule.default;
const ensureCarousel = (miniappHomeModule as any).ensureCarousel as
  | ((config: Record<string, unknown>) => unknown)
  | undefined;
const homeSource = readFileSync(
  'apps/web-antd/src/views/miniapp/home.vue',
  'utf8',
);

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

    expect(wrapper.text()).toContain('小程序首页顶部轮播');
    expect(config.home).toMatchObject({
      existingHomeSetting: { keep: true },
      miniappCarousel: { autoplay: true, interval: 4000, items: [] },
    });

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
      { enabled: false, image: '/api/upload-assets/new-banner' },
      { enabled: true, image: '' },
    ]);
    expect((config.home.miniappCarousel as any).items[1]).toBe(uploadedItem);
    expect(
      [...document.body.querySelectorAll('.image-input')].map((input) =>
        input.getAttribute('data-value'),
      ),
    ).toEqual(['', '/api/upload-assets/new-banner', '']);

    [...document.body.querySelectorAll('button')]
      .filter((button) => button.textContent?.trim() === '上移')[1]
      ?.click();
    await flushVuePromises();

    expect((config.home.miniappCarousel as any).items).toEqual([
      { enabled: false, image: '/api/upload-assets/new-banner' },
      { enabled: true, image: '' },
      { enabled: true, image: '' },
    ]);
    expect((config.home.miniappCarousel as any).items[0]).toBe(uploadedItem);
    expect(
      [...document.body.querySelectorAll('.image-input')].map((input) =>
        input.getAttribute('data-value'),
      ),
    ).toEqual(['/api/upload-assets/new-banner', '', '']);

    wrapper.button('删除')?.click();
    wrapper.button('保存配置')?.click();
    await flushVuePromises();

    expect((config.home.miniappCarousel as any).items).toEqual([
      { enabled: true, image: '' },
      { enabled: true, image: '' },
    ]);
    expect(config.home.untouched).toEqual({ value: 'preserved' });
    expect(updateSiteConfigApi).toHaveBeenCalledWith(config);
    wrapper.unmount();
  });
});
