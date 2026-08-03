import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, extname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const srcRoot = dirname(fileURLToPath(import.meta.url));

function sourceFiles(path: string): string[] {
    return readdirSync(path).flatMap((name) => {
        const child = join(path, name);
        if (statSync(child).isDirectory()) return sourceFiles(child);
        return [".css", ".ts", ".tsx"].includes(extname(child)) && !name.endsWith(".test.ts") ? [child] : [];
    });
}

describe("admin theme consistency", () => {
    it("does not ship the upstream stone or beige palette", () => {
        const violations = sourceFiles(srcRoot)
            .map((path) => ({ path, source: readFileSync(path, "utf8") }))
            .filter(({ source }) => /stone-|#f1eee8|#ebe6dc|#57534e|#1c1917|#a8a29e|120,\s*113,\s*108/i.test(source))
            .map(({ path }) => path.replace(`${srcRoot}/`, ""));

        expect(violations).toEqual([]);
    });
});
