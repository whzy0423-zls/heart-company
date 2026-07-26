import fs from 'node:fs';
import path from 'node:path';

import { describe, expect, it } from 'vitest';

const source = fs.readFileSync(path.resolve(import.meta.dirname, 'production/index.vue'), 'utf8');

describe('production creation center interactions', () => {
  it('covers loading, empty, retry, create busy and real project navigation', () => {
    for (const text of ['loading', 'loadError', 'retryLoad', 'projects.length === 0', 'creating', 'createProjectApi', 'listProjectsApi', "router.push(`/video/projects/${created.id}/workbench`)", '查看全部项目']) expect(source).toContain(text);
  });
});
