import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

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
    const source = read('views/classroom/index.vue');
    for (const token of ['视频课件', '音频课件', '系列内容', '独立内容']) {
      expect(source).toContain(token);
    }
    for (const status of ['草稿', '已发布', '已下线', '播放已阻断']) {
      expect(source).toContain(status);
    }
  });

  it('keeps upload, publish and price actions independently permissioned', () => {
    const source = read('views/classroom/index.vue');
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
