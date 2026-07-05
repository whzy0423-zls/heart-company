import { beforeEach, describe, expect, it, vi } from 'vitest';

import { safeDecodeURIComponent } from './safe-redirect';

const mocks = vi.hoisted(() => ({
  accessStore: {
    accessToken: 'token',
    isAccessChecked: false,
    setAccessMenus: vi.fn(),
    setAccessRoutes: vi.fn(),
    setIsAccessChecked: vi.fn(),
  },
  authStore: {
    fetchUserInfo: vi.fn(),
  },
  generateAccess: vi.fn(),
  stopProgress: vi.fn(),
  userStore: {
    userInfo: null as any,
  },
}));

vi.mock('@vben/constants', () => ({ LOGIN_PATH: '/auth/login' }));
vi.mock('@vben/preferences', () => ({
  preferences: {
    app: { defaultHomePath: '/dashboard/app' },
    transition: { progress: true },
  },
}));
vi.mock('@vben/stores', () => ({
  useAccessStore: () => mocks.accessStore,
  useUserStore: () => mocks.userStore,
}));
vi.mock('@vben/utils', () => ({
  startProgress: vi.fn(),
  stopProgress: mocks.stopProgress,
}));
vi.mock('#/router/routes', () => ({
  accessRoutes: [],
  coreRouteNames: ['Login'],
}));
vi.mock('#/store', () => ({
  useAuthStore: () => mocks.authStore,
}));
vi.mock('./access', () => ({
  generateAccess: mocks.generateAccess,
}));

import { createRouterGuard } from './guard';

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

describe('access guard backend failure fallback', () => {
  beforeEach(() => {
    mocks.accessStore.accessToken = 'token';
    mocks.accessStore.isAccessChecked = false;
    mocks.accessStore.setAccessMenus.mockReset();
    mocks.accessStore.setAccessRoutes.mockReset();
    mocks.accessStore.setIsAccessChecked.mockReset();
    mocks.authStore.fetchUserInfo.mockReset();
    mocks.generateAccess.mockReset();
    mocks.stopProgress.mockReset();
    mocks.userStore.userInfo = null;
  });

  function setup() {
    const beforeEachHandlers: any[] = [];
    const afterEachHandlers: any[] = [];
    const router = {
      afterEach: vi.fn((handler) => afterEachHandlers.push(handler)),
      beforeEach: vi.fn((handler) => beforeEachHandlers.push(handler)),
      resolve: vi.fn((path) => ({ path })),
    };
    createRouterGuard(router as any);
    return { accessGuard: beforeEachHandlers[1] };
  }

  it('redirects to offline fallback without marking access checked when fetching user info fails', async () => {
    const { accessGuard } = setup();
    mocks.authStore.fetchUserInfo.mockRejectedValueOnce(new Error('api down'));

    const result = await accessGuard(
      { fullPath: '/dashboard/app', meta: {}, name: 'Dashboard', path: '/dashboard/app' },
      { query: {} },
    );

    expect(result).toEqual({ path: '/offline', replace: true });
    expect(mocks.accessStore.setIsAccessChecked).not.toHaveBeenCalledWith(true);
    expect(mocks.stopProgress).toHaveBeenCalled();
  });

  it('redirects to offline fallback and keeps access unchecked when menu generation fails', async () => {
    const { accessGuard } = setup();
    mocks.authStore.fetchUserInfo.mockResolvedValueOnce({ roles: ['admin'] });
    mocks.generateAccess.mockRejectedValueOnce(new Error('menu down'));

    const result = await accessGuard(
      { fullPath: '/dashboard/app', meta: {}, name: 'Dashboard', path: '/dashboard/app' },
      { query: {} },
    );

    expect(result).toEqual({ path: '/offline', replace: true });
    expect(mocks.accessStore.setAccessMenus).not.toHaveBeenCalled();
    expect(mocks.accessStore.setAccessRoutes).not.toHaveBeenCalled();
    expect(mocks.accessStore.setIsAccessChecked).not.toHaveBeenCalledWith(true);
    expect(mocks.stopProgress).toHaveBeenCalled();
  });
});
