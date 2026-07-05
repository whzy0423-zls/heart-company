export interface ImagePathUploadResult {
  objectUrl?: string;
  url?: string;
}

export interface ImagePreviewStateInput {
  emptyText: string;
  imageError?: boolean;
  previewSrc?: string;
  uploading: boolean;
  value?: string;
}

export function selectStoredImagePath(
  result: ImagePathUploadResult,
  storeObjectUrl = false,
) {
  return storeObjectUrl ? result.objectUrl || result.url || '' : result.url || '';
}

export function getImagePreviewState(input: ImagePreviewStateInput) {
  const hasValue = !!input.value?.trim();
  const hasPreview = !!input.previewSrc?.trim();

  if (input.imageError && hasValue) {
    return {
      showImage: false,
      text: '预览加载失败，请重试上传或检查权限',
    };
  }

  if (hasPreview) {
    return { showImage: true, text: '' };
  }

  if (input.uploading) {
    return { showImage: false, text: '上传中...' };
  }

  if (hasValue) {
    return {
      showImage: false,
      text: '预览加载失败，请重试上传或检查权限',
    };
  }

  return { showImage: false, text: input.emptyText };
}
