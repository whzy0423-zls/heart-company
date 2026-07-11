import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }));

vi.mock('#/api/request', () => ({
  requestClient: { get: mocks.get, post: mocks.post },
}));

describe('guided video workflow api', () => {
  beforeEach(() => {
    mocks.get.mockReset();
    mocks.post.mockReset();
  });

  it('types safe active submission recovery metadata', () => {
    const source = readFileSync(resolve(import.meta.dirname, 'videoproject.ts'), 'utf8');
    expect(source).toContain('activeSubmission?: WorkflowActiveSubmission');
    expect(source).toContain('submissionId: number');
    expect(source).toContain('requestKey: string');
    expect(source).not.toContain('requestSnapshot:');
  });

  it('uses exact workflow endpoint contracts', async () => {
    const api = await import('./videoproject');
    const items = [{ requestKey: 'request-1', shotId: '2' }];

    await api.getProjectWorkflowApi('1');
    await api.getGenerationSubmissionApi('9');
    await api.createShotsFromScriptApi('1', { items: [{ content: 'shot', index: 0 }], scriptRevision: 3 });
    await api.generateShotSafeApi('2', { requestKey: 'request-1' });
    await api.batchGenerateShotsSafeApi('1', { items });
    await api.reconcileGenerationSubmissionApi('request-1', { taskId: 'task-1' });
    await api.setShotVideoVersionApi('2', '8');
    await api.composeProjectSafeApi('1', { transition: 'fade' });
    await api.getComposeJobApi('1', '7');

    expect(mocks.get).toHaveBeenCalledWith('/video/projects-workflow/1');
    expect(mocks.get).toHaveBeenCalledWith('/video/generation-submissions/9');
    expect(mocks.post).toHaveBeenCalledWith('/video/projects-shots/from-script/1', { items: [{ content: 'shot', index: 0 }], scriptRevision: 3 });
    expect(mocks.post).toHaveBeenCalledWith('/video/shots-generate-safe/2', { requestKey: 'request-1' });
    expect(mocks.post).toHaveBeenCalledWith('/video/projects-batch-generate-safe/1', { items }, { timeout: 180_000 });
    expect(mocks.post).toHaveBeenCalledWith('/video/generation-submissions/reconcile/request-1', { taskId: 'task-1' });
    expect(mocks.post).toHaveBeenCalledWith('/video/shots-video-versions/set/2/8');
    expect(mocks.post).toHaveBeenCalledWith('/video/projects-compose-safe/1', { transition: 'fade' });
    expect(mocks.get).toHaveBeenCalledWith('/video/projects-compose-safe-status/1/7');
  });
});
