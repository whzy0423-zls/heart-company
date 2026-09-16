import { describe, expect, it } from 'vitest';

import { resolveFallbackRetryPath } from './fallback';

describe('resolveFallbackRetryPath', () => {
  it('uses the original route from encoded offline redirect query', () => {
    expect(
      resolveFallbackRetryPath(
        encodeURIComponent('/app/distribution?tab=agents'),
        '/dashboard/app',
      ),
    ).toBe('/app/distribution?tab=agents');
  });

  it('falls back to home path when redirect is missing or unsafe', () => {
    expect(resolveFallbackRetryPath(undefined, '/dashboard/app')).toBe(
      '/dashboard/app',
    );
    expect(resolveFallbackRetryPath('https://example.com', '/dashboard/app')).toBe(
      '/dashboard/app',
    );
    expect(resolveFallbackRetryPath('%', '/dashboard/app')).toBe(
      '/dashboard/app',
    );
  });
});
