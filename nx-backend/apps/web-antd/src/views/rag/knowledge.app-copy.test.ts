import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

const source = readFileSync(
  resolve(dirname(fileURLToPath(import.meta.url)), 'knowledge.vue'),
  'utf8',
);

describe('rag knowledge app positioning copy', () => {
  it('positions managed knowledge for app chat rather than miniapp chat', () => {
    expect(source).toContain('App AI 对话');
    expect(source).not.toContain('小程序 RAG');
    expect(source).not.toContain('小程序问答');
  });
});
