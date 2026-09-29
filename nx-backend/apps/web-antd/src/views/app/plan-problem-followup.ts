export function problemFollowupFlag(
  level: string | undefined,
  flags: Record<string, unknown> = {},
): boolean {
  return (
    level === 'svip' &&
    (!Object.hasOwn(flags, 'problemFollowup') || flags.problemFollowup === true)
  );
}

export function updateProblemFollowupJSON(
  value: string,
  enabled: boolean,
): null | string {
  try {
    const flags = JSON.parse(value || '{}');
    if (!flags || typeof flags !== 'object' || Array.isArray(flags))
      return null;
    return JSON.stringify({ ...flags, problemFollowup: enabled }, null, 2);
  } catch {
    return null;
  }
}
