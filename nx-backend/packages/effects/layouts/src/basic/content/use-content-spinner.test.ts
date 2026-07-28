import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  afterEachHandlers: [] as Array<(to: any) => unknown>,
  beforeEachHandlers: [] as Array<(to: any) => unknown>,
  preferences: {
    transition: {
      loading: true,
    },
  },
}));

vi.mock('vue-router', () => ({
  useRouter: () => ({
    afterEach: (handler: (to: any) => unknown) => {
      mocks.afterEachHandlers.push(handler);
    },
    beforeEach: (handler: (to: any) => unknown) => {
      mocks.beforeEachHandlers.push(handler);
    },
  }),
}));

vi.mock('@vben/preferences', () => ({
  preferences: mocks.preferences,
}));

import { useContentSpinner } from './use-content-spinner';

describe('useContentSpinner', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.spyOn(performance, 'now').mockReturnValue(1000);
    mocks.beforeEachHandlers.length = 0;
    mocks.afterEachHandlers.length = 0;
    mocks.preferences.transition.loading = true;
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  it('shows the content spinner for every non-iframe route transition, including cached routes', async () => {
    const { spinning } = useContentSpinner();

    expect(mocks.beforeEachHandlers).toHaveLength(1);
    expect(mocks.afterEachHandlers).toHaveLength(1);

    mocks.beforeEachHandlers[0]?.({ meta: { loaded: true } });
    expect(spinning.value).toBe(true);

    mocks.afterEachHandlers[0]?.({ meta: { loaded: true } });
    expect(spinning.value).toBe(true);

    await vi.advanceTimersByTimeAsync(500);
    expect(spinning.value).toBe(false);
  });

  it('does not show the content spinner for iframe routes', () => {
    const { spinning } = useContentSpinner();

    mocks.beforeEachHandlers[0]?.({ meta: { iframeSrc: '/frame' } });

    expect(spinning.value).toBe(false);
  });
});
