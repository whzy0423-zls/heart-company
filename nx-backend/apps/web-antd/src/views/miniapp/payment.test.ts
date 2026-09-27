/** @vitest-environment happy-dom */
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { defineComponent, h } from 'vue';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

vi.mock('ant-design-vue', async () => {
  const stubs = await import('#/test-utils/antd-stubs');
  const Switch = defineComponent({
    inheritAttrs: false,
    props: { checked: { default: false, type: Boolean } },
    emits: ['update:checked'],
    setup(props, { attrs, emit }) {
      return () =>
        h('button', {
          ...attrs,
          role: 'switch',
          'aria-checked': String(props.checked),
          onClick: () => emit('update:checked', !props.checked),
        });
    },
  });
  return { ...stubs, Switch };
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

      return { config, loading, saveConfig, saving };
    },
  };
});

import { getSiteConfigApi, updateSiteConfigApi } from '#/api';

import * as paymentModule from './payment.vue';

const MiniappPayment = paymentModule.default;
const normalizeMiniappPayment = paymentModule.normalizeMiniappPayment as (
  config: Record<string, any>,
) => { enabled: boolean };

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

describe('miniapp payment management', () => {
  beforeEach(() => {
    vi.mocked(getSiteConfigApi).mockReset();
    vi.mocked(updateSiteConfigApi).mockReset();
  });

  it('defaults missing or malformed payment settings to enabled', () => {
    const config = createConfig({ preserve: { source: 'cms' } });
    expect(normalizeMiniappPayment(config)).toEqual({ enabled: true });
    expect(config.home.preserve).toEqual({ source: 'cms' });

    const disabled = createConfig({ miniappPayment: { enabled: false } });
    expect(normalizeMiniappPayment(disabled)).toEqual({ enabled: false });
    expect(normalizeMiniappPayment(createConfig({ miniappPayment: [] }))).toEqual({
      enabled: true,
    });
  });

  it('allows an administrator to take payment offline and save the setting', async () => {
    const config = createConfig({ miniappPayment: { enabled: true } });
    vi.mocked(getSiteConfigApi).mockResolvedValue(config as any);
    vi.mocked(updateSiteConfigApi).mockImplementation(async (value) => value);

    const wrapper = mountVueComponent(MiniappPayment);
    await flushVuePromises();

    expect(wrapper.text()).toContain('小程序支付');
    expect(wrapper.text()).toContain('下架后隐藏所有新支付入口');
    const toggle = document.body.querySelector(
      '[data-testid="miniapp-payment-enabled"]',
    ) as HTMLButtonElement;
    expect(toggle).not.toBeNull();
    expect(toggle.getAttribute('aria-checked')).toBe('true');

    toggle.click();
    await flushVuePromises();
    expect(toggle.getAttribute('aria-checked')).toBe('false');
    [...document.body.querySelectorAll('button')]
      .find((button) => button.textContent === '保存配置')
      ?.click();
    await flushVuePromises();

    expect(config.home.miniappPayment).toEqual({ enabled: false });
    expect(updateSiteConfigApi).toHaveBeenCalledWith(config);
    wrapper.unmount();
  });
});
