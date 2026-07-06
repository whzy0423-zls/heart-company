import fs from 'node:fs';
import path from 'node:path';

import { describe, expect, it } from 'vitest';

const file = path.resolve(__dirname, 'user-insights.vue');
const source = fs.readFileSync(file, 'utf8');

describe('user insights card viewer entry', () => {
  it('exposes a fortune card viewer backed by quiz cards API', () => {
    expect(source).toContain('getQuizCardsApi');
    expect(source).toContain('命运卡片');
    expect(source).toContain('openCards');
  });

  it('clears stale card data and handles load errors', () => {
    expect(source).toContain('cards.value = []');
    expect(source).toContain('命运卡片加载失败');
    expect(source).toContain('catch');
  });
});
