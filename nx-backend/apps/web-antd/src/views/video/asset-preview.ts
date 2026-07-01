import type { VideoAssetType } from '#/api';

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
  const cleanSource = source.trim();
  if (
    !cleanSource ||
    !token ||
    !cleanSource.startsWith('/api/upload-assets/')
  ) {
    return cleanSource;
  }
  const separator = cleanSource.includes('?') ? '&' : '?';
  return `${cleanSource}${separator}token=${encodeURIComponent(token)}`;
}

export function isImageAssetType(type: VideoAssetType) {
  return imageAssetTypes.has(type);
}
