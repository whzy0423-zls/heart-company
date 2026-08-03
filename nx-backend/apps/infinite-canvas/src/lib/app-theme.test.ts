import { describe, expect, it } from "vitest";

import { getAntThemeConfig, parseCssRadius } from "./app-theme";

describe("Ant Design admin theme alignment", () => {
    it("uses the synchronized admin primary color and radius", () => {
        const config = getAntThemeConfig(false, {
            primary: "hsl(212 100% 45%)",
            primaryForeground: "hsl(0 0% 98%)",
            radius: 8,
        });

        expect(config.token).toMatchObject({
            borderRadius: 8,
            colorPrimary: "hsl(212 100% 45%)",
            colorTextLightSolid: "hsl(0 0% 98%)",
        });
    });

    it("converts the admin rem radius into Ant Design pixels", () => {
        expect(parseCssRadius("0.5rem", 16)).toBe(8);
        expect(parseCssRadius("10px", 16)).toBe(10);
    });
});
