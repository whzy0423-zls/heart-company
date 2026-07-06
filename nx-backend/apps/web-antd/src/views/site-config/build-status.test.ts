import fs from 'node:fs';
import path from 'node:path';

import { describe, expect, it } from 'vitest';

const source = fs.readFileSync(path.resolve(__dirname, 'overview.vue'), 'utf8');

describe('website build status overview', () => {
  it('shows build status from backend', () => {
    expect(source).toContain('getSiteBuildStatusApi');
    expect(source).toContain('构建状态');
    expect(source).toContain('refreshBuildStatus');
  });

  it('recognizes backend pending/building states and displays load errors', () => {
    expect(source).toContain("state === 'pending'");
    expect(source).toContain("state === 'building'");
    expect(source).toContain('构建状态加载失败');
  });
});
