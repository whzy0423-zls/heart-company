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
