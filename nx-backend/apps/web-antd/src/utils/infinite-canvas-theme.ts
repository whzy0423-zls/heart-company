export const ADMIN_THEME_MESSAGE_TYPE = 'nine-xing:admin-theme' as const;

export type AdminThemeName = 'dark' | 'light';

export interface AdminThemeTokens {
  accent: string;
  accentForeground: string;
  background: string;
  border: string;
  card: string;
  cardForeground: string;
  foreground: string;
  muted: string;
  mutedForeground: string;
  primary: string;
  primaryForeground: string;
  radius: string;
  ring: string;
}

export interface AdminThemeSnapshot {
  theme: AdminThemeName;
  tokens: AdminThemeTokens;
}

const tokenVariables: Record<keyof AdminThemeTokens, string> = {
  accent: '--accent',
  accentForeground: '--accent-foreground',
  background: '--background',
  border: '--border',
  card: '--card',
  cardForeground: '--card-foreground',
  foreground: '--foreground',
  muted: '--muted',
  mutedForeground: '--muted-foreground',
  primary: '--primary',
  primaryForeground: '--primary-foreground',
  radius: '--radius',
  ring: '--ring',
};

export function readAdminThemeSnapshot(root: HTMLElement): AdminThemeSnapshot {
  const styles = getComputedStyle(root);
  const tokens = Object.fromEntries(
    Object.entries(tokenVariables).map(([name, variable]) => [
      name,
      styles.getPropertyValue(variable).trim(),
    ]),
  ) as unknown as AdminThemeTokens;
  const theme =
    root.classList.contains('dark') || styles.colorScheme === 'dark'
      ? 'dark'
      : 'light';

  return { theme, tokens };
}

export function createAdminThemeMessage(snapshot: AdminThemeSnapshot) {
  return {
    type: ADMIN_THEME_MESSAGE_TYPE,
    payload: snapshot,
  } as const;
}
