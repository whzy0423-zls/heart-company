/** @vitest-environment happy-dom */
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { defineComponent, h } from 'vue';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

vi.mock('ant-design-vue', async () => {
  const stubs = await import('#/test-utils/antd-stubs');
  const inputComponent = (tag: 'input' | 'textarea') =>
    defineComponent({
      inheritAttrs: false,
      props: { value: { default: '', type: String } },
      emits: ['update:value'],
      setup(props, { attrs, emit }) {
        return () =>
          h(tag, {
            ...attrs,
            value: props.value,
            onInput: (event: Event) =>
              emit('update:value', (event.target as HTMLInputElement).value),
          });
      },
    });
  return {
    ...stubs,
    Button: stubs.Button,
    Card: stubs.Card,
    Input: inputComponent('input'),
    Textarea: inputComponent('textarea'),
  };
});

vi.mock('./components/editor-shell.vue', () => ({
  default: {
    props: ['description', 'loading', 'saving', 'title'],
    template:
      '<section><h1>{{ title }}</h1><p>{{ description }}</p><slot /><button data-testid="save" @click="$emit(\'save\')">保存配置</button></section>',
  },
}));

vi.mock('#/api', () => ({
  getSiteConfigApi: vi.fn(),
  updateSiteConfigApi: vi.fn(),
}));

vi.mock('./use-site-config-editor', async () => {
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
import * as enterpriseModule from './enterprise.vue';

const Enterprise = enterpriseModule.default;
const ensureEnterpriseBookingFields = (enterpriseModule as any)
  .ensureEnterpriseBookingFields as
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

function edit(testId: string, value: string) {
  const element = document.body.querySelector(
    `[data-testid="${testId}"]`,
  ) as HTMLInputElement;
  element.value = value;
  element.dispatchEvent(new Event('input', { bubbles: true }));
}

describe('enterprise booking configuration', () => {
  beforeEach(() => {
    vi.mocked(getSiteConfigApi).mockReset();
    vi.mocked(updateSiteConfigApi).mockReset();
  });

  it('fills legacy enterprise booking defaults while preserving existing fields and unknown home data', () => {
    const config: Record<string, any> = createConfig({
      keep: { version: 1 },
      enterprise: {
        title: '团队共创',
        vendorExtension: { source: 'crm' },
        items: [{ title: '  定制内训 ', description: ' 面向关键团队 ' }],
        processSteps: [{ title: '  诊断  ', description: ' 了解现状 ' }],
      },
    });
    expect(ensureEnterpriseBookingFields).toBeTypeOf('function');
    expect(ensureEnterpriseBookingFields?.(config)).toMatchObject({
      title: '团队共创',
      items: [{ title: '定制内训', description: '面向关键团队' }],
      processSteps: [{ title: '诊断', description: '了解现状' }],
    });
    expect(config.home.keep).toEqual({ version: 1 });
    expect(config.home.enterprise.vendorExtension).toEqual({ source: 'crm' });

    const legacy: Record<string, any> = createConfig({
      enterprise: { modules: [] },
    });
    expect(
      ensureEnterpriseBookingFields?.(legacy).items.map(
        (item: any) => item.title,
      ),
    ).toEqual(['企业内训', '团队工作坊', '管理者培训']);
    expect(
      legacy.home.enterprise.processSteps.map((item: any) => item.title),
    ).toEqual(['需求沟通', '方案共创', '落地交付']);
  });

  it('binds service and process repeaters, supports ordering and persists a JSON round-trip', async () => {
    const config: Record<string, any> = createConfig({
      enterprise: { modules: ['工作坊'] },
      untouched: { x: true },
    });
    vi.mocked(getSiteConfigApi).mockResolvedValue(config as any);
    vi.mocked(updateSiteConfigApi).mockImplementation(async (value) => value);
    const wrapper = mountVueComponent(Enterprise);
    await flushVuePromises();

    expect(wrapper.text()).toContain('预约服务方式');
    expect(wrapper.text()).toContain('合作流程');
    document.body
      .querySelector('[data-testid="add-service"]')
      ?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    document.body
      .querySelector('[data-testid="add-process-step"]')
      ?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    await flushVuePromises();
    edit('service-title-3', '领导力共学');
    edit('service-description-3', '支持管理升级');
    edit('process-title-3', '复盘跟进');
    edit('process-description-3', '形成下一步');
    document.body
      .querySelector('[data-testid="service-move-up-3"]')
      ?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    document.body
      .querySelector('[data-testid="process-move-up-3"]')
      ?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    await flushVuePromises();
    document.body
      .querySelector('[data-testid="save"]')
      ?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    await flushVuePromises();

    expect(config.home.enterprise.items.map((item: any) => item.title)).toEqual(
      ['企业内训', '团队工作坊', '领导力共学', '管理者培训'],
    );
    expect(
      config.home.enterprise.processSteps.map((item: any) => item.title),
    ).toEqual(['需求沟通', '方案共创', '复盘跟进', '落地交付']);
    expect(config.home.untouched).toEqual({ x: true });
    expect(updateSiteConfigApi).toHaveBeenCalledWith(config);
    wrapper.unmount();
  });

  it('offers a clear way to restore defaults after all repeater rows are removed', async () => {
    const config: Record<string, any> = createConfig({ enterprise: {} });
    vi.mocked(getSiteConfigApi).mockResolvedValue(config as any);
    const wrapper = mountVueComponent(Enterprise);
    await flushVuePromises();
    for (let index = 2; index >= 0; index -= 1) {
      document.body
        .querySelector(`[data-testid="service-remove-${index}"]`)
        ?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
      document.body
        .querySelector(`[data-testid="process-remove-${index}"]`)
        ?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    }
    await flushVuePromises();
    expect(wrapper.text()).toContain('暂无服务方式，可恢复默认配置');
    expect(wrapper.text()).toContain('暂无合作流程，可恢复默认配置');
    document.body
      .querySelector('[data-testid="restore-service-defaults"]')
      ?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    document.body
      .querySelector('[data-testid="restore-process-defaults"]')
      ?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    await flushVuePromises();
    expect(config.home.enterprise.items).toHaveLength(3);
    expect(config.home.enterprise.processSteps).toHaveLength(3);
    wrapper.unmount();
  });
});
