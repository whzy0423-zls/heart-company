import { describe, expect, it } from "vitest";

import { createCanvasNode } from "./canvas-node-factory";
import { CanvasNodeType } from "@/types/canvas";

describe("createCanvasNode", () => {
    it("creates a default config node without a model override", () => {
        const node = createCanvasNode(CanvasNodeType.Config, { x: 200, y: 100 }, { generationMode: "video", count: 2 });
        expect(node.metadata).toMatchObject({ generationMode: "video", count: 2 });
        expect(node.metadata).not.toHaveProperty("model");
    });

    it("preserves an explicit legacy model override", () => {
        const node = createCanvasNode(CanvasNodeType.Config, { x: 200, y: 100 }, { generationMode: "image", model: "legacy::image-model" });
        expect(node.metadata?.model).toBe("legacy::image-model");
    });
});
