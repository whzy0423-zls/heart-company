/** @vitest-environment happy-dom */

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, h, reactive } from 'vue';
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
    Card: defineComponent({
      template: '<section><slot name="extra" /><slot /></section>',
    }),
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
  deleteClassroomContentCoverApi: vi.fn(),
  setClassroomContentCoverSettingsApi: vi.fn(),
  uploadClassroomContentCoverApi: vi.fn(),
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

  it('shows create series when the standalone menu has Write permission', async () => {
    mocks.accessCodes = [
      'Miniapp:Classroom:List',
      'Miniapp:Classroom:Write',
    ];
    const { default: SeriesView } = await import('./series.vue');
    const wrapper = mountVueComponent(SeriesView);
    await flushVuePromises();
    expect(wrapper.text()).toContain('新建系列');
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
          contents: [
            { id: 7, title: '课件', status: 'draft', contentType: 'video' },
          ] as any,
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

  it('shows the selected filename from component state', async () => {
    const wrapper = await mountUploadTasks();
    const file = new File(['xxxx'], 'teacher-training.mp4', {
      type: 'video/mp4',
    });

    wrapper.button('重试')?.click();
    await flushVuePromises();
    choose(file);
    await flushVuePromises();

    const text = wrapper.text();
    const uploadDisabled = wrapper.button('开始上传')?.disabled;
    wrapper.unmount();

    expect(text).toContain('teacher-training.mp4');
    expect(uploadDisabled).toBe(false);
  });

  it('forces a replacement file and rejects retry when filename/checksum identity mismatches', async () => {
    const identitySpy = vi.spyOn(uploadFlow, 'matchesUploadIdentity');
    const wrapper = await mountUploadTasks();
    wrapper.button('重试')?.click();
    await flushVuePromises();
    choose(new File(['xxxx'], 'other.mp4', { type: 'video/mp4' }));
    await flushVuePromises();
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
    await flushVuePromises();
    wrapper.button('开始上传')?.click();
    await vi.waitFor(() => expect(identitySpy).toHaveBeenCalled());
    await flushVuePromises();
    const { initiateClassroomUploadApi } = await import('#/api/core/classroom');
    expect(vi.mocked(initiateClassroomUploadApi)).toHaveBeenCalled();
    wrapper.unmount();
  });

  it('aborts and cleans the reserved task when multipart upload fails', async () => {
    const file = new File(['xxxx'], 'lesson.mp4', { type: 'video/mp4' });
    const expectedChecksum = await crc64File(file);
    const reserved = {
      ...task,
      id: 92,
      expectedChecksum,
      partSize: 4,
      status: 'initiated',
    } as any;
    const {
      abortClassroomUploadApi,
      initiateClassroomUploadApi,
      signClassroomUploadPartApi,
    } = await import('#/api/core/classroom');
    vi.mocked(initiateClassroomUploadApi).mockResolvedValue({ task: reserved });
    vi.mocked(signClassroomUploadPartApi).mockRejectedValue(
      new Error('part upload failed'),
    );
    vi.mocked(abortClassroomUploadApi).mockResolvedValue({
      ...reserved,
      status: 'aborted',
    });

    const wrapper = await mountUploadTasks({ ...task, expectedChecksum });
    wrapper.button('重试')?.click();
    await flushVuePromises();
    choose(file);
    await flushVuePromises();
    wrapper.button('开始上传')?.click();
    await vi.waitFor(() =>
      expect(vi.mocked(abortClassroomUploadApi)).toHaveBeenCalledWith(92),
    );
    wrapper.unmount();
  });

  it('renders persisted API progress and refreshes it through polling', async () => {
    const mergeSpy = vi.spyOn(uploadFlow, 'mergeUploadProgress');
    vi.useFakeTimers();
    mocks.uploadTasks
      .mockResolvedValueOnce({
        items: [
          {
            ...task,
            status: 'uploading',
            progressPercent: 42,
            completedBytes: 2,
          },
        ],
        page: 1,
        pageSize: 50,
        total: 1,
      })
      .mockResolvedValueOnce({
        items: [
          {
            ...task,
            status: 'uploading',
            progressPercent: 76,
            completedBytes: 3,
          },
        ],
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

describe('classroom cover editor workflow', () => {
  const baseContent = {
    badge: '',
    contentType: 'video',
    coverAspectRatio: '16:9',
    coverSource: 'video',
    coverUrl: 'https://cdn.example/video-cover.jpg',
    createdAt: '2026-01-01T00:00:00Z',
    description: '企业培训',
    durationSeconds: 120,
    effectiveAccessLevel: 'public',
    effectivePriceCents: 0,
    episodeNo: 1,
    id: 88,
    manualCoverObjectKey: '',
    playbackBlocked: false,
    priceCents: 0,
    recordedAt: '2026-01-01T00:00:00Z',
    seriesId: undefined,
    showAsStandalone: false,
    sortOrder: 1,
    status: 'draft',
    tags: [],
    teacherKey: 'teacher-1',
    teacherName: '韩老师',
    title: '封面课件',
    updatedAt: 'v1',
  } as any;

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows the unsaved cover reminder in the content editor', async () => {
    const { default: ContentEditor } = await import(
      './components/content-editor.vue'
    );
    const Host = defineComponent({
      setup: () => () =>
        h(ContentEditor, {
          canPrice: true,
          canWrite: true,
          content: undefined,
          series: [],
        }),
    });
    const wrapper = mountVueComponent(Host);
    await flushVuePromises();
    expect(wrapper.text()).toContain('请先保存课件，再管理封面');
    wrapper.unmount();
  });

  it('refreshes the cover version after upload, ratio update, and delete', async () => {
    const api = await import('#/api/core/classroom');
    const {
      deleteClassroomContentCoverApi,
      setClassroomContentCoverSettingsApi,
      uploadClassroomContentCoverApi,
    } = api;
    vi.mocked(uploadClassroomContentCoverApi).mockResolvedValue({
      ...baseContent,
      coverAspectRatio: '16:9',
      coverSource: 'manual',
      coverUrl: 'https://cdn.example/manual-cover.jpg',
      manualCoverObjectKey: 'classroom/covers/manual/88/cover.jpg',
      updatedAt: 'v2',
    } as any);
    vi.mocked(setClassroomContentCoverSettingsApi).mockResolvedValue({
      ...baseContent,
      coverAspectRatio: '9:16',
      coverSource: 'manual',
      coverUrl: 'https://cdn.example/manual-cover-portrait.jpg',
      manualCoverObjectKey: 'classroom/covers/manual/88/cover.jpg',
      updatedAt: 'v3',
    } as any);
    vi.mocked(deleteClassroomContentCoverApi).mockResolvedValue({
      ...baseContent,
      coverAspectRatio: '9:16',
      coverSource: 'video',
      coverUrl: 'https://cdn.example/video-cover-portrait.jpg',
      manualCoverObjectKey: '',
      updatedAt: 'v4',
    } as any);
    const content = reactive({ ...baseContent });
    const { default: ContentCoverEditor } = await import(
      './components/content-cover-editor.vue'
    );
    const Host = defineComponent({
      setup: () => () =>
        h(ContentCoverEditor, {
          content,
          onSaved: (value: any) => Object.assign(content, value),
        }),
    });
    const wrapper = mountVueComponent(Host);
    await flushVuePromises();

    const input = document.querySelector(
      'input[type="file"]',
    ) as HTMLInputElement;
    Object.defineProperty(input, 'files', {
      configurable: true,
      value: [new File(['cover'], 'cover.png', { type: 'image/png' })],
    });
    input.dispatchEvent(new Event('change'));
    await flushVuePromises();
    wrapper.button('上传封面')?.click();
    await flushVuePromises();
    expect(vi.mocked(uploadClassroomContentCoverApi)).toHaveBeenCalledWith(
      88,
      expect.any(File),
      'v1',
    );

    wrapper.button('9:16')?.click();
    await flushVuePromises();
    wrapper.button('保存比例')?.click();
    await flushVuePromises();
    expect(vi.mocked(setClassroomContentCoverSettingsApi)).toHaveBeenCalledWith(
      88,
      '9:16',
      'v2',
    );

    wrapper.button('删除封面')?.click();
    await flushVuePromises();
    expect(vi.mocked(deleteClassroomContentCoverApi)).toHaveBeenCalledWith(
      88,
      'v3',
    );
    wrapper.unmount();
  });
});
