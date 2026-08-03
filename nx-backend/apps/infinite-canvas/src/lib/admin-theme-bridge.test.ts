import { beforeEach, describe, expect, it } from "vitest";

import {
    ADMIN_THEME_MESSAGE_TYPE,
    applyAdminTheme,
    isAdminThemeMessage,
    normalizeAdminColor,
} from "./admin-theme-bridge";

describe("admin theme bridge", () => {
    beforeEach(() => {
        document.documentElement.className = "";
        document.documentElement.removeAttribute("style");
    });

    it("recognizes a complete versioned admin theme message", () => {
        expect(
            isAdminThemeMessage({
                type: ADMIN_THEME_MESSAGE_TYPE,
                payload: {
                    theme: "light",
                    tokens: {
                        primary: "212 100% 45%",
                        primaryForeground: "0 0% 98%",
                        background: "0 0% 100%",
                        foreground: "210 6% 21%",
                        card: "0 0% 100%",
                        cardForeground: "222.2 84% 4.9%",
                        muted: "240 4.8% 95.9%",
                        mutedForeground: "240 3.8% 46.1%",
                        accent: "240 5% 96%",
                        accentForeground: "240 6% 10%",
                        border: "240 5.9% 90%",
                        ring: "222.2 84% 4.9%",
                        radius: "0.5rem",
                    },
                },
            }),
        ).toBe(true);
    });

    it("normalizes Vben HSL channels without changing complete CSS colors", () => {
        expect(normalizeAdminColor("212 100% 45%")).toBe("hsl(212 100% 45%)");
        expect(normalizeAdminColor("222.34deg 10.43% 12.27%")).toBe("hsl(222.34deg 10.43% 12.27%)");
        expect(normalizeAdminColor("oklch(0.7 0.2 250)")).toBe("oklch(0.7 0.2 250)");
        expect(normalizeAdminColor("#1677ff")).toBe("#1677ff");
    });

    it("applies synchronized tokens and dark mode to the iframe root", () => {
        applyAdminTheme(document.documentElement, {
            theme: "dark",
            tokens: {
                primary: "212 100% 62%",
                primaryForeground: "0 0% 98%",
                background: "240 10% 4%",
                foreground: "0 0% 98%",
                card: "240 10% 8%",
                cardForeground: "0 0% 98%",
                muted: "240 4% 16%",
                mutedForeground: "240 5% 65%",
                accent: "240 4% 16%",
                accentForeground: "0 0% 98%",
                border: "240 4% 20%",
                ring: "212 100% 62%",
                radius: "0.5rem",
            },
        });

        expect(document.documentElement.classList.contains("dark")).toBe(true);
        expect(document.documentElement.style.getPropertyValue("--primary")).toBe("hsl(212 100% 62%)");
        expect(document.documentElement.style.getPropertyValue("--background")).toBe("hsl(240 10% 4%)");
        expect(document.documentElement.style.getPropertyValue("--radius")).toBe("0.5rem");
        expect(document.documentElement.style.colorScheme).toBe("dark");
    });
});
