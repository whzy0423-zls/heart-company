import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it, vi } from 'vitest';

import {
  contentMetadataPayload,
  createContentDraftDefaults,
  purchaseStrategyRequired,
} from './editor-model';
import { seriesMetadataPayload } from './series-model';
import {
  classroomOperationError,
  classroomPermissions,
  contentPublishGuard,
  playbackControl,
  uploadStatusLabel,
  visibleClassroomTabs,
} from './classroom-view-model';
import {
  classroomUploadMime,
  matchesClassroomContentType,
  matchesUploadIdentity,
  mergeUploadProgress,
  putSignedUploadPart,
  resolveUploadRetryContext,
  shouldAbortController,
} from './upload-flow';
import { saveContentWorkflow } from './editor-model';
import { crc64File } from './upload-checksum';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const read = (path: string) => readFileSync(resolve(root, path), 'utf8');

describe('teacher classroom admin UI contract', () => {
  it('registers a list-authorized classroom route and the three management tabs', () => {
    const route = read('router/routes/modules/classroom.ts');
    const index = read('views/classroom/index.vue');
    expect(route).toContain("authority: ['Miniapp:Classroom:List']");
    expect(route).toContain("path: '/classroom'");
    for (const label of ['课件管理', '课程系列', '上传任务']) {
      expect(index).toContain(label);
    }
  });

  it('supports video/audio, series/standalone and publication lifecycle', () => {
    const source =
      read('views/classroom/index.vue') +
      read('views/classroom/classroom-view-model.ts');
    for (const token of ['视频课件', '音频课件', '系列内容', '独立内容']) {
      expect(source).toContain(token);
    }
    for (const status of ['草稿', '已发布', '已下线', '播放已阻断']) {
      expect(source).toContain(status);
    }
  });

  it('keeps upload, publish and price actions independently permissioned', () => {
    const source =
      read('views/classroom/index.vue') +
      read('views/classroom/classroom-view-model.ts');
    const editor = read('views/classroom/components/content-editor.vue');
    expect(source).toContain('Miniapp:Classroom:Upload');
    expect(source).toContain('Miniapp:Classroom:Publish');
    expect(source).toContain('Miniapp:Classroom:Price');
    expect(source).toContain('canUpload');
    expect(source).toContain('canPublish');
    expect(source).toContain('canPrice');
    expect(editor).toContain('访问权限');
    expect(editor).toContain('单课价格');
    expect(editor).toContain(':disabled="!canPrice"');
  });

  it('makes paid standalone inheritance policy explicit before publish', () => {
    const editor = read('views/classroom/components/content-editor.vue');
    expect(editor).toContain('showAsStandalone');
    expect(editor).toContain('购买系列');
    expect(editor).toContain('单课付费');
    expect(editor).toContain('standalonePaidPolicyError');
  });

  it('persists content and pricing from the editor', () => {
    const editor = read('views/classroom/components/content-editor.vue');
    expect(editor).toContain('createClassroomContentApi');
    expect(editor).toContain('updateClassroomContentApi');
    expect(editor).toContain('setClassroomContentPriceApi');
    expect(editor).toContain('保存课件');
  });

  it('explains that an empty series list still allows standalone content', () => {
    const editor = read('views/classroom/components/content-editor.vue');
    const index = read('views/classroom/index.vue');
    expect(editor).toContain('暂无课程系列，可直接保存为独立课件');
    expect(editor).toContain('不加入系列，课件会独立展示');
    expect(index).toContain("editing ? '编辑课件' : '新建课件'");
  });

  it('provides series create/edit and pricing controls', () => {
    const series = read('views/classroom/series.vue');
    expect(series).toContain('createClassroomSeriesApi');
    expect(series).toContain('updateClassroomSeriesApi');
    expect(series).toContain('setClassroomSeriesPriceApi');
    expect(series).toContain('保存系列');
  });

  it('resolves series permissions when opened as a standalone menu page', () => {
    const series = read('views/classroom/series.vue');
    expect(series).toContain('useAccessStore');
    expect(series).toContain('props.canWrite ?? permissions.value.canWrite');
    expect(series).toContain('props.canPublish ?? permissions.value.canPublish');
    expect(series).toContain('props.canPrice ?? permissions.value.canPrice');
  });

  it('uploads selected media through the multipart API chain', () => {
    const upload = read('views/classroom/upload-tasks.vue');
    for (const api of [
      'initiateClassroomUploadApi',
      'signClassroomUploadPartApi',
      'completeClassroomUploadApi',
    ]) {
      expect(upload).toContain(api);
    }
    expect(upload).toContain(
      'accept="video/mp4,audio/mpeg,audio/mp4,audio/x-m4a"',
    );
  });

  it('makes the two upload prerequisites explicit before enabling upload', () => {
    const upload = read('views/classroom/upload-tasks.vue');
    expect(upload).toContain('1. 选择草稿课件');
    expect(upload).toContain('2. 选择媒体文件');
    expect(upload).toContain("return message.warning('请选择媒体文件')");
    expect(upload).toContain(
      ':disabled="!selectedContentId || !selectedFile || uploading"',
    );
  });

  it('builds legal defaults for a new standalone or series content draft', () => {
    const defaults = createContentDraftDefaults();
    expect(defaults).toMatchObject({
      accessLevel: 'public',
      contentType: 'video',
      showAsStandalone: false,
    });
    expect(defaults.seriesId).toBeUndefined();
  });

  it('does not leak access pricing controls into content metadata requests', () => {
    const payload = contentMetadataPayload({
      title: 'C',
      contentType: 'audio',
      accessLevel: 'paid',
      priceCents: 88,
    } as any);
    expect(payload).toMatchObject({ title: 'C', contentType: 'audio' });
    expect(payload).not.toHaveProperty('accessLevel');
    expect(payload).not.toHaveProperty('priceCents');
  });

  it('requires a purchase strategy only for standalone inherited paid series', () => {
    expect(
      purchaseStrategyRequired(
        { accessLevel: 'inherit', seriesId: 2, showAsStandalone: true },
        { accessLevel: 'paid' } as any,
      ),
    ).toBe(true);
    expect(
      purchaseStrategyRequired(
        { accessLevel: 'public', seriesId: 2, showAsStandalone: true },
        { accessLevel: 'paid' } as any,
      ),
    ).toBe(false);
    expect(
      purchaseStrategyRequired(
        { accessLevel: 'inherit', seriesId: 2, showAsStandalone: false },
        { accessLevel: 'paid' } as any,
      ),
    ).toBe(false);
  });

  it('strips pricing fields from series metadata requests', () => {
    const payload = seriesMetadataPayload({
      title: 'S',
      teacherName: 'T',
      accessLevel: 'paid',
      priceCents: 99,
    } as any);
    expect(payload).toEqual({
      title: 'S',
      teacherName: 'T',
      coverAssetId: undefined,
      coverUrl: undefined,
      sortOrder: undefined,
      summary: undefined,
      teacherKey: undefined,
    });
    expect(payload).not.toHaveProperty('accessLevel');
    expect(payload).not.toHaveProperty('priceCents');
  });

  it('keeps Upload, Publish and Price permissions independent', () => {
    expect(classroomPermissions(['Miniapp:Classroom:Upload'])).toEqual({
      canPrice: false,
      canPublish: false,
      canUpload: true,
      canWrite: false,
    });
    expect(
      classroomPermissions([
        'Miniapp:Classroom:Publish',
        'Miniapp:Classroom:Price',
      ]),
    ).toMatchObject({ canPrice: true, canPublish: true, canUpload: false });
  });

  it('switches playback controls between block and unblock', () => {
    expect(playbackControl(false)).toEqual({
      action: 'block',
      label: '阻断播放',
    });
    expect(playbackControl(true)).toEqual({
      action: 'unblock',
      label: '恢复播放',
    });
  });

  it('requires the parent series to be published before publishing its content', () => {
    const content = { id: 11, seriesId: 2, status: 'ready' } as any;
    expect(
      contentPublishGuard(content, [
        { id: 2, status: 'draft', title: '韩老师测试课程' },
      ] as any),
    ).toEqual({
      allowed: false,
      label: '先发布所属系列',
      reason: '请先到“课程系列”发布《韩老师测试课程》',
    });
    expect(
      contentPublishGuard(content, [
        { id: 2, status: 'published', title: '韩老师测试课程' },
      ] as any),
    ).toMatchObject({ allowed: true, label: '发布' });
  });

  it('shows the parent-series action instead of sending an invalid publish request', () => {
    const source = read('views/classroom/index.vue');
    expect(source).toContain('先发布所属系列');
    expect(source).toContain("activeTab = 'series'");
    expect(source).not.toContain('throw cause');
  });

  it('wires block and unblock controls for both content and series', () => {
    const content = read('views/classroom/index.vue');
    const series = read('views/classroom/series.vue');
    expect(content).toContain('setClassroomContentPlaybackBlockedApi');
    expect(series).toContain('setClassroomSeriesPlaybackBlockedApi');
    expect(content).toContain("record.playbackBlocked ? 'unblock' : 'block'");
    expect(series).toContain("record.playbackBlocked ? 'unblock' : 'block'");
  });

  it('restores upload retry from retained or reselected file context', () => {
    const task = { id: 7, contentId: 9 } as any;
    const original = new File(['a'], 'lesson.mp4', { type: 'video/mp4' });
    expect(
      resolveUploadRetryContext(
        task,
        new Map([[7, { contentId: 9, file: original }]]),
      ),
    ).toEqual({ contentId: 9, file: original });
    expect(resolveUploadRetryContext(task, new Map())).toBeUndefined();
    expect(resolveUploadRetryContext(task, new Map())).toBeUndefined();
  });

  it('requires confirmation before aborting multipart uploads', () => {
    const upload = read('views/classroom/upload-tasks.vue');
    expect(upload).toContain("title: '终止上传任务？'");
    expect(upload).toContain('Modal.confirm');
    expect(upload).toContain('performUpload(context.file, context.contentId)');
  });

  it('uses API error detail and fallback messages', () => {
    expect(classroomOperationError(new Error('价格冲突'), '保存失败')).toBe(
      '价格冲突',
    );
    expect(classroomOperationError({}, '保存失败')).toBe('保存失败');
  });

  it('hides upload tab and upload task mount when Upload permission is missing', () => {
    expect(visibleClassroomTabs(false)).toEqual(['contents', 'series']);
    expect(visibleClassroomTabs(true)).toContain('uploads');
    const source = read('views/classroom/index.vue');
    expect(source).toContain('visibleClassroomTabs(canUpload.value)');
    expect(source).toContain("activeTab === 'uploads' && canUpload");
  });

  it('merges upload task and content media statuses for operator-visible state', () => {
    expect(uploadStatusLabel('initiated')).toBe('等待上传');
    expect(uploadStatusLabel('uploading')).toBe('上传中');
    expect(uploadStatusLabel('completing')).toBe('正在合并');
    expect(uploadStatusLabel('completed', 'processing')).toBe('媒体处理中');
    expect(uploadStatusLabel('completed', 'ready')).toBe('可发布');
    expect(uploadStatusLabel('failed')).toBe('失败');
  });

  it('keeps metadata controls read-only for Price-only operators', () => {
    const editor = read('views/classroom/components/content-editor.vue');
    expect(editor).toContain(':disabled="!canWrite"');
    expect(editor).toContain(':disabled="!canPrice"');
  });

  it('does not create twice when the second price step fails and is retried', async () => {
    let creates = 0;
    let prices = 0;
    let persisted: any;
    const create = async () => {
      creates++;
      return { id: 1, updatedAt: 'v1' } as any;
    };
    const update = async () => {
      throw new Error('must not update');
    };
    await expect(
      saveContentWorkflow({
        create,
        current: undefined,
        metadataCommitted: false,
        onPersist: (value) => {
          persisted = value;
        },
        price: async () => {
          prices++;
          throw new Error('price down');
        },
        update,
      }),
    ).rejects.toThrow('price down');
    await expect(
      saveContentWorkflow({
        create,
        current: persisted,
        metadataCommitted: true,
        onPersist: (value) => {
          persisted = value;
        },
        price: async () => {
          prices++;
          return { id: 1, updatedAt: 'v2' } as any;
        },
        update,
      }),
    ).resolves.toMatchObject({ id: 1, updatedAt: 'v2' });
    expect(creates).toBe(1);
    expect(prices).toBe(2);
  });

  it('yields checksum progress and honors cancellation for large files', async () => {
    const file = new File([new Uint8Array(1024 * 1024)], 'large.mp4');
    const controller = new AbortController();
    const progress: number[] = [];
    await expect(
      crc64File(file, { onProgress: (value) => progress.push(value) }),
    ).resolves.toMatch(/^crc64:\d+$/);
    expect(progress.at(-1)).toBe(100);
    controller.abort();
    await expect(
      crc64File(file, { signal: controller.signal }),
    ).rejects.toMatchObject({ name: 'AbortError' });
    const midController = new AbortController();
    await expect(
      crc64File(file, {
        onProgress: () => midController.abort(),
        signal: midController.signal,
      }),
    ).rejects.toMatchObject({ name: 'AbortError' });
  });

  it('falls back to filename extension when browser MIME is empty', () => {
    expect(classroomUploadMime({ name: 'lesson.mp4', type: '' } as File)).toBe(
      'video/mp4',
    );
    expect(classroomUploadMime({ name: 'lesson.mp3', type: '' } as File)).toBe(
      'audio/mpeg',
    );
  });

  it('rejects audio files for video lessons and video files for audio lessons', () => {
    expect(matchesClassroomContentType('video', 'video/mp4')).toBe(true);
    expect(matchesClassroomContentType('audio', 'audio/x-m4a')).toBe(true);
    expect(matchesClassroomContentType('video', 'audio/x-m4a')).toBe(false);
    expect(matchesClassroomContentType('audio', 'video/mp4')).toBe(false);
  });

  it('aborts only the original active task controller', () => {
    expect(shouldAbortController(3, 3)).toBe(true);
    expect(shouldAbortController(3, 4)).toBe(false);
    expect(shouldAbortController(undefined, 3)).toBe(false);
    expect(read('views/classroom/upload-tasks.vue')).toContain(
      'const activeAtStart = activeTaskId.value',
    );
    expect(read('views/classroom/upload-tasks.vue')).toContain(
      'await abortClassroomUploadApi(taskId)',
    );
  });

  it('requires task-specific retry identity and merges real bytes progress', () => {
    const identity = {
      checksum: 'crc64:1',
      contentId: 7,
      filename: 'a.mp4',
      size: 10,
    };
    expect(
      matchesUploadIdentity(
        identity,
        { name: 'a.mp4', size: 10 },
        7,
        'crc64:1',
      ),
    ).toBe(true);
    expect(
      matchesUploadIdentity(
        identity,
        { name: 'b.mp4', size: 10 },
        7,
        'crc64:1',
      ),
    ).toBe(false);
    expect(
      mergeUploadProgress(
        { completedBytes: 4, totalBytes: 10 },
        { completedBytes: 7, expectedSize: 10 },
      ),
    ).toEqual({ completedBytes: 7, totalBytes: 10 });
  });

  it('requires successful PUT and exposed ETag for signed OSS parts', async () => {
    const signal = new AbortController().signal;
    await expect(
      putSignedUploadPart(
        'https://oss/part',
        new Blob(['x']),
        signal,
        vi.fn().mockResolvedValue(new Response('', { status: 200 })) as any,
      ),
    ).rejects.toThrow('OSS CORS');
    const fetcher = vi
      .fn()
      .mockResolvedValue(
        new Response('', { status: 200, headers: { ETag: '"part-etag"' } }),
      );
    await expect(
      putSignedUploadPart(
        'https://oss/part',
        new Blob(['x']),
        signal,
        fetcher as any,
      ),
    ).resolves.toBe('part-etag');
    expect(fetcher).toHaveBeenCalledWith(
      'https://oss/part',
      expect.objectContaining({ method: 'PUT', signal }),
    );
  });

  it('shows progress, retry, loading, empty and error feedback', () => {
    const source = read('views/classroom/upload-tasks.vue');
    for (const token of [
      'Progress',
      '上传进度',
      '重试',
      'Empty',
      'Alert',
      'loading',
    ]) {
      expect(source).toContain(token);
    }
  });

  it('uses responsive layout, visible focus and dangerous action confirmations', () => {
    const sources = [
      read('views/classroom/index.vue'),
      read('views/classroom/series.vue'),
    ].join('\n');
    expect(sources).toContain('@media (max-width: 768px)');
    expect(sources).toContain(':focus-visible');
    expect(sources).toContain('Modal.confirm');
  });
});
