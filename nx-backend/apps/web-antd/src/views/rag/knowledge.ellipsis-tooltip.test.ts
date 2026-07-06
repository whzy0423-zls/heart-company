import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';

const source = readFileSync('apps/web-antd/src/views/rag/knowledge.vue', 'utf8');

describe('RAG knowledge ellipsis tooltip', () => {
  it('wraps content preview with the shared ellipsis tooltip', () => {
    expect(source).toContain("import EllipsisTooltip from '#/components/ellipsis-tooltip/ellipsis-tooltip.vue'");
    expect(source).toContain('v-else-if="column.dataIndex === \'content\'"');
    expect(source).toContain('<EllipsisTooltip');
    expect(source).toContain(':text="record.content"');
    expect(source).toContain(':lines="2"');
  });
});
