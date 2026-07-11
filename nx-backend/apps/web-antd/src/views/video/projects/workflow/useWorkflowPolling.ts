export function createWorkflowPollingController(options: { delay: number; maxAttempts: number }) {
  const timers = new Map<string, ReturnType<typeof setTimeout>>();

  function stop(key: string) {
    const timer = timers.get(key);
    if (timer) clearTimeout(timer);
    timers.delete(key);
  }

  function start(
    key: string,
    poll: () => Promise<string>,
    onTerminal: (status: string) => void,
    onTimeout: () => void = () => undefined,
  ) {
    stop(key);
    let attempts = 0;
    const tick = async () => {
      attempts += 1;
      const status = await poll();
      if (['cancelled', 'completed', 'failed', 'unknown_outcome'].includes(status)) {
        stop(key);
        onTerminal(status);
        return;
      }
      if (attempts >= options.maxAttempts) {
        stop(key);
        onTimeout();
        return;
      }
      timers.set(key, setTimeout(tick, options.delay));
    };
    timers.set(key, setTimeout(tick, 0));
  }

  return {
    start,
    stop,
    stopAll() {
      for (const key of [...timers.keys()]) stop(key);
    },
  };
}
