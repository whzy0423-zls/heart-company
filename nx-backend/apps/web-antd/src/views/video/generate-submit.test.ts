import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const currentDir = dirname(fileURLToPath(import.meta.url));

describe('video generation submission guard', () => {
  it('delegates submission validation to the video generation API', () => {
    const page = readFileSync(resolve(currentDir, 'generate.vue'), 'utf8');
    const submitHandler = page.slice(
      page.indexOf('async function generate()'),
      page.indexOf('async function refresh('),
    );

    expect(submitHandler).not.toContain('if (!prompt) {');
    expect(submitHandler).not.toContain('message.warning(');
    expect(submitHandler).toContain('await generateVideoApi({');
  });
});
