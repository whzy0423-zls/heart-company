/* eslint-disable vue/one-component-per-file */
import type { AppRelease } from '#/api';

import { defineComponent, h, ref, watch } from 'vue';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

const mocks = vi.hoisted(() => ({
  accessCodes: ['Website:AppReleases:Write'],
  accessToken: 'page-token',
  messageError: vi.fn(),
  messageSuccess: vi.fn(),
  messageWarning: vi.fn(),
  updatePolicy: vi.fn(),
}));

vi.mock('@ant-design/icons-vue', () => ({
  SettingOutlined: defineComponent({
    name: 'SettingOutlined',
    setup: () => () => h('span', { class: 'setting-icon' }),
  }),
}));

vi.mock('@vben/common-ui', () => ({
  Page: defineComponent({
    name: 'PageStub',
    setup(_, { slots }) {
      return () => h('main', slots.default?.());
    },
  }),
}));

vi.mock('@vben/stores', () => ({
  useAccessStore: () => ({
    get accessCodes() {
      return mocks.accessCodes;
    },
    get accessToken() {
      return mocks.accessToken;
    },
  }),
}));

vi.mock('ant-design-vue', async () => {
  const stubs = await import('#/test-utils/antd-stubs');
  const passthrough = (name: string, tag = 'div') =>
    defineComponent({
      name,
      inheritAttrs: false,
      setup(_, { attrs, slots }) {
        return () => h(tag, attrs, slots.default?.());
      },
    });
  const Input = passthrough('Input', 'input') as any;
  Input.TextArea = passthrough('InputTextArea', 'textarea');
  const modelInput = (name: string, type = 'number') =>
    defineComponent({
      name,
      inheritAttrs: false,
      props: {
        value: { default: undefined, type: Number },
      },
      emits: ['update:value'],
      setup(props, { attrs, emit }) {
        return () =>
          h('input', {
            ...attrs,
            type,
            value: props.value,
            onInput: (event: Event) =>
              emit(
                'update:value',
                Number((event.target as HTMLInputElement).value),
              ),
          });
      },
    });
  const Switch = defineComponent({
    name: 'SwitchStub',
    inheritAttrs: false,
    props: { checked: { default: false, type: Boolean } },
    emits: ['update:checked'],
    setup(props, { attrs, emit }) {
      return () =>
        h('button', {
          ...attrs,
          'aria-checked': String(props.checked),
          role: 'switch',
          onClick: () => emit('update:checked', !props.checked),
        });
    },
  });
  const Slider = defineComponent({
    name: 'SliderStub',
    inheritAttrs: false,
    props: {
      ariaLabelForHandle: { default: undefined, type: String },
      value: { default: undefined, type: Number },
    },
    emits: ['update:value'],
    setup(props, { attrs, emit }) {
      return () =>
        h('div', attrs, [
          h('span', {
            'aria-label': props.ariaLabelForHandle,
            'aria-valuenow': props.value,
            role: 'slider',
            onKeydown: () => emit('update:value', props.value),
          }),
        ]);
    },
  });
  const Modal = Object.assign(
    defineComponent({
      name: 'ModalStub',
      inheritAttrs: false,
      props: {
        cancelButtonProps: { default: () => ({}), type: Object },
        confirmLoading: { default: false, type: Boolean },
        open: { default: false, type: Boolean },
        title: { default: '', type: String },
      },
      emits: ['afterClose', 'cancel', 'ok', 'update:open'],
      setup(props, { attrs, emit, slots }) {
        const rendered = ref(props.open);
        watch(
          () => props.open,
          (open) => {
            if (open) rendered.value = true;
          },
        );
        return () =>
          rendered.value
            ? h(
                'div',
                { ...attrs, 'data-open': String(props.open), role: 'dialog' },
                [
                  props.title,
                  slots.default?.(),
                  h(
                    'button',
                    {
                      disabled: props.confirmLoading,
                      onClick: () => emit('ok'),
                    },
                    '保存策略',
                  ),
                  h(
                    'button',
                    {
                      disabled: Boolean(
                        (props.cancelButtonProps as { disabled?: boolean })
                          .disabled,
                      ),
                      onClick: () => emit('cancel'),
                    },
                    '取消',
                  ),
                  h(
                    'button',
                    { onClick: () => emit('afterClose') },
                    '完成关闭动画',
                  ),
                ],
              )
            : null;
      },
    }),
    { confirm: vi.fn() },
  );
  const Table = defineComponent({
    name: 'TableStub',
    props: {
      columns: { default: () => [], type: Array },
      dataSource: { default: () => [], type: Array },
      scroll: { default: () => ({}), type: Object },
    },
    setup(props, { slots }) {
      return () =>
        h(
          'div',
          {
            class: 'mock-table',
            'data-scroll-x': (props.scroll as { x?: number }).x,
          },
          (props.dataSource as Record<string, any>[]).flatMap((record) =>
            (props.columns as Record<string, any>[]).map((column) =>
              h('div', { class: 'mock-cell' }, [
                String(record[column.dataIndex] ?? ''),
                slots.bodyCell?.({ column, record }),
              ]),
            ),
          ),
        );
    },
  });
  return {
    ...stubs,
    Input,
    InputNumber: modelInput('InputNumber'),
    message: {
      error: mocks.messageError,
      success: mocks.messageSuccess,
      warning: mocks.messageWarning,
    },
    Modal,
    Progress: passthrough('Progress'),
    Slider,
    Switch,
    Table,
    Upload: Object.assign(passthrough('Upload'), {
      LIST_IGNORE: 'LIST_IGNORE',
    }),
  };
});

vi.mock('./app-release-icon.vue', () => ({
  default: defineComponent({
    name: 'AppReleaseIcon',
    inheritAttrs: false,
    props: {
      appName: { default: '', type: String },
      packageName: { default: '', type: String },
      size: { default: 40, type: Number },
      src: { default: '', type: String },
    },
    setup(props) {
      return () =>
        h('span', {
          class: 'app-release-icon-stub',
          'data-app-name': props.appName,
          'data-package-name': props.packageName,
          'data-size': props.size,
          'data-src': props.src,
        });
    },
  }),
}));

vi.mock('#/api', () => ({
  archiveAppReleaseApi: vi.fn(),
  getAppReleaseListApi: vi.fn(),
  publishAppReleaseApi: vi.fn(),
  updateAppReleasePolicyApi: mocks.updatePolicy,
  uploadAppReleaseApi: vi.fn(),
}));

import { getAppReleaseListApi } from '#/api';

import AppReleases from './app-releases.vue';

const fetchMock = vi.fn<typeof fetch>();

async function flushResolver() {
  await flushVuePromises();
  await new Promise((resolve) => window.setTimeout(resolve, 0));
  await flushVuePromises();
}

function release(input: Partial<AppRelease>): AppRelease {
  return {
    appName: '默认应用',
    createdAt: '2026-07-21T08:00:00Z',
    fileAvailable: true,
    fileName: 'nine-xing.apk',
    fileSize: 12_345_678,
    iconUrl: '/api/app-release-icons/1',
    id: 1,
    forceUpdate: false,
    minSupportedVersionCode: 0,
    packageName: 'com.example.default',
    platform: 'android',
    publishedAt: null,
    releaseNotes: '稳定性改进',
    rolloutPercentage: 100,
    sha256: 'abc',
    status: 'draft',
    versionCode: 100,
    versionName: '1.0.0',
    ...input,
  };
}

describe('App release metadata page', () => {
  beforeEach(() => {
    mocks.accessCodes = ['Website:AppReleases:Write'];
    fetchMock.mockReset();
    fetchMock.mockResolvedValue({
      blob: async () => new Blob(['icon'], { type: 'image/png' }),
      ok: true,
      status: 200,
    } as Response);
    vi.stubGlobal('fetch', fetchMock);
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:shared-app-icon');
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined);
    vi.mocked(getAppReleaseListApi).mockReset();
    mocks.messageError.mockReset();
    mocks.messageSuccess.mockReset();
    mocks.messageWarning.mockReset();
    mocks.updatePolicy.mockReset();
    vi.mocked(getAppReleaseListApi).mockResolvedValue({
      current: release({
        appName: '当前正式应用',
        fileName: 'current.apk',
        iconUrl: '/api/app-release-icons/current',
        id: 10,
        forceUpdate: true,
        minSupportedVersionCode: 300,
        packageName: 'com.example.current.application',
        status: 'published',
        rolloutPercentage: 40,
        versionCode: 321,
        versionName: '3.2.1',
      }),
      items: [
        release({
          appName: '历史测试应用',
          fileName: 'history.apk',
          iconUrl: '/api/app-release-icons/current',
          id: 9,
          forceUpdate: false,
          minSupportedVersionCode: 120,
          packageName: 'com.example.history.application.with.long.name',
          status: 'archived',
          rolloutPercentage: 100,
          versionCode: 210,
          versionName: '2.1.0',
        }),
      ],
      page: 1,
      pageSize: 20,
      total: 1,
      totalFileSize: 12_345_678,
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    document.body.innerHTML = '';
  });

  it('renders current and history app identities with versions and stable icons', async () => {
    const wrapper = mountVueComponent(AppReleases);

    await flushResolver();

    expect(wrapper.text()).toContain('当前正式应用');
    expect(wrapper.text()).toContain('com.example.current.application');
    expect(wrapper.text()).toContain('3.2.1');
    expect(wrapper.text()).toContain('321');
    expect(wrapper.text()).toContain('历史测试应用');
    expect(wrapper.text()).toContain(
      'com.example.history.application.with.long.name',
    );
    expect(wrapper.text()).toContain('2.1.0');
    expect(wrapper.text()).toContain('210');

    const icons = document.body.querySelectorAll('.app-release-icon-stub');
    expect(icons).toHaveLength(2);
    expect((icons[0] as HTMLElement | undefined)?.dataset.size).toBe('48');
    expect((icons[1] as HTMLElement | undefined)?.dataset.size).toBe('40');
    expect((icons[0] as HTMLElement | undefined)?.dataset.src).toBe(
      'blob:shared-app-icon',
    );
    expect((icons[1] as HTMLElement | undefined)?.dataset.src).toBe(
      'blob:shared-app-icon',
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/app-release-icons/current',
      expect.objectContaining({
        headers: { Authorization: 'Bearer page-token' },
      }),
    );
    expect(
      (document.body.querySelector('.mock-table') as HTMLElement | null)
        ?.dataset.scrollX,
    ).toBe('1510');
    wrapper.unmount();
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:shared-app-icon');
  });

  it('shows update policy for the current release and history records', async () => {
    const wrapper = mountVueComponent(AppReleases);

    await flushResolver();

    expect(wrapper.text()).toContain('最低支持版本 #300');
    expect(wrapper.text()).toContain('强制更新');
    expect(wrapper.text()).toContain('灰度 40%');
    expect(wrapper.text()).toContain('最低支持版本 #120');
    expect(wrapper.text()).toContain('可选更新');
    expect(wrapper.text()).toContain('灰度 100%');
    wrapper.unmount();
  });

  it('opens a labeled policy form with the selected release values', async () => {
    const wrapper = mountVueComponent(AppReleases);
    await flushResolver();

    wrapper.button('更新策略')?.click();
    await flushVuePromises();

    const dialog = document.body.querySelector('[role="dialog"]');
    expect(dialog?.textContent).toContain('更新 2.1.0 (#210) 的更新策略');
    expect(dialog?.textContent).toContain('强制更新优先于灰度比例');
    expect(dialog?.textContent).toContain('Android 安装仍需用户确认');
    expect(
      dialog?.querySelector<HTMLInputElement>('[aria-label="最低支持版本号"]')
        ?.value,
    ).toBe('120');
    expect(
      dialog
        ?.querySelector('[aria-label="最低支持版本号"]')
        ?.getAttribute('max'),
    ).toBe('210');
    expect(
      dialog?.querySelector('[role="slider"][aria-label="灰度发布比例滑块"]'),
    ).not.toBeNull();
    expect(
      dialog?.querySelector<HTMLInputElement>('[aria-label="灰度发布比例"]')
        ?.value,
    ).toBe('100');
    expect(
      dialog
        ?.querySelector('[aria-label="强制更新"]')
        ?.getAttribute('aria-checked'),
    ).toBe('false');
    wrapper.unmount();
  });

  it('saves a complete policy once and updates current plus history in place', async () => {
    const current = release({
      id: 10,
      status: 'published',
      versionCode: 321,
      versionName: '3.2.1',
    });
    vi.mocked(getAppReleaseListApi).mockResolvedValue({
      current,
      items: [current],
      page: 1,
      pageSize: 20,
      total: 1,
      totalFileSize: current.fileSize,
    });
    let resolveUpdate: ((value: AppRelease) => void) | undefined;
    mocks.updatePolicy.mockReturnValue(
      new Promise((resolve) => {
        resolveUpdate = resolve;
      }),
    );
    const wrapper = mountVueComponent(AppReleases);
    await flushResolver();
    wrapper.button('更新策略')?.click();
    await flushVuePromises();

    input('[aria-label="最低支持版本号"]', '250');
    input('[aria-label="灰度发布比例"]', '30');
    document.body
      .querySelector<HTMLButtonElement>('[aria-label="强制更新"]')
      ?.click();
    await flushVuePromises();
    wrapper.button('保存策略')?.click();
    wrapper.button('保存策略')?.click();
    await flushVuePromises();

    expect(mocks.updatePolicy).toHaveBeenCalledTimes(1);
    expect(wrapper.button('取消')?.disabled).toBe(true);
    expect(mocks.updatePolicy).toHaveBeenCalledWith(10, {
      forceUpdate: true,
      minSupportedVersionCode: 250,
      rolloutPercentage: 30,
    });

    resolveUpdate?.(
      release({
        ...current,
        forceUpdate: true,
        minSupportedVersionCode: 250,
        rolloutPercentage: 30,
      }),
    );
    await flushResolver();

    expect(wrapper.text().match(/最低支持版本 #250/g)).toHaveLength(2);
    expect(wrapper.text().match(/灰度 30%/g)).toHaveLength(2);
    expect(mocks.messageSuccess).toHaveBeenCalledWith('更新策略已保存');
    expect(
      (document.body.querySelector('[role="dialog"]') as HTMLElement | null)
        ?.dataset.open,
    ).toBe('false');
    expect(
      document.body.querySelector('[role="dialog"]')?.textContent,
    ).toContain('更新 3.2.1 (#321) 的更新策略');
    expect(
      document.body.querySelector<HTMLInputElement>(
        '[aria-label="最低支持版本号"]',
      )?.value,
    ).toBe('250');
    expect(
      document.body
        .querySelector('[aria-label="最低支持版本号"]')
        ?.getAttribute('max'),
    ).toBe('321');
    wrapper.button('完成关闭动画')?.click();
    await flushVuePromises();
    expect(
      document.body.querySelector<HTMLInputElement>(
        '[aria-label="最低支持版本号"]',
      )?.value,
    ).toBe('0');
    expect(
      document.body
        .querySelector('[aria-label="最低支持版本号"]')
        ?.getAttribute('max'),
    ).toBe('0');
    wrapper.unmount();
  });

  it('validates integer and range constraints before saving', async () => {
    const wrapper = mountVueComponent(AppReleases);
    await flushResolver();
    wrapper.button('更新策略')?.click();
    await flushVuePromises();

    input('[aria-label="最低支持版本号"]', '210.5');
    input('[aria-label="灰度发布比例"]', '0');
    wrapper.button('保存策略')?.click();
    await flushVuePromises();

    expect(mocks.updatePolicy).not.toHaveBeenCalled();
    expect(mocks.messageError).toHaveBeenCalledWith(
      '最低支持版本号必须是 0 到 210 之间的整数',
    );

    mocks.messageError.mockClear();
    input('[aria-label="最低支持版本号"]', '211');
    input('[aria-label="灰度发布比例"]', '50');
    wrapper.button('保存策略')?.click();
    await flushVuePromises();
    expect(mocks.messageError).toHaveBeenCalledWith(
      '最低支持版本号必须是 0 到 210 之间的整数',
    );

    mocks.messageError.mockClear();
    input('[aria-label="最低支持版本号"]', '100');
    input('[aria-label="灰度发布比例"]', '0');
    wrapper.button('保存策略')?.click();
    await flushVuePromises();
    expect(mocks.messageError).toHaveBeenCalledWith(
      '灰度发布比例必须是 1 到 100 之间的整数',
    );

    mocks.messageError.mockClear();
    input('[aria-label="灰度发布比例"]', '20.5');
    wrapper.button('保存策略')?.click();
    await flushVuePromises();
    expect(mocks.messageError).toHaveBeenCalledWith(
      '灰度发布比例必须是 1 到 100 之间的整数',
    );
    expect(mocks.updatePolicy).not.toHaveBeenCalled();
    expect(document.body.querySelector('[role="dialog"]')).not.toBeNull();
    wrapper.unmount();
  });

  it('keeps edited values open when saving fails and resets after cancel', async () => {
    mocks.updatePolicy.mockRejectedValue(new Error('network failed'));
    const wrapper = mountVueComponent(AppReleases);
    await flushResolver();
    wrapper.button('更新策略')?.click();
    await flushVuePromises();

    input('[aria-label="最低支持版本号"]', '100');
    wrapper.button('保存策略')?.click();
    await flushResolver();

    expect(mocks.messageError).toHaveBeenCalledWith('更新策略保存失败，请重试');
    expect(
      document.body.querySelector<HTMLInputElement>(
        '[aria-label="最低支持版本号"]',
      )?.value,
    ).toBe('100');

    wrapper.button('取消')?.click();
    await flushVuePromises();
    expect(
      (document.body.querySelector('[role="dialog"]') as HTMLElement | null)
        ?.dataset.open,
    ).toBe('false');
    expect(
      document.body.querySelector('[role="dialog"]')?.textContent,
    ).toContain('更新 2.1.0 (#210) 的更新策略');
    expect(
      document.body.querySelector<HTMLInputElement>(
        '[aria-label="最低支持版本号"]',
      )?.value,
    ).toBe('100');
    expect(
      document.body
        .querySelector('[aria-label="最低支持版本号"]')
        ?.getAttribute('max'),
    ).toBe('210');
    wrapper.button('完成关闭动画')?.click();
    await flushVuePromises();
    expect(
      document.body.querySelector<HTMLInputElement>(
        '[aria-label="最低支持版本号"]',
      )?.value,
    ).toBe('0');
    expect(
      document.body
        .querySelector('[aria-label="最低支持版本号"]')
        ?.getAttribute('max'),
    ).toBe('0');
    wrapper.button('更新策略')?.click();
    await flushVuePromises();
    expect(
      document.body.querySelector<HTMLInputElement>(
        '[aria-label="最低支持版本号"]',
      )?.value,
    ).toBe('120');
    wrapper.unmount();
  });

  it('keeps policy read-only without write permission', async () => {
    mocks.accessCodes = [];
    const wrapper = mountVueComponent(AppReleases);
    await flushResolver();

    expect(wrapper.text()).toContain('最低支持版本 #300');
    expect(wrapper.text()).toContain('最低支持版本 #120');
    expect(wrapper.button('更新策略')).toBeUndefined();
    expect(wrapper.button('选择 APK')).toBeUndefined();
    wrapper.unmount();
    mocks.accessCodes = ['Website:AppReleases:Write'];
  });
});

function input(selector: string, value: string) {
  const element = document.body.querySelector<HTMLInputElement>(selector);
  if (!element) throw new Error(`Missing input: ${selector}`);
  element.value = value;
  element.dispatchEvent(new Event('input', { bubbles: true }));
}
