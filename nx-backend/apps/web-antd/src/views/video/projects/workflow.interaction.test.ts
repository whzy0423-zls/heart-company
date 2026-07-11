import { describe, expect, it, vi } from 'vitest';

import { createWorkflowNavigationController } from './workflow/useWorkflowNavigation';

describe('workflow dirty navigation controller', () => {
  it('runs clean navigation immediately', async () => {
    const navigate = vi.fn();
    const controller = createWorkflowNavigationController();
    await controller.request(navigate);
    expect(navigate).toHaveBeenCalledOnce();
    expect(controller.pending()).toBe(false);
  });

  it('queues dirty navigation and runs it exactly once after save', async () => {
    const navigate = vi.fn();
    const save = vi.fn().mockResolvedValue(undefined);
    const controller = createWorkflowNavigationController();
    controller.setDirty(true);
    await controller.request(navigate);
    expect(navigate).not.toHaveBeenCalled();
    expect(controller.pending()).toBe(true);
    await controller.saveAndContinue(save);
    expect(save).toHaveBeenCalledOnce();
    expect(navigate).toHaveBeenCalledOnce();
    expect(controller.isDirty()).toBe(false);
  });

  it('retains dirty state and selection when save fails', async () => {
    const navigate = vi.fn();
    const controller = createWorkflowNavigationController();
    controller.setDirty(true);
    await controller.request(navigate);
    await expect(controller.saveAndContinue(vi.fn().mockRejectedValue(new Error('invalid')))).rejects.toThrow('invalid');
    expect(controller.isDirty()).toBe(true);
    expect(controller.pending()).toBe(true);
    expect(navigate).not.toHaveBeenCalled();
  });

  it('supports discard and cancel', async () => {
    const navigate = vi.fn();
    const reload = vi.fn().mockResolvedValue(undefined);
    const controller = createWorkflowNavigationController();
    controller.setDirty(true);
    await controller.request(navigate);
    controller.cancel();
    expect(controller.pending()).toBe(false);
    await controller.request(navigate);
    await controller.discardAndContinue(reload);
    expect(reload).toHaveBeenCalledOnce();
    expect(navigate).toHaveBeenCalledOnce();
  });
});
