import { describe, expect, it } from 'vitest';

import { safeDecodeURIComponent } from './safe-redirect';

describe('safeDecodeURIComponent', () => {
  it('decodes valid redirect values', () => {
    expect(safeDecodeURIComponent('%2Fmessage%2Fmanagement', '/')).toBe(
      '/message/management',
    );
  });

  it('falls back for malformed redirect values', () => {
    expect(safeDecodeURIComponent('%', '/dashboard')).toBe('/dashboard');
    expect(safeDecodeURIComponent('%E0%A4%A', '/dashboard')).toBe(
      '/dashboard',
    );
  });

  it('uses the first array value and falls back for empty values', () => {
    expect(safeDecodeURIComponent(['%2Fprofile'], '/dashboard')).toBe(
      '/profile',
    );
    expect(safeDecodeURIComponent('', '/dashboard')).toBe('/dashboard');
  });
});
