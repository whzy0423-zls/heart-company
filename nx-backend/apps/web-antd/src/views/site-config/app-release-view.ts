import type { AppRelease } from '#/api';

export const MAX_APK_BYTES = 300 * 1024 * 1024;
export function validateAPKFile(file: Pick<File, 'name' | 'size'>) {
  if (!file.name.toLowerCase().endsWith('.apk')) return '只能上传 APK 文件';
  if (file.size > MAX_APK_BYTES) return '安装包不能超过 300 MiB';
  return null;
}
export function formatReleaseFileSize(bytes: number) { return `${(bytes / 1024 / 1024).toFixed(1)} MiB`; }
export function canPublishRelease(r: AppRelease) { return r.fileAvailable && r.status !== 'published'; }
export function canArchiveRelease(r: AppRelease) { return r.status === 'published'; }
export function releaseStatusLabel(status: AppRelease['status']) { return { archived: '已下架', draft: '草稿', published: '已发布' }[status]; }
