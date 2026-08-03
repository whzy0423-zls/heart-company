import { describe, expect, it } from 'vitest';

import { canvasRoutePaths } from './router-config';

describe('Infinite Canvas routes', () => {
  it('contains only the canvas library and editor routes', () => {
    expect(canvasRoutePaths).toEqual(['/canvas', '/canvas/:id']);
  });
});
