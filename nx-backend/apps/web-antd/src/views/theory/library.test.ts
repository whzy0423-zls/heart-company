import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

describe('theory library management page contract', () => {
  it('exposes dashboard, cards and publishing controls', () => {
    const source = readFileSync(resolve(__dirname, 'library.vue'), 'utf8');
    const api = readFileSync(
      resolve(__dirname, '../../api/core/theory-library.ts'),
      'utf8',
    );
    expect(source).toContain('理论库管理');
    expect(source).toContain('生成并发布');
    expect(api).toContain("'/theory-libraries'");
    expect(api).toContain("/publish");
  });
});
