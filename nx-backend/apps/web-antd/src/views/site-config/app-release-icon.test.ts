/* eslint-disable vue/one-component-per-file */
import { createApp, defineComponent, h, nextTick } from 'vue';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('ant-design-vue', () => ({
  Avatar: defineComponent({
    name: 'AvatarStub',
    inheritAttrs: false,
    props: {
      alt: { default: '', type: String },
      shape: { default: 'circle', type: String },
      size: { default: 40, type: Number },
      src: { default: '', type: String },
    },
    setup(props, { attrs, slots }) {
      return () =>
        h(
          'div',
          {
            ...attrs,
            class: ['avatar-stub', attrs.class],
            'data-shape': props.shape,
            style: { height: `${props.size}px`, width: `${props.size}px` },
          },
          props.src
            ? h('img', { alt: props.alt, src: props.src })
            : slots.default?.(),
        );
    },
  }),
}));

import AppReleaseIcon from './app-release-icon.vue';

const fetchMock = vi.fn<typeof fetch>();
const createObjectURL = vi.fn(() => 'blob:app-release-icon');
const revokeObjectURL = vi.fn();

async function flushPreview() {
  await Promise.resolve();
  await Promise.resolve();
  await nextTick();
}

function mountIcon(iconUrl: string) {
  const root = document.createElement('div');
  document.body.append(root);
  const app = createApp(AppReleaseIcon, {
    accessToken: 'protected-token',
    appName: '九型人格',
    iconUrl,
    packageName: 'com.example.ninexing',
    size: 48,
  });
  app.mount(root);
  return {
    root,
    unmount() {
      app.unmount();
      root.remove();
    },
  };
}

describe('AppReleaseIcon', () => {
  beforeEach(() => {
    fetchMock.mockReset();
    createObjectURL.mockClear();
    revokeObjectURL.mockClear();
    vi.stubGlobal('fetch', fetchMock);
    vi.stubGlobal(
      'URL',
      Object.assign(globalThis.URL, { createObjectURL, revokeObjectURL }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    document.body.innerHTML = '';
  });

  it('renders a protected icon blob with a meaningful accessible label', async () => {
    fetchMock.mockResolvedValue({
      blob: async () => new Blob(['icon'], { type: 'image/png' }),
      ok: true,
      status: 200,
    } as Response);
    const wrapper = mountIcon('/api/app-release-icons/1');

    await flushPreview();

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/app-release-icons/1',
      expect.objectContaining({
        headers: { Authorization: 'Bearer protected-token' },
      }),
    );
    expect(wrapper.root.querySelector('img')).toMatchObject({
      alt: '九型人格应用图标',
      src: 'blob:app-release-icon',
    });
    expect(
      wrapper.root.querySelector('.avatar-stub')?.getAttribute('aria-label'),
    ).toBe('九型人格应用图标');

    wrapper.unmount();
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:app-release-icon');
  });

  it.each([
    [
      'a rejected request',
      () => fetchMock.mockRejectedValue(new Error('offline')),
    ],
    [
      'a 404 response',
      () =>
        fetchMock.mockResolvedValue({
          blob: async () => new Blob(),
          ok: false,
          status: 404,
        } as Response),
    ],
  ])(
    'keeps an accessible fixed-size placeholder for %s',
    async (_, arrange) => {
      arrange();
      const wrapper = mountIcon('/api/app-release-icons/missing');

      await flushPreview();

      expect(wrapper.root.querySelector('img')).toBeNull();
      const avatar = wrapper.root.querySelector('.avatar-stub') as HTMLElement;
      expect(avatar.getAttribute('aria-label')).toBe('九型人格应用图标占位符');
      expect(avatar.style.height).toBe('48px');
      expect(avatar.style.width).toBe('48px');
      wrapper.unmount();
    },
  );

  it('uses the same stable placeholder without fetching when the icon url is empty', async () => {
    const wrapper = mountIcon('');

    await flushPreview();

    expect(fetchMock).not.toHaveBeenCalled();
    expect(wrapper.root.querySelector('img')).toBeNull();
    expect(
      wrapper.root.querySelector('.avatar-stub')?.getAttribute('aria-label'),
    ).toBe('九型人格应用图标占位符');
    wrapper.unmount();
  });
});
