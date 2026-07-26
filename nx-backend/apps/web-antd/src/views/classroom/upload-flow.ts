import type { ClassroomUploadTask } from '#/api/core/classroom';

export interface UploadRetryContext {
  contentId: number;
  file: File;
}

export function resolveUploadRetryContext(
  task: ClassroomUploadTask,
  contexts: Map<number, UploadRetryContext>,
  selectedFile?: File,
): UploadRetryContext | undefined {
  return (
    contexts.get(task.id) ??
    (selectedFile
      ? { contentId: task.contentId, file: selectedFile }
      : undefined)
  );
}

export function classroomUploadMime(file: Pick<File, 'name' | 'type'>) {
  if (file.type) return file.type;
  const name = file.name.toLowerCase();
  if (name.endsWith('.mp4')) return 'video/mp4';
  if (name.endsWith('.mp3')) return 'audio/mpeg';
  if (name.endsWith('.m4a')) return 'audio/x-m4a';
  return '';
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
