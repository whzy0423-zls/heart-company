import fs from 'node:fs';
import path from 'node:path';

import { describe, expect, it } from 'vitest';

const source = fs.readFileSync(path.resolve(__dirname, 'knowledge.vue'), 'utf8');

describe('rag knowledge reindex entry', () => {
  it('has a one-click reindex action', () => {
    expect(source).toContain('reindexRAGDocumentsApi');
    expect(source).toContain('重建索引');
    expect(source).toContain('handleReindex');
  });
});
