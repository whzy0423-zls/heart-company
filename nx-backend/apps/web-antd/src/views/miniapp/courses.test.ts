/** @vitest-environment happy-dom */
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { defineComponent, h } from 'vue';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

vi.mock('ant-design-vue', async () => {
  const stubs = await import('#/test-utils/antd-stubs');
  return {
    ...stubs,
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
    Textarea: defineComponent({
      inheritAttrs: false,
      props: { value: { default: '', type: String } },
      emits: ['update:value'],
      setup(props, { attrs, emit }) {
        return () =>
          h('textarea', {
            ...attrs,
            value: props.value,
            onInput: (event: Event) =>
              emit('update:value', (event.target as HTMLTextAreaElement).value),
          });
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
    Switch: defineComponent({
      props: { checked: { default: false, type: Boolean } },
      emits: ['update:checked'],
      setup(props, { attrs, emit }) {
        return () =>
          h(
            'button',
            { ...attrs, onClick: () => emit('update:checked', !props.checked) },
            props.checked ? '上架' : '下架',
          );
      },
    }),
  };
});

vi.mock('#/views/site-config/components/editor-shell.vue', () => ({
  default: {
    props: ['description', 'loading', 'saving', 'title'],
    template:
      '<section><h1>{{ title }}</h1><p>{{ description }}</p><slot /><button @click="$emit(\'save\')">保存配置</button></section>',
  },
}));

vi.mock('#/views/site-config/components/image-path-input.vue', () => ({
  default: {
    props: ['value'],
    template: '<span class="image-input">封面上传</span>',
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
        config.value = await getSiteConfigApi();
      });
      async function saveConfig() {
        if (!config.value) return;
        saving.value = true;
        config.value = await updateSiteConfigApi(config.value);
        saving.value = false;
      }
      return {
        config,
        linesToArray: (value: string) =>
          value
            .split('\n')
            .map((item) => item.trim())
            .filter(Boolean),
        loading,
        saveConfig,
        saving,
      };
    },
  };
});

import { getSiteConfigApi, updateSiteConfigApi } from '#/api';

import * as coursesModule from './courses.vue';

const MiniappCourses = coursesModule.default;
const normalizeMiniappCourses = coursesModule.normalizeMiniappCourses as (
  config: Record<string, any>,
) => { items: Array<Record<string, any>> };
const yuanToCents = coursesModule.yuanToCents as (
  value: string | number,
) => number | null;
const validateCourse = coursesModule.validateCourse as (
  course: Record<string, any>,
) => string[];

function createConfig(home: Record<string, unknown> = {}) {
  return {
    home,
    navigation: { drawer: [], main: [], tabs: [] },
    site: {
      brandName: '九型芯',
      copyright: '',
      customerServiceQr: '',
      footerTagline: '',
      logo: '/logo.png',
    },
    types: [],
  };
}

describe('miniapp course management', () => {
  beforeEach(() => {
    vi.mocked(getSiteConfigApi).mockReset();
    vi.mocked(updateSiteConfigApi).mockReset();
  });

  it('migrates legacy course cards and keeps unrelated home fields', () => {
    const config = createConfig({
      preserve: { source: 'cms' },
      courses: {
        items: [
          {
            badge: '关系',
            bullets: ['看见自己'],
            description: '从关系开始练习觉察',
            title: '关系里的自己',
          },
        ],
      },
    });

    const courses = normalizeMiniappCourses(config);
    expect(courses.items).toHaveLength(1);
    expect(courses.items[0]).toMatchObject({
      badge: '关系',
      bullets: ['看见自己'],
      enabled: true,
      paymentMode: 'consult',
      priceCents: 0,
      title: '关系里的自己',
    });
    expect(courses.items[0]?.id).toMatch(/^course-/);
    expect(config.home.preserve).toEqual({ source: 'cms' });
    expect(config.home.miniappCourses).toBe(courses);
  });

  it('converts yuan input to exact cents and rejects more than two decimals', () => {
    expect(yuanToCents('1')).toBe(100);
    expect(yuanToCents('12.30')).toBe(1230);
    expect(yuanToCents('0.01')).toBe(1);
    expect(yuanToCents('12.345')).toBeNull();
    expect(yuanToCents('-1')).toBeNull();
    expect(yuanToCents('1000000')).toBeNull();
    expect(yuanToCents('')).toBe(0);
    expect(yuanToCents('0')).toBe(0);
  });

  it('derives enrollment behavior from price even when saved payment mode disagrees', () => {
    const config = createConfig({
      miniappCourses: {
        items: [
          { id: 'zero', title: '咨询课', priceCents: 0, paymentMode: 'paid' },
          { id: 'positive', title: '付费课', priceCents: 1999, paymentMode: 'consult' },
          { id: 'blank', title: '未定价课', paymentMode: 'paid' },
        ],
      },
    });
    expect(normalizeMiniappCourses(config).items.map(({ paymentMode }) => paymentMode))
      .toEqual(['consult', 'paid', 'consult']);
    expect(
      validateCourse({
        id: 'course-growth',
        title: '共学课',
        paymentMode: 'paid',
        priceCents: 0,
      }),
    ).toEqual([]);
    expect(
      validateCourse({
        id: 'course-growth',
        title: '共学课',
        paymentMode: 'paid',
        priceCents: 1999,
      }),
    ).toEqual([]);
    expect(
      validateCourse({
        id: 'course-growth',
        title: '共学课',
        paymentMode: 'consult',
        priceCents: 0,
      }),
    ).toEqual([]);
  });

  it('saves positive prices as WeChat payment and clearing a price restores consultation', async () => {
    vi.mocked(getSiteConfigApi).mockResolvedValue(
      createConfig({
        preserve: { source: 'cms' },
        miniappCourses: {
          catalogExtension: 'retained',
          items: [{ id: 'course-growth', title: '共学课', priceCents: 0, paymentMode: 'consult', vendorExtension: 'retained' }],
        },
      }) as any,
    );
    vi.mocked(updateSiteConfigApi).mockImplementation(async (value) => value as any);
    const wrapper = mountVueComponent(MiniappCourses);
    await flushVuePromises();
    expect(document.querySelector('select')).toBeNull();
    const input = document.querySelector<HTMLInputElement>('input[aria-label="课程价格（元）"]');
    expect(input).not.toBeNull();
    const setPrice = async (value: string) => {
      input!.value = value;
      input!.dispatchEvent(new Event('input', { bubbles: true }));
      await flushVuePromises();
    };
    const save = async () => {
      wrapper.button('保存配置')?.click();
      await flushVuePromises();
    };
    await setPrice('19.99');
    await save();
    expect(updateSiteConfigApi).toHaveBeenLastCalledWith(expect.objectContaining({
      home: expect.objectContaining({
        preserve: { source: 'cms' },
        miniappCourses: expect.objectContaining({
          catalogExtension: 'retained',
          items: [expect.objectContaining({ priceCents: 1999, paymentMode: 'paid', vendorExtension: 'retained' })],
        }),
      }),
    }));
    await setPrice('');
    await save();
    const saved = vi.mocked(updateSiteConfigApi).mock.lastCall?.[0] as any;
    expect(saved.home.miniappCourses.items[0]).toMatchObject({ priceCents: 0, paymentMode: 'consult' });
    expect(wrapper.text()).toContain('咨询老师');

    for (const value of ['-1', '1.234', '1000000', 'abc']) {
      await setPrice(value);
      await save();
    }
    expect(updateSiteConfigApi).toHaveBeenCalledTimes(2);
    wrapper.unmount();
  });

  it('shows course-specific copy and saves the full site config', async () => {
    vi.mocked(getSiteConfigApi).mockResolvedValue(
      createConfig({
        preserve: { source: 'cms' },
        miniappCourses: {
          items: [
            {
              id: 'course-growth',
              title: '个人成长基础课',
              paymentMode: 'consult',
              priceCents: 0,
              enabled: true,
            },
          ],
        },
      }) as any,
    );
    vi.mocked(updateSiteConfigApi).mockImplementation(
      async (value) => value as any,
    );

    const wrapper = mountVueComponent(MiniappCourses);
    await flushVuePromises();
    expect(wrapper.text()).toContain('小程序课程配置');
    expect(wrapper.text()).toContain('微信支付金额');
    wrapper.button('保存配置')?.click();
    await flushVuePromises();
    expect(updateSiteConfigApi).toHaveBeenCalledWith(
      expect.objectContaining({
        home: expect.objectContaining({ preserve: { source: 'cms' } }),
      }),
    );
    wrapper.unmount();
  });
});
