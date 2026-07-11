export const workflowSteps = ['brief', 'assets', 'storyboard', 'generate', 'export'] as const;

export type WorkflowStepKey = (typeof workflowSteps)[number];
export type ShotReadiness =
  | 'completed'
  | 'failed'
  | 'generating'
  | 'incomplete'
  | 'ready'
  | 'recovery'
  | 'stale';
export type ReadinessFilter = 'completed' | 'generatable' | 'generating' | 'incomplete';

export function splitScriptIntoShots(script: string) {
  return script
    .replaceAll('\r\n', '\n')
    .replaceAll('\r', '\n')
    .split(/\n\s*\n+/)
    .map((content) => content.replace(/\s+/g, ' ').trim())
    .filter(Boolean)
    .map((content, index) => ({ content, index }));
}

export function normalizeWorkflowStep(
  candidate: null | string | undefined,
  fallback: WorkflowStepKey,
): WorkflowStepKey {
  return workflowSteps.includes(candidate as WorkflowStepKey)
    ? (candidate as WorkflowStepKey)
    : fallback;
}

export function recommendedStepForLegacy(input: {
  finalVideoUrl: string;
  scriptContent: string;
  shots: Array<{ readiness: ShotReadiness }>;
}): WorkflowStepKey {
  if (input.shots.length === 0) return 'brief';
  if (input.shots.some(({ readiness }) => readiness === 'incomplete')) return 'storyboard';
  if (input.shots.some(({ readiness }) => readiness !== 'completed')) return 'generate';
  return 'export';
}

export function readinessFilter(readiness: ShotReadiness): ReadinessFilter {
  if (readiness === 'ready' || readiness === 'stale' || readiness === 'failed') return 'generatable';
  if (readiness === 'incomplete' || readiness === 'recovery') return 'incomplete';
  if (readiness === 'generating') return 'generating';
  return 'completed';
}

export function groupGeneratableShots(shots: Array<{ readiness: ShotReadiness }>) {
  const result = { failed: 0, ready: 0, stale: 0, total: 0 };
  for (const shot of shots) {
    if (shot.readiness === 'ready' || shot.readiness === 'stale' || shot.readiness === 'failed') {
      result[shot.readiness] += 1;
      result.total += 1;
    }
  }
  return result;
}

export function composeResultLabel(result: { isCurrent: boolean; videoUrl: string }) {
  if (!result.videoUrl) return '尚未合成';
  return result.isCurrent ? '当前成片' : '内容已变化，需要重新合成';
}
