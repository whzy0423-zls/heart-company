import { beforeEach, describe, expect, it, vi } from 'vitest';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

vi.mock('ant-design-vue', async () => import('#/test-utils/antd-stubs'));

vi.mock('@vben/preferences', () => ({
  updatePreferences: vi.fn(),
}));

vi.mock('#/branding', () => ({
  BRANDING_CACHE_KEY: 'nx_admin_branding_cache_test',
}));

vi.mock('../site-config/components/editor-shell.vue', () => ({
  default: {
    name: 'EditorShell',
    template: '<section><slot /><button @click="$emit(\'save\')">保存</button></section>',
  },
}));

vi.mock('../site-config/components/image-path-input.vue', () => ({
  default: {
    name: 'ImagePathInput',
    template: '<div>ImagePathInput</div>',
  },
}));

vi.mock('#/api', () => ({
  getAdminBrandingApi: vi.fn(),
  updateAdminBrandingApi: vi.fn(),
}));

import { getAdminBrandingApi, updateAdminBrandingApi } from '#/api';

const storage = new Map<string, string>();
vi.stubGlobal('localStorage', {
  clear: () => storage.clear(),
  getItem: (key: string) => storage.get(key) ?? null,
  removeItem: (key: string) => storage.delete(key),
  setItem: (key: string, value: string) => storage.set(key, value),
});

import Branding from './branding.vue';

describe('branding management page P2 behavior', () => {
  beforeEach(() => {
    vi.mocked(getAdminBrandingApi).mockReset();
    vi.mocked(updateAdminBrandingApi).mockReset();
    localStorage.clear();
  });

  it('shows an inline load error and can retry without treating the empty form as loaded data', async () => {
    vi.mocked(getAdminBrandingApi)
      .mockRejectedValueOnce(new Error('branding api down'))
      .mockResolvedValueOnce({
        loadingText: '加载九型芯之力',
        logo: '/api/upload-assets/9',
        name: '九型芯后台',
      });

    const wrapper = mountVueComponent(Branding);
    await flushVuePromises();

    expect(wrapper.text()).toContain('后台品牌配置加载失败，请稍后重试');
    expect(wrapper.text()).toContain('重试');
    expect(wrapper.text()).not.toContain('九型芯后台');

    wrapper.button('重试')?.click();
    await flushVuePromises();

    const inputValues = [...document.body.querySelectorAll('input')].map(
      (item) => item.value,
    );
    expect(inputValues).toContain('九型芯后台');
    expect(wrapper.text()).not.toContain('后台品牌配置加载失败');
    wrapper.unmount();
  });
});
