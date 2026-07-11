import { describe, expect, it } from 'vitest';

import {
  composeResultLabel,
  groupGeneratableShots,
  normalizeWorkflowStep,
  readinessFilter,
  recommendedStepForLegacy,
  splitScriptIntoShots,
} from './workflow';

describe('guided workflow helpers', () => {
  it('splits normalized paragraphs without empty shots', () => {
    expect(splitScriptIntoShots(' 第一镜 \r\n\r\n 第二镜\n\n\n第三镜 ')).toEqual([
      { content: '第一镜', index: 0 },
      { content: '第二镜', index: 1 },
      { content: '第三镜', index: 2 },
    ]);
  });

  it('validates workflow step query values', () => {
    expect(normalizeWorkflowStep('generate', 'brief')).toBe('generate');
    expect(normalizeWorkflowStep('unknown', 'storyboard')).toBe('storyboard');
    expect(normalizeWorkflowStep(undefined, 'assets')).toBe('assets');
  });

  it('recommends a useful step for legacy projects', () => {
    expect(recommendedStepForLegacy({ finalVideoUrl: '', scriptContent: '', shots: [] })).toBe('brief');
    expect(recommendedStepForLegacy({ finalVideoUrl: '', scriptContent: '', shots: [{ readiness: 'incomplete' }] })).toBe('storyboard');
    expect(recommendedStepForLegacy({ finalVideoUrl: '', scriptContent: '', shots: [{ readiness: 'ready' }] })).toBe('generate');
    expect(recommendedStepForLegacy({ finalVideoUrl: '', scriptContent: '', shots: [{ readiness: 'completed' }] })).toBe('export');
  });

  it('maps technical readiness to four user filters', () => {
    expect(readinessFilter('ready')).toBe('generatable');
    expect(readinessFilter('stale')).toBe('generatable');
    expect(readinessFilter('failed')).toBe('generatable');
    expect(readinessFilter('incomplete')).toBe('incomplete');
    expect(readinessFilter('recovery')).toBe('incomplete');
    expect(readinessFilter('generating')).toBe('generating');
    expect(readinessFilter('completed')).toBe('completed');
  });

  it('groups confirmation counts without expanding server scope', () => {
    expect(groupGeneratableShots([
      { readiness: 'ready' }, { readiness: 'ready' }, { readiness: 'stale' },
      { readiness: 'failed' }, { readiness: 'completed' },
    ])).toEqual({ failed: 1, ready: 2, stale: 1, total: 4 });
  });

  it('labels current and stale compose results', () => {
    expect(composeResultLabel({ isCurrent: true, videoUrl: 'url' })).toBe('当前成片');
    expect(composeResultLabel({ isCurrent: false, videoUrl: 'url' })).toBe('内容已变化，需要重新合成');
    expect(composeResultLabel({ isCurrent: false, videoUrl: '' })).toBe('尚未合成');
  });
});
