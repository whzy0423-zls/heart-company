import type { VideoAssetType } from '#/api';

import { normalizeUploadAssetSource } from '#/utils/upload-asset-preview';

export type AssetPreviewKind = 'audio' | 'empty' | 'image' | 'video';

const imageAssetTypes = new Set<VideoAssetType>([
  'character',
  'outfit',
  'prop',
  'scene',
  'style',
]);

export function getAssetPreviewKind(
  type: VideoAssetType,
  source?: string,
): AssetPreviewKind {
  if (!source?.trim()) return 'empty';
  if (imageAssetTypes.has(type)) return 'image';
  if (type === 'audio') return 'audio';
  if (type === 'video') return 'video';
  return 'empty';
}

export function getAssetPreviewSource(asset: {
  coverUrl?: string;
  type: VideoAssetType;
  url?: string;
}) {
  const url = asset.url?.trim() || '';
  if (url) return url;
  return imageAssetTypes.has(asset.type) ? asset.coverUrl?.trim() || '' : '';
}

export function withPreviewToken(source: string, token?: null | string) {
  void token;
  return normalizeUploadAssetSource(source);
}

export function isImageAssetType(type: VideoAssetType) {
  return imageAssetTypes.has(type);
}
