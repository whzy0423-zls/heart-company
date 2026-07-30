import type { ClassroomContent, ClassroomSeries } from '#/api/core/classroom';

export function classroomPermissions(accessCodes: string[]) {
  const has = (code: string) => accessCodes.includes(code);
  return {
    canPrice: has('Miniapp:Classroom:Price'),
    canPublish: has('Miniapp:Classroom:Publish'),
    canUpload: has('Miniapp:Classroom:Upload'),
    canWrite: has('Miniapp:Classroom:Write'),
  };
}

export function contentPublishGuard(
  content: Pick<ClassroomContent, 'seriesId' | 'status'>,
  series: Array<Pick<ClassroomSeries, 'id' | 'status' | 'title'>>,
) {
  const republishing = content.status === 'offline';
  if (content.status !== 'ready' && !republishing)
    return {
      allowed: false,
      label: '等待媒体处理',
      reason: '媒体处理完成后才可发布',
    };
  if (!content.seriesId)
    return republishing
      ? { allowed: true, label: '重新发布', reason: '重新发布已下线课件' }
      : { allowed: true, label: '发布', reason: '发布课件' };
  const parent = series.find((item) => item.id === content.seriesId);
  if (!parent)
    return {
      allowed: false,
      label: '系列数据未加载',
      reason: '请刷新课程系列数据后再发布',
    };
  if (parent.status !== 'published')
    return {
      allowed: false,
      label: '先发布所属系列',
      reason: `请先到“课程系列”发布《${parent.title}》`,
    };
  return republishing
    ? { allowed: true, label: '重新发布', reason: '重新发布已下线课件' }
    : { allowed: true, label: '发布', reason: '发布课件' };
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
  if (taskStatus === 'completing') return '媒体处理中';
  if (taskStatus === 'completed') return '上传完成';
  if (taskStatus === 'failed') return '失败';
  if (taskStatus === 'expired') return '已过期';
  if (taskStatus === 'aborted') return '已终止';
  return taskStatus;
}
