import { describe, expect, it } from 'vitest';

import {
  problemFollowupFlag,
  updateProblemFollowupJSON,
} from './plan-problem-followup';

describe('problem followup tier policy', () => {
  it.each(['free', 'vip', undefined])(
    'never grants %s a stale true flag',
    (level) => {
      expect(problemFollowupFlag(level, { problemFollowup: true })).toBe(false);
    },
  );

  it('defaults SVIP to enabled and honors explicit closure', () => {
    expect(problemFollowupFlag('svip', {})).toBe(true);
    expect(problemFollowupFlag('svip', { problemFollowup: true })).toBe(true);
    expect(problemFollowupFlag('svip', { problemFollowup: false })).toBe(false);
    expect(problemFollowupFlag('svip', { problemFollowup: 'true' })).toBe(
      false,
    );
  });

  it('updates only the followup switch without losing configured flags', () => {
    const updated = updateProblemFollowupJSON(
      '{"xinzhili":false,"deepChat":true}',
      false,
    );
    expect(JSON.parse(updated!)).toEqual({
      deepChat: true,
      problemFollowup: false,
      xinzhili: false,
    });
  });

  it.each(['{broken', 'null', '[]', 'true'])(
    'preserves invalid input %s for correction',
    (value) => {
      expect(updateProblemFollowupJSON(value, true)).toBeNull();
    },
  );
});
