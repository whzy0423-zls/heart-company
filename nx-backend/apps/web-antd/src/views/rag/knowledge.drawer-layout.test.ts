import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

const source = readFileSync(
  resolve(process.cwd(), 'apps/web-antd/src/views/rag/knowledge.vue'),
  'utf8',
);

describe('RAG knowledge drawer layout', () => {
  it('keeps submit actions in the drawer footer', () => {
    expect(source).toContain('<template #footer>');
    expect(source).toContain('class="drawer-footer"');

    const formBody = source.slice(
      source.indexOf('<Form layout="vertical">'),
      source.indexOf('</Form>'),
    );
    expect(formBody).not.toContain('type="primary" @click="submit"');
  });
});
