/** @vitest-environment happy-dom */

import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { defineComponent, h, reactive } from 'vue';

import { beforeEach, describe, expect, it, vi } from 'vitest';

import { flushVuePromises, mountVueComponent } from '#/test-utils/vue-mount';

vi.mock('ant-design-vue', async () => {
  const stubs = await import('#/test-utils/antd-stubs');
  return {
    ...stubs,
    Progress: defineComponent({
      props: { percent: Number },
      template: '<div>progress:{{ percent }}</div>',
    }),
    Modal: Object.assign(stubs.Modal, {
      confirm: ({ onOk }: { onOk?: () => Promise<void> | void }) => onOk?.(),
    }),
  };
});

vi.mock('#/api/core/classroom', () => ({
  deleteClassroomSeriesCoverApi: vi.fn(),
  setClassroomSeriesCoverSettingsApi: vi.fn(),
  uploadClassroomSeriesCoverApi: vi.fn(),
}));

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const read = (path: string) => readFileSync(resolve(root, path), 'utf8');

const baseSeries = {
  accessLevel: 'public',
  coverAspectRatio: '16:9',
  coverUrl: 'https://cdn.example/first-lesson.jpg',
  createdAt: '2026-01-01T00:00:00Z',
  id: 28,
  manualCoverObjectKey: '',
  playbackBlocked: false,
  priceCents: 0,
  sortOrder: 1,
  status: 'draft',
  summary: '企业培训系列',
  teacherKey: 'teacher-1',
  teacherName: '韩老师',
  title: '企业九型课堂',
  updatedAt: 'v1',
} as any;

describe('classroom series cover API contract', () => {
  it('wires upload, delete and ratio settings to the series cover endpoints', () => {
    const api = read('api/core/classroom.ts');
    expect(api).toContain('uploadClassroomSeriesCoverApi');
    expect(api).toContain('deleteClassroomSeriesCoverApi');
    expect(api).toContain('setClassroomSeriesCoverSettingsApi');
    expect(api).toContain('`/admin/classroom/series/${id}/cover`');
    expect(api).toContain('`/admin/classroom/series/${id}/cover-settings`');
    expect(api).toContain("formData.set('file', file)");
    expect(api).toContain(
      "formData.set('expectedUpdatedAt', expectedUpdatedAt)",
    );
  });

  it('embeds series cover management in the series edit modal', () => {
    const source = read('views/classroom/series.vue');
    expect(source).toContain('SeriesCoverEditor');
    expect(source).toContain('请先保存系列，再管理封面');
    expect(source).toContain('@saved="replacePersistedSeries"');
  });
});

describe('classroom series cover editor', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('previews the effective cover and explains the first-lesson fallback', async () => {
    const { default: SeriesCoverEditor } =
      await import('./components/series-cover-editor.vue');
    const wrapper = mountVueComponent(
      defineComponent({
        setup: () => () => h(SeriesCoverEditor, { series: baseSeries }),
      }),
    );
    await flushVuePromises();

    expect(wrapper.text()).toContain('无手动封面时，自动回退到第一节课封面');
    expect(wrapper.text()).toContain('16:9');
    expect(wrapper.text()).toContain('9:16');
    expect(wrapper.text()).toContain('1:1');
    expect(document.querySelector('img')?.getAttribute('src')).toBe(
      baseSeries.coverUrl,
    );
    wrapper.unmount();
  });

  it('uploads, changes ratio, and deletes a manual series cover with current versions', async () => {
    const api = await import('#/api/core/classroom');
    vi.mocked(api.uploadClassroomSeriesCoverApi).mockResolvedValue({
      ...baseSeries,
      coverUrl: 'https://cdn.example/manual.jpg',
      manualCoverObjectKey: 'classroom/series/28/manual.jpg',
      updatedAt: 'v2',
    });
    vi.mocked(api.setClassroomSeriesCoverSettingsApi).mockResolvedValue({
      ...baseSeries,
      coverAspectRatio: '9:16',
      coverUrl: 'https://cdn.example/manual.jpg',
      manualCoverObjectKey: 'classroom/series/28/manual.jpg',
      updatedAt: 'v3',
    });
    vi.mocked(api.deleteClassroomSeriesCoverApi).mockResolvedValue({
      ...baseSeries,
      coverAspectRatio: '9:16',
      coverUrl: 'https://cdn.example/first-lesson-portrait.jpg',
      updatedAt: 'v4',
    });

    const series = reactive({ ...baseSeries });
    const { default: SeriesCoverEditor } =
      await import('./components/series-cover-editor.vue');
    const wrapper = mountVueComponent(
      defineComponent({
        setup: () => () =>
          h(SeriesCoverEditor, {
            series,
            onSaved: (value: any) => Object.assign(series, value),
          }),
      }),
    );
    await flushVuePromises();

    const input = document.querySelector(
      'input[type="file"]',
    ) as HTMLInputElement;
    const file = new File(['cover'], 'series-cover.webp', {
      type: 'image/webp',
    });
    Object.defineProperty(input, 'files', {
      configurable: true,
      value: [file],
    });
    input.dispatchEvent(new Event('change'));
    await flushVuePromises();
    wrapper.button('上传封面')?.click();
    await flushVuePromises();
    expect(api.uploadClassroomSeriesCoverApi).toHaveBeenCalledWith(
      28,
      file,
      'v1',
    );

    wrapper.button('9:16')?.click();
    await flushVuePromises();
    wrapper.button('保存比例')?.click();
    await flushVuePromises();
    expect(api.setClassroomSeriesCoverSettingsApi).toHaveBeenCalledWith(
      28,
      '9:16',
      'v2',
    );

    wrapper.button('删除封面')?.click();
    await flushVuePromises();
    expect(api.deleteClassroomSeriesCoverApi).toHaveBeenCalledWith(28, 'v3');
    wrapper.unmount();
  });
});
