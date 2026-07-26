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
