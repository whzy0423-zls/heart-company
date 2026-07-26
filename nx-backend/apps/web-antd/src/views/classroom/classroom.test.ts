import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

import {
  contentMetadataPayload,
  createContentDraftDefaults,
  purchaseStrategyRequired,
} from './editor-model';
import { seriesMetadataPayload } from './series-model';
import {
  classroomOperationError,
  classroomPermissions,
  playbackControl,
} from './classroom-view-model';
import { resolveUploadRetryContext } from './upload-flow';

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

  it('provides series create/edit and pricing controls', () => {
    const series = read('views/classroom/series.vue');
    expect(series).toContain('createClassroomSeriesApi');
    expect(series).toContain('updateClassroomSeriesApi');
    expect(series).toContain('setClassroomSeriesPriceApi');
    expect(series).toContain('保存系列');
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
    const reselected = new File(['b'], 'lesson.mp4', { type: 'video/mp4' });
    expect(resolveUploadRetryContext(task, new Map(), reselected)).toEqual({
      contentId: 9,
      file: reselected,
    });
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
