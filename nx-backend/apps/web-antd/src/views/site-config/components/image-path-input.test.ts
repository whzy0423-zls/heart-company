import { describe, expect, it } from 'vitest';

import { getImagePreviewState, selectStoredImagePath } from './image-path-input';

describe('image path upload storage', () => {
  it('stores the protected upload asset url by default', () => {
    expect(
      selectStoredImagePath({
        objectUrl: '/api/uploads/site/logo.png',
        url: '/api/upload-assets/12',
      }),
    ).toBe('/api/upload-assets/12');
  });

  it('can opt in to storing the public object url when explicitly requested', () => {
    expect(
      selectStoredImagePath(
        {
          objectUrl: 'https://cdn.example.com/logo.png',
          url: '/api/upload-assets/12',
        },
        true,
      ),
    ).toBe('https://cdn.example.com/logo.png');
  });
});


describe('image preview display state', () => {
  it('shows a fallback message instead of an empty image src when a configured value has no preview url', () => {
    expect(
      getImagePreviewState({
        emptyText: '未设置图片',
        previewSrc: '',
        uploading: false,
        value: '/api/upload-assets/1',
      }),
    ).toEqual({ showImage: false, text: '预览加载失败，请重试上传或检查权限' });
  });

  it('keeps showing a safe preview url when it is available', () => {
    expect(
      getImagePreviewState({
        emptyText: '未设置图片',
        previewSrc: 'https://cdn.example.com/logo.png',
        uploading: false,
        value: 'https://cdn.example.com/logo.png',
      }),
    ).toEqual({ showImage: true, text: '' });
  });

  it('shows a fallback message when the browser fails to load a configured preview image', () => {
    expect(
      getImagePreviewState({
        emptyText: '未设置图片',
        imageError: true,
        previewSrc: 'https://cdn.example.com/missing-logo.png',
        uploading: false,
        value: 'https://cdn.example.com/missing-logo.png',
      }),
    ).toEqual({ showImage: false, text: '预览加载失败，请重试上传或检查权限' });
  });
});
