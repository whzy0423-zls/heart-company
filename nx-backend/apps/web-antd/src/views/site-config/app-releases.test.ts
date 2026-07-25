/* eslint-disable vue/one-component-per-file */
import type { AppRelease } from '#/api';

import { defineComponent, h } from 'vue';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

const mocks = vi.hoisted(() => ({
  accessCodes: ['Website:AppReleases:Write'],
  accessToken: 'page-token',
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
    Progress: passthrough('Progress'),
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
    packageName: 'com.example.default',
    platform: 'android',
    publishedAt: null,
    releaseNotes: '稳定性改进',
    sha256: 'abc',
    status: 'draft',
    versionCode: 100,
    versionName: '1.0.0',
    ...input,
  };
}

describe('App release metadata page', () => {
  beforeEach(() => {
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
    vi.mocked(getAppReleaseListApi).mockResolvedValue({
      current: release({
        appName: '当前正式应用',
        fileName: 'current.apk',
        iconUrl: '/api/app-release-icons/current',
        id: 10,
        packageName: 'com.example.current.application',
        status: 'published',
        versionCode: 321,
        versionName: '3.2.1',
      }),
      items: [
        release({
          appName: '历史测试应用',
          fileName: 'history.apk',
          iconUrl: '/api/app-release-icons/current',
          id: 9,
          packageName: 'com.example.history.application.with.long.name',
          status: 'archived',
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
    ).toBe('1320');
    wrapper.unmount();
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:shared-app-icon');
  });
});
