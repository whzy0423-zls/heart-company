export function createWorkflowNavigationController() {
  let dirty = false;
  let pendingAction: null | (() => Promise<void> | void) = null;

  const runPending = async () => {
    const action = pendingAction;
    pendingAction = null;
    if (action) await action();
  };

  return {
    cancel() {
      pendingAction = null;
    },
    async discardAndContinue(reload: () => Promise<void>) {
      await reload();
      dirty = false;
      await runPending();
    },
    isDirty: () => dirty,
    pending: () => pendingAction !== null,
    async request(action: () => Promise<void> | void) {
      if (!dirty) {
        await action();
        return;
      }
      pendingAction = action;
    },
    async saveAndContinue(save: () => Promise<void>) {
      await save();
      dirty = false;
      await runPending();
    },
    setDirty(value: boolean) {
      dirty = value;
    },
  };
}
