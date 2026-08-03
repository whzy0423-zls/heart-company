import type { ThemeConfig } from "antd";
import { theme as antdTheme } from "antd";

export interface AntAdminThemeTokens {
    primary: string;
    primaryForeground: string;
    radius: number;
}

const fallbackTokens: AntAdminThemeTokens = {
    primary: "hsl(212 100% 45%)",
    primaryForeground: "hsl(0 0% 98%)",
    radius: 8,
};

export function parseCssRadius(value: string, rootFontSize = 16): number {
    const numeric = Number.parseFloat(value);
    if (!Number.isFinite(numeric)) return fallbackTokens.radius;
    return value.trim().endsWith("rem") ? numeric * rootFontSize : numeric;
}

export function getAntThemeConfig(dark: boolean, tokens: AntAdminThemeTokens = fallbackTokens): ThemeConfig {

    return {
        algorithm: dark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
        cssVar: { key: dark ? "infinite-canvas-dark" : "infinite-canvas-light" },
        token: {
            borderRadius: tokens.radius,
            colorPrimary: tokens.primary,
            colorInfo: tokens.primary,
            colorLink: tokens.primary,
            colorLinkHover: tokens.primary,
            colorLinkActive: tokens.primary,
            colorTextLightSolid: tokens.primaryForeground,
        },
        components: {
            Button: {
                primaryShadow: "none",
            },
            Menu: {
                itemSelectedColor: tokens.primary,
                darkItemSelectedColor: tokens.primary,
            },
        },
    };
}
