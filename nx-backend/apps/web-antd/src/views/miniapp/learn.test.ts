/** @vitest-environment happy-dom */
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { defineComponent, h } from 'vue';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

vi.mock('ant-design-vue', async () => {
  const stubs = await import('#/test-utils/antd-stubs');
  const Input = defineComponent({
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
  });
  const Textarea = defineComponent({
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
  });
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
    Input,
    Textarea,
  };
});

vi.mock('#/views/site-config/components/editor-shell.vue', () => ({
  default: {
    props: ['description', 'loading', 'saving', 'title'],
    template:
      '<section><h1>{{ title }}</h1><p>{{ description }}</p><slot /><button @click="$emit(\'save\')">保存配置</button></section>',
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
import * as learnModule from './learn.vue';

const MiniappLearn = learnModule.default;
const ensureMiniappLearn = (learnModule as any).ensureMiniappLearn as
  | ((config: Record<string, any>) => any)
  | undefined;

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

function input(testId: string, value: string) {
  const element = document.body.querySelector(`[data-testid="${testId}"]`) as
    | HTMLInputElement
    | HTMLTextAreaElement;
  element.value = value;
  element.dispatchEvent(new Event('input', { bubbles: true }));
}

describe('miniapp learn page management', () => {
  beforeEach(() => {
    vi.mocked(getSiteConfigApi).mockReset();
    vi.mocked(updateSiteConfigApi).mockReset();
  });

  it('fills every learn-page default for old or malformed config without dropping unknown fields', () => {
    const config: Record<string, any> = createConfig({
      keep: { enabled: true },
      miniappLearn: {
        vendorExtension: { source: 'cms' },
        hero: {
          title: '  自定义学习标题  ',
          meta: [' 视频 ', '', '音频', '九型', '超出'],
        },
        classroom: [],
        sections: { courses: { emptyTitle: ' 课程待定 ' }, quotes: null },
      },
    });

    expect(ensureMiniappLearn).toBeTypeOf('function');
    expect(ensureMiniappLearn?.(config)).toMatchObject({
      hero: {
        eyebrow: '老师课堂',
        title: '自定义学习标题',
        lead: '从视频与音频课件开始，理解自己、改善关系，也为团队协作建立更清晰的共同语言。',
        meta: ['视频', '音频', '九型'],
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
          emptyTitle: '课程待定',
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
    });
    expect(config.home.keep).toEqual({ enabled: true });
    expect(config.home.miniappLearn.vendorExtension).toEqual({ source: 'cms' });
  });

  it('renders professional learn copy and saves edited trimmed tags through the existing update API', async () => {
    const config = createConfig({ experimental: { preserve: true } });
    vi.mocked(getSiteConfigApi).mockResolvedValue(config as any);
    vi.mocked(updateSiteConfigApi).mockImplementation(async (value) => value);

    const wrapper = mountVueComponent(MiniappLearn);
    await flushVuePromises();

    expect(wrapper.text()).toContain('学习页管理');
    expect(wrapper.text()).toContain('配置学习页文案');
    expect(wrapper.text()).toContain('视频/音频实际内容请前往“老师课堂”');
    expect(
      [...document.body.querySelectorAll('.collapse-panel-stub > h2')].map(
        (node) => node.textContent,
      ),
    ).toEqual(['页面主视觉', '课堂精选', '区块标题与底部 CTA']);

    input('learn-hero-title', '新的学习页标题');
    input(
      'learn-hero-tags',
      '  视频课程  \n\n 音频精讲 \n 九型实践 \n 超出标签',
    );
    input('learn-classroom-empty-title', '新课准备中');
    input('learn-courses-empty-description', '课程会持续补充');
    input('learn-bottom-cta', '开始探索');
    document.body
      .querySelector('button')
      ?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    await flushVuePromises();

    expect(config.home.miniappLearn).toMatchObject({
      hero: {
        title: '新的学习页标题',
        meta: ['视频课程', '音频精讲', '九型实践'],
      },
      classroom: { emptyTitle: '新课准备中' },
      sections: { courses: { emptyDescription: '课程会持续补充' } },
      bottomCtaText: '开始探索',
    });
    expect(config.home.experimental).toEqual({ preserve: true });
    expect(updateSiteConfigApi).toHaveBeenCalledWith(config);
    wrapper.unmount();
  });
});
