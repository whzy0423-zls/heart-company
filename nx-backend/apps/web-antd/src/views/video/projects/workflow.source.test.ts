import fs from 'node:fs';
import path from 'node:path';

import { describe, expect, it } from 'vitest';

const root = path.resolve(import.meta.dirname);
const read = (file: string) => fs.readFileSync(path.join(root, file), 'utf8');

describe('guided workflow source contracts', () => {
  it('mounts the guided workflow by default and preserves hidden advanced route', () => {
    const routes = read('../../../router/routes/modules/video.ts');
    expect(routes).toContain("import('#/views/video/projects/workflow.vue')");
    expect(routes).toContain("path: 'projects/:id/workbench/advanced'");
    expect(routes).toContain("import('#/views/video/projects/workbench.vue')");
    expect(routes).not.toContain("path: 'production/short'");
  });

  it('renders five real steps and one primary action shell', () => {
    const shell = read('workflow.vue');
    const stepper = read('workflow/WorkflowStepper.vue');
    for (const label of ['项目与剧本', '角色与场景', '分镜', '生成', '导出']) {
      expect(stepper).toContain(label);
    }
    for (const step of ['brief', 'assets', 'storyboard', 'generate', 'export']) {
      expect(stepper).toContain(`'${step}'`);
    }
    expect(stepper).toContain('aria-current');
    expect(shell).toContain('aria-live="polite"');
    expect(shell).toContain('data-primary-action');
    expect(shell.match(/data-primary-action/g)).toHaveLength(1);
    expect(shell).toContain("router.replace({ query: { ...route.query, step } })");
    expect(shell).toContain('/workbench/advanced');
    expect(shell).not.toContain('workbenchMode');
  });

  it('provides loading, fatal retry, safe footer and reduced motion', () => {
    const shell = read('workflow.vue');
    expect(shell).toContain('workflow-loading');
    expect(shell).toContain('workflow-fatal');
    expect(shell).toContain('retryLoad');
    expect(shell).toContain('min-height: 44px');
    expect(shell).toContain('env(safe-area-inset-bottom)');
    expect(shell).toContain('@media (prefers-reduced-motion: reduce)');
  });
});
