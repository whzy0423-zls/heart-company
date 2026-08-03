import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const srcRoot = dirname(fileURLToPath(import.meta.url));

describe("canvas root layout", () => {
    it("keeps the React root in the full-height layout chain", () => {
        const css = readFileSync(join(srcRoot, "styles/globals.css"), "utf8");
        const fullHeightRule = css.match(/(?:html|body|#root)[^{]*\{[^}]*height:\s*100%[^}]*\}/g)?.join("\n") ?? "";

        expect(fullHeightRule).toContain("#root");
        expect(css).toMatch(/#root\s*>\s*\*\s*\{[^}]*height:\s*100%/);
    });
});
