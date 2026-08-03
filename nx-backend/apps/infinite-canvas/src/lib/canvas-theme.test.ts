import { describe, expect, it } from "vitest";

import { canvasThemes } from "./canvas-theme";

describe("canvas theme", () => {
    it("uses the admin semantic colors instead of a standalone stone palette", () => {
        expect(canvasThemes.light.canvas.background).toBe("var(--canvas-background)");
        expect(canvasThemes.light.node.panel).toBe("var(--card)");
        expect(canvasThemes.light.node.activeStroke).toBe("var(--primary)");
        expect(canvasThemes.dark.toolbar.activeText).toBe("var(--primary)");
    });
});
