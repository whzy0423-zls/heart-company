import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

const source = readFileSync(
  resolve(process.cwd(), 'apps/web-antd/src/views/reading/articles.vue'),
  'utf8',
);

describe('reading article voice settings layout', () => {
  it('uses a theme-aware settings panel for the global voice controls', () => {
    expect(source).toContain('class="voice-panel"');
    expect(source).toContain('class="voice-panel-main"');
    expect(source).toContain('hsl(var(--card)');
    expect(source).toContain('hsl(var(--border)');
    expect(source).toContain('hsl(var(--muted-foreground)');
    expect(source).not.toContain('background: #f6f8fc;');
  });
});
