import { describe, expect, it, vi } from 'vitest';

import { applyAdminBranding } from './branding';
import { startAdminBrandingSync } from './branding-startup';

vi.mock('./branding', () => ({
  applyAdminBranding: vi.fn(),
}));

describe('admin branding startup sync', () => {
  it('starts branding sync without returning the pending branding request', () => {
    const pending = new Promise<void>(() => {});
    vi.mocked(applyAdminBranding).mockReturnValue(pending);

    const result = startAdminBrandingSync();

    expect(result).toBeUndefined();
    expect(applyAdminBranding).toHaveBeenCalledOnce();
  });
});
