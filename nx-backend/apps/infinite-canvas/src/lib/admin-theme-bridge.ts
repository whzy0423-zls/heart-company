export const ADMIN_THEME_MESSAGE_TYPE = "nine-xing:admin-theme" as const;
export const ADMIN_THEME_APPLIED_EVENT = "nine-xing:admin-theme-applied" as const;

export type AdminThemeName = "light" | "dark";

export interface AdminThemeTokens {
    primary: string;
    primaryForeground: string;
    background: string;
    foreground: string;
    card: string;
    cardForeground: string;
    muted: string;
    mutedForeground: string;
    accent: string;
    accentForeground: string;
    border: string;
    ring: string;
    radius: string;
}

export interface AdminThemeSnapshot {
    theme: AdminThemeName;
    tokens: AdminThemeTokens;
}

export interface AdminThemeMessage {
    type: typeof ADMIN_THEME_MESSAGE_TYPE;
    payload: AdminThemeSnapshot;
}

const tokenNames: Array<keyof AdminThemeTokens> = [
    "primary",
    "primaryForeground",
    "background",
    "foreground",
    "card",
    "cardForeground",
    "muted",
    "mutedForeground",
    "accent",
    "accentForeground",
    "border",
    "ring",
    "radius",
];

const cssVariableNames: Record<keyof AdminThemeTokens, string> = {
    primary: "--primary",
    primaryForeground: "--primary-foreground",
    background: "--background",
    foreground: "--foreground",
    card: "--card",
    cardForeground: "--card-foreground",
    muted: "--muted",
    mutedForeground: "--muted-foreground",
    accent: "--accent",
    accentForeground: "--accent-foreground",
    border: "--border",
    ring: "--ring",
    radius: "--radius",
};

function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === "object" && value !== null;
}

export function isAdminThemeMessage(value: unknown): value is AdminThemeMessage {
    if (!isRecord(value) || value.type !== ADMIN_THEME_MESSAGE_TYPE || !isRecord(value.payload)) return false;
    const { payload } = value;
    if (payload.theme !== "light" && payload.theme !== "dark") return false;
    if (!isRecord(payload.tokens)) return false;
    const tokens = payload.tokens;
    return tokenNames.every((name) => typeof tokens[name] === "string" && tokens[name].trim().length > 0);
}

export function normalizeAdminColor(value: string): string {
    const normalized = value.trim();
    if (/^[+-]?(?:\d*\.)?\d+(?:deg|grad|rad|turn)?\s+[+-]?(?:\d*\.)?\d+%\s+[+-]?(?:\d*\.)?\d+%(?:\s*\/\s*[^\s]+)?$/.test(normalized)) {
        return `hsl(${normalized})`;
    }
    return normalized;
}

export function applyAdminTheme(root: HTMLElement, snapshot: AdminThemeSnapshot): void {
    for (const name of tokenNames) {
        const value = snapshot.tokens[name];
        root.style.setProperty(cssVariableNames[name], name === "radius" ? value.trim() : normalizeAdminColor(value));
    }
    root.classList.toggle("dark", snapshot.theme === "dark");
    root.style.colorScheme = snapshot.theme;
    root.dispatchEvent(new CustomEvent<AdminThemeSnapshot>(ADMIN_THEME_APPLIED_EVENT, { detail: snapshot }));
}

export function installAdminThemeBridge(targetWindow: Window = window): () => void {
    const onMessage = (event: MessageEvent<unknown>) => {
        if (targetWindow.parent === targetWindow) return;
        if (event.source !== targetWindow.parent || event.origin !== targetWindow.location.origin) return;
        if (!isAdminThemeMessage(event.data)) return;
        applyAdminTheme(targetWindow.document.documentElement, event.data.payload);
    };

    targetWindow.addEventListener("message", onMessage);
    return () => targetWindow.removeEventListener("message", onMessage);
}
