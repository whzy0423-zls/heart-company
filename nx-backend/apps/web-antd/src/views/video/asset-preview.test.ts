import { describe, expect, it } from 'vitest';

import {
  getAssetPreviewKind,
  getAssetPreviewSource,
  withPreviewToken,
} from './asset-preview';

describe('asset preview helpers', () => {
  it('uses image preview for visual asset types', () => {
    for (const type of [
      'scene',
      'character',
      'prop',
      'outfit',
      'style',
    ] as const) {
      expect(getAssetPreviewKind(type, '/asset.png')).toBe('image');
    }
  });

  it('uses media controls for audio and video assets', () => {
    expect(getAssetPreviewKind('audio', '/asset.mp3')).toBe('audio');
    expect(getAssetPreviewKind('video', '/asset.mp4')).toBe('video');
  });

  it('falls back to empty preview when there is no playable source', () => {
    expect(getAssetPreviewKind('scene', '')).toBe('empty');
    expect(
      getAssetPreviewSource({ coverUrl: '/cover.png', type: 'scene', url: '' }),
    ).toBe('/cover.png');
    expect(
      getAssetPreviewSource({ coverUrl: '/cover.png', type: 'video', url: '' }),
    ).toBe('');
  });

  it('adds token only to protected local upload assets', () => {
    expect(withPreviewToken('/api/upload-assets/1', 'abc 123')).toBe(
      '/api/upload-assets/1?token=abc%20123',
    );
    expect(withPreviewToken('/api/upload-assets/1?x=1', 'abc')).toBe(
      '/api/upload-assets/1?x=1&token=abc',
    );
    expect(withPreviewToken('https://cdn.example.com/a.mp4', 'abc')).toBe(
      'https://cdn.example.com/a.mp4',
    );
  });
});
