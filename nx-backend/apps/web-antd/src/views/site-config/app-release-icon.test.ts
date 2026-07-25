/* eslint-disable vue/one-component-per-file */
import { createApp, defineComponent, h, nextTick } from 'vue';

import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('ant-design-vue', () => ({
  Avatar: defineComponent({
    name: 'AvatarStub',
    inheritAttrs: false,
    props: {
      alt: { default: '', type: String },
      loadError: { default: undefined, type: Function },
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
            ? h('img', {
                alt: props.alt,
                onError: () => props.loadError?.(),
                src: props.src,
              })
            : slots.default?.(),
        );
    },
  }),
}));

import AppReleaseIcon from './app-release-icon.vue';

function mountIcon(src: string) {
  const root = document.createElement('div');
  document.body.append(root);
  const app = createApp(AppReleaseIcon, {
    appName: '九型人格',
    packageName: 'com.example.ninexing',
    size: 48,
    src,
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
  afterEach(() => {
    vi.restoreAllMocks();
    document.body.innerHTML = '';
  });

  it('renders a resolved image with a meaningful accessible label', () => {
    const wrapper = mountIcon('blob:app-release-icon');

    expect(wrapper.root.querySelector('img')).toMatchObject({
      alt: '九型人格应用图标',
      src: 'blob:app-release-icon',
    });
    expect(
      wrapper.root.querySelector('.avatar-stub')?.getAttribute('aria-label'),
    ).toBe('九型人格应用图标');
    wrapper.unmount();
  });

  it('uses the Avatar loadError callback to reveal the accessible fallback', async () => {
    const wrapper = mountIcon('blob:broken-app-release-icon');

    wrapper.root.querySelector('img')?.dispatchEvent(new Event('error'));
    await nextTick();

    expect(wrapper.root.querySelector('img')).toBeNull();
    const avatar = wrapper.root.querySelector('.avatar-stub') as HTMLElement;
    expect(avatar.getAttribute('aria-label')).toBe('九型人格应用图标占位符');
    expect(avatar.style.height).toBe('48px');
    expect(avatar.style.width).toBe('48px');
    expect(avatar.textContent).toBe('九');
    wrapper.unmount();
  });

  it('uses the same stable placeholder when the resolved src is empty', () => {
    const wrapper = mountIcon('');

    expect(wrapper.root.querySelector('img')).toBeNull();
    expect(
      wrapper.root.querySelector('.avatar-stub')?.getAttribute('aria-label'),
    ).toBe('九型人格应用图标占位符');
    wrapper.unmount();
  });
});
