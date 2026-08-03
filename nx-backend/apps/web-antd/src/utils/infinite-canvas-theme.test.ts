import { describe, expect, it } from 'vitest';

import {
  ADMIN_THEME_MESSAGE_TYPE,
  createAdminThemeMessage,
  readAdminThemeSnapshot,
} from './infinite-canvas-theme';

describe('infinite canvas admin theme sender', () => {
  it('reads the current Vben semantic tokens from the document root', () => {
    const root = document.documentElement;
    root.classList.remove('dark');
    root.style.setProperty('--primary', '212 100% 45%');
    root.style.setProperty('--primary-foreground', '0 0% 98%');
    root.style.setProperty('--background', '0 0% 100%');
    root.style.setProperty('--foreground', '210 6% 21%');
    root.style.setProperty('--card', '0 0% 100%');
    root.style.setProperty('--card-foreground', '222.2 84% 4.9%');
    root.style.setProperty('--muted', '240 4.8% 95.9%');
    root.style.setProperty('--muted-foreground', '240 3.8% 46.1%');
    root.style.setProperty('--accent', '240 5% 96%');
    root.style.setProperty('--accent-foreground', '240 6% 10%');
    root.style.setProperty('--border', '240 5.9% 90%');
    root.style.setProperty('--ring', '222.2 84% 4.9%');
    root.style.setProperty('--radius', '0.5rem');

    expect(readAdminThemeSnapshot(root)).toMatchObject({
      theme: 'light',
      tokens: {
        primary: '212 100% 45%',
        background: '0 0% 100%',
        foreground: '210 6% 21%',
        border: '240 5.9% 90%',
        radius: '0.5rem',
      },
    });
  });

  it('wraps a theme snapshot in the versioned iframe message', () => {
    const snapshot = readAdminThemeSnapshot(document.documentElement);
    expect(createAdminThemeMessage(snapshot)).toEqual({
      type: ADMIN_THEME_MESSAGE_TYPE,
      payload: snapshot,
    });
  });
});
