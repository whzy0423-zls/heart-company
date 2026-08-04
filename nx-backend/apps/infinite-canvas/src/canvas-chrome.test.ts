import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const srcRoot = dirname(fileURLToPath(import.meta.url));

describe("canvas chrome", () => {
    it("does not render the extra shortcut and appearance entries", () => {
        const topBar = readFileSync(join(srcRoot, "components/canvas/canvas-top-bar.tsx"), "utf8");
        const zoomControls = readFileSync(join(srcRoot, "components/canvas/canvas-zoom-controls.tsx"), "utf8");
        const toolbar = readFileSync(join(srcRoot, "components/canvas/canvas-toolbar.tsx"), "utf8");

        expect(topBar).not.toContain("快捷键");
        expect(zoomControls).not.toContain("快捷键");
        expect(toolbar).not.toContain("tool-style");
        expect(toolbar).not.toContain("画布外观");
        expect(toolbar).not.toContain("AnimatedThemeToggler");
    });
});
