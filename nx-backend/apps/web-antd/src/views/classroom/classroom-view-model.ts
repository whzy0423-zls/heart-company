export function classroomPermissions(accessCodes: string[]) {
  const has = (code: string) => accessCodes.includes(code);
  return {
    canPrice: has('Miniapp:Classroom:Price'),
    canPublish: has('Miniapp:Classroom:Publish'),
    canUpload: has('Miniapp:Classroom:Upload'),
    canWrite: has('Miniapp:Classroom:Write'),
  };
}

export function playbackControl(blocked: boolean) {
  return blocked
    ? { action: 'unblock' as const, label: '恢复播放' }
    : { action: 'block' as const, label: '阻断播放' };
}

export function classroomOperationError(cause: unknown, fallback: string) {
  return cause instanceof Error && cause.message ? cause.message : fallback;
}

export function visibleClassroomTabs(canUpload: boolean) {
  return canUpload ? ['contents', 'series', 'uploads'] : ['contents', 'series'];
}

export function uploadStatusLabel(taskStatus: string, contentStatus?: string) {
  if (contentStatus === 'processing') return '媒体处理中';
  if (contentStatus === 'ready') return '可发布';
  if (contentStatus === 'published') return '已发布';
  if (taskStatus === 'initiated' || taskStatus === 'initiating')
    return '等待上传';
  if (taskStatus === 'uploading') return '上传中';
  if (taskStatus === 'completing') return '正在合并';
  if (taskStatus === 'completed') return '上传完成';
  if (taskStatus === 'failed') return '失败';
  if (taskStatus === 'expired') return '已过期';
  if (taskStatus === 'aborted') return '已终止';
  return taskStatus;
}
