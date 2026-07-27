/** @vitest-environment happy-dom */

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent } from 'vue';
import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

const mocks = vi.hoisted(() => ({
  accessCodes: [] as string[],
  uploadTasks: vi.fn(),
}));
vi.mock('@vben/common-ui', () => ({
  Page: defineComponent({ template: '<main><slot /></main>' }),
}));
vi.mock('@vben/stores', () => ({
  useAccessStore: () => ({
    get accessCodes() {
      return mocks.accessCodes;
    },
  }),
}));
vi.mock('ant-design-vue', async () => {
  const stubs = await import('#/test-utils/antd-stubs');
  const simple = (name: string) =>
    defineComponent({ name, template: '<div><slot /></div>' });
  const Radio = Object.assign(simple('Radio'), {
    Button: simple('RadioButton'),
    Group: simple('RadioGroup'),
  });
  const Input = Object.assign(stubs.Input, {
    TextArea: simple('InputTextArea'),
  });
  return {
    ...stubs,
    Checkbox: simple('Checkbox'),
    Input,
    InputNumber: simple('InputNumber'),
    Progress: defineComponent({
      props: ['percent'],
      template: '<div>progress:{{ percent }}<slot /></div>',
    }),
    Radio,
    Modal: Object.assign(stubs.Modal, {
      confirm: ({ onOk }: { onOk?: () => void | Promise<void> }) => onOk?.(),
    }),
    Tabs: defineComponent({
      props: ['items'],
      template:
        '<nav><span v-for="item in items" :key="item.key">{{ item.label }}</span></nav>',
    }),
  };
});
vi.mock('#/api/core/classroom', () => ({
  abortClassroomUploadApi: vi.fn(),
  completeClassroomUploadApi: vi.fn(),
  createClassroomContentApi: vi.fn(),
  createClassroomSeriesApi: vi.fn(),
  deleteClassroomContentApi: vi.fn(),
  deleteClassroomSeriesApi: vi.fn(),
  getClassroomContentsApi: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  getClassroomSeriesApi: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  getClassroomUploadTasksApi: mocks.uploadTasks,
  initiateClassroomUploadApi: vi.fn(),
  reportClassroomUploadProgressApi: vi.fn().mockResolvedValue({}),
  offlineClassroomContentApi: vi.fn(),
  offlineClassroomSeriesApi: vi.fn(),
  publishClassroomContentApi: vi.fn(),
  publishClassroomSeriesApi: vi.fn(),
  setClassroomContentPlaybackBlockedApi: vi.fn(),
  setClassroomContentPriceApi: vi.fn(),
  setClassroomSeriesPlaybackBlockedApi: vi.fn(),
  setClassroomSeriesPriceApi: vi.fn(),
  signClassroomUploadPartApi: vi.fn(),
  updateClassroomContentApi: vi.fn(),
  updateClassroomSeriesApi: vi.fn(),
}));

import ClassroomIndex from './index.vue';
import { getClassroomContentsApi } from '#/api/core/classroom';
import * as uploadFlow from './upload-flow';
import { crc64File } from './upload-checksum';

describe('classroom management permission integration', () => {
  beforeEach(() => {
    mocks.accessCodes = ['Miniapp:Classroom:List'];
    mocks.uploadTasks.mockReset();
    vi.mocked(getClassroomContentsApi).mockResolvedValue({
      items: [],
      page: 1,
      pageSize: 50,
      total: 0,
    });
  });

  it('does not mount or request upload tasks without Upload permission', async () => {
    const wrapper = mountVueComponent(ClassroomIndex);
    await flushVuePromises();
    expect(wrapper.text()).not.toContain('上传任务');
    expect(mocks.uploadTasks).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});

describe('classroom upload retry identity and persisted progress integration', () => {
  const task = {
    id: 91,
    contentId: 7,
    originalFilename: 'lesson.mp4',
    expectedChecksum: 'crc64:expected',
    expectedSize: 4,
    completedParts: 1,
    completedBytes: 2,
    totalBytes: 4,
    progressPercent: 50,
    partSize: 4,
    maxParts: 1,
    status: 'failed',
    cleanupStatus: 'retained',
    attemptCount: 1,
    expiresAt: '2026-01-01T00:00:00Z',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
  } as any;

  beforeEach(() => {
    mocks.accessCodes = ['Miniapp:Classroom:List', 'Miniapp:Classroom:Upload'];
    mocks.uploadTasks.mockReset();
    vi.restoreAllMocks();
  });

  async function mountUploadTasks(taskValue = task) {
    mocks.uploadTasks.mockResolvedValue({
      items: [taskValue],
      page: 1,
      pageSize: 50,
      total: 1,
    });
    const { defineComponent, h } = await import('vue');
    const { default: UploadTasks } = await import('./upload-tasks.vue');
    const Host = defineComponent({
      setup: () => () =>
        h(UploadTasks, {
          canUpload: true,
          contents: [{ id: 7, title: '课件', status: 'draft' }] as any,
        }),
    });
    const wrapper = mountVueComponent(Host);
    await flushVuePromises();
    return wrapper;
  }

  function choose(file: File) {
    const input = document.querySelector(
      'input[type="file"]',
    ) as HTMLInputElement;
    Object.defineProperty(input, 'files', {
      configurable: true,
      value: [file],
    });
    input.dispatchEvent(new Event('change'));
  }

  it('forces a replacement file and rejects retry when filename/checksum identity mismatches', async () => {
    const identitySpy = vi.spyOn(uploadFlow, 'matchesUploadIdentity');
    const wrapper = await mountUploadTasks();
    wrapper.button('重试')?.click();
    await flushVuePromises();
    choose(new File(['xxxx'], 'other.mp4', { type: 'video/mp4' }));
    wrapper.button('开始上传')?.click();
    await vi.waitFor(() => expect(identitySpy).toHaveBeenCalled());
    const { initiateClassroomUploadApi } = await import('#/api/core/classroom');
    expect(identitySpy).toHaveBeenCalled();
    expect(vi.mocked(initiateClassroomUploadApi)).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it('initiates retry only after the replacement file matches persisted identity', async () => {
    const identitySpy = vi.spyOn(uploadFlow, 'matchesUploadIdentity');
    const file = new File(['xxxx'], 'lesson.mp4', { type: 'video/mp4' });
    const expectedChecksum = await crc64File(file);
    const wrapper = await mountUploadTasks({ ...task, expectedChecksum });
    wrapper.button('重试')?.click();
    await flushVuePromises();
    choose(file);
    wrapper.button('开始上传')?.click();
    await vi.waitFor(() => expect(identitySpy).toHaveBeenCalled());
    await flushVuePromises();
    const { initiateClassroomUploadApi } = await import('#/api/core/classroom');
    expect(vi.mocked(initiateClassroomUploadApi)).toHaveBeenCalled();
    wrapper.unmount();
  });

  it('renders persisted API progress and refreshes it through polling', async () => {
    const mergeSpy = vi.spyOn(uploadFlow, 'mergeUploadProgress');
    vi.useFakeTimers();
    mocks.uploadTasks
      .mockResolvedValueOnce({
        items: [{ ...task, progressPercent: 42, completedBytes: 2 }],
        page: 1,
        pageSize: 50,
        total: 1,
      })
      .mockResolvedValueOnce({
        items: [{ ...task, progressPercent: 76, completedBytes: 3 }],
        page: 1,
        pageSize: 50,
        total: 1,
      });
    const { defineComponent, h } = await import('vue');
    const { default: UploadTasks } = await import('./upload-tasks.vue');
    const Host = defineComponent({
      setup: () => () =>
        h(UploadTasks, { canUpload: true, contents: [] as any }),
    });
    const wrapper = mountVueComponent(Host);
    await flushVuePromises();
    expect(mergeSpy).toHaveBeenCalled();
    expect(wrapper.text()).toContain('progress:42');
    await vi.advanceTimersByTimeAsync(5000);
    await flushVuePromises();
    expect(wrapper.text()).toContain('progress:76');
    wrapper.unmount();
    vi.useRealTimers();
  });
});
