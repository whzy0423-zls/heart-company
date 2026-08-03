import { describe, expect, it } from "vitest";

import { createCanvasNode } from "./canvas-node-factory";
import { CanvasNodeType } from "@/types/canvas";

describe("createCanvasNode", () => {
    it("creates a new config node without persisting a model override", () => {
        const node = createCanvasNode(CanvasNodeType.Config, { x: 200, y: 100 }, { generationMode: "video", model: "stale-image-model", count: 2 });
        expect(node.metadata).toMatchObject({ generationMode: "video", count: 2 });
        expect(node.metadata).not.toHaveProperty("model");
    });
});
