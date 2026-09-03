import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const here = dirname(fileURLToPath(import.meta.url));

describe('enneagram library page contract', () => {
  it('keeps all nine types in one page and exposes lifecycle actions', () => {
    const source = readFileSync(resolve(here, 'enneagram-library.vue'), 'utf8');
    expect(source).toContain('v-for="type in 9"');
    expect(source).toContain('编辑草稿');
    expect(source).toContain('提交审核');
    expect(source).toContain('审核通过');
    expect(source).toContain('发布');
    expect(source).toContain('回滚');
    expect(source).toContain('sourcePages');
  });
});
