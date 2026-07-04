import { applyAdminBranding } from './branding';

/**
 * Fire-and-forget startup branding sync.
 *
 * Branding is cosmetic and must not block the admin app from mounting when the
 * API is slow, unavailable, or being restarted locally.
 */
export function startAdminBrandingSync() {
  void applyAdminBranding().catch(() => {
    // applyAdminBranding already falls back to build-time defaults.
  });
}
