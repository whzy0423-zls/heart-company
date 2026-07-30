import type {
  ClassroomContentType,
  ClassroomUploadMimeType,
  ClassroomUploadStatus,
  ClassroomUploadTask,
} from '#/api/core/classroom';

export function canAbortUploadTask(status: ClassroomUploadStatus) {
  return !['aborted', 'completed', 'completing'].includes(status);
}

export async function completeUploadWithStatusReconciliation(args: {
  complete: () => Promise<unknown>;
  readTask: () => Promise<Pick<ClassroomUploadTask, 'status'> | undefined>;
}) {
  try {
    await args.complete();
    return 'completed' as const;
  } catch (cause) {
    let task: Pick<ClassroomUploadTask, 'status'> | undefined;
    try {
      task = await args.readTask();
    } catch {
      // Preserve the original completion error when reconciliation is unavailable.
    }
    if (task?.status === 'completed') return 'completed' as const;
    if (task?.status === 'completing') return 'processing' as const;
    throw cause;
  }
}

export interface UploadRetryContext {
  checksum?: string;
  contentId: number;
  file: File;
  filename?: string;
  size?: number;
}

export function resolveUploadRetryContext(
  task: ClassroomUploadTask,
  contexts: Map<number, UploadRetryContext>,
): UploadRetryContext | undefined {
  return contexts.get(task.id);
}

export function shouldAbortController(
  activeTaskId: number | undefined,
  taskId: number,
) {
  return activeTaskId === taskId;
}

export interface UploadIdentity {
  checksum: string;
  contentId: number;
  filename: string;
  size: number;
}

export function matchesUploadIdentity(
  identity: UploadIdentity,
  candidate: Pick<File, 'name' | 'size'>,
  contentId: number,
  checksum?: string,
) {
  return (
    identity.contentId === contentId &&
    identity.filename === candidate.name &&
    identity.size === candidate.size &&
    (!checksum || identity.checksum === checksum)
  );
}

export function mergeUploadProgress(
  local: { completedBytes: number; totalBytes: number },
  task: {
    completedBytes?: number;
    expectedSize: number;
    totalBytes?: number;
  },
) {
  return {
    completedBytes: Math.max(local.completedBytes, task.completedBytes ?? 0),
    totalBytes: task.totalBytes || task.expectedSize || local.totalBytes,
  };
}

export function classroomUploadMime(file: Pick<File, 'name' | 'type'>) {
  if (file.type) return file.type;
  const name = file.name.toLowerCase();
  if (name.endsWith('.mp4')) return 'video/mp4';
  if (name.endsWith('.mp3')) return 'audio/mpeg';
  if (name.endsWith('.m4a')) return 'audio/x-m4a';
  return '';
}

export function matchesClassroomContentType(
  contentType: ClassroomContentType,
  mime: ClassroomUploadMimeType,
) {
  return contentType === 'video'
    ? mime === 'video/mp4'
    : mime.startsWith('audio/');
}

export async function putSignedUploadPart(
  url: string,
  body: Blob,
  signal: AbortSignal,
  fetcher: typeof fetch = fetch,
) {
  const response = await fetcher(url, { body, method: 'PUT', signal });
  if (!response.ok) throw new Error(`分片上传失败（HTTP ${response.status}）`);
  const etag = response.headers.get('ETag');
  if (!etag)
    throw new Error('OSS CORS 未暴露 ETag 响应头，请检查 Expose-Headers 配置');
  return etag.replaceAll('"', '');
}
